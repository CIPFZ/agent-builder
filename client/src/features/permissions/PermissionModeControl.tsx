import { CheckOutlined, DownOutlined, ExclamationCircleOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Button, Dropdown, Tooltip } from 'antd';
import type { ComposerViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './PermissionModeControl.module.css';

interface PermissionModeControlProps {
  composer: ComposerViewModel;
  onSelect: (mode: string) => Promise<void>;
}

export function PermissionModeControl({ composer, onSelect }: PermissionModeControlProps) {
  const selectedMode = composer.permissionMode?.mode;
  const selectedDescription = composer.permissionMode?.description;
  const buttonClassName = selectedMode === 'full_access' ? `${styles.trigger} ${styles.fullAccessTrigger}` : styles.trigger;
  const items = composer.permissionOptions.map((option) => ({
    key: option.value,
    label: (
      <Tooltip placement="right" title={option.disabledReason || option.description}>
        <span className={styles.menuItem}>
          <PermissionModeIcon mode={option.mode} />
          <span className={styles.menuLabel}>{option.label}</span>
          {selectedMode === option.mode && <CheckOutlined className={styles.selectedIcon} />}
        </span>
      </Tooltip>
    ),
    disabled: option.disabled,
  }));

  return (
    <Dropdown classNames={{ root: styles.dropdown }} menu={{ items, selectedKeys: selectedMode ? [selectedMode] : [], onClick: ({ key }) => void onSelect(key) }} trigger={['click']}>
      <span className={styles.triggerWrap}>
        <Tooltip title={selectedDescription}>
          <Button className={buttonClassName} type="text" data-testid="permission-mode-control">
            <PermissionModeIcon mode={selectedMode} />
            <span>{composer.permissionLabel}</span>
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
  return <SafetyCertificateOutlined />;
}
