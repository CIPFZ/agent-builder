import { Alert, Tag, Tooltip, Typography } from 'antd';
import { BranchesOutlined, CheckCircleOutlined, CloseCircleOutlined, ToolOutlined } from '@ant-design/icons';
import type { ReactCallchainNodeViewModel, ReactCallchainViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ReactCallchainInspector.module.css';

const { Text } = Typography;

interface ReactCallchainInspectorProps {
  callchain?: ReactCallchainViewModel;
}

export function ReactCallchainInspector({ callchain }: ReactCallchainInspectorProps) {
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
        <Text className={styles.truncate}>{summary.stopReason || 'running'}</Text>
      </div>

      <div className={styles.nodes}>
        {callchain.nodes.map((node) => (
          <CallchainNodeRow key={node.id} node={node} />
        ))}
      </div>

      <div className={styles.source}>
        <Tooltip title="Runtime events only trigger refreshes; this view is hydrated from runtime reads.">
          <Tag color={callchain.source.eventsAreRefreshOnly ? 'blue' : 'default'}>events refresh-only</Tag>
        </Tooltip>
        <Tag color={callchain.source.sessionActivityParity ? 'blue' : 'default'}>activity parity</Tag>
      </div>
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

function CallchainNodeRow({ node }: { node: ReactCallchainNodeViewModel }) {
  const statusKind = node.error ? 'error' : node.status === 'completed' || node.status === 'allowed_once' || node.status === 'allowed_session' ? 'success' : 'default';
  return (
    <div className={styles.node} data-kind={node.kind}>
      <span className={styles.nodeIcon}>{node.kind === 'tool_call' || node.kind === 'tool_result' ? <ToolOutlined /> : node.error ? <CloseCircleOutlined /> : <CheckCircleOutlined />}</span>
      <div className={styles.nodeBody}>
        <div className={styles.nodeTitle}>
          <Text strong className={styles.truncate}>
            {node.title || node.kind}
          </Text>
          <Tag color={statusKind === 'success' ? 'success' : statusKind === 'error' ? 'error' : undefined}>{node.status || node.kind}</Tag>
        </div>
        <Text type="secondary" className={styles.nodeMeta}>
          #{node.sequence} {node.kind}
          {node.finishReason ? ` / ${node.finishReason}` : ''}
        </Text>
        {node.summary ? <Text className={styles.nodeSummary}>{node.summary}</Text> : null}
        {node.error ? <Text type="danger" className={styles.nodeSummary}>{node.error}</Text> : null}
      </div>
    </div>
  );
}
