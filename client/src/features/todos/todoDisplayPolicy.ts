import type { TodoItemViewModel, TodoSummaryViewModel } from '../../runtime/workbenchTypes.ts';

export type TodoDisplayState = 'hidden' | 'running' | 'stopped';

export interface TodoDisplayModel {
  state: TodoDisplayState;
  items: TodoItemViewModel[];
  total: number;
  completed: number;
  activeIndex: number;
}

export function todoDisplayModel(todos: TodoSummaryViewModel | undefined, turnStatus?: string): TodoDisplayModel {
  const items = todos?.items ?? [];
  const total = items.length;
  const completed = items.filter((item) => item.status === 'completed').length;
  const activeIndex = items.findIndex((item) => item.status === 'in_progress');
  if (total === 0 || !turnStatus) return { state: 'hidden', items, total, completed, activeIndex };

  if (isTerminalTurnStatus(turnStatus)) {
    return { state: completed >= total ? 'hidden' : 'stopped', items, total, completed, activeIndex };
  }
  if (isActiveTurnStatus(turnStatus)) {
    return { state: completed >= total ? 'hidden' : 'running', items, total, completed, activeIndex };
  }
  return { state: 'hidden', items, total, completed, activeIndex };
}

export function shouldShowTodoTaskBar(todos: TodoSummaryViewModel | undefined, turnStatus?: string) {
  return todoDisplayModel(todos, turnStatus).state !== 'hidden';
}

function isTerminalTurnStatus(status: string) {
  return status === 'completed' || status === 'failed' || status === 'cancelled' || status === 'interrupted';
}

function isActiveTurnStatus(status: string) {
  return status === 'running' || status === 'queued' || status === 'waiting_permission';
}
