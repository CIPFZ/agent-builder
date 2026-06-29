import { CheckCircleOutlined, ClockCircleOutlined, LoadingOutlined, UnorderedListOutlined } from '@ant-design/icons';
import { Empty, Progress, Tag } from 'antd';
import type { TodoItemViewModel, TodoSummaryViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './TodoPanel.module.css';

export function TodoPanel({ todos }: { todos?: TodoSummaryViewModel }) {
  if (!todos?.items.length) {
    return (
      <section className={styles.panel} data-testid="todo-panel">
        <div className={styles.header}>
          <div className={styles.heading}>
            <UnorderedListOutlined />
            <span>Todos</span>
          </div>
        </div>
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No todos" />
      </section>
    );
  }
  const percent = todos.total > 0 ? Math.round((todos.completed / todos.total) * 100) : 0;
  return (
    <section className={styles.panel} data-testid="todo-panel">
      <div className={styles.header}>
        <div className={styles.heading}>
          <UnorderedListOutlined />
          <span>Todos</span>
        </div>
        <Tag>{todos.completed}/{todos.total}</Tag>
      </div>
      <Progress percent={percent} size="small" />
      <div className={styles.stats}>
        <Stat label="Pending" value={todos.pending} />
        <Stat label="Running" value={todos.inProgress} />
        <Stat label="Done" value={todos.completed} />
      </div>
      <div className={styles.list}>
        {todos.items.map((todo) => (
          <TodoRow key={todo.id} todo={todo} />
        ))}
      </div>
      {todos.updatedAt ? <div className={styles.updated}>Updated {formatTimestamp(todos.updatedAt)}</div> : null}
    </section>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className={styles.stat}>
      <span>{value}</span>
      <small>{label}</small>
    </div>
  );
}

function TodoRow({ todo }: { todo: TodoItemViewModel }) {
  const completed = todo.status === 'completed';
  const running = todo.status === 'in_progress';
  return (
    <div className={styles.row} data-status={todo.status}>
      <span className={styles.rowIcon}>{completed ? <CheckCircleOutlined /> : running ? <LoadingOutlined spin /> : <ClockCircleOutlined />}</span>
      <div className={styles.rowBody}>
        <span>{running ? todo.activeForm || todo.content : todo.content}</span>
        {todo.activeForm && !running ? <small>{todo.activeForm}</small> : null}
      </div>
      <Tag color={completed ? 'success' : running ? 'processing' : 'default'}>{todo.status}</Tag>
    </div>
  );
}

function formatTimestamp(value: number) {
  const timestamp = value < 1_000_000_000_000 ? value * 1000 : value;
  return new Date(timestamp).toLocaleString();
}
