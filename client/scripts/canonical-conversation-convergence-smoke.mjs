import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createCanonicalConversationCoordinator } from '../src/runtime/canonicalConversationCoordinator.ts';
import { subscribeCanonicalConversation } from '../src/runtime/canonicalConversationStream.ts';
import { applyCanonicalConversationDeltas, hydrateCanonicalConversationStore } from '../src/runtime/canonicalConversationStore.ts';
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
handlers.get('A').onBatch({ schemaVersion: 2, sessionId: 'A', afterCursor: '90071992547409931', cursor: '90071992547409931', events: [], deltas: [{ messageId: 'live-A', turnId: 'turn-1', partType: 'text', delta: 'live', contentLength: 4, createdAt: 2 }] });
assert.equal(coordinator.cached('A').streamingByMessageId['live-A'].text, 'live', 'active Session retains its live overlay');
const oldAHandler = handlers.get('A');
coordinator.activate('B'); await wait(); await wait();
assert.deepEqual(coordinator.cached('A').streamingByMessageId, {}, 'leaving a Session releases its ephemeral token buffers');
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

let cachedAFetches = 0; let cachedASubscriptions = 0;
const cachedReconnectCoordinator = createCanonicalConversationCoordinator({
  fetchSnapshot: async (sessionId) => { if (sessionId === 'CA') cachedAFetches += 1; return snapshot(sessionId, '1'); },
  subscribe: async (sessionId) => { if (sessionId === 'CA' && ++cachedASubscriptions === 2) throw new Error('reconnect failed'); return () => undefined; },
  onStore: () => undefined,
  retryDelayMs: () => 0,
});
cachedReconnectCoordinator.activate('CA'); await wait(); await wait();
cachedReconnectCoordinator.activate('CB'); await wait(); await wait();
cachedReconnectCoordinator.activate('CA'); await wait(); await wait(); await wait();
assert.equal(cachedAFetches, 2, 'cached Session reconnect failure performs snapshot recovery');
cachedReconnectCoordinator.stop();

const groupedSnapshot = snapshot('G', '10');
groupedSnapshot.messages = [{ id: 'middle', sessionId: 'G', turnId: 'turn-1', activitySequence: '3', revision: '1', createdAt: 1, updatedAt: 1, role: 'assistant', phase: 'intermediate', status: 'completed', content: 'between' }];
groupedSnapshot.toolCalls.push({ ...groupedSnapshot.toolCalls[0], id: 'tool-2', activitySequence: '4' });
groupedSnapshot.toolCalls[0].assistantStepId = 'step-1'; groupedSnapshot.toolCalls[1].assistantStepId = 'step-1';
const groupedItems = selectCanonicalConversationTurnViewModels(hydrateCanonicalConversationStore(groupedSnapshot))[0].process.items;
assert.equal(groupedItems.filter((item) => item.kind === 'tool_group').length, 1, 'canonical grouping runs once even when a message separates members');
assert.equal(groupedItems.find((item) => item.kind === 'tool_group').toolCalls.length, 2);

const firstSubmitSnapshot = snapshot('FIRST', '2');
firstSubmitSnapshot.turns[0].startedAt = 1000;
const adoptedFirstSubmit = selectCanonicalConversationTurnViewModels(
  hydrateCanonicalConversationStore(firstSubmitSnapshot),
  undefined,
  { 'prompt-first': { clientRequestId: 'prompt-first', sessionId: 'FIRST', prompt: 'hello', createdAt: 1000, status: 'submitting' } },
);
assert.equal(adoptedFirstSubmit.length, 1, 'first draft submit adopts the canonical Turn instead of rendering two processing rows');
assert.equal(adoptedFirstSubmit[0].id, 'turn-1', 'the canonical Turn remains the single lifecycle owner');
assert.equal(adoptedFirstSubmit[0].user?.content, 'hello', 'optimistic user content remains visible until the canonical user-message link arrives');

const liveSnapshot = snapshot('LIVE', '2');
liveSnapshot.messages = [{ id: 'message-live', sessionId: 'LIVE', turnId: 'turn-1', activitySequence: '2', revision: '2', createdAt: 1000, updatedAt: 1000, role: 'assistant', phase: 'intermediate', status: 'streaming', content: '', contentLength: 0, reasoningContent: '', reasoningContentLength: 0 }];
let liveStore = hydrateCanonicalConversationStore(liveSnapshot);
liveStore = applyCanonicalConversationDeltas(liveStore, [
  { messageId: 'message-live', turnId: 'turn-1', partType: 'reasoning', delta: '思', contentLength: Buffer.byteLength('思'), createdAt: 1010 },
  { messageId: 'message-live', turnId: 'turn-1', partType: 'reasoning', delta: '考', contentLength: Buffer.byteLength('思考'), createdAt: 1020 },
]);
const afterDuplicate = applyCanonicalConversationDeltas(liveStore, [{ messageId: 'message-live', turnId: 'turn-1', partType: 'reasoning', delta: '考', contentLength: Buffer.byteLength('思考'), createdAt: 1030 }]);
assert.equal(afterDuplicate, liveStore, 'duplicate deltas are ignored idempotently');
const afterGap = applyCanonicalConversationDeltas(liveStore, [{ messageId: 'message-live', turnId: 'turn-1', partType: 'reasoning', delta: '失', contentLength: Buffer.byteLength('思考缺失'), createdAt: 1040 }]);
assert.equal(afterGap, liveStore, 'a missing delta prefix is not fabricated');
const afterOversize = applyCanonicalConversationDeltas(liveStore, [{ messageId: 'message-live', turnId: 'turn-1', partType: 'text', delta: 'x', contentLength: 64 * 1024 + 1, createdAt: 1050 }]);
assert.equal(afterOversize, liveStore, 'live overlays remain bounded to the canonical message window');
const liveTurns = selectCanonicalConversationTurnViewModels(liveStore);
assert.match(liveTurns[0].process.items.find((item) => item.messageId === 'message-live')?.content ?? '', /思考/, 'reasoning deltas render before the durable message update');
const durableLiveSnapshot = structuredClone(liveSnapshot);
durableLiveSnapshot.cursor = '3';
durableLiveSnapshot.messages[0].revision = '3';
durableLiveSnapshot.messages[0].updatedAt = 1050;
durableLiveSnapshot.messages[0].reasoningContent = '思考';
durableLiveSnapshot.messages[0].reasoningContentLength = Buffer.byteLength('思考');
liveStore = hydrateCanonicalConversationStore(durableLiveSnapshot, liveStore);
assert.equal(liveStore.streamingByMessageId['message-live'], undefined, 'durable canonical content retires the matching live overlay');

