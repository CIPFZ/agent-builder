import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const root = process.cwd();
const tempDir = path.join(root, 'node_modules', '.tmp', 'conversation-output-smoke');
await mkdir(tempDir, { recursive: true });
await mkdir(path.join(tempDir, 'conversation'), { recursive: true });
await mkdir(path.join(tempDir, 'timeline'), { recursive: true });

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

{
  const source = await readFile(path.join(root, 'src', 'features', 'timeline', 'processDisclosurePolicy.ts'), 'utf8');
  const transpiled = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022, importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove },
  }).outputText.replaceAll('.ts', '.mjs');
  await writeFile(path.join(tempDir, 'timeline', 'processDisclosurePolicy.mjs'), transpiled);
}

for (const name of ['statusMachine', 'turnProjection']) {
  const source = await readFile(path.join(root, 'src', 'runtime', 'conversation', `${name}.ts`), 'utf8');
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022,
      importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove,
    },
  }).outputText.replaceAll('.ts', '.mjs');
  await writeFile(path.join(tempDir, 'conversation', `${name}.mjs`), transpiled);
}

const { createOutputStore } = await import(pathToFileURL(path.join(tempDir, 'outputStore.mjs')).href);
const { addOptimisticUserSubmit, applyOutputEvent, applyOutputEvents, hydrateOutputStore } = await import(
  pathToFileURL(path.join(tempDir, 'outputReducer.mjs')).href
);
const { selectConversationMessages, selectProjectedConversationItems, selectPendingPermissions } = await import(
  pathToFileURL(path.join(tempDir, 'outputSelectors.mjs')).href
);
const { selectConversationTurns } = await import(pathToFileURL(path.join(tempDir, 'conversation', 'turnProjection.mjs')).href);
const { shouldAutoOpenProcess } = await import(pathToFileURL(path.join(tempDir, 'timeline', 'processDisclosurePolicy.mjs')).href);

assert.equal(shouldAutoOpenProcess({ status: 'running', itemStatuses: [] }), true, 'running process auto-opens');
assert.equal(shouldAutoOpenProcess({ status: 'completed', itemStatuses: ['completed'] }), false, 'successful completed process auto-collapses');
assert.equal(shouldAutoOpenProcess({ status: 'failed', itemStatuses: [] }), true, 'failed process remains visible');
assert.equal(shouldAutoOpenProcess({ status: 'completed', failedCount: 1, itemStatuses: ['failed'] }), true, 'partially failed process remains visible');
assert.equal(shouldAutoOpenProcess({ status: 'waiting_permission', itemStatuses: [] }), true, 'permission wait remains visible');

// ── 1. Runtime-owned snapshot: version=1 items are the only conversation source.

const snapshot = {
  sessionId: 'session-runtime',
  cursor: '10',
  version: 1,
  items: [
    { id: 'item-user', kind: 'user_message', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1000000, role: 'user', content: 'run tools', messageId: 'msg-user', clientRequestId: 'client-A', createdAt: 10 },
    { id: 'item-exp', kind: 'exploration_summary', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1050000, status: 'exploring', title: 'Read 2 files', display: { counts: [{ kind: 'file_read', count: 2 }] }, exploration: { status: 'exploring', toolTotal: 2, toolCounts: [{ kind: 'file_read', count: 2 }] }, createdAt: 12 },
    { id: 'item-tool-group', kind: 'tool_group', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1102020, status: 'completed', title: 'Read 2 files', summary: 'Read 2 files', toolCallId: 'tool-1', toolCallIds: ['tool-1', 'tool-2'], display: { kind: 'file_read', quiet: true, groupable: true, counts: [{ kind: 'file_read', count: 2 }] }, createdAt: 22 },
    { id: 'item-stale-progress', kind: 'turn_progress', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 1200000, status: 'running', title: 'stale progress', createdAt: 25 },
    { id: 'item-assistant-final', kind: 'assistant_message', sessionId: 'session-runtime', turnId: 'turn-1', sequence: 91000039, role: 'assistant', phase: 'final', content: 'done', status: 'completed', messageId: 'msg-final', createdAt: 40 },
  ],
  messages: [
    { id: 'msg-user', sessionId: 'session-runtime', role: 'user', content: 'run tools', clientRequestId: 'client-A', createdAt: 10 },
    { id: 'msg-final', sessionId: 'session-runtime', role: 'assistant', content: 'done', finished: true, createdAt: 40 },
  ],
  turns: [
    { id: 'turn-1', sessionId: 'session-runtime', status: 'completed', userMessageId: 'msg-user', latestAssistantMessageId: 'msg-final', startedAt: 10, finishedAt: 40 },
  ],
  toolCalls: [
    { id: 'tool-1', sessionId: 'session-runtime', turnId: 'turn-1', name: 'view', source: 'builtin', kind: 'file_read', status: 'completed', quiet: true, groupable: true, display: { kind: 'file_read', title: 'Read file' }, startedAt: 21, finishedAt: 22 },
    { id: 'tool-2', sessionId: 'session-runtime', turnId: 'turn-1', name: 'view', source: 'builtin', kind: 'file_read', status: 'completed', quiet: true, groupable: true, display: { kind: 'file_read', title: 'Read file' }, startedAt: 23, finishedAt: 24 },
  ],
};

