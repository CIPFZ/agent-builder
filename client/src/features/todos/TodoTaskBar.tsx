import { CheckCircleOutlined, LoadingOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { Popover } from 'antd';
import type { TodoItemViewModel, TodoSummaryViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './TodoTaskBar.module.css';

export function TodoTaskBar({ todos }: { todos?: TodoSummaryViewModel }) {
  const total = todos?.total || todos?.items.length || 0;
  const completed = Math.min(todos?.completed || 0, total);
  const allComplete = total > 0 && completed >= total;
  if (!todos?.items.length || total <= 0 || allComplete) {
    return null;
  }
  const active = todos.items.find((todo) => todo.status === 'in_progress');
  const activeIndex = todos.items.findIndex((todo) => todo.status === 'in_progress');
  const nextIndex = todos.items.findIndex((todo) => todo.status !== 'completed');
  const currentIndex = activeIndex >= 0 ? activeIndex : nextIndex >= 0 ? nextIndex : completed;
  const currentStep = Math.min(total, Math.max(1, currentIndex + 1));
  const currentText = active?.activeForm || active?.content || todos.items[currentIndex]?.content || '任务进行中';

  return (
    <div className={styles.taskDock} data-testid="todo-task-bar">
      <Popover
        trigger={['hover', 'click']}
        placement="top"
        content={
          <div className={styles.popover} data-testid="todo-task-popover">
            <div className={styles.popoverHeader}>
              <div>
                <div className={styles.popoverTitle}>任务进度</div>
                <div className={styles.popoverSubtitle}>{currentText}</div>
              </div>
              <span className={styles.count}>{completed}/{total}</span>
            </div>
            <div className={styles.list}>
              {todos.items.map((todo) => (
                <TodoTaskBarItem key={todo.id} todo={todo} />
              ))}
            </div>
          </div>
        }
      >
        <button className={styles.taskChip} type="button" aria-label={`查看任务进度：第 ${currentStep} / ${total} 步`}>
          <span className={styles.icon}>{active ? <LoadingOutlined spin /> : <UnorderedListOutlined />}</span>
          <span className={styles.currentText}>第 {currentStep} / {total} 步</span>
        </button>
      </Popover>
    </div>
  );
}

function TodoTaskBarItem({ todo }: { todo: TodoItemViewModel }) {
  const completed = todo.status === 'completed';
  const running = todo.status === 'in_progress';
  return (
    <div className={styles.item} data-status={todo.status}>
      <span className={styles.itemIcon}>{completed ? <CheckCircleOutlined /> : running ? <LoadingOutlined spin /> : <span />}</span>
      <span className={styles.itemText}>{running ? todo.activeForm || todo.content : todo.content}</span>
      <span className={styles.status}>{formatStatus(todo.status)}</span>
    </div>
  );
}

function formatStatus(status: TodoItemViewModel['status']) {
  if (status === 'completed') {
    return '已完成';
  }
  if (status === 'in_progress') {
    return '进行中';
  }
  if (status === 'pending') {
    return '待处理';
  }
  return status;
}
