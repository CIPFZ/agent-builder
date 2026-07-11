import type {
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  PermissionRequestViewModel,
  ToolCallViewModel,
} from './workbenchTypes.ts';
import type { OutputStore, RuntimeAgentTaskOutput, RuntimeConversationItem, RuntimeOutputToolCall, RuntimeOutputToolResult, RuntimeToolResultView } from './outputTypes.ts';

// Mirrors runtimeConversationItemSequence in internal/runtime/runtime_output.go:
// sequence = (turnStartMs / 100) * RUNTIME_SEQUENCE_SPAN + rank + intra.
// Keeping the same scale lets an optimistic user submit slot between the
// previous turn's items (smaller turn-start base) and the runtime items of the
// turn it triggers (turn start >= submit time, response ranks > 0).
const RUNTIME_SEQUENCE_SPAN = 100_000;

function optimisticSubmitSequence(createdAtMs: number): number {
  return Math.floor(createdAtMs / 100) * RUNTIME_SEQUENCE_SPAN;
}

export function selectConversationMessages(store: OutputStore): ConversationMessageViewModel[] {
  return selectConversationMessagesFromRuntimeItems(store);
}

export function selectPendingPermissions(store: OutputStore): PermissionRequestViewModel[] {
  return Object.values(store.permissionsById)
    .filter((permission) => permission.status === 'pending' && (!permission.turnId || !isTerminalTurn(store.turnsById[permission.turnId]?.status)))
    .sort((left, right) => compareNumbers(left.createdAt, right.createdAt) || left.id.localeCompare(right.id));
}

export function selectActiveTurn(store: OutputStore) {
  return Object.values(store.turnsById)
    .filter((turn) => !isTerminalTurn(turn.status))
    .sort((left, right) => compareNumbers(right.startedAt, left.startedAt))[0];
}

function isTerminalTurn(status: string | undefined) {
  return ['completed', 'failed', 'cancelled', 'interrupted'].includes(status ?? '');
}

export function selectProjectedConversationItems(store: OutputStore): ConversationTimelineItemViewModel[] {
  return selectRuntimeConversationTimeline(store);
}

function selectConversationMessagesFromRuntimeItems(store: OutputStore): ConversationMessageViewModel[] {
  // A runtime user_message item is the authoritative echo of an optimistic
  // submit. Reconcile via clientRequestId when the runtime provides it;
  // fall back to content match only for the transitional period before the
  // clientRequestId propagates into the item.
  const optimistic = Object.values(store.optimisticByClientRequestId).filter((submit) => {
    return !Object.values(store.itemsById).some((item) => item.role === 'user' && (
      item.clientRequestId === submit.clientRequestId || item.content === submit.prompt
    ));
  }).map((submit) => ({
    id: `optimistic-${submit.clientRequestId}`,
    role: 'user' as const,
    content: submit.prompt,
    createdAt: submit.createdAt,
    clientRequestId: submit.clientRequestId,
    status: submit.status === 'error' ? 'error' as const : 'loading' as const,
    error: submit.error,
  }));
  const messages = Object.values(store.itemsById)
    .filter((item) => item.kind === 'user_message' || item.kind === 'assistant_message')
    .map((item) => {
      const message = item.messageId ? store.messagesById[item.messageId] : undefined;
      const overlay = item.messageId ? store.streamingByMessageId[item.messageId] : undefined;
      const isAssistant = item.role === 'assistant';
      const streamingText = overlay && isAssistant && overlay.text ? overlay.text : undefined;
      return {
        id: item.id,
        role: (isAssistant ? 'assistant' : 'user') as 'user' | 'assistant',
        content: streamingText ?? item.content ?? '',
        createdAt: item.createdAt,
        clientRequestId: item.clientRequestId ?? message?.clientRequestId,
        provider: message?.provider,
        model: message?.model,
        status: item.status === 'streaming' || streamingText ? 'loading' as const : item.error ? 'error' as const : 'success' as const,
        error: item.error,
      };
    });
  return [...optimistic, ...messages].sort(compareConversationRows);
}

