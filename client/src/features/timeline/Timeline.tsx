import { useEffect, useState } from 'react';
import { BranchesOutlined, CheckOutlined, CopyOutlined, DownOutlined, WarningOutlined } from '@ant-design/icons';
import { Button, Collapse, Progress, Tag, Tooltip, message } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { ConversationTimelineItemViewModel, ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import { ThinkingItem } from './ThinkingItem.tsx';
import { ToolCallCard } from '../tools/ToolCallCard.tsx';
import styles from './Timeline.module.css';

interface TimelineProps {
  items: ConversationTimelineItemViewModel[];
}

type RenderTimelineItem = ConversationTimelineItemViewModel | ToolCallGroupRenderItem;

interface ToolCallGroupRenderItem {
  id: string;
  kind: 'tool_call_group';
  turnId?: string;
  toolCalls: ToolCallViewModel[];
}

interface TimelineTurnBlock {
  id: string;
  turnId?: string;
  userMessage?: ConversationTimelineItemViewModel;
  processItems: ConversationTimelineItemViewModel[];
  finalMessage?: ConversationTimelineItemViewModel;
  looseItems: ConversationTimelineItemViewModel[];
  status?: string;
  startedAt?: number;
  finishedAt?: number;
}

export function Timeline({ items }: TimelineProps) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const blocks = buildTurnBlocks(items);

  return (
    <div className={styles.timeline} data-testid="conversation-timeline">
      {messageContextHolder}
      {blocks.map((block) => (
        <TurnBlock key={block.id} block={block} messageApi={messageApi} />
      ))}
    </div>
  );
}

function TurnBlock({ block, messageApi }: { block: TimelineTurnBlock; messageApi: ReturnType<typeof message.useMessage>[0] }) {
  return (
    <section className={styles.turnBlock} data-testid="timeline-turn-block" data-turn-id={block.turnId}>
      {block.userMessage && <TimelineMessage item={block.userMessage} messageApi={messageApi} />}
      {block.processItems.length > 0 && <ProcessTrace block={block} />}
      {block.finalMessage && <TimelineMessage item={block.finalMessage} messageApi={messageApi} />}
      {block.looseItems.map((item) => (
        <TimelineProcessItem key={item.id} item={item} />
      ))}
    </section>
  );
}

function ProcessTrace({ block }: { block: TimelineTurnBlock }) {
  const traceLabel = processTraceSummary(block);
  const groupedItems = groupAdjacentToolCalls(block.processItems);
  return (
    <section className={styles.processTrace} data-testid="process-trace" data-process-label={traceLabel} data-process-status={block.status}>
      <Collapse
        ghost
        size="small"
        defaultActiveKey={shouldOpenProcessTrace(block) ? ['trace'] : []}
        expandIcon={({ isActive }) => <DownOutlined rotate={isActive ? 180 : 0} />}
        items={[
          {
            key: 'trace',
            label: <ProcessTraceLabel block={block} />,
            children: (
              <div className={styles.processSteps}>
                {groupedItems.map((item) => (
                  <TimelineProcessItem key={item.id} item={item} />
                ))}
              </div>
            ),
          },
        ]}
      />
    </section>
  );
}

function ProcessTraceLabel({ block }: { block: TimelineTurnBlock }) {
  const { duration, label, toolCount } = processTraceSummaryParts(block);
  return (
    <span className={styles.processTraceLabel}>
      <span>{label}</span>
      {duration && <span>{duration}</span>}
      {toolCount > 0 && <span>{toolCount} 个工具</span>}
    </span>
  );
}

