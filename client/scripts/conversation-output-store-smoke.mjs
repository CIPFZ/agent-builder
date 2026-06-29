import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const root = process.cwd();
const tempDir = path.join(root, 'node_modules', '.tmp', 'conversation-output-smoke');
await mkdir(tempDir, { recursive: true });

for (const name of ['outputStore', 'outputReducer', 'outputSelectors']) {
  const source = await readFile(path.join(root, 'src', 'runtime', `${name}.ts`), 'utf8');
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022,
      importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
    },
  }).outputText.replaceAll('.ts', '.mjs');
  await writeFile(path.join(tempDir, `${name}.mjs`), transpiled);
}

const { createOutputStore } = await import(pathToFileURL(path.join(tempDir, 'outputStore.mjs')).href);
const { addOptimisticUserSubmit, applyOutputEvent, hydrateOutputStore } = await import(pathToFileURL(path.join(tempDir, 'outputReducer.mjs')).href);
const { selectConversationMessages, selectConversationTimeline, selectPendingPermissions } = await import(pathToFileURL(path.join(tempDir, 'outputSelectors.mjs')).href);

let store = createOutputStore('session-1');
store = addOptimisticUserSubmit(store, { clientRequestId: 'client-1', prompt: 'same text', createdAt: 100, status: 'submitting' });
store = addOptimisticUserSubmit(store, { clientRequestId: 'client-2', prompt: 'same text', createdAt: 101, status: 'submitting' });
store = applyOutputEvent(store, {
  id: 'turn-first',
  sequence: 1,
  sessionId: 'session-1',
  turnId: 'turn-1',
  kind: 'turn.created',
  entityId: 'turn-1',
  operation: 'append',
  turn: { id: 'turn-1', sessionId: 'session-1', status: 'running', userMessageId: 'msg-1', startedAt: 90 },
});
assert.deepEqual(selectConversationMessages(store).map((message) => message.clientRequestId), ['client-1', 'client-2']);

store = applyOutputEvent(store, {
  id: 'message-client-1',
  sequence: 2,
  sessionId: 'session-1',
  turnId: 'turn-1',
  kind: 'message.created',
  entityId: 'msg-1',
  operation: 'append',
  message: { id: 'msg-1', sessionId: 'session-1', role: 'user', content: 'same text', clientRequestId: 'client-1', createdAt: 110 },
});
assert.deepEqual(selectConversationMessages(store).map((message) => message.clientRequestId), ['client-2', 'client-1']);

const snapshot = {
  sessionId: 'session-1',
  cursor: '10',
  messages: [
    { id: 'msg-user', sessionId: 'session-1', role: 'user', content: 'run tools', clientRequestId: 'client-3', createdAt: 300 },
  ],
  turns: [
    { id: 'turn-2', sessionId: 'session-1', status: 'running', userMessageId: 'msg-user', startedAt: 200 },
  ],
  assistantSteps: [
    { id: 'step-1', sessionId: 'session-1', turnId: 'turn-2', messageId: 'msg-assistant-1', index: 0, status: 'completed', text: 'Checking', thinkingSummary: 'Plan', toolCallIds: ['tool-read-1', 'tool-read-2', 'tool-failed', 'tool-running', 'tool-wait'], startedAt: 310, updatedAt: 350 },
    { id: 'step-2', sessionId: 'session-1', turnId: 'turn-2', messageId: 'msg-assistant-2', index: 1, status: 'completed', text: 'Done', toolCallIds: ['tool-read-next'], startedAt: 360, updatedAt: 370 },
  ],
  toolCalls: [
    { id: 'tool-read-1', sessionId: 'session-1', turnId: 'turn-2', assistantStepId: 'step-1', name: 'view', source: 'builtin', status: 'completed', display: { kind: 'file_read' }, resultIds: ['result-1'], latestResultId: 'result-1', startedAt: 320, finishedAt: 321 },
    { id: 'tool-read-2', sessionId: 'session-1', turnId: 'turn-2', assistantStepId: 'step-1', name: 'view', source: 'builtin', status: 'completed', display: { kind: 'file_read' }, resultIds: ['result-2'], latestResultId: 'result-2', startedAt: 322, finishedAt: 323 },
    { id: 'tool-failed', sessionId: 'session-1', turnId: 'turn-2', assistantStepId: 'step-1', name: 'bash', source: 'builtin', status: 'failed', error: 'boom', startedAt: 324, finishedAt: 325 },
    { id: 'tool-running', sessionId: 'session-1', turnId: 'turn-2', assistantStepId: 'step-1', name: 'bash', source: 'builtin', status: 'running', startedAt: 326 },
    { id: 'tool-wait', sessionId: 'session-1', turnId: 'turn-2', assistantStepId: 'step-1', name: 'bash', source: 'builtin', status: 'waiting_permission', startedAt: 327 },
    { id: 'tool-read-next', sessionId: 'session-1', turnId: 'turn-2', assistantStepId: 'step-2', name: 'view', source: 'builtin', status: 'completed', display: { kind: 'file_read' }, startedAt: 361, finishedAt: 362 },
  ],
  toolResults: [
    { id: 'result-1', sessionId: 'session-1', turnId: 'turn-2', messageId: 'tool-msg-1', toolCallId: 'tool-read-1', toolName: 'view', status: 'success', contentPreview: 'a.go', createdAt: 321 },
    { id: 'result-2', sessionId: 'session-1', turnId: 'turn-2', messageId: 'tool-msg-2', toolCallId: 'tool-read-2', toolName: 'view', status: 'success', contentPreview: 'b.go', createdAt: 323 },
  ],
  permissions: [
    { id: 'perm-1', sessionId: 'session-1', turnId: 'turn-2', toolCallId: 'tool-wait', toolName: 'bash', action: 'run', status: 'pending', createdAt: 328 },
  ],
};

