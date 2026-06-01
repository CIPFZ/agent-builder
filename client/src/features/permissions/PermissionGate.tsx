import { CheckCircleOutlined, CloseCircleOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Button, Tag } from 'antd';
import type { PermissionRequestViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './PermissionGate.module.css';

interface PermissionGateProps {
  permission: PermissionRequestViewModel;
  onDecide: (permissionID: string, action: 'allow' | 'allow_for_session' | 'deny') => Promise<void>;
}

export function PermissionGate({ permission, onDecide }: PermissionGateProps) {
  const pending = permission.status === 'pending';

  return (
    <section className={styles.gate} data-testid="permission-gate" data-permission-id={permission.id} data-permission-status={permission.status}>
      <div className={styles.header}>
        <span className={styles.title}>
          <SafetyCertificateOutlined />
          权限请求
        </span>
        <PermissionStatus status={permission.status} />
      </div>
      <div className={styles.tool}>{permission.toolName}</div>
      {permission.target && <div className={styles.target}>{permission.target}</div>}
      {permission.reason && <div className={styles.reason}>{permission.reason}</div>}
      <div className={styles.meta}>
        {permission.risk && <Tag>{riskLabel(permission.risk)}</Tag>}
        {permission.policyMode && <Tag>{policyModeLabel(permission.policyMode)}</Tag>}
      </div>
      {pending && (
        <div className={styles.actions}>
          <Button type="primary" onClick={() => void onDecide(permission.id, 'allow')}>
            允许一次
          </Button>
          <Button onClick={() => void onDecide(permission.id, 'allow_for_session')}>本会话允许</Button>
          <Button danger onClick={() => void onDecide(permission.id, 'deny')}>
            拒绝
          </Button>
        </div>
      )}
    </section>
  );
}

function PermissionStatus({ status }: { status: string }) {
  switch (status) {
    case 'allowed':
    case 'allowed_once':
      return (
        <Tag icon={<CheckCircleOutlined />} color="success">
          已允许
        </Tag>
      );
    case 'allowed_session':
      return (
        <Tag icon={<CheckCircleOutlined />} color="success">
          本会话已允许
        </Tag>
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
      return <Tag color="warning">等待审批</Tag>;
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
