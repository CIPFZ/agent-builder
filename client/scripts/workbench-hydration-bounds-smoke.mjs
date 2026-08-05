import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const adapter = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const shell = await readFile(new URL('../src/app/shell/WorkbenchShell.tsx', import.meta.url), 'utf8');

assert.equal(
  shell.match(/adapter\.refreshRuntimeState \?\? adapter\.refresh/g)?.length,
  1,
  'runtime events must use the lightweight refresh path without a busy polling loop',
);
assert.match(adapter, /background: true,[\s\S]*refreshTargets: \['status', 'recovery', 'run', 'run_projection'\]/);
assert.match(adapter, /runtimeStateRefreshPromise/, 'background refreshes must be singleflight');
assert.match(adapter, /refreshDiagnostics\(current\)/, 'diagnostics must expose an explicit opt-in hydration path');
assert.match(shell, /onReviewOpen=\{refreshDiagnostics\}/, 'opening Review must trigger diagnostics hydration');
assert.doesNotMatch(adapter, /bridge\.SessionActivity\?\.\(/, 'hydration must never fall back to full SessionActivity');
assert.doesNotMatch(adapter, /hydrateAgentTasks/, 'canonical AgentTasks must not fan out through workbench hydration');
assert.doesNotMatch(adapter, /bridge\.AgentTask\?\.\(/, 'task detail must be user-driven');
assert.doesNotMatch(adapter, /bridge\.AgentTaskOutput\?\.\(/, 'task output must be user-driven');
assert.match(adapter, /!Array\.isArray\(sidebarProjection\?\.sessions\)/, 'Sessions may only be a SidebarProjection fallback');
assert.match(adapter, /!Array\.isArray\(sidebarProjection\?\.projects\)/, 'Projects may only be a SidebarProjection fallback');

console.log('workbench hydration bounds smoke passed');
