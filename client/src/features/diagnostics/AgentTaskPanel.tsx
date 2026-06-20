import { useMemo, useState } from 'react';
import { BranchesOutlined, SendOutlined, StopOutlined } from '@ant-design/icons';
import { Button, Empty, Input, Progress, Tag, Tooltip, Typography } from 'antd';
import type { AgentTaskViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './AgentTaskPanel.module.css';

const { Text } = Typography;

export function AgentTaskPanel({
  tasks,
  onCancelTask,
  onFollowUp,
}: {
  tasks?: AgentTaskViewModel[];
  onCancelTask?: (taskID: string) => Promise<void>;
  onFollowUp?: (taskID: string, message: string) => Promise<void>;
}) {
  const [selectedTaskID, setSelectedTaskID] = useState('');
  const [draft, setDraft] = useState('');
  const [pendingAction, setPendingAction] = useState('');
  const orderedTasks = useMemo(() => [...(tasks ?? [])].sort(compareTasks), [tasks]);
  const selectedTask = orderedTasks.find((task) => task.id === selectedTaskID) ?? orderedTasks[0];
  const activeTasks = orderedTasks.filter((task) => !isFinalTaskStatus(task.status));
  const finalTasks = orderedTasks.filter((task) => isFinalTaskStatus(task.status));

  if (!orderedTasks.length) {
    return null;
  }

  const submitFollowUp = async () => {
    const message = draft.trim();
    if (!selectedTask || !message || !onFollowUp || isFinalTaskStatus(selectedTask.status)) {
      return;
    }
    setPendingAction(`follow-up:${selectedTask.id}`);
    try {
      await onFollowUp(selectedTask.id, message);
      setDraft('');
    } finally {
      setPendingAction('');
    }
  };
  const cancelTask = async () => {
    if (!selectedTask || !onCancelTask || isFinalTaskStatus(selectedTask.status)) {
      return;
    }
    setPendingAction(`cancel:${selectedTask.id}`);
    try {
      await onCancelTask(selectedTask.id);
    } finally {
      setPendingAction('');
    }
  };
  const selectedFinal = isFinalTaskStatus(selectedTask?.status);

  return (
    <section className={styles.panel} data-testid="agent-task-panel" aria-label="Agent tasks">
      <div className={styles.header}>
        <div className={styles.heading}>
          <BranchesOutlined />
          <span>Agent tasks</span>
        </div>
        <div className={styles.counts}>
          <Tag color="processing">{activeTasks.length} active</Tag>
          <Tag>{finalTasks.length} final</Tag>
        </div>
      </div>

      <div className={styles.body}>
        <div className={styles.list} role="listbox" aria-label="Agent task list">
          {orderedTasks.map((task) => (
            <button
              key={task.id}
              className={`${styles.taskButton} ${selectedTask?.id === task.id ? styles.taskButtonActive : ''}`}
              type="button"
              role="option"
              aria-selected={selectedTask?.id === task.id}
              data-task-id={task.id}
              onClick={() => setSelectedTaskID(task.id)}
            >
              <span className={styles.taskButtonTitle}>{task.title || task.id}</span>
              <span className={styles.taskButtonMeta}>
                {task.status} · {task.role || task.kind}
              </span>
            </button>
          ))}
        </div>

        {selectedTask ? (
          <div className={styles.detail} data-testid="agent-task-detail" data-task-id={selectedTask.id}>
            <div className={styles.detailHeader}>
              <div className={styles.detailTitle}>
                <Text strong>{selectedTask.title || selectedTask.id}</Text>
                <Text type="secondary">{selectedTask.promptSummary || selectedTask.resultSummary || selectedTask.id}</Text>
              </div>
              <Tag color={taskStatusColor(selectedTask.status)}>{selectedTask.status}</Tag>
            </div>
            <Progress percent={selectedTask.progress ?? 0} size="small" status={selectedTask.status === 'failed' ? 'exception' : undefined} />
            <InfoGrid task={selectedTask} />
            <RefList title="Output refs" values={selectedTask.outputRefs} />
            <RefList title="Artifact refs" values={selectedTask.artifactRefs} />
            <RefList title="Compact refs" values={selectedTask.compactBoundaryRefs} />
            <MessageList task={selectedTask} />
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
                    loading={pendingAction === `follow-up:${selectedTask.id}`}
                    disabled={selectedFinal || !draft.trim() || !onFollowUp}
                    onClick={submitFollowUp}
                  >
                    Send
                  </Button>
                </Tooltip>
                <Tooltip title={selectedFinal ? 'Final task is read-only' : 'Cancel task'}>
                  <Button
                    danger
                    icon={<StopOutlined />}
                    loading={pendingAction === `cancel:${selectedTask.id}`}
                    disabled={selectedFinal || !onCancelTask}
                    onClick={cancelTask}
                  >
                    Cancel
                  </Button>
                </Tooltip>
              </div>
            </div>
          </div>
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No task selected" />
        )}
      </div>
    </section>
  );
}

function InfoGrid({ task }: { task: AgentTaskViewModel }) {
  const rows = [
    ['Task', task.id],
    ['Parent session', task.parentSessionId],
    ['Parent turn', task.parentTurnId],
    ['Parent tool', task.parentToolCallId],
    ['Child session', task.childSessionId],
    ['Role', task.role],
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

function compareTasks(left: AgentTaskViewModel, right: AgentTaskViewModel) {
  const leftActive = isFinalTaskStatus(left.status) ? 1 : 0;
  const rightActive = isFinalTaskStatus(right.status) ? 1 : 0;
  if (leftActive !== rightActive) {
    return leftActive - rightActive;
  }
  return (right.updatedAt ?? right.startedAt ?? 0) - (left.updatedAt ?? left.startedAt ?? 0);
}

function isFinalTaskStatus(status?: string) {
  return status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'interrupted';
}

function taskStatusColor(status?: string) {
  switch (status) {
    case 'running':
    case 'queued':
      return 'processing';
    case 'completed':
      return 'success';
    case 'failed':
    case 'interrupted':
      return 'error';
    case 'cancelled':
      return 'default';
    default:
      return 'default';
  }
}
