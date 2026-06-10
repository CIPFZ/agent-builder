import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase2610-scheduler-ui-runtime-fixture-smoke');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const shellPath = resolve(repoRoot, 'client', 'src', 'app', 'shell', 'WorkbenchShell.tsx');

mkdirSync(runtimeDevRoot, { recursive: true });

const preview = readFileSync(previewPath, 'utf8');
const adapter = readFileSync(adapterPath, 'utf8');
const shell = readFileSync(shellPath, 'utf8');

const fixture = {
  id: 'run-fixture',
  schedulerTaskCandidates: [
    {
      id: 'task:queued-1',
      runID: 'run-fixture',
      taskID: 'queued-1',
      executeEligible: true,
      ownershipVerified: true,
      requiredPreflight: true,
    },
    {
      id: 'task:terminal-1',
      runID: 'run-fixture',
      taskID: 'terminal-1',
      executeEligible: false,
      disabledReason: 'terminal_task',
      ownershipVerified: true,
      requiredPreflight: true,
    },
    {
      id: 'task:queued-1-duplicate-terminal-event',
      runID: 'run-fixture',
      taskID: 'queued-1',
      executeEligible: false,
      disabledReason: 'duplicate_terminal_event',
      ownershipVerified: true,
      requiredPreflight: true,
    },
  ],
};

const deduped = new Map();
for (const candidate of fixture.schedulerTaskCandidates) {
  deduped.set(`${candidate.runID}:${candidate.taskID}`, candidate);
}

assert(deduped.size === 2, 'fixture duplicate task evidence dedupes by runID/taskID');
assert(fixture.schedulerTaskCandidates[0].executeEligible === true, 'queued fixture candidate is executable');
assert(fixture.schedulerTaskCandidates[1].executeEligible === false, 'terminal fixture candidate is disabled');
assert(fixture.schedulerTaskCandidates[1].disabledReason === 'terminal_task', 'terminal fixture carries durable disabled reason');

assertIncludes(preview, 'disabled={!canExecute || Boolean(pendingTaskID)}', 'preview disables blocked rows and concurrent local actions');
assertIncludes(preview, 'const disabledReason = candidate.disabledReason', 'preview displays durable disabled reason');
assertIncludes(preview, 'await onExecuteTask(run.id, taskID);', 'preview executes by durable run/task ids');
assertIncludes(preview, "setTaskError(actionErrorMessage(error, 'Task execution failed'));", 'preview action errors remain local feedback');
assertNotIncludes(preview, 'duplicate_terminal_event', 'preview does not hard-code fixture/event state');
assertIncludes(adapter, 'await bridge.ExecuteRunTask(runID, taskID);', 'adapter calls explicit execute action');
assertIncludes(adapter, 'return hydrateWorkbench(current, bridge);', 'adapter rehydrates after execution');
assertIncludes(shell, 'setViewModel(nextViewModel);', 'shell replaces UI state with hydrated view model');
assertNotIncludes(shell, 'executionStarted', 'shell does not infer state from action response payload');

console.log('Phase 26.10 scheduler UI runtime fixture smoke passed');

function assert(condition, label) {
  if (!condition) {
    throw new Error(label);
  }
}

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
