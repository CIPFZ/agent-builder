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
const harnessRoot = resolve(runtimeDevRoot, 'memory-long-stream');
const webviewUserData = resolve(harnessRoot, 'webview-user-data');
const manifestPath = resolve(harnessRoot, 'memory-long-stream-manifest.json');
const reportPath = resolve(harnessRoot, 'long-stream-memory.json');
const providerStatsPath = resolve(harnessRoot, 'long-stream-provider-stats.json');
const cdpDiagnosticsPath = resolve(harnessRoot, 'long-stream-cdp-diagnostics.json');
const exePath = resolve(desktopRoot, 'bin', 'AgentBuilder.exe');
const bridgeModulePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const profilerPath = resolve(repoRoot, 'scripts', 'conversation-memory-profile.ps1');
const durationSeconds = positiveInteger(process.env.AGENT_BUILDER_MEMORY_LONG_STREAM_DURATION_SECONDS, 3600);
const warmupSeconds = durationSeconds >= 1800 ? 300 : Math.max(1, Math.floor(durationSeconds / 6));
const streamIntervalMs = 500;
const resourceMonitorIntervalMs = 10_000;
const cdpDiagnosticIntervalMs = positiveInteger(
  process.env.AGENT_BUILDER_MEMORY_LONG_STREAM_CDP_INTERVAL_SECONDS,
  30,
) * 1000;
const children = [];

assertInside(runtimeDevRoot, harnessRoot);
rmSync(harnessRoot, { force: true, recursive: true });
mkdirSync(webviewUserData, { recursive: true });

const provider = await startStreamingProvider(streamIntervalMs, (durationSeconds + 120) * 1000);
try {
  startKeepAwake();
  await runProcess('sync-frontend', 'wails3', ['task', 'sync:frontend'], desktopRoot);
  await runProcess('wails-build', 'wails3', ['task', 'build', 'EXTRA_TAGS=webview_test'], desktopRoot);
  await runProcess('seed-long-stream', 'go', [
    'test', './internal/runtime', '-run', 'TestMemoryAcceptanceSeedLongStream', '-count=1', '-v',
  ], repoRoot, {
    AGENT_BUILDER_MEMORY_LONG_STREAM_SEED: '1',
    AGENT_BUILDER_MEMORY_ACCEPTANCE_ROOT: harnessRoot,
    AGENT_BUILDER_MEMORY_ACCEPTANCE_MANIFEST: manifestPath,
    AGENT_BUILDER_MEMORY_LONG_STREAM_PROVIDER_URL: provider.url,
  });
  if (!existsSync(exePath)) throw new Error(`AgentBuilder.exe was not found at ${exePath}`);
  const manifest = readJSON(manifestPath);
  expect(typeof manifest.sessionID).toBe('string');

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
    await callBridge(page, 'SelectSession', [manifest.sessionID]);
    await page.reload();
    await page.waitForLoadState('load');
    await expect(page.getByTestId('conversation-scroll-container')).toBeVisible({ timeout: 30000 });
    await callBridge(page, 'Chat', [{
      sessionId: manifest.sessionID,
      prompt: 'Continuously stream a bounded response for the memory plateau acceptance scenario.',
    }]);
    await waitFor(() => provider.stats.byKind.turn.active === 1, 30000, 'streaming Turn provider request');

    const observed = { activeSessions: 0, modelRequests: 0, turnWorkingSets: 0 };
    let monitoring = true;
    const cdpDiagnostics = await startCDPDiagnostics(page, cdpDiagnosticIntervalMs);
    const monitor = (async () => {
      while (monitoring) {
        try {
          const status = await callBridge(page, 'Status');
          observed.activeSessions = Math.max(observed.activeSessions, status.activeSessions?.length ?? 0);
          const resources = status.resourceGovernor?.resources ?? [];
          observed.modelRequests = Math.max(observed.modelRequests, resourceUsage(resources, 'model_request'));
          observed.turnWorkingSets = Math.max(observed.turnWorkingSets, resourceUsage(resources, 'turn_working_set'));
        } catch {
          // Retry transient WebView navigation on the next sample.
        }
        await delay(resourceMonitorIntervalMs);
      }
    })();

    try {
      await runProcess('profile-long-stream', 'powershell', [
        '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', profilerPath,
        '-ProcessName', 'AgentBuilder', '-RootProcessId', String(app.pid),
        '-Scenario', 'long-stream', '-DurationSeconds', String(durationSeconds),
        '-MinDurationSeconds', String(durationSeconds), '-WarmupSeconds', String(warmupSeconds),
        '-IntervalSeconds', '2', '-MinSampleCount', String(Math.max(2, Math.floor(durationSeconds / 3))),
        '-MaxPrivateMB', '900', '-MaxGrowthMBPerMinute', '1',
        '-MaxRendererPrivateMB', '800', '-RendererSustainedSamples', '3',
        '-OutputPath', reportPath, '-NoJsonOutput',
      ], repoRoot);
    } finally {
      monitoring = false;
      await Promise.all([monitor, cdpDiagnostics.stop()]);
    }

    provider.persistStats();
    const report = readJSON(reportPath);
    const minimumChunks = Math.floor(durationSeconds * 1000 / streamIntervalMs * 0.8);
    expect(report.passed).toBe(true);
    expect(report.peakTotalPrivateMB).toBeLessThanOrEqual(900);
    expect(report.growthMBPerMinute).toBeLessThanOrEqual(1);
    expect(observed.activeSessions).toBe(1);
    expect(observed.modelRequests).toBeGreaterThan(0);
    expect(observed.modelRequests).toBeLessThanOrEqual(16);
    expect(observed.turnWorkingSets).toBe(1);
    expect(provider.stats.maxActive).toBeLessThanOrEqual(2);
    expect(provider.stats.byKind.turn.requestCount).toBe(1);
    expect(provider.stats.byKind.turn.active).toBe(1);
    expect(provider.stats.byKind.turn.chunks).toBeGreaterThanOrEqual(minimumChunks);
    expect(provider.stats.byKind.auxiliary.requestCount).toBe(0);

    console.log(JSON.stringify({
      durationSeconds: report.durationSeconds,
      warmupSeconds: report.warmupSeconds,
      analysisSamples: report.analysisSampleCount,
      growthMBPerMinute: report.growthMBPerMinute,
      peakPrivateMB: report.peakTotalPrivateMB,
      finalPrivateMB: report.totalPrivateMB,
      maximumModelRequests: observed.modelRequests,
      maximumTurnWorkingSets: observed.turnWorkingSets,
      providerTurnChunks: provider.stats.byKind.turn.chunks,
      providerTitleRequests: provider.stats.byKind.title.requestCount,
      reportPath,
      providerStatsPath,
      cdpDiagnosticsPath,
    }));
  } finally {
    await browser.close();
  }
} finally {
  await cleanupChildren();
  provider.persistStats();
  await provider.close();
}

