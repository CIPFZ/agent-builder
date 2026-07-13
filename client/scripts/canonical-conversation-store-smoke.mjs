import assert from 'node:assert/strict';
import { applyCanonicalConversationBatch, createCanonicalConversationStore, hydrateCanonicalConversationStore } from '../src/runtime/canonicalConversationStore.ts';
import { selectCanonicalConversationTurns, selectTodoPlanForTurn } from '../src/runtime/canonicalConversationSelectors.ts';
import { canonicalToolGroupKey, groupCanonicalProcess } from '../src/runtime/canonicalConversationPresentation.ts';
import { selectCanonicalConversationTurnViewModels } from '../src/runtime/canonicalConversationView.ts';
import { compactProcessItems } from '../src/features/timeline/processGrouping.ts';

const meta = (id, turnId = 'turn-1', revision = '1', activitySequence = '1') => ({ id, sessionId: 'session-1', turnId, revision, activitySequence, createdAt: 1, updatedAt: 1 });
const turn = (revision = '1') => ({ ...meta('turn-1', undefined, revision), status: 'running', userMessageId: 'user-1', finalMessageId: 'final-1' });
const message = (id, phase, revision = '1', activitySequence = '2') => ({ ...meta(id, 'turn-1', revision, activitySequence), role: id === 'user-1' ? 'user' : 'assistant', phase, status: 'completed', content: id });
const tool = (id, revision = '1', status = 'running', activitySequence = '4') => ({ ...meta(id, 'turn-1', revision, activitySequence), assistantStepId: 'step-1', name: 'shell', source: 'builtin', kind: 'command', status });
const snapshot = (scope = 'full', cursor = '10', overrides = {}) => ({ schemaVersion: 2, sessionId: 'session-1', cursor, scope, turns: [turn()], messages: [message('user-1', 'intermediate', '1', '1'), message('middle-1', 'intermediate'), message('final-1', 'final', '1', '9')], assistantSteps: [], toolCalls: [tool('tool-1')], toolResults: [], permissions: [], todoPlans: [], agentTasks: [], notices: [], ...overrides });
const event = (entityType, entityId, revision, payload, operation = 'upsert') => ({ schemaVersion: 2, id: `event-${entityId}-${revision}`, sessionId: 'session-1', turnId: 'turn-1', sequence: revision, createdAt: 1, entityType, entityId, operation, revision, ...(payload ? { [entityType]: payload } : {}) });
const batch = (afterCursor, cursor, events, extra = {}) => ({ schemaVersion: 2, sessionId: 'session-1', afterCursor, cursor, events, ...extra });

let store = hydrateCanonicalConversationStore(snapshot());
assert.equal(store.toolCallsById['tool-1'].id, 'tool-1');
assert.ok(store.entityKeysByTurnId['turn-1'].includes('toolCall:tool-1'), 'snapshot hydration builds the Turn entity index');
store = hydrateCanonicalConversationStore(snapshot('window', '11', { turns: [], messages: [], toolCalls: [], window: { turnIds: [] } }), store);
assert.ok(store.toolCallsById['tool-1'], 'window omission does not delete');

const messagesBeforeToolUpdate = store.messagesById;
const permissionsBeforeToolUpdate = store.permissionsById;
store = applyCanonicalConversationBatch(store, batch('11', '12', [event('toolCall', 'tool-1', '12', tool('tool-1', '12', 'completed'))]));
assert.equal(store.messagesById, messagesBeforeToolUpdate, 'tool-only batch preserves the untouched Message dictionary reference');
assert.equal(store.permissionsById, permissionsBeforeToolUpdate, 'tool-only batch preserves the untouched Permission dictionary reference');
store = hydrateCanonicalConversationStore(snapshot('full', '10', { toolCalls: [] }), store);
assert.equal(store.toolCallsById['tool-1'].revision, '12', 'older full snapshot preserves newer local entity');
assert.equal(applyCanonicalConversationBatch(store, batch('12', '13', [event('toolCall', 'tool-1', '11', tool('tool-1', '11'))])).toolCallsById['tool-1'].revision, '12');
const snapshotConflict = hydrateCanonicalConversationStore(snapshot('window', '12', { toolCalls: [tool('tool-1', '12', 'failed')] }), store);
assert.equal(snapshotConflict.recovery.reason, 'revision_conflict', 'same-revision snapshot payload conflict requires recovery');