function selectRuntimeConversationTimeline(store: OutputStore): ConversationTimelineItemViewModel[] {
  const runtimeItems = Object.values(store.itemsById).sort((left, right) => compareNumbers(left.sequence, right.sequence) || left.id.localeCompare(right.id));
  const optimistic = Object.values(store.optimisticByClientRequestId)
    .filter((submit) => !runtimeItems.some((item) => item.kind === 'user_message' && (
      item.clientRequestId === submit.clientRequestId || item.content === submit.prompt
    )))
    .map((submit): ConversationTimelineItemViewModel => ({
      id: `optimistic-${submit.clientRequestId}`,
      kind: 'user_message',
      sessionId: store.sessionId,
      role: 'user',
      content: submit.prompt,
      status: submit.status === 'error' ? 'error' : 'loading',
      createdAt: submit.createdAt,
      // Without a sequence the shared comparator treats the item as 0 and
      // pins it to the very top of the timeline; project the submit time
      // onto the runtime sequence scale instead so it sorts after history
      // and before the turn it triggers.
      sequence: optimisticSubmitSequence(submit.createdAt),
      clientRequestId: submit.clientRequestId,
      error: submit.error,
      source: 'runtime_activity',
    }));
  return [
    ...optimistic,
    ...runtimeItems.map((item) => projectRuntimeConversationItem(item, store)),
  ].sort((left, right) => compareNumbers(left.sequence, right.sequence) || compareNumbers(left.createdAt, right.createdAt) || left.id.localeCompare(right.id));
}

export function projectRuntimeConversationItem(item: RuntimeConversationItem, store: OutputStore): ConversationTimelineItemViewModel {
  const toolCall = item.toolCallId ? store.toolCallsById[item.toolCallId] : undefined;
  const permission = item.permissionId ? store.permissionsById[item.permissionId] : undefined;
  const overlay = item.messageId ? store.streamingByMessageId[item.messageId] : undefined;
  const isAssistant = item.role === 'assistant';
  let content = item.content;
  let streaming = false;
  if (overlay && isAssistant) {
    if (item.kind === 'assistant_thinking' && overlay.thinking && overlay.thinking.length > (item.content?.length ?? 0)) {
      content = overlay.thinking;
      streaming = true;
    }
    if (item.kind === 'assistant_message' && overlay.text && overlay.text.length > (item.content?.length ?? 0)) {
      content = overlay.text;
      streaming = true;
    }
  }
  return {
    id: item.id,
    kind: normalizeRuntimeTimelineKind(item.kind),
    sessionId: item.sessionId,
    turnId: item.turnId,
    messageId: item.messageId,
    toolCallId: item.toolCallId,
    role: item.role === 'assistant' || item.role === 'user' || item.role === 'tool' || item.role === 'system' ? item.role : undefined,
    title: item.title,
    content,
    summary: item.summary,
    status: item.status,
    phase: item.phase === 'final' ? 'final' : item.phase === 'intermediate' ? 'intermediate' : undefined,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
    sequence: item.sequence,
    source: 'runtime_activity',
    error: item.error,
    clientRequestId: item.clientRequestId,
    streaming: streaming || item.status === 'streaming' ? true : undefined,
    exploration: item.exploration,
    compact: item.compact,
    displayCounts: item.display?.counts,
    toolCall: toolCall ? toolCallViewModel(toolCall, toolCall.result ?? latestToolResult(toolCall, store), store) : item.toolCallIds?.length ? toolGroupViewModel(item, store) : undefined,
    permission: permission ? runtimePermissionViewModel(permission) : undefined,
    agentTask: item.agentTaskId ? runtimeAgentTaskViewModel(store.agentTasksById[item.agentTaskId], item) : undefined,
  };
}

function normalizeRuntimeTimelineKind(kind: string): ConversationTimelineItemViewModel['kind'] {
  if (kind === 'assistant_thinking') {
    return 'assistant_thinking';
  }
  if (kind === 'permission_request') {
    return 'permission_request';
  }
  if (kind === 'turn_progress') {
    return 'turn_progress';
  }
  if (kind === 'diagnostic_warning') {
    return 'diagnostic_warning';
  }
  return kind as ConversationTimelineItemViewModel['kind'];
}

function toolGroupViewModel(item: RuntimeConversationItem, store: OutputStore): ToolCallViewModel | undefined {
  const first = (item.toolCallIds ?? []).map((id) => store.toolCallsById[id]).find(Boolean);
  if (!first) {
    return undefined;
  }
  return toolCallViewModel({
    ...first,
    id: item.id,
    status: item.status ?? first.status,
    kind: item.display?.kind ?? first.kind,
    outputSummary: item.summary,
    display: {
      ...first.display,
      kind: item.display?.kind ?? first.display?.kind,
      title: item.title,
      detail: item.summary,
    },
    groupKey: item.display?.groupKey,
    groupable: item.display?.groupable,
    quiet: item.display?.quiet,
    defaultExpanded: item.display?.defaultExpanded,
  }, undefined, store);
}

