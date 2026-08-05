import { chromium, expect } from '@playwright/test';
import { createWriteStream, existsSync, mkdirSync, rmSync } from 'node:fs';
import { createServer as createTCPServer } from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const desktopRoot = resolve(repoRoot, 'desktop');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev');
const harnessRoot = resolve(runtimeDevRoot, 'idle-memory-guard-packaged');
const webviewUserData = resolve(harnessRoot, 'webview-user-data');
const exePath = resolve(desktopRoot, 'bin', 'AgentBuilder.exe');
const bridgeModulePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const children = [];

assertInside(runtimeDevRoot, harnessRoot);
rmSync(harnessRoot, { force: true, recursive: true });
mkdirSync(webviewUserData, { recursive: true });

try {
  await runProcess('sync-frontend', 'wails3', ['task', 'sync:frontend'], desktopRoot);
  await runProcess('wails-build', 'wails3', ['task', 'build', 'EXTRA_TAGS=webview_test'], desktopRoot);
  if (!existsSync(exePath)) throw new Error(`AgentBuilder.exe was not found at ${exePath}`);

  const remoteDebugPort = await freePort();
  const app = startProcess('packaged-app', exePath, [], dirname(exePath), {
    AGENT_BUILDER_DESKTOP_ROOT: harnessRoot,
    AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT: String(remoteDebugPort),
    AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR: webviewUserData,
    AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_TREE_BYTES: '1',
    AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_WEBVIEW_BYTES: '1',
    AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_REQUIRED_SAMPLES: '3',
    AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_INTERVAL_MS: '1000',
  });
  await waitForCDP(remoteDebugPort, app);
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${remoteDebugPort}`);
  try {
    const page = await firstCDPPage(browser);
    await page.waitForLoadState('load');
    await callBridge(page, 'Status');

    const idle = await readGuard(page, { clientIdleMs: 180000 });
    expect(idle.eligible).toBe(true);
    expect(idle.memorySupported).toBe(true);
    expect(idle.processTreePrivateBytes).toBeGreaterThan(0);
    expect(idle.webViewPrivateBytes).toBeGreaterThan(0);
    expect(idle.highWater).toBe(true);
    expect(idle.sustained ?? false).toBe(false);
    expect(idle.requiredSamples).toBe(3);
    expect(idle.nextSampleAfterMs).toBe(1000);

    const draft = await readGuard(page, { clientIdleMs: 180000, hasUnsavedDraft: true });
    expect(draft.eligible).toBe(false);
    expect(draft.reason).toBe('unsaved_draft');
    expect(draft.memorySupported ?? false).toBe(false);

    await page.addInitScript(() => {
      const bootCount = Number(window.sessionStorage.getItem('agent-builder:memory-guard-smoke-boots') ?? '0');
      window.sessionStorage.setItem('agent-builder:memory-guard-smoke-boots', String(bootCount + 1));
      const realStartedAt = performance.now();
      const logicalStartedAt = Date.now();
      Date.now = () => logicalStartedAt + (performance.now() - realStartedAt) * 100;
    });
    await page.reload();
    await page.waitForLoadState('load');
    await expect.poll(async () => {
      try {
        return await page.evaluate(() => Number(window.sessionStorage.getItem('agent-builder:memory-guard-smoke-boots') ?? '0'));
      } catch {
        return 0;
      }
    }, { timeout: 60000, intervals: [1000] }).toBeGreaterThanOrEqual(2);
    await expect.poll(async () => {
      try {
        return await page.evaluate(() => window.sessionStorage.getItem('agent-builder:idle-memory-guard-ui-state'));
      } catch {
        return 'navigating';
      }
    }, { timeout: 15000, intervals: [250] }).toBeNull();
  } finally {
    await browser.close();
  }
  console.log(`Packaged idle memory guard smoke passed. Runtime root: ${harnessRoot}`);
} finally {
  await cleanupChildren();
}

function readGuard(page, request) {
  return callBridge(page, 'IdleMemoryGuard', [request]);
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
  const child = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    shell: needsShell(command),
    windowsHide: true,
  });
  children.push({ child, label });
  child.stdout.on('data', (chunk) => log.write(chunk));
  child.stderr.on('data', (chunk) => log.write(chunk));
  child.on('exit', (code, signal) => {
    log.write(`\n[${label}] exited code=${code ?? ''} signal=${signal ?? ''}\n`);
    log.end();
  });
  return child;
}

function runProcess(label, command, args, cwd) {
  return new Promise((resolveRun, rejectRun) => {
    const child = startProcess(label, command, args, cwd);
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

function needsShell(command) {
  return process.platform === 'win32' && /\.(cmd|bat)$/i.test(command);
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}
