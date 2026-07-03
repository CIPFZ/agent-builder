import { useState } from 'react';
import { Button, Tag } from 'antd';
import { CompressOutlined, LoadingOutlined, WarningOutlined } from '@ant-design/icons';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './CompactDivider.module.css';

interface CompactDividerProps {
  item: ConversationTimelineItemViewModel;
}

export function CompactDivider({ item }: CompactDividerProps) {
  const [open, setOpen] = useState(false);
  const failed = item.status === 'failed' || Boolean(item.error);
  const compacting = item.status === 'compacting' || item.status === 'started';
  const title = compactTitle(item, compacting, failed);
  return (
    <div className={`${styles.divider} ${failed ? styles.failed : ''}`} data-testid="timeline-compact-divider" data-compact-status={item.status}>
      <div className={styles.line} />
      <Button
        className={styles.pill}
        icon={failed ? <WarningOutlined /> : compacting ? <LoadingOutlined spin /> : <CompressOutlined />}
        size="small"
        type="default"
        onClick={() => setOpen((value) => !value)}
      >
        {title}
      </Button>
      <div className={styles.line} />
      {open && (
        <div className={styles.details}>
          <div className={styles.meta}>
            {item.title && <Tag>{triggerLabel(item.title)}</Tag>}
            {item.status && <Tag color={failed ? 'error' : compacting ? 'processing' : 'success'}>{item.status}</Tag>}
          </div>
          {(item.error || item.summary || item.content) && <div className={styles.summary}>{item.error || item.summary || item.content}</div>}
        </div>
      )}
    </div>
  );
}

function compactTitle(item: ConversationTimelineItemViewModel, compacting: boolean, failed: boolean) {
  if (failed) {
    return '上下文压缩失败';
  }
  if (compacting) {
    return '正在压缩上下文...';
  }
  const trigger = triggerLabel(item.title || '');
  return trigger ? `${trigger}上下文已压缩` : '上下文已压缩';
}

function triggerLabel(trigger: string) {
  switch (trigger) {
    case 'auto':
      return '自动';
    case 'manual':
      return '手动';
    case 'reactive':
      return '恢复触发';
    default:
      return trigger;
  }
}
