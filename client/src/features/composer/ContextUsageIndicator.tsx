import { Button, Popover, Tag, Tooltip } from 'antd';
import { CompressOutlined } from '@ant-design/icons';
import type { CSSProperties } from 'react';
import type { ContextUsageViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ContextUsageIndicator.module.css';

interface ContextUsageIndicatorProps {
  usage?: ContextUsageViewModel;
  compacting?: boolean;
  onManualCompact?: () => Promise<void>;
}

export function ContextUsageIndicator({ usage, compacting, onManualCompact }: ContextUsageIndicatorProps) {
  if (!usage || usage.contextWindow <= 0) {
    return null;
  }
  const levelClass = usage.level === 'error' ? styles.error : usage.level === 'warning' ? styles.warning : styles.ok;
  const ringStyle = {
    '--context-usage-percent': `${usage.percentUsed}%`,
  } as CSSProperties;
  const reserved = usage.breakdown.find((category) => category.key === 'reserved');
  return (
    <Popover
      trigger="click"
      placement="topRight"
      content={
        <div className={styles.popover} data-testid="context-usage-popover">
          <div className={styles.popoverHeader}>
            <div>
              <div className={styles.model}>{usage.model || '模型'}</div>
              <div className={styles.total}>
                已用 {formatTokens(usage.usedTokens)} / {formatTokens(usage.contextWindow)} ({usage.percentUsed}%)
              </div>
            </div>
            {usage.estimated && <Tag>估算</Tag>}
          </div>
          <div className={styles.leftText}>距自动压缩还剩 {usage.percentLeft}%</div>
          <div className={styles.breakdown}>
            {usage.breakdown
              .filter((category) => category.tokens > 0)
              .map((category) => (
                <div className={styles.category} key={category.key}>
                  <div className={styles.categoryMeta}>
                    <span>{category.label}</span>
                    <span>{formatTokens(category.tokens)}</span>
                  </div>
                  <meter className={styles.meter} min={0} max={usage.contextWindow} value={category.tokens} />
                </div>
              ))}
          </div>
          {reserved && <div className={styles.reserved}>预留用于输出与压缩缓冲：{formatTokens(reserved.tokens)}</div>}
          <div className={styles.footer}>
            <span>{usage.updatedAt ? new Date(usage.updatedAt).toLocaleTimeString() : ''}</span>
            <Tooltip title={onManualCompact ? '' : '当前对话没有可压缩的 turn'}>
              <Button size="small" disabled={!onManualCompact || compacting} icon={<CompressOutlined />} loading={compacting} onClick={() => void onManualCompact?.()}>
                手动压缩
              </Button>
            </Tooltip>
          </div>
        </div>
      }
    >
      <button className={`${styles.trigger} ${levelClass} ${usage.estimated ? styles.estimated : ''}`} type="button" aria-label="上下文用量">
        <span className={styles.ring} style={ringStyle} />
        <span className={styles.percent}>{usage.percentUsed}%</span>
      </button>
    </Popover>
  );
}

function formatTokens(tokens: number) {
  if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(tokens >= 100000 ? 0 : 1)}k`;
  }
  return `${tokens}`;
}
