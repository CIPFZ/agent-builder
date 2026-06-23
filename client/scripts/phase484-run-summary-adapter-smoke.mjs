import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const workspacePath = resolve(repoRoot, 'client', 'src', 'features', 'workspace', 'Workspace.tsx');
const previewPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'RunProjectionPreview.tsx');

const adapter = readFileSync(adapterPath, 'utf8');
const types = readFileSync(typesPath, 'utf8');
const workspace = readFileSync(workspacePath, 'utf8');
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
  adapter,
  'interface RuntimeRunSummaryDTO',
  'adapter must declare a transport DTO for summary-only persisted Run reads',
);
assertIncludes(
  adapter,
  'RunSummaries?: () => Promise<RuntimeRunSummariesResponseDTO>;',
  'Wails bridge must expose optional RunSummaries read transport',
);
assertIncludes(
  adapter,
  'RunSummary?: (runID: string) => Promise<RuntimeRunSummaryResponseDTO>;',
  'Wails bridge must expose optional RunSummary read transport',
);
assertIncludes(
  adapter,
  "RunSummaries: () => runtimeFetch<RuntimeRunSummariesResponseDTO>('/v1/run-summaries')",
  'HTTP bridge must reread run summaries from the runtime route',
);
assertIncludes(
  adapter,
  'RunSummary: (runID) => runtimeFetch<RuntimeRunSummaryResponseDTO>(`/v1/run-summaries/${encodeURIComponent(runID)}`)',
  'HTTP bridge must reread a run summary from the runtime route',
);

assertNotIncludes(
  adapter,
  'bridge.RunSummaries?.()',
  'hydration must not automatically merge run summaries into the workbench view model',
);
assertNotIncludes(
  adapter,
  'bridge.RunSummary?.(',
  'hydration/actions must not automatically merge a run summary into frontend state',
);
assertNotIncludes(
  adapter,
  'mapRunSummary',
  'adapter must not map summary DTOs into UI state in this phase',
);
assertNotIncludes(
  types,
  'runSummary',
  'WorkbenchViewModel must not gain Run summary UI state in this phase',
);
assertNotIncludes(
  workspace,
  'RunSummary',
  'workspace UI must not render Run summary DTOs in this phase',
);
assertNotIncludes(
  preview,
  'RunSummary',
  'RunProjectionPreview must continue rendering projection DTOs only',
);

console.log('phase48.4 run summary adapter smoke passed');
