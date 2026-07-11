export interface RuntimeOutputMessage {
  id: string;
  sessionId: string;
  role: 'user' | 'assistant' | 'tool' | 'system';
  content?: string;
  parts?: RuntimeOutputMessagePart[];
  metadata?: Record<string, string>;
  clientRequestId?: string;
  inputMode?: string;
  hidden?: boolean;
  provider?: string;
  model?: string;
  createdAt?: number;
  updatedAt?: number;
  finished?: boolean;
  finishReason?: string;
  error?: string;
}

export interface RuntimeOutputMessagePart {
  type: string;
  text?: string;
  thinking?: string;
  toolCallId?: string;
  name?: string;
  input?: string;
  content?: string;
  data?: string;
  isError?: boolean;
}

export interface RuntimeOutputTurn {
  id: string;
  sessionId: string;
  status: string;
  userMessageId?: string;
  latestAssistantMessageId?: string;
  latestMessageId?: string;
  startedAt?: number;
  finishedAt?: number;
  error?: string;
}

export interface RuntimeOutputAssistantStep {
  id: string;
  sessionId: string;
  turnId: string;
  messageId: string;
  index: number;
  status: string;
  text?: string;
  thinkingSummary?: string;
  toolCallIds?: string[];
  startedAt?: number;
  updatedAt?: number;
  finishedAt?: number;
}

export interface RuntimeOutputToolCall {
  id: string;
  sessionId: string;
  turnId: string;
  messageId?: string;
  assistantStepId?: string;
  assistantItemId?: string;
  parentToolCallId?: string;
  resultIds?: string[];
  latestResultId?: string;
  name: string;
  source: string;
  capabilityId?: string;
  kind?: string;
  command?: string;
  risk?: string;
  status: string;
  inputSummary?: string;
  outputSummary?: string;
  stdout?: string;
  stderr?: string;
  error?: string;
  policyMode?: string;
  policyReason?: string;
  policyTargetSummary?: string;
  display?: {
    kind?: string;
    title?: string;
    detail?: string;
    target?: string;
    primaryTarget?: string;
    targets?: string[];
    workingDir?: string;
    command?: string;
    exitCode?: number;
    durationMs?: number;
    stdoutExcerpt?: string;
    stderrExcerpt?: string;
    inputExcerpt?: string;
    outputExcerpt?: string;
    failureReason?: string;
    artifactCount?: number;
    diffCount?: number;
    artifactRefs?: string[];
    diffRefs?: string[];
    artifactSummary?: string;
    diffSummary?: string;
  };
  result?: RuntimeToolResultView;
  groupKey?: string;
  groupable?: boolean;
  quiet?: boolean;
  defaultExpanded?: boolean;
  exitCode?: number;
  outputRefs?: string[];
  artifactRefs?: string[];
  diffRefs?: string[];
  startedAt?: number;
  finishedAt?: number;
}

export interface RuntimeConversationDisplay {
  kind?: string;
  title?: string;
  detail?: string;
  target?: string;
  primaryTarget?: string;
  targets?: string[];
  groupKey?: string;
  groupable?: boolean;
  quiet?: boolean;
  defaultExpanded?: boolean;
  toolCallIds?: string[];
  counts?: RuntimeExplorationCount[];
}

export interface RuntimeExplorationCount {
  kind: string;
  count: number;
  failed?: number;
}

export interface RuntimeExplorationSummary {
  status: string;
  toolCounts?: RuntimeExplorationCount[];
  toolTotal?: number;
  failedCount?: number;
  subagentCount?: number;
  thinkingMs?: number;
  elapsedMs?: number;
}

export interface RuntimeToolResultView {
  id?: string;
  messageId?: string;
  status?: string;
  contentPreview?: string;
  dataPreview?: string;
  deliveredToModel?: boolean;
  synthetic?: boolean;
  reason?: string;
  artifactRefs?: string[];
  diffRefs?: string[];
  createdAt?: number;
}

export interface RuntimeOutputToolResult {
  id: string;
  sessionId: string;
  turnId: string;
  messageId: string;
  toolCallId: string;
  toolName: string;
  status: string;
  contentPreview?: string;
  dataPreview?: string;
  metadata?: string;
  artifactRefs?: string[];
  diffRefs?: string[];
  deliveredToModel?: boolean;
  createdAt?: number;
}

export interface RuntimeConversationItem {
  id: string;
  kind: string;
  sessionId: string;
  turnId?: string;
  parentId?: string;
  sequence: number;
  role?: 'user' | 'assistant' | 'tool' | 'system' | string;
  phase?: 'intermediate' | 'final' | string;
  status?: string;
  title?: string;
  summary?: string;
  content?: string;
  error?: string;
  messageId?: string;
  toolCallId?: string;
  toolCallIds?: string[];
  permissionId?: string;
  hookRunId?: string;
  agentTaskId?: string;
  contextId?: string;
  clientRequestId?: string;
  display?: RuntimeConversationDisplay;
  exploration?: RuntimeExplorationSummary;
  compact?: RuntimeCompactInfo;
  createdAt?: number;
  updatedAt?: number;
}

