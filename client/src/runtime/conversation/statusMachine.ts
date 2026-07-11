import type { ConversationTurnStatus } from './conversationTypes.ts';

const terminalStatuses = new Set<ConversationTurnStatus>([
  'completed',
  'failed',
  'cancelled',
  'interrupted',
]);

export function normalizeTurnStatus(status?: string): ConversationTurnStatus {
  switch (status) {
    case 'queued':
    case 'running':
    case 'waiting_permission':
    case 'completed':
    case 'failed':
    case 'cancelled':
    case 'interrupted':
      return status;
    default:
      return 'running';
  }
}

export function isTerminalTurnStatus(status: ConversationTurnStatus) {
  return terminalStatuses.has(status);
}

export function isActiveTurnStatus(status: ConversationTurnStatus) {
  return !isTerminalTurnStatus(status);
}

export function mergeMonotonicTurnStatus(
  previous: string | undefined,
  incoming: string | undefined,
): string | undefined {
  if (!incoming) return previous;
  if (!previous) return incoming;
  const current = normalizeTurnStatus(previous);
  const next = normalizeTurnStatus(incoming);
  if (isTerminalTurnStatus(current) && isActiveTurnStatus(next)) return previous;
  return incoming;
}
