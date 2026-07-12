import type { TodoItemViewModel, TodoSummaryViewModel } from './workbenchTypes.ts';
import type { OutputStore, RuntimeOutputTodo, RuntimeOutputTodoSummary } from './outputTypes.ts';

export function selectSessionTodos(store: OutputStore | undefined, sessionId: string | undefined): TodoSummaryViewModel | undefined {
  if (!store || !sessionId || store.sessionId !== sessionId) return undefined;
  return mapRuntimeTodoSummary(store.todos, sessionId);
}

export function mapRuntimeTodoSummary(summary: RuntimeOutputTodoSummary | undefined, fallbackSessionId = ''): TodoSummaryViewModel | undefined {
  if (!summary) return undefined;
  const items = (summary.todos ?? []).map(mapRuntimeTodo).filter((item): item is TodoItemViewModel => Boolean(item));
  return {
    sessionId: summary.sessionId || fallbackSessionId,
    turnId: summary.turnId,
    items,
    pending: items.filter((item) => item.status === 'pending').length,
    inProgress: items.filter((item) => item.status === 'in_progress').length,
    completed: items.filter((item) => item.status === 'completed').length,
    total: items.length,
    updatedAt: summary.updatedAt,
  };
}

function mapRuntimeTodo(todo: RuntimeOutputTodo, index: number): TodoItemViewModel | undefined {
  const content = todo.content?.trim();
  if (!content) return undefined;
  return {
    id: todo.id || `todo:${index + 1}:${content}`,
    content,
    status: todo.status || 'pending',
    activeForm: todo.activeForm || todo.active_form,
    createdAt: todo.createdAt,
    updatedAt: todo.updatedAt,
    source: todo.source?.kind ? { kind: todo.source.kind, label: todo.source.label, ref: todo.source.ref } : undefined,
  };
}
