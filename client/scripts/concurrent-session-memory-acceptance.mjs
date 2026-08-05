import { chromium, expect } from '@playwright/test';
import { createWriteStream, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { createServer as createTCPServer } from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const desktopRoot = resolve(repoRoot, 'desktop');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev');
const harnessRoot = resolve(runtimeDevRoot, 'memory-concurrency');
const webviewUserData = resolve(harnessRoot, 'webview-user-data');
const manifestPath = resolve(harnessRoot, 'memory-concurrency-manifest.json');
const reportPath = resolve(harnessRoot, 'concurrent-session-memory.json');
const providerStatsPath = resolve(harnessRoot, 'concurrent-session-provider-stats.json');
const exePath = resolve(desktopRoot, 'bin', 'AgentBuilder.exe');
const bridgeModulePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const profilerPath = resolve(repoRoot, 'scripts', 'conversation-memory-profile.ps1');
const children = [];

assertInside(runtimeDevRoot, harnessRoot);
rmSync(harnessRoot, { force: true, recursive: true });
mkdirSync(webviewUserData, { recursive: true });

const provider = await startSlowProvider(15000);
try {
  await runProcess('sync-frontend', 'wails3', ['task', 'sync:frontend'], desktopRoot);
  await runProcess('wails-build', 'wails3', ['task', 'build', 'EXTRA_TAGS=webview_test'], desktopRoot);
  await runProcess('seed-concurrent-sessions', 'go', [
    'test', './internal/runtime', '-run', 'TestMemoryAcceptanceSeedConcurrentSessions', '-count=1', '-v',
  ], repoRoot, {
    AGENT_BUILDER_MEMORY_CONCURRENCY_SEED: '1',
    AGENT_BUILDER_MEMORY_ACCEPTANCE_ROOT: harnessRoot,
    AGENT_BUILDER_MEMORY_ACCEPTANCE_MANIFEST: manifestPath,
    AGENT_BUILDER_MEMORY_CONCURRENCY_PROVIDER_URL: provider.url,
    AGENT_BUILDER_MEMORY_CONCURRENCY_SESSIONS: '100',
  });
  if (!existsSync(exePath)) throw new Error(`AgentBuilder.exe was not found at ${exePath}`);
  const manifest = readJSON(manifestPath);
  expect(manifest.sessionCount).toBe(100);
  expect(manifest.sessionIDs).toHaveLength(100);

  const remoteDebugPort = await freePort();
  const app = startProcess('packaged-app', exePath, [], dirname(exePath), {
    AGENT_BUILDER_DESKTOP_ROOT: harnessRoot,
    AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT: String(remoteDebugPort),
    AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR: webviewUserData,
  });
  await waitForCDP(remoteDebugPort, app);
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${remoteDebugPort}`);
  try {
    const page = await firstCDPPage(browser);
    await page.waitForLoadState('load');
    await callBridge(page, 'Status');
    await page.evaluate(async ({ bridgeModulePath, sessionIDs }) => {
      const bridge = await import(bridgeModulePath);
      await Promise.all(sessionIDs.map((sessionId, index) => bridge.Chat({
        sessionId,
        prompt: `Concurrency memory acceptance prompt ${index + 1}`,
      })));
    }, { bridgeModulePath, sessionIDs: manifest.sessionIDs });

    await callBridge(page, 'SelectSession', [manifest.sessionIDs[0]]);
    await page.reload();
    await page.waitForLoadState('load');
    await expect(page.getByTestId('conversation-scroll-container')).toBeVisible({ timeout: 30000 });

    const observed = { activeSessions: 0, modelRequests: 0, turnWorkingSets: 0 };
    let monitoring = true;
    const monitor = (async () => {
      while (monitoring) {
        try {
          const status = await callBridge(page, 'Status');
          observed.activeSessions = Math.max(observed.activeSessions, status.activeSessions?.length ?? 0);
          const resources = status.resourceGovernor?.resources ?? [];
          observed.modelRequests = Math.max(observed.modelRequests, resourceUsage(resources, 'model_request'));
          observed.turnWorkingSets = Math.max(observed.turnWorkingSets, resourceUsage(resources, 'turn_working_set'));
        } catch {
          // A transient WebView navigation is retried by the next sample.
        }
        await delay(250);
      }
    })();

    await runProcess('profile-concurrency', 'powershell', [
      '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', profilerPath,
      '-ProcessName', 'AgentBuilder', '-RootProcessId', String(app.pid),
      '-Scenario', 'active-conversation', '-DurationSeconds', '120', '-MinDurationSeconds', '120',
      '-IntervalSeconds', '2', '-MaxPrivateMB', '1000',
      '-MaxRendererPrivateMB', '800', '-RendererSustainedSamples', '3',
      '-OutputPath', reportPath, '-NoJsonOutput',
    ], repoRoot);
    monitoring = false;
    await monitor;

    const report = readJSON(reportPath);
    expect(report.passed).toBe(true);
    expect(report.peakTotalPrivateMB).toBeLessThanOrEqual(1000);
    expect(observed.activeSessions).toBe(100);
    expect(observed.modelRequests).toBeGreaterThan(0);
    expect(observed.modelRequests).toBeLessThanOrEqual(16);
    expect(observed.turnWorkingSets).toBeGreaterThan(0);
    expect(observed.turnWorkingSets).toBeLessThanOrEqual(32);
    provider.persistStats();
    expect(provider.stats.model.maxComputing).toBeLessThanOrEqual(16);
    expect(provider.stats.model.maxOpenResponses).toBeLessThanOrEqual(16);
    expect(provider.stats.byKind.turn).toBe(100);
    expect(provider.stats.byKind.title).toBeLessThanOrEqual(2);
    expect(provider.stats.byKind.auxiliary).toBe(0);

    console.log(JSON.stringify({
      persistentSessions: manifest.sessionCount,
      maximumLogicalActiveSessions: observed.activeSessions,
      maximumTurnWorkingSets: observed.turnWorkingSets,
      maximumModelRequests: observed.modelRequests,
      maximumProviderConcurrency: provider.stats.model.maxComputing,
      maximumProviderOpenResponses: provider.stats.model.maxOpenResponses,
      providerRequests: provider.stats.model.requestCount,
      providerTurnRequests: provider.stats.byKind.turn,
      providerTitleRequests: provider.stats.byKind.title,
      providerStatsPath,
      peakPrivateMB: report.peakTotalPrivateMB,
      finalPrivateMB: report.totalPrivateMB,
      reportPath,
    }));
  } finally {
    await browser.close();
  }
} finally {
  await cleanupChildren();
  provider.persistStats();
  await provider.close();
}

function resourceUsage(resources, kind) {
  return resources.find((resource) => resource.kind === kind)?.inUseCount ?? 0;
}

function startSlowProvider(delayMs) {
  const stats = {
    requestCount: 0,
    byPath: {},
    model: { computing: 0, maxComputing: 0, openResponses: 0, maxOpenResponses: 0, requestCount: 0 },
    byKind: { turn: 0, title: 0, auxiliary: 0 },
    requests: [],
  };
  const timers = new Set();
  const server = createServer((req, res) => {
    const path = new URL(req.url ?? '/', 'http://127.0.0.1').pathname;
    stats.requestCount++;
    stats.byPath[path] = (stats.byPath[path] ?? 0) + 1;
    const record = {
      id: stats.requestCount,
      method: req.method ?? '',
      path,
      startedAt: new Date().toISOString(),
      startedMs: performance.now(),
    };
    stats.requests.push(record);
    if (path.endsWith('/models')) {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ object: 'list', data: [{ id: 'phase-27-local-model', object: 'model' }] }));
      record.payloadCompletedAt = new Date().toISOString();
      record.payloadCompletedMs = performance.now();
      res.on('close', () => {
        record.closedAt = new Date().toISOString();
        record.closedMs = performance.now();
      });
      return;
    }
    record.category = path.endsWith('/chat/completions') ? 'model' : 'auxiliary';
    const bodyChunks = [];
    let bodyBytes = 0;
    req.on('data', (chunk) => {
      bodyBytes += chunk.length;
      if (bodyBytes <= 1024 * 1024) bodyChunks.push(chunk);
    });
    req.on('end', () => {
      record.kind = classifyProviderRequest(record.category, bodyChunks, bodyBytes);
      stats.byKind[record.kind]++;
    });
    if (record.category === 'model') {
      stats.model.computing++;
      stats.model.openResponses++;
      stats.model.requestCount++;
      stats.model.maxComputing = Math.max(stats.model.maxComputing, stats.model.computing);
      stats.model.maxOpenResponses = Math.max(stats.model.maxOpenResponses, stats.model.openResponses);
    }
    let payloadFinished = false;
    const finishPayload = () => {
      if (payloadFinished) return;
      payloadFinished = true;
      record.payloadCompletedAt = new Date().toISOString();
      record.payloadCompletedMs = performance.now();
      if (record.category === 'model') stats.model.computing--;
    };
    let closed = false;
    res.on('close', () => {
      if (closed) return;
      closed = true;
      record.closedAt = new Date().toISOString();
      record.closedMs = performance.now();
      if (record.category === 'model') stats.model.openResponses--;
      finishPayload();
    });
    res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
    res.write('data: {"id":"concurrency","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}\n\n');
    const timer = setTimeout(() => {
      timers.delete(timer);
      res.write('data: {"id":"concurrency","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"concurrency memory acceptance completed"},"finish_reason":null}]}\n\n');
      res.write('data: {"id":"concurrency","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":6,"total_tokens":14}}\n\n');
      res.end('data: [DONE]\n\n');
      finishPayload();
    }, delayMs);
    timers.add(timer);
  });
  return new Promise((resolveProvider, rejectProvider) => {
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      resolveProvider({
        url: `http://127.0.0.1:${port}`,
        stats,
        persistStats: () => writeFileSync(providerStatsPath, `${JSON.stringify(stats, null, 2)}\n`),
        close: () => new Promise((resolveClose) => {
          for (const timer of timers) clearTimeout(timer);
          server.closeAllConnections();
          server.close(resolveClose);
        }),
      });
    });
    server.on('error', rejectProvider);
  });
}

