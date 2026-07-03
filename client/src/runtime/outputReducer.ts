import { createOutputStore } from './outputStore.ts';
import type { OptimisticUserSubmit, OutputStore, RuntimeOutputEvent, RuntimeOutputSnapshot, RuntimeStreamingState } from './outputTypes.ts';

export function hydrateOutputStore(snapshot: RuntimeOutputSnapshot | undefined, previous?: OutputStore): OutputStore {
  if (!snapshot) {
    return previous ?? createOutputStore();
  }
  const store = createOutputStore(snapshot.sessionId);
  store.cursor = snapshot.cursor;
  store.version = snapshot.version;
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
  return store;
}

export function applyOutputEvent(store: OutputStore, event: RuntimeOutputEvent): OutputStore {
  if (!event) {
    return store;
  }
  // Delta events are ephemeral and idempotent; the id may be reused across
  // ticks or omitted entirely. Handle them before the dedup check.
  if (event.textDelta) {
    return applyTextDeltaEvent(store, event);
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
    next.itemsById[event.item.id] = { ...next.itemsById[event.item.id], ...event.item };
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
    next.messagesById[event.message.id] = { ...next.messagesById[event.message.id], ...event.message };
    if (event.message.clientRequestId) {
      delete next.optimisticByClientRequestId[event.message.clientRequestId];
    }
    if (event.message.finished) {
      delete next.streamingByMessageId[event.message.id];
    }
  }
  if (event.turn) {
    next.turnsById[event.turn.id] = { ...next.turnsById[event.turn.id], ...event.turn };
  }
  if (event.assistantStep) {
    next.assistantStepsById[event.assistantStep.id] = { ...next.assistantStepsById[event.assistantStep.id], ...event.assistantStep };
  }
  if (event.toolCall) {
    next.toolCallsById[event.toolCall.id] = { ...next.toolCallsById[event.toolCall.id], ...event.toolCall };
  }
  if (event.toolResult) {
    next.toolResultsById[event.toolResult.id] = { ...next.toolResultsById[event.toolResult.id], ...event.toolResult };
  }
  if (event.permission) {
    next.permissionsById[event.permission.id] = { ...next.permissionsById[event.permission.id], ...event.permission };
  }
  if (event.agentTask) {
    next.agentTasksById[event.agentTask.id] = { ...next.agentTasksById[event.agentTask.id], ...event.agentTask };
  }
  return next;
}

function applyTextDeltaEvent(store: OutputStore, event: RuntimeOutputEvent): OutputStore {
  const delta = event.textDelta;
  if (!delta || !delta.messageId) {
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
