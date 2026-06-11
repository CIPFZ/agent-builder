import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const selectorPath = resolve(repoRoot, 'client', 'src', 'runtime', 'actionRefreshSelector.ts');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');

const adapter = readFileSync(adapterPath, 'utf8');
const selector = readFileSync(selectorPath, 'utf8');
const preview = readFileSync(previewPath, 'utf8');

function assertIncludes(source, needle, message) {
  if (!source.includes(needle)) {
    throw new Error(`${message}\nMissing: ${needle}`);
  }
}

function assertNotIncludes(source, needle, message) {
  if (source.includes(needle)) {
    throw new Error(`${message}\nUnexpected: ${needle}`);
  }
}

assertIncludes(
  selector,
  "'run_projection'",
  'action refresh selector must allow backend run projection reread targets',
);
assertIncludes(
  adapter,
  "const refreshRuns = actionTargetsInclude(refreshTargets, 'run', 'run_projection', 'run_transition_history', 'run_scheduler_plan');",
  'adapter must choose run refresh from allowlisted refresh targets',
);
assertIncludes(
  adapter,
  "bridge.RunProjection?.({ sessionId: activeSessionID, limit: 24 })",
  'adapter must reread RunProjection DTOs for run status surfaces',
);
assertIncludes(
  adapter,
  'runProjection: mapRunProjection(runProjection, schedulerTaskCandidates)',
  'adapter must map RunProjection DTOs into the view model',
);
assertIncludes(
  adapter,
  'return hydrateWorkbench(current, bridge, { refreshTargets });',
  'action responses must only select refresh targets before hydration',
);
assertNotIncludes(
  adapter,
  'response.run',
  'adapter must not merge action response run payloads into frontend run state',
);
assertNotIncludes(
  adapter,
  'response.projection',
  'adapter must not merge action response projection payloads into frontend run state',
);
assertNotIncludes(
  adapter,
  'payload.status',
  'adapter must not derive Run status from runtime event payload status',
);
assertNotIncludes(
  preview,
  'runtimeActivityRefreshHint',
  'RunProjectionPreview must render durable projection DTOs, not event refresh hints',
);
assertNotIncludes(
  preview,
  'RuntimeWriteAction',
  'RunProjectionPreview must not import write-action metadata as Run state',
);
assertNotIncludes(
  preview,
  'response.action',
  'RunProjectionPreview must not read write-action responses as Run state',
);
assertNotIncludes(
  preview,
  'refreshTargets',
  'RunProjectionPreview must not select refresh targets from write-action metadata',
);

console.log('phase46 status writer adapter reread smoke passed');
