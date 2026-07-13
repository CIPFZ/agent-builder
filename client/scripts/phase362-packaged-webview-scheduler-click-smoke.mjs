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
const harnessRoot = resolve(runtimeDevRoot, 'phase362-packaged-webview-scheduler-click');
const manifestPath = resolve(harnessRoot, 'harness-manifest.json');
const configRoot = resolve(harnessRoot, 'config');
const webviewUserData = resolve(harnessRoot, 'webview-user-data');
const exePath = resolve(desktopRoot, 'bin', 'AgentBuilder.exe');
const bridgeModulePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const children = [];

assertInside(runtimeDevRoot, harnessRoot);
rmSync(harnessRoot, { force: true, recursive: true });
mkdirSync(configRoot, { recursive: true });
mkdirSync(webviewUserData, { recursive: true });

try {
  await assertPlaywrightAvailable();
  const provider = await startLoopbackProvider();
  try {
    await runProcess('sync-frontend', 'wails3', ['task', 'sync:frontend'], desktopRoot);
    await runProcess('wails-build', 'wails3', ['task', 'build', 'EXTRA_TAGS=webview_test'], desktopRoot);
    if (!existsSync(exePath)) {
      throw new Error(`AgentBuilder.exe was not found at ${exePath}`);
    }

    await runProcess('seed', 'go', [
      'test', './internal/runtime', '-run', 'TestPhase362PackagedWebViewSchedulerSeed', '-count=1', '-v',
    ], repoRoot, {
      AGENT_BUILDER_PHASE362_PACKAGED_WEBVIEW_SEED: '1',
      AGENT_BUILDER_PHASE362_HARNESS_ROOT: harnessRoot,
      AGENT_BUILDER_PHASE362_PROVIDER_URL: provider.url,
      AGENT_BUILDER_PHASE362_MANIFEST: manifestPath,
    });

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
      await ensurePackagedRuntimeStarted(page);

      const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));

      await selectPackagedSession(page, manifest.sessionID);
      await page.reload();
      await page.waitForLoadState('load');

      await expect(page.getByTestId('run-projection-preview')).toBeVisible({ timeout: 30000 });
      await expect(page.getByTestId('run-scheduler-candidates')).toBeVisible({ timeout: 15000 });
      await expect(page.getByTestId('run-scheduler-candidate')).toHaveCount(2);

      const before = await readDurableDTOs(page, manifest);
      expect(before.projection.run?.taskIds ?? []).toContain(manifest.queuedTaskID);
      expect(before.projection.run?.taskIds ?? []).toContain(manifest.terminalTaskID);
      expect(before.queuedPlan.plan?.items?.[0]?.canSchedule).toBe(true);
      expect(before.terminalPlan.plan?.items?.[0]?.canSchedule).toBe(false);
      expect((before.permissions.permissions ?? []).filter((permission) => permission.status === 'pending')).toHaveLength(0);

      const queued = page.locator(`[data-testid="run-scheduler-candidate"][data-task-id="${manifest.queuedTaskID}"]`);
      const terminal = page.locator(`[data-testid="run-scheduler-candidate"][data-task-id="${manifest.terminalTaskID}"]`);
      await expect(queued.getByRole('button', { name: `Execute task ${manifest.queuedTaskID}` })).toBeEnabled();
      await expect(terminal.getByRole('button', { name: `Execute task ${manifest.terminalTaskID}` })).toBeDisabled();
      await queued.getByRole('button', { name: `Execute task ${manifest.queuedTaskID}` }).click();
      await expect(page.getByTestId('run-scheduler-candidate')).toHaveCount(2);

      await expect.poll(async () => {
        const result = await page.evaluate(async ({ bridgeModulePath, taskID }) => {
          const bridge = await import(bridgeModulePath);
          try {
            return await bridge.AgentTaskResult(taskID);
          } catch (error) {
            return { error: String(error) };
          }
        }, { bridgeModulePath, taskID: manifest.queuedTaskID });
        return result.result?.status ?? result.error ?? 'missing';
      }, { timeout: 45000 }).toBe('completed');

      const after = await readDurableDTOs(page, manifest);
      expect(after.result.result?.summary ?? '').toContain('phase 36.2 packaged WebView provider completed');
      expect(after.result.result?.artifactRefs ?? []).toHaveLength(0);
      expect(after.refs.refs ?? []).toHaveLength(0);
      expect(after.projection.run?.producedArtifacts ?? []).toHaveLength(0);
      expect((after.permissions.permissions ?? []).filter((permission) => permission.status === 'pending')).toHaveLength(0);
      expect(after.activity.turns ?? []).toHaveLength(1);
    } finally {
      await browser.close();
    }
  } finally {
    await provider.close();
  }
  console.log(`Phase 36.2 packaged WebView scheduler click smoke passed. Runtime root: ${harnessRoot}`);
} finally {
  await cleanupChildren();
}