let optimisticStore = createOutputStore('session-runtime');
optimisticStore = addOptimisticUserSubmit(optimisticStore, { clientRequestId: 'client-A', prompt: 'run tools', createdAt: 5, status: 'submitting' });
optimisticStore = hydrateOutputStore(snapshot, optimisticStore);

const runtimeTimeline = selectProjectedConversationItems(optimisticStore);
assert.deepEqual(runtimeTimeline.map((item) => item.id), ['item-user', 'item-exp', 'item-tool-group', 'item-stale-progress', 'item-assistant-final'], 'runtime sequence controls item ordering');
assert.equal(runtimeTimeline.find((item) => item.id === 'item-tool-group')?.kind, 'tool_group', 'runtime tool group is rendered directly');
const exploration = runtimeTimeline.find((item) => item.id === 'item-exp');
assert.equal(exploration?.exploration?.toolTotal, 2, 'exploration_summary carries toolTotal');
assert.deepEqual(exploration?.displayCounts?.[0], { kind: 'file_read', count: 2 }, 'exploration_summary counts propagate to displayCounts');
const runtimeMessages = selectConversationMessages(optimisticStore);
assert.equal(runtimeMessages.some((message) => message.id.startsWith('optimistic-')), false, 'runtime user_message with matching clientRequestId replaces optimistic submit');
const runtimeTurns = selectConversationTurns(optimisticStore);
assert.equal(runtimeTurns.length, 1, 'turn projection uses runtime turns as the conversation boundary');
assert.equal(runtimeTurns[0].user?.messageId, 'msg-user', 'turn projection uses authoritative userMessageId');
assert.equal(runtimeTurns[0].final?.messageId, 'msg-final', 'turn projection uses authoritative final assistant id');
assert.deepEqual(runtimeTurns[0].process.items.map((item) => item.id), ['item-tool-group', 'item-stale-progress'], 'summary and final artifacts are excluded from process rows');
assert.equal(runtimeTurns[0].process.items.find((item) => item.id === 'item-stale-progress')?.status, 'completed', 'terminal turn normalizes stale active process state');

// Terminal turn state is monotonic: a delayed running event must not reopen it.
optimisticStore = applyOutputEvent(optimisticStore, {
  id: 'stale-running-turn',
  sequence: 999,
  sessionId: 'session-runtime',
  turnId: 'turn-1',
  kind: 'turn.updated',
  entityId: 'turn-1',
  operation: 'update',
  turn: { id: 'turn-1', sessionId: 'session-runtime', status: 'running' },
});
assert.equal(selectConversationTurns(optimisticStore)[0].status, 'completed', 'delayed active state cannot overwrite terminal turn state');

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
assert.deepEqual(selectProjectedConversationItems(emptyRuntimeContractStore), [], 'runtime snapshot with empty items must not fall back to legacy item composition');
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

let timelineAfterDeltas = selectProjectedConversationItems(streamingStore);
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
timelineAfterDeltas = selectProjectedConversationItems(streamingStore);
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
timelineAfterDeltas = selectProjectedConversationItems(streamingStore);
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
const batchTimeline = selectProjectedConversationItems(batchStore);
assert.equal(batchTimeline[0].id, 'user-batch', 'batch application delivers items in order');
assert.equal(batchTimeline[0].clientRequestId, 'client-batch', 'runtime item carries clientRequestId through the reducer');

const nearBottomShouldFollow = (distanceToBottom, pinned) => pinned || distanceToBottom < 160;
assert.equal(nearBottomShouldFollow(40, false), true, 'new output follows near bottom');
assert.equal(nearBottomShouldFollow(900, false), false, 'history browsing does not force jump to bottom');

// ── 6. WP5: compact_boundary item's Compact payload (trigger/tokens/summary)
// survives the runtime snapshot -> outputStore -> selector round trip so
// the timeline compact trace row can render its meta line + summary text.

let compactStore = createOutputStore('session-compact');
compactStore = hydrateOutputStore({
  sessionId: 'session-compact',
  cursor: '1',
  version: 1,
  items: [
    {
      id: 'compact-1',
      kind: 'compact_boundary',
      sessionId: 'session-compact',
      turnId: 'turn-compact',
      sequence: 5200,
      status: 'completed',
      title: 'manual',
      summary: '3 messages, 0 tool outputs',
      compact: {
        trigger: 'manual',
        status: 'completed',
        preTokens: 5000,
        postTokens: 800,
        summarizedCount: 3,
        summaryMessageId: 'summary-1',
        summaryText: 'the compacted summary text',
      },
      createdAt: 40,
      updatedAt: 41,
    },
  ],
  messages: [],
}, compactStore);
const compactTimeline = selectProjectedConversationItems(compactStore);
const compactItem = compactTimeline.find((item) => item.kind === 'compact_boundary');
assert.equal(compactItem?.compact?.trigger, 'manual', 'compact item carries trigger');
assert.equal(compactItem?.compact?.preTokens, 5000, 'compact item carries preTokens');
assert.equal(compactItem?.compact?.postTokens, 800, 'compact item carries postTokens');
assert.equal(compactItem?.compact?.summarizedCount, 3, 'compact item carries summarizedCount');
assert.equal(compactItem?.compact?.summaryText, 'the compacted summary text', 'compact item carries the full summary text');

console.log('conversation output store smoke passed');
