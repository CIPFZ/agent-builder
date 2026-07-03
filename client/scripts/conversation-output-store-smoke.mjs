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
const { addOptimisticUserSubmit, applyOutputEvent, applyOutputEvents, hydrateOutputStore } = await import(
  pathToFileURL(path.join(tempDir, 'outputReducer.mjs')).href
);
const { selectConversationMessages, selectConversationTimeline, selectPendingPermissions } = await import(
  pathToFileURL(path.join(tempDir, 'outputSelectors.mjs')).href
);

// ── 1. Runtime-owned snapshot: version=1 items are the only conversation source.

const snapshot = {
  sessionId: 'session-runtime',
  cursor: '10',
  version: 1,
  items: [
    { id: 'item-user', kind: 'user_message', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1000000, role: 'user', content: 'run tools', messageId: 'msg-user', clientRequestId: 'client-A', createdAt: 10 },
    { id: 'item-exp', kind: 'exploration_summary', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1050000, status: 'exploring', title: 'Read 2 files', display: { counts: [{ kind: 'file_read', count: 2 }] }, exploration: { status: 'exploring', toolTotal: 2, toolCounts: [{ kind: 'file_read', count: 2 }] }, createdAt: 12 },
    { id: 'item-tool-group', kind: 'tool_group', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1102020, status: 'completed', title: 'Read 2 files', summary: 'Read 2 files', toolCallId: 'tool-1', toolCallIds: ['tool-1', 'tool-2'], display: { kind: 'file_read', quiet: true, groupable: true, counts: [{ kind: 'file_read', count: 2 }] }, createdAt: 22 },
    { id: 'item-assistant-final', kind: 'assistant_message', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 91000039, role: 'assistant', phase: 'final', content: 'done', status: 'completed', messageId: 'msg-final', createdAt: 40 },
  ],
  messages: [
    { id: 'msg-user', sessionId: 'session-runtime', role: 'user', content: 'run tools', clientRequestId: 'client-A', createdAt: 10 },
    { id: 'msg-final', sessionId: 'session-runtime', role: 'assistant', content: 'done', finished: true, createdAt: 40 },
  ],
  toolCalls: [
    { id: 'tool-1', sessionId: 'session-runtime', turnId: 'turn-1', name: 'view', source: 'builtin', kind: 'file_read', status: 'completed', quiet: true, groupable: true, display: { kind: 'file_read', title: 'Read file' }, startedAt: 21, finishedAt: 22 },
    { id: 'tool-2', sessionId: 'session-runtime', turnId: 'turn-1', name: 'view', source: 'builtin', kind: 'file_read', status: 'completed', quiet: true, groupable: true, display: { kind: 'file_read', title: 'Read file' }, startedAt: 23, finishedAt: 24 },
  ],
};

let optimisticStore = createOutputStore('session-runtime');
optimisticStore = addOptimisticUserSubmit(optimisticStore, { clientRequestId: 'client-A', prompt: 'run tools', createdAt: 5, status: 'submitting' });
optimisticStore = hydrateOutputStore(snapshot, optimisticStore);

const runtimeTimeline = selectConversationTimeline(optimisticStore);
assert.deepEqual(runtimeTimeline.map((item) => item.id), ['item-user', 'item-exp', 'item-tool-group', 'item-assistant-final'], 'runtime sequence controls item ordering');
assert.equal(runtimeTimeline.find((item) => item.id === 'item-tool-group')?.kind, 'tool_group', 'runtime tool group is rendered directly');
const exploration = runtimeTimeline.find((item) => item.id === 'item-exp');
assert.equal(exploration?.exploration?.toolTotal, 2, 'exploration_summary carries toolTotal');
assert.deepEqual(exploration?.displayCounts?.[0], { kind: 'file_read', count: 2 }, 'exploration_summary counts propagate to displayCounts');
const runtimeMessages = selectConversationMessages(optimisticStore);
assert.equal(runtimeMessages.some((message) => message.id.startsWith('optimistic-')), false, 'runtime user_message with matching clientRequestId replaces optimistic submit');

// ── 2. Empty-items runtime snapshot must not fall back to any legacy path.

const emptyRuntimeContractStore = hydrateOutputStore({
  sessionId: 'session-empty-runtime',
  cursor: '30',
  version: 1,
  items: [],
  messages: [
    { id: 'legacy-user', sessionId: 'session-empty-runtime', role: 'user', content: 'hello', createdAt: 1 },
  ],
}, createOutputStore('session-empty-runtime'));
assert.deepEqual(selectConversationTimeline(emptyRuntimeContractStore), [], 'runtime snapshot with empty items must not fall back to legacy timeline composition');
assert.deepEqual(selectConversationMessages(emptyRuntimeContractStore), [], 'runtime snapshot with empty items must not fall back to legacy messages');

// ── 3. Delta reducer: contentLen guard + suffix append + overlay clears on completion.

let streamingStore = createOutputStore('session-stream');
streamingStore = hydrateOutputStore({
  sessionId: 'session-stream',
  cursor: '0',
  version: 1,
  items: [
    { id: 'assistant-msg-stream', kind: 'assistant_message', sessionId: 'session-stream', turnId: 'turn-stream', sequence: 100, role: 'assistant', phase: 'intermediate', content: '', status: 'streaming', messageId: 'msg-stream', createdAt: 100 },
  ],
  messages: [
    { id: 'msg-stream', sessionId: 'session-stream', role: 'assistant', content: '', finished: false, createdAt: 100 },
  ],
}, streamingStore);

