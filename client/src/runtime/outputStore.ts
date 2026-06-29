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
  };
}
