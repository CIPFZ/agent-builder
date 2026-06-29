import type { AgentRoleViewModel, AgentTaskViewModel } from '../../runtime/workbenchTypes.ts';

export function compareAgentTasks(left: AgentTaskViewModel, right: AgentTaskViewModel) {
  const leftFinal = isFinalAgentTaskStatus(left.status) ? 1 : 0;
  const rightFinal = isFinalAgentTaskStatus(right.status) ? 1 : 0;
  if (leftFinal !== rightFinal) {
    return leftFinal - rightFinal;
  }
  return (right.updatedAt ?? right.startedAt ?? 0) - (left.updatedAt ?? left.startedAt ?? 0);
}

export function isFinalAgentTaskStatus(status?: string) {
  return status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'interrupted';
}

export function agentTaskStatusColor(status?: string) {
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

export function roleForTask(task: AgentTaskViewModel, roles?: AgentRoleViewModel[]) {
  return roles?.find((role) => role.id === task.role);
}

export function roleLabel(task: AgentTaskViewModel, roles?: AgentRoleViewModel[]) {
  const role = roleForTask(task, roles);
  return role?.title || role?.name || task.role || task.kind;
}