async function readDurableDTOs(page, manifest) {
  return page.evaluate(async ({ bridgeModulePath, manifest }) => {
    const bridge = await import(bridgeModulePath);
    const [projection, queuedPlan, terminalPlan, permissions, result, refs, activity] = await Promise.all([
      bridge.RunProjection({ sessionId: manifest.sessionID, limit: 24 }),
      bridge.RunSchedulerPlan({ runId: manifest.runID, taskId: manifest.queuedTaskID, mode: 'task_turn' }),
      bridge.RunSchedulerPlan({ runId: manifest.runID, taskId: manifest.terminalTaskID, mode: 'task_turn' }),
      bridge.Permissions(),
      bridge.AgentTaskResult(manifest.queuedTaskID).catch((error) => ({ error: String(error) })),
      bridge.Refs({ taskId: manifest.queuedTaskID }),
      bridge.SessionActivity(manifest.sessionID),
    ]);
    return { projection, queuedPlan, terminalPlan, permissions, result, refs, activity };
  }, { bridgeModulePath, manifest });
}

async function selectPackagedSession(page, sessionID) {
  const started = Date.now();
  while (Date.now() - started < 30000) {
    try {
      await page.evaluate(async ({ bridgeModulePath, sessionID }) => {
        const bridge = await import(bridgeModulePath);
        await bridge.SelectSession(sessionID);
      }, { bridgeModulePath, sessionID });
      return;
    } catch (error) {
      if (!String(error).includes('Execution context was destroyed')) {
        throw error;
      }
      await page.waitForLoadState('load').catch(() => undefined);
      await delay(250);
    }
  }
  throw new Error(`timed out selecting packaged session ${sessionID}`);
}

async function ensurePackagedRuntimeStarted(page) {
  await page.evaluate(async ({ bridgeModulePath }) => {
    const bridge = await import(bridgeModulePath);
    await bridge.Status();
  }, { bridgeModulePath });
}

async function firstCDPPage(browser) {
  const started = Date.now();
  while (Date.now() - started < 30000) {
    for (const context of browser.contexts()) {
      const page = context.pages().find((candidate) => candidate.url() !== 'about:blank') ?? context.pages()[0];
      if (page) {
        return page;
      }
    }
    await delay(250);
  }
  throw new Error('timed out waiting for packaged WebView page');
}

function startLoopbackProvider() {
  const server = createServer((req, res) => {
    if (req.url?.endsWith('/models')) {
      writeJSON(res, { object: 'list', data: [{ id: 'phase-27-local-model', object: 'model' }] });
      return;
    }
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.write('data: {"id":"chatcmpl-phase362","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}\n\n');
    res.write('data: {"id":"chatcmpl-phase362","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"phase 36.2 packaged WebView provider completed"},"finish_reason":null}]}\n\n');
    res.write('data: {"id":"chatcmpl-phase362","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":6,"total_tokens":14}}\n\n');
    res.end('data: [DONE]\n\n');
  });
  return new Promise((resolveProvider, rejectProvider) => {
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      resolveProvider({
        url: `http://127.0.0.1:${port}`,
        close: () => new Promise((resolveClose) => server.close(resolveClose)),
      });
    });
    server.on('error', rejectProvider);
  });
}

