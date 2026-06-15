import { CheckOutlined, CloseCircleOutlined, LoadingOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Alert, Button, Tag } from 'antd';
import { useState } from 'react';
import type { PermissionRequestViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './PermissionGate.module.css';

interface PermissionGateProps {
  permission: PermissionRequestViewModel;
  onDecide: (permissionID: string, action: 'allow' | 'allow_session' | 'deny') => Promise<void>;
}

type PermissionDecision = 'allow' | 'allow_session' | 'deny';

export function PermissionGate({ permission, onDecide }: PermissionGateProps) {
  const pending = permission.status === 'pending';
  const [deciding, setDeciding] = useState<PermissionDecision | undefined>();
  const [error, setError] = useState<string | undefined>();

  const decide = async (action: PermissionDecision) => {
    if (deciding) {
      return;
    }
    setError(undefined);
    setDeciding(action);
    try {
      await onDecide(permission.id, action);
    } catch (err) {
      setError(permissionDecisionErrorMessage(err));
    } finally {
      setDeciding(undefined);
    }
  };

  return (
    <section className={pending ? `${styles.gate} ${styles.pendingGate}` : styles.gate} data-testid="permission-gate" data-permission-id={permission.id} data-permission-status={permission.status}>
      <div className={styles.summary}>
        <span className={styles.title}>
          <SafetyCertificateOutlined />
          {pending ? '需要权限' : permissionSummary(permission.status)}
        </span>
        <PermissionStatus status={permission.status} />
      </div>

      <div className={styles.body}>
        <span className={styles.tool}>{permission.toolName}</span>
        {permission.target && <code className={styles.target}>{permission.target}</code>}
        {permission.reason && <span className={styles.reason}>{permission.reason}</span>}
      </div>

      {(permission.risk || permission.policyMode || permission.action) && (
        <div className={styles.meta}>
          {permission.risk && <Tag>{riskLabel(permission.risk)}</Tag>}
          {permission.policyMode && <Tag>{policyModeLabel(permission.policyMode)}</Tag>}
          {permission.action && <Tag>{actionLabel(permission.action)}</Tag>}
        </div>
      )}

      {pending && (
        <div className={styles.actions}>
          <Button type="primary" size="small" loading={deciding === 'allow'} disabled={Boolean(deciding)} onClick={() => void decide('allow')}>
            允许一次
          </Button>
          <Button size="small" loading={deciding === 'allow_session'} disabled={Boolean(deciding)} onClick={() => void decide('allow_session')}>
            本会话允许
          </Button>
          <Button danger size="small" loading={deciding === 'deny'} disabled={Boolean(deciding)} onClick={() => void decide('deny')}>
            拒绝
          </Button>
        </div>
      )}
      {error && <Alert className={styles.error} type="error" showIcon message={error} />}
    </section>
  );
}

function PermissionStatus({ status }: { status: string }) {
  switch (status) {
    case 'allowed':
    case 'allowed_once':
      return (
        <span className={styles.allowedStatus}>
          <CheckOutlined />
          已允许
        </span>
      );
    case 'allowed_session':
      return (
        <span className={styles.allowedStatus}>
          <CheckOutlined />
          本会话已允许
        </span>
      );
    case 'denied':
      return (
        <Tag icon={<CloseCircleOutlined />} color="error">
          已拒绝
        </Tag>
      );
    case 'expired':
      return <Tag color="default">已过期</Tag>;
    case 'cancelled':
      return <Tag color="default">已取消</Tag>;
    default:
      return (
        <Tag icon={<LoadingOutlined spin />} color="warning">
          等待确认
        </Tag>
      );
  }
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
      return '请求批准';
    case 'auto_read':
      return '替我审批';
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

function actionLabel(action: string) {
  switch (action) {
    case 'allow':
      return '允许一次';
    case 'allow_session':
      return '本会话允许';
    case 'deny':
      return '拒绝';
    default:
      return action;
  }
}

function permissionDecisionErrorMessage(error: unknown) {
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return '\u6743\u9650\u51b3\u7b56\u5931\u8d25\uff0c\u8bf7\u5237\u65b0\u540e\u91cd\u8bd5';
}
