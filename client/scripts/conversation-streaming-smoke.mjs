// End-to-end smoke test for the frontend streaming reducer + selector overlay.
// Scripts a plausible turn event sequence (user, deltas, tool started, tool
// completed via full item update, final message) and asserts the intermediate
// timeline states the UI must render.

import assert from 'node:assert/strict';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const root = process.cwd();
const tempDir = path.join(root, 'node_modules', '.tmp', 'conversation-streaming-smoke');
await mkdir(tempDir, { recursive: true });
await mkdir(path.join(tempDir, 'conversation'), { recursive: true });

for (const name of ['outputStore', 'outputReducer', 'outputSelectors', 'outputStream']) {
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

for (const name of ['statusMachine']) {
  const source = await readFile(path.join(root, 'src', 'runtime', 'conversation', `${name}.ts`), 'utf8');
  const transpiled = ts.transpileModule(source, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ES2022, importsNotUsedAsValues: ts.ImportsNotUsedAsValues.Remove },
  }).outputText.replaceAll('.ts', '.mjs');
  await writeFile(path.join(tempDir, 'conversation', `${name}.mjs`), transpiled);
}

const { createOutputStore } = await import(pathToFileURL(path.join(tempDir, 'outputStore.mjs')).href);
const { applyOutputEvents, addOptimisticUserSubmit } = await import(pathToFileURL(path.join(tempDir, 'outputReducer.mjs')).href);
const { selectProjectedConversationItems } = await import(pathToFileURL(path.join(tempDir, 'outputSelectors.mjs')).href);
const { mergeAdjacentDeltas } = await import(pathToFileURL(path.join(tempDir, 'outputStream.mjs')).href);

const sessionId = 'session-turn';
const turnId = 'turn-1';
const messageId = 'msg-assistant';

let store = createOutputStore(sessionId);

// Batch 1: turn started + user message.
store = applyOutputEvents(store, [
  { id: 'turn-1', sequence: 1, sessionId, turnId, kind: 'turn.created', entityId: turnId, operation: 'append',
    turn: { id: turnId, sessionId, status: 'running', userMessageId: 'msg-user', startedAt: 1 } },
  { id: 'item-user', sequence: 2, sessionId, turnId, kind: 'conversation_item.created', entityId: 'user-1', operation: 'append',
    item: { id: 'user-1', kind: 'user_message', sessionId, turnId, sequence: 1000000, role: 'user', content: 'go', messageId: 'msg-user', clientRequestId: 'client-1', createdAt: 5 } },
]);

let timeline = selectProjectedConversationItems(store);
assert.deepEqual(timeline.map((item) => item.kind), ['user_message'], 'after user message we only see user_message');

// Batch 2: assistant streaming message + text deltas.
store = applyOutputEvents(store, [
  { id: 'assistant-created', sequence: 3, sessionId, turnId, kind: 'conversation_item.created', entityId: 'assistant-1', operation: 'append',
    item: { id: 'assistant-1', kind: 'assistant_message', sessionId, turnId, sequence: 1050000, role: 'assistant', phase: 'intermediate', content: '', status: 'streaming', messageId, createdAt: 10 } },
  { id: 'delta-1', sequence: 0, sessionId, kind: 'output.text.delta', entityId: messageId, operation: 'delta',
    textDelta: { messageId, partType: 'text', delta: 'plan', contentLen: 4 } },
]);

timeline = selectProjectedConversationItems(store);
let assistant = timeline.find((item) => item.messageId === messageId);
assert.equal(assistant?.content, 'plan', 'delta content populates assistant item via streaming overlay');
assert.equal(assistant?.streaming, true, 'assistant item marked streaming while overlay is active');

// Batch 3: transport preserves individual deltas so duplicate/out-of-order
// fragments remain visible to the reducer's contentLen guard.
const streamed = mergeAdjacentDeltas([
  { id: 'delta-2', sequence: 0, sessionId, kind: 'output.text.delta', entityId: messageId, operation: 'delta',
    textDelta: { messageId, partType: 'text', delta: 'ning', contentLen: 8 } },
  { id: 'delta-3', sequence: 0, sessionId, kind: 'output.text.delta', entityId: messageId, operation: 'delta',
    textDelta: { messageId, partType: 'text', delta: ', running tool', contentLen: 22 } },
]);
assert.equal(streamed.length, 2, 'transport preserves adjacent deltas for reducer-level idempotency');

