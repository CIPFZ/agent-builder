import { Popover, Tag } from 'antd';
import type { ContextUsageViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ContextUsageIndicator.module.css';

interface ContextUsageIndicatorProps {
  usage?: ContextUsageViewModel;
}

export function ContextUsageIndicator({ usage }: ContextUsageIndicatorProps) {
  if (!usage || usage.contextWindow <= 0) {
    return null;
  }
  const levelClass = usage.level === 'error' ? styles.error : usage.level === 'warning' ? styles.warning : styles.ok;
  const reserved = usage.breakdown.find((category) => category.key === 'reserved');
  const categories = rankedBreakdown(usage.breakdown);
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
                上下文估算 {formatTokens(usage.usedTokens)} / {formatTokens(usage.contextWindow)}
              </div>
            </div>
            <Tag>{usage.estimated ? '全部估算' : '含模型用量'}</Tag>
          </div>
          <div className={styles.leftText}>距自动压缩还剩 {usage.percentLeft}%</div>
          <div className={styles.breakdown}>
            {categories.map((category) => (
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
          <div className={styles.footer}>{usage.updatedAt ? `更新于 ${new Date(usage.updatedAt).toLocaleTimeString()}` : ''}</div>
        </div>
      }
    >
      <button className={`${styles.trigger} ${levelClass} ${usage.estimated ? styles.estimated : ''}`} type="button" aria-label={`上下文估算已用 ${usage.percentUsed}%`}>
        <span
          aria-hidden="true"
          className={styles.usageRing}
          style={{ background: `conic-gradient(currentColor ${usage.percentUsed}%, var(--app-border-default) 0)` }}
        >
          <span className={styles.usageRingCenter} />
        </span>
        <span className={styles.percent}>{usage.percentUsed}%</span>
      </button>
    </Popover>
  );
}

// rankedBreakdown collapses the popover's category list to its top 5 (by
// token count, largest first) plus a single "其他" row summing the rest.
// 'reserved' is rendered on its own explanatory line (below the list) and
// 'free' is remaining headroom rather than "usage", so neither competes for
// a top-5 slot.
function rankedBreakdown(categories: ContextUsageViewModel['breakdown']) {
  const ranked = categories
    .filter((category) => category.key !== 'reserved' && category.key !== 'free' && category.tokens > 0)
    .sort((left, right) => right.tokens - left.tokens);
  if (ranked.length <= 5) {
    return ranked;
  }
  const rest = ranked.slice(5);
  const otherTokens = rest.reduce((sum, category) => sum + category.tokens, 0);
  return [
    ...ranked.slice(0, 5),
    { key: 'other', label: '其他', tokens: otherTokens, estimated: rest.some((category) => category.estimated) },
  ];
}

function formatTokens(tokens: number) {
  if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(tokens >= 100000 ? 0 : 1)}k`;
  }
  return `${tokens}`;
}
