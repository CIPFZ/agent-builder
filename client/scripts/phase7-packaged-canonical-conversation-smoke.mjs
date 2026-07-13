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
const harnessRoot = resolve(runtimeDevRoot, 'phase7-packaged-canonical-conversation');
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

    // Seed the durable completed conversation before startup so the real app
    // must recover it through its historical canonical hydration path.
    await runProcess('seed-base', 'go', [
      'test', './internal/runtime', '-run', 'TestPhase7PackagedCanonicalSeed', '-count=1', '-v',
    ], repoRoot, {
      AGENT_BUILDER_PHASE362_PACKAGED_WEBVIEW_SEED: '1',
      AGENT_BUILDER_PHASE362_HARNESS_ROOT: harnessRoot,
      AGENT_BUILDER_PHASE362_PROVIDER_URL: provider.url,
      AGENT_BUILDER_PHASE362_MANIFEST: manifestPath,
    });
    let manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));

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
      page.on('console', (message) => console.log(`[packaged-console:${message.type()}] ${message.text()}`));
      page.on('pageerror', (error) => console.log(`[packaged-pageerror] ${String(error)}`));
      await page.waitForLoadState('load');
      await ensurePackagedRuntimeStarted(page);

      await expect(page.getByText(manifest.finalText, { exact: true })).toBeVisible({ timeout: 30000 });
      const process = page.locator(`[data-testid="timeline-turn-block"][data-turn-id="${manifest.completedTurnID}"] [data-testid="process-trace"]`);
      await expect(process).toHaveAttribute('data-process-open', 'false');
      await expect(page.getByTestId('agent-activity-monitor')).toBeVisible();
      await expect(page.getByTestId('todo-task-bar')).toHaveCount(0);
      await expect(process.getByTestId('todo-task-bar')).toHaveCount(0);
      const processToggle = process.getByRole('button').first();
      await processToggle.click();
      await expect(page.getByText(manifest.processText, { exact: true })).toBeVisible();
      const teamRow = page.getByTestId('timeline-agent-team-row');
      await expect(teamRow).toHaveAttribute('data-team-id', manifest.teamID);
      await teamRow.locator('button[aria-expanded]').click();
      const member = teamRow.locator(`[data-task-id="${manifest.taskID}"]`);
      await expect(member).toBeVisible();
      await member.click();
      await expect(page.locator(`[data-testid="agent-task-detail"][data-task-id="${manifest.taskID}"]`)).toBeVisible();
      await processToggle.click();
      await expect(process).toHaveAttribute('data-process-disclosure-mode', 'manual_closed');
      await page.waitForTimeout(100);
      const distanceToBottom = await page.getByTestId('conversation-scroll-container').evaluate((node) => node.scrollHeight - node.scrollTop - node.clientHeight);
      expect(distanceToBottom).toBeLessThanOrEqual(48);
      const conversation = await page.evaluate(async ({ bridgeModulePath, sessionID }) => {
        const bridge = await import(bridgeModulePath);
        return bridge.SessionConversationSnapshotV2(sessionID, {});
      }, { bridgeModulePath, sessionID: manifest.sessionID });
      expect(conversation.schemaVersion).toBe(2);
      expect(conversation.sessionId).toBe(manifest.sessionID);
      expect(conversation.turns ?? []).toHaveLength(1);
      expect((conversation.messages ?? []).some((message) => message.role === 'assistant' && message.content === manifest.finalText)).toBe(true);
      expect((conversation.agentTasks ?? []).some((task) => task.id === manifest.taskID && task.teamId === manifest.teamID)).toBe(true);
    } finally {
      await browser.close();
    }
  } finally {
    await provider.close();
  }
  console.log(`Phase 7 packaged canonical conversation smoke passed. Runtime root: ${harnessRoot}`);
} finally {
  await cleanupChildren();
}

async function readDurableDTOs(page, manifest) {
  return page.evaluate(async ({ bridgeModulePath, manifest }) => {
    const bridge = await import(bridgeModulePath);
    const [projection, queuedPlan, terminalPlan, permissions, result, refs, activity, conversation] = await Promise.all([
      bridge.RunProjection({ sessionId: manifest.sessionID, limit: 24 }),
      bridge.RunSchedulerPlan({ runId: manifest.runID, taskId: manifest.queuedTaskID, mode: 'task_turn' }),
      bridge.RunSchedulerPlan({ runId: manifest.runID, taskId: manifest.terminalTaskID, mode: 'task_turn' }),
      bridge.Permissions(),
      bridge.AgentTaskResult(manifest.queuedTaskID).catch((error) => ({ error: String(error) })),
      bridge.Refs({ taskId: manifest.queuedTaskID }),
      bridge.SessionActivity(manifest.sessionID),
      bridge.SessionConversationSnapshotV2(manifest.sessionID, {}),
    ]);
    return { projection, queuedPlan, terminalPlan, permissions, result, refs, activity, conversation };
  }, { bridgeModulePath, manifest });
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