store = hydrateOutputStore(snapshot, createOutputStore('session-1'));
const timeline = selectConversationTimeline(store);
assert.equal(timeline[0].id, 'message-msg-user', 'user message stays before turn progress even when turn starts first');
assert(timeline.some((item) => item.id.startsWith('tool-group-step-1') && item.summary === 'Read 2 files'), 'completed tools are grouped per assistant step');
assert(timeline.some((item) => item.id === 'tool-step-2-tool-read-next'), 'next assistant step is not mixed into previous completed group');
assert(timeline.some((item) => item.toolCallId === 'tool-failed' && item.status === 'failed'), 'failed tool remains visible');
assert(timeline.some((item) => item.toolCallId === 'tool-running' && item.status === 'running'), 'running tool remains visible');
assert(timeline.some((item) => item.kind === 'permission' && item.permission?.id === 'perm-1'), 'waiting permission remains visible');
assert.equal(selectPendingPermissions(store).length, 1);

let runtimeStore = createOutputStore('session-runtime');
runtimeStore = addOptimisticUserSubmit(runtimeStore, { clientRequestId: 'client-runtime', prompt: 'runtime prompt', createdAt: 1, status: 'submitting' });
runtimeStore = hydrateOutputStore({
  sessionId: 'session-runtime',
  cursor: '20',
  version: 1,
  items: [
    { id: 'item-user', kind: 'user_message', sessionId: 'session-runtime', turnId: 'turn-runtime', sequence: 1, role: 'user', content: 'runtime prompt', messageId: 'msg-runtime-user', createdAt: 10 },
    { id: 'item-tool-group', kind: 'tool_group', sessionId: 'session-runtime', turnId: 'turn-runtime', sequence: 3, status: 'completed', title: 'Read 2 files', summary: 'Read 2 files', toolCallId: 'tool-read-1', toolCallIds: ['tool-read-1', 'tool-read-2'], display: { kind: 'file_read', quiet: true, groupable: true, defaultExpanded: false, toolCallIds: ['tool-read-1', 'tool-read-2'] }, createdAt: 20 },
    { id: 'item-context', kind: 'context_source', sessionId: 'session-runtime', turnId: 'turn-runtime', sequence: 4, status: 'failed', title: 'file', summary: 'unreadable', error: 'unreadable', contextId: 'file:/work/broken.md', display: { kind: 'file', target: '/work/broken.md' }, createdAt: 25 },
    { id: 'item-assistant-final', kind: 'assistant_message', sessionId: 'session-runtime', turnId: 'turn-runtime', sequence: 5, role: 'assistant', phase: 'final', content: 'final answer', status: 'completed', messageId: 'msg-runtime-final', createdAt: 30 },
  ],
  messages: [
    { id: 'msg-runtime-user', sessionId: 'session-runtime', role: 'user', content: 'runtime prompt', clientRequestId: 'client-runtime', createdAt: 10 },
    { id: 'msg-runtime-final', sessionId: 'session-runtime', role: 'assistant', content: 'final answer', finished: true, createdAt: 30 },
  ],
  toolCalls: [
    { id: 'tool-read-1', sessionId: 'session-runtime', turnId: 'turn-runtime', name: 'view', source: 'builtin', kind: 'file_read', status: 'completed', quiet: true, groupable: true, defaultExpanded: false, display: { kind: 'file_read', title: 'Read file' }, result: { id: 'result-1', messageId: 'tool-result-msg', status: 'success', contentPreview: 'file contents' }, startedAt: 20, finishedAt: 21 },
    { id: 'tool-read-2', sessionId: 'session-runtime', turnId: 'turn-runtime', name: 'view', source: 'builtin', kind: 'file_read', status: 'completed', quiet: true, groupable: true, defaultExpanded: false, display: { kind: 'file_read', title: 'Read file' }, startedAt: 22, finishedAt: 23 },
  ],
  toolResults: [
    { id: 'result-1', sessionId: 'session-runtime', turnId: 'turn-runtime', messageId: 'tool-result-msg', toolCallId: 'tool-read-1', toolName: 'view', status: 'success', contentPreview: 'file contents', createdAt: 21 },
  ],
}, runtimeStore);
const runtimeTimeline = selectConversationTimeline(runtimeStore);
assert.deepEqual(runtimeTimeline.map((item) => item.id), ['item-user', 'item-tool-group', 'item-context', 'item-assistant-final'], 'runtime sequence controls item ordering');
assert.equal(runtimeTimeline.some((item) => item.kind === 'tool_result'), false, 'runtime tool result is not a standalone row');
assert.equal(runtimeTimeline.find((item) => item.id === 'item-tool-group')?.kind, 'tool_group', 'runtime tool group is rendered directly');
assert.equal(runtimeTimeline.find((item) => item.id === 'item-context')?.kind, 'context_source', 'high-signal runtime context governance is an explicit item');
assert.equal(selectConversationMessages(runtimeStore).some((message) => message.id.startsWith('optimistic-')), false, 'runtime user item replaces optimistic submit');

