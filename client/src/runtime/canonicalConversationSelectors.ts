import { canonicalEntityKeysForTurn, type CanonicalConversationStore } from './canonicalConversationStore.ts';
import type { CanonicalAgentTask, CanonicalAssistantStep, CanonicalMessage, CanonicalNotice, CanonicalPermission, CanonicalTodoPlan, CanonicalToolCall, CanonicalToolResult, CanonicalTurn } from './canonicalConversationTypes.ts';

export type CanonicalProcessEntity = CanonicalMessage | CanonicalAssistantStep | CanonicalToolCall | CanonicalToolResult | CanonicalPermission | CanonicalAgentTask | CanonicalNotice;

export interface CanonicalTurnProjection {
  turn: CanonicalTurn;
  user?: CanonicalMessage;
  final?: CanonicalMessage;
  process: CanonicalProcessEntity[];
  todoPlan?: CanonicalTodoPlan;
}

export function selectCanonicalConversationTurns(store?: CanonicalConversationStore): CanonicalTurnProjection[] {
  if (!store?.sessionId) return [];
  return Object.values(store.turnsById)
    .filter((turn) => turn.sessionId === store.sessionId)
    .sort(compareCanonicalEntities)
    .map((turn) => selectCanonicalTurn(store, turn));
}

// An optimistic submit is settled only after a canonical Turn owns its echoed
// user message. Message and Turn entities can arrive in separate atomic
// batches; treating a bare Message echo as rendered creates a visible gap.
export function selectOwnedClientRequestIds(store?: CanonicalConversationStore): Set<string> {
  const owned = new Set<string>();
  if (!store?.sessionId) return owned;
  for (const turn of Object.values(store.turnsById)) {
    const message = ownedMessage(store, turn, turn.userMessageId);
    if (message?.clientRequestId) owned.add(message.clientRequestId);
  }
  return owned;
}

export function selectCanonicalTurn(store: CanonicalConversationStore, turn: CanonicalTurn): CanonicalTurnProjection {
  const user = ownedMessage(store, turn, turn.userMessageId);
  const finalCandidate = ownedMessage(store, turn, turn.finalMessageId);
  const final = finalCandidate?.phase === 'final' ? finalCandidate : undefined;
  const excluded = new Set([user?.id, final?.id].filter((id): id is string => Boolean(id)));
  const process: CanonicalProcessEntity[] = [];
  for (const key of canonicalEntityKeysForTurn(store, turn.id)) {
    const separator = key.indexOf(':');
    const kind = key.slice(0, separator);
    const id = key.slice(separator + 1);
    const entity = processEntityByKind(store, kind, id);
    if (entity && entity.sessionId === store.sessionId && !excluded.has(entity.id)) process.push(entity);
  }
  process.sort(compareCanonicalEntities);
  return { turn, user, final, process, todoPlan: selectTodoPlanForTurn(store, turn.id) };
}

export function selectTodoPlanForTurn(store: CanonicalConversationStore, turnId: string): CanonicalTodoPlan | undefined {
  return canonicalEntityKeysForTurn(store, turnId)
    .filter((key) => key.startsWith('todoPlan:'))
    .map((key) => store.todoPlansById[key.slice('todoPlan:'.length)])
    .filter((plan): plan is CanonicalTodoPlan => Boolean(plan) && plan.sessionId === store.sessionId && plan.ownerTurnId === turnId)
    .sort(compareCanonicalEntities)
    .at(-1);
}

function processEntityByKind(store: CanonicalConversationStore, kind: string, id: string): CanonicalProcessEntity | undefined {
  switch (kind) {
    case 'message': return store.messagesById[id];
    case 'assistantStep': return store.assistantStepsById[id];
    case 'toolCall': return store.toolCallsById[id];
    case 'toolResult': return store.toolResultsById[id];
    case 'permission': return store.permissionsById[id];
    case 'agentTask': return store.agentTasksById[id];
    case 'notice': return store.noticesById[id];
    default: return undefined;
  }
}

function ownedMessage(store: CanonicalConversationStore, turn: CanonicalTurn, id?: string) {
  if (!id) return undefined;
  const message = store.messagesById[id];
  return message?.sessionId === store.sessionId && message.turnId === turn.id ? message : undefined;
}

export function compareCanonicalEntities(left: { activitySequence: string; id: string }, right: { activitySequence: string; id: string }) {
  const sequence = compareBigInt(left.activitySequence, right.activitySequence);
  return sequence || left.id.localeCompare(right.id);
}

function compareBigInt(left: string, right: string) {
  const a = BigInt(left || '0');
  const b = BigInt(right || '0');
  return a < b ? -1 : a > b ? 1 : 0;
}