function TimelineProcessItem({ item }: { item: RenderTimelineItem }) {
  if (item.kind === 'tool_call_group') {
    return <ToolCallCard toolCalls={item.toolCalls} />;
  }
  if (item.kind === 'tool_call' && item.toolCall) {
    return <ToolCallCard toolCall={item.toolCall} />;
  }
  if (item.kind === 'permission' && item.permission) {
    return <PermissionTraceRow item={item} />;
  }
  if (item.kind === 'thinking') {
    return <ThinkingItem item={item} />;
  }
  if (item.kind === 'progress') {
    const detail = progressDetail(item);
    return (
      <div className={styles.progress} data-testid="turn-progress" data-progress-status={item.status}>
        <div>{progressLabel(item.status)}</div>
        {detail && <div className={styles.progressDetail}>{detail}</div>}
      </div>
    );
  }
  if (item.kind === 'turn_terminal') {
    return null;
  }
  if (item.kind === 'diagnostic') {
    return <TurnDiagnosticWarning item={item} />;
  }
  if (item.kind === 'agent_task' && item.agentTask) {
    return <AgentTaskTimelineRow item={item} />;
  }
  if (item.kind === 'message') {
    return <AssistantProcessNote item={item} />;
  }
  return null;
}

function TimelineMessage({ item, messageApi }: { item: ConversationTimelineItemViewModel; messageApi: ReturnType<typeof message.useMessage>[0] }) {
  return (
    <Bubble
      className={item.role === 'user' ? styles.userBubble : styles.assistantBubble}
      content={item.content}
      placement={item.role === 'user' ? 'end' : 'start'}
      variant={item.role === 'user' ? 'filled' : 'borderless'}
      footer={
        (item.role === 'user' || item.role === 'assistant') && isCompleteMessage(item) ? (
          <MessageFooter align={item.role === 'user' ? 'end' : 'start'} content={item.content ?? ''} createdAt={item.createdAt} messageApi={messageApi} />
        ) : item.status === 'error' ? (
          <Tag color="error">失败</Tag>
        ) : undefined
      }
    />
  );
}

function AssistantProcessNote({ item }: { item: ConversationTimelineItemViewModel }) {
  const content = item.content?.trim();
  if (!content) {
    return null;
  }
  return (
    <details className={styles.processNote} data-testid="timeline-process-note">
      <summary>{summarizeProcessNote(content)}</summary>
      <div>{content}</div>
    </details>
  );
}

function PermissionTraceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const permission = item.permission;
  if (!permission) {
    return null;
  }
  return (
    <div className={styles.permissionTraceRow} data-testid="permission-trace-row" data-permission-status={permission.status}>
      <span>{permissionStatusLabel(permission.status)}</span>
      <code>{permission.target || permission.path || permission.toolName}</code>
      {permission.reason || permission.policyReason ? <span>{permission.reason || permission.policyReason}</span> : null}
    </div>
  );
}

function AgentTaskTimelineRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const task = item.agentTask;
  if (!task) {
    return null;
  }
  const refs = [...(task.outputRefs ?? []), ...(task.artifactRefs ?? [])];
  const summary = task.resultSummary || task.promptSummary || '';
  return (
    <div className={styles.agentTaskRow} data-testid="timeline-agent-task-row" data-task-id={task.id}>
      <div className={styles.agentTaskIcon}>
        <BranchesOutlined />
      </div>
      <div className={styles.agentTaskBody}>
        <div className={styles.agentTaskHeader}>
          <span>{task.title || task.id}</span>
          <Tag color={agentTaskStatusColor(task.status)}>{task.status}</Tag>
        </div>
        <Progress percent={task.progress ?? 0} size="small" showInfo={false} />
        <div className={styles.agentTaskMeta}>
          {[task.role || task.kind, task.provider && task.model ? `${task.provider}/${task.model}` : task.model, task.childSessionId ? `child ${task.childSessionId}` : undefined]
            .filter(Boolean)
            .join(' / ')}
        </div>
        {summary ? (
          <details className={styles.agentTaskSummary}>
            <summary>{summarizeProcessNote(summary)}</summary>
            <div>{summary}</div>
          </details>
        ) : null}
        {refs.length ? <div className={styles.agentTaskRefs}>{refs.slice(0, 3).join(' / ')}</div> : null}
      </div>
    </div>
  );
}

