import { Alert, Button, Tag, Tooltip, Typography } from 'antd';
import { BranchesOutlined, CheckCircleOutlined, CloseCircleOutlined, ToolOutlined } from '@ant-design/icons';
import { useState } from 'react';
import type { HookExecutionViewModel, ReactCallchainNodeViewModel, ReactCallchainViewModel } from '../../runtime/workbenchTypes.ts';
import { HookExecutionDetailDrawer } from '../hooks/HookExecutionDetailDrawer.tsx';
import styles from './ReactCallchainInspector.module.css';

const { Text } = Typography;

interface ReactCallchainInspectorProps {
  callchain?: ReactCallchainViewModel;
  onHookExecutionLoad?: (executionID: string) => Promise<HookExecutionViewModel>;
}

export function ReactCallchainInspector({ callchain, onHookExecutionLoad }: ReactCallchainInspectorProps) {
  const [selectedHookExecutionID, setSelectedHookExecutionID] = useState('');
  if (!callchain) {
    return null;
  }

  const summary = callchain.summary;
  return (
    <aside className={styles.panel} data-testid="react-callchain-inspector" aria-label="Turn chain">
      <div className={styles.header}>
        <span className={styles.heading}>
          <BranchesOutlined />
          <span>Turn chain</span>
        </span>
        <Tag color={summary.hasFinalAssistant ? 'success' : 'warning'}>{summary.hasFinalAssistant ? 'final' : 'no final'}</Tag>
      </div>

      {summary.missingEvidence.length ? (
        <Alert
          type="warning"
          showIcon
          message="Missing or conflicting evidence"
          description={summary.missingEvidence.join(', ')}
          className={styles.alert}
        />
      ) : null}

      <div className={styles.metrics}>
        <Metric label="Tools" value={summary.toolCallCount} />
        <Metric label="Perms" value={summary.permissionCount} />
        <Metric label="Hooks" value={summary.hookCount} />
      </div>

      <div className={styles.statusLine}>
        <Text type="secondary">Stop</Text>
        <Text className={styles.truncate}>{summary.stopReasonMessage || summary.stopReason || 'running'}</Text>
      </div>

      {summary.toolResultDeliveries?.length ? (
        <div className={styles.statusLine}>
          <Text type="secondary">Tool results</Text>
          <Text className={styles.truncate}>
            {summary.deliveredToolResultCount ?? 0} fed back / {summary.undeliveredToolResultCount ?? 0} pending
          </Text>
        </div>
      ) : null}

      <div className={styles.nodes}>
        {callchain.nodes.map((node) => (
          <CallchainNodeRow key={node.id} node={node} onHookOpen={setSelectedHookExecutionID} />
        ))}
      </div>

      <div className={styles.source}>
        <Tooltip title="Runtime events only trigger refreshes; this view is hydrated from runtime reads.">
          <Tag color={callchain.source.eventsAreRefreshOnly ? 'blue' : 'default'}>events refresh-only</Tag>
        </Tooltip>
        <Tag color={callchain.source.sessionActivityParity ? 'blue' : 'default'}>activity parity</Tag>
      </div>
      <HookExecutionDetailDrawer
        executionId={selectedHookExecutionID}
        open={Boolean(selectedHookExecutionID)}
        onClose={() => setSelectedHookExecutionID('')}
        onLoad={onHookExecutionLoad}
      />
    </aside>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className={styles.metric}>
      <Text type="secondary">{label}</Text>
      <Text strong>{value}</Text>
    </div>
  );
}

function CallchainNodeRow({ node, onHookOpen }: { node: ReactCallchainNodeViewModel; onHookOpen: (executionID: string) => void }) {
  const statusKind = node.error ? 'error' : node.status === 'completed' || node.status === 'allowed_once' || node.status === 'allowed_session' ? 'success' : 'default';
  const isHook = node.kind === 'hook_execution';
  return (
    <div className={styles.node} data-kind={node.kind}>
      <span className={styles.nodeIcon}>{node.kind === 'tool_call' || node.kind === 'tool_result' ? <ToolOutlined /> : node.error ? <CloseCircleOutlined /> : <CheckCircleOutlined />}</span>
      <div className={styles.nodeBody}>
        <div className={styles.nodeTitle}>
          <Text strong className={styles.truncate}>
            {node.title || node.kind}
          </Text>
          <Tag color={isHook ? hookStatusColor(node.hook?.status || node.status) : statusKind === 'success' ? 'success' : statusKind === 'error' ? 'error' : undefined}>{node.hook?.status || node.status || node.kind}</Tag>
        </div>
        <Text type="secondary" className={styles.nodeMeta}>
          #{node.sequence} {node.kind}
          {node.finishReason ? ` / ${node.finishReason}` : ''}
          {node.hook?.durationMs ? ` / ${formatDuration(node.hook.durationMs)}` : ''}
        </Text>
        {isHook ? <HookNodeEvidence node={node} onOpen={onHookOpen} /> : null}
        {node.summary ? <Text className={styles.nodeSummary}>{node.summary}</Text> : null}
        {node.error ? <Text type="danger" className={styles.nodeSummary}>{node.error}</Text> : null}
        {node.kind === 'tool_result' ? <ToolResultEvidence node={node} /> : null}
      </div>
    </div>
  );
}

function HookNodeEvidence({ node, onOpen }: { node: ReactCallchainNodeViewModel; onOpen: (executionID: string) => void }) {
  return (
    <div className={styles.nodeTags}>
      {node.hook?.event || node.evidence?.event ? <Tag color="blue">{node.hook?.event || node.evidence?.event}</Tag> : null}
      {node.hook?.inputRewritten ? <Tag color="purple">rewritten</Tag> : null}
      {node.hook?.contextInjected ? <Tag color="cyan">context</Tag> : null}
      {node.hook?.reason ? <Tag>{node.hook.reason}</Tag> : null}
      {node.hookExecutionId ? (
        <Button size="small" type="link" onClick={() => onOpen(node.hookExecutionId || '')}>
          Details
        </Button>
      ) : null}
    </div>
  );
}

function hookStatusColor(status?: string) {
  switch (status) {
    case 'completed':
      return 'success';
    case 'blocked':
    case 'denied':
      return 'warning';
    case 'failed':
      return 'error';
    case 'started':
    case 'running':
      return 'processing';
    default:
      return undefined;
  }
}

function formatDuration(value: number) {
  if (value < 1000) {
    return `${value}ms`;
  }
  return `${Math.round(value / 100) / 10}s`;
}

function ToolResultEvidence({ node }: { node: ReactCallchainNodeViewModel }) {
  const delivered = node.evidence?.deliveredToModel;
  const persisted = node.evidence?.persistedOutput;
  if (!delivered && !persisted) {
    return null;
  }
  return (
    <div className={styles.nodeTags}>
      {delivered ? <Tag color={delivered === 'true' ? 'success' : 'warning'}>{delivered === 'true' ? 'fed back to model' : 'not fed back'}</Tag> : null}
      {node.evidence?.deliveryReason ? <Tag>{node.evidence.deliveryReason}</Tag> : null}
      {persisted === 'true' ? <Tag color="blue">persisted output</Tag> : null}
      {node.evidence?.truncatedBy ? <Tag>{node.evidence.truncatedBy}</Tag> : null}
    </div>
  );
}
