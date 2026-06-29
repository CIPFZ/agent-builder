import { BranchesOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import type { HookExecutionViewModel } from '../../runtime/workbenchTypes.ts';
import { executionStatusColor } from './hookExecutionUtils.ts';
import styles from './HookTimelineRow.module.css';

export function HookTimelineRow({
  execution,
  onOpen,
}: {
  execution: HookExecutionViewModel;
  onOpen?: (execution: HookExecutionViewModel) => void;
}) {
  return (
    <button className={styles.row} type="button" onClick={() => onOpen?.(execution)}>
      <span className={styles.icon}>
        <BranchesOutlined />
      </span>
      <div className={styles.body}>
        <div className={styles.title}>
          <span className={styles.name}>{execution.hookName || execution.event}</span>
          <Tag color="blue">{execution.event}</Tag>
          <Tag color={executionStatusColor(execution.status)}>{execution.status}</Tag>
        </div>
        <div className={styles.tags}>
          {execution.durationMs ? <Tag>{formatDuration(execution.durationMs)}</Tag> : null}
          {execution.inputRewritten ? <Tag color="purple">rewritten</Tag> : null}
          {execution.contextInjected ? <Tag color="cyan">context</Tag> : null}
          {execution.redacted ? <Tag color="gold">redacted</Tag> : null}
        </div>
        {execution.reason || execution.error ? <div className={styles.summary}>{execution.error || execution.reason}</div> : null}
      </div>
    </button>
  );
}

function formatDuration(value: number) {
  if (value < 1000) {
    return `${value}ms`;
  }
  return `${Math.round(value / 100) / 10}s`;
}
