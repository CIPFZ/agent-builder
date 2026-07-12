import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const root = process.cwd();
const tempDir = path.join(root, 'node_modules', '.tmp', 'conversation-target-submit-smoke');
await mkdir(tempDir, { recursive: true });

for (const name of ['conversationSubmitQueue', 'outputStore', 'outputReducer', 'outputSelectors']) {
  const source = await readFile(path.join(root, 'src', 'runtime', `${name}.ts`), 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022, importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove },
  }).outputText.replaceAll('.ts', '.mjs');
  await writeFile(path.join(tempDir, `${name}.mjs`), output);
}
await mkdir(path.join(tempDir, 'conversation'), { recursive: true });
const statusSource = await readFile(path.join(root, 'src', 'runtime', 'conversation', 'statusMachine.ts'), 'utf8');
await writeFile(path.join(tempDir, 'conversation', 'statusMachine.mjs'), ts.transpileModule(statusSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022, importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove },
}).outputText.replaceAll('.ts', '.mjs'));

const { createConversationSubmitQueue } = await import(pathToFileURL(path.join(tempDir, 'conversationSubmitQueue.mjs')).href);
const { createOutputStore, retargetOutputStore } = await import(pathToFileURL(path.join(tempDir, 'outputStore.mjs')).href);
const { addOptimisticUserSubmit, applyOutputEvents, hydrateOutputStore } = await import(pathToFileURL(path.join(tempDir, 'outputReducer.mjs')).href);
const { selectConversationMessages } = await import(pathToFileURL(path.join(tempDir, 'outputSelectors.mjs')).href);

// Two immediate submits must observe an atomic draft -> session transition.
const queue = createConversationSubmitQueue();
let target = { kind: 'draft' };
const requests = [];
const submit = (prompt) => queue.enqueue(async () => {
  requests.push({ prompt, sessionId: target.kind === 'session' ? target.sessionId : undefined });
  await Promise.resolve();
  target = { kind: 'session', sessionId: 'session-1' };
});
await Promise.all([submit('hello'), submit('hi')]);
assert.deepEqual(requests, [
  { prompt: 'hello', sessionId: undefined },
  { prompt: 'hi', sessionId: 'session-1' },
], 'consecutive sends create one session and append two turns to it');

// Retargeting preserves only the optimistic user submit. Assistant loading is
// produced by authoritative Wails output, then converges on its terminal item.
let store = addOptimisticUserSubmit(createOutputStore(''), { clientRequestId: 'request-1', prompt: 'hello', createdAt: 1, status: 'submitting' });
store = retargetOutputStore(store, 'session-1');
assert.deepEqual(selectConversationMessages(store).map(({ role }) => role), ['user']);
store = applyOutputEvents(store, [{
  id: 'assistant-streaming', sequence: 1, sessionId: 'session-1', turnId: 'turn-1', kind: 'conversation_item.created', entityId: 'assistant-1', operation: 'append',
  item: { id: 'assistant-1', kind: 'assistant_message', sessionId: 'session-1', turnId: 'turn-1', sequence: 2, role: 'assistant', content: 'hello', status: 'streaming' },
}]);
assert.equal(selectConversationMessages(store).find(({ role }) => role === 'assistant')?.status, 'loading');
store = applyOutputEvents(store, [{
  id: 'assistant-completed', sequence: 2, sessionId: 'session-1', turnId: 'turn-1', kind: 'conversation_item.updated', entityId: 'assistant-1', operation: 'update',
  item: { id: 'assistant-1', kind: 'assistant_message', sessionId: 'session-1', turnId: 'turn-1', sequence: 2, role: 'assistant', content: 'hello!', status: 'completed' },
}]);
assert.equal(selectConversationMessages(store).find(({ role }) => role === 'assistant')?.status, 'success', 'runtime loading converges when Wails emits terminal output');

// A first-submit SessionOutput snapshot must also replace draft optimism even
// when all turn events were emitted before the session stream was subscribed.
let snapshotStore = addOptimisticUserSubmit(createOutputStore(''), { clientRequestId: 'request-snapshot', prompt: 'snapshot prompt', createdAt: 1, status: 'submitting' });
snapshotStore = retargetOutputStore(snapshotStore, 'session-snapshot');
snapshotStore = hydrateOutputStore({
  sessionId: 'session-snapshot', cursor: '4', version: 1,
  items: [
    { id: 'snapshot-user', kind: 'user_message', sessionId: 'session-snapshot', turnId: 'turn-snapshot', sequence: 1, role: 'user', content: 'snapshot prompt', clientRequestId: 'request-snapshot', createdAt: 2 },
    { id: 'snapshot-assistant', kind: 'assistant_message', sessionId: 'session-snapshot', turnId: 'turn-snapshot', sequence: 2, role: 'assistant', content: 'snapshot answer', status: 'completed', createdAt: 3 },
  ],
  messages: [], turns: [{ id: 'turn-snapshot', sessionId: 'session-snapshot', status: 'completed' }], assistantSteps: [], toolCalls: [], toolResults: [], permissions: [], agentTasks: [],
}, snapshotStore);
const snapshotMessages = selectConversationMessages(snapshotStore);
assert.deepEqual(snapshotMessages.map(({ content }) => content), ['snapshot prompt', 'snapshot answer']);
assert.equal(snapshotMessages.some(({ status }) => status === 'loading'), false, 'initial Wails snapshot settles draft loading without a session click');

console.log('conversation target submit smoke passed');
