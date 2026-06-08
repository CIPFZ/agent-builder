import { BranchesOutlined, CheckCircleOutlined, FileDoneOutlined, ToolOutlined } from '@ant-design/icons';
import { Tag, Typography } from 'antd';
import type React from 'react';
import type { RunProjectionViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './RunProjectionPreview.module.css';

const { Text } = Typography;

export function RunProjectionPreview({ run }: { run?: RunProjectionViewModel }) {
  if (!run) {
    return null;
  }
  return (
    <aside className={styles.panel} data-testid="run-projection-preview" aria-label="Run projection preview">
      <div className={styles.header}>
        <div className={styles.heading}>
          <BranchesOutlined />
          <span>Run projection</span>
        </div>
        <Tag color={statusColor(run.status)}>{run.status || 'unknown'}</Tag>
      </div>

      {run.objective ? <Text className={styles.objective}>{run.objective}</Text> : null}

      <div className={styles.grid}>
        <Metric icon={<BranchesOutlined />} label="Turns" value={formatCount(run.turnCount)} />
        <Metric icon={<ToolOutlined />} label="Tools" value={formatCount(run.toolCallCount)} />
        <Metric icon={<FileDoneOutlined />} label="Artifacts" value={artifactSummary(run)} />
        <Metric icon={<CheckCircleOutlined />} label="Parity" value={run.sessionActivityParity ? 'session activity' : 'unverified'} />
      </div>

      <div className={styles.tags}>
        <SignalTag label="tasks" value={run.taskCount} />
        <SignalTag label="permissions" value={run.permissionRequestCount} />
        <SignalTag label="waiting" value={run.waitingPermissionTurnCount} />
        <SignalTag label="running" value={run.runningTurnCount} />
        <SignalTag label="interrupted" value={run.interruptedTurnCount} />
        <SignalTag label="failed" value={run.failedTurnCount} />
        <SignalTag label="cancelled" value={run.cancelledTurnCount} />
        <SignalTag label="checkpoints" value={run.checkpointCount} />
      </div>

      <div className={styles.footer}>
        <Text type="secondary">{run.sourceReadOnly ? 'read-only' : 'runtime'}</Text>
        <span className={styles.truncate}>{run.sourceKind || run.evidenceCursor || run.id}</span>
      </div>
    </aside>
  );
}

function Metric({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className={styles.metric}>
      <span className={styles.metricIcon}>{icon}</span>
      <span className={styles.metricLabel}>{label}</span>
      <span className={styles.metricValue}>{value}</span>
    </div>
  );
}

function SignalTag({ label, value }: { label: string; value?: number }) {
  if (!value) {
    return null;
  }
  return <Tag>{label}: {value}</Tag>;
}

function artifactSummary(run: RunProjectionViewModel) {
  const parts = [
    labeledCount('expected', run.expectedArtifactCount),
    labeledCount('produced', run.producedArtifactCount),
    labeledCount('verified', run.verifiedArtifactCount),
    labeledCount('missing', run.missingArtifactCount),
  ].filter(Boolean);
  return parts.length ? parts.join(' / ') : 'none';
}

function labeledCount(label: string, value?: number) {
  return value ? `${label} ${value}` : '';
}

function formatCount(value?: number) {
  return typeof value === 'number' ? String(value) : '0';
}

function statusColor(status?: string) {
  switch (status) {
    case 'active':
      return 'processing';
    case 'waiting_user':
      return 'warning';
    case 'interrupted':
    case 'failed':
      return 'error';
    case 'cancelled':
      return 'default';
    case 'completed':
      return 'success';
    default:
      return 'default';
  }
}
