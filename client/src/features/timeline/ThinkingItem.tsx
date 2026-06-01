import { BulbOutlined } from '@ant-design/icons';
import { Collapse, Tag } from 'antd';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ThinkingItem.module.css';

export function ThinkingItem({ item }: { item: ConversationTimelineItemViewModel }) {
  if (!item.content?.trim()) {
    return null;
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
                运行摘要
                <Tag color="default">runtime 提供</Tag>
              </span>
            ),
            children: <div className={styles.content}>{item.content}</div>,
          },
        ]}
      />
    </section>
  );
}
