import { BranchesOutlined, SafetyCertificateOutlined, WarningOutlined } from '@ant-design/icons';
import { Progress, Tag } from 'antd';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { InlineExpandable, TraceRow } from './TraceRow.tsx';
import styles from './Timeline.module.css';

export function PermissionTraceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const permission = item.permission;
  if (!permission) return null;
  const failed = permission.status === 'denied' || permission.status === 'cancelled' || permission.status === 'expired';
  const reason = permission.reason || permission.policyReason;
  return (
    <TraceRow dataAttrs={{ 'data-permission-status': permission.status }} extra={reason ? <span>{reason}</span> : null} icon={failed ? <WarningOutlined /> : <SafetyCertificateOutlined />} testId="permission-trace-row" title={<>{permissionStatusLabel(permission.status)}<code className={styles.inlineCode}>{permission.target || permission.path || permission.toolName}</code></>} tone={failed ? 'error' : 'default'} />
  );
}

export function AgentTaskTimelineRow({ item, onAgentTaskOpen }: { item: ConversationTimelineItemViewModel; onAgentTaskOpen?: (taskID: string) => void }) {
  const task = item.agentTask;
  if (!task) return null;
  const refs = [...(task.outputRefs ?? []), ...(task.artifactRefs ?? [])];
  const summary = task.resultSummary || task.promptSummary || '';
  const failed = task.status === 'failed' || task.status === 'interrupted';
  const metaLine = [task.role || task.kind, task.provider && task.model ? `${task.provider}/${task.model}` : task.model, task.childSessionId ? `child ${task.childSessionId}` : undefined].filter(Boolean).join(' / ');
  return (
    <TraceRow clickable dataAttrs={{ 'data-task-id': task.id }} extra={<div className={styles.agentTaskExtra}><Progress percent={task.progress ?? 0} size="small" showInfo={false} />{metaLine ? <div className={styles.agentTaskMetaLine}>{metaLine}</div> : null}{summary ? <InlineExpandable summary={summarizeProcessNote(summary)}>{summary}</InlineExpandable> : null}{refs.length ? <div className={styles.agentTaskRefsLine}>{refs.slice(0, 3).join(' / ')}</div> : null}</div>} icon={<BranchesOutlined />} meta={<Tag color={agentTaskStatusColor(task.status)}>{task.status}</Tag>} testId="timeline-agent-task-row" title={task.title || task.id} tone={failed ? 'error' : 'default'} onRowClick={() => onAgentTaskOpen?.(task.id)} />
  );
}

function permissionStatusLabel(status?: string) {
  if (status === 'pending') return '等待权限确认';
  if (status === 'allowed' || status === 'allowed_once') return '已允许';
  if (status === 'allowed_session') return '本会话已允许';
  if (status === 'denied') return '已拒绝';
  if (status === 'cancelled') return '已取消';
  if (status === 'expired') return '已过期';
  return status || '权限记录';
}

function agentTaskStatusColor(status?: string) {
  if (status === 'queued' || status === 'running') return 'processing';
  if (status === 'completed') return 'success';
  if (status === 'failed' || status === 'interrupted') return 'error';
  return 'default';
}

function summarizeProcessNote(content: string) {
  const normalized = content.replace(/\s+/g, ' ').trim();
  return normalized.length <= 140 ? normalized : `${normalized.slice(0, 140)}...`;
}
