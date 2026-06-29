import type {
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  PermissionRequestViewModel,
  ToolCallViewModel,
} from './workbenchTypes.ts';
import type { OutputStore, RuntimeAgentTaskOutput, RuntimeConversationItem, RuntimeOutputAssistantStep, RuntimeOutputToolCall, RuntimeOutputToolResult, RuntimeToolResultView } from './outputTypes.ts';

export function selectConversationMessages(store: OutputStore): ConversationMessageViewModel[] {
  if (store.version) {
    return selectConversationMessagesFromRuntimeItems(store);
  }
  return [
    ...Object.values(store.optimisticByClientRequestId).map((optimistic) => ({
      id: `optimistic-${optimistic.clientRequestId}`,
      role: 'user' as const,
      content: optimistic.prompt,
      createdAt: optimistic.createdAt,
      clientRequestId: optimistic.clientRequestId,
      status: optimistic.status === 'error' ? 'error' as const : 'loading' as const,
      error: optimistic.error,
    })),
    ...Object.values(store.messagesById)
      .filter((message) => !message.hidden && (message.role === 'user' || message.role === 'assistant'))
      .map((message) => ({
        id: message.id,
        role: message.role,
        content: message.content ?? '',
        createdAt: message.createdAt,
        clientRequestId: message.clientRequestId,
        provider: message.provider,
        model: message.model,
        status: conversationMessageStatus(message.role, message.error, message.finished),
        error: message.error,
      })),
  ].sort(compareConversationRows);
}

export function selectPendingPermissions(store: OutputStore): PermissionRequestViewModel[] {
  return Object.values(store.permissionsById)
    .filter((permission) => permission.status === 'pending')
    .sort((left, right) => compareNumbers(left.createdAt, right.createdAt) || left.id.localeCompare(right.id));
}

export function selectActiveTurn(store: OutputStore) {
  return Object.values(store.turnsById)
    .filter((turn) => !['completed', 'failed', 'cancelled'].includes(turn.status))
    .sort((left, right) => compareNumbers(right.startedAt, left.startedAt))[0];
}

export function selectConversationTimeline(store: OutputStore): ConversationTimelineItemViewModel[] {
  if (store.version) {
    return selectRuntimeConversationTimeline(store);
  }
  // Diagnostics-only fallback for legacy snapshots that do not provide runtime-owned conversation items.
  const items: ConversationTimelineItemViewModel[] = [];
  const stepsByTurn = groupAssistantStepsByTurn(store);
  const turnIDByUserMessage = Object.fromEntries(Object.values(store.turnsById).filter((turn) => turn.userMessageId).map((turn) => [turn.userMessageId, turn.id]));

  for (const optimistic of Object.values(store.optimisticByClientRequestId)) {
    items.push({
      id: `optimistic-${optimistic.clientRequestId}`,
      kind: 'message',
      sessionId: store.sessionId,
      role: 'user',
      content: optimistic.prompt,
      status: optimistic.status === 'error' ? 'error' : 'loading',
      createdAt: optimistic.createdAt,
      clientRequestId: optimistic.clientRequestId,
      error: optimistic.error,
      source: 'runtime_activity',
    });
  }

  for (const message of Object.values(store.messagesById)) {
    if (message.hidden || message.role !== 'user') {
      continue;
    }
    items.push({
      id: `message-${message.id}`,
      kind: 'message',
      sessionId: message.sessionId,
      turnId: turnIDByUserMessage[message.id],
      messageId: message.id,
      role: 'user',
      content: message.content ?? '',
      createdAt: message.createdAt,
      updatedAt: message.updatedAt,
      clientRequestId: message.clientRequestId,
      status: message.error ? 'error' : 'success',
      error: message.error,
      source: 'runtime_activity',
    });
  }

  const turns = Object.values(store.turnsById).sort((left, right) => compareNumbers(left.startedAt, right.startedAt) || left.id.localeCompare(right.id));
  for (const turn of turns) {
    for (const step of stepsByTurn[turn.id] ?? []) {
      pushAssistantStepItems(items, store, step);
    }
    if (!['completed', 'failed', 'cancelled'].includes(turn.status)) {
      items.push({
        id: `turn-progress-${turn.id}`,
        kind: 'progress',
        sessionId: turn.sessionId,
        turnId: turn.id,
        title: turn.status,
        status: turn.status,
        createdAt: turn.startedAt,
        updatedAt: turn.finishedAt,
        source: 'runtime_activity',
      });
    }
  }

  for (const step of Object.values(store.assistantStepsById).filter((step) => !store.turnsById[step.turnId])) {
    pushAssistantStepItems(items, store, step);
  }

  return items.sort(compareTimelineRows);
}

function selectConversationMessagesFromRuntimeItems(store: OutputStore): ConversationMessageViewModel[] {
  const optimistic = Object.values(store.optimisticByClientRequestId).filter((submit) => {
    return !Object.values(store.itemsById).some((item) => item.role === 'user' && item.content === submit.prompt);
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
      return {
        id: item.id,
        role: (item.role === 'assistant' ? 'assistant' : 'user') as 'user' | 'assistant',
        content: item.content ?? '',
        createdAt: item.createdAt,
        clientRequestId: message?.clientRequestId,
        provider: message?.provider,
        model: message?.model,
        status: item.status === 'streaming' ? 'loading' as const : item.error ? 'error' as const : 'success' as const,
        error: item.error,
      };
    });
  return [...optimistic, ...messages].sort(compareConversationRows);
}