const conflict = applyCanonicalConversationBatch(store, batch('12', '13', [event('toolCall', 'tool-1', '12', tool('tool-1', '12', 'failed'))]));
assert.equal(conflict.recovery.reason, 'revision_conflict');
assert.equal(conflict.cursor, '12', 'conflicting batch rolls back atomically');
const multiConflict = applyCanonicalConversationBatch(store, batch('12', '13', [event('toolCall', 'tool-2', '13', tool('tool-2', '13')), event('toolCall', 'tool-1', '12', tool('tool-1', '12', 'failed'))]));
assert.equal(multiConflict.toolCallsById['tool-2'], undefined, 'a later conflict rolls back earlier events in the batch');
const deleted = applyCanonicalConversationBatch(store, batch('12', '13', [event('toolCall', 'tool-1', '13', undefined, 'delete')]));
assert.equal(deleted.toolCallsById['tool-1'], undefined);
assert.equal(deleted.entityKeysByTurnId['turn-1'].includes('toolCall:tool-1'), false, 'deleting an entity removes its Turn index entry');
assert.equal(applyCanonicalConversationBatch(deleted, batch('13', '14', [event('toolCall', 'tool-1', '12', tool('tool-1', '12'))])).toolCallsById['tool-1'], undefined, 'tombstone blocks stale resurrection');
assert.equal(applyCanonicalConversationBatch(deleted, batch('99', '100', [])).recovery.reason, 'cursor_gap');
assert.equal(applyCanonicalConversationBatch(deleted, batch('13', '14', [], { snapshotRequired: true })).recovery.reason, 'snapshot_required');
assert.equal(applyCanonicalConversationBatch(deleted, { ...batch('13', '14', []), sessionId: 'other' }).recovery.reason, 'session_mismatch');

const huge = hydrateCanonicalConversationStore(snapshot('full', '90071992547409930', { turns: [{ ...turn(), activitySequence: '90071992547409930' }, { ...turn(), id: 'turn-2', activitySequence: '90071992547409931' }] }));
assert.deepEqual(selectCanonicalConversationTurns(huge).map((item) => item.turn.id), ['turn-1', 'turn-2'], 'ordering remains precise beyond Number.MAX_SAFE_INTEGER');
const hugeRevision = applyCanonicalConversationBatch(huge, batch('90071992547409930', '90071992547409931', [event('toolCall', 'tool-1', '90071992547409931', tool('tool-1', '90071992547409931'))]));
assert.equal(hugeRevision.toolCallsById['tool-1'].revision, '90071992547409931');

const projected = selectCanonicalConversationTurns(hydrateCanonicalConversationStore(snapshot()))[0];
assert.equal(projected.user.id, 'user-1');
assert.equal(projected.final.id, 'final-1');
assert.ok(projected.process.some((item) => item.id === 'tool-1'), 'tool remains before/after final');
assert.deepEqual(compactProcessItems([{ id: 'empty', kind: 'assistant_message', content: '', status: 'completed' }, { id: 'visible', kind: 'assistant_message', content: 'kept', status: 'completed' }]).map((item) => item.id), ['visible'], 'empty narration cannot create invisible flex-gap rows');
const beforeFinalStore = hydrateCanonicalConversationStore(snapshot('full', '9', { turns: [{ ...turn(), finalMessageId: undefined }], messages: snapshot().messages.filter((item) => item.id !== 'final-1') }));
const beforeIDs = selectCanonicalConversationTurns(beforeFinalStore)[0].process.map((item) => item.id);
const afterIDs = projected.process.map((item) => item.id);
assert.deepEqual(afterIDs.filter((id) => id !== 'middle-1'), beforeIDs.filter((id) => id !== 'middle-1'), 'final arrival preserves existing process IDs');
const windowProjection = selectCanonicalConversationTurns(hydrateCanonicalConversationStore(snapshot('window', '11', { turns: [], messages: [], toolCalls: [], window: { turnIds: [] } }), hydrateCanonicalConversationStore(snapshot())))[0];
assert.deepEqual(windowProjection.process.map((item) => item.id), projected.process.map((item) => item.id), 'window omission preserves selector identity');
const wrongFinal = hydrateCanonicalConversationStore(snapshot('full', '10', { turns: [{ ...turn(), finalMessageId: 'middle-1' }] }));
assert.equal(selectCanonicalConversationTurns(wrongFinal)[0].final, undefined, 'final ownership requires final phase');

