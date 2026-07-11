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
  const groupedItems = compactProcessItems(props.items);
  const autoOpen = shouldAutoOpenProcess({ status: props.status, explorationStatus: props.exploration?.status, failedCount: props.exploration?.failedCount, itemStatuses: props.items.map((item) => item.status) });
  const [open, setOpen] = useLatchedOpen(autoOpen, props.turnId);
  return (
    <section className={styles.processTrace} data-testid="process-trace" data-process-label={processSummary(props)} data-process-status={props.status}>
      <Collapse ghost size="small" activeKey={open ? ['trace'] : []} expandIcon={({ isActive }) => <DownOutlined rotate={isActive ? 180 : 0} />} items={[{ key: 'trace', label: <ProcessLabel {...props} />, children: <div className={styles.processStream} data-testid="process-stream">{groupedItems.map((item) => <div key={item.id} className={styles.processStreamItem}>{props.renderItem(item)}</div>)}</div> }]} onChange={(keys) => setOpen(Array.isArray(keys) ? keys.includes('trace') : keys === 'trace')} />
    </section>
  );
}

function ProcessLabel(props: ProcessDisclosureProps) {
  const status = props.exploration?.status ?? props.status;
  const verb = useMinDisplay(explorationStatusVerb(status, props.exploration?.failedCount), 700);
  const duration = props.exploration?.elapsedMs ? formatElapsed(props.exploration.elapsedMs) : blockDuration(props);
  return <span className={styles.processTraceLabel} data-testid="process-trace-label" data-exploration-status={status}><span>{verb}</span>{duration ? <span>{duration}</span> : null}{props.exploration?.subagentCount ? <span>{props.exploration.subagentCount} 个子任务</span> : null}</span>;
}

function explorationStatusVerb(status?: string, failedCount?: number) {
  if (failedCount) return '部分失败';
  if (status === 'exploring' || status === 'running' || status === 'queued' || status === 'waiting_permission') return '正在探索';
  if (status === 'done' || status === 'completed' || status === 'success') return '已完成';
  if (status === 'failed') return '失败';
  if (status === 'interrupted') return '已中断';
  return '探索';
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