function TurnDiagnosticWarning({ item }: { item: ConversationTimelineItemViewModel }) {
  const missingArtifacts = item.diagnostics?.missingArtifacts ?? [];
  if (!item.diagnostics?.warning || missingArtifacts.length === 0) {
    return null;
  }
  return (
    <div className={styles.diagnosticWarning} data-testid="turn-diagnostic-warning">
      <WarningOutlined className={styles.diagnosticIcon} />
      <div className={styles.diagnosticBody}>
        <div className={styles.diagnosticTitle}>{diagnosticWarningTitle(item)}</div>
        <div className={styles.diagnosticContent}>{formatMissingArtifacts(missingArtifacts)}</div>
      </div>
    </div>
  );
}

function buildTurnBlocks(items: ConversationTimelineItemViewModel[]): TimelineTurnBlock[] {
  const blocks: TimelineTurnBlock[] = [];
  const blockByTurn = new Map<string, TimelineTurnBlock>();

  const ensureBlock = (turnId?: string, fallbackId?: string) => {
    const id = turnId || fallbackId || `loose:${blocks.length}`;
    if (turnId && blockByTurn.has(turnId)) {
      return blockByTurn.get(turnId)!;
    }
    const block: TimelineTurnBlock = {
      id: turnId ? `turn:${turnId}` : id,
      turnId,
      processItems: [],
      looseItems: [],
    };
    blocks.push(block);
    if (turnId) {
      blockByTurn.set(turnId, block);
    }
    return block;
  };

  for (const item of items) {
    const block = ensureBlock(item.turnId, item.id);
    block.startedAt = minDefined(block.startedAt, item.createdAt);
    block.finishedAt = maxDefined(block.finishedAt, item.updatedAt || item.createdAt);
    block.status = mergeTurnStatus(block.status, item.status);

    if (item.kind === 'message' && item.role === 'user') {
      block.userMessage = block.userMessage ?? item;
      continue;
    }
    if (item.kind === 'message' && item.role === 'assistant') {
      if (isFinalAssistantMessage(item)) {
        block.finalMessage = item;
      } else {
        block.processItems.push(item);
      }
      continue;
    }
    if (isProcessItem(item)) {
      block.processItems.push(item);
      continue;
    }
    block.looseItems.push(item);
  }

  for (const block of blocks) {
    if (block.finalMessage && !isActiveTurnStatus(block.status)) {
      block.status = block.finalMessage.status === 'error' ? 'failed' : 'completed';
    }
  }

  return blocks.filter((block) => block.userMessage || block.finalMessage || block.processItems.length > 0 || block.looseItems.length > 0);
}

function isProcessItem(item: ConversationTimelineItemViewModel) {
  return item.kind === 'thinking' || item.kind === 'tool_call' || item.kind === 'permission' || item.kind === 'progress' || item.kind === 'diagnostic' || item.kind === 'agent_task' || item.kind === 'turn_terminal';
}

function isFinalAssistantMessage(item: ConversationTimelineItemViewModel) {
  return item.role === 'assistant' && item.kind === 'message' && (item.content?.trim() || item.status === 'error');
}

function shouldOpenProcessTrace(block: TimelineTurnBlock) {
  if (!block.finalMessage) {
    return true;
  }
  return block.processItems.some((item) => isActiveTurnStatus(item.status));
}

function isActiveTurnStatus(status?: string) {
  return status === 'running' || status === 'queued' || status === 'waiting_permission';
}

function processTraceLabel(status?: string) {
  switch (status) {
    case 'running':
    case 'queued':
    case 'waiting_permission':
      return '处理中';
    case 'failed':
      return '处理失败';
    case 'denied':
      return '已拒绝';
    case 'cancelled':
      return '已取消';
    default:
      return '已处理';
  }
}

function processTraceSummary(block: TimelineTurnBlock) {
  const { duration, label, toolCount } = processTraceSummaryParts(block);
  return [label, duration, toolCount > 0 ? `${toolCount} 个工具` : undefined, block.turnId].filter(Boolean).join(' ');
}