function runtimePermissionViewModel(permission: NonNullable<OutputStore['permissionsById'][string]>): PermissionRequestViewModel {
  return {
    id: permission.id,
    sessionId: permission.sessionId,
    turnId: permission.turnId,
    toolCallId: permission.toolCallId,
    toolName: permission.toolName,
    description: permission.description,
    action: permission.action,
    risk: permission.risk,
    status: permission.status,
    path: permission.path,
    target: permission.target,
    reason: permission.reason,
    policyMode: permission.policyMode,
    policyReason: permission.policyReason,
    policyTargetSummary: permission.policyTargetSummary,
    createdAt: permission.createdAt,
    decidedAt: permission.decidedAt,
  };
}

function runtimeAgentTaskViewModel(task: RuntimeAgentTaskOutput | undefined, item?: RuntimeConversationItem) {
  if (!task) {
    if (!item?.agentTaskId) {
      return undefined;
    }
    return {
      id: item.agentTaskId,
      parentSessionId: item.sessionId,
      parentTurnId: item.turnId,
      parentToolCallId: item.toolCallId,
      title: item.title || item.agentTaskId,
      kind: 'agent_task',
      status: item.status || 'unknown',
      progress: 0,
      resultSummary: item.summary,
      startedAt: item.createdAt,
      updatedAt: item.updatedAt,
      error: item.error,
    };
  }
  return {
    id: task.id,
    parentSessionId: task.parentSessionId,
    parentTurnId: task.parentTurnId,
    parentToolCallId: task.parentToolCallId,
    childSessionId: task.childSessionId,
    title: task.title,
    kind: task.kind,
    role: task.role,
    name: task.name,
    promptSummary: task.promptSummary,
    model: task.model,
    provider: task.provider,
    allowedTools: task.allowedTools,
    capabilityScope: task.capabilityScope,
    cwd: task.cwd,
    worktree: task.worktree,
    status: task.status,
    progress: task.progress ?? 0,
    resultSummary: task.resultSummary,
    artifactRefs: task.artifactRefs,
    compactBoundaryRefs: task.compactBoundaryRefs,
    cancellationDetail: task.cancellationDetail,
    startedAt: task.startedAt,
    updatedAt: task.updatedAt,
    finishedAt: task.finishedAt,
    error: task.error,
  };
}

function toolCallViewModel(call: RuntimeOutputToolCall, result: RuntimeOutputToolResult | RuntimeToolResultView | undefined, store?: OutputStore): ToolCallViewModel {
  const agentTask = Object.values(store?.agentTasksById ?? {}).find((task) => task.parentToolCallId === call.id);
  return {
    id: call.id,
    sessionId: call.sessionId,
    turnId: call.turnId,
    name: call.name,
    source: call.source,
    kind: call.kind || call.display?.kind,
    command: call.command,
    risk: call.risk,
    status: call.status,
    inputSummary: call.inputSummary,
    outputSummary: call.outputSummary || result?.contentPreview || result?.dataPreview || call.result?.contentPreview || call.result?.dataPreview,
    error: call.error || (result?.status === 'error' ? result.contentPreview || result.dataPreview : undefined) || (call.result?.status === 'error' ? call.result.contentPreview || call.result.dataPreview : undefined),
    policyMode: call.policyMode,
    policyReason: call.policyReason,
    policyTargetSummary: call.policyTargetSummary,
    display: call.display,
    exitCode: call.exitCode,
    outputRefs: call.outputRefs,
    artifactRefs: call.artifactRefs || result?.artifactRefs || call.result?.artifactRefs,
    diffRefs: call.diffRefs || result?.diffRefs || call.result?.diffRefs,
    startedAt: call.startedAt,
    finishedAt: call.finishedAt,
    parentToolCallId: call.parentToolCallId,
    groupKey: call.groupKey,
    groupable: call.groupable,
    quiet: call.quiet,
    defaultExpanded: call.defaultExpanded,
    agentTask: agentTask ? runtimeAgentTaskViewModel(agentTask) : undefined,
  };
}

function latestToolResult(call: RuntimeOutputToolCall, store: OutputStore) {
  if (call.latestResultId) {
    return store.toolResultsById[call.latestResultId];
  }
  const results = (call.resultIds ?? []).map((id) => store.toolResultsById[id]).filter(Boolean);
  return results[results.length - 1];
}

function compareConversationRows(left: ConversationMessageViewModel, right: ConversationMessageViewModel) {
  return compareNumbers(left.createdAt, right.createdAt) || left.id.localeCompare(right.id);
}

function compareNumbers(left?: number, right?: number) {
  return (left ?? 0) - (right ?? 0);
}