streamingStore = applyOutputEvent(streamingStore, {
  id: 'delta-1',
  sequence: 0,
  sessionId: 'session-stream',
  kind: 'output.text.delta',
  entityId: 'msg-stream',
  operation: 'delta',
  textDelta: { messageId: 'msg-stream', partType: 'text', delta: 'hel', contentLen: 3 },
});
streamingStore = applyOutputEvent(streamingStore, {
  id: 'delta-2',
  sequence: 0,
  sessionId: 'session-stream',
  kind: 'output.text.delta',
  entityId: 'msg-stream',
  operation: 'delta',
  textDelta: { messageId: 'msg-stream', partType: 'text', delta: 'lo world', contentLen: 11 },
});

let timelineAfterDeltas = selectConversationTimeline(streamingStore);
let streamingItem = timelineAfterDeltas.find((item) => item.messageId === 'msg-stream');
assert.equal(streamingItem?.content, 'hello world', 'delta suffix appended');
assert.equal(streamingItem?.streaming, true, 'streaming flag set while overlay is active');
const messagesAfterDeltas = selectConversationMessages(streamingStore);
assert.equal(messagesAfterDeltas.find((message) => message.id === 'assistant-msg-stream')?.content, 'hello world', 'conversation view uses streamed text');

// Idempotent: replaying the same delta must not re-append.
streamingStore = applyOutputEvent(streamingStore, {
  id: 'delta-2-again',
  sequence: 0,
  sessionId: 'session-stream',
  kind: 'output.text.delta',
  entityId: 'msg-stream',
  operation: 'delta',
  textDelta: { messageId: 'msg-stream', partType: 'text', delta: 'lo world', contentLen: 11 },
});
timelineAfterDeltas = selectConversationTimeline(streamingStore);
streamingItem = timelineAfterDeltas.find((item) => item.messageId === 'msg-stream');
assert.equal(streamingItem?.content, 'hello world', 'delta idempotent when contentLen already applied');

// Out-of-order (contentLen smaller than known) is rejected.
streamingStore = applyOutputEvent(streamingStore, {
  id: 'delta-stale',
  sequence: 0,
  sessionId: 'session-stream',
  kind: 'output.text.delta',
  entityId: 'msg-stream',
  operation: 'delta',
  textDelta: { messageId: 'msg-stream', partType: 'text', delta: 'oops', contentLen: 5 },
});
timelineAfterDeltas = selectConversationTimeline(streamingStore);
streamingItem = timelineAfterDeltas.find((item) => item.messageId === 'msg-stream');
assert.equal(streamingItem?.content, 'hello world', 'out-of-order stale delta must not shrink content');

// Full message.completed clears the overlay so subsequent snapshots take over.
streamingStore = applyOutputEvent(streamingStore, {
  id: 'msg-complete',
  sequence: 100,
  sessionId: 'session-stream',
  kind: 'message.updated',
  entityId: 'msg-stream',
  operation: 'update',
  message: { id: 'msg-stream', sessionId: 'session-stream', role: 'assistant', content: 'hello world', finished: true, createdAt: 100 },
});
assert.equal(streamingStore.streamingByMessageId['msg-stream'], undefined, 'overlay cleared when message.finished arrives');

// ── 4. Pending permissions selector still works from runtime permissions.
streamingStore = applyOutputEvent(streamingStore, {
  id: 'perm-event',
  sequence: 200,
  sessionId: 'session-stream',
  kind: 'permission.created',
  entityId: 'perm-1',
  operation: 'append',
  permission: { id: 'perm-1', sessionId: 'session-stream', turnId: 'turn-stream', toolCallId: 'tool-x', toolName: 'bash', action: 'run', status: 'pending', createdAt: 300 },
});
assert.equal(selectPendingPermissions(streamingStore).length, 1, 'pending permission surfaced via runtime event');

// ── 5. applyOutputEvents batch order is preserved.

let batchStore = createOutputStore('session-batch');
batchStore = applyOutputEvents(batchStore, [
  {
    id: 'batch-1',
    sequence: 1,
    sessionId: 'session-batch',
    kind: 'turn.created',
    entityId: 'turn-batch',
    operation: 'append',
    turn: { id: 'turn-batch', sessionId: 'session-batch', status: 'running', userMessageId: 'msg-batch', startedAt: 1 },
  },
  {
    id: 'batch-2',
    sequence: 2,
    sessionId: 'session-batch',
    kind: 'conversation_item.created',
    entityId: 'user-batch',
    operation: 'append',
    item: { id: 'user-batch', kind: 'user_message', sessionId: 'session-batch', turnId: 'turn-batch', sequence: 1000000, role: 'user', content: 'batched', messageId: 'msg-batch', clientRequestId: 'client-batch', createdAt: 1 },
  },
]);
const batchTimeline = selectConversationTimeline(batchStore);
assert.equal(batchTimeline[0].id, 'user-batch', 'batch application delivers items in order');
assert.equal(batchTimeline[0].clientRequestId, 'client-batch', 'runtime item carries clientRequestId through the reducer');

const nearBottomShouldFollow = (distanceToBottom, pinned) => pinned || distanceToBottom < 160;
assert.equal(nearBottomShouldFollow(40, false), true, 'new output follows near bottom');
assert.equal(nearBottomShouldFollow(900, false), false, 'history browsing does not force jump to bottom');

console.log('conversation output store smoke passed');
