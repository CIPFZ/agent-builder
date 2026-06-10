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

assertIncludes(shell, 'adapter.subscribeRuntimeEvents((event) => scheduleRuntimeRefresh(runtimeEventRefreshDelay(event)))', 'event envelope only schedules a refresh');
assertIncludes(shell, 'const nextViewModel = await adapter.refresh({ ...viewModelRef.current, mode: modeRef.current });', 'event-triggered path rereads durable workbench state');
assertNotIncludes(shell, 'event.payload', 'WorkbenchShell does not merge event payloads');
assertNotIncludes(shell, 'schedulerTaskCandidates', 'WorkbenchShell does not own scheduler candidate state');
assertIncludes(refresh, "'tool.call.cancelled'", 'terminal tool cancellation triggers refresh');
assertIncludes(refresh, "'turn.cancelled'", 'terminal turn cancellation triggers refresh');
assertIncludes(refresh, "'artifact.ref.created'", 'artifact refs trigger durable refresh');
assertIncludes(adapter, 'runtimeActivityRefreshHint = event;', 'adapter stores event only as a refresh hint');
assertIncludes(adapter, 'await hydrateRunSchedulerTaskCandidates(bridge, runProjection)', 'refresh hydration rereads scheduler candidates');
assertIncludes(adapter, 'bridge.RunSchedulerPlan?.({', 'scheduler candidate refresh uses durable bridge read');
assertIncludes(adapter, "mode: 'task_turn'", 'scheduler candidate refresh uses task-turn plan mode');
assertIncludes(adapter, 'byKey.set(`${candidate.runID}:${candidate.taskID}`', 'duplicate task events cannot duplicate candidate rows');
assertIncludes(adapter, 'executeEligible: item.canSchedule === true', 'terminal/actionability state comes from durable canSchedule');
assertIncludes(adapter, 'disabledReason: item.canSchedule ? undefined : item.preflightReason', 'blocked state comes from durable preflight reason');
assertNotIncludes(adapter, 'accepted ? true', 'execute action accepted flag is not used as UI actionability');
assertNotIncludes(adapter, 'executionStarted ? true', 'execute action started flag is not used as UI actionability');
assertIncludes(types, 'schedulerTaskCandidates?: RunSchedulerTaskCandidateViewModel[];', 'scheduler candidates remain hidden on RunProjection');
assertNotIncludes(preview, 'schedulerTaskCandidates', 'RunProjectionPreview does not render scheduler candidates yet');
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
