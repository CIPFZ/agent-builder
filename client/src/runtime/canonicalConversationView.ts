import { selectCanonicalConversationTurns, type CanonicalProcessEntity } from './canonicalConversationSelectors.ts';
import { canonicalToolGroupKey } from './canonicalConversationPresentation.ts';
import { groupCanonicalProcess } from './canonicalConversationPresentation.ts';
import type { CanonicalConversationStore } from './canonicalConversationStore.ts';
import type { CanonicalMessage, CanonicalToolCall } from './canonicalConversationTypes.ts';
import type { ConversationTurnViewModel, ConversationTurnStatus } from './conversation/conversationTypes.ts';
import type { ConversationTimelineItemViewModel, OptimisticConversationSubmit, ToolCallViewModel } from './workbenchTypes.ts';
import { selectCanonicalStructuredActivity, type CanonicalStructuredActivity } from './canonicalStructuredActivity.ts';

export function selectCanonicalConversationTurnViewModels(store?: CanonicalConversationStore, structured = selectCanonicalStructuredActivity(store), optimistic?: Record<string, OptimisticConversationSubmit>): ConversationTurnViewModel[] {
  const turns: ConversationTurnViewModel[] = selectCanonicalConversationTurns(store).map(({ turn, user, final, process }) => {
    const status = turn.status as ConversationTurnStatus;
    const items = groupCanonicalProcess(process).map((item) => item.type === 'tool-group'
      ? projectToolGroup(item.key, item.tools, store!, structured)
      : projectProcessEntity(item.entity, store!, structured)).filter((item): item is ConversationTimelineItemViewModel => Boolean(item));
    return {
      id: turn.id, sessionId: turn.sessionId, status,
      user: user ? projectMessage(user) : undefined,
      final: final ? projectMessage(final) : undefined,
      process: { status, items, startedAt: turn.startedAt, finishedAt: turn.finishedAt, hasFailure: status === 'failed' || items.some((item) => isFailure(item.status)) },
      startedAt: turn.startedAt, finishedAt: turn.finishedAt, error: turn.error,
    };
  });
  const echoed = new Set(Object.values(store?.messagesById ?? {}).map((message) => message.clientRequestId).filter(Boolean));
  for (const submit of Object.values(optimistic ?? {})) {
    if (echoed.has(submit.clientRequestId) || (store?.sessionId && submit.sessionId && submit.sessionId !== store.sessionId)) continue;
    turns.push({ id: `optimistic:${submit.clientRequestId}`, sessionId: submit.sessionId || store?.sessionId || '', status: submit.status === 'error' ? 'failed' : 'queued', startedAt: submit.createdAt, user: { id: `optimistic-message:${submit.clientRequestId}`, kind: 'user_message', sessionId: submit.sessionId, role: 'user', content: submit.prompt, status: submit.status === 'error' ? 'error' : 'loading', createdAt: submit.createdAt, clientRequestId: submit.clientRequestId, error: submit.error }, process: { status: submit.status === 'error' ? 'failed' : 'queued', items: [], startedAt: submit.createdAt, finishedAt: undefined, hasFailure: submit.status === 'error' }, error: submit.error });
  }
  return turns.sort((left, right) => (left.startedAt ?? 0) - (right.startedAt ?? 0) || left.id.localeCompare(right.id));
}

function projectProcessEntity(entity: CanonicalProcessEntity, store: CanonicalConversationStore, structured: CanonicalStructuredActivity): ConversationTimelineItemViewModel | undefined {
  if ('role' in entity) return projectMessage(entity);
  if ('name' in entity && 'source' in entity) return projectTool(entity, store, structured);
  if ('toolCallId' in entity && !('ordinal' in entity)) return { id: `permission:${entity.id}`, kind: 'permission', sessionId: entity.sessionId, turnId: entity.turnId, status: entity.status, createdAt: entity.createdAt, updatedAt: entity.updatedAt, permission: structured.permissionsById[entity.id] };
  if (structured.agentTasksById[entity.id]) return { id: `agentTask:${entity.id}`, kind: 'agent_task', sessionId: entity.sessionId, turnId: entity.turnId, status: entity.status, createdAt: entity.createdAt, updatedAt: entity.updatedAt, agentTask: structured.agentTasksById[entity.id] };
  if (store.noticesById[entity.id]) return projectNotice(store.noticesById[entity.id]);
  return undefined;
}

