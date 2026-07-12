import type { CanonicalConversationStore } from './canonicalConversationStore.ts';
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

export function selectCanonicalTurn(store: CanonicalConversationStore, turn: CanonicalTurn): CanonicalTurnProjection {
  const user = ownedMessage(store, turn, turn.userMessageId);
  const finalCandidate = ownedMessage(store, turn, turn.finalMessageId);
  const final = finalCandidate?.phase === 'final' ? finalCandidate : undefined;
  const excluded = new Set([user?.id, final?.id].filter((id): id is string => Boolean(id)));
  const process: CanonicalProcessEntity[] = [
    ...Object.values(store.messagesById),
    ...Object.values(store.assistantStepsById),
    ...Object.values(store.toolCallsById),
    ...Object.values(store.toolResultsById),
    ...Object.values(store.permissionsById),
    ...Object.values(store.agentTasksById),
    ...Object.values(store.noticesById),
  ].filter((entity) => entity.sessionId === store.sessionId && entity.turnId === turn.id && !excluded.has(entity.id));
  process.sort(compareCanonicalEntities);
  return { turn, user, final, process, todoPlan: selectTodoPlanForTurn(store, turn.id) };
}

export function selectTodoPlanForTurn(store: CanonicalConversationStore, turnId: string): CanonicalTodoPlan | undefined {
  return Object.values(store.todoPlansById)
    .filter((plan) => plan.sessionId === store.sessionId && plan.ownerTurnId === turnId)
    .sort(compareCanonicalEntities)
    .at(-1);
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
