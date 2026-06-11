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
  'interface RuntimeRunCheckpointMarkerDTO',
  'adapter must declare a transport DTO for marker-only checkpoint reads',
);
assertIncludes(
  adapter,
  'RunCheckpointMarkers?: (runID: string) => Promise<RuntimeRunCheckpointMarkersResponseDTO>;',
  'Wails bridge must expose optional marker list read transport',
);
assertIncludes(
  adapter,
  'RunCheckpointMarker?: (runID: string, checkpointID: string) => Promise<RuntimeRunCheckpointMarkerResponseDTO>;',
  'Wails bridge must expose optional marker detail read transport',
);
assertIncludes(
  adapter,
  'runtimeFetch<RuntimeRunCheckpointMarkersResponseDTO>(`/v1/runs/${encodeURIComponent(runID)}/checkpoint-markers`)',
  'HTTP bridge must reread marker lists from the backend route',
);
assertIncludes(
  adapter,
  'runtimeFetch<RuntimeRunCheckpointMarkerResponseDTO>(',
  'HTTP bridge must reread marker details from the backend route',
);

assertNotIncludes(
  adapter,
  'bridge.RunCheckpointMarkers?.(',
  'hydration must not automatically merge checkpoint markers into the workbench view model',
);
assertNotIncludes(
  adapter,
  'bridge.RunCheckpointMarker?.(',
  'hydration/actions must not automatically merge a checkpoint marker into frontend state',
);
assertNotIncludes(
  adapter,
  'mapRunCheckpointMarker',
  'adapter must not map marker DTOs into UI state in this phase',
);
assertNotIncludes(
  types,
  'checkpointMarker',
  'WorkbenchViewModel must not gain checkpoint marker UI state in this phase',
);
assertNotIncludes(
  workspace,
  'CheckpointMarker',
  'workspace UI must not render checkpoint marker DTOs in this phase',
);
assertNotIncludes(
  preview,
  'CheckpointMarker',
  'RunProjectionPreview must continue rendering projection DTOs only',
);

console.log('phase50.10 checkpoint marker adapter smoke passed');
