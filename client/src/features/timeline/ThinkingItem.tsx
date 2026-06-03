import { BulbOutlined } from '@ant-design/icons';
import { Collapse, Tag } from 'antd';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ThinkingItem.module.css';

export function ThinkingItem({ item }: { item: ConversationTimelineItemViewModel }) {
  if (!item.content?.trim()) {
    return (
      <section className={styles.thinking} data-testid="thinking-item">
        <span className={styles.label}>
          <BulbOutlined />
          {thinkingLabel(item)}
          <Tag color="default">runtime</Tag>
        </span>
      </section>
    );
  }

  return (
    <section className={styles.thinking} data-testid="thinking-item">
      <Collapse
        ghost
        size="small"
        items={[
          {
            key: item.id,
            label: (
              <span className={styles.label}>
                <BulbOutlined />
                {thinkingLabel(item)}
                <Tag color="default">runtime</Tag>
              </span>
            ),
            children: <div className={styles.content}>{item.content}</div>,
          },
        ]}
      />
    </section>
  );
}

function thinkingLabel(item: ConversationTimelineItemViewModel) {
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
