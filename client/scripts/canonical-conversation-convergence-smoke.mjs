import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createCanonicalConversationCoordinator } from '../src/runtime/canonicalConversationCoordinator.ts';
import { subscribeCanonicalConversation } from '../src/runtime/canonicalConversationStream.ts';
import { hydrateCanonicalConversationStore } from '../src/runtime/canonicalConversationStore.ts';
import { selectCanonicalConversationTurnViewModels } from '../src/runtime/canonicalConversationView.ts';

const wait = () => new Promise((resolve) => setTimeout(resolve, 0));
const snapshot = (sessionId, cursor, toolStatus = 'running') => ({ schemaVersion: 2, sessionId, cursor, scope: 'full', turns: [{ id: 'turn-1', sessionId, activitySequence: '1', revision: '1', createdAt: 1, updatedAt: 1, status: 'running' }], messages: [], assistantSteps: [], toolCalls: [{ id: 'tool-1', sessionId, turnId: 'turn-1', activitySequence: '2', revision: cursor, createdAt: 1, updatedAt: 1, name: 'shell', source: 'builtin', status: toolStatus }], toolResults: [], permissions: [], todoPlans: [], agentTasks: [], notices: [] });
const upsertBatch = (sessionId, afterCursor, cursor, status = 'completed') => ({ schemaVersion: 2, sessionId, afterCursor, cursor, events: [{ schemaVersion: 2, id: `event-${cursor}`, sessionId, turnId: 'turn-1', sequence: cursor, createdAt: 1, entityType: 'toolCall', entityId: 'tool-1', operation: 'upsert', revision: cursor, toolCall: { ...snapshot(sessionId, cursor, status).toolCalls[0], status } }] });

const fetches = []; const fetchRequests = []; const subscriptions = []; const stores = []; const handlers = new Map();
const coordinator = createCanonicalConversationCoordinator({
  fetchSnapshot: async (sessionId, request) => { fetches.push(sessionId); fetchRequests.push(request); return snapshot(sessionId, sessionId === 'A' ? '90071992547409930' : '5'); },
  subscribe: (sessionId, after, nextHandlers) => { subscriptions.push({ sessionId, after }); handlers.set(sessionId, nextHandlers); return () => undefined; },
  onStore: (store) => stores.push(store),
});
coordinator.activate('A'); await wait(); await wait();
assert.deepEqual(fetches, ['A'], 'first activation hydrates exactly once');
assert.deepEqual(fetchRequests[0], { scope: 'window', limit: 30 }, 'first activation requests only the newest bounded Turn window');
assert.equal(subscriptions[0].after, '90071992547409930', 'cursor remains a decimal string');
handlers.get('A').onBatch(upsertBatch('A', '90071992547409930', '90071992547409931'));
assert.equal(stores.at(-1).toolCallsById['tool-1'].status, 'completed');
const oldAHandler = handlers.get('A');
coordinator.activate('B'); await wait(); await wait();
oldAHandler.onBatch(upsertBatch('A', '90071992547409931', '90071992547409932', 'failed'));
assert.equal(stores.at(-1).sessionId, 'B', 'late batch from old generation is ignored');
coordinator.activate('A'); await wait();
assert.deepEqual(fetches, ['A', 'B'], 'switching back uses cache rather than snapshot');
assert.equal(subscriptions.at(-1).after, '90071992547409931', 'switch-back catches up from cached cursor');
handlers.get('A').onBatch({ schemaVersion: 2, sessionId: 'A', afterCursor: '90071992547409931', cursor: '90071992547409931', events: [], snapshotRequired: true });
await wait(); await wait();
assert.deepEqual(fetches, ['A', 'B', 'A'], 'explicit recovery performs one snapshot');

let retryFetches = 0;
const retryCoordinator = createCanonicalConversationCoordinator({ fetchSnapshot: async () => { retryFetches += 1; if (retryFetches === 1) throw new Error('transient'); return snapshot('R', '1'); }, subscribe: () => () => undefined, onStore: () => undefined, retryDelayMs: () => 0 });
retryCoordinator.activate('R'); await wait(); await wait(); await wait();
assert.equal(retryFetches, 2, 'transient recovery failure retries with generation-aware backoff');
retryCoordinator.stop();

const pageRequests = []; const pageStores = [];
const pageSnapshot = (turnId, hasMoreBefore) => ({ ...snapshot('P', '20'), scope: 'window', turns: [{ id: turnId, sessionId: 'P', activitySequence: turnId === 'turn-new' ? '10' : '1', revision: '1', createdAt: 1, updatedAt: 1, status: 'completed' }], toolCalls: [], window: { turnIds: [turnId], hasMoreBefore } });
const pageCoordinator = createCanonicalConversationCoordinator({
  fetchSnapshot: async (_sessionId, request) => { pageRequests.push(request); return request?.before ? pageSnapshot('turn-old', false) : pageSnapshot('turn-new', true); },
  subscribe: () => () => undefined,
  onStore: (store) => pageStores.push(store),
});
pageCoordinator.activate('P'); await wait(); await wait();
assert.equal(await pageCoordinator.loadEarlier('P'), true, 'earlier history page is loaded on demand');
assert.deepEqual(pageRequests[1], { scope: 'window', limit: 30, before: 'turn-new' });
assert.deepEqual(Object.keys(pageStores.at(-1).turnsById).sort(), ['turn-new', 'turn-old'], 'historical window merges without replacing the newest window');
assert.equal(pageStores.at(-1).cursor, '20', 'historical loading does not regress the live cursor');
assert.equal(await pageCoordinator.loadEarlier('P'), false, 'pagination stops when Runtime reports no earlier history');

