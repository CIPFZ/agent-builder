import { Tag } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { PermissionGate } from '../permissions/PermissionGate.tsx';
import { ThinkingItem } from './ThinkingItem.tsx';
import { ToolCallCard } from '../tools/ToolCallCard.tsx';
import styles from './Timeline.module.css';

interface TimelineProps {
  items: ConversationTimelineItemViewModel[];
  onPermissionDecide: (permissionID: string, action: 'allow' | 'allow_for_session' | 'deny') => Promise<void>;
}

export function Timeline({ items, onPermissionDecide }: TimelineProps) {
  return (
    <div className={styles.timeline} data-testid="conversation-timeline">
      {items.map((item) => {
        if (item.kind === 'tool_call' && item.toolCall) {
          return <ToolCallCard key={item.id} toolCall={item.toolCall} />;
        }
        if (item.kind === 'permission' && item.permission) {
          return <PermissionGate key={item.id} permission={item.permission} onDecide={onPermissionDecide} />;
        }
        if (item.kind === 'thinking') {
          return <ThinkingItem key={item.id} item={item} />;
        }
        if (item.kind === 'progress') {
          return (
            <div key={item.id} className={styles.progress} data-testid="turn-progress" data-progress-status={item.status}>
              {progressLabel(item.status)}
            </div>
          );
        }
        return (
          <Bubble
            key={item.id}
            className={item.role === 'user' ? styles.userBubble : styles.assistantBubble}
            content={item.content}
            placement={item.role === 'user' ? 'end' : 'start'}
            variant={item.role === 'user' ? 'filled' : 'borderless'}
            footer={item.status === 'error' ? <Tag color="error">失败</Tag> : undefined}
          />
        );
      })}
    </div>
  );
}

function progressLabel(status?: string) {
  switch (status) {
    case 'waiting_permission':
      return '等待权限确认';
    case 'running':
    case 'queued':
      return '正在思考';
    case 'cancelled':
      return '已取消';
    case 'failed':
      return '执行失败';
    default:
      return status || '正在思考';
  }
}
