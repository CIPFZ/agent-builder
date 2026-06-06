import { AuditOutlined, CheckOutlined, DownOutlined, ExclamationCircleOutlined, SafetyCertificateOutlined, StopOutlined } from '@ant-design/icons';
import { Button, Dropdown, Tooltip } from 'antd';
import type { ComposerViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './PermissionModeControl.module.css';

interface PermissionModeControlProps {
  composer: ComposerViewModel;
  onSelect: (mode: string) => Promise<void>;
}

export function PermissionModeControl({ composer, onSelect }: PermissionModeControlProps) {
  const selectedMode = composer.permissionMode?.mode;
  const buttonClassName = [
    styles.trigger,
    selectedMode === 'full_access' ? styles.fullAccessTrigger : '',
  ].filter(Boolean).join(' ');
  const items = composer.permissionOptions.map((option) => ({
    key: option.value,
    label: (
      <PermissionModeMenuItem
        description={option.disabledReason || permissionModeDescription(option.mode, option.description)}
        label={permissionModeLabel(option.mode, option.label)}
        mode={option.mode}
        selected={selectedMode === option.mode}
      />
    ),
    disabled: option.disabled,
  }));

  return (
    <Dropdown
      classNames={{ root: styles.dropdown }}
      menu={{ items, selectedKeys: selectedMode ? [selectedMode] : [], onClick: ({ key }) => void onSelect(key) }}
      trigger={['click']}
    >
      <span className={styles.triggerWrap}>
        <Tooltip classNames={{ root: styles.permissionTooltip }} title="更换权限">
          <Button className={buttonClassName} type="text" data-testid="permission-mode-control">
            <PermissionModeIcon mode={selectedMode} />
            <span className={styles.triggerLabel}>{composer.permissionLabel}</span>
            <DownOutlined className={styles.chevron} />
          </Button>
        </Tooltip>
      </span>
    </Dropdown>
  );
}

function PermissionModeIcon({ mode }: { mode?: string }) {
  if (mode === 'full_access') {
    return <ExclamationCircleOutlined />;
  }
  if (mode === 'auto_read') {
    return <AuditOutlined />;
  }
  if (mode === 'ask') {
    return <StopOutlined />;
  }
  return <SafetyCertificateOutlined />;
}

function PermissionModeMenuItem({
  description,
  label,
  mode,
  selected,
}: {
  description: string;
  label: string;
  mode?: string;
  selected: boolean;
}) {
  return (
    <span className={styles.menuItem}>
      <PermissionModeIcon mode={mode} />
      <span className={styles.menuCopy}>
        <span className={styles.menuLabel}>{label}</span>
        <span className={styles.menuDescription}>{description}</span>
      </span>
      {selected && <CheckOutlined className={styles.selectedIcon} />}
    </span>
  );
}

function permissionModeLabel(mode: string | undefined, fallback: string) {
  switch (mode) {
    case 'ask':
      return '请求批准';
    case 'auto_read':
      return '替我审批';
    case 'full_access':
      return '完全访问权限';
    default:
      return fallback;
  }
}

function permissionModeDescription(mode: string | undefined, fallback?: string) {
  switch (mode) {
    case 'ask':
      return '编辑外部文件和使用互联网时始终询问';
    case 'auto_read':
      return '仅对检测到的风险操作请求批准';
    case 'full_access':
      return '可不受限制地访问互联网和您电脑上的任何文件';
    default:
      return fallback || '';
  }
}