const lruCoordinator = createCanonicalConversationCoordinator({ fetchSnapshot: async (sessionId) => snapshot(sessionId, '1'), subscribe: () => () => undefined, onStore: () => undefined });
for (const sessionId of ['L1', 'L2', 'L3']) { lruCoordinator.activate(sessionId); await wait(); await wait(); }
assert.equal(lruCoordinator.cached('L1'), undefined, 'canonical Session cache evicts the least recently used window');
assert.ok(lruCoordinator.cached('L2') && lruCoordinator.cached('L3'), 'canonical Session cache retains only two recent windows');

const groupedSnapshot = snapshot('G', '10');
groupedSnapshot.messages = [{ id: 'middle', sessionId: 'G', turnId: 'turn-1', activitySequence: '3', revision: '1', createdAt: 1, updatedAt: 1, role: 'assistant', phase: 'intermediate', status: 'completed', content: 'between' }];
groupedSnapshot.toolCalls.push({ ...groupedSnapshot.toolCalls[0], id: 'tool-2', activitySequence: '4' });
groupedSnapshot.toolCalls[0].assistantStepId = 'step-1'; groupedSnapshot.toolCalls[1].assistantStepId = 'step-1';
const groupedItems = selectCanonicalConversationTurnViewModels(hydrateCanonicalConversationStore(groupedSnapshot))[0].process.items;
assert.equal(groupedItems.filter((item) => item.kind === 'tool_group').length, 1, 'canonical grouping runs once even when a message separates members');
assert.equal(groupedItems.find((item) => item.kind === 'tool_group').toolCalls.length, 2);

const order = []; let listener; let startedStreamId;
const close = subscribeCanonicalConversation({
  sessionId: 'A', after: '7',
  bridge: { StartSessionConversationStreamV2: async (req) => { order.push('start'); startedStreamId = req.streamId; return { streamId: req.streamId, eventName: 'agent-builder:conversation-v2-stream' }; }, StopSessionConversationStreamV2: async () => true },
  loadWailsEvents: async () => ({ Events: { On: (_name, callback) => { order.push('listen'); listener = callback; return () => undefined; } } }),
  onBatch: (batch) => order.push(`batch:${batch.cursor}`), onTransportFailure: () => order.push('recover'),
});
await wait();
assert.deepEqual(order.slice(0, 2), ['listen', 'start'], 'Wails listener is registered before stream start');
listener({ data: { streamId: 'canonical-conversation-A-ignored', ...upsertBatch('A', '7', '8') } });
assert.equal(order.some((item) => item === 'batch:8'), false, 'foreign stream is ignored');
listener({ data: { streamId: startedStreamId, ...upsertBatch('A', '7', '8') } });
assert.equal(order.at(-1), 'batch:8', 'one canonical batch is delivered without flattening');
listener({ data: { streamId: startedStreamId, lifecycle: 'stream_closed' } });
assert.equal(order.at(-1), 'recover', 'silent stream close enters explicit transport recovery');
close();

const shellSource = await readFile(new URL('../src/app/shell/WorkbenchShell.tsx', import.meta.url), 'utf8');
const adapterSource = await readFile(new URL('../src/runtime/wailsWorkbenchAdapter.ts', import.meta.url), 'utf8');
const dockSource = await readFile(new URL('../src/features/conversationDock/ConversationDock.tsx', import.meta.url), 'utf8');
const dockStyles = await readFile(new URL('../src/features/conversationDock/ConversationDock.module.css', import.meta.url), 'utf8');
const traceStyles = await readFile(new URL('../src/features/timeline/TraceRow.module.css', import.meta.url), 'utf8');
const toolStyles = await readFile(new URL('../src/features/tools/ToolCallCard.module.css', import.meta.url), 'utf8');
assert.equal(shellSource.includes('withFresherOutputStore'), false, 'lagging snapshot heuristic is removed');
assert.ok(shellSource.includes('createCanonicalConversationCoordinator'), 'canonical coordinator is the conversation writer');
assert.equal(adapterSource.includes('bridge.SessionOutput?.(activeSessionID, { snapshot: true'), false, 'workbench refresh cannot fetch legacy conversation snapshot');
assert.doesNotMatch(dockSource, /action\.key === 'jump-to-bottom'/, 'dock layout does not special-case a business action key');
assert.match(dockSource, /<ConversationActions actions=\{activeActions\}/, 'all visible dock actions share the centered action rail');
assert.doesNotMatch(dockStyles, /\.floatingActions\s*\{/, 'jump action is not rendered in a separate absolute layer');
assert.doesNotMatch(traceStyles, /max-height:\s*min\(52vh, 420px\)/, 'tool groups do not create a second nested scroll viewport');
assert.match(toolStyles, /max-height:\s*min\(46vh, 360px\)/, 'individual tool details have a bounded viewport');
console.log('canonical conversation convergence smoke passed');
