import { runBrowserSchedulerHarness } from './browserSchedulerHarness.mjs';

await runBrowserSchedulerHarness({
  goEnv: (harnessRoot) => ({
    AGENT_BUILDER_PHASE291_BROWSER_HARNESS: '1',
    AGENT_BUILDER_PHASE291_HARNESS_ROOT: harnessRoot,
    AGENT_BUILDER_PHASE291_HARNESS_TIMEOUT_SECONDS: '180',
  }),
  goTestName: 'TestPhase291BrowserSchedulerWorkerCompletionHarnessServer',
  harnessName: 'phase29-worker-completion-browser-smoke',
  manifestEnvName: 'PHASE291_MANIFEST',
  specFileName: 'phase291-browser-scheduler-worker-completion.spec.mjs',
  successMessage: 'Phase 29.1 browser scheduler worker completion smoke passed',
  viteURLEnvName: 'PHASE291_VITE_URL',
  specSource: ({ manifestEnvName, playwrightTestURL, viteURLEnvName }) => `
import { test, expect } from ${JSON.stringify(playwrightTestURL)};
import { readFileSync } from 'node:fs';

const manifest = JSON.parse(readFileSync(process.env.${manifestEnvName}, 'utf8'));
const viteURL = process.env.${viteURLEnvName};
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
  await expect(page.getByTestId('conversation-timeline')).toBeVisible({ timeout: 30000 });
  const queued = page.locator(\`[data-testid="timeline-agent-task-row"][data-task-id="\${manifest.queuedTaskID}"]\`);
  const terminal = page.locator(\`[data-testid="timeline-agent-task-row"][data-task-id="\${manifest.terminalTaskID}"]\`);
  await expect(queued).toBeVisible({ timeout: 15000 });
  await expect(terminal).toBeVisible();

  const executeResponse = await request.post(\`\${manifest.runtimeURL}/v1/runs/\${manifest.runID}/tasks/\${manifest.queuedTaskID}/execute\`, { headers: authHeaders });
  expect(executeResponse.ok()).toBeTruthy();

  await expect.poll(async () => {
    const response = await request.get(\`\${manifest.runtimeURL}/v1/agent-tasks/\${manifest.queuedTaskID}\`, { headers: authHeaders });
    if (!response.ok()) {
      return \`http \${response.status()}\`;
    }
    const payload = await response.json();
    return payload.task?.status;
  }, { timeout: 15000 }).toBe('completed');

  const resultResponse = await request.get(\`\${manifest.runtimeURL}/v1/agent-tasks/\${manifest.queuedTaskID}/result\`, { headers: authHeaders });
  expect(resultResponse.ok()).toBeTruthy();
  const resultPayload = await resultResponse.json();
  expect(resultPayload.result?.status).toBe('completed');
  expect(resultPayload.result?.artifactRefs?.length ?? 0).toBeGreaterThan(0);
  await expect(queued).toContainText('completed', { timeout: 15000 });

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
`,
});

process.exit(0);