function projectNotice(notice: CanonicalConversationStore['noticesById'][string]): ConversationTimelineItemViewModel {
  const data = parseNoticeData(notice.dataJson);
  const common = { id: `notice:${notice.id}`, sessionId: notice.sessionId, turnId: notice.turnId, title: String(data.trigger || data.source_id || notice.kind), content: notice.summary, summary: notice.summary, status: notice.status, createdAt: notice.createdAt, updatedAt: notice.updatedAt };
  if (notice.kind === 'hook') return { ...common, kind: 'hook_run' };
  if (notice.kind === 'context') return { ...common, kind: 'context_source' };
  if (notice.kind === 'compact') return { ...common, kind: 'compact_boundary', compact: { trigger: stringValue(data.trigger), status: notice.status, preTokens: numberValue(data.pre_tokens), postTokens: numberValue(data.post_tokens), summarizedCount: numberValue(data.summarized_count), summaryMessageId: stringValue(data.summary_message_id), summaryText: stringValue(data.summary_text), error: stringValue(data.error) }, error: stringValue(data.error) };
  return { ...common, kind: 'recovery_notice', error: notice.status === 'failed' ? notice.summary : undefined };
}

function parseNoticeData(value?: string): Record<string, unknown> { try { return value ? JSON.parse(value) as Record<string, unknown> : {}; } catch { return {}; } }
function stringValue(value: unknown) { return typeof value === 'string' ? value : undefined; }
function numberValue(value: unknown) { return typeof value === 'number' ? value : undefined; }

function projectMessage(message: CanonicalMessage): ConversationTimelineItemViewModel {
  return { id: `message:${message.id}`, kind: message.role === 'user' ? 'user_message' : message.phase === 'reasoning' ? 'assistant_thinking' : 'assistant_message', sessionId: message.sessionId, turnId: message.turnId, messageId: message.id, role: message.role as 'user' | 'assistant' | 'tool' | 'system', phase: message.phase === 'final' ? 'final' : 'intermediate', content: message.content, status: message.status, createdAt: message.createdAt, updatedAt: message.updatedAt, clientRequestId: message.clientRequestId, error: message.error };
}

function projectTool(call: CanonicalToolCall, store: CanonicalConversationStore, structured: CanonicalStructuredActivity): ConversationTimelineItemViewModel {
  const toolCall = projectToolCall(call, store, structured);
  return { id: `toolCall:${call.id}`, kind: 'tool_call', sessionId: call.sessionId, turnId: call.turnId, messageId: call.messageId, toolCallId: call.id, status: call.status, createdAt: call.createdAt, updatedAt: call.updatedAt, error: toolCall.error, toolCall };
}

function projectToolGroup(key: string, calls: CanonicalToolCall[], store: CanonicalConversationStore, structured: CanonicalStructuredActivity): ConversationTimelineItemViewModel {
  return { id: key, kind: 'tool_group', sessionId: calls[0]?.sessionId, turnId: calls[0]?.turnId, status: calls.some((call) => isFailure(call.status)) ? 'failed' : calls.some((call) => call.status === 'running' || call.status === 'queued') ? 'running' : 'completed', toolCalls: calls.map((call) => projectToolCall(call, store, structured)), createdAt: calls[0]?.createdAt, updatedAt: Math.max(...calls.map((call) => call.updatedAt)) };
}

function projectToolCall(call: CanonicalToolCall, store: CanonicalConversationStore, structured: CanonicalStructuredActivity): ToolCallViewModel {
  const results = (call.resultIds ?? []).map((id) => store.toolResultsById[id]).filter(Boolean).sort((left, right) => left.ordinal - right.ordinal);
  const output = results.map((result) => result.contentPreview).filter(Boolean).join('\n');
  const error = call.error || results.map((result) => result.errorPreview).find(Boolean);
  return { id: call.id, sessionId: call.sessionId, turnId: call.turnId ?? '', name: call.name, source: call.source, command: call.command, risk: call.risk, status: call.status, kind: call.kind, inputSummary: call.inputJson, outputSummary: output, error, exitCode: call.exitCode, outputRefs: results.flatMap((result) => result.outputRefs ?? []), artifactRefs: results.flatMap((result) => result.artifactRefs ?? []), diffRefs: results.flatMap((result) => result.diffRefs ?? []), startedAt: call.startedAt, finishedAt: call.finishedAt, parentToolCallId: call.parentToolCallId, agentTask: structured.agentTasks.find((task) => task.parentToolCallId === call.id), groupKey: canonicalToolGroupKey(call), display: { kind: call.kind, targets: call.targets, workingDir: call.workingDir, command: call.command, exitCode: call.exitCode, outputExcerpt: output, failureReason: error } };
}

function isFailure(status?: string) { return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted'; }
