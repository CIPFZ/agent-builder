import { createOutputStore } from './outputStore.ts';
import type { OptimisticUserSubmit, OutputStore, RuntimeOutputEvent, RuntimeOutputSnapshot, RuntimeStreamingState } from './outputTypes.ts';
import { mergeMonotonicTurnStatus } from './conversation/statusMachine.ts';

export function hydrateOutputStore(snapshot: RuntimeOutputSnapshot | undefined, previous?: OutputStore): OutputStore {
  if (!snapshot) {
    return previous ?? createOutputStore();
  }
  const store = createOutputStore(snapshot.sessionId);
  store.cursor = snapshot.cursor;
  store.version = snapshot.version;
  store.todos = snapshot.todos;
  store.optimisticByClientRequestId = { ...(previous?.sessionId === snapshot.sessionId ? previous.optimisticByClientRequestId : {}) };
  // Fresh snapshots supersede any live streaming buffers; the snapshot's
  // messages/items already carry the accumulated content.
  store.streamingByMessageId = {};
  for (const item of snapshot.items ?? []) {
    store.itemsById[item.id] = item;
    if (item.clientRequestId) {
      delete store.optimisticByClientRequestId[item.clientRequestId];
    }
  }
  for (const message of snapshot.messages ?? []) {
    store.messagesById[message.id] = message;
    if (message.clientRequestId) {
      delete store.optimisticByClientRequestId[message.clientRequestId];
    }
  }
  for (const turn of snapshot.turns ?? []) {
    store.turnsById[turn.id] = turn;
  }
  for (const step of snapshot.assistantSteps ?? []) {
    store.assistantStepsById[step.id] = step;
  }
  for (const call of snapshot.toolCalls ?? []) {
    store.toolCallsById[call.id] = call;
  }
  for (const result of snapshot.toolResults ?? []) {
    store.toolResultsById[result.id] = result;
  }
  for (const permission of snapshot.permissions ?? []) {
    store.permissionsById[permission.id] = permission;
  }
  for (const task of snapshot.agentTasks ?? []) {
    store.agentTasksById[task.id] = task;
  }
  preserveUnconfirmedLiveTurnOutput(store, previous);
  return store;
}

// A persisted snapshot can briefly lag behind the Wails event stream. Keep
// live entities that belong to an active turn until a later snapshot confirms
// them, otherwise running tools visibly flash and disappear every time the
// workbench performs its low-frequency background hydration.
function preserveUnconfirmedLiveTurnOutput(store: OutputStore, previous?: OutputStore) {
  if (!previous || previous.sessionId !== store.sessionId) return;
  const activeTurnIds = new Set(
    [...Object.values(previous.turnsById), ...Object.values(store.turnsById)]
      .filter((turn) => !isTerminalTurnStatus(turn.status))
      .map((turn) => turn.id),
  );
  if (activeTurnIds.size === 0) return;

  for (const [id, turn] of Object.entries(previous.turnsById)) {
    if (activeTurnIds.has(id) && !store.turnsById[id]) store.turnsById[id] = turn;
  }
  for (const [id, item] of Object.entries(previous.itemsById)) {
    if (item.turnId && activeTurnIds.has(item.turnId) && !store.itemsById[id]) store.itemsById[id] = item;
  }
  for (const [id, call] of Object.entries(previous.toolCallsById)) {
    if (activeTurnIds.has(call.turnId) && !store.toolCallsById[id]) store.toolCallsById[id] = call;
  }
  for (const [id, result] of Object.entries(previous.toolResultsById)) {
    if (activeTurnIds.has(result.turnId) && !store.toolResultsById[id]) store.toolResultsById[id] = result;
  }
  for (const [id, step] of Object.entries(previous.assistantStepsById)) {
    if (activeTurnIds.has(step.turnId) && !store.assistantStepsById[id]) store.assistantStepsById[id] = step;
  }
  store.streamingByMessageId = { ...previous.streamingByMessageId, ...store.streamingByMessageId };
}

function isTerminalTurnStatus(status?: string) {
  return status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'interrupted';
}

