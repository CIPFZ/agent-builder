import { BranchesOutlined, SafetyCertificateOutlined, WarningOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import { isFailedAgentTaskStatus, isWaitingAgentTaskStatus } from '../../runtime/agentTimelineProjection.ts';
import type { AgentTaskViewModel, ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { TraceRow } from './TraceRow.tsx';
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
  const failed = isFailedAgentTaskStatus(task.status);
  return (
    <TraceRow clickable dataAttrs={{ 'data-task-id': task.id }} icon={<BranchesOutlined />} meta={<Tag color={agentTaskStatusColor(task.status)}>{task.status}</Tag>} testId="timeline-agent-task-row" title={task.title || task.id} tone={failed ? 'error' : isWaitingAgentTaskStatus(task.status) ? 'warning' : 'default'} onRowClick={() => onAgentTaskOpen?.(task.id)} />
  );
}

export function AgentTeamTimelineRow({ item, onAgentTaskOpen }: { item: ConversationTimelineItemViewModel; onAgentTaskOpen?: (taskID: string) => void }) {
  const members = item.agentTasks ?? [];
  if (!item.teamId || members.length === 0) return null;
  const active = members.filter((task) => ['queued', 'running', 'streaming', 'in_progress', 'starting'].includes(task.status)).length;
  const completed = members.filter((task) => ['completed', 'complete', 'success', 'succeeded', 'done'].includes(task.status)).length;
  const waiting = members.filter((task) => isWaitingAgentTaskStatus(task.status)).length;
  const failed = members.filter((task) => isFailedAgentTaskStatus(task.status)).length;
  const attention = members.some((task) => isFailedAgentTaskStatus(task.status) || isWaitingAgentTaskStatus(task.status));
  const counts = [`${active} running`, `${completed} completed`, waiting ? `${waiting} waiting` : '', failed ? `${failed} failed` : ''].filter(Boolean).join(' · ');
  return (
    <TraceRow dataAttrs={{ 'data-team-id': item.teamId }} defaultOpen={attention} expandable icon={<BranchesOutlined />} meta={<span>{counts}</span>} testId="timeline-agent-team-row" title="Agent Team" tone={failed ? 'error' : attention ? 'warning' : 'default'}>
      <div className={styles.agentTeamMembers}>
        {members.map((task) => <AgentTeamMember key={task.id} task={task} onAgentTaskOpen={onAgentTaskOpen} />)}
      </div>
    </TraceRow>
  );
}

function AgentTeamMember({ task, onAgentTaskOpen }: { task: AgentTaskViewModel; onAgentTaskOpen?: (taskID: string) => void }) {
  return <AgentTaskTimelineRow item={{ id: `agentTask:${task.id}`, kind: 'agent_task', status: task.status, agentTask: task }} onAgentTaskOpen={onAgentTaskOpen} />;
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
