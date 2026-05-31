import { Button, Dropdown } from 'antd';
import Sender from '@ant-design/x/es/sender';
import {
  ArrowUpOutlined,
  BranchesOutlined,
  DownOutlined,
  FolderOpenOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import type { ComposerViewModel, ProjectViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './Composer.module.css';

interface ComposerProps {
  composer: ComposerViewModel;
  project: ProjectViewModel;
  showProjectContext: boolean;
}

const menu = {
  items: [
    {
      key: 'current',
      label: '当前选项',
    },
  ],
};

export function Composer({ composer, project, showProjectContext }: ComposerProps) {
  const footer = (
    <div className={styles.footer} data-testid="composer-button-row">
      <div className={styles.leftControls}>
        <Button aria-label="添加上下文" icon={<PlusOutlined />} type="text" />
        <Dropdown menu={menu} trigger={['click']}>
          <Button type="text">
            <SafetyCertificateOutlined />
            <span>{composer.permissionLabel}</span>
            <DownOutlined className={styles.chevron} />
          </Button>
        </Dropdown>
      </div>

      <div className={styles.rightControls}>
        <Dropdown menu={menu} trigger={['click']}>
          <Button className={styles.modelButton} type="text">
            <span className={styles.truncatedLabel}>{composer.modelLabel}</span>
            <DownOutlined className={styles.chevron} />
          </Button>
        </Dropdown>
        <Button aria-label="发送" icon={<ArrowUpOutlined />} shape="circle" type="primary" />
      </div>
    </div>
  );

  return (
    <div className={styles.composerWrap} data-testid="composer">
      <div className={styles.composerShell}>
        <Sender
          autoSize={{ minRows: 3, maxRows: 5 }}
          className={styles.sender}
          footer={footer}
          placeholder={composer.placeholder}
          rootClassName={styles.senderRoot}
          suffix={false}
        />
        {showProjectContext && (
          <div className={styles.limitBar} data-testid="composer-limit-bar">
            <Dropdown menu={menu} trigger={['click']}>
              <Button className={styles.limitButton} type="text">
                <FolderOpenOutlined />
                <span>{project.name}</span>
                <DownOutlined className={styles.chevron} />
              </Button>
            </Dropdown>
            {project.isGitRepository && project.branch && (
              <Dropdown menu={menu} trigger={['click']}>
                <Button className={styles.limitButton} type="text">
                  <BranchesOutlined />
                  <span>{project.branch}</span>
                  <DownOutlined className={styles.chevron} />
                </Button>
              </Dropdown>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
