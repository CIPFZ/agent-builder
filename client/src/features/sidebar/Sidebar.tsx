import { useState } from 'react';
import { Button, ConfigProvider, Popconfirm, Tooltip } from 'antd';
import {
  CaretDownOutlined,
  CaretRightOutlined,
  DeleteOutlined,
  EditOutlined,
  FolderAddOutlined,
  FolderOutlined,
  LoadingOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoreOutlined,
} from '@ant-design/icons';
import type { SidebarActionViewModel, WorkbenchMode, WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { settingAction } from '../../runtime/staticWorkbenchAdapter.tsx';
import styles from './Sidebar.module.css';

interface SidebarProps {
  collapsed?: boolean;
  viewModel: WorkbenchViewModel;
  onCollapsedChange: (collapsed: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onProjectCreate: () => void;
  onSessionCreate: () => void;
  onSessionDelete: (sessionID: string) => void;
  onSessionSelect: (sessionID: string) => void;
}

export function Sidebar({
  collapsed = false,
  viewModel,
  onCollapsedChange,
  onModeChange,
  onProjectCreate,
  onSessionCreate,
  onSessionDelete,
  onSessionSelect,
}: SidebarProps) {
  const [projectsOpen, setProjectsOpen] = useState(true);
  const [sessionsOpen, setSessionsOpen] = useState(true);
  const newChatAction = viewModel.sidebarActions.find((action) => action.id === 'new-chat');
  const searchAction = viewModel.sidebarActions.find((action) => action.id === 'search');
  const primaryActions = viewModel.sidebarActions.filter((action) => action.id !== 'search');
  const showCollapsedShortcuts = viewModel.mode !== 'plugins';
  const isPrimaryActionCurrent = (action: SidebarActionViewModel) => action.id !== 'new-chat' && viewModel.mode === action.id;

  const runSidebarAction = (action: SidebarActionViewModel) => {
    if (action.id === 'new-chat') {
      onSessionCreate();
      return;
    }
    if (action.id === 'plugins') {
      onModeChange('plugins');
    }
  };

  if (collapsed) {
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
        <aside className={`${styles.sidebar} ${styles.collapsed}`} aria-label="工作台快捷导航">
          <div className={styles.floatingControls}>
            <Tooltip title="打开边栏">
              <Button
                aria-label="打开边栏"
                className={styles.floatingButton}
                icon={<MenuUnfoldOutlined />}
                type="text"
                onClick={() => onCollapsedChange(false)}
              />
            </Tooltip>
            {showCollapsedShortcuts && searchAction && (
              <Tooltip title={searchAction.label}>
                <Button aria-label={searchAction.label} className={styles.floatingButton} icon={searchAction.icon} type="text" />
              </Tooltip>
            )}
            {showCollapsedShortcuts && newChatAction && (
              <Tooltip title={newChatAction.label}>
                <Button
                  aria-label={newChatAction.label}
                  className={styles.floatingButton}
                  icon={newChatAction.icon}
                  type="text"
                  onClick={() => runSidebarAction(newChatAction)}
                />
              </Tooltip>
            )}
          </div>
        </aside>
      </ConfigProvider>
    );
  }

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
        <div className={styles.header}>
          <div className={styles.brand}>
            <span className={styles.brandMark}>A</span>
            <span className={styles.brandText}>Agent Builder</span>
          </div>
          <div className={styles.headerActions}>
            {searchAction && (
              <Tooltip title={searchAction.label}>
                <Button aria-label={searchAction.label} icon={searchAction.icon} type="text" />
              </Tooltip>
            )}
            <Tooltip title="收起边栏">
              <Button
                aria-label="收起边栏"
                className={styles.collapseButton}
                icon={<MenuFoldOutlined />}
                type="text"
                onClick={() => onCollapsedChange(true)}
              />
            </Tooltip>
          </div>
        </div>

        <nav className={styles.primaryNav} aria-label="主菜单">
          {primaryActions.map((action) => (
            <Tooltip key={action.id} title={undefined} placement="right">
              <Button
                className={`${styles.navButton} ${isPrimaryActionCurrent(action) ? styles.currentRow : ''}`}
                data-nav-id={action.id}
                disabled={action.disabled}
                block
                icon={action.icon}
                type="text"
                onClick={() => runSidebarAction(action)}
              >
                {action.label}
              </Button>
            </Tooltip>
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
                    <div
                      key={session.id}
                      className={`${styles.sessionRow} ${session.active ? styles.currentRow : ''}`}
                      data-session-id={session.id}
                      data-session-busy={session.busy ? 'true' : 'false'}
                    >
                      <Button className={styles.sessionButton} block type="text" onClick={() => onSessionSelect(session.id)}>
                        <span className={styles.sessionTitle}>{session.title}</span>
                        <span className={styles.sessionAge}>{session.updatedLabel}</span>
                      </Button>
                      {session.busy && (
                        <span className={styles.sessionSpinner} aria-label="会话执行中" title="会话执行中">
                          <LoadingOutlined spin />
                        </span>
                      )}
                      <Popconfirm
                        title="删除对话"
                        description="此操作会删除该对话及其消息记录。"
                        okText="删除"
                        cancelText="取消"
                        okButtonProps={{ danger: true }}
                        onConfirm={() => onSessionDelete(session.id)}
                      >
                        <Button
                          aria-label={`删除对话 ${session.title}`}
                          className={styles.sessionDeleteButton}
                          danger
                          icon={<DeleteOutlined />}
                          size="small"
                          title="删除对话"
                          type="text"
                        />
                      </Popconfirm>
                    </div>
                  ))}
                </div>
              )}
            </section>
        </div>

        <div className={styles.footer}>
          <Tooltip title={undefined} placement="right">
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
          </Tooltip>
        </div>
      </aside>
    </ConfigProvider>
  );
}
