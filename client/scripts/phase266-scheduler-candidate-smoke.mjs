import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase266-scheduler-candidate-smoke');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');

mkdirSync(runtimeDevRoot, { recursive: true });

const adapter = readFileSync(adapterPath, 'utf8');
const types = readFileSync(typesPath, 'utf8');
const preview = readFileSync(previewPath, 'utf8');

assertIncludes(types, 'RunSchedulerTaskCandidateViewModel', 'candidate view model type exists');
assertIncludes(types, 'schedulerTaskCandidates?: RunSchedulerTaskCandidateViewModel[];', 'RunProjection carries hidden scheduler candidates');
assertIncludes(types, 'readRunSchedulerPlan:', 'WorkbenchAdapter exposes hidden scheduler plan read');
assertIncludes(adapter, 'RunSchedulerPlan?: (req: RuntimeRunSchedulerPlanRequestDTO)', 'runtime bridge declares optional RunSchedulerPlan');
assertIncludes(adapter, 'return mapRunSchedulerPlanCandidates(await bridge.RunSchedulerPlan(toRunSchedulerPlanRequestDTO(request)));', 'adapter reads scheduler plans through Wails binding');
assertIncludes(adapter, 'executeEligible: item.canSchedule === true', 'execute eligibility maps only from durable canSchedule');
assertIncludes(adapter, 'disabledReason: item.canSchedule ? undefined : item.preflightReason', 'disabled reason maps from durable preflight reason');
assertIncludes(adapter, 'await hydrateRunSchedulerTaskCandidates(bridge, runProjection)', 'workbench hydration rereads durable scheduler candidates');
assertIncludes(adapter, 'byKey.set(`${candidate.runID}:${candidate.taskID}`', 'candidate rows dedupe by stable run/task key');
assertNotIncludes(preview, 'readRunSchedulerPlan', 'RunProjectionPreview does not expose scheduler plan UI control');
assertNotIncludes(preview, 'executeRunTask', 'RunProjectionPreview still does not expose executeRunTask UI control');

console.log('Phase 26.6 scheduler candidate smoke passed');

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
