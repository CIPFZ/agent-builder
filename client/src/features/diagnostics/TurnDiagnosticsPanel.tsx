import { AlertOutlined, CheckOutlined, ClockCircleOutlined, CopyOutlined, FileDoneOutlined, SearchOutlined, ToolOutlined } from '@ant-design/icons';
import { Button, Tag, Tooltip, Typography } from 'antd';
import { useState } from 'react';
import type React from 'react';
import type { HookExecutionSummaryViewModel, HookExecutionViewModel, InterruptedToolViewModel, InterruptedTurnViewModel, TurnDiagnosticsViewModel } from '../../runtime/workbenchTypes.ts';
import { HookExecutionsPanel } from '../hooks/HookExecutionsPanel.tsx';
import styles from './TurnDiagnosticsPanel.module.css';

const { Text } = Typography;

export function TurnDiagnosticsPanel({
  diagnostics,
  interrupted,
  onInterruptedCopy,
  onInterruptedDone,
  onInterruptedFollowUp,
  hookExecutions,
  onHookExecutionLoad,
}: {
  diagnostics?: TurnDiagnosticsViewModel;
  interrupted?: InterruptedTurnViewModel;
  hookExecutions?: HookExecutionSummaryViewModel;
  onHookExecutionLoad?: (executionId: string) => Promise<HookExecutionViewModel>;
  onInterruptedCopy?: (summary: string) => Promise<void> | void;
  onInterruptedDone?: (turnId: string) => Promise<void> | void;
  onInterruptedFollowUp?: (summary: string) => Promise<void> | void;
}) {
  const [interruptedExpanded, setInterruptedExpanded] = useState(false);
  if (!diagnostics && !interrupted && !hookExecutions) {
    return null;
  }
  const status = diagnostics?.status || interrupted?.status || 'unknown';
  const failureSignals =
    (diagnostics?.failedToolCount ?? interrupted?.failedToolCount ?? 0) +
    (diagnostics?.deniedToolCount ?? interrupted?.deniedToolCount ?? 0) +
    (diagnostics?.cancelledToolCount ?? interrupted?.cancelledToolCount ?? 0) +
    (diagnostics?.nonzeroExitShellCount ?? interrupted?.nonzeroExitShellCount ?? 0);
  const duration = diagnostics?.runningDurationMs || diagnostics?.durationMs || interrupted?.durationMs;
  const interruptedSummary = interruptedSummaryText(interrupted);
  return (
    <aside className={styles.panel} data-testid="turn-diagnostics-panel" aria-label="Turn diagnostics">
      <div className={styles.header}>
        <div className={styles.heading}>
          <AlertOutlined />
          <span>Turn diagnostics</span>
        </div>
        <Tag color={statusColor(status)}>{statusLabel(status)}</Tag>
      </div>

      {interrupted ? (
        <div className={styles.interrupted} data-testid="interrupted-recovery-surface">
          <div className={styles.interruptedHeader}>
            <div className={styles.interruptedTitle}>
              <AlertOutlined />
              <span>Interrupted recovery</span>
            </div>
            <Tag color="red">{statusLabel(interrupted.status || 'interrupted')}</Tag>
          </div>
          <Text type="secondary">{[interrupted.reason, interrupted.source].filter(Boolean).join(' / ') || 'runtime recovery'}</Text>
          <div className={styles.compactRows}>
            <CompactRow label="Last tool" value={toolSummary(interrupted.lastCompletedTool) || 'none'} />
            <CompactRow label="Pending" value={toolSummary(interrupted.pendingTool) || 'none'} />
            <CompactRow label="Signals" value={interruptedSignals(interrupted)} />
            <CompactRow label="Artifacts" value={interruptedArtifacts(interrupted)} />
            <CompactRow label="Permissions" value={interruptedPermissions(interrupted)} />
            <CompactRow label="Last event" value={interruptedEvent(interrupted)} />
          </div>
          {interruptedExpanded ? (
            <div className={styles.inspectBlock}>
              <ToolInspect title="Last completed" tool={interrupted.lastCompletedTool} />
              <ToolInspect title="Last failed" tool={interrupted.lastFailedTool} />
              <ToolInspect title="Pending at interruption" tool={interrupted.pendingTool} />
              {interrupted.missingArtifacts?.length ? <PathList paths={interrupted.missingArtifacts} /> : null}
            </div>
          ) : null}
          <div className={styles.actions}>
            <Button icon={<SearchOutlined />} size="small" onClick={() => setInterruptedExpanded((value) => !value)}>
              Inspect
            </Button>
            <Button icon={<CopyOutlined />} size="small" onClick={() => onInterruptedCopy?.(interruptedSummary)}>
              Copy
            </Button>
            <Button size="small" onClick={() => onInterruptedFollowUp?.(followUpPrompt(interruptedSummary))}>
              Follow-up
            </Button>
            <Button icon={<CheckOutlined />} size="small" onClick={() => interrupted.turnId && onInterruptedDone?.(interrupted.turnId)}>
              Mark done
            </Button>
          </div>
        </div>
      ) : null}

      <div className={styles.grid}>
        <Metric icon={<ClockCircleOutlined />} label="Duration" value={formatDuration(duration)} />
        <Metric icon={<ToolOutlined />} label="Tools" value={formatCountMap(diagnostics?.toolCountsByStatus)} />
        <Metric icon={<ToolOutlined />} label="Kinds" value={formatCountMap(diagnostics?.toolCountsByKind)} />
        <Metric icon={<FileDoneOutlined />} label="Artifacts" value={diagnostics ? artifactSummary(diagnostics) : interruptedArtifacts(interrupted)} />
      </div>

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Signals</div>
        <div className={styles.tags}>
          <SignalTag label="failed" value={diagnostics?.failedToolCount ?? interrupted?.failedToolCount} danger />
          <SignalTag label="denied" value={diagnostics?.deniedToolCount ?? interrupted?.deniedToolCount} danger />
          <SignalTag label="cancelled" value={diagnostics?.cancelledToolCount ?? interrupted?.cancelledToolCount} />
          <SignalTag label="nonzero shell" value={diagnostics?.nonzeroExitShellCount ?? interrupted?.nonzeroExitShellCount} danger />
          {failureSignals === 0 ? <Tag>none</Tag> : null}
        </div>
      </div>

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Permissions</div>
        <div className={styles.tags}>
          <SignalTag label="pending" value={(diagnostics?.permissionCounts ?? interrupted?.permissionCounts)?.pending} />
          <SignalTag label="allowed" value={(diagnostics?.permissionCounts ?? interrupted?.permissionCounts)?.allowed} />
          <SignalTag label="denied" value={(diagnostics?.permissionCounts ?? interrupted?.permissionCounts)?.denied} danger />
          <SignalTag label="expired" value={(diagnostics?.permissionCounts ?? interrupted?.permissionCounts)?.expired} />
          <SignalTag label="cancelled" value={(diagnostics?.permissionCounts ?? interrupted?.permissionCounts)?.cancelled} />
        </div>
      </div>

      <HookExecutionsPanel summary={hookExecutions} onLoadExecution={onHookExecutionLoad} />

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Artifact confidence</div>
        <div className={styles.tags}>
          <SignalTag label="verified local" value={diagnostics?.artifactConfidenceSummary?.localVerifiedFile} />
          <SignalTag label="tool metadata" value={diagnostics?.artifactConfidenceSummary?.producedToolMetadata} />
          <SignalTag label="runtime refs" value={diagnostics?.artifactConfidenceSummary?.runtimeOutputRefs} />
          <SignalTag label="structured refs" value={diagnostics?.artifactConfidenceSummary?.structuredMcpCustomRefs} />
          <SignalTag label="unknown" value={diagnostics?.artifactConfidenceSummary?.unknownNotDetected} danger />
        </div>
      </div>

      {diagnostics?.warning ? (
        <div className={styles.warning}>
          <Text strong>{diagnostics.warning}</Text>
          <Text type="secondary">{[diagnostics.warningReason, diagnostics.warningSource].filter(Boolean).join(' / ')}</Text>
          {diagnostics.missingArtifacts?.length ? <PathList paths={diagnostics.missingArtifacts} /> : null}
        </div>
      ) : null}

      <div className={styles.footer}>
        <Text type="secondary">Last tool</Text>
        <Tooltip title={diagnostics?.lastToolId}>
          <span className={styles.truncate}>{diagnostics?.lastToolTitle || diagnostics?.lastToolStatus || 'none'}</span>
        </Tooltip>
      </div>
      <div className={styles.footer}>
        <Text type="secondary">Last event</Text>
        <span className={styles.truncate}>{diagnostics ? eventSummary(diagnostics) : interruptedEvent(interrupted)}</span>
      </div>
    </aside>
  );
}