function classifyProviderRequest(category, bodyChunks, bodyBytes) {
  if (category !== 'model' || bodyBytes > 1024 * 1024) return 'auxiliary';
  try {
    const body = JSON.parse(Buffer.concat(bodyChunks).toString('utf8'));
    const messages = Array.isArray(body.messages) ? body.messages : [];
    const isTitle = messages.some((message) => typeof message?.content === 'string'
      && message.content.includes('Generate a concise title for the following content:'));
    return isTitle ? 'title' : 'turn';
  } catch {
    return 'auxiliary';
  }
}

async function callBridge(page, method, args = []) {
  const started = Date.now();
  while (Date.now() - started < 30000) {
    try {
      return await page.evaluate(async ({ bridgeModulePath, method, args }) => {
        const bridge = await import(bridgeModulePath);
        return bridge[method](...args);
      }, { bridgeModulePath, method, args });
    } catch (error) {
      if (!String(error).includes('Execution context was destroyed')) throw error;
      await page.waitForLoadState('load').catch(() => undefined);
      await delay(250);
    }
  }
  throw new Error(`timed out calling Wails bridge method ${method}`);
}

async function firstCDPPage(browser) {
  const started = Date.now();
  while (Date.now() - started < 30000) {
    for (const context of browser.contexts()) {
      const page = context.pages().find((candidate) => candidate.url() !== 'about:blank') ?? context.pages()[0];
      if (page) return page;
    }
    await delay(250);
  }
  throw new Error('timed out waiting for packaged WebView page');
}