// RuntimeCompactInfo mirrors internal/runtime.RuntimeCompactInfo (WP5): the
// compact_boundary item's trigger/status/token-delta meta plus the full
// summary text, so the timeline process trace can render compact detail
// without a second round trip.
export interface RuntimeCompactInfo {
  trigger?: string;
  status?: string;
  preTokens?: number;
  postTokens?: number;
  summarizedCount?: number;
  summaryMessageId?: string;
  summaryText?: string;
  error?: string;
}

export interface RuntimeAgentTaskOutput {
  id: string;
  parentSessionId: string;
  parentTurnId?: string;
  parentToolCallId?: string;
  childSessionId?: string;
  title: string;
  kind: string;
  role?: string;
  name?: string;
  promptSummary?: string;
  model?: string;
  provider?: string;
  allowedTools?: string[];
  capabilityScope?: string[];
  cwd?: string;
  worktree?: string;
  status: string;
  progress: number;
  resultSummary?: string;
  artifactRefs?: string[];
  compactBoundaryRefs?: string[];
  cancellationDetail?: string;
  startedAt?: number;
  updatedAt?: number;
  finishedAt?: number;
  error?: string;
}

export interface RuntimeOutputPermission {
  id: string;
  sessionId: string;
  turnId?: string;
  toolCallId: string;
  toolName: string;
  description?: string;
  action: string;
  risk?: string;
  status: string;
  path?: string;
  target?: string;
  reason?: string;
  policyMode?: string;
  policyReason?: string;
  policyRuleId?: string;
  policyRuleSource?: string;
  policyScopeKind?: string;
  policyScopeValue?: string;
  policyTargetSummary?: string;
  createdAt?: number;
  decidedAt?: number;
}

export interface RuntimeOutputSnapshot {
  sessionId: string;
  cursor?: string;
  version?: number;
  items?: RuntimeConversationItem[];
  messages?: RuntimeOutputMessage[];
  turns?: RuntimeOutputTurn[];
  assistantSteps?: RuntimeOutputAssistantStep[];
  toolCalls?: RuntimeOutputToolCall[];
  toolResults?: RuntimeOutputToolResult[];
  permissions?: RuntimeOutputPermission[];
  hooks?: unknown[];
  agentTasks?: RuntimeAgentTaskOutput[];
  todos?: unknown;
  compact?: unknown[];
}

export interface RuntimeOutputEvent {
  id: string;
  sequence: number;
  sessionId: string;
  turnId?: string;
  kind: string;
  entityId: string;
  operation: 'append' | 'update' | 'delete' | 'snapshot' | 'delta' | 'reset' | string;
  createdAt?: number;
  item?: RuntimeConversationItem;
  message?: RuntimeOutputMessage;
  turn?: RuntimeOutputTurn;
  assistantStep?: RuntimeOutputAssistantStep;
  toolCall?: RuntimeOutputToolCall;
  toolResult?: RuntimeOutputToolResult;
  permission?: RuntimeOutputPermission;
  hook?: unknown;
  agentTask?: RuntimeAgentTaskOutput;
  todos?: unknown;
  compact?: unknown;
  textDelta?: RuntimeOutputTextDelta;
}

export interface RuntimeOutputTextDelta {
  messageId: string;
  turnId?: string;
  partType: 'text' | 'reasoning' | string;
  delta: string;
  contentLen: number;
}

export interface RuntimeStreamingState {
  text: string;
  thinking: string;
  textLen: number;
  thinkingLen: number;
}

export interface RuntimeOutputEventsResponse {
  sessionId: string;
  cursor?: string;
  events?: RuntimeOutputEvent[];
}

export interface OptimisticUserSubmit {
  clientRequestId: string;
  prompt: string;
  createdAt: number;
  status: 'submitting' | 'error';
  error?: string;
}

export interface OutputStore {
  sessionId: string;
  cursor?: string;
  version?: number;
  lastSequence?: number;
  itemsById: Record<string, RuntimeConversationItem>;
  messagesById: Record<string, RuntimeOutputMessage>;
  turnsById: Record<string, RuntimeOutputTurn>;
  assistantStepsById: Record<string, RuntimeOutputAssistantStep>;
  toolCallsById: Record<string, RuntimeOutputToolCall>;
  toolResultsById: Record<string, RuntimeOutputToolResult>;
  permissionsById: Record<string, RuntimeOutputPermission>;
  agentTasksById: Record<string, RuntimeAgentTaskOutput>;
  optimisticByClientRequestId: Record<string, OptimisticUserSubmit>;
  appliedEventIds: Record<string, true>;
  streamingByMessageId: Record<string, RuntimeStreamingState>;
}
