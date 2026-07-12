import { CANONICAL_CONVERSATION_SCHEMA_VERSION, type CanonicalAgentTask, type CanonicalAssistantStep, type CanonicalConversationEventBatch, type CanonicalConversationSnapshot, type CanonicalConversationScope, type CanonicalEntityType, type CanonicalMessage, type CanonicalNotice, type CanonicalPermission, type CanonicalTodoPlan, type CanonicalToolCall, type CanonicalToolResult, type CanonicalTurn } from './canonicalConversationTypes.ts';

export interface CanonicalStoreRecovery { reason: 'snapshot_required' | 'cursor_gap' | 'revision_conflict' | 'session_mismatch'; entityKey?: string }
export interface CanonicalConversationStore {
  schemaVersion: typeof CANONICAL_CONVERSATION_SCHEMA_VERSION;
  sessionId: string;
  cursor: string;
  scope: CanonicalConversationScope;
  window?: CanonicalConversationSnapshot['window'];
  turnsById: Record<string, CanonicalTurn>;
  messagesById: Record<string, CanonicalMessage>;
  assistantStepsById: Record<string, CanonicalAssistantStep>;
  toolCallsById: Record<string, CanonicalToolCall>;
  toolResultsById: Record<string, CanonicalToolResult>;
  permissionsById: Record<string, CanonicalPermission>;
  todoPlansById: Record<string, CanonicalTodoPlan>;
  agentTasksById: Record<string, CanonicalAgentTask>;
  noticesById: Record<string, CanonicalNotice>;
  tombstoneRevisionByKey: Record<string, string>;
  recovery?: CanonicalStoreRecovery;
}

export function createCanonicalConversationStore(sessionId = ''): CanonicalConversationStore { return { schemaVersion: CANONICAL_CONVERSATION_SCHEMA_VERSION, sessionId, cursor: '0', scope: 'full', turnsById: {}, messagesById: {}, assistantStepsById: {}, toolCallsById: {}, toolResultsById: {}, permissionsById: {}, todoPlansById: {}, agentTasksById: {}, noticesById: {}, tombstoneRevisionByKey: {} } }

export function hydrateCanonicalConversationStore(snapshot: CanonicalConversationSnapshot, previous?: CanonicalConversationStore): CanonicalConversationStore {
  if (snapshot.schemaVersion !== CANONICAL_CONVERSATION_SCHEMA_VERSION) return { ...(previous ?? createCanonicalConversationStore(snapshot.sessionId)), recovery: { reason: 'snapshot_required' } };
  const sameSession = previous?.sessionId === snapshot.sessionId;
  let next = snapshot.scope === 'window' && sameSession ? cloneCanonicalStore(previous) : createCanonicalConversationStore(snapshot.sessionId);
  next.cursor = sameSession ? maxRevision(previous.cursor, snapshot.cursor) : snapshot.cursor; next.scope = snapshot.scope; next.window = snapshot.window; next.recovery = undefined;
  if (snapshot.scope === 'full' && sameSession) preserveNewerThanSnapshot(next, previous, snapshot.cursor);
  for (const entity of snapshot.turns) next = upsertEntity(next, 'turn', entity);
  for (const entity of snapshot.messages) next = upsertEntity(next, 'message', entity);
  for (const entity of snapshot.assistantSteps) next = upsertEntity(next, 'assistantStep', entity);
  for (const entity of snapshot.toolCalls) next = upsertEntity(next, 'toolCall', entity);
  for (const entity of snapshot.toolResults) next = upsertEntity(next, 'toolResult', entity);
  for (const entity of snapshot.permissions) next = upsertEntity(next, 'permission', entity);
  for (const entity of snapshot.todoPlans) next = upsertEntity(next, 'todoPlan', entity);
  for (const entity of snapshot.agentTasks) next = upsertEntity(next, 'agentTask', entity);
  for (const entity of snapshot.notices) next = upsertEntity(next, 'notice', entity);
  return next;
}

export function applyCanonicalConversationBatch(store: CanonicalConversationStore, batch: CanonicalConversationEventBatch): CanonicalConversationStore {
  if (batch.schemaVersion !== CANONICAL_CONVERSATION_SCHEMA_VERSION) return { ...store, recovery: { reason: 'snapshot_required' } };
  if (batch.sessionId !== store.sessionId) return { ...store, recovery: { reason: 'session_mismatch' } };
  if (batch.snapshotRequired) return { ...store, recovery: { reason: 'snapshot_required' } };
  if (batch.cursor === store.cursor && compareDecimal(batch.afterCursor, store.cursor) <= 0) return store;
  if (batch.afterCursor !== store.cursor) return { ...store, recovery: { reason: 'cursor_gap' } };
  let next = cloneCanonicalStore(store); next.recovery = undefined;
  for (const event of batch.events) {
    if (event.sessionId !== store.sessionId) return { ...store, recovery: { reason: 'session_mismatch' } };
    const incoming = event.operation === 'upsert' ? entityFromEvent(event) : undefined; if (event.entityId === '' || (incoming && (incoming.id !== event.entityId || incoming.sessionId !== event.sessionId || incoming.revision !== event.revision || (event.turnId && incoming.turnId !== event.turnId)))) return { ...store, recovery: { reason: 'revision_conflict', entityKey: entityKey(event.entityType, event.entityId) } };
    const key = entityKey(event.entityType, event.entityId); const current = entityFromStore(next, event.entityType, event.entityId); const knownRevision = maxRevision(current?.revision, next.tombstoneRevisionByKey[key]); const relation = compareDecimal(event.revision, knownRevision);
    if (event.operation === 'upsert' && !current && next.tombstoneRevisionByKey[key] === undefined && incoming) { next = upsertEntity(next, event.entityType, incoming); continue; }
    if (relation < 0) continue;
    if (event.operation === 'delete') { if (relation === 0 && current) return { ...store, recovery: { reason: 'revision_conflict', entityKey: key } }; deleteEntity(next, event.entityType, event.entityId); next.tombstoneRevisionByKey[key] = event.revision; continue; }
    if (!incoming) return { ...store, recovery: { reason: 'revision_conflict', entityKey: key } };
    if (relation === 0) { if (current && equivalentEntity(current, incoming)) continue; return { ...store, recovery: { reason: 'revision_conflict', entityKey: key } }; }
    next = upsertEntity(next, event.entityType, incoming); delete next.tombstoneRevisionByKey[key];
  }
  next.cursor = batch.cursor;
  return next;
}

