import { Tag } from 'antd';
import type { AgentRoleViewModel, AgentTaskViewModel } from '../../runtime/workbenchTypes.ts';
import type { AgentTeamPresentation } from '../../runtime/conversationPresentationModel.ts';
import { agentTaskStatusColor, roleLabel } from './agentTaskUtils.ts';
import styles from './AgentTaskPanel.module.css';

export function AgentTaskList({
  roles,
  selectedTaskID,
  teams,
  independent,
  onSelect,
}: {
  roles?: AgentRoleViewModel[];
  selectedTaskID?: string;
  teams: AgentTeamPresentation[];
  independent: AgentTaskViewModel[];
  onSelect: (taskID: string) => void;
}) {
  return (
    <div className={styles.list} role="listbox" aria-label="Agent task list">
      {independent.length ? <TaskGroup label="Independent tasks" tasks={independent} roles={roles} selectedTaskID={selectedTaskID} onSelect={onSelect} /> : null}
      {teams.map((team) => <TaskGroup key={team.id} label={`Agent Team · ${team.teamId}`} tasks={team.members} roles={roles} selectedTaskID={selectedTaskID} onSelect={onSelect} />)}
    </div>
  );
}

function TaskGroup({ label, tasks, roles, selectedTaskID, onSelect }: { label: string; tasks: AgentTaskViewModel[]; roles?: AgentRoleViewModel[]; selectedTaskID?: string; onSelect: (taskID: string) => void }) {
  return (
    <section className={styles.taskGroup} data-testid="agent-task-group">
      <div className={styles.taskGroupLabel}>{label}</div>
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
    </section>
  );
}
