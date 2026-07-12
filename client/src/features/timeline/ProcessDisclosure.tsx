import type { ReactNode } from 'react';
import { DownOutlined } from '@ant-design/icons';
import { Collapse } from 'antd';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import type { RuntimeExplorationSummary } from '../../runtime/outputTypes.ts';
import { useLatchedOpen, useMinDisplay } from './hooks.ts';
import { compactProcessItems } from './processGrouping.ts';
import type { RenderTimelineItem } from './processGrouping.ts';
import { isActiveProcessStatus, shouldAutoOpenProcess } from './processDisclosurePolicy.ts';
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
  const autoOpen = shouldAutoOpenProcess({ status: props.status, explorationStatus: props.exploration?.status, itemStatuses: props.items.map((item) => item.status) });
  const [open, setOpen] = useLatchedOpen(autoOpen, props.turnId);
  const sectionProps = {
    className: styles.processTrace,
    'data-testid': 'process-trace',
    'data-process-label': processSummary(props),
    'data-process-status': props.status,
    'data-process-has-failures': (props.exploration?.failedCount ?? 0) > 0 ? 'true' : undefined,
  };
  if (groupedItems.length === 0) {
    return <section {...sectionProps}><div className={styles.processTraceStandalone}><ProcessLabel {...props} /></div></section>;
  }
  return (
    <section {...sectionProps}>
      <Collapse ghost size="small" activeKey={open ? ['trace'] : []} expandIcon={({ isActive }) => <DownOutlined rotate={isActive ? 180 : 0} />} items={[{ key: 'trace', label: <ProcessLabel {...props} />, children: <div className={styles.processStream} data-testid="process-stream">{groupedItems.map((item) => <div key={item.id} className={styles.processStreamItem}>{props.renderItem(item)}</div>)}</div> }]} onChange={(keys) => setOpen(Array.isArray(keys) ? keys.includes('trace') : keys === 'trace')} />
    </section>
  );
}

function ProcessLabel(props: ProcessDisclosureProps) {
  const status = props.exploration?.status ?? props.status;
  const verb = useMinDisplay(processStatusVerb(props), 700);
  const duration = props.exploration?.elapsedMs ? formatElapsed(props.exploration.elapsedMs) : blockDuration(props);
  return <span className={styles.processTraceLabel} data-testid="process-trace-label" data-exploration-status={status}><span>{verb}</span>{duration ? <span>{duration}</span> : null}{props.exploration?.subagentCount ? <span>{props.exploration.subagentCount} 个子任务</span> : null}</span>;
}

function processStatusVerb(props: ProcessDisclosureProps) {
  const status = props.exploration?.status ?? props.status;
  if (props.exploration?.failedCount) return '部分失败';
  if (status === 'waiting_permission' || props.items.some((item) => item.status === 'waiting_permission')) return '等待确认';
  if (props.items.some((item) => (item.kind === 'tool_call' || item.kind === 'tool_group') && isActiveProcessStatus(item.status))) return '正在使用工具';
  if (props.items.some((item) => (item.kind === 'assistant_message' || item.kind === 'message') && (item.status === 'running' || item.status === 'streaming'))) return '正在组织回复';
  if (status === 'exploring' || status === 'running' || status === 'queued') return '正在思考';
  if (status === 'done' || status === 'completed' || status === 'success') return '已完成';
  if (status === 'failed') return '失败';
  if (status === 'interrupted') return '已中断';
  if (status === 'cancelled') return '已取消';
  return '处理过程';
}

function isRedundantActivePlaceholder(item: ConversationTimelineItemViewModel) {
  if (!isActiveProcessStatus(item.status)) return false;
  if (item.kind === 'progress' || item.kind === 'turn_progress') return true;
  const isNarration = item.kind === 'thinking' || item.kind === 'assistant_thinking' || item.kind === 'message' || item.kind === 'assistant_message';
  return isNarration && !item.content?.trim() && !(item.source === 'react_callchain' && item.title);
}

function processSummary(props: ProcessDisclosureProps) {
  return [processLabel(props.status), blockDuration(props), props.turnId].filter(Boolean).join(' ');
}

function processLabel(status?: string) {
  if (isActiveProcessStatus(status)) return '处理中';
  if (status === 'failed') return '处理失败';
  if (status === 'denied') return '已拒绝';
  if (status === 'cancelled') return '已取消';
  return '已处理';
}

function blockDuration(props: ProcessDisclosureProps) {
  return props.startedAt && props.finishedAt ? formatDuration(props.startedAt, props.finishedAt) : '';
}

function formatElapsed(ms: number) {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60_000)}m${Math.round((ms % 60_000) / 1000)}s`;
}

function formatDuration(startedAt: number, finishedAt: number) {
  const elapsed = Math.max(0, normalizeTimestamp(finishedAt) - normalizeTimestamp(startedAt));
  if (elapsed < 1000) return '<1s';
  if (elapsed < 60_000) return `${Math.round(elapsed / 1000)}s`;
  return `${Math.floor(elapsed / 60_000)}m ${Math.round((elapsed % 60_000) / 1000)}s`;
}

function normalizeTimestamp(value: number) { return value < 1_000_000_000_000 ? value * 1000 : value; }
