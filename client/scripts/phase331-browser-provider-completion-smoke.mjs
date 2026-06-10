import { runBrowserSchedulerHarness } from './browserSchedulerHarness.mjs';

await runBrowserSchedulerHarness({
  goEnv: (harnessRoot) => ({
    AGENT_BUILDER_PHASE331_BROWSER_HARNESS: '1',
    AGENT_BUILDER_PHASE331_HARNESS_ROOT: harnessRoot,
    AGENT_BUILDER_PHASE331_HARNESS_TIMEOUT_SECONDS: '240',
  }),
  goTestName: 'TestPhase331BrowserProviderCompletionHarnessServer',
  harnessName: 'phase33-browser-provider-completion-smoke',
  manifestEnvName: 'PHASE331_MANIFEST',
  specFileName: 'phase331-browser-provider-completion.spec.mjs',
  successMessage: 'Phase 33.1 browser provider completion smoke passed',
  viteURLEnvName: 'PHASE331_VITE_URL',
  specSource: ({ manifestEnvName, playwrightTestURL, viteURLEnvName }) => `
import { test, expect } from ${JSON.stringify(playwrightTestURL)};
import { readFileSync } from 'node:fs';

const manifest = JSON.parse(readFileSync(process.env.${manifestEnvName}, 'utf8'));
const viteURL = process.env.${viteURLEnvName};
const authHeaders = { Authorization: \`Bearer \${manifest.runtimeToken}\` };

test('browser execute click completes through loopback provider-backed coordinator path', async ({ page, request }) => {
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
  }, { timeout: 30000 }).toBe('completed');

  const resultResponse = await request.get(\`\${manifest.runtimeURL}/v1/tasks/\${manifest.queuedTaskID}/result\`, { headers: authHeaders });
  expect(resultResponse.ok()).toBeTruthy();
  const resultPayload = await resultResponse.json();
  expect(resultPayload.result?.status).toBe('completed');
  expect(resultPayload.result?.summary ?? '').toContain('phase 33.1 browser provider completed');
  expect(resultPayload.result?.artifactRefs ?? []).toHaveLength(0);

  const refsResponse = await request.get(\`\${manifest.runtimeURL}/v1/refs?task_id=\${manifest.queuedTaskID}\`, { headers: authHeaders });
  expect(refsResponse.ok()).toBeTruthy();
  expect(((await refsResponse.json()).refs ?? [])).toHaveLength(0);

  const projectionResponse = await request.get(\`\${manifest.runtimeURL}/v1/sessions/\${manifest.sessionID}/run-projection?limit=24\`, { headers: authHeaders });
  expect(projectionResponse.ok()).toBeTruthy();
  const projection = await projectionResponse.json();
  expect(projection.run?.taskIds ?? []).toContain(manifest.queuedTaskID);
  expect(projection.run?.producedArtifacts ?? []).toHaveLength(0);

  const permissionsResponse = await request.get(\`\${manifest.runtimeURL}/v1/permissions\`, { headers: authHeaders });
  expect(permissionsResponse.ok()).toBeTruthy();
  const permissionsPayload = await permissionsResponse.json();
  expect((permissionsPayload.permissions ?? []).filter((permission) => permission.status === 'pending')).toHaveLength(0);
});
`,
});
