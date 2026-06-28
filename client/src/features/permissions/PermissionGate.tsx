import { CheckOutlined, CloseCircleOutlined, EnterOutlined, LoadingOutlined } from '@ant-design/icons';
import { Alert, Button, Checkbox, Input, Radio, Space, Tag } from 'antd';
import { useMemo, useState } from 'react';
import type { PermissionRequestViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './PermissionGate.module.css';

interface PermissionGateProps {
  permission: PermissionRequestViewModel;
  onDecide: (permissionID: string, action: PermissionDecision, guidance?: string) => Promise<void>;
}

type PermissionDecision = 'allow' | 'allow_session' | 'deny';

export function PermissionGate({ permission, onDecide }: PermissionGateProps) {
  const pending = permission.status === 'pending';
  const [selected, setSelected] = useState<PermissionDecision>('allow');
  const [tellAgent, setTellAgent] = useState(false);
  const [guidance, setGuidance] = useState('');
  const [deciding, setDeciding] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const detail = useMemo(() => permissionDetail(permission), [permission]);
  const decisionGuidance = tellAgent ? guidance.trim() : undefined;

  const decide = async (action: PermissionDecision = selected) => {
    if (deciding || !pending) {
      return;
    }
    setError(undefined);
    setDeciding(true);
    try {
      await onDecide(permission.id, action, action === 'deny' ? decisionGuidance : undefined);
    } catch (err) {
      setError(permissionDecisionErrorMessage(err));
    } finally {
      setDeciding(false);
    }
  };

  if (!pending) {
    return (
      <section className={styles.resolvedGate} data-testid="permission-gate" data-permission-id={permission.id} data-permission-status={permission.status}>
        <PermissionStatus status={permission.status} />
        <span>{permissionSummary(permission.status)}</span>
      </section>
    );
  }

  return (
    <section className={styles.gate} data-testid="permission-gate" data-permission-id={permission.id} data-permission-status={permission.status}>
      <div className={styles.actionSection}>
        <div className={styles.header}>
          <div className={styles.title}>{permissionTitle(permission)}</div>
          {detail && (
            <Button className={styles.expandButton} size="small" type="link" onClick={() => setExpanded((value) => !value)}>
              {expanded ? '折叠' : '展开'}
            </Button>
          )}
        </div>
        {detail && <pre className={expanded ? styles.previewExpanded : styles.previewCollapsed}>{detail}</pre>}
      </div>

      <div className={styles.policySection}>
        <Radio.Group
          className={styles.options}
          optionType="default"
          value={selected}
          onChange={(event) => setSelected(event.target.value as PermissionDecision)}
        >
          <Space direction="vertical" size={4}>
            <Radio value="allow">同意</Radio>
            <Radio value="allow_session">{sessionAllowLabel(permission)}</Radio>
            <Radio value="deny">不同意</Radio>
          </Space>
        </Radio.Group>
        <Checkbox className={styles.tellAgentOption} checked={tellAgent} onChange={(event) => setTellAgent(event.target.checked)}>
          告诉 Codex 应该如何调整
        </Checkbox>
        {tellAgent && (
          <Input.TextArea
            autoSize={{ minRows: 2, maxRows: 4 }}
            className={styles.guidanceInput}
            maxLength={500}
            placeholder="例如：不要执行这个命令，改为先说明需要我手动完成的步骤。"
            value={guidance}
            onChange={(event) => setGuidance(event.target.value)}
          />
        )}
      </div>

      <div className={styles.footer}>
        <div className={styles.meta}>
          <Tag>{riskLabel(permission.risk)}</Tag>
          <Tag>{policyModeLabel(permission.policyMode)}</Tag>
          {permission.action && <Tag>{permissionRequestActionLabel(permission.action)}</Tag>}
          {permission.policyTargetSummary && <Tag>{permission.policyTargetSummary}</Tag>}
          <code>{permission.toolName}</code>
        </div>
        <Button type="text" disabled={deciding} onClick={() => void decide('deny')}>
          跳过
        </Button>
        <Button className={styles.submitButton} type="primary" icon={<EnterOutlined />} loading={deciding} onClick={() => void decide()}>
          提交
        </Button>
      </div>

      {error && <Alert className={styles.error} type="error" showIcon message={error} />}
    </section>
  );
}

function permissionDetail(permission: PermissionRequestViewModel) {
  const params = formatParams(permission);
  return [
    permission.description,
    permission.target ? `目标: ${permission.target}` : '',
    permission.path && permission.path !== permission.target ? `路径: ${permission.path}` : '',
    params ? `参数: ${params}` : '',
    permission.reason || permission.policyReason ? `原因: ${permission.reason || permission.policyReason}` : '',
    permission.policyRuleId ? `匹配规则: ${permission.policyRuleId}${permission.policyRuleSource ? ` (${permission.policyRuleSource})` : ''}` : '',
    permission.policyScopeKind || permission.policyScopeValue ? `范围: ${[permission.policyScopeKind, permission.policyScopeValue].filter(Boolean).join(' / ')}` : '',
  ]
    .filter((value): value is string => Boolean(value?.trim()))
    .join('\n');
}

function formatParams(permission: PermissionRequestViewModel) {
  const source = permission.description?.trim();
  if (!source || !source.startsWith('{')) {
    return '';
  }
  try {
    const parsed = JSON.parse(source) as Record<string, unknown>;
    if (typeof parsed.command === 'string') {
      return parsed.command;
    }
  } catch {
    return '';
  }
  return '';
}

function PermissionStatus({ status }: { status: string }) {
  switch (status) {
    case 'allowed':
    case 'allowed_once':
    case 'allowed_session':
      return (
        <span className={styles.allowedStatus}>
          <CheckOutlined />
        </span>
      );
    case 'denied':
      return (
        <span className={styles.deniedStatus}>
          <CloseCircleOutlined />
        </span>
      );
    default:
      return (
        <span className={styles.pendingStatus}>
          <LoadingOutlined spin />
        </span>
      );
  }
}

function permissionTitle(permission: PermissionRequestViewModel) {
  const tool = permission.toolName || '工具';
  const target = permission.target || permission.path || tool;
  if (permission.risk === 'execute') {
    return `允许运行 ${tool} 吗？`;
  }
  if (permission.risk === 'write' || permission.risk === 'destructive') {
    return `允许修改 ${target} 吗？`;
  }
  if (permission.risk === 'read') {
    return `允许读取 ${target} 吗？`;
  }
  return `允许 ${tool} 执行该操作吗？`;
}

function sessionAllowLabel(permission: PermissionRequestViewModel) {
  if (permission.risk === 'execute') {
    return '是，且本会话内相同的命令不再询问';
  }
  if (permission.risk === 'read') {
    return '是，且本会话内相同的读取不再询问';
  }
  return '是，且本会话内相同的操作不再询问';
}

function permissionSummary(status: string) {
  switch (status) {
    case 'denied':
      return '权限已拒绝';
    case 'expired':
      return '权限已过期';
    case 'cancelled':
      return '权限已取消';
    default:
      return '权限已处理';
  }
}

function riskLabel(risk?: string) {
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
      return risk || '权限';
  }
}

function policyModeLabel(mode?: string) {
  switch (mode) {
    case 'ask':
      return '请求批准';
    case 'auto_read':
      return '自动只读';
    case 'full_access':
      return '完全访问';
    case 'plan':
      return '计划模式';
    case 'deny_all':
      return '全部阻止';
    default:
      return mode || '策略';
  }
}

function permissionRequestActionLabel(action: string) {
  switch (action) {
    case 'execute':
      return '执行';
    case 'read':
      return '读取';
    case 'write':
      return '写入';
    default:
      return action;
  }
}

function permissionDecisionErrorMessage(error: unknown) {
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return '权限决策失败，请刷新后重试';
}