function CompactRow({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.compactRow}>
      <Text type="secondary">{label}</Text>
      <span className={styles.truncate}>{value}</span>
    </div>
  );
}

function ToolInspect({ title, tool }: { title: string; tool?: InterruptedToolViewModel }) {
  if (!tool?.id) {
    return null;
  }
  return (
    <div className={styles.toolInspect}>
      <Text strong>{title}</Text>
      <CompactRow label="Tool" value={toolSummary(tool)} />
      {tool.command ? <CompactRow label="Command" value={tool.command} /> : null}
      {tool.workingDir ? <CompactRow label="Cwd" value={tool.workingDir} /> : null}
      {typeof tool.exitCode === 'number' ? <CompactRow label="Exit" value={String(tool.exitCode)} /> : null}
      {tool.stderrExcerpt ? <CompactRow label="Stderr" value={tool.stderrExcerpt} /> : null}
      {tool.target ? <CompactRow label="Target" value={tool.target} /> : null}
    </div>
  );
}

function Metric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className={styles.metric}>
      <span className={styles.metricIcon}>{icon}</span>
      <span className={styles.metricLabel}>{label}</span>
      <span className={styles.metricValue}>{value}</span>
    </div>
  );
}

function SignalTag({ label, value, danger = false }: { label: string; value?: number; danger?: boolean }) {
  if (!value) {
    return null;
  }
  return <Tag color={danger ? 'red' : 'default'}>{`${label} ${value}`}</Tag>;
}

