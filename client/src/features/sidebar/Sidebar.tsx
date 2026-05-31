import { useState } from 'react';
import { Button, ConfigProvider } from 'antd';
import { CaretDownOutlined, CaretRightOutlined, EditOutlined, FolderAddOutlined, FolderOutlined, MoreOutlined } from '@ant-design/icons';
import type { WorkbenchMode, WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { settingAction } from '../../runtime/staticWorkbenchAdapter.tsx';
import styles from './Sidebar.module.css';

interface SidebarProps {
  viewModel: WorkbenchViewModel;
  onModeChange: (mode: WorkbenchMode) => void;
  onProjectCreate: () => void;
  onSessionCreate: () => void;
}

export function Sidebar({ viewModel, onModeChange, onProjectCreate, onSessionCreate }: SidebarProps) {
  const [projectsOpen, setProjectsOpen] = useState(true);
  const [sessionsOpen, setSessionsOpen] = useState(true);

  return (
    <ConfigProvider
      theme={{
        components: {
          Button: {
            textHoverBg: '#f6f6f6',
            defaultActiveBg: '#f2f2f2',
          },
        },
      }}
    >
      <aside className={styles.sidebar} aria-label="工作台导航">
        <nav className={styles.primaryNav} aria-label="主菜单">
          {viewModel.sidebarActions.map((action) => (
            <Button
              key={action.id}
              className={styles.navButton}
              data-nav-id={action.id}
              disabled={action.disabled}
              block
              icon={action.icon}
              type="text"
              onClick={() => {
                if (action.id === 'new-chat') {
                  onModeChange('new-chat');
                }
              }}
            >
              {action.label}
            </Button>
          ))}
        </nav>

        <div className={styles.scrollGroups}>
          <section className={`${styles.group} ${projectsOpen ? styles.expandedGroup : ''}`} aria-labelledby="projects-heading">
            <div className={styles.groupHeader}>
              <span className={styles.groupLabel} id="projects-heading">
                项目
              </span>
              <Button
                aria-label={projectsOpen ? '折叠项目' : '展开项目'}
                aria-controls="project-list"
                aria-expanded={projectsOpen}
                className={styles.groupToggle}
                icon={projectsOpen ? <CaretDownOutlined /> : <CaretRightOutlined />}
                size="small"
                type="text"
                onClick={() => setProjectsOpen((open) => !open)}
              />
              <Button
                aria-controls="project-list"
                aria-expanded={projectsOpen}
                className={styles.groupSpacer}
                size="small"
                type="link"
                onClick={() => setProjectsOpen((open) => !open)}
              />
              <div className={styles.groupActions}>
                <Button aria-label="项目更多操作" icon={<MoreOutlined />} size="small" type="text" />
                <Button aria-label="添加项目" icon={<FolderAddOutlined />} size="small" type="text" onClick={onProjectCreate} />
              </div>
            </div>
            {projectsOpen && (
              <div className={styles.list} data-testid="project-list">
                {viewModel.projects.map((project) => (
                  <Button
                    key={project.id}
                    className={`${styles.rowButton} ${project.current ? styles.currentRow : ''}`}
                    block
                    type="text"
                    onClick={() => onModeChange('project')}
                  >
                    <FolderOutlined />
                    <span className={styles.rowText}>{project.name}</span>
                  </Button>
                ))}
              </div>
            )}
          </section>

          <section className={`${styles.group} ${sessionsOpen ? styles.expandedGroup : ''}`} aria-labelledby="sessions-heading">
            <div className={styles.groupHeader}>
              <span className={styles.groupLabel} id="sessions-heading">
                对话
              </span>
              <Button
                aria-label={sessionsOpen ? '折叠对话' : '展开对话'}
                aria-controls="session-list"
                aria-expanded={sessionsOpen}
                className={styles.groupToggle}
                icon={sessionsOpen ? <CaretDownOutlined /> : <CaretRightOutlined />}
                size="small"
                type="text"
                onClick={() => setSessionsOpen((open) => !open)}
              />
              <Button
                aria-controls="session-list"
                aria-expanded={sessionsOpen}
                className={styles.groupSpacer}
                size="small"
                type="link"
                onClick={() => setSessionsOpen((open) => !open)}
              />
              <div className={styles.groupActions}>
                <Button aria-label="对话更多操作" icon={<MoreOutlined />} size="small" type="text" />
                <Button aria-label="新建对话" icon={<EditOutlined />} size="small" type="text" onClick={onSessionCreate} />
              </div>
            </div>
            {sessionsOpen && (
              <div className={styles.list} data-testid="session-list">
                {viewModel.sessions.map((session) => (
                  <Button key={session.id} className={styles.sessionButton} block type="text">
                    <span className={styles.sessionTitle}>{session.title}</span>
                    <span className={styles.sessionAge}>{session.updatedLabel}</span>
                  </Button>
                ))}
              </div>
            )}
          </section>
        </div>

        <div className={styles.footer}>
          <Button
            className={styles.navButton}
            data-nav-id="settings"
            block
            icon={settingAction.icon}
            type="text"
            onClick={() => onModeChange('settings')}
          >
            {settingAction.label}
          </Button>
        </div>
      </aside>
    </ConfigProvider>
  );
}
