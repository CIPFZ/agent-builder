import { BranchesOutlined } from '@ant-design/icons';
import type { AgentActivitySummary } from '../../runtime/conversationPresentationModel.ts';
import styles from './AgentActivityMonitor.module.css';

export function AgentActivityMonitor({ summary, onOpen }: { summary: AgentActivitySummary; onOpen: () => void }) {
  if (summary.total === 0) return null;
  const attention = summary.waiting + summary.failed;
  return <button className={styles.monitor} type="button" data-testid="agent-activity-monitor" data-attention={attention > 0 ? 'true' : undefined} onClick={onOpen} aria-label={`Agent tasks: ${summary.active} active, ${summary.completed} completed`}><BranchesOutlined /><span>{summary.active} active · {summary.completed} done{attention ? ` · ${attention} attention` : ''}</span></button>;
}
