import { runBrowserSchedulerHarness } from './browserSchedulerHarness.mjs';

await runBrowserSchedulerHarness({
  goEnv: (harnessRoot) => ({
    AGENT_BUILDER_PHASE281_BROWSER_HARNESS: '1',
    AGENT_BUILDER_PHASE281_HARNESS_ROOT: harnessRoot,
    AGENT_BUILDER_PHASE281_HARNESS_TIMEOUT_SECONDS: '180',
  }),
  goTestName: 'TestPhase281BrowserSchedulerClickHarnessServer',
  harnessName: 'phase28-browser-scheduler-click',
  manifestEnvName: 'PHASE281_MANIFEST',
  specFileName: 'phase281-browser-scheduler-click.spec.mjs',
  successMessage: 'Phase 28.1 browser scheduler click harness passed',
  viteURLEnvName: 'PHASE281_VITE_URL',
  specSource: ({ manifestEnvName, playwrightTestURL, viteURLEnvName }) => `
import { test, expect } from ${JSON.stringify(playwrightTestURL)};
import { readFileSync } from 'node:fs';

const manifest = JSON.parse(readFileSync(process.env.${manifestEnvName}, 'utf8'));
const viteURL = process.env.${viteURLEnvName};
const authHeaders = { Authorization: \`Bearer \${manifest.runtimeToken}\` };

test('scheduler execute button hydrates from durable runtime DTOs', async ({ page, request }) => {
  page.on('response', (response) => {
    if (!response.ok() && response.url().includes('run-scheduler-plan')) {
      console.log(\`[browser:response] \${response.status()} \${response.url()}\`);
    }
  });

  const projectionResponse = await request.get(\`\${manifest.runtimeURL}/v1/sessions/\${manifest.sessionID}/run-projection?limit=24\`, { headers: authHeaders });
  expect(projectionResponse.ok()).toBeTruthy();
  const projection = await projectionResponse.json();
  expect(projection.run?.taskIds ?? []).toContain(manifest.queuedTaskID);
  expect(projection.run?.taskIds ?? []).toContain(manifest.terminalTaskID);

  const queuedPlanResponse = await request.get(\`\${manifest.runtimeURL}/v1/run-scheduler-plan?run_id=\${manifest.runID}&task_id=\${manifest.queuedTaskID}&mode=task_turn\`, { headers: authHeaders });
  expect(queuedPlanResponse.ok()).toBeTruthy();
  const queuedPlan = await queuedPlanResponse.json();
  expect(queuedPlan.plan?.items?.[0]?.canSchedule).toBe(true);

  const terminalPlanResponse = await request.get(\`\${manifest.runtimeURL}/v1/run-scheduler-plan?run_id=\${manifest.runID}&task_id=\${manifest.terminalTaskID}&mode=task_turn\`, { headers: authHeaders });
  expect(terminalPlanResponse.ok()).toBeTruthy();
  const terminalPlan = await terminalPlanResponse.json();
  expect(terminalPlan.plan?.items?.[0]?.canSchedule).toBe(false);

  await page.goto(viteURL);
  await expect(page.getByTestId('run-projection-preview')).toBeVisible({ timeout: 30000 });
  await expect(page.getByTestId('run-scheduler-candidates')).toBeVisible({ timeout: 15000 });
  await expect(page.getByTestId('run-scheduler-candidate')).toHaveCount(2);

  const queued = page.locator(\`[data-testid="run-scheduler-candidate"][data-task-id="\${manifest.queuedTaskID}"]\`);
  const terminal = page.locator(\`[data-testid="run-scheduler-candidate"][data-task-id="\${manifest.terminalTaskID}"]\`);
  await expect(queued).toBeVisible();
  await expect(terminal).toBeVisible();
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
  }, { timeout: 15000 }).toBe('running');

  const permissionsResponse = await request.get(\`\${manifest.runtimeURL}/v1/permissions\`, { headers: authHeaders });
  expect(permissionsResponse.ok()).toBeTruthy();
  const permissionsPayload = await permissionsResponse.json();
  expect((permissionsPayload.permissions ?? []).filter((permission) => permission.status === 'pending')).toHaveLength(0);
});
`,
});
