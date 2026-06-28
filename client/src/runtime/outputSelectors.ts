import type {
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  PermissionRequestViewModel,
  ToolCallViewModel,
} from './workbenchTypes.ts';
import type { OutputStore, RuntimeOutputAssistantStep, RuntimeOutputToolCall, RuntimeOutputToolResult } from './outputTypes.ts';

export function selectConversationMessages(store: OutputStore): ConversationMessageViewModel[] {
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
        status: message.error ? 'error' as const : 'success' as const,
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
      status: step.status === 'failed' ? 'error' : 'success',
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
      }, undefined),
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
  const viewModel = toolCallViewModel(call, result);
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

function toolCallViewModel(call: RuntimeOutputToolCall, result: RuntimeOutputToolResult | undefined): ToolCallViewModel {
  return {
    id: call.id,
    sessionId: call.sessionId,
    turnId: call.turnId,
    name: call.name,
    source: call.source,
    command: call.command,
    risk: call.risk,
    status: call.status,
    inputSummary: call.inputSummary,
    outputSummary: call.outputSummary || result?.contentPreview || result?.dataPreview,
    error: call.error || (result?.status === 'error' ? result.contentPreview || result.dataPreview : undefined),
    policyMode: call.policyMode,
    policyReason: call.policyReason,
    policyTargetSummary: call.policyTargetSummary,
    display: call.display,
    exitCode: call.exitCode,
    outputRefs: call.outputRefs,
    artifactRefs: call.artifactRefs || result?.artifactRefs,
    diffRefs: call.diffRefs || result?.diffRefs,
    startedAt: call.startedAt,
    finishedAt: call.finishedAt,
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
