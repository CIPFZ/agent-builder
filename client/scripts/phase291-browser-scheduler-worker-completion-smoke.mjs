import { createWriteStream, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawn } from 'node:child_process';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const clientRoot = resolve(repoRoot, 'client');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev');
const harnessRoot = resolve(runtimeDevRoot, 'phase29-worker-completion-browser-smoke');
const manifestPath = resolve(harnessRoot, 'harness-manifest.json');
const stopPath = resolve(harnessRoot, 'harness-stop');
const playwrightConfigPath = resolve(harnessRoot, 'playwright.config.mjs');
const specPath = resolve(harnessRoot, 'phase291-browser-scheduler-worker-completion.spec.mjs');

assertInside(runtimeDevRoot, harnessRoot);
rmSync(harnessRoot, { force: true, recursive: true });
mkdirSync(harnessRoot, { recursive: true });

const children = [];

try {
  await assertPlaywrightAvailable();
  const go = startProcess('go-test', 'go', [
    'test',
    './internal/runtime',
    '-run',
    'TestPhase291BrowserSchedulerWorkerCompletionHarnessServer',
    '-count=1',
    '-v',
  ], repoRoot, {
    AGENT_BUILDER_PHASE291_BROWSER_HARNESS: '1',
    AGENT_BUILDER_PHASE291_HARNESS_ROOT: harnessRoot,
    AGENT_BUILDER_PHASE291_HARNESS_TIMEOUT_SECONDS: '180',
  });

  const manifest = await waitForManifest(go);
  const vitePort = await freePort();
  const vite = startProcess('vite', process.execPath, [
    viteCLIPath(),
    '--host',
    '127.0.0.1',
    '--port',
    String(vitePort),
    '--strictPort',
  ], clientRoot, {
    VITE_AGENT_BUILDER_RUNTIME_URL: manifest.runtimeURL,
    VITE_AGENT_BUILDER_RUNTIME_TOKEN: manifest.runtimeToken,
  });
  const viteURL = `http://127.0.0.1:${vitePort}/`;
  await waitForHTTP(viteURL, vite);

  writePlaywrightSpec(specPath);
  writePlaywrightConfig(playwrightConfigPath);
  await runProcess('playwright', process.execPath, [
    playwrightCLIPath(),
    'test',
    '--config',
    playwrightConfigPath,
    '--reporter=line',
    '--workers=1',
  ], repoRoot, {
    PHASE291_MANIFEST: manifestPath,
    PHASE291_VITE_URL: viteURL,
  });

  writeFileSync(stopPath, 'done\n', { encoding: 'utf8' });
  await waitForExit(go, 15_000);
  console.log('Phase 29.1 browser scheduler worker completion smoke passed');
} finally {
  if (!existsSync(stopPath)) {
    writeFileSync(stopPath, 'cleanup\n', { encoding: 'utf8' });
  }
  await cleanupChildren();
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

async function waitForManifest(child) {
  const started = Date.now();
  while (Date.now() - started < 60_000) {
    if (existsSync(manifestPath)) {
      return JSON.parse(readFileSync(manifestPath, 'utf8'));
    }
    if (child.exitCode !== null) {
      throw new Error(`runtime harness exited before writing manifest; see ${resolve(harnessRoot, 'go-test.log')}`);
    }
    await delay(200);
  }
  throw new Error(`timed out waiting for ${manifestPath}`);
}

async function waitForHTTP(url, child) {
  const started = Date.now();
  while (Date.now() - started < 60_000) {
    if (child.exitCode !== null) {
      throw new Error(`vite exited before becoming ready; see ${resolve(harnessRoot, 'vite.log')}`);
    }
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
    } catch {
      // Retry until Vite is listening.
    }
    await delay(250);
  }
  throw new Error(`timed out waiting for Vite at ${url}`);
}

