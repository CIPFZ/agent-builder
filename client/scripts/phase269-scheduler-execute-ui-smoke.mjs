import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase269-scheduler-execute-ui-smoke');
const shellPath = resolve(repoRoot, 'client', 'src', 'app', 'shell', 'WorkbenchShell.tsx');
const workspacePath = resolve(repoRoot, 'client', 'src', 'features', 'workspace', 'Workspace.tsx');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');

mkdirSync(runtimeDevRoot, { recursive: true });

const shell = readFileSync(shellPath, 'utf8');
const workspace = readFileSync(workspacePath, 'utf8');
const preview = readFileSync(previewPath, 'utf8');
const adapter = readFileSync(adapterPath, 'utf8');

assertIncludes(shell, 'const executeRunTask = async (runID: string, taskID: string)', 'WorkbenchShell has explicit task execute handler');
assertIncludes(shell, 'await adapter.executeRunTask({ ...viewModelRef.current, mode: modeRef.current }, runID, taskID)', 'WorkbenchShell delegates execution to adapter');
assertIncludes(shell, 'onRunTaskExecute={executeRunTask}', 'WorkbenchShell passes execute handler through Workspace');
assertIncludes(workspace, 'onRunTaskExecute?: (runID: string, taskID: string) => Promise<void>;', 'Workspace exposes optional execute callback');
assertIncludes(workspace, 'onExecuteTask={onRunTaskExecute}', 'Workspace passes execute callback to RunProjectionPreview');
assertIncludes(preview, 'const schedulerTaskCandidates = run.schedulerTaskCandidates ?? [];', 'RunProjectionPreview renders durable scheduler candidates');
assertIncludes(preview, 'const canExecute = candidate.executeEligible && Boolean(onExecuteTask);', 'UI enablement uses durable executeEligible');
assertIncludes(preview, 'await onExecuteTask(run.id, taskID);', 'UI click calls run/task execute callback');
assertIncludes(preview, 'data-testid="run-scheduler-candidates"', 'scheduler candidate rows are testable');
assertIncludes(preview, 'data-testid="run-scheduler-candidate"', 'scheduler candidate row is testable');
assertIncludes(preview, 'setPendingTaskID(taskID);', 'local pending state is scoped to clicked task');
assertIncludes(preview, "setTaskError(actionErrorMessage(error, 'Task execution failed'));", 'local errors remain ephemeral UI feedback');
assertNotIncludes(preview, 'accepted', 'RunProjectionPreview does not inspect execute action accepted payload');
assertNotIncludes(preview, 'executionStarted', 'RunProjectionPreview does not inspect execute action started payload');
assertNotIncludes(preview, 'runtimeActivityRefreshHint', 'RunProjectionPreview does not read runtime event hints');
assertIncludes(adapter, 'await bridge.ExecuteRunTask(runID, taskID);', 'adapter executes task through explicit action');
assertIncludes(adapter, 'return hydrateWorkbench(current, bridge);', 'adapter rehydrates durable DTOs after execute');

console.log('Phase 26.9 scheduler execute UI smoke passed');

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
