import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const repoRoot = resolve(import.meta.dirname, '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const composerPath = resolve(repoRoot, 'client', 'src', 'features', 'composer', 'Composer.tsx');

const adapter = readFileSync(adapterPath, 'utf8');
const types = readFileSync(typesPath, 'utf8');
const composer = readFileSync(composerPath, 'utf8');

assertIncludes(
  types,
  'submitUserInput?: (current: WorkbenchViewModel, input: RuntimeUserInputRequestViewModel)',
  'Workbench adapter contract must expose runtime-owned user input submission',
);
assertIncludes(adapter, 'SubmitUserInput?: (req: RuntimeUserInputRequestDTO)', 'Wails bridge must expose SubmitUserInput');
assertIncludes(adapter, "mode: trimmed.startsWith('/') ? 'slash' : 'prompt'", 'composer slash text must be sent as slash mode');
assertIncludes(
  adapter,
  'bridge.SubmitUserInput\n                ? bridge.SubmitUserInput',
  'adapter must prefer SubmitUserInput when the bridge supports it',
);
assertIncludes(adapter, ': bridge.Chat({', 'adapter may keep Chat only as legacy fallback');
assertIncludes(adapter, 'function mapNormalizedInputConversation(response: RuntimeChatResponseDTO', 'UI may render runtime-returned normalized input results');
assertIncludes(adapter, 'if (!normalized || response.turnId || (normalized.shouldQuery === true && !hookPrevented))', 'normalized rendering must be limited to non-query runtime results');
assertIncludes(adapter, "const runtimeBridgePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';", 'adapter must load Wails generated bindings directly');
assertNotIncludes(adapter, 'runtimeHTTPBridge', 'adapter must not use the legacy HTTP bridge');
assertNotIncludes(adapter, 'runtimeFetch<', 'adapter must not call runtime HTTP routes');
assertNotIncludes(adapter, 'action.metadata.normalized', 'React must not infer normalized evidence from action metadata');
// mapActivityTimeline / mergeActivityTimeline / compareActivityTimelineItems
// were removed with the two-phase conversation refactor — the runtime-owned
// SessionOutput projection is now the only conversation timeline source.
// Covers the two-phase input normalization adapter contract.
assertNotIncludes(adapter, 'function mapActivityTimeline(', 'legacy SessionActivity-derived timeline path must be removed');
assertNotIncludes(adapter, 'function mergeActivityTimeline(', 'legacy SessionActivity-derived timeline merge must be removed');
assertNotIncludes(adapter, 'function compareActivityTimelineItems(', 'legacy SessionActivity-derived comparator must be removed');
assertIncludes(composer, "const requiresSelectedModel = (prompt: string) => !prompt.trim().startsWith('/');", 'composer must not block slash commands before runtime normalization');
assertIncludes(composer, 'requiresSelectedModel(prompt) && !composer.selectedModel?.configuredProviderId', 'model selection guard must apply only to model-backed prompt submits');

console.log('phase02 input normalization adapter smoke passed');

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
