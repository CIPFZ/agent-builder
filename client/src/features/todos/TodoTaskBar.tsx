import { CheckCircleOutlined, LoadingOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { Popover } from 'antd';
import type { TodoItemViewModel, TodoSummaryViewModel } from '../../runtime/workbenchTypes.ts';
import { todoDisplayModel } from './todoDisplayPolicy.ts';
import styles from './TodoTaskBar.module.css';

export function TodoTaskBar({ todos, turnStatus }: { todos?: TodoSummaryViewModel; turnStatus?: string }) {
  const display = todoDisplayModel(todos, turnStatus);
  if (display.state === 'hidden') {
    return null;
  }
  const nextIndex = display.items.findIndex((todo) => todo.status !== 'completed');
  const currentIndex = display.activeIndex >= 0 ? display.activeIndex : nextIndex >= 0 ? nextIndex : display.completed;
  const currentStep = Math.min(display.total, Math.max(1, currentIndex + 1));
  const active = display.activeIndex >= 0 ? display.items[display.activeIndex] : undefined;
  const currentText = display.state === 'stopped'
    ? '对话已结束，计划状态未完全更新'
    : active?.activeForm || active?.content || display.items[currentIndex]?.content || '任务进行中';

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
              <span className={styles.count}>{display.completed}/{display.total}</span>
            </div>
            <div className={styles.list}>
              {display.items.map((todo) => (
                <TodoTaskBarItem key={todo.id} todo={todo} running={display.state === 'running'} />
              ))}
            </div>
          </div>
        }
      >
        <button className={styles.taskChip} data-state={display.state} type="button" aria-label={`查看任务进度：第 ${currentStep} / ${display.total} 步`}>
          <span className={styles.icon}>{display.state === 'running' && active ? <LoadingOutlined spin /> : <UnorderedListOutlined />}</span>
          <span className={styles.currentText}>{display.state === 'stopped' ? `已停止 · ${display.completed}/${display.total}` : `第 ${currentStep} / ${display.total} 步`}</span>
        </button>
      </Popover>
    </div>
  );
}

function TodoTaskBarItem({ todo, running: allowRunning }: { todo: TodoItemViewModel; running: boolean }) {
  const completed = todo.status === 'completed';
  const running = allowRunning && todo.status === 'in_progress';
  return (
    <div className={styles.item} data-status={!allowRunning && todo.status === 'in_progress' ? 'stopped' : todo.status}>
      <span className={styles.itemIcon}>{completed ? <CheckCircleOutlined /> : running ? <LoadingOutlined spin /> : <span />}</span>
      <span className={styles.itemText}>{running ? todo.activeForm || todo.content : todo.content}</span>
      <span className={styles.status}>{formatStatus(todo.status, allowRunning)}</span>
    </div>
  );
}

function formatStatus(status: TodoItemViewModel['status'], allowRunning: boolean) {
  if (status === 'completed') {
    return '已完成';
  }
  if (status === 'in_progress') {
    return allowRunning ? '进行中' : '已停止';
  }
  if (status === 'pending') {
    return '待处理';
  }
  return status;
}