export function applyOutputEvent(store: OutputStore, event: RuntimeOutputEvent): OutputStore {
  if (!event || (store.sessionId && event.sessionId !== store.sessionId)) {
    return store;
  }
  // Delta events are ephemeral and idempotent; the id may be reused across
  // ticks or omitted entirely. Handle them before the dedup check.
  if (event.textDelta) {
    return applyTextDeltaEvent(store, event);
  }
  const sequenceKey = eventEntitySequenceKey(event);
  if (sequenceKey && event.sequence > 0 && event.sequence <= (store.entitySequenceByKey[sequenceKey] ?? 0)) {
    return store;
  }
  if (!event.id || store.appliedEventIds[event.id]) {
    return store;
  }
  const next: OutputStore = {
    ...store,
    cursor: event.sequence ? String(Math.floor(event.sequence / 100)) : store.cursor,
    lastSequence: Math.max(store.lastSequence ?? 0, event.sequence ?? 0),
    itemsById: { ...store.itemsById },
    messagesById: { ...store.messagesById },
    turnsById: { ...store.turnsById },
    assistantStepsById: { ...store.assistantStepsById },
    toolCallsById: { ...store.toolCallsById },
    toolResultsById: { ...store.toolResultsById },
    permissionsById: { ...store.permissionsById },
    agentTasksById: { ...store.agentTasksById },
    optimisticByClientRequestId: { ...store.optimisticByClientRequestId },
    appliedEventIds: { ...store.appliedEventIds, [event.id]: true },
    entitySequenceByKey: {
      ...store.entitySequenceByKey,
      ...(sequenceKey && event.sequence > 0 ? { [sequenceKey]: event.sequence } : {}),
    },
    streamingByMessageId: { ...store.streamingByMessageId },
  };
  if (event.operation === 'delete') {
    delete next.itemsById[event.entityId];
    delete next.messagesById[event.entityId];
    delete next.turnsById[event.entityId];
    delete next.assistantStepsById[event.entityId];
    delete next.toolCallsById[event.entityId];
    delete next.toolResultsById[event.entityId];
    delete next.permissionsById[event.entityId];
    delete next.agentTasksById[event.entityId];
    delete next.streamingByMessageId[event.entityId];
    return next;
  }
  if (event.item) {
    const previous = next.itemsById[event.item.id];
    next.itemsById[event.item.id] = {
      ...previous,
      ...event.item,
      status: event.item.status ? mergeTerminalEntityStatus(previous?.status, event.item.status) : previous?.status,
    };
    if (event.item.clientRequestId) {
      delete next.optimisticByClientRequestId[event.item.clientRequestId];
    } else if (event.item.role === 'user') {
      for (const message of Object.values(next.messagesById)) {
        if (message.id === event.item.messageId && message.clientRequestId) {
          delete next.optimisticByClientRequestId[message.clientRequestId];
        }
      }
    }
    // Any full assistant message/thinking payload supersedes the live
    // streaming buffer for that message; drop it so we don't double-append.
    if (event.item.messageId && (event.item.kind === 'assistant_message' || event.item.kind === 'assistant_thinking')) {
      const messageId = event.item.messageId;
      const isTerminal = event.item.status && event.item.status !== 'streaming';
      if (isTerminal) {
        delete next.streamingByMessageId[messageId];
      }
    }
  }
  if (event.message) {
    const previous = next.messagesById[event.message.id];
    next.messagesById[event.message.id] = {
      ...previous,
      ...event.message,
      finished: previous?.finished || event.message.finished,
    };
    if (event.message.clientRequestId) {
      delete next.optimisticByClientRequestId[event.message.clientRequestId];
    }
    if (event.message.finished) {
      delete next.streamingByMessageId[event.message.id];
    }
  }
  if (event.turn) {
    const previous = next.turnsById[event.turn.id];
    next.turnsById[event.turn.id] = {
      ...previous,
      ...event.turn,
      status: mergeMonotonicTurnStatus(previous?.status, event.turn.status) ?? event.turn.status,
    };
    if (['completed', 'failed', 'cancelled', 'interrupted'].includes(next.turnsById[event.turn.id].status)) {
      for (const item of Object.values(next.itemsById)) {
        if (item.turnId === event.turn.id && item.messageId) delete next.streamingByMessageId[item.messageId];
      }
    }
  }
  if (event.assistantStep) {
    const previous = next.assistantStepsById[event.assistantStep.id];
    next.assistantStepsById[event.assistantStep.id] = {
      ...previous,
      ...event.assistantStep,
      status: mergeTerminalEntityStatus(previous?.status, event.assistantStep.status),
    };
  }
  if (event.toolCall) {
    const previous = next.toolCallsById[event.toolCall.id];
    next.toolCallsById[event.toolCall.id] = {
      ...previous,
      ...event.toolCall,
      status: mergeTerminalEntityStatus(previous?.status, event.toolCall.status),
    };
  }
  if (event.toolResult) {
    next.toolResultsById[event.toolResult.id] = { ...next.toolResultsById[event.toolResult.id], ...event.toolResult };
  }
  if (event.permission) {
    const previous = next.permissionsById[event.permission.id];
    next.permissionsById[event.permission.id] = {
      ...previous,
      ...event.permission,
      status: mergeTerminalEntityStatus(previous?.status, event.permission.status),
    };
  }
  if (event.agentTask) {
    next.agentTasksById[event.agentTask.id] = { ...next.agentTasksById[event.agentTask.id], ...event.agentTask };
  }
  if (event.todos) {
    next.todos = event.todos;
  }
  return next;
}

