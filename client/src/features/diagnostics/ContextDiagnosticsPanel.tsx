import { Alert, Tag, Tooltip, Typography } from 'antd';
import { ApiOutlined, CompressOutlined, DatabaseOutlined, FileSearchOutlined, ProfileOutlined, ToolOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';
import type { BudgetBucketViewModel, ContextDiagnosticsViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ContextDiagnosticsPanel.module.css';

const { Text } = Typography;

interface ContextDiagnosticsPanelProps {
  diagnostics?: ContextDiagnosticsViewModel;
}

export function ContextDiagnosticsPanel({ diagnostics }: ContextDiagnosticsPanelProps) {
  if (!diagnostics) {
    return null;
  }

  return (
    <aside className={styles.panel} data-testid="context-diagnostics-panel" aria-label="Context diagnostics">
      <div className={styles.header}>
        <span className={styles.heading}>
          <ProfileOutlined />
          <span>Context</span>
        </span>
        <Tag color={diagnostics.system.redacted ? 'blue' : 'warning'}>{diagnostics.system.redacted ? 'summary' : 'check'}</Tag>
      </div>

      {diagnostics.warnings.length ? (
        <Alert type="warning" showIcon message="Context warnings" description={diagnostics.warnings.join(' / ')} className={styles.alert} />
      ) : null}

      <div className={styles.modelLine}>
        <Text type="secondary">Model</Text>
        <Text className={styles.truncate}>{[diagnostics.provider, diagnostics.model].filter(Boolean).join(' / ') || 'unknown'}</Text>
      </div>

      <div className={styles.metrics}>
        <Metric icon={<DatabaseOutlined />} label="Budget" value={`${diagnostics.budget.totalEstimatedTokens || 0} tokens`} />
        <Metric icon={<ToolOutlined />} label="Tools" value={`${diagnostics.tools.selectedCount} selected / ${diagnostics.tools.omittedCount} omitted`} />
        <Metric icon={<FileSearchOutlined />} label="Sources" value={`${diagnostics.contextSources.length} context`} />
      </div>

      <Section title="Budget">
        <BudgetRow label="Messages" bucket={diagnostics.budget.messages} />
        <BudgetRow label="Context" bucket={diagnostics.budget.contextSources} />
        <BudgetRow label="Tools" bucket={diagnostics.budget.selectedToolSchemas} />
        <BudgetRow label="Outputs" bucket={diagnostics.budget.toolOutputs} />
      </Section>

      <Section title="Tools">
        <TagList values={diagnostics.tools.selected} fallback={`${diagnostics.tools.selectedCount} selected`} color="blue" />
        {diagnostics.tools.omitted.length ? <TagList values={diagnostics.tools.omitted} color="default" /> : null}
        <Text type="secondary" className={styles.detailLine}>
          results {diagnostics.tools.resultCount}, refs {diagnostics.tools.persistedResults}, compacted {diagnostics.tools.compactedResults}
        </Text>
      </Section>

      <Section title="Skills and MCP">
        <div className={styles.inlineTags}>
          <Tooltip title={diagnostics.skills.xmlHash || 'No skill instruction hash'}>
            <Tag color={diagnostics.skills.xmlPresent ? 'green' : 'default'}>{diagnostics.skills.loadedCount} skills</Tag>
          </Tooltip>
          <Tooltip title={diagnostics.mcp.instructionHash || 'No MCP instruction hash'}>
            <Tag color={diagnostics.mcp.instructionCount ? 'purple' : 'default'}>{diagnostics.mcp.serverCount} MCP servers</Tag>
          </Tooltip>
        </div>
        <TagList values={[...diagnostics.skills.loadedNames, ...diagnostics.mcp.servers]} fallback="No loaded instruction sources" />
      </Section>

      <Section title="Context sources">
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
          <Text type="secondary">No context sources recorded.</Text>
        )}
      </Section>

      {diagnostics.compactBoundaries.length ? (
        <Section title="Compact">
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
