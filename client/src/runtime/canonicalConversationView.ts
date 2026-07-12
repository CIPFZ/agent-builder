import { selectCanonicalConversationTurns, type CanonicalProcessEntity } from './canonicalConversationSelectors.ts';
import { canonicalToolGroupKey } from './canonicalConversationPresentation.ts';
import { groupCanonicalProcess } from './canonicalConversationPresentation.ts';
import type { CanonicalConversationStore } from './canonicalConversationStore.ts';
import type { CanonicalMessage, CanonicalToolCall } from './canonicalConversationTypes.ts';
import type { ConversationTurnViewModel, ConversationTurnStatus } from './conversation/conversationTypes.ts';
import type { ConversationTimelineItemViewModel, ToolCallViewModel } from './workbenchTypes.ts';

export function selectCanonicalConversationTurnViewModels(store?: CanonicalConversationStore): ConversationTurnViewModel[] {
  return selectCanonicalConversationTurns(store).map(({ turn, user, final, process }) => {
    const status = turn.status as ConversationTurnStatus;
    const items = groupCanonicalProcess(process).map((item) => item.type === 'tool-group'
      ? projectToolGroup(item.key, item.tools, store!)
      : projectProcessEntity(item.entity, store!)).filter((item): item is ConversationTimelineItemViewModel => Boolean(item));
    return {
      id: turn.id, sessionId: turn.sessionId, status,
      user: user ? projectMessage(user) : undefined,
      final: final ? projectMessage(final) : undefined,
      process: { status, items, startedAt: turn.startedAt, finishedAt: turn.finishedAt, hasFailure: status === 'failed' || items.some((item) => isFailure(item.status)) },
      startedAt: turn.startedAt, finishedAt: turn.finishedAt, error: turn.error,
    };
  });
}

function projectProcessEntity(entity: CanonicalProcessEntity, store: CanonicalConversationStore): ConversationTimelineItemViewModel | undefined {
  if ('role' in entity) return projectMessage(entity);
  if ('name' in entity && 'source' in entity) return projectTool(entity, store);
  if ('kind' in entity && !('teamId' in entity)) return { id: `notice:${entity.id}`, kind: 'progress', sessionId: entity.sessionId, turnId: entity.turnId, title: entity.kind, content: entity.summary, status: entity.status, createdAt: entity.createdAt, updatedAt: entity.updatedAt };
  return undefined;
}

function projectMessage(message: CanonicalMessage): ConversationTimelineItemViewModel {
  return { id: `message:${message.id}`, kind: message.role === 'user' ? 'user_message' : message.phase === 'reasoning' ? 'assistant_thinking' : 'assistant_message', sessionId: message.sessionId, turnId: message.turnId, messageId: message.id, role: message.role as 'user' | 'assistant' | 'tool' | 'system', phase: message.phase === 'final' ? 'final' : 'intermediate', content: message.content, status: message.status, createdAt: message.createdAt, updatedAt: message.updatedAt, clientRequestId: message.clientRequestId, error: message.error };
}

function projectTool(call: CanonicalToolCall, store: CanonicalConversationStore): ConversationTimelineItemViewModel {
  const toolCall = projectToolCall(call, store);
  return { id: `toolCall:${call.id}`, kind: 'tool_call', sessionId: call.sessionId, turnId: call.turnId, messageId: call.messageId, toolCallId: call.id, status: call.status, createdAt: call.createdAt, updatedAt: call.updatedAt, error: toolCall.error, toolCall };
}

function projectToolGroup(key: string, calls: CanonicalToolCall[], store: CanonicalConversationStore): ConversationTimelineItemViewModel {
  return { id: key, kind: 'tool_group', sessionId: calls[0]?.sessionId, turnId: calls[0]?.turnId, status: calls.some((call) => isFailure(call.status)) ? 'failed' : calls.some((call) => call.status === 'running' || call.status === 'queued') ? 'running' : 'completed', toolCalls: calls.map((call) => projectToolCall(call, store)), createdAt: calls[0]?.createdAt, updatedAt: Math.max(...calls.map((call) => call.updatedAt)) };
}

function projectToolCall(call: CanonicalToolCall, store: CanonicalConversationStore): ToolCallViewModel {
  const results = (call.resultIds ?? []).map((id) => store.toolResultsById[id]).filter(Boolean).sort((left, right) => left.ordinal - right.ordinal);
  const output = results.map((result) => result.contentPreview).filter(Boolean).join('\n');
  const error = call.error || results.map((result) => result.errorPreview).find(Boolean);
  return { id: call.id, sessionId: call.sessionId, turnId: call.turnId ?? '', name: call.name, source: call.source, command: call.command, risk: call.risk, status: call.status, kind: call.kind, inputSummary: call.inputJson, outputSummary: output, error, exitCode: call.exitCode, outputRefs: results.flatMap((result) => result.outputRefs ?? []), artifactRefs: results.flatMap((result) => result.artifactRefs ?? []), diffRefs: results.flatMap((result) => result.diffRefs ?? []), startedAt: call.startedAt, finishedAt: call.finishedAt, parentToolCallId: call.parentToolCallId, groupKey: canonicalToolGroupKey(call), display: { kind: call.kind, targets: call.targets, workingDir: call.workingDir, command: call.command, exitCode: call.exitCode, outputExcerpt: output, failureReason: error } };
}

function isFailure(status?: string) { return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted'; }