function processTraceSummaryParts(block: TimelineTurnBlock) {
  return {
    duration: blockDuration(block),
    label: processTraceLabel(block.status),
    toolCount: block.processItems.filter((item) => item.kind === 'tool_call').length,
  };
}

function blockDuration(block: TimelineTurnBlock) {
  if (!block.startedAt || !block.finishedAt) {
    return '';
  }
  return formatDuration(block.startedAt, block.finishedAt);
}

function mergeTurnStatus(current?: string, next?: string) {
  if (!next) {
    return current;
  }
  const rank: Record<string, number> = {
    denied: 5,
    failed: 5,
    waiting_permission: 4,
    running: 3,
    queued: 2,
    cancelled: 1,
    success: 0,
    completed: 0,
  };
  if (!current) {
    return next;
  }
  return (rank[next] ?? 0) > (rank[current] ?? 0) ? next : current;
}

function groupAdjacentToolCalls(items: ConversationTimelineItemViewModel[]): RenderTimelineItem[] {
  const grouped: RenderTimelineItem[] = [];
  let pending: ConversationTimelineItemViewModel[] = [];

  const flush = () => {
    if (pending.length === 0) {
      return;
    }
    if (pending.length === 1) {
      grouped.push(pending[0]);
      pending = [];
      return;
    }
    grouped.push({
      id: `tool-group:${pending.map((item) => item.toolCallId || item.id).join(':')}`,
      kind: 'tool_call_group',
      turnId: pending[0].turnId,
      toolCalls: pending.map((item) => item.toolCall).filter((toolCall): toolCall is ToolCallViewModel => Boolean(toolCall)),
    });
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

function timelineToolKind(toolCall?: ToolCallViewModel) {
  if (!toolCall) {
    return 'generic';
  }
  if (toolCall.display?.kind) {
    return toolCall.display.kind;
  }
  const name = toolCall.name.toLowerCase();
  const summary = `${toolCall.inputSummary ?? ''} ${toolCall.command ?? ''}`.toLowerCase();
  const source = toolCall.source?.toLowerCase() ?? '';
  const shellNames = new Set(['bash', 'cmd', 'command', 'go', 'npm', 'node', 'python', 'powershell', 'pwsh', 'shell']);
  if (toolCall.command || toolCall.risk === 'execute' || source.includes('shell') || shellNames.has(name) || name.includes('command')) {
    return 'shell';
  }
  if (name.includes('edit') || name.includes('patch') || summary.includes('apply_patch')) {
    return 'file_edit';
  }
  if (name.includes('write') || name.includes('create') || summary.includes('write')) {
    return 'file_write';
  }
  if (name.includes('read') || name.includes('view') || name.includes('open') || summary.includes('read')) {
    return 'file_read';
  }
  if (isSearchToolName(name) || summary.includes('glob') || summary.includes('grep') || summary.includes('search')) {
    return 'file_search';
  }
  return 'generic';
}

function isSearchToolName(name: string) {
  return name === 'glob' || name === 'grep' || name === 'list' || name === 'ls' || name === 'dir' || name.includes('search') || name.includes('find');
}

function agentTaskStatusColor(status?: string) {
  switch (status) {
    case 'queued':
    case 'running':
      return 'processing';
    case 'completed':
      return 'success';
    case 'failed':
    case 'interrupted':
      return 'error';
    default:
      return 'default';
  }
}

function diagnosticWarningTitle(item: ConversationTimelineItemViewModel) {
  switch (item.diagnostics?.warningReason) {
    case 'produced_artifact_missing_on_disk':
      return '工具已报告生成文件，但磁盘上不存在';
    case 'expected_artifact_not_produced':
      return '期望文件未由工具生成';
    default:
      return item.diagnostics?.warning ?? '产物警告';
  }
}

function isCompleteMessage(item: ConversationTimelineItemViewModel) {
  return item.status === 'success' || item.status === 'error';
}

function MessageFooter({
  align,
  content,
  createdAt,
  messageApi,
}: {
  align: 'start' | 'end';
  content: string;
  createdAt?: number;
  messageApi: ReturnType<typeof message.useMessage>[0];
}) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) {
      return undefined;
    }
    const timer = window.setTimeout(() => setCopied(false), 1200);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const copyMessage = async () => {
    try {
      await copyText(content);
      setCopied(true);
      void messageApi.success('已复制');
    } catch {
      void messageApi.error('复制失败');
    }
  };

  return (
    <div className={`${styles.messageFooter} ${align === 'end' ? styles.userMessageFooter : styles.assistantMessageFooter}`}>
      <span className={styles.messageTime}>{formatMessageTime(createdAt)}</span>
      <Tooltip title={copied ? '已复制' : '复制'}>
        <Button
          aria-label={copied ? '已复制' : '复制消息'}
          className={styles.copyButton}
          icon={copied ? <CheckOutlined /> : <CopyOutlined />}
          size="small"
          type="text"
          onClick={copyMessage}
        />
      </Tooltip>
    </div>
  );
}