const t1 = tool('tool-1'); const t2 = tool('tool-2', '2', 'completed', '5');
assert.equal(canonicalToolGroupKey(t1), canonicalToolGroupKey({ ...t1, revision: '9', status: 'completed' }));
const one = groupCanonicalProcess([t1]); const two = groupCanonicalProcess([t1, t2]);
assert.equal(one[0].key, two[0].key, 'group wrapper key survives membership changes');
assert.equal(two[0].tools.length, 2);
const collisionItems = groupCanonicalProcess([{ ...message('shared', 'intermediate') }, { ...meta('shared'), kind: 'hook', status: 'completed' }]);
assert.notEqual(collisionItems[0].key, collisionItems[1].key, 'cross-type IDs cannot collide');

const todoStore = hydrateCanonicalConversationStore(snapshot('full', '10', { todoPlans: [{ ...meta('todo-1'), ownerTurnId: 'turn-1', status: 'running', items: [] }, { ...meta('todo-other', 'turn-2'), ownerTurnId: 'turn-2', status: 'running', items: [] }] }));
assert.equal(selectTodoPlanForTurn(todoStore, 'turn-1').id, 'todo-1');
assert.ok(todoStore.entityKeysByTurnId['turn-1'].includes('todoPlan:todo-1'), 'Todo plans are indexed by semantic owner Turn');

const manyTurns = Array.from({ length: 500 }, (_, index) => ({ ...turn(), id: `bulk-turn-${index}`, userMessageId: undefined, finalMessageId: undefined, activitySequence: String(index + 1) }));
const manyMessages = manyTurns.map((item, index) => ({ ...message(`bulk-message-${index}`, 'intermediate'), turnId: item.id, activitySequence: String(index + 501) }));
const indexedLargeStore = hydrateCanonicalConversationStore(snapshot('full', '2000', { turns: manyTurns, messages: manyMessages, toolCalls: [] }));
const selectedLargeTurn = selectCanonicalConversationTurns(indexedLargeStore).find((item) => item.turn.id === 'bulk-turn-321');
assert.deepEqual(selectedLargeTurn.process.map((item) => item.id), ['bulk-message-321'], 'a Turn selector reads only entities from its canonical Turn index');

const optimistic = { 'request-1': { clientRequestId: 'request-1', sessionId: 'session-1', prompt: 'stay visible', createdAt: 1, status: 'submitting' } };
const echoedUser = { ...message('user-echo', 'intermediate'), role: 'user', clientRequestId: 'request-1' };
const messageFirst = hydrateCanonicalConversationStore(snapshot('full', '20', { turns: [{ ...turn(), userMessageId: undefined, finalMessageId: undefined }], messages: [echoedUser], toolCalls: [] }));
const messageFirstTurns = selectCanonicalConversationTurnViewModels(messageFirst, undefined, optimistic);
assert.equal(messageFirstTurns.some((item) => item.id === 'optimistic:request-1'), false, 'an active canonical Turn adopts optimism before its user-message link arrives');
assert.equal(messageFirstTurns.find((item) => item.id === 'turn-1')?.user?.content, 'stay visible', 'adoption keeps the optimistic user bubble visible without a second process row');
const turnOwned = hydrateCanonicalConversationStore(snapshot('full', '21', { turns: [{ ...turn(), userMessageId: 'user-echo', finalMessageId: undefined }], messages: [echoedUser], toolCalls: [] }), messageFirst);
assert.equal(selectCanonicalConversationTurnViewModels(turnOwned, undefined, optimistic).some((item) => item.id === 'optimistic:request-1'), false, 'owned canonical user message settles optimism without a duplicate');
assert.equal(createCanonicalConversationStore('session-1').cursor, '0');
console.log('canonical conversation store smoke passed');
