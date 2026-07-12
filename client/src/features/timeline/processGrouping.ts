import type { ConversationTimelineItemViewModel, ToolCallViewModel } from '../../runtime/workbenchTypes.ts';

export type RenderTimelineItem = ConversationTimelineItemViewModel | ToolCallGroupRenderItem;
export interface ToolCallGroupRenderItem { id: string; kind: 'tool_call_group'; turnId?: string; toolCalls: ToolCallViewModel[] }

/** Canonical grouping already happened before this render boundary. */
export function compactProcessItems(items: ConversationTimelineItemViewModel[]): RenderTimelineItem[] {
  return items.filter(isVisibleProcessItem).map((item) => item.kind === 'tool_group' && item.toolCalls
    ? { id: item.id, kind: 'tool_call_group' as const, turnId: item.turnId, toolCalls: item.toolCalls }
    : item);
}

// Tool-use assistant messages often carry structure but no visible text. The
// renderer returns null for them; filtering before ProcessDisclosure maps the
// list prevents their empty wrappers from accumulating flex gaps between real
// tool rows.
function isVisibleProcessItem(item: ConversationTimelineItemViewModel) {
  const narration = item.kind === 'thinking' || item.kind === 'assistant_thinking' || item.kind === 'message' || item.kind === 'assistant_message';
  if (!narration) return true;
  return Boolean(item.content?.trim() || (item.source === 'react_callchain' && item.title));
}

export function toolCallsDuration(toolCalls: ToolCallViewModel[]) {
  const starts = toolCalls.map((call) => call.startedAt).filter((value): value is number => value !== undefined);
  const finishes = toolCalls.map((call) => call.finishedAt).filter((value): value is number => value !== undefined);
  if (!starts.length || !finishes.length) return '';
  const elapsed = Math.max(0, normalizeTimestamp(Math.max(...finishes)) - normalizeTimestamp(Math.min(...starts)));
  if (elapsed < 1000) return '<1s';
  if (elapsed < 60_000) return `${Math.round(elapsed / 1000)}s`;
  return `${Math.floor(elapsed / 60_000)}m ${Math.round((elapsed % 60_000) / 1000)}s`;
}

export function timelineToolKind(toolCall?: ToolCallViewModel) { return toolCall?.display?.kind || toolCall?.kind || 'generic'; }
function normalizeTimestamp(value: number) { return value < 1_000_000_000_000 ? value * 1000 : value; }
