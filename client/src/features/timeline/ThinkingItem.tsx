import { BulbOutlined, LoadingOutlined } from '@ant-design/icons';
import { Collapse } from 'antd';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ThinkingItem.module.css';

export function ThinkingItem({ item }: { item: ConversationTimelineItemViewModel }) {
  const label = (
    <span className={styles.label}>
      {item.status === 'running' ? <LoadingOutlined spin /> : <BulbOutlined />}
      {thinkingLabel(item)}
    </span>
  );

  if (!item.content?.trim()) {
    return (
      <section className={styles.thinking} data-testid="thinking-item" data-thinking-status={item.status}>
        {label}
      </section>
    );
  }

  return (
    <section className={styles.thinking} data-testid="thinking-item" data-thinking-status={item.status}>
      <Collapse
        ghost
        size="small"
        expandIconPlacement="end"
        items={[
          {
            key: item.id,
            label,
            children: <div className={styles.content}>{item.content}</div>,
          },
        ]}
      />
    </section>
  );
}

function thinkingLabel(item: ConversationTimelineItemViewModel) {
  if (item.title && item.source === 'react_callchain') {
    return item.title;
  }
  const duration = thinkingDuration(item);
  if (item.status === 'running') {
    return duration ? `正在思考 ${duration}` : '正在思考';
  }
  return duration ? `已思考 ${duration}` : '已思考';
}

function thinkingDuration(item: ConversationTimelineItemViewModel) {
  if (!item.createdAt || !item.updatedAt) {
    return '';
  }
  const startedAt = normalizeTimestamp(item.createdAt);
  const updatedAt = normalizeTimestamp(item.updatedAt);
  const elapsed = Math.max(0, updatedAt - startedAt);
  if (elapsed < 1000) {
    return '<1s';
  }
  if (elapsed < 60_000) {
    return `${Math.round(elapsed / 1000)}s`;
  }
  return `${Math.floor(elapsed / 60_000)}m ${Math.round((elapsed % 60_000) / 1000)}s`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}
