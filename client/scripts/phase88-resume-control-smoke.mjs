import { existsSync, mkdirSync, readFileSync, symlinkSync } from 'node:fs';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { dirname, resolve } from 'node:path';
import { build as viteBuild } from 'vite';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const runtimeDevRoot = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase88-resume-control-smoke');
const componentEntry = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');
const contractEntry = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'runProjectionResumeContract.ts');
const bundlePath = resolve(runtimeDevRoot, 'RunProjectionPreview.bundle.mjs');
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

mkdirSync(runtimeDevRoot, { recursive: true });
const tempNodeModules = resolve(runtimeDevRoot, 'node_modules');
const clientNodeModules = resolve(repoRoot, 'client', 'node_modules');

await viteBuild({
  configFile: false,
  root: resolve(repoRoot, 'client'),
  logLevel: 'silent',
  build: {
    lib: {
      entry: contractEntry,
      formats: ['es'],
      fileName: () => 'RunProjectionPreview.bundle.mjs',
    },
    outDir: runtimeDevRoot,
    emptyOutDir: true,
    rollupOptions: {
      external: [],
    },
  },
});

if (!existsSync(tempNodeModules)) {
  symlinkSync(clientNodeModules, tempNodeModules, 'junction');
}

const { selectResumableCheckpoint } = await import(pathToFileURL(bundlePath).href);

const selected = selectResumableCheckpoint([
  { id: 'checkpoint-1', summary: 'Structured checkpoint one', resumeEligible: true },
  { id: 'checkpoint-2', summary: 'Structured checkpoint two', resumeEligible: true },
]);
assertEquals(selected?.id, 'checkpoint-1', 'first eligible checkpoint controls actionability');

const ineligible = selectResumableCheckpoint([
  {
    id: 'checkpoint-3',
    summary: 'Already resumed checkpoint',
    resumeEligible: false,
    resumedTurnIds: ['turn-resumed'],
  },
]);
assertEquals(ineligible, undefined, 'ineligible checkpoint does not expose resume actionability');

const componentSource = readFileSync(componentEntry, 'utf8');
assertIncludes(componentSource, 'data-testid="run-checkpoint-resume"', 'component keeps a stable resume control marker');
assertIncludes(componentSource, 'pendingCheckpointID', 'component keeps resume pending state local');
assertIncludes(componentSource, 'onResumeCheckpoint(run.id, resumableCheckpoint.id)', 'component submits explicit run/checkpoint IDs only');

if (!existsSync(bindingPath)) {
  throw new Error(`Wails binding not found: ${bindingPath}. Run "cd desktop && wails3 task common:generate:bindings" first.`);
}
const binding = readFileSync(bindingPath, 'utf8');
assertIncludes(binding, 'export function ResumeRunCheckpoint(runID, checkpointID)', 'Wails binding exports ResumeRunCheckpoint');
assertIncludes(binding, '$Call.ByID', 'Wails binding delegates through generated call ID');

console.log('Phase 8.8 resume control smoke passed');

function assertIncludes(value, needle, label) {
  if (!value.includes(needle)) {
    throw new Error(`${label}: expected to include ${needle}`);
  }
}

function assertEquals(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${expected}, got ${actual}`);
  }
}
