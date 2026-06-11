import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import ts from 'typescript';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const helperPath = resolve(repoRoot, 'client', 'src', 'runtime', 'actionRefreshSelector.ts');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const tmpDir = resolve(repoRoot, 'tmp', 'runtime-dev', 'phase401');
const compiledHelperPath = resolve(tmpDir, 'actionRefreshSelector.mjs');

await mkdir(tmpDir, { recursive: true });

const helperSource = await readFile(helperPath, 'utf8');
const compiled = ts.transpileModule(helperSource, {
  compilerOptions: {
    module: ts.ModuleKind.ES2022,
    target: ts.ScriptTarget.ES2022,
    strict: true,
  },
});
await writeFile(compiledHelperPath, compiled.outputText, 'utf8');

const { runtimeActionRefreshTargets } = await import(pathToFileURL(compiledHelperPath).href);

assert.deepEqual(
  runtimeActionRefreshTargets({
    action: {
      accepted: true,
      refreshTargets: ['status', 'session_activity_window', 'status', 'run_projection'],
      reason: 'ignored',
      source: { action: 'ignored' },
    },
  }),
  ['status', 'session_activity_window', 'run_projection'],
);
assert.equal(runtimeActionRefreshTargets({ action: { accepted: false, refreshTargets: ['status'] } }), undefined);
assert.equal(runtimeActionRefreshTargets({ action: { accepted: true, refreshTargets: ['status', 'unknown'] } }), undefined);
assert.equal(runtimeActionRefreshTargets({ action: { accepted: true, refreshTargets: 'status' } }), undefined);
assert.deepEqual(runtimeActionRefreshTargets({ accepted: true, refreshTargets: ['run_scheduler_plan'] }), ['run_scheduler_plan']);
assert.equal(runtimeActionRefreshTargets({ accepted: false, refreshTargets: ['status'] }), undefined);
assert.equal(runtimeActionRefreshTargets(undefined), undefined);

const adapterSource = await readFile(adapterPath, 'utf8');
for (const method of ['DecidePermission', 'CancelTurn', 'MarkInterruptedDone', 'ResumeRunCheckpoint', 'ExecuteRunTask']) {
  assert.match(adapterSource, new RegExp(`(?:const response =|response =) await bridge\\.${method}`));
}
assert.match(adapterSource, /hydrateWorkbenchForAction\(/);
assert.doesNotMatch(adapterSource, /response\.action\.(?:source|reason|evidence)/);

console.log('phase401 action refresh selector smoke passed');
