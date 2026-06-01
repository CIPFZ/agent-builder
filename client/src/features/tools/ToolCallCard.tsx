import { CheckCircleOutlined, CloseCircleOutlined, CodeOutlined, LoadingOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Collapse, Tag } from 'antd';
import type { ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ToolCallCard.module.css';

export function ToolCallCard({ toolCall }: { toolCall: ToolCallViewModel }) {
  const detail = toolCall.command || toolCall.inputSummary;
  const output = toolCall.outputSummary || toolCall.error;
  const hasDetails = Boolean(toolCall.stdout || toolCall.stderr || toolCall.policyReason || toolCall.policyTargetSummary);

  return (
    <section className={styles.card} data-testid="tool-call-card" data-tool-call-id={toolCall.id} data-tool-status={toolCall.status}>
      <div className={styles.header}>
        <span className={styles.title}>
          <CodeOutlined />
          {toolCall.name}
        </span>
        <ToolStatus status={toolCall.status} />
      </div>
      {detail && <pre className={styles.detail}>{detail}</pre>}
      {output && <div className={styles.output}>{output}</div>}
      {(toolCall.risk || toolCall.policyMode) && (
        <div className={styles.meta}>
          {toolCall.risk && <Tag icon={<SafetyCertificateOutlined />}>{riskLabel(toolCall.risk)}</Tag>}
          {toolCall.policyMode && <Tag>{policyModeLabel(toolCall.policyMode)}</Tag>}
        </div>
      )}
      {hasDetails && (
        <Collapse
          ghost
          size="small"
          items={[
            {
              key: 'details',
              label: '详情',
              children: (
                <div className={styles.details}>
                  {toolCall.policyReason && <DetailRow label="策略原因" value={toolCall.policyReason} />}
                  {toolCall.policyTargetSummary && <DetailRow label="策略目标" value={toolCall.policyTargetSummary} />}
                  {toolCall.stdout && <DetailBlock label="stdout" value={toolCall.stdout} />}
                  {toolCall.stderr && <DetailBlock label="stderr" value={toolCall.stderr} />}
                </div>
              ),
            },
          ]}
        />
      )}
    </section>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.detailRow}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function DetailBlock({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className={styles.detailBlockLabel}>{label}</div>
      <pre className={styles.detail}>{value}</pre>
    </div>
  );
}

function ToolStatus({ status }: { status: string }) {
  if (status === 'completed') {
    return (
      <Tag icon={<CheckCircleOutlined />} color="success">
        已完成
      </Tag>
    );
  }
  if (status === 'failed' || status === 'denied' || status === 'cancelled') {
    return (
      <Tag icon={<CloseCircleOutlined />} color="error">
        {statusLabel(status)}
      </Tag>
    );
  }
  return (
    <Tag icon={<LoadingOutlined spin />} color="processing">
      {statusLabel(status)}
    </Tag>
  );
}

function statusLabel(status: string) {
  switch (status) {
    case 'waiting_permission':
      return '等待权限';
    case 'running':
      return '执行中';
    case 'denied':
      return '已拒绝';
    case 'cancelled':
      return '已取消';
    case 'failed':
      return '失败';
    default:
      return status;
  }
}

function riskLabel(risk: string) {
  switch (risk) {
    case 'read':
      return '只读';
    case 'write':
      return '写入';
    case 'execute':
      return '执行';
    case 'network':
      return '网络';
    case 'secret':
      return '敏感';
    case 'destructive':
      return '破坏性';
    default:
      return risk;
  }
}

function policyModeLabel(mode: string) {
  switch (mode) {
    case 'ask':
      return '默认模式';
    case 'auto_read':
      return '自动审查';
    case 'full_access':
      return '完全访问权限';
    case 'plan':
      return '计划模式';
    case 'deny_all':
      return '全部阻止';
    default:
      return mode;
  }
}