function waitForExit(child, timeoutMS) {
  if (child.exitCode !== null) {
    return Promise.resolve();
  }
  return new Promise((resolveWait) => {
    const timer = setTimeout(resolveWait, timeoutMS);
    child.once('exit', () => {
      clearTimeout(timer);
      resolveWait();
    });
  });
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

function writePlaywrightConfig(path) {
  writeFileSync(path, `
export default {
  testDir: ${JSON.stringify(harnessRoot)},
  testMatch: 'phase291-browser-scheduler-worker-completion.spec.mjs',
  outputDir: ${JSON.stringify(resolve(harnessRoot, 'test-results'))},
  timeout: 60000,
  use: {
    trace: 'off',
    video: 'off',
    screenshot: 'only-on-failure',
  },
};
`, { encoding: 'utf8' });
}

function writePlaywrightSpec(path) {
  const playwrightTestURL = pathToFileURL(resolve(clientRoot, 'node_modules', '@playwright', 'test', 'index.mjs')).href;
  writeFileSync(path, `
import { test, expect } from ${JSON.stringify(playwrightTestURL)};
import { readFileSync } from 'node:fs';

const manifest = JSON.parse(readFileSync(process.env.PHASE291_MANIFEST, 'utf8'));
const viteURL = process.env.PHASE291_VITE_URL;
const authHeaders = { Authorization: \`Bearer \${manifest.runtimeToken}\` };

test('scheduler worker completion hydrates durable task result and refs', async ({ page, request }) => {
  page.on('response', (response) => {
    if (!response.ok() && response.url().includes('run-scheduler-plan')) {
      console.log(\`[browser:response] \${response.status()} \${response.url()}\`);
    }
  });

  const beforeRefs = await request.get(\`\${manifest.runtimeURL}/v1/refs?task_id=\${manifest.queuedTaskID}\`, { headers: authHeaders });
  expect(beforeRefs.ok()).toBeTruthy();
  expect(((await beforeRefs.json()).refs ?? [])).toHaveLength(0);

  await page.goto(viteURL);
  await expect(page.getByTestId('run-projection-preview')).toBeVisible({ timeout: 30000 });
  await expect(page.getByTestId('run-scheduler-candidates')).toBeVisible({ timeout: 15000 });
  await expect(page.getByTestId('run-scheduler-candidate')).toHaveCount(2);

  const queued = page.locator(\`[data-testid="run-scheduler-candidate"][data-task-id="\${manifest.queuedTaskID}"]\`);
  const terminal = page.locator(\`[data-testid="run-scheduler-candidate"][data-task-id="\${manifest.terminalTaskID}"]\`);
  await expect(queued.getByRole('button', { name: \`Execute task \${manifest.queuedTaskID}\` })).toBeEnabled();
  await expect(terminal.getByRole('button', { name: \`Execute task \${manifest.terminalTaskID}\` })).toBeDisabled();

  await queued.getByRole('button', { name: \`Execute task \${manifest.queuedTaskID}\` }).click();
  await expect(page.getByTestId('run-scheduler-candidate')).toHaveCount(2);

  await expect.poll(async () => {
    const response = await request.get(\`\${manifest.runtimeURL}/v1/tasks/\${manifest.queuedTaskID}\`, { headers: authHeaders });
    if (!response.ok()) {
      return \`http \${response.status()}\`;
    }
    const payload = await response.json();
    return payload.task?.status;
  }, { timeout: 15000 }).toBe('completed');

  const resultResponse = await request.get(\`\${manifest.runtimeURL}/v1/tasks/\${manifest.queuedTaskID}/result\`, { headers: authHeaders });
  expect(resultResponse.ok()).toBeTruthy();
  const resultPayload = await resultResponse.json();
  expect(resultPayload.result?.status).toBe('completed');
  expect(resultPayload.result?.artifactRefs?.length ?? 0).toBeGreaterThan(0);

  const refsResponse = await request.get(\`\${manifest.runtimeURL}/v1/refs?task_id=\${manifest.queuedTaskID}\`, { headers: authHeaders });
  expect(refsResponse.ok()).toBeTruthy();
  const refsPayload = await refsResponse.json();
  expect(refsPayload.refs ?? []).toHaveLength(1);
  expect(refsPayload.refs?.[0]?.uri ?? '').toContain('runtime://refs/');

  const projectionResponse = await request.get(\`\${manifest.runtimeURL}/v1/sessions/\${manifest.sessionID}/run-projection?limit=24\`, { headers: authHeaders });
  expect(projectionResponse.ok()).toBeTruthy();
  const projection = await projectionResponse.json();
  expect(projection.run?.producedArtifacts?.length ?? 0).toBeGreaterThan(0);
});
`, { encoding: 'utf8' });
}

function assertPlaywrightAvailable() {
  return runCheck(process.execPath, [playwrightCLIPath(), '--version'], repoRoot, 'Playwright CLI is required for Phase 29.1 smoke');
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
    const server = createServer();
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
  return resolve(clientRoot, 'node_modules', '@playwright', 'test', 'cli.js');
}

function viteCLIPath() {
  return resolve(clientRoot, 'node_modules', 'vite', 'bin', 'vite.js');
}

function needsShell(command) {
  return process.platform === 'win32' && /\.(cmd|bat)$/i.test(command);
}

function redact(value) {
  return value.replace(/Bearer\\s+[A-Za-z0-9._-]+/g, 'Bearer [redacted]');
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}
