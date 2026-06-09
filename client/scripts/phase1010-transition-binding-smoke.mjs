import { mkdirSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase1010-transition-binding-smoke');
const bindingPath = resolve(
  repoRoot,
  'desktop',
  'frontend',
  'bindings',
  'github.com',
  'charmbracelet',
  'crush',
  'desktop',
  'runtimebridge.js',
);
const clientRuntimePath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const clientTypesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');

mkdirSync(runtimeDevRoot, { recursive: true });

const binding = readFileSync(bindingPath, 'utf8');
assertIncludes(binding, 'export function RunTransitionHistory(req)', 'Wails binding exports RunTransitionHistory');
assertIncludes(binding, '$Call.ByID', 'Wails binding delegates through generated call ID');

const clientRuntime = readFileSync(clientRuntimePath, 'utf8');
const clientTypes = readFileSync(clientTypesPath, 'utf8');
assertNotIncludes(clientRuntime, 'RunTransitionHistory', 'client runtime adapter does not consume RunTransitionHistory');
assertNotIncludes(clientTypes, 'RunTransitionHistory', 'client workbench types do not expose RunTransitionHistory');

console.log('Phase 10.10 transition binding smoke passed');

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