function eventEntitySequenceKey(event: RuntimeOutputEvent) {
  if (event.item) return `item:${event.item.id}`;
  if (event.message) return `message:${event.message.id}`;
  if (event.turn) return `turn:${event.turn.id}`;
  if (event.assistantStep) return `assistant-step:${event.assistantStep.id}`;
  if (event.toolCall) return `tool-call:${event.toolCall.id}`;
  if (event.toolResult) return `tool-result:${event.toolResult.id}`;
  if (event.permission) return `permission:${event.permission.id}`;
  if (event.agentTask) return `agent-task:${event.agentTask.id}`;
  if (event.kind.includes('conversation_item')) return `item:${event.entityId}`;
  if (event.kind.includes('message')) return `message:${event.entityId}`;
  if (event.kind.includes('turn')) return `turn:${event.entityId}`;
  if (event.kind.includes('assistant_step')) return `assistant-step:${event.entityId}`;
  if (event.kind.includes('tool_call')) return `tool-call:${event.entityId}`;
  if (event.kind.includes('tool_result')) return `tool-result:${event.entityId}`;
  if (event.kind.includes('permission')) return `permission:${event.entityId}`;
  if (event.kind.includes('agent_task')) return `agent-task:${event.entityId}`;
  return event.entityId ? `entity:${event.entityId}` : '';
}

function applyTextDeltaEvent(store: OutputStore, event: RuntimeOutputEvent): OutputStore {
  const delta = event.textDelta;
  if (!delta || !delta.messageId) {
    return store;
  }
  const turnId = delta.turnId || event.turnId || Object.values(store.itemsById).find((item) => item.messageId === delta.messageId)?.turnId;
  if (turnId && isTerminalStatus(store.turnsById[turnId]?.status)) {
    return store;
  }
  const partType = delta.partType === 'reasoning' ? 'reasoning' : 'text';
  const previous = store.streamingByMessageId[delta.messageId] ?? emptyStreamingState();
  const knownLen = partType === 'reasoning' ? previous.thinkingLen : previous.textLen;
  // contentLen is the total length after this delta is applied. Any tick
  // that reports a shorter (or equal) total than what we already applied
  // is a duplicate or out-of-order fragment; drop it.
  if (typeof delta.contentLen === 'number' && delta.contentLen <= knownLen) {
    return store;
  }
  const nextState: RuntimeStreamingState = { ...previous };
  if (partType === 'reasoning') {
    nextState.thinking = (previous.thinking ?? '') + (delta.delta ?? '');
    nextState.thinkingLen = typeof delta.contentLen === 'number' ? delta.contentLen : nextState.thinking.length;
  } else {
    nextState.text = (previous.text ?? '') + (delta.delta ?? '');
    nextState.textLen = typeof delta.contentLen === 'number' ? delta.contentLen : nextState.text.length;
  }
  return {
    ...store,
    streamingByMessageId: {
      ...store.streamingByMessageId,
      [delta.messageId]: nextState,
    },
  };
}

function mergeTerminalEntityStatus(previous: string | undefined, incoming: string) {
  return isTerminalStatus(previous) && !isTerminalStatus(incoming) ? previous! : incoming;
}

function isTerminalStatus(status: string | undefined) {
  return ['completed', 'success', 'failed', 'denied', 'cancelled', 'interrupted', 'expired'].includes(status ?? '');
}

function emptyStreamingState(): RuntimeStreamingState {
  return { text: '', thinking: '', textLen: 0, thinkingLen: 0 };
}

export function applyOutputEvents(store: OutputStore, events: RuntimeOutputEvent[] | undefined): OutputStore {
  return (events ?? []).reduce(applyOutputEvent, store);
}

export function addOptimisticUserSubmit(store: OutputStore, submit: OptimisticUserSubmit): OutputStore {
  return {
    ...store,
    optimisticByClientRequestId: {
      ...store.optimisticByClientRequestId,
      [submit.clientRequestId]: submit,
    },
  };
}
