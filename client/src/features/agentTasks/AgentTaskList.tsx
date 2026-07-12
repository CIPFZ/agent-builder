import { Tag } from 'antd';
import type { AgentRoleViewModel, AgentTaskViewModel } from '../../runtime/workbenchTypes.ts';
import { agentTaskStatusColor, roleLabel } from './agentTaskUtils.ts';
import styles from './AgentTaskPanel.module.css';

export function AgentTaskList({
  roles,
  selectedTaskID,
  tasks,
  onSelect,
}: {
  roles?: AgentRoleViewModel[];
  selectedTaskID?: string;
  tasks: AgentTaskViewModel[];
  onSelect: (taskID: string) => void;
}) {
  return (
    <div className={styles.list} role="listbox" aria-label="Agent task list">
      {tasks.map((task) => (
        <button
          key={task.id}
          className={`${styles.taskButton} ${selectedTaskID === task.id ? styles.taskButtonActive : ''}`}
          type="button"
          role="option"
          aria-selected={selectedTaskID === task.id}
          data-task-id={task.id}
          onClick={() => onSelect(task.id)}
        >
          <span className={styles.taskButtonTitle}>{task.title || task.id}</span>
          <span className={styles.taskButtonMeta}>
            <Tag color={agentTaskStatusColor(task.status)}>{task.status}</Tag>
            {task.teamId ? <Tag color="blue">Team</Tag> : null}
            <span>{roleLabel(task, roles)}</span>
          </span>
        </button>
      ))}
    </div>
  );
}
