import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AuditOutlined,
  ArrowDownOutlined,
  CloseOutlined,
  ConsoleSqlOutlined,
  CopyOutlined,
  EditOutlined,
  FolderOpenOutlined,
  LayoutOutlined,
  LeftOutlined,
  MoreOutlined,
  PlusOutlined,
  RightOutlined,
} from '@ant-design/icons';
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react';
import { Button, Dropdown, Input, Modal, Tooltip, message as antdMessage } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { NewConversationDraftViewModel, TerminalEventViewModel, TerminalViewModel, WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Composer } from '../composer/Composer.tsx';
import { RunProjectionPreview } from '../diagnostics/RunProjectionPreview.tsx';
import { ReactCallchainInspector } from '../diagnostics/ReactCallchainInspector.tsx';
import { TurnDiagnosticsPanel } from '../diagnostics/TurnDiagnosticsPanel.tsx';
import { Timeline } from '../timeline/Timeline.tsx';
import { TerminalPane } from './TerminalPane.tsx';
import styles from './Workspace.module.css';

type RightPanelKind = 'review' | 'files' | 'terminal';

const RIGHT_PANEL_DEFAULT_WIDTH = 360;
const RIGHT_PANEL_MIN_WIDTH = 300;
const RIGHT_PANEL_MAX_WIDTH = 720;
const WORKSPACE_MIN_WIDTH_WITH_PANEL = 480;
const RIGHT_PANEL_DOCKED_MIN_WORKSPACE_WIDTH = WORKSPACE_MIN_WIDTH_WITH_PANEL + RIGHT_PANEL_MIN_WIDTH;
const RIGHT_PANEL_DRAWER_MARGIN = 72;

interface RightPanelTabState {
  id: string;
  kind: RightPanelKind;
  title: string;
  terminal?: TerminalViewModel;
}

interface WorkspaceProps {
  sidebarCollapsed?: boolean;
  viewModel: WorkbenchViewModel;
  switchingSessionID?: string;
  onMinimumWorkspaceWidthChange?: (width: number) => void;
  onModelSelect: (configuredProviderID: string, model: string) => Promise<void>;
  onPermissionDecide: (permissionID: string, action: 'allow' | 'allow_session' | 'deny') => Promise<void>;
  onPermissionModeSelect: (mode: string) => Promise<void>;
  onPromptCancel: () => Promise<void>;
  onSessionRename: (sessionID: string, title: string) => Promise<void>;
  onPromptSubmit: (prompt: string) => Promise<void>;
  onNewConversationDraftChange: (target: NewConversationDraftViewModel) => void;
  onInterruptedDone: (turnID: string) => Promise<void>;
  onRunCheckpointResume?: (runID: string, checkpointID: string) => Promise<void>;
  onRunTaskExecute?: (runID: string, taskID: string) => Promise<void>;
  onSessionTerminalsList: (sessionID: string) => Promise<TerminalViewModel[]>;
  onTerminalCreate: (request: { sessionId: string; cwd?: string; columns?: number; rows?: number }) => Promise<TerminalViewModel>;
  onTerminalDelete: (terminalID: string) => Promise<void>;
  onTerminalInput: (terminalID: string, data: string) => Promise<TerminalViewModel>;
  onTerminalResize: (terminalID: string, columns: number, rows: number) => Promise<TerminalViewModel>;
  onTerminalSubscribe: (terminalID: string, onEvent: (event: TerminalEventViewModel) => void) => Promise<() => void> | (() => void);
}

