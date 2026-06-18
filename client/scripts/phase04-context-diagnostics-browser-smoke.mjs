import { runBrowserSchedulerHarness } from './browserSchedulerHarness.mjs';

await runBrowserSchedulerHarness({
  goEnv: (harnessRoot) => ({
    AGENT_BUILDER_PHASE4_CONTEXT_BROWSER_HARNESS: '1',
    AGENT_BUILDER_PHASE4_CONTEXT_HARNESS_ROOT: harnessRoot,
    AGENT_BUILDER_PHASE4_CONTEXT_HARNESS_TIMEOUT_SECONDS: '180',
  }),
  goTestName: 'TestPhase4BrowserContextDiagnosticsHarnessServer',
  harnessName: 'phase4-context-diagnostics-browser-smoke',
  manifestEnvName: 'PHASE4_CONTEXT_MANIFEST',
  specFileName: 'phase04-context-diagnostics-browser.spec.mjs',
  successMessage: 'Phase 4 context diagnostics browser smoke passed',
  viteURLEnvName: 'PHASE4_CONTEXT_VITE_URL',
  specSource: ({ manifestEnvName, playwrightTestURL, viteURLEnvName }) => `
import { test, expect } from ${JSON.stringify(playwrightTestURL)};
import { readFileSync } from 'node:fs';

const manifest = JSON.parse(readFileSync(process.env.${manifestEnvName}, 'utf8'));
const viteURL = process.env.${viteURLEnvName};
const authHeaders = { Authorization: \`Bearer \${manifest.runtimeToken}\` };

test('context diagnostics renders runtime prompt assembly DTO without raw prompt ownership', async ({ page, request }) => {
  const assembliesResponse = await request.get(\`\${manifest.runtimeURL}/v1/turns/\${manifest.turnID}/prompt-assemblies\`, { headers: authHeaders });
  expect(assembliesResponse.ok()).toBeTruthy();
  const assemblies = await assembliesResponse.json();
  expect(assemblies.assemblies?.[0]?.id).toBe(manifest.assemblyID);
  expect(assemblies.assemblies?.[0]?.messages?.rawPromptStored).toBe(false);
  expect(JSON.stringify(assemblies)).not.toContain('full prompt');

  await page.goto(viteURL);
  await expect(page.getByRole('button', { name: '打开右侧面板' })).toBeVisible({ timeout: 30000 });
  await page.getByRole('button', { name: '打开右侧面板' }).click();
  await page.getByRole('button', { name: 'audit 审查 Ctrl+Shift+G' }).click();

  const panel = page.getByTestId('context-diagnostics-panel');
  await expect(panel).toBeVisible({ timeout: 30000 });
  await expect(panel).toContainText('openai / phase-4-context-model');
  await expect(panel).toContainText('205 tokens');
  await expect(panel).toContainText('2 selected / 1 omitted');
  await expect(panel).toContainText('AGENTS.md');
  await expect(panel).toContainText('User memory');
  await expect(panel).toContainText('user memory unavailable in harness');
  await expect(panel).toContainText('microcompact');
  await expect(panel).toContainText('sha256:phase4-system');

  const state = await page.evaluate(() => {
    const panelText = document.querySelector('[data-testid="context-diagnostics-panel"]')?.textContent ?? '';
    return {
      panelText,
      rawPromptStored: panelText.includes('full prompt'),
      rawToolOutputStored: panelText.includes('complete terminal output'),
    };
  });
  expect(state.rawPromptStored).toBe(false);
  expect(state.rawToolOutputStored).toBe(false);
});
`,
});
