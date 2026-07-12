import type { ConversationTimelineItemViewModel } from '../workbenchTypes.ts';
import type { RuntimeExplorationSummary } from '../conversationPresentationTypes.ts';

export type ConversationTurnStatus =
  | 'queued'
  | 'running'
  | 'waiting_permission'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'interrupted';

export interface ConversationProcessViewModel {
  status: ConversationTurnStatus;
  items: ConversationTimelineItemViewModel[];
  exploration?: RuntimeExplorationSummary;
  startedAt?: number;
  finishedAt?: number;
  hasFailure: boolean;
}

export interface ConversationTurnViewModel {
  id: string;
  sessionId: string;
  status: ConversationTurnStatus;
  user?: ConversationTimelineItemViewModel;
  final?: ConversationTimelineItemViewModel;
  process: ConversationProcessViewModel;
  startedAt?: number;
  finishedAt?: number;
  error?: string;
}
