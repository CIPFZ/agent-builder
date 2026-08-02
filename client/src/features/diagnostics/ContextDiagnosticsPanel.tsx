import { Alert, Button, Tag, Tooltip, Typography } from 'antd';
import { ApiOutlined, CompressOutlined, CopyOutlined, DatabaseOutlined, FileSearchOutlined, ProfileOutlined, ToolOutlined } from '@ant-design/icons';
import { useState, type ReactNode } from 'react';
import type { BudgetBucketViewModel, ContextCompactionStatusViewModel, ContextDiagnosticsViewModel, ContextUsageViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ContextDiagnosticsPanel.module.css';

const { Text } = Typography;

interface ContextDiagnosticsPanelProps {
  diagnostics?: ContextDiagnosticsViewModel;
  // Same source the composer's ContextUsageIndicator reads
  // (viewModel.composer.contextUsage) — passed down so the header's big
  // number/percentage share one source of truth instead of the indicator
  // and this panel drifting apart on separate estimates.
  contextUsage?: ContextUsageViewModel;
  compactionStatus?: ContextCompactionStatusViewModel;
  onManualCompact?: () => Promise<void>;
}

export function ContextDiagnosticsPanel({ diagnostics, contextUsage, compactionStatus, onManualCompact }: ContextDiagnosticsPanelProps) {
  const [runningAction, setRunningAction] = useState<'compact' | undefined>();
  if (!diagnostics) {
    return null;
  }
  const runAction = async (action: 'compact', handler?: () => Promise<void>) => {
    if (!handler || runningAction) {
      return;
    }
    setRunningAction(action);
    try {
      await handler();
    } finally {
      setRunningAction(undefined);
    }
  };

  return (
    <aside className={styles.panel} data-testid="context-diagnostics-panel" aria-label="上下文诊断">
      <div className={styles.header}>
        <span className={styles.heading}>
          <ProfileOutlined />
          <span>上下文诊断</span>
        </span>
        <div className={styles.headerActions}>
          <Tooltip title="手动整理上下文">
            <Button
              aria-label="手动整理上下文"
              disabled={!onManualCompact}
              icon={<CompressOutlined />}
              loading={runningAction === 'compact'}
              size="small"
              type="text"
              onClick={() => void runAction('compact', onManualCompact)}
            />
          </Tooltip>
          <Tooltip title="复制不含正文的诊断摘要">
            <Button aria-label="复制诊断摘要" icon={<CopyOutlined />} size="small" type="text" onClick={() => void copyDiagnosticSummary(diagnostics, contextUsage, compactionStatus)} />
          </Tooltip>
          <Tag color={diagnostics.system.redacted ? 'blue' : 'warning'}>{diagnostics.system.redacted ? 'summary' : 'check'}</Tag>
        </div>
      </div>

      {diagnostics.warnings.length ? (
        <Alert type="warning" showIcon message="上下文警告" description={diagnostics.warnings.join(' / ')} className={styles.alert} />
      ) : null}

      <div className={styles.modelLine}>
        <Text type="secondary">模型</Text>
        <Text className={styles.truncate}>{[diagnostics.provider, diagnostics.model].filter(Boolean).join(' / ') || '未知'}</Text>
      </div>
      {diagnostics.projectionId ? (
        <div className={styles.modelLine}>
          <Text type="secondary">投影</Text>
          <Text className={styles.truncate}>{diagnostics.projectionId}</Text>
        </div>
      ) : null}

      <div className={styles.metrics}>
        <Metric icon={<DatabaseOutlined />} label="当前预算" value={budgetHeaderValue(diagnostics, contextUsage)} />
        <Metric icon={<ToolOutlined />} label="工具" value={`${diagnostics.tools.selectedCount} 已选 / ${diagnostics.tools.omittedCount} 省略`} />
        <Metric icon={<FileSearchOutlined />} label="提示词" value={`${diagnostics.sections.length} 段`} />
      </div>

      {compactionStatus ? <CompactionOverview status={compactionStatus} /> : null}

      <Section title="当前预算">
        {diagnostics.sections.length ? (
          diagnostics.sections.map((section) => (
            <div key={section.id} className={styles.sourceRow}>
              <div className={styles.sourceTitle}>
                <Text strong className={styles.truncate}>{section.name}</Text>
                <Tag color={cachePolicyColor(section.cachePolicy)}>{section.cachePolicy}</Tag>
              </div>
              <Text type="secondary" className={styles.detailLine}>
                {[section.kind, section.role, section.source, section.scope, section.tokenEstimate ? `${section.tokenEstimate} tokens` : undefined].filter(Boolean).join(' / ')}
              </Text>
              <div className={styles.inlineTags}>
                <Tag color={section.redacted ? 'blue' : 'warning'}>{section.redacted ? 'redacted' : 'visible'}</Tag>
                <Tag color={section.rawStored ? 'error' : 'default'}>{section.rawStored ? 'raw stored' : 'summary only'}</Tag>
              </div>
              {section.diagnostics ? <Text type="secondary" className={styles.detailLine}>{section.diagnostics}</Text> : null}
              {section.hash ? <Text type="secondary" className={styles.hashLine}>{section.hash}</Text> : null}
            </div>
          ))
        ) : (
          <Text type="secondary">暂无提示词分段记录。</Text>
        )}
      </Section>

      <Section title="投影优化">
        <BudgetRow label="消息" bucket={diagnostics.budget.messages} />
        <BudgetRow label="上下文来源" bucket={diagnostics.budget.contextSources} />
        <BudgetRow label="工具定义" bucket={diagnostics.budget.selectedToolSchemas} />
        <BudgetRow label="工具输出" bucket={diagnostics.budget.toolOutputs} />
      </Section>

      <Section title="工具投影">
        <TagList values={diagnostics.tools.selected} fallback={`${diagnostics.tools.selectedCount} selected`} color="blue" />
        {diagnostics.tools.omitted.length ? <TagList values={diagnostics.tools.omitted} color="default" /> : null}
        <Text type="secondary" className={styles.detailLine}>
          results {diagnostics.tools.resultCount}, refs {diagnostics.tools.persistedResults}, compacted {diagnostics.tools.compactedResults}
        </Text>
      </Section>

      <Section title="Skills 与 MCP">
        <div className={styles.inlineTags}>
          <Tooltip title={diagnostics.skills.xmlHash || 'No skill instruction hash'}>
            <Tag color={diagnostics.skills.xmlPresent ? 'green' : 'default'}>{diagnostics.skills.loadedCount} skills</Tag>
          </Tooltip>
          <Tooltip title={mcpHashTooltip(diagnostics.mcp.serverListHash, diagnostics.mcp.instructionHash)}>
            <Tag color={diagnostics.mcp.instructionCount ? 'purple' : 'default'}>{diagnostics.mcp.serverCount} MCP servers</Tag>
          </Tooltip>
        </div>
        <TagList values={[...diagnostics.skills.loadedNames, ...diagnostics.mcp.servers]} fallback="No loaded instruction sources" />
      </Section>

      <Section title="上下文来源">
        {diagnostics.contextSources.length ? (
          diagnostics.contextSources.map((source) => (
            <div key={source.id} className={styles.sourceRow}>
              <div className={styles.sourceTitle}>
                <Text strong className={styles.truncate}>{source.name}</Text>
                <Tag color={source.state === 'loaded' ? 'success' : source.state === 'failed' ? 'error' : 'default'}>{source.state}</Tag>
              </div>
              <Text type="secondary" className={styles.detailLine}>
                {[source.kind, source.scope, source.tokenEstimate ? `${source.tokenEstimate} tokens` : undefined].filter(Boolean).join(' / ')}
              </Text>
              {source.error || source.reason ? <Text type={source.error ? 'danger' : 'secondary'} className={styles.detailLine}>{source.error || source.reason}</Text> : null}
              {source.contentHash ? <Text type="secondary" className={styles.hashLine}>{source.contentHash}</Text> : null}
            </div>
          ))
        ) : (
          <Text type="secondary">暂无上下文来源记录。</Text>
        )}
      </Section>

      {diagnostics.compactBoundaries.length ? (
        <Section title="最近压缩">
          {diagnostics.compactBoundaries.map((boundary) => (
            <div key={boundary.id} className={styles.compactRow}>
              <CompressOutlined />
              <div className={styles.compactBody}>
                <Text strong className={styles.truncate}>{boundary.kind}</Text>
                <Text type="secondary" className={styles.detailLine}>
                  {[boundary.trigger, boundary.status, boundary.summaryRef].filter(Boolean).join(' / ')}
                </Text>
                <Text type="secondary" className={styles.detailLine}>
                  messages {boundary.messageRefs.length}, tools {boundary.toolCallRefCount}, refs {boundary.reinjectedRefCount}
                </Text>
              </div>
            </div>
          ))}
        </Section>
      ) : null}

      {diagnostics.snipBoundaries.length || diagnostics.replacements.length || diagnostics.reactiveAttempts.length ? (
        <Section title="恢复尝试">
          {diagnostics.snipBoundaries.map((boundary) => (
            <div key={boundary.id} className={styles.compactRow}>
              <CompressOutlined />
              <div className={styles.compactBody}>
                <Text strong className={styles.truncate}>snip</Text>
                <Text type="secondary" className={styles.detailLine}>
                  {[boundary.reason, `${boundary.removedMessageCount} removed`, boundary.summaryRef].filter(Boolean).join(' / ')}
                </Text>
              </div>
            </div>
          ))}
          {diagnostics.replacements.length ? (
            <Text type="secondary" className={styles.detailLine}>
              replacements {diagnostics.replacements.length}
            </Text>
          ) : null}
          {diagnostics.reactiveAttempts.map((attempt) => (
            <Text key={attempt.id} type={attempt.status === 'failed' ? 'danger' : 'secondary'} className={styles.detailLine}>
              reactive #{attempt.attempt} {attempt.action} / {attempt.status}
            </Text>
          ))}
        </Section>
      ) : null}

      <div className={styles.footer}>
        <ApiOutlined />
        <Text type="secondary" className={styles.truncate}>
          step {diagnostics.step ?? 0}, system {diagnostics.system.hash || 'unhashed'}
        </Text>
      </div>
    </aside>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className={styles.section}>
      <Text type="secondary" className={styles.sectionTitle}>{title}</Text>
      {children}
    </section>
  );
}

function CompactionOverview({ status }: { status: ContextCompactionStatusViewModel }) {
  const completed = status.latestCompleted;
  const memory = status.latestSessionMemory;
  return (
    <>
      <Section title="最近压缩">
        {status.activeOperation ? <Text>正在整理上下文：{status.activeOperation.kind} / {status.activeOperation.trigger}</Text> : null}
        {completed ? (
          <Text type="secondary" className={styles.detailLine}>
            {completed.kind} / {completed.trigger}，{completed.preTokens} → {completed.postTokens} tokens，节省 {completed.savedTokens}
          </Text>
        ) : <Text type="secondary">尚无已完成的压缩。</Text>}
        {status.latestFailed ? <Text type="danger" className={styles.detailLine}>最近失败：{status.latestFailed.error || status.latestFailed.kind}</Text> : null}
        {status.circuitOpen ? <Alert type="error" showIcon message="自动压缩已暂停" description={`连续失败 ${status.consecutiveFailures} 次，可手动压缩或切换模型。`} /> : null}
      </Section>
      <Section title="Session Memory">
        {memory ? (
          <Text type={memory.status === 'failed' ? 'danger' : 'secondary'} className={styles.detailLine}>
            revision {memory.revision} / {memory.status}，覆盖 {memory.sourceMessageCount} 条消息，约 {memory.sourceTokenEstimate} tokens
          </Text>
        ) : <Text type="secondary">尚未生成 Session Memory。</Text>}
      </Section>
      <Section title="恢复尝试">
        <Text type="secondary">{status.circuitOpen ? '熔断已打开，Runtime 不会继续消耗摘要模型调用。' : '恢复链路可用：投影缩减 → Session Memory / Full Compact。'}</Text>
      </Section>
    </>
  );
}

async function copyDiagnosticSummary(
  diagnostics: ContextDiagnosticsViewModel,
  usage?: ContextUsageViewModel,
  status?: ContextCompactionStatusViewModel,
) {
  const summary = [
    `模型: ${[diagnostics.provider, diagnostics.model].filter(Boolean).join(' / ') || '未知'}`,
    `预算: ${usage ? `${usage.usedTokens}/${usage.contextWindow} (${usage.percentUsed}%)` : diagnostics.budget.totalEstimatedTokens}`,
    `压缩: ${status?.latestCompleted ? `${status.latestCompleted.kind}/${status.latestCompleted.trigger}, saved=${status.latestCompleted.savedTokens}` : 'none'}`,
    `Session Memory: ${status?.latestSessionMemory ? `revision=${status.latestSessionMemory.revision}, status=${status.latestSessionMemory.status}` : 'none'}`,
    `恢复: circuit_open=${Boolean(status?.circuitOpen)}, failures=${status?.consecutiveFailures ?? 0}`,
    `投影: replacements=${diagnostics.replacements.length}, attempts=${diagnostics.reactiveAttempts.length}`,
  ].join('\n');
  await navigator.clipboard?.writeText(summary);
}

function Metric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className={styles.metric}>
      <span className={styles.metricIcon}>{icon}</span>
      <Text type="secondary">{label}</Text>
      <Text strong className={styles.truncate}>{value}</Text>
    </div>
  );
}

