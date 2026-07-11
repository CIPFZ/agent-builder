import type { OutputStore, RuntimeConversationItem } from '../outputTypes.ts';
import { projectRuntimeConversationItem } from '../outputSelectors.ts';
import type { ConversationTurnViewModel } from './conversationTypes.ts';
import { normalizeTurnStatus } from './statusMachine.ts';

export function selectConversationTurns(store?: OutputStore): ConversationTurnViewModel[] {
  if (!store?.sessionId) return [];

  const runtimeItems = Object.values(store.itemsById);
  const itemsByTurn = new Map<string, RuntimeConversationItem[]>();
  for (const item of runtimeItems) {
    if (!item.turnId) continue;
    itemsByTurn.set(item.turnId, [...(itemsByTurn.get(item.turnId) ?? []), item]);
  }

  const turns = Object.values(store.turnsById)
    .filter((turn) => turn.sessionId === store.sessionId)
    .sort((left, right) => (left.startedAt ?? 0) - (right.startedAt ?? 0));

  const projected = turns.map((turn): ConversationTurnViewModel => {
    const turnItems = (itemsByTurn.get(turn.id) ?? []).sort(compareRuntimeItems);
    const status = normalizeTurnStatus(turn.status);
    const userItem = findMessageItem(turnItems, turn.userMessageId, 'user_message');
    const finalItem = findFinalItem(turnItems, turn.latestAssistantMessageId);
    const exploration = turnItems.find((item) => item.kind === 'exploration_summary');
    const excluded = new Set([userItem?.id, finalItem?.id, exploration?.id].filter(Boolean));
    const processItems = turnItems
      .filter((item) => !excluded.has(item.id))
      .map((item) => normalizeTerminalProcessItem(projectRuntimeConversationItem(item, store), status));
    return {
      id: turn.id,
      sessionId: turn.sessionId,
      status,
      user: userItem ? projectRuntimeConversationItem(userItem, store) : undefined,
      final: finalItem ? projectRuntimeConversationItem(finalItem, store) : undefined,
      process: {
        status,
        items: processItems,
        exploration: exploration?.exploration,
        startedAt: turn.startedAt,
        finishedAt: turn.finishedAt,
        hasFailure: status === 'failed' || processItems.some((item) => isFailure(item.status)),
      },
      startedAt: turn.startedAt,
      finishedAt: turn.finishedAt,
      error: turn.error,
    };
  });

  appendOptimisticTurns(projected, store);
  return projected.sort((left, right) => (left.startedAt ?? 0) - (right.startedAt ?? 0));
}

function findMessageItem(items: RuntimeConversationItem[], messageId: string | undefined, kind: string) {
  return items.find((item) => messageId && item.messageId === messageId) ?? items.find((item) => item.kind === kind);
}

function findFinalItem(items: RuntimeConversationItem[], messageId?: string) {
  return items.find((item) => item.kind === 'assistant_message' && item.phase === 'final' && (!messageId || item.messageId === messageId));
}

function appendOptimisticTurns(turns: ConversationTurnViewModel[], store: OutputStore) {
  for (const submit of Object.values(store.optimisticByClientRequestId)) {
    turns.push({
      id: `optimistic:${submit.clientRequestId}`,
      sessionId: store.sessionId,
      status: 'queued',
      startedAt: submit.createdAt,
      user: {
        id: `optimistic-message:${submit.clientRequestId}`,
        kind: 'user_message',
        sessionId: store.sessionId,
        role: 'user',
        content: submit.prompt,
        status: submit.status === 'error' ? 'error' : 'loading',
        createdAt: submit.createdAt,
        clientRequestId: submit.clientRequestId,
        error: submit.error,
      },
      process: { status: 'queued', items: [], hasFailure: submit.status === 'error' },
      error: submit.error,
    });
  }
}

function compareRuntimeItems(left: RuntimeConversationItem, right: RuntimeConversationItem) {
  return left.sequence - right.sequence || left.id.localeCompare(right.id);
}

function isFailure(status?: string) {
  return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted';
}

function normalizeTerminalProcessItem<T extends ReturnType<typeof projectRuntimeConversationItem>>(item: T, turnStatus: ConversationTurnViewModel['status']): T {
  if (!['completed', 'failed', 'cancelled', 'interrupted'].includes(turnStatus)) return item;
  const active = item.status === 'running' || item.status === 'queued' || item.status === 'waiting_permission' || item.status === 'streaming';
  const activeTool = item.toolCall && ['running', 'queued', 'waiting_permission'].includes(item.toolCall.status);
  if (!active && !activeTool) return item;
  return {
    ...item,
    status: active ? turnStatus : item.status,
    toolCall: activeTool ? { ...item.toolCall, status: turnStatus } : item.toolCall,
    streaming: active ? undefined : item.streaming,
  };
}
