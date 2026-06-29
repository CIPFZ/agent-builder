import { useState } from 'react';
import { CheckCircleOutlined, DownOutlined, LoadingOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { Button, Progress, Tag, Tooltip } from 'antd';
import type { TodoItemViewModel, TodoSummaryViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './TodoTaskBar.module.css';

export function TodoTaskBar({ todos }: { todos?: TodoSummaryViewModel }) {
  const [expanded, setExpanded] = useState(false);
  if (!todos?.items.length) {
    return null;
  }
  const active = todos.items.find((todo) => todo.status === 'in_progress');
  const percent = todos.total > 0 ? Math.round((todos.completed / todos.total) * 100) : 0;

  return (
    <section className={styles.taskBar} data-testid="todo-task-bar">
      <div className={styles.summary}>
        <div className={styles.current}>
          <span className={styles.icon}>{active ? <LoadingOutlined spin /> : <UnorderedListOutlined />}</span>
          <span className={styles.currentText}>{active ? active.activeForm || active.content : todos.completed === todos.total ? 'All tasks complete' : 'Tasks pending'}</span>
        </div>
        <div className={styles.meta}>
          <Tag>{todos.completed}/{todos.total}</Tag>
          <Progress className={styles.progress} percent={percent} size="small" showInfo={false} />
          <Tooltip title={expanded ? 'Collapse tasks' : 'Show tasks'}>
            <Button
              aria-label={expanded ? 'Collapse tasks' : 'Show tasks'}
              icon={<DownOutlined rotate={expanded ? 180 : 0} />}
              size="small"
              type="text"
              onClick={() => setExpanded((value) => !value)}
            />
          </Tooltip>
        </div>
      </div>
      {expanded ? (
        <div className={styles.list}>
          {todos.items.map((todo) => (
            <TodoTaskBarItem key={todo.id} todo={todo} />
          ))}
        </div>
      ) : null}
    </section>
  );
}

function TodoTaskBarItem({ todo }: { todo: TodoItemViewModel }) {
  const completed = todo.status === 'completed';
  const running = todo.status === 'in_progress';
  return (
    <div className={styles.item} data-status={todo.status}>
      <span className={styles.itemIcon}>{completed ? <CheckCircleOutlined /> : running ? <LoadingOutlined spin /> : <span />}</span>
      <span className={styles.itemText}>{running ? todo.activeForm || todo.content : todo.content}</span>
      <Tag color={completed ? 'success' : running ? 'processing' : 'default'}>{todo.status}</Tag>
    </div>
  );
}
