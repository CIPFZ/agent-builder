export const CANONICAL_CONVERSATION_SCHEMA_VERSION = 2 as const;

export type CanonicalConversationScope = 'full' | 'window';
export type CanonicalConversationOperation = 'upsert' | 'delete';
export type CanonicalMessagePhase = 'reasoning' | 'intermediate' | 'final';
export type CanonicalEntityType = 'turn' | 'message' | 'assistantStep' | 'toolCall' | 'toolResult' | 'permission' | 'todoPlan' | 'agentTask' | 'notice';

export interface CanonicalEntityMeta {
  id: string;
  sessionId: string;
  turnId?: string;
  activitySequence: string;
  revision: string;
  createdAt: number;
  updatedAt: number;
}

export interface CanonicalConversationSnapshot {
  schemaVersion: typeof CANONICAL_CONVERSATION_SCHEMA_VERSION;
  sessionId: string;
  cursor: string;
  scope: CanonicalConversationScope;
  window?: { turnIds?: string[]; beforeCursor?: string; afterCursor?: string; hasMoreBefore?: boolean };
  turns: CanonicalTurn[];
  messages: CanonicalMessage[];
  assistantSteps: CanonicalAssistantStep[];
  toolCalls: CanonicalToolCall[];
  toolResults: CanonicalToolResult[];
  permissions: CanonicalPermission[];
  todoPlans: CanonicalTodoPlan[];
  agentTasks: CanonicalAgentTask[];
  notices: CanonicalNotice[];
}

export interface CanonicalConversationSnapshotRequest {
  scope?: CanonicalConversationScope;
  limit?: number;
  before?: string;
}

export interface CanonicalConversationEventsRequest { after: string; limitRawEvents?: number }
export interface CanonicalConversationEventsResponse { schemaVersion: typeof CANONICAL_CONVERSATION_SCHEMA_VERSION; sessionId: string; afterCursor: string; cursor: string; events: CanonicalConversationEntityEvent[]; snapshotRequired?: boolean; reason?: string }
export type CanonicalConversationEventBatch = CanonicalConversationEventsResponse;
export interface CanonicalConversationMismatch { sessionId: string; cursor: string; entityType: string; entityId?: string; field: string; legacy?: string; canonical?: string }
export interface CanonicalConversationDiagnostics { mode: 'legacy' | 'canonical_v2_shadow' | 'canonical_v2'; mismatches: CanonicalConversationMismatch[] }

export interface CanonicalTurn extends CanonicalEntityMeta { status: string; userMessageId?: string; finalMessageId?: string; startedAt?: number; finishedAt?: number; error?: string }
export interface CanonicalMessage extends CanonicalEntityMeta { role: string; phase?: CanonicalMessagePhase; assistantStepId?: string; status: string; content?: string; partsJson?: string; clientRequestId?: string; error?: string }
export interface CanonicalAssistantStep extends CanonicalEntityMeta { messageId: string; index: number; status: string; startedAt?: number; finishedAt?: number }
export interface CanonicalToolCall extends CanonicalEntityMeta { messageId?: string; assistantStepId?: string; parentToolCallId?: string; roundId?: string; name: string; source: string; kind?: string; status: string; inputJson?: string; command?: string; targets?: string[]; workingDir?: string; risk?: string; resultIds?: string[]; startedAt?: number; finishedAt?: number; exitCode?: number; error?: string }
export interface CanonicalToolResult extends CanonicalEntityMeta { toolCallId: string; ordinal: number; status: string; contentPreview?: string; errorPreview?: string; outputRefs?: string[]; artifactRefs?: string[]; diffRefs?: string[]; deliveredToModel?: boolean }
export interface CanonicalPermission extends CanonicalEntityMeta { toolCallId: string; status: string; action?: string; risk?: string; policyMode?: string; reason?: string; decision?: string; requestedAt?: number; decidedAt?: number }
export interface CanonicalTodoPlan extends CanonicalEntityMeta { ownerTurnId: string; status: string; items: CanonicalTodoItem[] }
export interface CanonicalTodoItem { id: string; order: number; status: string; content: string; activeForm?: string }
export interface CanonicalAgentTask extends CanonicalEntityMeta { parentToolCallId?: string; parentTaskId?: string; childSessionId?: string; teamId?: string; teamRole?: string; title?: string; status: string; progress?: number; dependencies?: string[]; resultRefs?: string[]; outputRefs?: string[] }
export interface CanonicalNotice extends CanonicalEntityMeta { kind: string; status?: string; summary?: string; refs?: string[]; dataJson?: string }

export interface CanonicalConversationEntityEvent {
  schemaVersion: typeof CANONICAL_CONVERSATION_SCHEMA_VERSION;
  id: string;
  sessionId: string;
  turnId?: string;
  sequence: string;
  createdAt: number;
  entityType: CanonicalEntityType;
  entityId: string;
  operation: CanonicalConversationOperation;
  revision: string;
  tombstoneReason?: string;
  turn?: CanonicalTurn;
  message?: CanonicalMessage;
  assistantStep?: CanonicalAssistantStep;
  toolCall?: CanonicalToolCall;
  toolResult?: CanonicalToolResult;
  permission?: CanonicalPermission;
  todoPlan?: CanonicalTodoPlan;
  agentTask?: CanonicalAgentTask;
  notice?: CanonicalNotice;
}
