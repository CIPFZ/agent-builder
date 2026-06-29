import { useState } from 'react';
import { SendOutlined, StopOutlined } from '@ant-design/icons';
import { Button, Empty, Input, Progress, Tag, Tooltip, Typography } from 'antd';
import type { AgentRoleViewModel, AgentTaskViewModel } from '../../runtime/workbenchTypes.ts';
import { agentTaskStatusColor, isFinalAgentTaskStatus, roleForTask, roleLabel } from './agentTaskUtils.ts';
import styles from './AgentTaskPanel.module.css';

const { Text } = Typography;

export function AgentTaskDetail({
  roles,
  task,
  onCancelTask,
  onFollowUp,
}: {
  roles?: AgentRoleViewModel[];
  task?: AgentTaskViewModel;
  onCancelTask?: (taskID: string) => Promise<void>;
  onFollowUp?: (taskID: string, message: string) => Promise<void>;
}) {
  const [draft, setDraft] = useState('');
  const [pendingAction, setPendingAction] = useState('');
  if (!task) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No task selected" />;
  }
  const selectedFinal = isFinalAgentTaskStatus(task.status);
  const role = roleForTask(task, roles);
  const submitFollowUp = async () => {
    const message = draft.trim();
    if (!message || !onFollowUp || selectedFinal) {
      return;
    }
    setPendingAction(`follow-up:${task.id}`);
    try {
      await onFollowUp(task.id, message);
      setDraft('');
    } finally {
      setPendingAction('');
    }
  };
  const cancelTask = async () => {
    if (!onCancelTask || selectedFinal) {
      return;
    }
    setPendingAction(`cancel:${task.id}`);
    try {
      await onCancelTask(task.id);
    } finally {
      setPendingAction('');
    }
  };

  return (
    <div className={styles.detail} data-testid="agent-task-detail" data-task-id={task.id}>
      <div className={styles.detailHeader}>
        <div className={styles.detailTitle}>
          <Text strong>{task.title || task.id}</Text>
          <Text type="secondary">{task.resultSummary || task.promptSummary || task.id}</Text>
        </div>
        <Tag color={agentTaskStatusColor(task.status)}>{task.status}</Tag>
      </div>
      <Progress percent={task.progress ?? 0} size="small" status={task.status === 'failed' ? 'exception' : undefined} />
      {role?.description ? <div className={styles.roleDescription}>{role.description}</div> : null}
      <InfoGrid task={task} roleLabel={roleLabel(task, roles)} />
      <RefList title="Output refs" values={task.outputRefs} />
      <RefList title="Artifact refs" values={task.artifactRefs} />
      <RefList title="Compact refs" values={task.compactBoundaryRefs} />
      <MessageList task={task} />
      <div className={styles.actions}>
        <Input.TextArea
          aria-label="Task follow-up"
          autoSize={{ minRows: 2, maxRows: 4 }}
          disabled={selectedFinal}
          placeholder={selectedFinal ? 'Final tasks are read-only' : 'Send follow-up'}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
        <div className={styles.actionButtons}>
          <Tooltip title={selectedFinal ? 'Final task is read-only' : 'Send follow-up'}>
            <Button
              icon={<SendOutlined />}
              loading={pendingAction === `follow-up:${task.id}`}
              disabled={selectedFinal || !draft.trim() || !onFollowUp}
              onClick={submitFollowUp}
            >
              Send
            </Button>
          </Tooltip>
          <Tooltip title={selectedFinal ? 'Final task is read-only' : 'Cancel task'}>
            <Button danger icon={<StopOutlined />} loading={pendingAction === `cancel:${task.id}`} disabled={selectedFinal || !onCancelTask} onClick={cancelTask}>
              Cancel
            </Button>
          </Tooltip>
        </div>
      </div>
    </div>
  );
}

function InfoGrid({ roleLabel: taskRoleLabel, task }: { roleLabel?: string; task: AgentTaskViewModel }) {
  const rows = [
    ['Task', task.id],
    ['Parent session', task.parentSessionId],
    ['Parent turn', task.parentTurnId],
    ['Parent tool', task.parentToolCallId],
    ['Child session', task.childSessionId],
    ['Role', taskRoleLabel],
    ['Provider/model', [task.provider, task.model].filter(Boolean).join(' / ')],
    ['CWD', task.cwd],
    ['Worktree', task.worktree],
    ['Allowed tools', task.allowedTools?.join(', ')],
    ['Capability scope', task.capabilityScope?.join(', ')],
    ['Cancellation', task.cancellationDetail],
  ].filter(([, value]) => Boolean(value));
  return (
    <dl className={styles.infoGrid}>
      {rows.map(([label, value]) => (
        <div key={label} className={styles.infoRow}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function RefList({ title, values }: { title: string; values?: string[] }) {
  if (!values?.length) {
    return null;
  }
  return (
    <div className={styles.refs}>
      <Text type="secondary">{title}</Text>
      {values.map((value) => (
        <code key={value}>{value}</code>
      ))}
    </div>
  );
}

function MessageList({ task }: { task: AgentTaskViewModel }) {
  const messages = task.messages ?? [];
  if (!messages.length) {
    return null;
  }
  return (
    <div className={styles.messages}>
      <Text type="secondary">Messages</Text>
      {messages.slice(-6).map((message) => (
        <div key={message.id} className={styles.messageRow}>
          <span>{message.sequence}</span>
          <Tag>{message.status || message.kind}</Tag>
          <p>{message.contentSummary || message.error || message.id}</p>
        </div>
      ))}
    </div>
  );
}
