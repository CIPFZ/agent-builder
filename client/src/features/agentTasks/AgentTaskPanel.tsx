import { useState } from 'react';
import { BranchesOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import type { AgentRoleViewModel, AgentTaskViewModel } from '../../runtime/workbenchTypes.ts';
import { AgentTaskDetail } from './AgentTaskDetail.tsx';
import { AgentTaskList } from './AgentTaskList.tsx';
import { isFinalAgentTaskStatus } from './agentTaskUtils.ts';
import { projectAgentTaskPanel } from '../../runtime/agentTaskPanelProjection.ts';
import styles from './AgentTaskPanel.module.css';

export function AgentTaskPanel({
  agentRoles,
  selectedTaskID,
  tasks,
  onCancelTask,
  onFollowUp,
  onSelectTask,
}: {
  agentRoles?: AgentRoleViewModel[];
  selectedTaskID?: string;
  tasks?: AgentTaskViewModel[];
  onCancelTask?: (taskID: string) => Promise<void>;
  onFollowUp?: (taskID: string, message: string) => Promise<void>;
  onSelectTask?: (taskID: string) => void;
}) {
  const [localSelectedTaskID, setLocalSelectedTaskID] = useState('');
  const orderedTasks = tasks ?? [];
  const projection = projectAgentTaskPanel(orderedTasks[0]?.parentSessionId ?? '', orderedTasks);
  const effectiveSelectedTaskID = selectedTaskID || localSelectedTaskID;
  const selectedTask = orderedTasks.find((task) => task.id === effectiveSelectedTaskID) ?? orderedTasks[0];
  const activeTasks = orderedTasks.filter((task) => !isFinalAgentTaskStatus(task.status));
  const finalTasks = orderedTasks.filter((task) => isFinalAgentTaskStatus(task.status));

  if (!orderedTasks.length) {
    return null;
  }

  const selectTask = (taskID: string) => {
    setLocalSelectedTaskID(taskID);
    onSelectTask?.(taskID);
  };

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
        <AgentTaskList roles={agentRoles} selectedTaskID={selectedTask?.id} teams={projection.teams} independent={projection.independent} onSelect={selectTask} />
        <AgentTaskDetail roles={agentRoles} task={selectedTask} onCancelTask={onCancelTask} onFollowUp={onFollowUp} />
      </div>
    </section>
  );
}
