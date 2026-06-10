import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase263-execute-adapter-smoke');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');

mkdirSync(runtimeDevRoot, { recursive: true });

const adapter = readFileSync(adapterPath, 'utf8');
const types = readFileSync(typesPath, 'utf8');
const preview = readFileSync(previewPath, 'utf8');

assertIncludes(types, 'executeRunTask: (current: WorkbenchViewModel, runID: string, taskID: string)', 'WorkbenchAdapter exposes hidden executeRunTask method');
assertIncludes(adapter, 'ExecuteRunTask?: (runID: string, taskID: string)', 'runtime bridge module declares optional ExecuteRunTask');
assertIncludes(adapter, 'await bridge.ExecuteRunTask(runID, taskID);', 'adapter calls explicit execute action');
assertIncludes(adapter, 'return hydrateWorkbench(current, bridge);', 'adapter rehydrates durable DTOs after execute action');
assertIncludes(adapter, '/v1/runs/${encodeURIComponent(runID)}/tasks/${encodeURIComponent(taskID)}/execute', 'HTTP fallback targets explicit execute route');
assertNotIncludes(preview, 'executeRunTask', 'RunProjectionPreview does not expose executeRunTask UI control');

console.log('Phase 26.3 execute adapter smoke passed');

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