const replayed = applyOutputEvent(runtimeStore, {
  id: 'runtime-event-1',
  sequence: 2101,
  sessionId: 'session-runtime',
  turnId: 'turn-runtime',
  kind: 'conversation_item.updated',
  entityId: 'item-assistant-final',
  operation: 'update',
  item: { id: 'item-assistant-final', kind: 'assistant_message', sessionId: 'session-runtime', turnId: 'turn-runtime', sequence: 5, role: 'assistant', phase: 'final', content: 'final answer updated', status: 'completed' },
});
const replayedAgain = applyOutputEvent(replayed, {
  id: 'runtime-event-1',
  sequence: 2101,
  sessionId: 'session-runtime',
  turnId: 'turn-runtime',
  kind: 'conversation_item.updated',
  entityId: 'item-assistant-final',
  operation: 'update',
  item: { id: 'item-assistant-final', kind: 'assistant_message', sessionId: 'session-runtime', turnId: 'turn-runtime', sequence: 5, role: 'assistant', phase: 'final', content: 'should not apply twice', status: 'completed' },
});
assert.equal(selectConversationTimeline(replayedAgain).find((item) => item.id === 'item-assistant-final')?.content, 'final answer updated', 'output event replay is idempotent');

const streamingStore = hydrateOutputStore({
  sessionId: 'session-2',
  messages: [
    { id: 'msg-stream', sessionId: 'session-2', role: 'assistant', content: 'partial', finished: false, createdAt: 500 },
  ],
  assistantSteps: [
    { id: 'step-stream', sessionId: 'session-2', turnId: 'turn-stream', messageId: 'msg-stream', index: 0, status: 'streaming', text: 'partial', startedAt: 500, updatedAt: 501 },
  ],
}, createOutputStore('session-2'));
assert.equal(selectConversationMessages(streamingStore)[0].status, 'loading', 'streaming assistant conversation message is not complete');
assert.equal(selectConversationTimeline(streamingStore).find((item) => item.id === 'assistant-step-stream')?.status, 'loading', 'streaming assistant timeline message is not complete');

const nearBottomShouldFollow = (distanceToBottom, pinned) => pinned || distanceToBottom < 160;
assert.equal(nearBottomShouldFollow(40, false), true, 'new output follows near bottom');
assert.equal(nearBottomShouldFollow(900, false), false, 'history browsing does not force jump to bottom');

console.log('conversation output store smoke passed');
