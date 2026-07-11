import type { ConversationTimelineItemViewModel, ToolCallViewModel } from '../../runtime/workbenchTypes.ts';

export type RenderTimelineItem = ConversationTimelineItemViewModel | ToolCallGroupRenderItem | ToolCallSummaryRenderItem;

export interface ToolCallGroupRenderItem {
  id: string;
  kind: 'tool_call_group';
  turnId?: string;
  toolCalls: ToolCallViewModel[];
}

export interface ToolCallSummaryRenderItem {
  id: string;
  kind: 'tool_call_summary';
  turnId?: string;
  toolCalls: ToolCallViewModel[];
}

export function compactProcessItems(items: ConversationTimelineItemViewModel[]): RenderTimelineItem[] {
  const compacted: RenderTimelineItem[] = [];
  let quietSummary: ToolCallSummaryRenderItem | undefined;
  let quietSummaryRoundKey: string | undefined;

  for (const item of items) {
    if (item.kind === 'tool_call' && item.toolCall && isQuietCompletedToolCall(item.toolCall)) {
      const roundKey = toolCallRoundKey(item);
      if (!quietSummary || quietSummaryRoundKey !== roundKey) {
        quietSummary = {
          id: `tool-summary:${roundKey}`,
          kind: 'tool_call_summary',
          turnId: item.turnId,
          toolCalls: [],
        };
        quietSummaryRoundKey = roundKey;
        compacted.push(quietSummary);
      }
      quietSummary.toolCalls.push(item.toolCall);
      continue;
    }
    quietSummary = undefined;
    quietSummaryRoundKey = undefined;
    compacted.push(item);
  }

  return groupAdjacentToolCalls(compacted);
}

export function toolCallsDuration(toolCalls: ToolCallViewModel[]) {
  const startedAt = toolCalls.reduce<number | undefined>((current, call) => minDefined(current, call.startedAt), undefined);
  const finishedAt = toolCalls.reduce<number | undefined>((current, call) => maxDefined(current, call.finishedAt), undefined);
  return startedAt && finishedAt ? formatDuration(startedAt, finishedAt) : '';
}

export function timelineToolKind(toolCall?: ToolCallViewModel) {
  return toolCall?.display?.kind || toolCall?.kind || 'generic';
}

function groupAdjacentToolCalls(items: RenderTimelineItem[]): RenderTimelineItem[] {
  const grouped: RenderTimelineItem[] = [];
  let pending: ConversationTimelineItemViewModel[] = [];
  const flush = () => {
    if (pending.length === 1) grouped.push(pending[0]);
    if (pending.length > 1) {
      grouped.push({
        id: `tool-group:${pending.map((item) => item.toolCallId || item.id).join(':')}`,
        kind: 'tool_call_group',
        turnId: pending[0].turnId,
        toolCalls: pending.map((item) => item.toolCall).filter((call): call is ToolCallViewModel => Boolean(call)),
      });
    }
    pending = [];
  };

  for (const item of items) {
    if (item.kind === 'tool_call' && item.toolCall) {
      const previous = pending[pending.length - 1];
      if (!previous || (previous.turnId === item.turnId && timelineToolKind(previous.toolCall) === timelineToolKind(item.toolCall) && previous.toolCall?.status === item.toolCall.status)) {
        pending.push(item);
        continue;
      }
    }
    flush();
    grouped.push(item);
  }
  flush();
  return grouped;
}

function isQuietCompletedToolCall(toolCall: ToolCallViewModel) {
  return toolCall.quiet === true && (toolCall.status === 'completed' || toolCall.status === 'success') && !toolCall.error;
}

function toolCallRoundKey(item: ConversationTimelineItemViewModel) {
  return item.messageId || item.toolCall?.id || item.toolCallId || item.id;
}

function formatDuration(startedAt: number, finishedAt: number) {
  const elapsed = Math.max(0, normalizeTimestamp(finishedAt) - normalizeTimestamp(startedAt));
  if (elapsed < 1000) return '<1s';
  if (elapsed < 60_000) return `${Math.round(elapsed / 1000)}s`;
  return `${Math.floor(elapsed / 60_000)}m ${Math.round((elapsed % 60_000) / 1000)}s`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function minDefined(left?: number, right?: number) {
  if (left === undefined) return right;
  if (right === undefined) return left;
  return Math.min(left, right);
}

function maxDefined(left?: number, right?: number) {
  if (left === undefined) return right;
  if (right === undefined) return left;
  return Math.max(left, right);
}
