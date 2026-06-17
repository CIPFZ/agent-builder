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
assertIncludes(adapter, "SubmitUserInput: (req) =>\n    runtimeFetch<RuntimeChatResponseDTO>('/v1/user-inputs'", 'HTTP bridge must post normalized input');
assertIncludes(adapter, "mode: trimmed.startsWith('/') ? 'slash' : 'prompt'", 'composer slash text must be sent as slash mode');
assertIncludes(
  adapter,
  'bridge.SubmitUserInput\n                ? bridge.SubmitUserInput',
  'adapter must prefer SubmitUserInput when the bridge supports it',
);
assertIncludes(adapter, ': bridge.Chat({', 'adapter may keep Chat only as legacy fallback');
assertIncludes(adapter, 'function mapNormalizedInputConversation(response: RuntimeChatResponseDTO', 'UI may render runtime-returned normalized input results');
assertIncludes(adapter, 'response.turnId || normalized.shouldQuery === true', 'normalized rendering must be limited to non-query runtime results');
assertIncludes(adapter, 'await runtimeHTTPBridge.Status();', 'HTTP bridge availability must use runtime status, not provider/settings routes');
assertIncludes(adapter, 'if (import.meta.env.DEV) {\n    return Promise.resolve(null);', 'Vite development must use HTTP transport instead of Wails bindings');
assertIncludes(adapter, 'function runtimeJSONP<T>(path: string, init?: RuntimeHTTPInit)', 'JSONP fallback must preserve POST method/body for browser dev transport');
assertIncludes(adapter, "params.set('body', init.body);", 'JSONP fallback must forward request bodies');
assertNotIncludes(adapter, 'action.metadata.normalized', 'React must not infer normalized evidence from action metadata');
assertIncludes(adapter, 'function compareActivityTimelineItems(messages: RuntimeMessageDTO[], turns: RuntimeTurnDTO[])', 'activity timeline merge must preserve runtime turn ordering');
assertIncludes(adapter, 'return [...kept, ...replacement].sort(compareActivityTimelineItems(activityMessages, activityTurns));', 'merged runtime activity must not fall back to timestamp/id-only sorting');
assertIncludes(adapter, 'const inferredTurnID = turnIDForMessage(activityTurnContext, item.messageId);', 'merged runtime activity must replace same-turn stale timeline items');
assertNotIncludes(
  adapter,
  'return [...kept, ...replacement].sort((left, right) => {',
  'merged runtime activity must not reorder same-turn user/tool/assistant items by local timestamp/id heuristics',
);
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
