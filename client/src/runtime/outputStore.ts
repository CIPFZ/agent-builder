import type { OutputStore } from './outputTypes.ts';

export function createOutputStore(sessionId = ''): OutputStore {
  return {
    sessionId,
    itemsById: {},
    messagesById: {},
    turnsById: {},
    assistantStepsById: {},
    toolCallsById: {},
    toolResultsById: {},
    permissionsById: {},
    agentTasksById: {},
    optimisticByClientRequestId: {},
    appliedEventIds: {},
    entitySequenceByKey: {},
    streamingByMessageId: {},
  };
}

export function retargetOutputStore(store: OutputStore | undefined, sessionId: string): OutputStore {
  if (store?.sessionId === sessionId) {
    return store;
  }
  return {
    ...createOutputStore(sessionId),
    optimisticByClientRequestId: { ...(store?.optimisticByClientRequestId ?? {}) },
  };
}