function cloneCanonicalStore(store: CanonicalConversationStore): CanonicalConversationStore { return { ...store, turnsById: { ...store.turnsById }, messagesById: { ...store.messagesById }, assistantStepsById: { ...store.assistantStepsById }, toolCallsById: { ...store.toolCallsById }, toolResultsById: { ...store.toolResultsById }, permissionsById: { ...store.permissionsById }, todoPlansById: { ...store.todoPlansById }, agentTasksById: { ...store.agentTasksById }, noticesById: { ...store.noticesById }, tombstoneRevisionByKey: { ...store.tombstoneRevisionByKey } } }

function preserveNewerThanSnapshot(next: CanonicalConversationStore, previous: CanonicalConversationStore, cursor: string) { for (const kind of entityKinds) { for (const entity of Object.values(entityMap(previous, kind))) if (compareDecimal(entity.revision, cursor) > 0) setEntity(next, kind, entity); } for (const [key, revision] of Object.entries(previous.tombstoneRevisionByKey)) if (compareDecimal(revision, cursor) > 0) next.tombstoneRevisionByKey[key] = revision; }
function upsertEntity(store: CanonicalConversationStore, kind: CanonicalEntityType, entity: CanonicalEntity): CanonicalConversationStore { const current = entityFromStore(store, kind, entity.id); const tombstone = store.tombstoneRevisionByKey[entityKey(kind, entity.id)]; if (!current && tombstone === undefined) { setEntity(store, kind, entity); return store; } const known = maxRevision(current?.revision, tombstone); const relation = compareDecimal(entity.revision, known); if (relation < 0) return store; if (relation === 0) { if (current && equivalentEntity(current, entity)) return store; return { ...store, recovery: { reason: 'revision_conflict', entityKey: entityKey(kind, entity.id) } }; } setEntity(store, kind, entity); delete store.tombstoneRevisionByKey[entityKey(kind, entity.id)]; return store; }

type CanonicalEntity = CanonicalTurn | CanonicalMessage | CanonicalAssistantStep | CanonicalToolCall | CanonicalToolResult | CanonicalPermission | CanonicalTodoPlan | CanonicalAgentTask | CanonicalNotice;
const entityKinds: CanonicalEntityType[] = ['turn', 'message', 'assistantStep', 'toolCall', 'toolResult', 'permission', 'todoPlan', 'agentTask', 'notice'];
function entityMap(store: CanonicalConversationStore, kind: CanonicalEntityType): Record<string, CanonicalEntity> { switch (kind) { case 'turn': return store.turnsById; case 'message': return store.messagesById; case 'assistantStep': return store.assistantStepsById; case 'toolCall': return store.toolCallsById; case 'toolResult': return store.toolResultsById; case 'permission': return store.permissionsById; case 'todoPlan': return store.todoPlansById; case 'agentTask': return store.agentTasksById; case 'notice': return store.noticesById; } }
function entityFromStore(store: CanonicalConversationStore, kind: CanonicalEntityType, id: string): CanonicalEntity | undefined { return entityMap(store, kind)[id]; }
function setEntity(store: CanonicalConversationStore, kind: CanonicalEntityType, entity: CanonicalEntity) { entityMap(store, kind)[entity.id] = entity; }
function deleteEntity(store: CanonicalConversationStore, kind: CanonicalEntityType, id: string) { delete entityMap(store, kind)[id]; }
function entityKey(kind: CanonicalEntityType, id: string) { return `${kind}:${id}`; }
function entityFromEvent(event: CanonicalConversationEventBatch['events'][number]): CanonicalEntity | undefined { switch (event.entityType) { case 'turn': return event.turn; case 'message': return event.message; case 'assistantStep': return event.assistantStep; case 'toolCall': return event.toolCall; case 'toolResult': return event.toolResult; case 'permission': return event.permission; case 'todoPlan': return event.todoPlan; case 'agentTask': return event.agentTask; case 'notice': return event.notice; } }
function maxRevision(left?: string, right?: string) { return compareDecimal(left ?? '0', right ?? '0') >= 0 ? left ?? '0' : right ?? '0'; }
export function compareDecimal(left: string, right: string) { const a = BigInt(left || '0'); const b = BigInt(right || '0'); return a < b ? -1 : a > b ? 1 : 0; }
function equivalentEntity(left: CanonicalEntity, right: CanonicalEntity) { return JSON.stringify(left) === JSON.stringify(right); }
