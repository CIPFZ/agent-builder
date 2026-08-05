import { chromium, expect } from '@playwright/test';
import { createWriteStream, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { createServer as createTCPServer } from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const desktopRoot = resolve(repoRoot, 'desktop');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev');
const harnessRoot = resolve(runtimeDevRoot, 'memory-long-session');
const webviewUserData = resolve(harnessRoot, 'webview-user-data');
const manifestPath = resolve(harnessRoot, 'memory-acceptance-manifest.json');
const longReportPath = resolve(harnessRoot, 'long-session-memory.json');
const recoveryReportPath = resolve(harnessRoot, 'long-session-recovery-memory.json');
const exePath = resolve(desktopRoot, 'bin', 'AgentBuilder.exe');
const bridgeModulePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const profilerPath = resolve(repoRoot, 'scripts', 'conversation-memory-profile.ps1');
const children = [];

assertInside(runtimeDevRoot, harnessRoot);
rmSync(harnessRoot, { force: true, recursive: true });
mkdirSync(webviewUserData, { recursive: true });

try {
  await runProcess('sync-frontend', 'wails3', ['task', 'sync:frontend'], desktopRoot);
  await runProcess('wails-build', 'wails3', ['task', 'build', 'EXTRA_TAGS=webview_test'], desktopRoot);
  await runProcess('seed-long-session', 'go', [
    'test', './internal/runtime', '-run', 'TestMemoryAcceptanceSeedLongSession', '-count=1', '-v',
  ], repoRoot, {
    AGENT_BUILDER_MEMORY_ACCEPTANCE_SEED: '1',
    AGENT_BUILDER_MEMORY_ACCEPTANCE_ROOT: harnessRoot,
    AGENT_BUILDER_MEMORY_ACCEPTANCE_MANIFEST: manifestPath,
    AGENT_BUILDER_MEMORY_ACCEPTANCE_TURNS: '1000',
  });
  if (!existsSync(exePath)) throw new Error(`AgentBuilder.exe was not found at ${exePath}`);
  const manifest = readJSON(manifestPath);
  expect(manifest.turnCount).toBe(1000);

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
    const baseline = await callBridge(page, 'IdleMemoryGuard', [{ clientIdleMs: 180000 }]);
    expect(baseline.eligible).toBe(true);
    expect(baseline.memorySupported).toBe(true);
    expect(baseline.processTreePrivateBytes).toBeGreaterThan(0);
    const baselinePrivateMB = baseline.processTreePrivateBytes / 1024 / 1024;

    await callBridge(page, 'SelectSession', [manifest.sessionID]);
    await page.reload();
    await page.waitForLoadState('load');
    const windowSnapshot = await callBridge(page, 'SessionConversationSnapshotV2', [manifest.sessionID, { scope: 'window', limit: 30 }]);
    expect(windowSnapshot.turns).toHaveLength(30);
    expect(windowSnapshot.window?.turnIds).toHaveLength(30);
    expect(windowSnapshot.window?.hasMoreBefore).toBe(true);
    await expect(page.getByTestId('conversation-scroll-container')).toBeVisible({ timeout: 30000 });

    await runProcess('profile-long-session', 'powershell', [
      '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', profilerPath,
      '-ProcessName', 'AgentBuilder', '-RootProcessId', String(app.pid),
      '-Scenario', 'long-session', '-DurationSeconds', '120', '-IntervalSeconds', '2',
      '-MaxRendererPrivateMB', '800', '-RendererSustainedSamples', '3',
      '-OutputPath', longReportPath, '-NoJsonOutput',
    ], repoRoot);
    const longReport = readJSON(longReportPath);
    expect(longReport.passed).toBe(true);
    expect(longReport.peakTotalPrivateMB).toBeLessThanOrEqual(900);

    await page.getByRole('button', { name: /新对话/ }).first().click();
    await expect(page.getByTestId('conversation-scroll-container')).toHaveCount(0, { timeout: 30000 });
    await runProcess('profile-recovery', 'powershell', [
      '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', profilerPath,
      '-ProcessName', 'AgentBuilder', '-RootProcessId', String(app.pid),
      '-Scenario', 'recovery', '-DurationSeconds', '60', '-IntervalSeconds', '2',
      '-RecoveryBaselinePrivateMB', String(baselinePrivateMB),
      '-OutputPath', recoveryReportPath, '-NoJsonOutput',
    ], repoRoot);
    const recoveryReport = readJSON(recoveryReportPath);
    expect(recoveryReport.passed).toBe(true);
    expect(recoveryReport.recoveryDeltaMB).toBeLessThanOrEqual(150);

    console.log(JSON.stringify({
      sessionID: manifest.sessionID,
      persistedTurns: manifest.turnCount,
      mountedTurnWindow: windowSnapshot.turns.length,
      baselinePrivateMB: Number(baselinePrivateMB.toFixed(2)),
      peakPrivateMB: longReport.peakTotalPrivateMB,
      recoveryDeltaMB: recoveryReport.recoveryDeltaMB,
      longReportPath,
      recoveryReportPath,
    }));
  } finally {
    await browser.close();
  }
} finally {
  await cleanupChildren();
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

function needsShell(command) {
  return process.platform === 'win32' && /\.(cmd|bat)$/i.test(command);
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8').replace(/^\uFEFF/, ''));
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}
