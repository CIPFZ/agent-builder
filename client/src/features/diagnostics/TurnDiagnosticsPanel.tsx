import { AlertOutlined, ClockCircleOutlined, FileDoneOutlined, ToolOutlined } from '@ant-design/icons';
import { Tag, Tooltip, Typography } from 'antd';
import type React from 'react';
import type { TurnDiagnosticsViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './TurnDiagnosticsPanel.module.css';

const { Text } = Typography;

export function TurnDiagnosticsPanel({ diagnostics }: { diagnostics?: TurnDiagnosticsViewModel }) {
  if (!diagnostics) {
    return null;
  }
  const status = diagnostics.status || 'unknown';
  const failureSignals =
    (diagnostics.failedToolCount ?? 0) +
    (diagnostics.deniedToolCount ?? 0) +
    (diagnostics.cancelledToolCount ?? 0) +
    (diagnostics.nonzeroExitShellCount ?? 0);
  const duration = diagnostics.runningDurationMs || diagnostics.durationMs;
  return (
    <aside className={styles.panel} data-testid="turn-diagnostics-panel" aria-label="Turn diagnostics">
      <div className={styles.header}>
        <div className={styles.heading}>
          <AlertOutlined />
          <span>Turn diagnostics</span>
        </div>
        <Tag color={statusColor(status)}>{statusLabel(status)}</Tag>
      </div>

      <div className={styles.grid}>
        <Metric icon={<ClockCircleOutlined />} label="Duration" value={formatDuration(duration)} />
        <Metric icon={<ToolOutlined />} label="Tools" value={formatCountMap(diagnostics.toolCountsByStatus)} />
        <Metric icon={<ToolOutlined />} label="Kinds" value={formatCountMap(diagnostics.toolCountsByKind)} />
        <Metric icon={<FileDoneOutlined />} label="Artifacts" value={artifactSummary(diagnostics)} />
      </div>

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Signals</div>
        <div className={styles.tags}>
          <SignalTag label="failed" value={diagnostics.failedToolCount} danger />
          <SignalTag label="denied" value={diagnostics.deniedToolCount} danger />
          <SignalTag label="cancelled" value={diagnostics.cancelledToolCount} />
          <SignalTag label="nonzero shell" value={diagnostics.nonzeroExitShellCount} danger />
          {failureSignals === 0 ? <Tag>none</Tag> : null}
        </div>
      </div>

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Permissions</div>
        <div className={styles.tags}>
          <SignalTag label="pending" value={diagnostics.permissionCounts?.pending} />
          <SignalTag label="allowed" value={diagnostics.permissionCounts?.allowed} />
          <SignalTag label="denied" value={diagnostics.permissionCounts?.denied} danger />
          <SignalTag label="expired" value={diagnostics.permissionCounts?.expired} />
          <SignalTag label="cancelled" value={diagnostics.permissionCounts?.cancelled} />
        </div>
      </div>

      <div className={styles.section}>
        <div className={styles.sectionTitle}>Artifact confidence</div>
        <div className={styles.tags}>
          <SignalTag label="verified local" value={diagnostics.artifactConfidenceSummary?.localVerifiedFile} />
          <SignalTag label="tool metadata" value={diagnostics.artifactConfidenceSummary?.producedToolMetadata} />
          <SignalTag label="runtime refs" value={diagnostics.artifactConfidenceSummary?.runtimeOutputRefs} />
          <SignalTag label="structured refs" value={diagnostics.artifactConfidenceSummary?.structuredMcpCustomRefs} />
          <SignalTag label="unknown" value={diagnostics.artifactConfidenceSummary?.unknownNotDetected} danger />
        </div>
      </div>

      {diagnostics.warning ? (
        <div className={styles.warning}>
          <Text strong>{diagnostics.warning}</Text>
          <Text type="secondary">{[diagnostics.warningReason, diagnostics.warningSource].filter(Boolean).join(' / ')}</Text>
          {diagnostics.missingArtifacts?.length ? <PathList paths={diagnostics.missingArtifacts} /> : null}
        </div>
      ) : null}

      <div className={styles.footer}>
        <Text type="secondary">Last tool</Text>
        <Tooltip title={diagnostics.lastToolId}>
          <span className={styles.truncate}>{diagnostics.lastToolTitle || diagnostics.lastToolStatus || 'none'}</span>
        </Tooltip>
      </div>
      <div className={styles.footer}>
        <Text type="secondary">Last event</Text>
        <span className={styles.truncate}>{eventSummary(diagnostics)}</span>
      </div>
    </aside>
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
