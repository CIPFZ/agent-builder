import { CheckCircleOutlined, CloseCircleOutlined, CodeOutlined, LoadingOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Collapse, Tag } from 'antd';
import type { ToolCallViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './ToolCallCard.module.css';

export function ToolCallCard({ toolCall }: { toolCall: ToolCallViewModel }) {
  const detail = toolCall.command || toolCall.inputSummary;
  const output = readableToolOutput(toolCall);
  const hasDetails = Boolean(toolCall.stdout || toolCall.stderr || toolCall.error || toolCall.policyReason || toolCall.policyTargetSummary);

  return (
    <section className={styles.card} data-testid="tool-call-card" data-tool-call-id={toolCall.id} data-tool-status={toolCall.status}>
      <div className={styles.header}>
        <span className={styles.title}>
          <CodeOutlined />
          {toolCallTitle(toolCall)}
        </span>
        <ToolStatus status={toolCall.status} />
      </div>

      {detail && <code className={styles.command}>{detail}</code>}
      {output && <pre className={styles.output}>{output}</pre>}

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
                  {toolCall.error && <DetailBlock label="error" value={toolCall.error} tone="error" />}
                  {toolCall.stdout && <DetailBlock label="stdout" value={toolCall.stdout} />}
                  {toolCall.stderr && <DetailBlock label="stderr" value={toolCall.stderr} tone="error" />}
                </div>
              ),
            },
          ]}
        />
      )}
    </section>
  );
}

function toolCallTitle(toolCall: ToolCallViewModel) {
  if (toolCall.status === 'completed') {
    return isShellTool(toolCall) ? '已运行 1 条命令' : `已运行 ${toolCall.name}`;
  }
  if (toolCall.status === 'waiting_permission') {
    return isShellTool(toolCall) ? '等待运行 1 条命令' : `等待运行 ${toolCall.name}`;
  }
  if (toolCall.status === 'running' || toolCall.status === 'queued') {
    return isShellTool(toolCall) ? '正在运行 1 条命令' : `正在运行 ${toolCall.name}`;
  }
  return isShellTool(toolCall) ? '命令执行未完成' : toolCall.name;
}

function isShellTool(toolCall: ToolCallViewModel) {
  const name = toolCall.name.toLowerCase();
  return Boolean(toolCall.command || name === 'bash' || name === 'shell' || name.includes('command'));
}

function readableToolOutput(toolCall: ToolCallViewModel) {
  const output = extractWrappedOutput(toolCall.stdout) || extractWrappedOutput(toolCall.outputSummary);
  if (output) {
    return output.trim();
  }
  if (toolCall.status === 'failed' || toolCall.status === 'denied' || toolCall.status === 'cancelled') {
    return (extractWrappedOutput(toolCall.stderr) || toolCall.error || '').trim();
  }
  return '';
}

function extractWrappedOutput(value?: string) {
  const text = value?.trim();
  if (!text) {
    return '';
  }

  const parsed = parseJSON(text);
  if (parsed && typeof parsed === 'object') {
    const record = parsed as Record<string, unknown>;
    for (const key of ['output', 'stdout', 'stderr', 'content', 'data']) {
      const field = record[key];
      if (typeof field === 'string' && field.trim()) {
        return field.trim();
      }
    }
  }

  return text;
}

function parseJSON(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.detailRow}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function DetailBlock({ label, value, tone }: { label: string; value: string; tone?: 'error' }) {
  return (
    <div>
      <div className={styles.detailBlockLabel}>{label}</div>
      <pre className={tone === 'error' ? `${styles.detail} ${styles.errorDetail}` : styles.detail}>{value}</pre>
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
    <Tag icon={<LoadingOutlined spin />} color={status === 'waiting_permission' ? 'warning' : 'processing'}>
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
    case 'queued':
      return '排队中';
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
      return '完全访问';
    case 'plan':
      return '计划模式';
    case 'deny_all':
      return '全部阻止';
    default:
      return mode;
  }
}