function selectRuntimeConversationTimeline(store: OutputStore): ConversationTimelineItemViewModel[] {
  const runtimeItems = Object.values(store.itemsById).sort((left, right) => compareNumbers(left.sequence, right.sequence) || left.id.localeCompare(right.id));
  const optimistic = Object.values(store.optimisticByClientRequestId)
    .filter((submit) => !runtimeItems.some((item) => item.kind === 'user_message' && item.content === submit.prompt))
    .map((submit): ConversationTimelineItemViewModel => ({
      id: `optimistic-${submit.clientRequestId}`,
      kind: 'user_message',
      sessionId: store.sessionId,
      role: 'user',
      content: submit.prompt,
      status: submit.status === 'error' ? 'error' : 'loading',
      createdAt: submit.createdAt,
      clientRequestId: submit.clientRequestId,
      error: submit.error,
      source: 'runtime_activity',
    }));
  return [
    ...optimistic,
    ...runtimeItems.map((item) => runtimeConversationItemViewModel(item, store)),
  ].sort((left, right) => compareNumbers(left.sequence, right.sequence) || compareNumbers(left.createdAt, right.createdAt) || left.id.localeCompare(right.id));
}

function runtimeConversationItemViewModel(item: RuntimeConversationItem, store: OutputStore): ConversationTimelineItemViewModel {
  const toolCall = item.toolCallId ? store.toolCallsById[item.toolCallId] : undefined;
  const permission = item.permissionId ? store.permissionsById[item.permissionId] : undefined;
  return {
    id: item.id,
    kind: normalizeRuntimeTimelineKind(item.kind),
    sessionId: item.sessionId,
    turnId: item.turnId,
    messageId: item.messageId,
    toolCallId: item.toolCallId,
    role: item.role === 'assistant' || item.role === 'user' || item.role === 'tool' || item.role === 'system' ? item.role : undefined,
    title: item.title,
    content: item.content,
    summary: item.summary,
    status: item.status,
    phase: item.phase === 'final' ? 'final' : item.phase === 'intermediate' ? 'intermediate' : undefined,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
    sequence: item.sequence,
    source: 'runtime_activity',
    error: item.error,
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

function pushAssistantStepItems(items: ConversationTimelineItemViewModel[], store: OutputStore, step: RuntimeOutputAssistantStep) {
  if (step.thinkingSummary) {
    items.push({
      id: `thinking-${step.id}`,
      kind: 'thinking',
      sessionId: step.sessionId,
      turnId: step.turnId,
      messageId: step.messageId,
      content: step.thinkingSummary,
      summary: step.thinkingSummary,
      status: step.status,
      phase: 'intermediate',
      createdAt: step.startedAt,
      updatedAt: step.updatedAt,
      source: 'runtime_activity',
    });
  }
  if (step.text) {
    items.push({
      id: `assistant-${step.id}`,
      kind: 'message',
      sessionId: step.sessionId,
      turnId: step.turnId,
      messageId: step.messageId,
      role: 'assistant',
      content: step.text,
      status: assistantStepMessageStatus(step.status),
      phase: step.status === 'completed' ? 'final' : 'intermediate',
      createdAt: step.startedAt,
      updatedAt: step.updatedAt,
      source: 'runtime_activity',
    });
  }
  pushToolItems(items, store, step);
}

function pushToolItems(items: ConversationTimelineItemViewModel[], store: OutputStore, step: RuntimeOutputAssistantStep) {
  const calls = (step.toolCallIds ?? [])
    .map((id) => store.toolCallsById[id])
    .filter((call): call is RuntimeOutputToolCall => Boolean(call));
  const completedGroups = new Map<string, RuntimeOutputToolCall[]>();
  for (const call of calls) {
    const permission = Object.values(store.permissionsById).find((candidate) => candidate.toolCallId === call.id && candidate.status === 'pending');
    const bucket = toolStatusBucket(call, permission);
    if (bucket === 'completed') {
      const key = `${step.id}:${call.display?.kind ?? call.source ?? call.name}:completed`;
      completedGroups.set(key, [...(completedGroups.get(key) ?? []), call]);
      continue;
    }
    items.push(toolTimelineItem(step, call, store, permission, bucket));
    if (permission) {
      items.push(permissionTimelineItem(step, permission));
    }
  }
  for (const [key, group] of completedGroups) {
    if (group.length === 1) {
      items.push(toolTimelineItem(step, group[0], store, undefined, 'completed'));
      continue;
    }
    const first = group[0];
    items.push({
      id: `tool-group-${key}`,
      kind: 'tool_call',
      sessionId: step.sessionId,
      turnId: step.turnId,
      toolCallId: first.id,
      title: completedToolGroupTitle(group),
      summary: completedToolGroupTitle(group),
      status: 'completed',
      createdAt: first.startedAt ?? step.startedAt,
      updatedAt: group[group.length - 1].finishedAt ?? step.updatedAt,
      toolCall: toolCallViewModel({
        ...first,
        id: `group-${key}`,
        name: first.name,
        status: 'completed',
        outputSummary: completedToolGroupTitle(group),
      }, undefined, store),
      source: 'runtime_activity',
    });
  }
}

function toolTimelineItem(
  step: RuntimeOutputAssistantStep,
  call: RuntimeOutputToolCall,
  store: OutputStore,
  permission: PermissionRequestViewModel | undefined,
  bucket: string,
): ConversationTimelineItemViewModel {
  const result = latestToolResult(call, store);
  const viewModel = toolCallViewModel(call, result, store);
  return {
    id: `tool-${step.id}-${call.id}`,
    kind: 'tool_call',
    sessionId: call.sessionId,
    turnId: call.turnId || step.turnId,
    messageId: call.messageId || step.messageId,
    toolCallId: call.id,
    title: viewModel.display?.title || call.name,
    summary: viewModel.outputSummary || viewModel.inputSummary || viewModel.display?.detail,
    status: permission ? 'waiting_permission' : bucket,
    createdAt: call.startedAt ?? step.startedAt,
    updatedAt: call.finishedAt ?? step.updatedAt,
    toolCall: viewModel,
    permission,
    source: 'runtime_activity',
  };
}

function permissionTimelineItem(step: RuntimeOutputAssistantStep, permission: PermissionRequestViewModel): ConversationTimelineItemViewModel {
  return {
    id: `permission-${step.id}-${permission.id}`,
    kind: 'permission',
    sessionId: permission.sessionId,
    turnId: permission.turnId || step.turnId,
    toolCallId: permission.toolCallId,
    title: permission.toolName,
    summary: permission.description || permission.reason || permission.policyReason,
    status: permission.status,
    createdAt: permission.createdAt,
    updatedAt: permission.decidedAt,
    permission,
    source: 'runtime_activity',
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

function toolStatusBucket(call: RuntimeOutputToolCall, permission?: PermissionRequestViewModel) {
  if (permission) {
    return 'waiting_permission';
  }
  if (['failed', 'error'].includes(call.status) || call.error) {
    return 'failed';
  }
  if (['completed', 'success'].includes(call.status)) {
    return 'completed';
  }
  if (['running', 'started'].includes(call.status)) {
    return 'running';
  }
  return 'pending';
}

function completedToolGroupTitle(group: RuntimeOutputToolCall[]) {
  const kind = group[0]?.display?.kind || group[0]?.source || 'tools';
  if (kind === 'shell') {
    return `Ran ${group.length} commands`;
  }
  if (kind.includes('file_read')) {
    return `Read ${group.length} files`;
  }
  if (kind.includes('file_write') || kind.includes('file_edit')) {
    return `Edited ${group.length} files`;
  }
  return `Completed ${group.length} tools`;
}

function conversationMessageStatus(role: string, error?: string, finished?: boolean) {
  if (error) {
    return 'error' as const;
  }
  if (role === 'assistant' && !finished) {
    return 'loading' as const;
  }
  return 'success' as const;
}

function assistantStepMessageStatus(status: string) {
  if (status === 'failed') {
    return 'error';
  }
  if (status === 'completed') {
    return 'success';
  }
  return 'loading';
}

function groupAssistantStepsByTurn(store: OutputStore) {
  const grouped: Record<string, RuntimeOutputAssistantStep[]> = {};
  for (const step of Object.values(store.assistantStepsById)) {
    grouped[step.turnId] = [...(grouped[step.turnId] ?? []), step];
  }
  for (const turnID of Object.keys(grouped)) {
    grouped[turnID].sort((left, right) => compareNumbers(left.index, right.index) || left.id.localeCompare(right.id));
  }
  return grouped;
}

function compareTimelineRows(left: ConversationTimelineItemViewModel, right: ConversationTimelineItemViewModel) {
  if (left.turnId && left.turnId === right.turnId) {
    const rank = timelineTurnRank(left) - timelineTurnRank(right);
    if (rank !== 0) {
      return rank;
    }
  }
  return compareNumbers(left.createdAt, right.createdAt) || compareNumbers(left.sequence, right.sequence) || left.id.localeCompare(right.id);
}

function timelineTurnRank(item: ConversationTimelineItemViewModel) {
  if (item.kind === 'message' && item.role === 'user') {
    return 0;
  }
  if (item.kind === 'thinking') {
    return 1;
  }
  if (item.kind === 'message' && item.role === 'assistant') {
    return 2;
  }
  if (item.kind === 'tool_call') {
    return 3;
  }
  if (item.kind === 'permission') {
    return 4;
  }
  if (item.kind === 'progress') {
    return 9;
  }
  return 5;
}

function compareConversationRows(left: ConversationMessageViewModel, right: ConversationMessageViewModel) {
  return compareNumbers(left.createdAt, right.createdAt) || left.id.localeCompare(right.id);
}

function compareNumbers(left?: number, right?: number) {
  return (left ?? 0) - (right ?? 0);
}