function startStreamingProvider(intervalMs, responseLifetimeMs) {
  const stats = {
    active: 0,
    maxActive: 0,
    byKind: {
      turn: { active: 0, requestCount: 0, chunks: 0 },
      title: { active: 0, requestCount: 0, chunks: 0 },
      auxiliary: { active: 0, requestCount: 0, chunks: 0 },
    },
    requests: [],
  };
  const intervals = new Set();
  const timers = new Set();
  const server = createServer((req, res) => {
    const path = new URL(req.url ?? '/', 'http://127.0.0.1').pathname;
    if (path.endsWith('/models')) {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ object: 'list', data: [{ id: 'phase-27-local-model', object: 'model' }] }));
      return;
    }
    const chunks = [];
    let bytes = 0;
    req.on('data', (chunk) => {
      bytes += chunk.length;
      if (bytes <= 1024 * 1024) chunks.push(chunk);
    });
    req.on('end', () => beginStream(classifyProviderRequest(path, chunks, bytes)));

    const beginStream = (kind) => {
      const requestStats = stats.byKind[kind];
      requestStats.active++;
      requestStats.requestCount++;
      stats.active++;
      stats.maxActive = Math.max(stats.maxActive, stats.active);
      const record = { kind, path, startedAt: new Date().toISOString(), chunks: 0 };
      stats.requests.push(record);
      let finished = false;
      let streamInterval;
      let lifetimeTimer;
      const finish = () => {
        if (finished) return;
        finished = true;
        clearInterval(streamInterval);
        clearTimeout(lifetimeTimer);
        intervals.delete(streamInterval);
        timers.delete(lifetimeTimer);
        requestStats.active--;
        stats.active--;
        record.finishedAt = new Date().toISOString();
      };
      res.on('close', finish);
      res.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
      res.write('data: {"id":"long-stream","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}\n\n');
      streamInterval = setInterval(() => {
        if (res.destroyed) return;
        res.write('data: {"id":"long-stream","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"."},"finish_reason":null}]}\n\n');
        requestStats.chunks++;
        record.chunks++;
      }, intervalMs);
      intervals.add(streamInterval);
      lifetimeTimer = setTimeout(() => {
        timers.delete(lifetimeTimer);
        res.write('data: {"id":"long-stream","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\n');
        res.end('data: [DONE]\n\n');
        finish();
      }, responseLifetimeMs);
      timers.add(lifetimeTimer);
    };
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
          for (const interval of intervals) clearInterval(interval);
          for (const timer of timers) clearTimeout(timer);
          server.closeAllConnections();
          server.close(resolveClose);
        }),
      });
    });
    server.on('error', rejectProvider);
  });
}