async function copyText(text: string) {
  if (typeof document !== 'undefined' && copyTextWithSelection(text)) {
    return;
  }
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await withTimeout(navigator.clipboard.writeText(text), 300);
      return;
    } catch {
      // Embedded webviews can expose Clipboard API but reject writes.
    }
  }
  if (typeof document === 'undefined') {
    throw new Error('clipboard is unavailable');
  }
  copyTextWithSelection(text);
}

function copyTextWithSelection(text: string) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-1000px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);
  try {
    return document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number) {
  return new Promise<T>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('clipboard write timed out')), timeoutMs);
    promise.then(
      (value) => {
        window.clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        window.clearTimeout(timer);
        reject(error);
      },
    );
  });
}

function formatMessageTime(createdAt?: number) {
  if (!createdAt) {
    return '';
  }
  const milliseconds = normalizeTimestamp(createdAt);
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(milliseconds));
}

function progressLabel(status?: string) {
  switch (status) {
    case 'waiting_permission':
      return '等待权限确认';
    case 'running':
    case 'queued':
      return '正在处理';
    case 'cancelled':
      return '已取消';
    case 'failed':
      return '执行失败';
    default:
      return status || '正在处理';
  }
}

function permissionStatusLabel(status?: string) {
  switch (status) {
    case 'pending':
      return '等待权限确认';
    case 'allowed':
    case 'allowed_once':
      return '已允许';
    case 'allowed_session':
      return '本会话已允许';
    case 'denied':
      return '已拒绝';
    case 'cancelled':
      return '已取消';
    case 'expired':
      return '已过期';
    default:
      return status || '权限记录';
  }
}

function progressDetail(item: ConversationTimelineItemViewModel) {
  if (item.status !== 'failed' && item.status !== 'cancelled') {
    return '';
  }
  return item.error || item.summary || '';
}

function formatMissingArtifacts(paths: string[]) {
  if (paths.length === 1) {
    return paths[0];
  }
  return `${paths.length} 个文件缺失：${paths.join('、')}`;
}

function formatDuration(startedAt: number, finishedAt: number) {
  const elapsed = Math.max(0, normalizeTimestamp(finishedAt) - normalizeTimestamp(startedAt));
  if (elapsed < 1000) {
    return '<1s';
  }
  if (elapsed < 60_000) {
    return `${Math.round(elapsed / 1000)}s`;
  }
  return `${Math.floor(elapsed / 60_000)}m ${Math.round((elapsed % 60_000) / 1000)}s`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function minDefined(left?: number, right?: number) {
  if (left === undefined) {
    return right;
  }
  if (right === undefined) {
    return left;
  }
  return Math.min(left, right);
}

function maxDefined(left?: number, right?: number) {
  if (left === undefined) {
    return right;
  }
  if (right === undefined) {
    return left;
  }
  return Math.max(left, right);
}

function summarizeProcessNote(content: string) {
  const normalized = content.replace(/\s+/g, ' ').trim();
  if (normalized.length <= 140) {
    return normalized;
  }
  return `${normalized.slice(0, 140)}...`;
}