store = applyOutputEvents(store, [
  ...streamed,
  { id: 'tool-started', sequence: 4, sessionId, turnId, kind: 'conversation_item.created', entityId: 'tool-1', operation: 'append',
    item: { id: 'tool-1', kind: 'tool_call', sessionId, turnId, sequence: 1100000, status: 'running', title: 'view', toolCallId: 'call-1', display: { kind: 'file_read', title: 'view', defaultExpanded: true }, createdAt: 15 } },
  { id: 'tool-call', sequence: 5, sessionId, turnId, kind: 'tool_call.created', entityId: 'call-1', operation: 'append',
    toolCall: { id: 'call-1', sessionId, turnId, name: 'view', source: 'builtin', kind: 'file_read', status: 'running', display: { kind: 'file_read', title: 'view' }, startedAt: 15 } },
]);

timeline = selectProjectedConversationItems(store);
assistant = timeline.find((item) => item.messageId === messageId);
assert.equal(assistant?.content, 'planning, running tool', 'stream text continues to grow');
assert.equal(assistant?.streaming, true, 'assistant still streaming until the message is finished');
const toolItem = timeline.find((item) => item.id === 'tool-1');
assert.equal(toolItem?.status, 'running', 'tool_call surfaced live with running status');

// Batch 4: tool completed + exploration summary update.
store = applyOutputEvents(store, [
  { id: 'tool-completed', sequence: 6, sessionId, turnId, kind: 'conversation_item.updated', entityId: 'tool-1', operation: 'update',
    item: { id: 'tool-1', kind: 'tool_call', sessionId, turnId, sequence: 1100000, status: 'completed', title: 'view', toolCallId: 'call-1', display: { kind: 'file_read', title: 'view', defaultExpanded: false }, createdAt: 15, updatedAt: 20 } },
  { id: 'exp-updated', sequence: 7, sessionId, turnId, kind: 'conversation_item.updated', entityId: `exploration-${turnId}`, operation: 'update',
    item: { id: `exploration-${turnId}`, kind: 'exploration_summary', sessionId, turnId, sequence: 1030000, status: 'exploring', title: 'Read 1 file', exploration: { status: 'exploring', toolTotal: 1, toolCounts: [{ kind: 'file_read', count: 1 }] }, display: { counts: [{ kind: 'file_read', count: 1 }] }, createdAt: 12, updatedAt: 20 } },
]);

timeline = selectProjectedConversationItems(store);
assert.equal(timeline.find((item) => item.id === 'tool-1')?.status, 'completed', 'tool item promoted to completed');
assert.equal(timeline.find((item) => item.id === `exploration-${turnId}`)?.exploration?.toolTotal, 1, 'exploration summary counts the completed tool');

// Batch 5: final assistant message + message finished — overlay must clear.
store = applyOutputEvents(store, [
  { id: 'assistant-final', sequence: 8, sessionId, turnId, kind: 'conversation_item.updated', entityId: 'assistant-1', operation: 'update',
    item: { id: 'assistant-1', kind: 'assistant_message', sessionId, turnId, sequence: 90000000, role: 'assistant', phase: 'final', content: 'planning, running tool - done.', status: 'completed', messageId, createdAt: 10, updatedAt: 30 } },
  { id: 'msg-finished', sequence: 9, sessionId, turnId, kind: 'message.updated', entityId: messageId, operation: 'update',
    message: { id: messageId, sessionId, role: 'assistant', content: 'planning, running tool - done.', finished: true, createdAt: 10, updatedAt: 30 } },
]);

assert.equal(store.streamingByMessageId[messageId], undefined, 'streaming overlay cleared after finished message');
timeline = selectProjectedConversationItems(store);
assistant = timeline.find((item) => item.messageId === messageId);
assert.equal(assistant?.phase, 'final', 'assistant item is now in final phase');
assert.notEqual(assistant?.streaming, true, 'assistant item no longer flagged streaming');
assert.equal(assistant?.content, 'planning, running tool - done.', 'final assistant content matches runtime snapshot');

// Terminal and session boundaries reject late/foreign activity.
store = applyOutputEvents(store, [
  { id: 'turn-finished', sequence: 10, sessionId, turnId, kind: 'turn.updated', entityId: turnId, operation: 'update',
    turn: { id: turnId, sessionId, status: 'completed', startedAt: 10, finishedAt: 31 } },
  { id: 'delta-late', sequence: 11, sessionId, turnId, kind: 'output.text.delta', entityId: messageId, operation: 'delta',
    textDelta: { messageId, turnId, partType: 'text', delta: ' late', contentLen: 33 } },
  { id: 'foreign-item', sequence: 12, sessionId: 'other-session', kind: 'conversation_item.created', entityId: 'foreign', operation: 'append',
    item: { id: 'foreign', kind: 'assistant_message', sessionId: 'other-session', sequence: 1, role: 'assistant', content: 'wrong session' } },
]);
assert.equal(store.streamingByMessageId[messageId], undefined, 'late delta cannot revive a terminal turn');
assert.equal(store.itemsById.foreign, undefined, 'foreign-session event cannot pollute the active store');