function classifyProviderRequest(path, chunks, bytes) {
  if (!path.endsWith('/chat/completions') || bytes > 1024 * 1024) return 'auxiliary';
  try {
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    const messages = Array.isArray(body.messages) ? body.messages : [];
    const isTitle = messages.some((message) => typeof message?.content === 'string'
      && message.content.includes('Generate a concise title for the following content:'));
    return isTitle ? 'title' : 'turn';
  } catch {
    return 'auxiliary';
  }
}

function resourceUsage(resources, kind) {
  return resources.find((resource) => resource.kind === kind)?.inUseCount ?? 0;
}

async function startCDPDiagnostics(page, intervalMs) {
  const session = await page.context().newCDPSession(page);
  const samples = [];
  const errors = [];
  const startedAt = Date.now();
  let running = true;
  await session.send('Performance.enable');

  const persist = () => writeFileSync(cdpDiagnosticsPath, `${JSON.stringify({
    capturedAt: new Date().toISOString(),
    intervalMs,
    samples,
    errors,
  }, null, 2)}\n`);

  const sample = async () => {
    try {
      const [heap, performance, dom] = await Promise.all([
        session.send('Runtime.getHeapUsage'),
        session.send('Performance.getMetrics'),
        session.send('Memory.getDOMCounters'),
      ]);
      const metrics = Object.fromEntries(performance.metrics.map(({ name, value }) => [name, value]));
      samples.push({
        capturedAt: new Date().toISOString(),
        elapsedSeconds: Math.round((Date.now() - startedAt) / 100) / 10,
        usedHeapMB: bytesToMB(heap.usedSize),
        totalHeapMB: bytesToMB(heap.totalSize),
        embedderHeapUsedMB: bytesToMB(heap.embedderHeapUsedSize),
        backingStorageMB: bytesToMB(heap.backingStorageSize),
        documents: dom.documents,
        nodes: dom.nodes,
        jsEventListeners: dom.jsEventListeners,
        layoutCount: metrics.LayoutCount,
        recalcStyleCount: metrics.RecalcStyleCount,
        layoutDurationSeconds: metrics.LayoutDuration,
        recalcStyleDurationSeconds: metrics.RecalcStyleDuration,
        taskDurationSeconds: metrics.TaskDuration,
        jsHeapUsedMB: bytesToMB(metrics.JSHeapUsedSize),
        jsHeapTotalMB: bytesToMB(metrics.JSHeapTotalSize),
      });
      persist();
    } catch (error) {
      if (errors.length < 32) errors.push({ capturedAt: new Date().toISOString(), message: String(error) });
      persist();
    }
  };

  const loop = (async () => {
    while (running) {
      await sample();
      if (running) await delay(intervalMs);
    }
  })();

  return {
    stop: async () => {
      running = false;
      await loop;
      await session.detach().catch(() => undefined);
      persist();
    },
  };
}

function bytesToMB(value) {
  return typeof value === 'number' ? Math.round(value / 1024 / 1024 * 1000) / 1000 : undefined;
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

function startKeepAwake() {
  if (process.platform !== 'win32') return;
  const command = [
    "Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public static class AgentBuilderMemoryAcceptancePower { [DllImport(\"kernel32.dll\")] public static extern uint SetThreadExecutionState(uint flags); }'",
    '[AgentBuilderMemoryAcceptancePower]::SetThreadExecutionState([uint32]0x80000001) | Out-Null',
    `Wait-Process -Id ${process.pid}`,
  ].join('; ');
  startProcess('keep-awake', 'powershell', ['-NoProfile', '-Command', command], repoRoot);
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

async function waitFor(predicate, timeoutMs, label) {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    if (predicate()) return;
    await delay(100);
  }
  throw new Error(`timed out waiting for ${label}`);
}

function positiveInteger(value, fallback) {
  const parsed = Number.parseInt(value ?? '', 10);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
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
