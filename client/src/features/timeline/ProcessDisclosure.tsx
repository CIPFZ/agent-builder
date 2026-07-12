import type { ReactNode } from 'react';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import type { RuntimeExplorationSummary } from '../../runtime/conversationPresentationTypes.ts';
import { compactProcessItems } from './processGrouping.ts';
import type { RenderTimelineItem } from './processGrouping.ts';
import { isActiveProcessStatus } from './processDisclosurePolicy.ts';
import styles from './Timeline.module.css';

interface ProcessDisclosureProps {
  turnId?: string;
  status?: string;
  startedAt?: number;
  finishedAt?: number;
  exploration?: RuntimeExplorationSummary;
  items: ConversationTimelineItemViewModel[];
  renderItem: (item: RenderTimelineItem) => ReactNode;
}

export function ProcessDisclosure(props: ProcessDisclosureProps) {
  const detailItems = props.items.filter((item) => !isRedundantActivePlaceholder(item));
  const groupedItems = compactProcessItems(detailItems);
  const sectionProps = {
    className: styles.processTrace,
    'data-testid': 'process-trace',
    'data-process-label': [processLabel(props.status), props.turnId].filter(Boolean).join(' '),
    'data-process-status': props.status,
  };
  if (groupedItems.length === 0) {
    return <section {...sectionProps}><div className={styles.processTraceStandalone}><ProcessLabel {...props} /></div></section>;
  }
  return (
    <section {...sectionProps}>
      <div className={styles.processTraceHeader}><ProcessLabel {...props} /></div>
      <div className={styles.processStream} data-testid="process-stream">
        {groupedItems.map((item) => <div key={item.id} className={styles.processStreamItem}>{props.renderItem(item)}</div>)}
      </div>
    </section>
  );
}

function ProcessLabel(props: ProcessDisclosureProps) {
  const status = props.exploration?.status ?? props.status;
  return <span className={styles.processTraceLabel} data-testid="process-trace-label" data-exploration-status={status}><span>{isActiveProcessStatus(props.status) ? '处理中' : '处理完成'}</span></span>;
}

function isRedundantActivePlaceholder(item: ConversationTimelineItemViewModel) {
  if (!isActiveProcessStatus(item.status)) return false;
  if (item.kind === 'progress' || item.kind === 'turn_progress') return true;
  const isNarration = item.kind === 'thinking' || item.kind === 'assistant_thinking' || item.kind === 'message' || item.kind === 'assistant_message';
  return isNarration && !item.content?.trim() && !(item.source === 'react_callchain' && item.title);
}

function processLabel(status?: string) {
  return isActiveProcessStatus(status) ? '处理中' : '处理完成';
}