function PathList({ paths }: { paths: string[] }) {
  return (
    <ul className={styles.paths}>
      {paths.slice(0, 3).map((path) => (
        <li key={path} title={path}>
          {path}
        </li>
      ))}
      {paths.length > 3 ? <li>{`+${paths.length - 3} more`}</li> : null}
    </ul>
  );
}

function statusColor(status: string) {
  if (status === 'failed' || status === 'interrupted') {
    return 'red';
  }
  if (status === 'waiting_permission') {
    return 'gold';
  }
  if (status === 'running' || status === 'queued' || status === 'cancelling') {
    return 'blue';
  }
  return 'default';
}

function statusLabel(status: string) {
  return status.replaceAll('_', ' ');
}

function formatDuration(value?: number) {
  if (!value || value < 0) {
    return '0s';
  }
  if (value < 1000) {
    return `${value}ms`;
  }
  const seconds = Math.floor(value / 1000);
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}

function formatCountMap(counts?: Record<string, number>) {
  const entries = Object.entries(counts ?? {}).filter(([, value]) => value > 0);
  if (entries.length === 0) {
    return 'none';
  }
  return entries
    .slice(0, 3)
    .map(([key, value]) => `${key.replaceAll('_', ' ')} ${value}`)
    .join(', ');
}

function artifactSummary(diagnostics: TurnDiagnosticsViewModel) {
  const counts = diagnostics.artifactCounts;
  return `expected ${counts?.expected ?? diagnostics.expectedArtifacts?.length ?? 0}, verified ${counts?.verified ?? diagnostics.verifiedArtifacts?.length ?? 0}, missing ${
    counts?.missing ?? diagnostics.missingArtifacts?.length ?? 0
  }`;
}

