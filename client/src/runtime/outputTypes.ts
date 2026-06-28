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
  resultIds?: string[];
  latestResultId?: string;
  name: string;
  source: string;
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
  exitCode?: number;
  outputRefs?: string[];
  artifactRefs?: string[];
  diffRefs?: string[];
  startedAt?: number;
  finishedAt?: number;
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
  messages?: RuntimeOutputMessage[];
  turns?: RuntimeOutputTurn[];
  assistantSteps?: RuntimeOutputAssistantStep[];
  toolCalls?: RuntimeOutputToolCall[];
  toolResults?: RuntimeOutputToolResult[];
  permissions?: RuntimeOutputPermission[];
}

export interface RuntimeOutputEvent {
  id: string;
  sequence: number;
  sessionId: string;
  turnId?: string;
  kind: string;
  entityId: string;
  operation: 'append' | 'update' | 'delete' | 'snapshot' | string;
  createdAt?: number;
  message?: RuntimeOutputMessage;
  turn?: RuntimeOutputTurn;
  assistantStep?: RuntimeOutputAssistantStep;
  toolCall?: RuntimeOutputToolCall;
  toolResult?: RuntimeOutputToolResult;
  permission?: RuntimeOutputPermission;
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
  lastSequence?: number;
  messagesById: Record<string, RuntimeOutputMessage>;
  turnsById: Record<string, RuntimeOutputTurn>;
  assistantStepsById: Record<string, RuntimeOutputAssistantStep>;
  toolCallsById: Record<string, RuntimeOutputToolCall>;
  toolResultsById: Record<string, RuntimeOutputToolResult>;
  permissionsById: Record<string, RuntimeOutputPermission>;
  optimisticByClientRequestId: Record<string, OptimisticUserSubmit>;
  appliedEventIds: Record<string, true>;
}
