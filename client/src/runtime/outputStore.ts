import type { OutputStore } from './outputTypes.ts';

export function createOutputStore(sessionId = ''): OutputStore {
  return {
    sessionId,
    messagesById: {},
    turnsById: {},
    assistantStepsById: {},
    toolCallsById: {},
    toolResultsById: {},
    permissionsById: {},
    optimisticByClientRequestId: {},
    appliedEventIds: {},
  };
}
