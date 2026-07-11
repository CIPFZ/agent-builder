import { MoreOutlined } from '@ant-design/icons';
import { Dropdown } from 'antd';
import type { ReactNode } from 'react';
import styles from './ConversationDock.module.css';

export interface ConversationAction {
  key: string;
  node: ReactNode;
  priority?: number;
  pinned?: boolean;
  overflow?: {
    label: ReactNode;
    icon?: ReactNode;
    onSelect: () => void;
  };
}

const MAX_VISIBLE_ACTIONS = 3;

export function ConversationActions({ actions }: { actions: ConversationAction[] }) {
  const sorted = [...actions].sort((left, right) => {
    if (Boolean(left.pinned) !== Boolean(right.pinned)) {
      return left.pinned ? -1 : 1;
    }
    return (left.priority ?? 100) - (right.priority ?? 100);
  });
  const pinned = sorted.filter((action) => action.pinned || !action.overflow);
  const optional = sorted.filter((action) => !action.pinned && action.overflow);
  const availableSlots = Math.max(0, MAX_VISIBLE_ACTIONS - pinned.length);
  const visible = [...pinned, ...optional.slice(0, availableSlots)].sort(
    (left, right) => (left.priority ?? 100) - (right.priority ?? 100),
  );
  const overflow = optional.slice(availableSlots);

  return (
    <div className={styles.actions} data-testid="conversation-actions">
      {visible.map((action) => <span className={styles.action} key={action.key}>{action.node}</span>)}
      {overflow.length > 0 ? (
        <Dropdown
          trigger={['click']}
          menu={{
            items: overflow.map((action) => ({
              key: action.key,
              icon: action.overflow?.icon,
              label: action.overflow?.label ?? action.key,
            })),
            onClick: ({ key }) => overflow.find((action) => action.key === key)?.overflow?.onSelect(),
          }}
        >
          <button aria-label="更多对话操作" className={styles.moreButton} type="button">
            <MoreOutlined />
          </button>
        </Dropdown>
      ) : null}
    </div>
  );
}