function eventSummary(diagnostics: TurnDiagnosticsViewModel) {
  const sequence = diagnostics.lastRuntimeEventSequence ? `#${diagnostics.lastRuntimeEventSequence}` : 'none';
  if (!diagnostics.lastRuntimeEventAt) {
    return sequence;
  }
  return `${sequence} at ${new Date(diagnostics.lastRuntimeEventAt).toLocaleTimeString()}`;
}

function toolSummary(tool?: InterruptedToolViewModel) {
  if (!tool?.id) {
    return '';
  }
  const title = tool.display?.title || tool.name || tool.id;
  const detail = tool.target || tool.command || tool.failureReason;
  const status = tool.status ? ` ${tool.status}` : '';
  return detail ? `${title}${status}: ${detail}` : `${title}${status}`;
}

function interruptedSignals(interrupted?: InterruptedTurnViewModel) {
  if (!interrupted) {
    return 'none';
  }
  const parts = [
    countLabel('failed', interrupted.failedToolCount),
    countLabel('denied', interrupted.deniedToolCount),
    countLabel('cancelled', interrupted.cancelledToolCount),
    countLabel('nonzero shell', interrupted.nonzeroExitShellCount),
  ].filter(Boolean);
  return parts.length ? parts.join(', ') : 'none';
}

function interruptedArtifacts(interrupted?: InterruptedTurnViewModel) {
  const counts = interrupted?.artifactCounts;
  return `expected ${counts?.expected ?? interrupted?.expectedArtifacts?.length ?? 0}, produced ${
    counts?.produced ?? interrupted?.producedArtifacts?.length ?? 0
  }, verified ${counts?.verified ?? interrupted?.verifiedArtifacts?.length ?? 0}, missing ${counts?.missing ?? interrupted?.missingArtifacts?.length ?? 0}`;
}

function interruptedPermissions(interrupted?: InterruptedTurnViewModel) {
  const counts = interrupted?.permissionCounts;
  const parts = [
    countLabel('pending', counts?.pending),
    countLabel('allowed', counts?.allowed),
    countLabel('denied', counts?.denied),
    countLabel('expired', counts?.expired),
    countLabel('cancelled', counts?.cancelled),
  ].filter(Boolean);
  return parts.length ? parts.join(', ') : 'none';
}

function interruptedEvent(interrupted?: InterruptedTurnViewModel) {
  const sequence = interrupted?.lastRuntimeEventSequence ? `#${interrupted.lastRuntimeEventSequence}` : 'none';
  if (!interrupted?.lastRuntimeEventAt) {
    return sequence;
  }
  return `${sequence} at ${new Date(interrupted.lastRuntimeEventAt).toLocaleTimeString()}`;
}

function interruptedSummaryText(interrupted?: InterruptedTurnViewModel) {
  if (!interrupted) {
    return '';
  }
  if (interrupted.summaryText?.trim()) {
    return interrupted.summaryText;
  }
  return [
    `Interrupted turn ${interrupted.turnId || ''}`.trim(),
    interrupted.reason ? `Reason: ${interrupted.reason}` : '',
    toolSummary(interrupted.lastCompletedTool) ? `Last completed tool: ${toolSummary(interrupted.lastCompletedTool)}` : '',
    toolSummary(interrupted.lastFailedTool) ? `Last failed tool: ${toolSummary(interrupted.lastFailedTool)}` : '',
    toolSummary(interrupted.pendingTool) ? `Pending tool: ${toolSummary(interrupted.pendingTool)}` : '',
    `Artifacts: ${interruptedArtifacts(interrupted)}`,
    `Permissions: ${interruptedPermissions(interrupted)}`,
    `Signals: ${interruptedSignals(interrupted)}`,
    `Last event: ${interruptedEvent(interrupted)}`,
  ]
    .filter(Boolean)
    .join('\n');
}

function followUpPrompt(summary: string) {
  return [
    'Continue from this interrupted runtime state. Do not replay completed tools unless needed; inspect the current filesystem/state first.',
    '',
    summary,
  ].join('\n');
}

function countLabel(label: string, value?: number) {
  return value && value > 0 ? `${label} ${value}` : '';
}
