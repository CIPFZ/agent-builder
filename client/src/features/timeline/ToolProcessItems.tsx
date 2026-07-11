import { CheckOutlined, LoadingOutlined, MessageOutlined, WarningOutlined } from '@ant-design/icons';
import type { ConversationTimelineItemViewModel, ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import { MarkdownMessage } from '../markdown/MarkdownMessage.tsx';
import { QuietToolRowList } from '../tools/ToolCallCard.tsx';
import { TraceRow } from './TraceRow.tsx';
import { timelineToolKind, toolCallsDuration } from './processGrouping.ts';

export function ToolTraceGroup({ onAgentTaskOpen, toolCalls }: { onAgentTaskOpen?: (taskID: string) => void; toolCalls: ToolCallViewModel[] }) {
  const duration = toolCallsDuration(toolCalls);
  const status = toolTraceStatus(toolCalls);
  const failed = isFailedStatus(status);
  const running = isActiveStatus(status);
  return <TraceRow expandable icon={failed ? <WarningOutlined /> : running ? <LoadingOutlined spin /> : <CheckOutlined />} meta={<>{duration && <span>{duration}</span>}{toolCalls.length > 1 && <span>{toolCalls.length} items</span>}</>} testId="tool-run-summary" title={toolTraceTitle(toolCalls)} tone={failed ? 'error' : 'default'}><QuietToolRowList toolCalls={toolCalls} onAgentTaskOpen={onAgentTaskOpen} /></TraceRow>;
}

export function AssistantProcessNote({ item }: { item: ConversationTimelineItemViewModel }) {
  const content = item.content?.trim();
  if (!content) return null;
  return <TraceRow expandable icon={<MessageOutlined />} testId="timeline-process-note" title={summarizeProcessNote(content)}><MarkdownMessage content={content} role="assistant" /></TraceRow>;
}

function toolTraceStatus(toolCalls: ToolCallViewModel[]) {
  if (toolCalls.some((call) => isActiveStatus(call.status))) return 'running';
  if (toolCalls.some((call) => isFailedStatus(call.status))) return 'failed';
  if (toolCalls.some((call) => call.status === 'denied')) return 'denied';
  return 'completed';
}

function toolTraceTitle(toolCalls: ToolCallViewModel[]) {
  const kind = dominantToolKind(toolCalls);
  const count = toolCalls.length;
  const prefix = toolTraceStatus(toolCalls) === 'running' ? '正在' : '已';
  if (kind === 'shell') return `${prefix}运行 ${count} 条命令`;
  if (kind === 'file_read') return `${prefix}读取 ${count} 个文件`;
  if (kind === 'file_search') return `${prefix}搜索 ${count} 次`;
  if (kind === 'file_write') return `${prefix}写入 ${count} 个文件`;
  if (kind === 'file_edit') return `${prefix}编辑 ${count} 个文件`;
  return `${prefix}处理 ${count} 个工具调用`;
}

function dominantToolKind(toolCalls: ToolCallViewModel[]) {
  const counts = new Map<string, number>();
  for (const call of toolCalls) {
    const kind = timelineToolKind(call);
    counts.set(kind, (counts.get(kind) ?? 0) + 1);
  }
  return [...counts.entries()].sort((left, right) => right[1] - left[1])[0]?.[0] ?? 'generic';
}

function isActiveStatus(status?: string) { return status === 'running' || status === 'queued' || status === 'waiting_permission'; }
function isFailedStatus(status?: string) { return status === 'failed' || status === 'denied' || status === 'cancelled' || status === 'interrupted'; }
function summarizeProcessNote(content: string) { const normalized = content.replace(/\s+/g, ' ').trim(); return normalized.length <= 140 ? normalized : `${normalized.slice(0, 140)}...`; }
