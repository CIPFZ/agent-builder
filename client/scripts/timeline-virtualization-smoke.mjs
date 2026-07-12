import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { applyCanonicalConversationBatch, hydrateCanonicalConversationStore } from '../src/runtime/canonicalConversationStore.ts';
import { selectCanonicalConversationTurnViewModels } from '../src/runtime/canonicalConversationView.ts';

const turn = (id, sequence) => ({ id, sessionId: 'session-1', activitySequence: sequence, revision: sequence, createdAt: 1, updatedAt: 1, status: 'completed' });
const message = (id, turnId, sequence) => ({ id, sessionId: 'session-1', turnId, activitySequence: sequence, revision: sequence, createdAt: 1, updatedAt: 1, role: 'assistant', phase: 'intermediate', status: 'completed', content: id });
const snapshot = { schemaVersion: 2, sessionId: 'session-1', cursor: '10', scope: 'window', window: { turnIds: ['turn-1', 'turn-2'], hasMoreBefore: false }, turns: [turn('turn-1', '1'), turn('turn-2', '2')], messages: [message('message-1', 'turn-1', '3'), message('message-2', 'turn-2', '4')], assistantSteps: [], toolCalls: [], toolResults: [], permissions: [], todoPlans: [], agentTasks: [], notices: [] };

let store = hydrateCanonicalConversationStore(snapshot);
const before = selectCanonicalConversationTurnViewModels(store);
const updated = { ...message('message-2', 'turn-2', '11'), content: 'updated' };
store = applyCanonicalConversationBatch(store, { schemaVersion: 2, sessionId: 'session-1', afterCursor: '10', cursor: '11', events: [{ schemaVersion: 2, id: 'event-11', sessionId: 'session-1', turnId: 'turn-2', sequence: '11', createdAt: 1, entityType: 'message', entityId: updated.id, operation: 'upsert', revision: '11', message: updated }] });
const after = selectCanonicalConversationTurnViewModels(store);
assert.equal(after[0].revisionKey, before[0].revisionKey, 'an unrelated Turn keeps a stable render revision key');
assert.notEqual(after[1].revisionKey, before[1].revisionKey, 'the updated Turn receives a new render revision key');

const timelineSource = await readFile(new URL('../src/features/timeline/Timeline.tsx', import.meta.url), 'utf8');
assert.match(timelineSource, /new IntersectionObserver/, 'Turn working set follows viewport proximity');
assert.match(timelineSource, /new ResizeObserver/, 'virtual placeholders retain measured Turn height');
assert.match(timelineSource, /rootMargin: '1000px 0px'/, 'Turn details mount before entering the visible viewport');
assert.match(timelineSource, /keepMounted = isActiveTurnStatus/, 'active Turn output cannot be virtualized away');
assert.match(timelineSource, /previous\.block\.revisionKey === next\.block\.revisionKey/, 'unrelated Turn updates are memoized');

console.log('timeline virtualization smoke passed');
