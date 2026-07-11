import { ArrowDownOutlined } from '@ant-design/icons';
import styles from './ConversationDock.module.css';

export function JumpToBottomAction({ onClick }: { onClick: () => void }) {
  return (
    <button aria-label="跳到底部" className={styles.iconButton} type="button" onClick={onClick}>
      <ArrowDownOutlined />
    </button>
  );
}