const earlyDeltaSnapshot = snapshot('EARLY', '1');
earlyDeltaSnapshot.messages = [];
earlyDeltaSnapshot.toolCalls = [];
let earlyDeltaStore = hydrateCanonicalConversationStore(earlyDeltaSnapshot);
earlyDeltaStore = applyCanonicalConversationDeltas(earlyDeltaStore, [{ messageId: 'message-early', turnId: 'turn-1', partType: 'text', delta: 'first token', contentLength: Buffer.byteLength('first token'), createdAt: 1010 }]);
const earlyDeltaTurn = selectCanonicalConversationTurnViewModels(earlyDeltaStore)[0];
assert.equal(earlyDeltaTurn.process.items.find((item) => item.messageId === 'message-early')?.content, 'first token', 'a live delta renders even when it beats canonical message creation');

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
const timelineSource = await readFile(new URL('../src/features/timeline/Timeline.tsx', import.meta.url), 'utf8');
const permissionSource = await readFile(new URL('../src/features/permissions/PermissionGate.tsx', import.meta.url), 'utf8');
const workspaceSource = await readFile(new URL('../src/features/workspace/Workspace.tsx', import.meta.url), 'utf8');
const coordinatorSource = await readFile(new URL('../src/runtime/canonicalConversationCoordinator.ts', import.meta.url), 'utf8');
const toolCardSource = await readFile(new URL('../src/features/tools/ToolCallCard.tsx', import.meta.url), 'utf8');
const dockStyles = await readFile(new URL('../src/features/conversationDock/ConversationDock.module.css', import.meta.url), 'utf8');
const traceStyles = await readFile(new URL('../src/features/timeline/TraceRow.module.css', import.meta.url), 'utf8');
const toolStyles = await readFile(new URL('../src/features/tools/ToolCallCard.module.css', import.meta.url), 'utf8');
assert.equal(shellSource.includes('withFresherOutputStore'), false, 'lagging snapshot heuristic is removed');
assert.ok(shellSource.includes('createCanonicalConversationCoordinator'), 'canonical coordinator is the conversation writer');
assert.match(shellSource, /const commitConversationAction[\s\S]*?preserveCanonicalConversation\(nextViewModel, current\)/, 'conversation action hydration cannot overwrite a newer canonical stream store');
assert.match(shellSource, /const decidePermission[\s\S]*?commitConversationAction\(nextViewModel\)/, 'permission decisions use the canonical-safe action commit');
assert.equal(adapterSource.includes('bridge.SessionOutput?.(activeSessionID, { snapshot: true'), false, 'workbench refresh cannot fetch legacy conversation snapshot');
assert.doesNotMatch(adapterSource, /async submitUserInput[\s\S]*?catch \(error\)[\s\S]*?conversationTarget: target/, 'submit errors propagate to the Shell optimistic error boundary');
assert.match(timelineSource, /shouldRenderProcess\(block\)/, 'canonical Turn lifecycle rendering is not gated on the first process entity');
assert.match(timelineSource, /isFailedProcessStatus\(block\.status\)/, 'a failed canonical Turn remains visible even when it has no process entities');
assert.match(workspaceSource, /<PermissionGate key=\{activePendingPermission\.id\}/, 'each permission request gets isolated local form state');
assert.match(permissionSource, /decisionInFlight\.current/, 'permission decisions have a synchronous duplicate-submit guard');
assert.match(coordinatorSource, /connect\(sessionId, gen, cached\)\.catch/, 'cached Session stream reconnect failures enter snapshot recovery');
assert.match(adapterSource, /ReadObjectContent\(refID\)/, 'Runtime Object content is read through the Wails binding');
assert.match(toolCardSource, /onObjectContentLoad\(outputRef\)/, 'tool details load full output from its canonical Object ref');
assert.doesNotMatch(dockSource, /action\.key === 'jump-to-bottom'/, 'dock layout does not special-case a business action key');
assert.match(dockSource, /<ConversationActions actions=\{activeActions\}/, 'all visible dock actions share the centered action rail');
assert.doesNotMatch(dockStyles, /\.floatingActions\s*\{/, 'jump action is not rendered in a separate absolute layer');
assert.doesNotMatch(traceStyles, /max-height:\s*min\(52vh, 420px\)/, 'tool groups do not create a second nested scroll viewport');
assert.match(toolStyles, /max-height:\s*min\(46vh, 360px\)/, 'individual tool details have a bounded viewport');
console.log('canonical conversation convergence smoke passed');
