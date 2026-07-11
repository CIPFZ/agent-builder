import { BranchesOutlined, CompressOutlined, LoadingOutlined, WarningOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { TraceRow } from './TraceRow.tsx';
import styles from './Timeline.module.css';

export function WorkflowNoticeRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const failed = item.status === 'failed' || item.status === 'interrupted' || Boolean(item.error);
  return <TraceRow dataAttrs={{ 'data-workflow-kind': item.kind, 'data-workflow-status': item.status }} extra={item.summary || item.error || item.content ? <span>{item.error || item.summary || item.content}</span> : null} icon={failed ? <WarningOutlined /> : <BranchesOutlined />} meta={item.status ? <Tag color={failed ? 'error' : 'default'}>{item.status}</Tag> : null} testId="timeline-workflow-row" title={item.title || workflowNoticeTitle(item.kind)} tone={failed ? 'error' : 'default'} />;
}

export function CompactTraceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const compact = item.compact;
  const failed = item.status === 'failed' || Boolean(item.error || compact?.error);
  const compacting = item.status === 'compacting' || item.status === 'started';
  const summary = compact?.summaryText || item.summary || item.content;
  const error = item.error || compact?.error;
  const meta = <>{(compact?.trigger || item.title) && <Tag>{compactTriggerLabel(compact?.trigger || item.title || '')}</Tag>}{item.status && <Tag color={failed ? 'error' : compacting ? 'processing' : 'success'}>{compactStatusLabel(item.status)}</Tag>}{!failed && !compacting && typeof compact?.preTokens === 'number' && typeof compact?.postTokens === 'number' && <span>{formatTokenCount(compact.preTokens)} -&gt; {formatTokenCount(compact.postTokens)} tokens</span>}{!failed && !compacting && typeof compact?.summarizedCount === 'number' && compact.summarizedCount > 0 && <span>{compact.summarizedCount} messages</span>}</>;
  return (
    <TraceRow expandable={Boolean(summary)} icon={failed ? <WarningOutlined /> : compacting ? <LoadingOutlined spin /> : <CompressOutlined />} meta={meta} testId="timeline-compact-row" title={compactTraceTitle(item, compacting, failed)} tone={failed ? 'error' : 'default'} extra={error ? <span>{error}</span> : null}>
      {summary ? <div className={styles.compactSummaryPanel}><div className={styles.compactSummaryLabel}>Summary retained for future turns</div><div className={styles.compactSummaryText}>{summary}</div></div> : null}
    </TraceRow>
  );
}

export function ContextGovernanceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const failed = item.status === 'failed' || Boolean(item.error);
  return <TraceRow dataAttrs={{ 'data-context-kind': item.kind, 'data-context-status': item.status }} extra={item.summary || item.error ? <span>{item.error || item.summary}</span> : null} icon={failed ? <WarningOutlined /> : <BranchesOutlined />} meta={item.status ? <Tag color={failed ? 'error' : 'default'}>{item.status}</Tag> : null} testId="timeline-context-governance-row" title={`Context source${item.title ? `: ${item.title}` : ''}`} tone={failed ? 'error' : 'default'} />;
}

export function TurnDiagnosticWarning({ item }: { item: ConversationTimelineItemViewModel }) {
  const missingArtifacts = item.diagnostics?.missingArtifacts ?? [];
  if (!item.diagnostics?.warning && !item.summary && missingArtifacts.length === 0) return null;
  return <TraceRow extra={<span>{missingArtifacts.length > 0 ? formatMissingArtifacts(missingArtifacts) : item.summary}</span>} icon={<WarningOutlined />} testId="turn-diagnostic-warning" title={diagnosticWarningTitle(item)} tone="warning" />;
}

function compactTraceTitle(item: ConversationTimelineItemViewModel, compacting: boolean, failed: boolean) {
  const trigger = compactTriggerLabel(item.compact?.trigger || item.title || '');
  if (failed) return trigger ? `${trigger} context compact failed` : 'Context compact failed';
  if (compacting) return trigger ? `${trigger} context compacting` : 'Compacting context';
  return trigger ? `${trigger} context compacted` : 'Context compacted';
}

function compactStatusLabel(status: string) {
  return status === 'started' ? 'running' : status === 'completed' ? 'done' : status;
}

function compactTriggerLabel(trigger: string) {
  return trigger === 'auto' ? 'Auto' : trigger === 'manual' ? 'Manual' : trigger === 'reactive' ? 'Reactive' : trigger;
}

function formatTokenCount(tokens: number) {
  return tokens >= 1000 ? `${(tokens / 1000).toFixed(tokens >= 100000 ? 0 : 1)}k` : `${tokens}`;
}

function workflowNoticeTitle(kind: string) {
  return kind === 'hook_run' ? 'Hook' : kind === 'todo_summary' ? 'Todo' : kind === 'recovery_notice' ? 'Recovery' : kind === 'turn_terminal' ? 'Turn ended' : 'Workflow';
}

function diagnosticWarningTitle(item: ConversationTimelineItemViewModel) {
  if (item.diagnostics?.warningReason === 'produced_artifact_missing_on_disk') return '工具已报告生成文件，但磁盘上不存在';
  if (item.diagnostics?.warningReason === 'expected_artifact_not_produced') return '期望文件未由工具生成';
  return item.diagnostics?.warning ?? '产物警告';
}

function formatMissingArtifacts(paths: string[]) {
  return paths.length === 1 ? paths[0] : `${paths.length} 个文件缺失：${paths.join('、')}`;
}
