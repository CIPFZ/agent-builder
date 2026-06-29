import { createOutputStore } from './outputStore.ts';
import type { OptimisticUserSubmit, OutputStore, RuntimeOutputEvent, RuntimeOutputSnapshot } from './outputTypes.ts';

export function hydrateOutputStore(snapshot: RuntimeOutputSnapshot | undefined, previous?: OutputStore): OutputStore {
  if (!snapshot) {
    return previous ?? createOutputStore();
  }
  const store = createOutputStore(snapshot.sessionId);
  store.cursor = snapshot.cursor;
  store.version = snapshot.version;
  store.optimisticByClientRequestId = { ...(previous?.sessionId === snapshot.sessionId ? previous.optimisticByClientRequestId : {}) };
  for (const item of snapshot.items ?? []) {
    store.itemsById[item.id] = item;
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
  if (!event?.id || store.appliedEventIds[event.id]) {
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
    return next;
  }
  if (event.item) {
    next.itemsById[event.item.id] = { ...next.itemsById[event.item.id], ...event.item };
    if (event.item.role === 'user') {
      for (const message of Object.values(next.messagesById)) {
        if (message.id === event.item.messageId && message.clientRequestId) {
          delete next.optimisticByClientRequestId[message.clientRequestId];
        }
      }
    }
  }
  if (event.message) {
    next.messagesById[event.message.id] = { ...next.messagesById[event.message.id], ...event.message };
    if (event.message.clientRequestId) {
      delete next.optimisticByClientRequestId[event.message.clientRequestId];
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
