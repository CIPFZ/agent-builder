import { useState } from 'react';
import type { KeyboardEvent, ReactNode } from 'react';
import { DownOutlined } from '@ant-design/icons';
import styles from './TraceRow.module.css';

export type TraceRowTone = 'default' | 'error' | 'warning';

export interface TraceRowProps {
  testId?: string;
  dataAttrs?: Record<string, string | undefined>;
  icon?: ReactNode;
  title: ReactNode;
  meta?: ReactNode;
  tone?: TraceRowTone;
  clickable?: boolean;
  onRowClick?: () => void;
  expandable?: boolean;
  defaultOpen?: boolean;
  /** Content rendered under the header, always visible regardless of expand state. */
  extra?: ReactNode;
  /** Collapsible body, revealed via the row's expand toggle. */
  children?: ReactNode;
  className?: string;
}

// TraceRow is the single row primitive for the exploration trace: an icon
// slot, a title, optional meta (duration/status tag), optional "always
// visible" extra content (used for failure excerpts so they never require
// expanding), and an optional collapsible body. It replaces the native
// <details> elements and bare divs previously scattered across permission,
// hook, context-governance, agent-task and workflow rows.
export function TraceRow({
  testId,
  dataAttrs,
  icon,
  title,
  meta,
  tone = 'default',
  clickable = false,
  onRowClick,
  expandable = false,
  defaultOpen = false,
  extra,
  children,
  className,
}: TraceRowProps) {
  const hasBody = expandable && Boolean(children);
  const [open, setOpen] = useState(defaultOpen);

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!clickable) {
      return;
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onRowClick?.();
    }
  };

  const toggleOpen = () => setOpen((value) => !value);

  return (
    <div
      className={[styles.row, tone === 'error' ? styles.rowError : tone === 'warning' ? styles.rowWarning : '', clickable ? styles.rowClickable : '', className]
        .filter(Boolean)
        .join(' ')}
      data-testid={testId}
      role={clickable ? 'button' : undefined}
      tabIndex={clickable ? 0 : undefined}
      onClick={clickable ? onRowClick : undefined}
      onKeyDown={clickable ? handleKeyDown : undefined}
      {...dataAttrs}
    >
      {icon ? <span className={styles.rowIcon}>{icon}</span> : null}
      <div className={styles.rowBody}>
        <div className={styles.rowHeader}>
          <span className={styles.rowTitle}>{title}</span>
          {meta ? <span className={styles.rowMeta}>{meta}</span> : null}
          {hasBody ? (
            <button
              aria-expanded={open}
              aria-label={open ? '收起详情' : '展开详情'}
              className={styles.rowToggle}
              type="button"
              onClick={(event) => {
                event.stopPropagation();
                toggleOpen();
              }}
            >
              <DownOutlined rotate={open ? 180 : 0} />
            </button>
          ) : null}
        </div>
        {extra ? <div className={styles.rowExtra}>{extra}</div> : null}
        {hasBody && open ? (
          <div className={`${styles.rowExpand} ${open ? styles.rowExpandOpen : ''}`}>
            <div className={styles.rowExpandInner}>{children}</div>
          </div>
        ) : null}
      </div>
    </div>
  );
}

// InlineExpandable is a lighter-weight expand/collapse affordance for
// content nested inside another interactive row (e.g. an agent task
// summary nested inside a clickable TraceRow). It stops propagation so it
// never triggers the parent row's click handler.
export function InlineExpandable({ children, className, summary }: { children: ReactNode; className?: string; summary: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <div className={[styles.inline, className].filter(Boolean).join(' ')}>
      <button
        aria-expanded={open}
        className={styles.inlineSummary}
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          setOpen((value) => !value);
        }}
      >
        {summary}
      </button>
      {open ? (
        <div className={`${styles.rowExpand} ${styles.rowExpandOpen}`}>
          <div className={styles.rowExpandInner}>{children}</div>
        </div>
      ) : null}
    </div>
  );
}
