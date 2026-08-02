import { BranchesOutlined, CompressOutlined, LoadingOutlined, WarningOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import { boundedText } from '../../runtime/boundedText.ts';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { InlineExpandable, TraceRow } from './TraceRow.tsx';
import styles from './Timeline.module.css';

export function WorkflowNoticeRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const failed = item.status === 'failed' || item.status === 'interrupted' || Boolean(item.error);
  return <TraceRow dataAttrs={{ 'data-workflow-kind': item.kind, 'data-workflow-status': item.status }} extra={<BoundedNoticeText text={item.error || item.summary || item.content} />} icon={failed ? <WarningOutlined /> : <BranchesOutlined />} meta={item.status ? <Tag color={failed ? 'error' : 'default'}>{item.status}</Tag> : null} testId="timeline-workflow-row" title={item.title || workflowNoticeTitle(item.kind)} tone={failed ? 'error' : 'default'} />;
}

export function CompactTraceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const compact = item.compact;
  const failed = item.status === 'failed' || Boolean(item.error || compact?.error);
  const compacting = item.status === 'compacting' || item.status === 'started';
  const error = item.error || compact?.error;
  const meta = <>{(compact?.trigger || item.title) && <Tag>{compactTriggerLabel(compact?.trigger || item.title || '')}</Tag>}{item.status && <Tag color={failed ? 'error' : compacting ? 'processing' : 'success'}>{compactStatusLabel(item.status)}</Tag>}{!failed && !compacting && typeof compact?.preTokens === 'number' && typeof compact?.postTokens === 'number' && <span>{formatTokenCount(compact.preTokens)} -&gt; {formatTokenCount(compact.postTokens)} tokens</span>}{!failed && !compacting && typeof compact?.summarizedCount === 'number' && compact.summarizedCount > 0 && <span>{compact.summarizedCount} messages</span>}</>;
  return (
    <TraceRow expandable={!compacting} icon={failed ? <WarningOutlined /> : compacting ? <LoadingOutlined spin /> : <CompressOutlined />} meta={meta} testId="timeline-compact-row" title={compactTraceTitle(item, compacting, failed)} tone={failed ? 'error' : 'default'} extra={error ? <span>{error}</span> : null}>
      {!compacting ? <div className={styles.compactSummaryPanel}><div className={styles.compactSummaryLabel}>压缩详情</div><span>{compactDetail(compact?.trigger, item.status, compact?.preTokens, compact?.postTokens, compact?.summarizedCount)}</span></div> : null}
    </TraceRow>
  );
}

export function ContextGovernanceRow({ item }: { item: ConversationTimelineItemViewModel }) {
  const failed = item.status === 'failed' || Boolean(item.error);
  return <TraceRow dataAttrs={{ 'data-context-kind': item.kind, 'data-context-status': item.status }} extra={<BoundedNoticeText text={item.error || item.summary} />} icon={failed ? <WarningOutlined /> : <BranchesOutlined />} meta={item.status ? <Tag color={failed ? 'error' : 'default'}>{item.status}</Tag> : null} testId="timeline-context-governance-row" title={`Context source${item.title ? `: ${item.title}` : ''}`} tone={failed ? 'error' : 'default'} />;
}

function BoundedNoticeText({ text }: { text?: string }) {
  if (!text) return null;
  const preview = boundedText(text, 6, 900);
  if (!preview.truncated) return <span>{preview.text}</span>;
  return <InlineExpandable className={styles.noticeText} summary={preview.text}><div className={styles.noticeFullText}>{text}</div></InlineExpandable>;
}

export function TurnDiagnosticWarning({ item }: { item: ConversationTimelineItemViewModel }) {
  const missingArtifacts = item.diagnostics?.missingArtifacts ?? [];
  if (!item.diagnostics?.warning && !item.summary && missingArtifacts.length === 0) return null;
  return <TraceRow extra={<span>{missingArtifacts.length > 0 ? formatMissingArtifacts(missingArtifacts) : item.summary}</span>} icon={<WarningOutlined />} testId="turn-diagnostic-warning" title={diagnosticWarningTitle(item)} tone="warning" />;
}

function compactTraceTitle(item: ConversationTimelineItemViewModel, compacting: boolean, failed: boolean) {
  const trigger = compactTriggerLabel(item.compact?.trigger || item.title || '');
  if (failed) return trigger ? `${trigger}上下文压缩失败` : '上下文压缩失败';
  if (compacting) return item.compact?.trigger === 'reactive' ? '上下文超限，正在缩减并重试' : '正在整理上下文';
  return trigger ? `${trigger}上下文已压缩` : '上下文已压缩';
}

function compactStatusLabel(status: string) {
  return status === 'started' ? '进行中' : status === 'completed' ? '已完成' : status === 'failed' ? '失败' : status;
}

function compactTriggerLabel(trigger: string) {
  return trigger === 'auto' ? '自动' : trigger === 'manual' ? '手动' : trigger === 'reactive' ? '恢复' : trigger;
}

function compactDetail(trigger?: string, status?: string, preTokens?: number, postTokens?: number, summarizedCount?: number) {
  return [
    `类型：${compactTriggerLabel(trigger || '') || '上下文压缩'}`,
    `结果：${compactStatusLabel(status || 'completed')}`,
    typeof preTokens === 'number' && typeof postTokens === 'number' ? `Token：${formatTokenCount(preTokens)} → ${formatTokenCount(postTokens)}` : undefined,
    typeof summarizedCount === 'number' ? `摘要消息：${summarizedCount}` : undefined,
  ].filter(Boolean).join('；');
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
