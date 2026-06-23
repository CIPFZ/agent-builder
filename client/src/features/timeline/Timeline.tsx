import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { BranchesOutlined, CheckOutlined, CopyOutlined, WarningOutlined } from '@ant-design/icons';
import { Button, Progress, Tag, Tooltip, message } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { ConversationTimelineItemViewModel, PermissionRequestViewModel, ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import { PermissionGate } from '../permissions/PermissionGate.tsx';
import { ThinkingItem } from './ThinkingItem.tsx';
import { ToolCallCard } from '../tools/ToolCallCard.tsx';
import styles from './Timeline.module.css';

interface TimelineProps {
  items: ConversationTimelineItemViewModel[];
  onPermissionDecide: (permissionID: string, action: 'allow' | 'allow_session' | 'deny') => Promise<void>;
}

type RenderTimelineItem = ToolCallRenderItem | ToolCallGroupRenderItem;

interface ToolCallRenderItem extends ConversationTimelineItemViewModel {
  pendingPermissions?: PermissionRequestViewModel[];
}

interface ToolCallGroupRenderItem {
  id: string;
  kind: 'tool_call_group';
  turnId?: string;
  toolCalls: ToolCallViewModel[];
  pendingPermissions?: PermissionRequestViewModel[];
}

export function Timeline({ items, onPermissionDecide }: TimelineProps) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const renderItems = attachPendingPermissions(groupAdjacentToolCalls(items));

  return (
    <div className={styles.timeline} data-testid="conversation-timeline">
      {messageContextHolder}
      {renderItems.map((item) => {
        if (item.kind === 'tool_call_group') {
          return (
            <ToolProcessCluster key={item.id} pendingPermissions={item.pendingPermissions} onPermissionDecide={onPermissionDecide}>
              <ToolCallCard toolCalls={item.toolCalls} />
            </ToolProcessCluster>
          );
        }
        if (item.kind === 'tool_call' && item.toolCall) {
          return (
            <ToolProcessCluster key={item.id} pendingPermissions={item.pendingPermissions} onPermissionDecide={onPermissionDecide}>
              <ToolCallCard toolCall={item.toolCall} />
            </ToolProcessCluster>
          );
        }
        if (item.kind === 'permission' && item.permission) {
          if (item.permission.status !== 'pending') {
            return null;
          }
          return <PermissionGate key={item.id} permission={item.permission} onDecide={onPermissionDecide} />;
        }
        if (item.kind === 'thinking') {
          return <ThinkingItem key={item.id} item={item} />;
        }
        if (item.kind === 'progress') {
          const detail = progressDetail(item);
          return (
            <div key={item.id} className={styles.progress} data-testid="turn-progress" data-progress-status={item.status}>
              <div>{progressLabel(item.status)}</div>
              {detail && <div className={styles.progressDetail}>{detail}</div>}
            </div>
          );
        }
        if (item.kind === 'turn_terminal') {
          return null;
        }
        if (item.kind === 'diagnostic') {
          return <TurnDiagnosticWarning key={item.id} item={item} />;
        }
        if (item.kind === 'agent_task' && item.agentTask) {
          return <AgentTaskTimelineRow key={item.id} item={item} />;
        }
        return (
          <Bubble
            key={item.id}
            className={item.role === 'user' ? styles.userBubble : styles.assistantBubble}
            content={item.content}
            placement={item.role === 'user' ? 'end' : 'start'}
            variant={item.role === 'user' ? 'filled' : 'borderless'}
            footer={
              (item.role === 'user' || item.role === 'assistant') && isCompleteMessage(item) ? (
                <MessageFooter
                  align={item.role === 'user' ? 'end' : 'start'}
                  content={item.content ?? ''}
                  createdAt={item.createdAt}
                  messageApi={messageApi}
                />
              ) : item.status === 'error' ? (
                <Tag color="error">失败</Tag>
              ) : undefined
            }
          />
        );
      })}
    </div>
  );
}

function AgentTaskTimelineRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const task = item.agentTask;
  if (!task) {
    return null;
  }
  const refs = [...(task.outputRefs ?? []), ...(task.artifactRefs ?? [])];
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
            .join(' · ')}
        </div>
        {task.resultSummary || task.promptSummary ? <div className={styles.agentTaskSummary}>{task.resultSummary || task.promptSummary}</div> : null}
        {refs.length ? <div className={styles.agentTaskRefs}>{refs.slice(0, 3).join(' · ')}</div> : null}
      </div>
    </div>
  );
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

function diagnosticWarningTitle(item: ConversationTimelineItemViewModel) {
  switch (item.diagnostics?.warningReason) {
    case 'produced_artifact_missing_on_disk':
      return '工具已报告生成期望文件，但磁盘上不存在';
    case 'expected_artifact_not_produced':
      return '期望文件未由工具生成';
    default:
      return item.diagnostics?.warning ?? '期望产物警告';
  }
}

function ToolProcessCluster({
  children,
  onPermissionDecide,
  pendingPermissions,
}: {
  children: ReactNode;
  onPermissionDecide: TimelineProps['onPermissionDecide'];
  pendingPermissions?: PermissionRequestViewModel[];
}) {
  if (!pendingPermissions?.length) {
    return <>{children}</>;
  }

  return (
    <div className={styles.toolProcessCluster} data-testid="tool-process-cluster">
      {children}
      <div className={styles.embeddedPermissions}>
        {pendingPermissions.map((permission) => (
          <PermissionGate key={permission.id} permission={permission} onDecide={onPermissionDecide} />
        ))}
      </div>
    </div>
  );
}

function attachPendingPermissions(items: RenderTimelineItem[]): RenderTimelineItem[] {
  const attached: RenderTimelineItem[] = [];
  const seenPermissionIDs = new Set<string>();

  for (const item of items) {
    if (item.kind === 'permission' && item.permission?.status === 'pending') {
      if (seenPermissionIDs.has(item.permission.id)) {
        continue;
      }
      seenPermissionIDs.add(item.permission.id);
      const target = [...attached].reverse().find((candidate) => ownsToolCall(candidate, item.permission?.toolCallId));
      if (target) {
        const existing = target.pendingPermissions ?? [];
        target.pendingPermissions = existing.some((permission) => permission.id === item.permission?.id) ? existing : [...existing, item.permission];
        continue;
      }
    }
    attached.push(item);
  }

  return attached;
}

function ownsToolCall(item: RenderTimelineItem, toolCallID?: string) {
  if (!toolCallID) {
    return false;
  }
  if (item.kind === 'tool_call' && item.toolCall?.id === toolCallID) {
    return true;
  }
  if (item.kind === 'tool_call_group' && item.toolCalls.some((toolCall) => toolCall.id === toolCallID)) {
    return true;
  }
  return false;
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
      // Fall back to execCommand for embedded browsers that expose Clipboard API but reject writes.
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
  const milliseconds = createdAt < 1_000_000_000_000 ? createdAt * 1000 : createdAt;
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
      return '正在思考';
    case 'cancelled':
      return '已取消';
    case 'failed':
      return '执行失败';
    default:
      return status || '正在思考';
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