export function Workspace({
  sidebarCollapsed = false,
  viewModel,
  switchingSessionID = '',
  onMinimumWorkspaceWidthChange,
  onModelSelect,
  onPermissionDecide,
  onPermissionModeSelect,
  onPromptCancel,
  onSessionRename,
  onPromptSubmit,
  onNewConversationDraftChange,
  onInterruptedDone,
  onRunCheckpointResume,
  onRunTaskExecute,
  onSessionTerminalsList,
  onTerminalCreate,
  onTerminalDelete,
  onTerminalInput,
  onTerminalResize,
  onTerminalSubscribe,
}: WorkspaceProps) {
  const [messageApi, messageContextHolder] = antdMessage.useMessage();
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameTitle, setRenameTitle] = useState('');
  const [renaming, setRenaming] = useState(false);
  const [rightPanelVisible, setRightPanelVisible] = useState(false);
  const [rightPanelWidth, setRightPanelWidth] = useState(RIGHT_PANEL_DEFAULT_WIDTH);
  const [rightPanelTabs, setRightPanelTabs] = useState<RightPanelTabState[]>([]);
  const [activeRightPanelID, setActiveRightPanelID] = useState('');
  const [rightPanelTabsOverflow, setRightPanelTabsOverflow] = useState(false);
  const [rightPanelDocked, setRightPanelDocked] = useState(false);
  const rightPanelTabsRef = useRef<HTMLDivElement | null>(null);
  const workspaceRef = useRef<HTMLElement | null>(null);
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);
  const hasProjectContext = Boolean(viewModel.currentProject.id || viewModel.currentProject.name || viewModel.currentProject.path);
  const canUseProjectSideTools = hasProjectContext;
  const hasConversation = viewModel.conversation.length > 0;
  const activeSession = viewModel.sessions.find((session) => session.active);
  const isSessionSwitching = Boolean(switchingSessionID && activeSession?.id === switchingSessionID && !hasConversation && viewModel.timeline.length === 0);
  const sessionTitle = activeSession?.title || viewModel.currentProject.name || '新对话';
  const title =
    viewModel.mode === 'project' && hasProjectContext ? `我们应该在 ${viewModel.currentProject.name} 中构建什么？` : '我们该做什么？';
  const bubbleItems = viewModel.conversation.map((message) => ({
    key: message.id,
    role: message.role === 'user' ? 'user' : 'ai',
    content: message.content,
    status: message.status,
    footer: <MessageActions content={message.content} createdAt={message.createdAt} messageApi={messageApi} role={message.role} />,
  }));
  const updateJumpToBottomVisibility = useCallback(() => {
    const container = scrollContainerRef.current;
    if (!container) {
      setShowJumpToBottom(false);
      return;
    }
    const distanceToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
    setShowJumpToBottom(distanceToBottom > 180);
  }, []);
  const jumpToBottom = () => {
    scrollContainerRef.current?.scrollTo({
      top: scrollContainerRef.current.scrollHeight,
      behavior: 'smooth',
    });
  };
  const rightPanelHasTabs = rightPanelTabs.length > 0;
  const rightPanelOpen = rightPanelVisible;
  const activeRightPanelTab = rightPanelTabs.find((tab) => tab.id === activeRightPanelID) ?? rightPanelTabs[0];
  const activeSessionID = activeSession?.id ?? '';
  const replaceTerminalTabs = useCallback((terminals: TerminalViewModel[]) => {
    setRightPanelTabs((current) => {
      const projectTabs = current.filter((tab) => tab.kind !== 'terminal');
      const terminalTabs = terminals.map((terminal, index) => ({
        id: terminal.id,
        kind: 'terminal' as const,
        title: terminal.title || `终端 ${index + 1}`,
        terminal,
      }));
      const next = [...projectTabs, ...terminalTabs];
      setActiveRightPanelID((activeID) => {
        if (!activeID) {
          return next[0]?.id ?? '';
        }
        if (next.some((tab) => tab.id === activeID)) {
          return activeID;
        }
        if (current.some((tab) => tab.id === activeID && tab.kind === 'terminal')) {
          return terminalTabs[0]?.id ?? projectTabs[0]?.id ?? '';
        }
        return next[0]?.id ?? '';
      });
      return next;
    });
  }, []);
  const refreshSessionTerminals = useCallback(
    async (sessionID: string) => {
      const terminals = await onSessionTerminalsList(sessionID);
      replaceTerminalTabs(terminals.filter((terminal) => terminal.sessionId === sessionID));
    },
    [onSessionTerminalsList, replaceTerminalTabs],
  );
  const openSingletonPanel = (kind: Exclude<RightPanelKind, 'terminal'>) => {
    if (!canUseProjectSideTools) {
      void messageApi.warning('文件和审查只在项目中可用');
      return;
    }
    setRightPanelVisible(true);
    const existing = rightPanelTabs.find((tab) => tab.kind === kind);
    if (existing) {
      setActiveRightPanelID(existing.id);
      return;
    }
    const tab: RightPanelTabState = {
      id: kind,
      kind,
      title: kind === 'review' ? '审查' : '文件',
    };
    setRightPanelTabs((current) => [...current, tab]);
    setActiveRightPanelID(tab.id);
  };
  const toggleRightPanel = () => {
    if (rightPanelOpen) {
      setRightPanelVisible(false);
      return;
    }
    setRightPanelVisible(true);
  };
  const addTerminalTab = async () => {
    if (!activeSession?.id) {
      void messageApi.warning('请先进入会话再创建终端');
      return;
    }
    setRightPanelVisible(true);
    try {
      const terminal = await onTerminalCreate({ sessionId: activeSession.id, columns: 100, rows: 24 });
      await refreshSessionTerminals(activeSession.id);
      setActiveRightPanelID(terminal.id);
    } catch (error) {
      void messageApi.error(runtimePanelErrorMessage(error, '创建终端失败'));
    }
  };
  const closeRightPanelTab = async (id: string) => {
    const closingTerminalID = rightPanelTabs.find((tab) => tab.id === id && tab.kind === 'terminal')?.terminal?.id;
    if (closingTerminalID) {
      try {
        await onTerminalDelete(closingTerminalID);
        if (activeSession?.id) {
          await refreshSessionTerminals(activeSession.id);
        }
      } catch (error) {
        void messageApi.error(runtimePanelErrorMessage(error, '关闭终端失败'));
      }
      return;
    }
    setRightPanelTabs((current) => {
      const index = current.findIndex((tab) => tab.id === id);
      const next = current.filter((tab) => tab.id !== id);
      if (next.length === 0) {
        setRightPanelVisible(false);
      }
      if (activeRightPanelID === id) {
        setActiveRightPanelID(next[Math.max(0, index - 1)]?.id ?? next[0]?.id ?? '');
      }
      return next;
    });
  };
  const updateTerminalTab = useCallback((terminal: TerminalViewModel) => {
    setRightPanelTabs((current) =>
      current.map((tab) => (tab.kind === 'terminal' && tab.terminal?.id === terminal.id ? { ...tab, terminal } : tab)),
    );
  }, []);
  const openRightPanelTool = (kind: RightPanelKind) => {
    if (kind === 'terminal') {
      void addTerminalTab();
    } else {
      openSingletonPanel(kind);
    }
  };
  const rightPanelMaxWidth = useCallback(() => {
    const workspaceWidth = workspaceRef.current?.getBoundingClientRect().width ?? window.innerWidth;
    if (workspaceWidth >= RIGHT_PANEL_DOCKED_MIN_WORKSPACE_WIDTH) {
      return Math.min(RIGHT_PANEL_MAX_WIDTH, Math.max(RIGHT_PANEL_MIN_WIDTH, workspaceWidth - WORKSPACE_MIN_WIDTH_WITH_PANEL));
    }
    return Math.min(520, Math.max(260, window.innerWidth - RIGHT_PANEL_DRAWER_MARGIN));
  }, []);
  const clampRightPanelWidth = useCallback(
    (width: number) => {
      const maxWidth = rightPanelMaxWidth();
      const minWidth = Math.min(RIGHT_PANEL_MIN_WIDTH, maxWidth);
      return Math.min(maxWidth, Math.max(minWidth, width));
    },
    [rightPanelMaxWidth],
  );
  const startRightPanelResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    const pointerID = event.pointerId;
    const startX = event.clientX;
    const startWidth = rightPanelWidth;
    const target = event.currentTarget;

    target.setPointerCapture(pointerID);

    const updateRightPanelWidth = (moveEvent: PointerEvent) => {
      setRightPanelWidth(clampRightPanelWidth(startWidth + startX - moveEvent.clientX));
    };
    const stopRightPanelResize = () => {
      target.releasePointerCapture(pointerID);
      window.removeEventListener('pointermove', updateRightPanelWidth);
      window.removeEventListener('pointerup', stopRightPanelResize);
      window.removeEventListener('pointercancel', stopRightPanelResize);
    };

    window.addEventListener('pointermove', updateRightPanelWidth);
    window.addEventListener('pointerup', stopRightPanelResize);
    window.addEventListener('pointercancel', stopRightPanelResize);
  };
  const scrollRightPanelTabs = (direction: -1 | 1) => {
    const tabs = rightPanelTabsRef.current;
    if (!tabs) {
      return;
    }
    tabs.scrollBy({ left: direction * Math.max(140, tabs.clientWidth * 0.7), behavior: 'smooth' });
  };
  useEffect(() => {
    let cancelled = false;
    if (!activeSessionID) {
      replaceTerminalTabs([]);
      return () => {
        cancelled = true;
      };
    }
    onSessionTerminalsList(activeSessionID)
      .then((terminals) => {
        if (!cancelled) {
          replaceTerminalTabs(terminals.filter((terminal) => terminal.sessionId === activeSessionID));
        }
      })
      .catch((error) => {
        if (!cancelled) {
          void messageApi.error(runtimePanelErrorMessage(error, '读取终端列表失败'));
          replaceTerminalTabs([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeSessionID, messageApi, onSessionTerminalsList, replaceTerminalTabs]);
  useEffect(() => {
    const frame = window.requestAnimationFrame(updateJumpToBottomVisibility);
    return () => window.cancelAnimationFrame(frame);
  }, [updateJumpToBottomVisibility, viewModel.conversation.length, viewModel.timeline.length]);
  useEffect(() => {
    const activeTab = rightPanelTabsRef.current?.querySelector<HTMLElement>('[data-active="true"]');
    activeTab?.scrollIntoView({ block: 'nearest', inline: 'center' });
  }, [activeRightPanelID, rightPanelTabs.length]);
  useEffect(() => {
    if (canUseProjectSideTools) {
      return;
    }
    setRightPanelTabs((current) => current.filter((tab) => tab.kind === 'terminal'));
  }, [canUseProjectSideTools]);
  useEffect(() => {
    if (rightPanelTabs.length === 0) {
      if (activeRightPanelID) {
        setActiveRightPanelID('');
      }
      return;
    }
    if (!rightPanelTabs.some((tab) => tab.id === activeRightPanelID)) {
      setActiveRightPanelID(rightPanelTabs[0].id);
    }
  }, [activeRightPanelID, rightPanelTabs]);
  useEffect(() => {
    const tabs = rightPanelTabsRef.current;
    if (!tabs) {
      setRightPanelTabsOverflow(false);
      return;
    }
    const updateOverflow = () => {
      setRightPanelTabsOverflow(tabs.scrollWidth > tabs.clientWidth + 1);
    };
    updateOverflow();
    window.addEventListener('resize', updateOverflow);
    return () => window.removeEventListener('resize', updateOverflow);
  }, [rightPanelTabs.length, rightPanelWidth, rightPanelOpen]);
  useEffect(() => {
    const updateRightPanelBounds = () => {
      setRightPanelWidth((current) => clampRightPanelWidth(current));
    };
    window.addEventListener('resize', updateRightPanelBounds);
    return () => window.removeEventListener('resize', updateRightPanelBounds);
  }, [clampRightPanelWidth]);
  useEffect(() => {
    const updateDockedState = () => {
      const workspaceWidth = workspaceRef.current?.getBoundingClientRect().width ?? window.innerWidth;
      setRightPanelDocked(workspaceWidth >= RIGHT_PANEL_DOCKED_MIN_WORKSPACE_WIDTH);
    };
    updateDockedState();

    const workspaceElement = workspaceRef.current;
    const observer =
      typeof ResizeObserver === 'undefined' || !workspaceElement
        ? undefined
        : new ResizeObserver(() => {
            updateDockedState();
          });
    if (workspaceElement) {
      observer?.observe(workspaceElement);
    }
    window.addEventListener('resize', updateDockedState);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', updateDockedState);
    };
  }, []);
  useEffect(() => {
    setRightPanelWidth((current) => clampRightPanelWidth(current));
  }, [clampRightPanelWidth, rightPanelDocked, rightPanelOpen, sidebarCollapsed]);
  useEffect(() => {
    onMinimumWorkspaceWidthChange?.(rightPanelOpen ? RIGHT_PANEL_DOCKED_MIN_WORKSPACE_WIDTH : 0);
    return () => onMinimumWorkspaceWidthChange?.(0);
  }, [onMinimumWorkspaceWidthChange, rightPanelOpen]);
  const openRenameDialog = () => {
    if (!activeSession) {
      void messageApi.warning('请先选择一个对话');
      return;
    }
    setRenameTitle(activeSession.title);
    setRenameOpen(true);
  };
  const submitRename = async () => {
    if (!activeSession) {
      setRenameOpen(false);
      return;
    }
    const nextTitle = renameTitle.trim();
    if (!nextTitle) {
      void messageApi.warning('请输入对话名称');
      return;
    }
    if (nextTitle === activeSession.title) {
      setRenameOpen(false);
      return;
    }
    setRenaming(true);
    try {
      await onSessionRename(activeSession.id, nextTitle);
      setRenameOpen(false);
      void messageApi.success('对话名称已更新');
    } catch {
      void messageApi.error('更新对话名称失败');
    } finally {
      setRenaming(false);
    }
  };
  const copyInterruptedSummary = async (summary: string) => {
    try {
      await copyText(summary);
      void messageApi.success('Copied');
    } catch {
      void messageApi.error('Copy failed');
    }
  };
  const startInterruptedFollowUp = async (prompt: string) => {
    await onPromptSubmit(prompt);
  };
  const markInterruptedDone = async (turnID: string) => {
    await onInterruptedDone(turnID);
    void messageApi.success('Interrupted turn marked done');
  };

  return (
    <section
      ref={workspaceRef}
      className={`${styles.workspace} ${rightPanelOpen && rightPanelDocked ? styles.workspacePanelDocked : ''}`}
      data-has-conversation={hasConversation}
      data-mode={viewModel.mode}
      data-sidebar-collapsed={sidebarCollapsed ? 'true' : 'false'}
      style={{ '--right-panel-width': `${rightPanelWidth}px` } as CSSProperties}
    >
      {messageContextHolder}
      <header className={styles.sessionHeader}>
        <div className={styles.sessionTitleWrap}>
          <h2 className={styles.sessionTitle}>{sessionTitle}</h2>
          <Dropdown
            menu={{
              items: [{ key: 'rename', icon: <EditOutlined />, label: '重命名' }],
              onClick: ({ key }) => {
                if (key === 'rename') {
                  openRenameDialog();
                }
              },
            }}
            trigger={['click']}
          >
            <Button aria-label="更多对话操作" className={styles.headerIconButton} icon={<MoreOutlined />} type="text" />
          </Dropdown>
        </div>
        <div className={styles.headerActions} aria-label="工作区面板">
          <Tooltip title={rightPanelOpen ? '关闭右侧面板' : '打开右侧面板'}>
            <Button
              aria-label={rightPanelOpen ? '关闭右侧面板' : '打开右侧面板'}
              className={`${styles.headerIconButton} ${rightPanelOpen ? styles.headerIconButtonActive : ''}`}
              icon={<LayoutOutlined />}
              type="text"
              onClick={toggleRightPanel}
            />
          </Tooltip>
        </div>
      </header>
      <Modal
        cancelText="取消"
        confirmLoading={renaming}
        okText="保存"
        open={renameOpen}
        title="重命名对话"
        onCancel={() => {
          if (!renaming) {
            setRenameOpen(false);
          }
        }}
        onOk={submitRename}
      >
        <Input
          aria-label="对话名称"
          autoFocus
          maxLength={80}
          placeholder="请输入对话名称"
          showCount
          value={renameTitle}
          onChange={(event) => setRenameTitle(event.target.value)}
          onPressEnter={() => {
            void submitRename();
          }}
        />
      </Modal>
        <div
          ref={hasConversation || isSessionSwitching ? scrollContainerRef : undefined}
          className={hasConversation || isSessionSwitching ? styles.chatContent : styles.content}
          onScroll={hasConversation || isSessionSwitching ? updateJumpToBottomVisibility : undefined}
        >
          {viewModel.timeline.length > 0 ? (
            <div className={styles.timelineLayout}>
              <div className={styles.timelineColumn}>
                <Timeline items={viewModel.timeline} onPermissionDecide={onPermissionDecide} />
              </div>
            </div>
          ) : hasConversation ? (
            <Bubble.List
              autoScroll
              className={styles.conversation}
              items={bubbleItems}
              role={{
                ai: {
                  placement: 'start',
                  variant: 'borderless',
                  className: styles.assistantBubble,
                },
                user: {
                  placement: 'end',
                  variant: 'filled',
                  className: styles.userBubble,
                },
              }}
            />
          ) : isSessionSwitching ? (
            <div className={styles.sessionLoading} role="status" aria-live="polite">
              <span className={styles.sessionLoadingDot} />
              <span>正在加载对话...</span>
            </div>
          ) : (
            <h1 className={styles.title}>{title}</h1>
          )}
          {hasConversation && showJumpToBottom && (
            <button
              aria-label="璺冲埌搴曢儴"
              className={styles.jumpToBottomButton}
              type="button"
              onClick={jumpToBottom}
            >
              <ArrowDownOutlined />
            </button>
          )}
          <Composer
            composer={viewModel.composer}
            project={viewModel.currentProject}
            projects={viewModel.projects}
            newConversationDraft={viewModel.newConversationDraft}
            showProjectContext={viewModel.mode === 'project' && hasProjectContext && !hasConversation}
            onNewConversationDraftChange={onNewConversationDraftChange}
            onModelSelect={onModelSelect}
            onPermissionModeSelect={onPermissionModeSelect}
            onCancel={onPromptCancel}
            onSubmit={onPromptSubmit}
          />
        </div>
        {rightPanelOpen && (
          <aside className={`${styles.rightPanel} ${rightPanelHasTabs ? '' : styles.rightPanelNoTabs}`} aria-label="右侧工作区">
            <div
              aria-label="调整右侧栏宽度"
              aria-orientation="vertical"
              aria-valuemax={rightPanelMaxWidth()}
              aria-valuemin={Math.min(RIGHT_PANEL_MIN_WIDTH, rightPanelMaxWidth())}
              aria-valuenow={rightPanelWidth}
              className={styles.rightPanelResizer}
              role="separator"
              tabIndex={0}
              onPointerDown={startRightPanelResize}
            />
            {rightPanelHasTabs ? (
              <div className={styles.terminalHeader}>
                {rightPanelTabsOverflow ? (
                  <Button
                    aria-label="向左滚动标签"
                    className={styles.terminalScrollButton}
                    icon={<LeftOutlined />}
                    size="small"
                    type="text"
                    onClick={() => scrollRightPanelTabs(-1)}
                  />
                ) : null}
                <div
                  ref={rightPanelTabsRef}
                  className={`${styles.terminalTabs} ${rightPanelTabsOverflow ? styles.terminalTabsOverflow : ''}`}
                  role="tablist"
                  aria-label="右侧工作区标签"
                  onWheel={(event) => {
                    if (!rightPanelTabsOverflow || !rightPanelTabsRef.current) {
                      return;
                    }
                    event.preventDefault();
                    rightPanelTabsRef.current.scrollLeft += event.deltaX || event.deltaY;
                  }}
                >
                  {rightPanelTabs.map((tab) => (
                    <button
                      key={tab.id}
                      className={`${styles.terminalTab} ${activeRightPanelID === tab.id ? styles.terminalTabActive : ''}`}
                      type="button"
                      role="tab"
                      data-active={activeRightPanelID === tab.id ? 'true' : undefined}
                      aria-selected={activeRightPanelID === tab.id}
                      onClick={() => setActiveRightPanelID(tab.id)}
                    >
                      {tab.kind === 'review' ? <AuditOutlined /> : tab.kind === 'files' ? <FolderOpenOutlined /> : <ConsoleSqlOutlined />}
                      <span>{tab.title}</span>
                      <CloseOutlined
                        className={styles.terminalTabClose}
                        onClick={(event) => {
                          event.stopPropagation();
                          void closeRightPanelTab(tab.id);
                        }}
                      />
                    </button>
                  ))}
                </div>
                {rightPanelTabsOverflow ? (
                  <Button
                    aria-label="向右滚动标签"
                    className={styles.terminalScrollButton}
                    icon={<RightOutlined />}
                    size="small"
                    type="text"
                    onClick={() => scrollRightPanelTabs(1)}
                  />
                ) : null}
                <RightPanelAddMenu
                  className={styles.terminalAddButton}
                  canUseProjectSideTools={canUseProjectSideTools}
                  hasFilesTab={rightPanelTabs.some((tab) => tab.kind === 'files')}
                  hasReviewTab={rightPanelTabs.some((tab) => tab.kind === 'review')}
                  onOpenTool={openRightPanelTool}
                />
              </div>
            ) : null}
            {!rightPanelHasTabs ? (
              <div className={styles.rightPanelLauncher}>
                {canUseProjectSideTools ? (
                  <button className={styles.rightPanelLauncherItem} type="button" onClick={() => openRightPanelTool('review')}>
                    <AuditOutlined />
                    <span>审查</span>
                    <kbd>Ctrl+Shift+G</kbd>
                  </button>
                ) : null}
                <button className={styles.rightPanelLauncherItem} type="button" onClick={() => openRightPanelTool('terminal')}>
                  <ConsoleSqlOutlined />
                  <span>终端</span>
                  <kbd>Ctrl+`</kbd>
                </button>
                {canUseProjectSideTools ? (
                  <button className={styles.rightPanelLauncherItem} type="button" onClick={() => openRightPanelTool('files')}>
                    <FolderOpenOutlined />
                    <span>文件</span>
                    <kbd>Ctrl+P</kbd>
                  </button>
                ) : null}
              </div>
            ) : activeRightPanelTab?.kind === 'terminal' ? (
              <TerminalPane
                key={activeRightPanelTab.terminal?.id ?? activeRightPanelTab.id}
                terminal={activeRightPanelTab.terminal}
                title={activeRightPanelTab.title}
                onInput={onTerminalInput}
                onResize={onTerminalResize}
                onSubscribe={onTerminalSubscribe}
                onTerminalChange={updateTerminalTab}
              />
            ) : activeRightPanelTab?.kind === 'files' ? (
              <div className={styles.sideToolPane} role="tabpanel">
                <div className={styles.sideToolTitle}>文件</div>
                <div className={styles.sideToolEmpty}>File tree and file preview will be connected to the runtime workspace.</div>
              </div>
            ) : activeRightPanelTab?.kind === 'review' ? (
              <div className={styles.sideToolPane} role="tabpanel">
                <div className={styles.reviewStack}>
                  <RunProjectionPreview run={viewModel.runProjection} onResumeCheckpoint={onRunCheckpointResume} onExecuteTask={onRunTaskExecute} />
                  <ReactCallchainInspector callchain={viewModel.reactCallchain} />
                  <TurnDiagnosticsPanel
                    diagnostics={viewModel.turnDiagnostics}
                    interrupted={viewModel.interruptedTurn}
                    onInterruptedCopy={copyInterruptedSummary}
                    onInterruptedDone={markInterruptedDone}
                    onInterruptedFollowUp={startInterruptedFollowUp}
                  />
                  {!viewModel.runProjection && !viewModel.reactCallchain && !viewModel.turnDiagnostics && !viewModel.interruptedTurn ? (
                    <>
                      <div className={styles.sideToolTitle}>审查</div>
                      <div className={styles.sideToolEmpty}>Review findings, diffs, and approvals will appear here.</div>
                    </>
                  ) : null}
                </div>
              </div>
            ) : null}
          </aside>
        )}
    </section>
  );
}

function RightPanelAddMenu({
  className,
  canUseProjectSideTools,
  hasFilesTab,
  hasReviewTab,
  onOpenTool,
}: {
  className: string;
  canUseProjectSideTools: boolean;
  hasFilesTab: boolean;
  hasReviewTab: boolean;
  onOpenTool: (kind: RightPanelKind) => void;
}) {
  const items = [
    { key: 'terminal', icon: <ConsoleSqlOutlined />, label: '新建终端' },
    canUseProjectSideTools && !hasFilesTab ? { key: 'files', icon: <FolderOpenOutlined />, label: '打开文件' } : null,
    canUseProjectSideTools && !hasReviewTab ? { key: 'review', icon: <AuditOutlined />, label: '打开审查' } : null,
  ].filter((item): item is Exclude<typeof item, null> => Boolean(item));

  return (
    <Dropdown
      menu={{
        items,
        onClick: ({ key }) => {
          if (key === 'terminal' || key === 'files' || key === 'review') {
            onOpenTool(key);
          }
        },
      }}
      placement="bottomRight"
      trigger={['click']}
    >
      <Button aria-label="添加右侧工具" className={className} icon={<PlusOutlined />} size="small" type="text" />
    </Dropdown>
  );
}

function runtimePanelErrorMessage(error: unknown, fallback: string) {
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return fallback;
}

function MessageActions({
  content,
  createdAt,
  messageApi,
  role,
}: {
  content: string;
  createdAt?: number;
  messageApi: ReturnType<typeof antdMessage.useMessage>[0];
  role: string;
}) {
  const copyMessage = async () => {
    try {
      await copyText(content);
      void messageApi.success('已复制');
    } catch {
      void messageApi.error('复制失败');
    }
  };
  const sentAt = role === 'user' ? formatMessageTime(createdAt) : '';

  return (
    <div className={role === 'user' ? styles.userMessageActions : styles.assistantMessageActions}>
      <Tooltip title="复制">
        <Button aria-label="复制" icon={<CopyOutlined />} size="small" type="text" onClick={copyMessage} />
      </Tooltip>
      {sentAt && <span className={styles.messageTime}>{sentAt}</span>}
    </div>
  );
}

async function copyText(text: string) {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  if (typeof document === 'undefined') {
    throw new Error('clipboard is unavailable');
  }
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-1000px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    const copied = document.execCommand('copy');
    if (!copied) {
      throw new Error('copy command failed');
    }
  } finally {
    document.body.removeChild(textarea);
  }
}

function formatMessageTime(createdAt?: number) {
  if (!createdAt) {
    return '';
  }
  const milliseconds = createdAt < 1_000_000_000_000 ? createdAt * 1000 : createdAt;
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(milliseconds));
}
