import { useLayoutEffect, useReducer, type ReactNode } from 'react';
import { RightOutlined } from '@ant-design/icons';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import type { RuntimeExplorationSummary } from '../../runtime/conversationPresentationTypes.ts';
import { compactProcessItems } from './processGrouping.ts';
import type { RenderTimelineItem } from './processGrouping.ts';
import { initialProcessDisclosureState, isActiveProcessStatus, reduceProcessDisclosure, type ProcessDisclosureSignal } from './processDisclosurePolicy.ts';
import styles from './Timeline.module.css';

interface ProcessDisclosureProps {
  turnId?: string;
  status?: string;
  startedAt?: number;
  finishedAt?: number;
  exploration?: RuntimeExplorationSummary;
  hasFinalResponse: boolean;
  items: ConversationTimelineItemViewModel[];
  renderItem: (item: RenderTimelineItem) => ReactNode;
}

export function ProcessDisclosure(props: ProcessDisclosureProps) {
  const detailItems = props.items.filter((item) => !isRedundantActivePlaceholder(item));
  const groupedItems = compactProcessItems(detailItems);
  const itemStatusKey = detailItems.map((item) => item.status).join('|');
  const hasPendingPermission = detailItems.some((item) => (item.kind === 'permission' || item.kind === 'permission_request') && isPendingStatus(item.permission?.status ?? item.status));
  const explorationStatus = props.exploration?.status;
  const signal: ProcessDisclosureSignal = {
    status: props.status,
    hasFinalResponse: props.hasFinalResponse,
    explorationStatus,
    itemStatuses: itemStatusKey.split('|'),
    hasPendingPermission,
  };
  const [disclosure, dispatch] = useReducer(reduceProcessDisclosure, signal, initialProcessDisclosureState);
  useLayoutEffect(() => {
    dispatch({ type: 'sync', signal: { status: props.status, hasFinalResponse: props.hasFinalResponse, explorationStatus, itemStatuses: itemStatusKey.split('|'), hasPendingPermission } });
  }, [props.status, props.hasFinalResponse, explorationStatus, itemStatusKey, hasPendingPermission]);
  const sectionProps = {
    className: styles.processTrace,
    'data-testid': 'process-trace',
    'data-process-label': [processLabel(props.status), props.turnId].filter(Boolean).join(' '),
    'data-process-status': props.status,
    'data-process-disclosure-mode': disclosure.mode,
    'data-process-open': disclosure.open,
  };
  if (groupedItems.length === 0) {
    return <section {...sectionProps}><div className={styles.processTraceStandalone}><ProcessLabel {...props} /></div></section>;
  }
  return (
    <section {...sectionProps}>
      <button className={styles.processTraceHeader} type="button" aria-expanded={disclosure.open} onClick={() => dispatch({ type: 'manual', open: !disclosure.open })}><ProcessLabel {...props} /><RightOutlined className={styles.processTraceChevron} aria-hidden="true" /></button>
      <div className={styles.processStream} data-testid="process-stream" hidden={!disclosure.open}>
        {groupedItems.map((item) => <div key={item.id} className={styles.processStreamItem}>{props.renderItem(item)}</div>)}
      </div>
    </section>
  );
}

function isPendingStatus(status?: string) { return status === 'pending' || status === 'requested' || status === 'waiting' || status === 'waiting_permission'; }

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