const duplicateDeltas = mergeAdjacentDeltas([
  { id: 'dup-1', sequence: 0, sessionId, kind: 'output.text.delta', entityId: 'dup-message', operation: 'delta', textDelta: { messageId: 'dup-message', partType: 'text', delta: 'abc', contentLen: 3 } },
  { id: 'dup-2', sequence: 0, sessionId, kind: 'output.text.delta', entityId: 'dup-message', operation: 'delta', textDelta: { messageId: 'dup-message', partType: 'text', delta: 'abc', contentLen: 3 } },
]);
const duplicateStore = applyOutputEvents(createOutputStore(sessionId), duplicateDeltas);
assert.equal(duplicateStore.streamingByMessageId['dup-message']?.text, 'abc', 'duplicate adjacent delta is rejected without duplicated text');

// --- Scenario 2: optimistic submit ordering with realistic runtime-scale
// sequences (sequence = turnStartMs/100 * 100000 + rank + intra). The
// optimistic user bubble must sort after all history and before the items of
// the turn it triggers, both before and after the runtime user_message echo.
const seqBase = (startedAtMs, rank = 0) => Math.floor(startedAtMs / 100) * 100000 + rank;
const oldTurnStart = 1_751_400_000_000;
const submitAt = 1_751_500_000_000;
const newTurnStart = submitAt + 300;

let store2 = createOutputStore('session-2');
store2 = applyOutputEvents(store2, [
  { id: 'old-user', sequence: 1, sessionId: 'session-2', kind: 'conversation_item.created', entityId: 'old-user', operation: 'append',
    item: { id: 'old-user', kind: 'user_message', sessionId: 'session-2', turnId: 'turn-old', sequence: seqBase(oldTurnStart), role: 'user', content: 'earlier question', createdAt: oldTurnStart } },
  { id: 'old-final', sequence: 2, sessionId: 'session-2', kind: 'conversation_item.created', entityId: 'old-final', operation: 'append',
    item: { id: 'old-final', kind: 'assistant_message', sessionId: 'session-2', turnId: 'turn-old', sequence: seqBase(oldTurnStart, 99000), role: 'assistant', phase: 'final', content: 'earlier answer', status: 'completed', createdAt: oldTurnStart + 2000 } },
]);
store2 = addOptimisticUserSubmit(store2, { clientRequestId: 'client-new', prompt: 'new question', createdAt: submitAt, status: 'submitting' });

let timeline2 = selectProjectedConversationItems(store2);
assert.deepEqual(timeline2.map((item) => item.id), ['old-user', 'old-final', 'optimistic-client-new'],
  'optimistic submit sorts after history, not at the top of the timeline');

// Streaming response items arrive before the user_message echo.
store2 = applyOutputEvents(store2, [
  { id: 'new-assistant', sequence: 3, sessionId: 'session-2', kind: 'conversation_item.created', entityId: 'new-assistant', operation: 'append',
    item: { id: 'new-assistant', kind: 'assistant_message', sessionId: 'session-2', turnId: 'turn-new', sequence: seqBase(newTurnStart, 1010), role: 'assistant', phase: 'intermediate', content: 'thinking', status: 'streaming', messageId: 'msg-new-assistant', createdAt: newTurnStart + 400 } },
]);
timeline2 = selectProjectedConversationItems(store2);
assert.deepEqual(timeline2.map((item) => item.id), ['old-user', 'old-final', 'optimistic-client-new', 'new-assistant'],
  'streaming response renders below the optimistic user bubble, never above it');

// The runtime user_message echo replaces the optimistic bubble in place.
store2 = applyOutputEvents(store2, [
  { id: 'new-user-echo', sequence: 4, sessionId: 'session-2', kind: 'conversation_item.created', entityId: 'new-user', operation: 'append',
    item: { id: 'new-user', kind: 'user_message', sessionId: 'session-2', turnId: 'turn-new', sequence: seqBase(newTurnStart), role: 'user', content: 'new question', clientRequestId: 'client-new', createdAt: submitAt } },
]);
timeline2 = selectProjectedConversationItems(store2);
assert.deepEqual(timeline2.map((item) => item.id), ['old-user', 'old-final', 'new-user', 'new-assistant'],
  'runtime echo replaces the optimistic bubble and keeps send -> response order');

console.log('conversation streaming smoke passed');