function startProcess(label, command, args, cwd, env = {}) {
  const log = createWriteStream(resolve(harnessRoot, `${label}.log`), { flags: 'a' });
  const child = spawn(command, args, { cwd, env: { ...process.env, ...env }, shell: needsShell(command), windowsHide: true });
  children.push({ child, label });
  child.stdout.on('data', (chunk) => log.write(chunk));
  child.stderr.on('data', (chunk) => log.write(chunk));
  child.on('exit', (code, signal) => {
    log.write(`\n[${label}] exited code=${code ?? ''} signal=${signal ?? ''}\n`);
    log.end();
  });
  return child;
}

function runProcess(label, command, args, cwd, env = {}) {
  return new Promise((resolveRun, rejectRun) => {
    const child = startProcess(label, command, args, cwd, env);
    child.on('exit', (code) => code === 0
      ? resolveRun()
      : rejectRun(new Error(`${label} failed with exit code ${code}; see ${resolve(harnessRoot, `${label}.log`)}`)));
  });
}

async function waitForCDP(port, child) {
  const url = `http://127.0.0.1:${port}/json/version`;
  const started = Date.now();
  while (Date.now() - started < 30000) {
    if (child.exitCode !== null) throw new Error('packaged app exited before CDP became ready');
    try {
      const payload = await (await fetch(url)).json();
      if (payload.webSocketDebuggerUrl) return;
    } catch {
      // WebView2 has not exposed its tagged test endpoint yet.
    }
    await delay(250);
  }
  throw new Error(`timed out waiting for CDP endpoint at ${url}`);
}

async function cleanupChildren() {
  for (const { child } of children.reverse()) {
    if (child.exitCode !== null || !child.pid) continue;
    await new Promise((resolveKill) => {
      const killer = spawn('taskkill', ['/pid', String(child.pid), '/t', '/f'], { windowsHide: true });
      killer.on('exit', resolveKill);
      killer.on('error', resolveKill);
    });
  }
}

function freePort() {
  return new Promise((resolvePort, rejectPort) => {
    const server = createTCPServer();
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      server.close(() => resolvePort(port));
    });
    server.on('error', rejectPort);
  });
}

function assertInside(root, target) {
  if (!resolve(target).startsWith(resolve(root))) throw new Error(`refusing path outside runtime-dev: ${target}`);
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8').replace(/^\uFEFF/, ''));
}

function needsShell(command) {
  return process.platform === 'win32' && /\.(cmd|bat)$/i.test(command);
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}