function writeJSON(res, payload) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(payload));
}

function startProcess(label, command, args, cwd, env = {}) {
  const logPath = resolve(harnessRoot, `${label}.log`);
  const log = createWriteStream(logPath, { flags: 'a' });
  const child = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    shell: needsShell(command),
    windowsHide: true,
  });
  children.push({ child, label });
  writeFileSync(resolve(harnessRoot, `${label}.pid`), `${child.pid ?? ''}\n`, { encoding: 'utf8' });
  child.stdout.on('data', (chunk) => log.write(redact(chunk.toString())));
  child.stderr.on('data', (chunk) => log.write(redact(chunk.toString())));
  child.on('exit', (code, signal) => {
    log.write(`\n[${label}] exited code=${code ?? ''} signal=${signal ?? ''}\n`);
    log.end();
  });
  return child;
}

function runProcess(label, command, args, cwd, env = {}) {
  return new Promise((resolveRun, rejectRun) => {
    const child = startProcess(label, command, args, cwd, env);
    child.on('exit', (code) => {
      if (code === 0) {
        resolveRun();
        return;
      }
      rejectRun(new Error(`${label} failed with exit code ${code}; see ${resolve(harnessRoot, `${label}.log`)}`));
    });
  });
}

async function waitForCDP(port, child) {
  const url = `http://127.0.0.1:${port}/json/version`;
  const started = Date.now();
  while (Date.now() - started < 30000) {
    if (child.exitCode !== null) {
      throw new Error(`packaged app exited before CDP became ready; see ${resolve(harnessRoot, 'packaged-app.log')}`);
    }
    try {
      const response = await fetch(url);
      const payload = await response.json();
      if (payload.webSocketDebuggerUrl) {
        return;
      }
    } catch {
      // Retry until WebView2 exposes the tagged remote debugging endpoint.
    }
    await delay(250);
  }
  throw new Error(`timed out waiting for CDP endpoint at ${url}`);
}

async function cleanupChildren() {
  for (const { child, label } of children.reverse()) {
    if (child.exitCode !== null || !child.pid) {
      continue;
    }
    await killProcessTree(child.pid, label);
  }
}

function killProcessTree(pid, label) {
  return new Promise((resolveKill) => {
    const command = process.platform === 'win32' ? 'taskkill' : 'kill';
    const args = process.platform === 'win32' ? ['/pid', String(pid), '/t', '/f'] : ['-TERM', String(pid)];
    const child = spawn(command, args, { windowsHide: true });
    child.on('exit', () => resolveKill());
    child.on('error', () => {
      console.warn(`failed to terminate ${label} pid ${pid}`);
      resolveKill();
    });
  });
}

function assertPlaywrightAvailable() {
  return runCheck(process.execPath, [playwrightCLIPath(), '--version'], repoRoot, 'Playwright CLI is required for packaged WebView smoke');
}

function runCheck(command, args, cwd, message) {
  return new Promise((resolveCheck, rejectCheck) => {
    const child = spawn(command, args, { cwd, shell: needsShell(command), windowsHide: true });
    child.on('exit', (code) => {
      if (code === 0) {
        resolveCheck();
        return;
      }
      rejectCheck(new Error(message));
    });
    child.on('error', () => rejectCheck(new Error(message)));
  });
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
  const normalizedRoot = resolve(root);
  const normalizedTarget = resolve(target);
  if (!normalizedTarget.startsWith(normalizedRoot)) {
    throw new Error(`refusing path outside runtime-dev: ${target}`);
  }
}

function playwrightCLIPath() {
  return resolve(repoRoot, 'client', 'node_modules', '@playwright', 'test', 'cli.js');
}

function needsShell(command) {
  return process.platform === 'win32' && /\.(cmd|bat)$/i.test(command);
}

function redact(value) {
  return value.replace(/Bearer\s+[A-Za-z0-9._-]+/g, 'Bearer [redacted]');
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}
