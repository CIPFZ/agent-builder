import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase267-scheduler-refresh-contract-smoke');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const shellPath = resolve(repoRoot, 'client', 'src', 'app', 'shell', 'WorkbenchShell.tsx');
const refreshPath = resolve(repoRoot, 'client', 'src', 'runtime', 'runtimeEventRefresh.ts');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');

mkdirSync(runtimeDevRoot, { recursive: true });

const adapter = readFileSync(adapterPath, 'utf8');
const types = readFileSync(typesPath, 'utf8');
const shell = readFileSync(shellPath, 'utf8');
const refresh = readFileSync(refreshPath, 'utf8');
const preview = readFileSync(previewPath, 'utf8');

assertIncludes(shell, 'const delay = runtimeEventRefreshDelay(event);', 'event envelope derives an explicit refresh policy');
assertIncludes(shell, 'if (delay === undefined)', 'unknown runtime events do not trigger full hydration');
assertIncludes(refresh, 'return undefined;', 'refresh policy defaults to no hydration');
assertIncludes(refresh, "'task.role.loaded'", 'role changes opt in to one durable refresh');
assertIncludes(shell, 'const refresh = adapter.refreshRuntimeState ?? adapter.refresh;', 'event-triggered path selects bounded Runtime hydration');
assertIncludes(shell, 'const nextViewModel = await refresh({ ...viewModelRef.current, mode: modeRef.current });', 'event-triggered path rereads durable workbench state');
assertNotIncludes(shell, 'event.payload', 'WorkbenchShell does not merge event payloads');
assertNotIncludes(shell, 'schedulerTaskCandidates', 'WorkbenchShell does not own scheduler candidate state');
assertIncludes(refresh, "'tool.call.cancelled'", 'terminal tool cancellation triggers refresh');
assertIncludes(refresh, "'turn.cancelled'", 'terminal turn cancellation triggers refresh');
assertIncludes(refresh, "'artifact.ref.created'", 'artifact refs trigger durable refresh');
assertIncludes(adapter, 'runtimeActivityRefreshHint = viewEvent;', 'adapter stores the mapped event only as a refresh hint');
assertIncludes(adapter, "actionTargetsInclude(refreshTargets, 'run_scheduler_plan') ? hydrateRunSchedulerTaskCandidates(bridge, runProjection)", 'refresh hydration rereads scheduler candidates only for the requested target');
assertIncludes(adapter, 'bridge.RunSchedulerPlan?.({', 'scheduler candidate refresh uses durable bridge read');
assertIncludes(adapter, "mode: 'task_turn'", 'scheduler candidate refresh uses task-turn plan mode');
assertIncludes(adapter, 'byKey.set(`${candidate.runID}:${candidate.taskID}`', 'duplicate task events cannot duplicate candidate rows');
assertIncludes(adapter, 'executeEligible: item.canSchedule === true', 'terminal/actionability state comes from durable canSchedule');
assertIncludes(adapter, 'disabledReason: item.canSchedule ? undefined : item.preflightReason', 'blocked state comes from durable preflight reason');
assertNotIncludes(adapter, 'accepted ? true', 'execute action accepted flag is not used as UI actionability');
assertNotIncludes(adapter, 'executionStarted ? true', 'execute action started flag is not used as UI actionability');
assertIncludes(types, 'schedulerTaskCandidates?: RunSchedulerTaskCandidateViewModel[];', 'scheduler candidates remain hidden on RunProjection');
assertIncludes(preview, 'const schedulerTaskCandidates = run.schedulerTaskCandidates ?? [];', 'RunProjectionPreview renders durable scheduler candidates only');
assertNotIncludes(preview, 'runtimeActivityRefreshHint', 'RunProjectionPreview does not read runtime event hints');
assertNotIncludes(preview, 'readRunSchedulerPlan', 'RunProjectionPreview does not call hidden scheduler read');
assertNotIncludes(preview, 'executeRunTask', 'RunProjectionPreview does not call hidden execute action');

console.log('Phase 26.7 scheduler refresh contract smoke passed');

function assertIncludes(value, needle, label) {
  if (!value.includes(needle)) {
    throw new Error(`${label}: expected to include ${needle}`);
  }
}

function assertNotIncludes(value, needle, label) {
  if (value.includes(needle)) {
    throw new Error(`${label}: expected not to include ${needle}`);
  }
}
