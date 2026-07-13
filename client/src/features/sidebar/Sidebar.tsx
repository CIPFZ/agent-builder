import { useEffect, useState } from 'react';
import { Alert, Button, ConfigProvider, Dropdown, Input, Modal, Tooltip } from 'antd';
import {
  CaretDownOutlined,
  CaretRightOutlined,
  DeleteOutlined,
  EditOutlined,
  FolderAddOutlined,
  FolderOpenOutlined,
  FolderOutlined,
  LoadingOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MoreOutlined,
} from '@ant-design/icons';
import type {
  CreateProjectRequestViewModel,
  NewConversationDraftViewModel,
  OpenProjectRequestViewModel,
  ProjectActionRequestViewModel,
  ProjectViewModel,
  RenameProjectRequestViewModel,
  SidebarActionViewModel,
  WorkbenchMode,
  WorkbenchViewModel,
} from '../../runtime/workbenchTypes.ts';
import {
  loadSidebarPreferences,
  reconcileProjectExpandedPreferences,
  saveProjectExpandedPreference,
  saveSidebarGroupPreference,
} from '../../runtime/sidebarPreferences.ts';
import { settingAction } from '../../runtime/staticWorkbenchAdapter.tsx';
import styles from './Sidebar.module.css';

interface SidebarProps {
  collapsed?: boolean;
  viewModel: WorkbenchViewModel;
  onCollapsedChange: (collapsed: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onProjectCreate: (request: CreateProjectRequestViewModel) => Promise<void>;
  onProjectOpen: (request: OpenProjectRequestViewModel) => Promise<void>;
  onProjectRename: (request: RenameProjectRequestViewModel) => Promise<void>;
  onProjectOpenInExplorer: (request: ProjectActionRequestViewModel) => Promise<void>;
  onProjectRemove: (request: ProjectActionRequestViewModel) => Promise<void>;
  onProjectDirectorySelect: () => Promise<string>;
  onSessionCreate: (target?: NewConversationDraftViewModel) => void;
  onSessionRename: (sessionID: string, title: string) => Promise<void>;
  onSessionDelete: (sessionID: string) => void;
  onSessionSelect: (sessionID: string) => void;
}

export function Sidebar({
  collapsed = false,
  viewModel,
  onCollapsedChange,
  onModeChange,
  onProjectCreate,
  onProjectOpen,
  onProjectRename,
  onProjectOpenInExplorer,
  onProjectRemove,
  onProjectDirectorySelect,
  onSessionCreate,
  onSessionRename,
  onSessionDelete,
  onSessionSelect,
}: SidebarProps) {
  const [initialPreferences] = useState(loadSidebarPreferences);
  const [projectsOpen, setProjectsOpen] = useState(initialPreferences.projectsOpen);
  const [sessionsOpen, setSessionsOpen] = useState(initialPreferences.sessionsOpen);
  const [expandedProjectIDs, setExpandedProjectIDs] = useState<Record<string, boolean>>(initialPreferences.projectOverrides);
  const [projectDialogMode, setProjectDialogMode] = useState<'new' | 'rename' | undefined>();
  const [projectDialogTarget, setProjectDialogTarget] = useState<ProjectViewModel | undefined>();
  const [projectName, setProjectName] = useState('New project');
  const [projectBusy, setProjectBusy] = useState(false);
  const [projectError, setProjectError] = useState('');
  const [sessionDialogTarget, setSessionDialogTarget] = useState<{ id: string; title: string } | undefined>();
  const [sessionTitle, setSessionTitle] = useState('');
  const [sessionBusy, setSessionBusy] = useState(false);
  const [sessionError, setSessionError] = useState('');
  const newChatAction = viewModel.sidebarActions.find((action) => action.id === 'new-chat');
  const searchAction = viewModel.sidebarActions.find((action) => action.id === 'search');
  const primaryActions = viewModel.sidebarActions.filter((action) => action.id !== 'search');
  const standaloneSessions = viewModel.sessions.filter((session) => session.scope === 'standalone');
  const showCollapsedShortcuts = viewModel.mode !== 'plugins';
  const isPrimaryActionCurrent = (action: SidebarActionViewModel) => action.id !== 'new-chat' && viewModel.mode === action.id;

  useEffect(() => {
    reconcileProjectExpandedPreferences(viewModel.projects.map((project) => project.id));
  }, [viewModel.projects]);

  const toggleGroup = (group: 'projects' | 'sessions') => {
    if (group === 'projects') {
      setProjectsOpen((open) => {
        saveSidebarGroupPreference(group, !open);
        return !open;
      });
      return;
    }
    setSessionsOpen((open) => {
      saveSidebarGroupPreference(group, !open);
      return !open;
    });
  };

  const runSidebarAction = (action: SidebarActionViewModel) => {
    if (action.id === 'new-chat') {
      onSessionCreate();
      return;
    }
    if (action.id === 'plugins') {
      onModeChange('plugins');
    }
  };
  const preserveCurrentProjectExpansion = () => {
    const currentProjectID = viewModel.projects.find((project) => project.current)?.id;
    if (!currentProjectID) return;
    setExpandedProjectIDs((current) => {
      if (current[currentProjectID] !== undefined) return current;
      saveProjectExpandedPreference(currentProjectID, true);
      return { ...current, [currentProjectID]: true };
    });
  };
  const submitProjectDialog = async () => {
    const name = projectName.trim();
    if (!projectDialogMode || !name) {
      return;
    }
    if (!isValidProjectFolderName(name)) {
      setProjectError('项目名称不能包含路径分隔符或 Windows 保留字符');
      return;
    }
    setProjectBusy(true);
    setProjectError('');
    try {
      if (projectDialogMode === 'rename') {
        if (!projectDialogTarget) {
          throw new Error('请选择要重命名的项目');
        }
        await onProjectRename({ projectId: projectDialogTarget.id, name });
      } else {
        preserveCurrentProjectExpansion();
        await onProjectCreate({ name });
      }
      setProjectDialogMode(undefined);
      setProjectDialogTarget(undefined);
      setProjectName('New project');
    } catch (error) {
      setProjectError(error instanceof Error ? error.message : projectDialogMode === 'rename' ? '重命名项目失败' : '创建项目失败');
    } finally {
      setProjectBusy(false);
    }
  };
  const openExistingProject = async () => {
    if (projectBusy) {
      return;
    }
    setProjectBusy(true);
    try {
      const path = await onProjectDirectorySelect();
      if (!path.trim()) {
        return;
      }
      preserveCurrentProjectExpansion();
      await onProjectOpen({ path: path.trim() });
    } catch (error) {
      Modal.error({
        title: '无法选择文件夹',
        content: error instanceof Error ? error.message : '打开项目失败',
      });
    } finally {
      setProjectBusy(false);
    }
  };
  const isProjectExpanded = (project: ProjectViewModel) => expandedProjectIDs[project.id] ?? project.current;
  const toggleProject = (project: ProjectViewModel) => {
    setExpandedProjectIDs((current) => ({
      ...current,
      [project.id]: (() => {
        const expanded = !(current[project.id] ?? project.current);
        saveProjectExpandedPreference(project.id, expanded);
        return expanded;
      })(),
    }));
  };
  const openSessionRenameDialog = (session: { id: string; title: string }) => {
    setSessionDialogTarget({ id: session.id, title: session.title });
    setSessionTitle(session.title);
    setSessionError('');
  };
  const submitSessionRename = async () => {
    const title = sessionTitle.trim();
    if (!sessionDialogTarget || !title) {
      return;
    }
    setSessionBusy(true);
    setSessionError('');
    try {
      await onSessionRename(sessionDialogTarget.id, title);
      setSessionDialogTarget(undefined);
      setSessionTitle('');
    } catch (error) {
      setSessionError(error instanceof Error ? error.message : '重命名对话失败');
    } finally {
      setSessionBusy(false);
    }
  };
  const confirmSessionDelete = (session: { id: string; title: string }) => {
    Modal.confirm({
      title: '删除对话',
      content: `将删除“${session.title || '未命名对话'}”及其消息记录。`,
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: () => onSessionDelete(session.id),
    });
  };
  const sessionActionMenu = (session: { id: string; title: string }) => ({
    items: [
      { key: 'rename', icon: <EditOutlined />, label: '重命名' },
      { type: 'divider' as const },
      { key: 'delete', danger: true, icon: <DeleteOutlined />, label: '删除' },
    ],
    onClick: ({ key }: { key: string }) => {
      if (key === 'rename') {
        openSessionRenameDialog(session);
        return;
      }
      if (key === 'delete') {
        confirmSessionDelete(session);
      }
    },
  });

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
            <img className={styles.brandMark} src="/appicon.svg" alt="" aria-hidden="true" />
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
                  onClick={() => toggleGroup('projects')}
                />
                <Button
                  aria-controls="project-list"
                  aria-expanded={projectsOpen}
                  className={styles.groupSpacer}
                  size="small"
                  type="link"
                  onClick={() => toggleGroup('projects')}
                />
                <div className={styles.groupActions}>
                  <Button aria-label="项目更多操作" icon={<MoreOutlined />} size="small" type="text" />
                  <Dropdown
                    trigger={['click']}
                    menu={{
                      items: [
                        { key: 'new', disabled: projectBusy, icon: <FolderAddOutlined />, label: '新建空白项目' },
                        { key: 'existing', disabled: projectBusy, icon: <FolderOpenOutlined />, label: '使用现有文件夹' },
                      ],
                      onClick: ({ key }) => {
                        if (key === 'existing') {
                          void openExistingProject();
                          return;
                        }
                        setProjectDialogMode('new');
                        setProjectDialogTarget(undefined);
                        setProjectName('New project');
                        setProjectError('');
                      },
                    }}
                  >
                    <Button aria-label="添加项目" icon={<FolderAddOutlined />} size="small" type="text" />
                  </Dropdown>
                </div>
              </div>
              {projectsOpen && (
                <div className={styles.list} data-testid="project-list">
                  {viewModel.projects.map((project) => {
                    const projectExpanded = isProjectExpanded(project);
                    const projectSessions = viewModel.sessions.filter((session) => session.scope === 'project' && session.projectId === project.id);
                    return (
                      <div key={project.id} className={styles.projectBlock} data-project-id={project.id}>
                        <div className={`${styles.projectRow} ${project.current ? styles.currentRow : ''}`} onClick={() => toggleProject(project)}>
                          <Button
                            aria-expanded={projectExpanded}
                            className={styles.projectMainButton}
                            type="text"
                            onClick={(event) => {
                              event.stopPropagation();
                              toggleProject(project);
                            }}
                          >
                            <FolderOutlined />
                            <span className={styles.rowText}>{project.name}</span>
                            <span className={styles.projectChevron}>{projectExpanded ? <CaretDownOutlined /> : <CaretRightOutlined />}</span>
                          </Button>
                          <div className={styles.projectRowActions} onClick={(event) => event.stopPropagation()}>
                            <Dropdown
                              trigger={['click']}
                              menu={{
                                items: [
                                  { key: 'rename', icon: <EditOutlined />, label: '重命名项目' },
                                  { key: 'open-explorer', icon: <FolderOpenOutlined />, label: '在资源管理器打开' },
                                  { type: 'divider' },
                                  { key: 'remove', danger: true, icon: <DeleteOutlined />, label: '移除' },
                                ],
                                onClick: ({ key }) => {
                                  if (key === 'rename') {
                                    setProjectDialogMode('rename');
                                    setProjectDialogTarget(project);
                                    setProjectName(project.name);
                                    setProjectError('');
                                    return;
                                  }
                                  if (key === 'open-explorer') {
                                    void onProjectOpenInExplorer({ projectId: project.id }).catch((error) => {
                                      Modal.error({
                                        title: '无法在资源管理器打开',
                                        content: error instanceof Error ? error.message : '打开项目文件夹失败',
                                      });
                                    });
                                    return;
                                  }
                                  if (key === 'remove') {
                                    const runningCount = projectSessions.filter((session) => session.busy).length;
                                    Modal.confirm({
                                      title: '移除项目',
                                      content: (
                                        <div>
                                          {runningCount > 0 ? (
                                            <Alert
                                              showIcon
                                              type="warning"
                                              message={`该项目中有 ${runningCount} 个正在运行的会话`}
                                              description="移除项目后，这些任务将被终止，且无法继续恢复。"
                                            />
                                          ) : null}
                                          <p>将从应用中移除项目记录及其关联的会话和消息数据，但不会删除磁盘上的项目文件夹。</p>
                                        </div>
                                      ),
                                      okText: '移除',
                                      cancelText: '取消',
                                      okButtonProps: { danger: true },
                                      onOk: () => onProjectRemove({ projectId: project.id }),
                                    });
                                    return;
                                  }
                                },
                              }}
                            >
                              <Button
                                aria-label={`项目更多操作 ${project.name}`}
                                className={styles.projectActionButton}
                                icon={<MoreOutlined />}
                                size="small"
                                type="text"
                              />
                            </Dropdown>
                            <Button
                              aria-label={`在项目 ${project.name} 中新建对话`}
                              className={styles.projectActionButton}
                              icon={<EditOutlined />}
                              size="small"
                              type="text"
                              onClick={() => onSessionCreate({ active: true, scope: 'project', projectId: project.id })}
                            />
                          </div>
                        </div>
                        {projectExpanded && projectSessions.length > 0 && (
                          <div className={styles.projectSessionList}>
                            {projectSessions.map((session) => (
                              <div
                                key={session.id}
                                className={`${styles.projectSessionRow} ${session.active ? styles.currentRow : ''}`}
                                data-session-id={session.id}
                                data-session-busy={session.busy ? 'true' : 'false'}
                              >
                                <Button className={styles.projectSessionButton} block type="text" onClick={() => onSessionSelect(session.id)}>
                                  <span className={styles.projectSessionTitle}>{session.title}</span>
                                  <span className={styles.projectSessionAge}>{session.updatedLabel}</span>
                                </Button>
                                {session.busy && (
                                  <span className={styles.sessionSpinner} aria-label="会话执行中" title="会话执行中">
                                    <LoadingOutlined spin />
                                  </span>
                                )}
                                <Dropdown menu={sessionActionMenu(session)} trigger={['click']}>
                                  <Button
                                    aria-label={`对话更多操作 ${session.title}`}
                                    className={styles.sessionActionsButton}
                                    icon={<MoreOutlined />}
                                    size="small"
                                    type="text"
                                  />
                                </Dropdown>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    );
                  })}
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
                  onClick={() => toggleGroup('sessions')}
                />
                <Button
                  aria-controls="session-list"
                  aria-expanded={sessionsOpen}
                  className={styles.groupSpacer}
                  size="small"
                  type="link"
                  onClick={() => toggleGroup('sessions')}
                />
                <div className={styles.groupActions}>
                  <Button aria-label="对话更多操作" icon={<MoreOutlined />} size="small" type="text" />
                  <Button aria-label="新建对话" icon={<EditOutlined />} size="small" type="text" onClick={() => onSessionCreate({ active: true, scope: 'standalone' })} />
                </div>
              </div>
              {sessionsOpen && (
                <div className={`${styles.list} ${styles.sessionList}`} data-testid="session-list">
                  {standaloneSessions.map((session) => (
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
                      <Dropdown menu={sessionActionMenu(session)} trigger={['click']}>
                        <Button
                          aria-label={`对话更多操作 ${session.title}`}
                          className={styles.sessionActionsButton}
                          icon={<MoreOutlined />}
                          size="small"
                          type="text"
                        />
                      </Dropdown>
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
        <Modal
          centered
          className={styles.projectNameModal}
          width={420}
          title={
            <div className={styles.projectNameTitleBlock}>
              <div className={styles.projectNameTitle}>{projectDialogMode === 'rename' ? 'Rename project' : 'Name project'}</div>
              <div className={styles.projectNameSubtitle}>{projectDialogMode === 'rename' ? 'Keep it short and recognizable' : 'Keep it short and recognizable'}</div>
            </div>
          }
          open={Boolean(projectDialogMode)}
          okText="保存"
          cancelText="取消"
          confirmLoading={projectBusy}
          okButtonProps={{ disabled: !projectName.trim(), className: styles.projectNameSaveButton }}
          cancelButtonProps={{ className: styles.projectNameCancelButton }}
          onCancel={() => {
            if (!projectBusy) {
              setProjectDialogMode(undefined);
              setProjectDialogTarget(undefined);
              setProjectName('New project');
              setProjectError('');
            }
          }}
          onOk={submitProjectDialog}
        >
          <Input
            autoFocus
            className={styles.projectNameInput}
            placeholder="New project"
            value={projectName}
            onChange={(event) => setProjectName(event.target.value)}
            onPressEnter={() => {
              void submitProjectDialog();
            }}
          />
          <div className={styles.projectNameHint}>{projectDialogMode === 'rename' ? `Renames folder: ${projectDialogTarget?.path ?? ''}` : 'Creates under the desktop app data/projects directory.'}</div>
          {projectError ? <Alert message={projectError} type="error" showIcon style={{ marginTop: 12 }} /> : null}
        </Modal>
        <Modal
          centered
          className={styles.projectNameModal}
          width={420}
          title={
            <div className={styles.projectNameTitleBlock}>
              <div className={styles.projectNameTitle}>重命名对话</div>
              <div className={styles.projectNameSubtitle}>用于在项目和对话列表中识别这次会话</div>
            </div>
          }
          open={Boolean(sessionDialogTarget)}
          okText="保存"
          cancelText="取消"
          confirmLoading={sessionBusy}
          okButtonProps={{ disabled: !sessionTitle.trim(), className: styles.projectNameSaveButton }}
          cancelButtonProps={{ className: styles.projectNameCancelButton }}
          onCancel={() => {
            if (!sessionBusy) {
              setSessionDialogTarget(undefined);
              setSessionTitle('');
              setSessionError('');
            }
          }}
          onOk={submitSessionRename}
        >
          <Input
            autoFocus
            className={styles.projectNameInput}
            placeholder="New chat"
            value={sessionTitle}
            onChange={(event) => setSessionTitle(event.target.value)}
            onPressEnter={() => {
              void submitSessionRename();
            }}
          />
          {sessionError ? <Alert message={sessionError} type="error" showIcon style={{ marginTop: 12 }} /> : null}
        </Modal>
      </aside>
    </ConfigProvider>
  );
}

function isValidProjectFolderName(name: string) {
  return Boolean(name.trim()) && !/[\\/:*?"<>|]/.test(name) && name !== '.' && name !== '..';
}