function BudgetRow({ label, bucket }: { label: string; bucket?: BudgetBucketViewModel }) {
  return (
    <div className={styles.budgetRow}>
      <Text type="secondary">{label}</Text>
      <Text>{bucket ? `${bucket.count} / ${bucket.estimatedTokens}` : '0 / 0'}</Text>
    </div>
  );
}

// budgetHeaderValue prefers the session's live context usage (the same
// source ContextUsageIndicator reads) for the header's big number and
// percentage, since it reflects the anchored, post-boundary token count
// rather than this single prompt-assembly snapshot's estimate. It falls back
// to the assembly's own budget total when usage isn't available yet (e.g.
// before the first assistant step completes).
function budgetHeaderValue(diagnostics: ContextDiagnosticsViewModel, contextUsage?: ContextUsageViewModel) {
  if (contextUsage) {
    return `${contextUsage.usedTokens} / ${contextUsage.contextWindow} tokens (${contextUsage.percentUsed}%)`;
  }
  return `${diagnostics.budget.totalEstimatedTokens || 0} tokens`;
}

function cachePolicyColor(policy: string) {
  switch (policy) {
    case 'stable':
      return 'green';
    case 'session_cached':
      return 'cyan';
    case 'uncached':
      return 'volcano';
    default:
      return 'gold';
  }
}

function mcpHashTooltip(serverListHash?: string, instructionHash?: string) {
  if (!serverListHash && !instructionHash) {
    return 'No MCP hashes recorded';
  }
  return [serverListHash ? `servers ${serverListHash}` : undefined, instructionHash ? `instructions ${instructionHash}` : undefined].filter(Boolean).join(' / ');
}

function TagList({ values, fallback, color }: { values: string[]; fallback?: string; color?: string }) {
  if (!values.length) {
    return fallback ? <Text type="secondary">{fallback}</Text> : null;
  }
  return (
    <div className={styles.inlineTags}>
      {values.slice(0, 12).map((value) => (
        <Tag key={value} color={color}>{value}</Tag>
      ))}
      {values.length > 12 ? <Tag>+{values.length - 12}</Tag> : null}
    </div>
  );
}
