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
  UnorderedListOutlined,
} from '@ant-design/icons';
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react';
import { Button, Dropdown, Empty, Input, Modal, Tooltip, message as antdMessage } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { HookExecutionViewModel, NewConversationDraftViewModel, TerminalEventViewModel, TerminalViewModel, WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Composer } from '../composer/Composer.tsx';
import { AgentTaskPanel } from '../agentTasks/AgentTaskPanel.tsx';
import { PermissionGate } from '../permissions/PermissionGate.tsx';
import { RunProjectionPreview } from '../diagnostics/RunProjectionPreview.tsx';
import { ReactCallchainInspector } from '../diagnostics/ReactCallchainInspector.tsx';
import { ContextDiagnosticsPanel } from '../diagnostics/ContextDiagnosticsPanel.tsx';
import { TurnDiagnosticsPanel } from '../diagnostics/TurnDiagnosticsPanel.tsx';
import { RecoveryCenter } from '../recovery/RecoveryCenter.tsx';
import { Timeline } from '../timeline/Timeline.tsx';
import { MarkdownMessage } from '../markdown/MarkdownMessage.tsx';
import { TodoPanel } from '../todos/TodoPanel.tsx';
import { TodoTaskBar } from '../todos/TodoTaskBar.tsx';
import { TerminalPane } from './TerminalPane.tsx';
import { disposeTerminalRuntime } from './terminalRuntime.ts';
import { useStickToBottom } from './useStickToBottom.ts';
import { nudgeCursorRecompute } from '../../lib/webviewCursor.ts';
import styles from './Workspace.module.css';

type RightPanelKind = 'review' | 'files' | 'terminal' | 'tasks';

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
  onPermissionDecide: (permissionID: string, action: 'allow' | 'allow_session' | 'deny', guidance?: string) => Promise<void>;
  onPermissionModeSelect: (mode: string) => Promise<void>;
  onPromptCancel: () => Promise<void>;
  onSessionRename: (sessionID: string, title: string) => Promise<void>;
  onPromptSubmit: (prompt: string) => Promise<void>;
  onNewConversationDraftChange: (target: NewConversationDraftViewModel) => void;
  onInterruptedResume: (turnID: string) => Promise<void>;
  onInterruptedDone: (turnID: string) => Promise<void>;
  onInterruptedDiscard: (turnID: string) => Promise<void>;
  onRecoverableErrorRetry: (errorID: string) => Promise<void>;
  onManualCompact?: (instructions?: string) => Promise<void>;
  onManualSnip?: () => Promise<void>;
  onRunCheckpointResume?: (runID: string, checkpointID: string) => Promise<void>;
  onRunTaskExecute?: (runID: string, taskID: string) => Promise<void>;
  onAgentTaskFollowUp?: (taskID: string, message: string) => Promise<void>;
  onAgentTaskCancel?: (taskID: string) => Promise<void>;
  onSessionTerminalsList: (sessionID: string) => Promise<TerminalViewModel[]>;
  onTerminalCreate: (request: { sessionId: string; cwd?: string; profileId?: string; columns?: number; rows?: number }) => Promise<TerminalViewModel>;
  onTerminalDelete: (terminalID: string) => Promise<void>;
  onTerminalInput: (terminalID: string, data: string) => Promise<TerminalViewModel>;
  onTerminalResize: (terminalID: string, columns: number, rows: number) => Promise<TerminalViewModel>;
  onTerminalSubscribe: (terminalID: string, onEvent: (event: TerminalEventViewModel) => void) => Promise<() => void> | (() => void);
  onHookExecutionLoad?: (executionID: string) => Promise<HookExecutionViewModel>;
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
  onInterruptedResume,
  onInterruptedDone,
  onInterruptedDiscard,
  onRecoverableErrorRetry,
  onManualCompact,
  onManualSnip,
  onRunCheckpointResume,
  onRunTaskExecute,
  onAgentTaskFollowUp,
  onAgentTaskCancel,
  onSessionTerminalsList,
  onTerminalCreate,
  onTerminalDelete,
  onTerminalInput,
  onTerminalResize,
  onTerminalSubscribe,
  onHookExecutionLoad,
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
  const [rightPanelMaxValue, setRightPanelMaxValue] = useState(RIGHT_PANEL_MAX_WIDTH);
  const [selectedAgentTaskID, setSelectedAgentTaskID] = useState('');
  const rightPanelTabsRef = useRef<HTMLDivElement | null>(null);
  const workspaceRef = useRef<HTMLElement | null>(null);
  const {
    containerRef: scrollContainerRef,
    showJumpToBottom,
    jumpToBottom,
    pinAndScrollToBottom,
    handleScroll,
    handleKeyDown: handleScrollKeyDown,
    handlePointerDown: handleScrollPointerDown,
  } = useStickToBottom();
  const hasProjectContext = Boolean(viewModel.currentProject.id || viewModel.currentProject.name || viewModel.currentProject.path);
  const canUseProjectSideTools = hasProjectContext;
  const hasTimeline = viewModel.timeline.length > 0;
  const hasConversation = viewModel.conversation.length > 0 || hasTimeline;
  const activeSession = viewModel.sessions.find((session) => session.active);
  const isDraftSurface = !activeSession && !hasConversation && !switchingSessionID;
  const composerDraftTarget =
    viewModel.newConversationDraft ?? (isDraftSurface ? defaultComposerDraftTarget(viewModel) : undefined);
  const activePendingPermission = viewModel.pendingPermissions.find((permission) => !activeSession?.id || permission.sessionId === activeSession.id) ?? viewModel.pendingPermissions[0];
  const isSessionSwitching = Boolean(switchingSessionID && activeSession?.id === switchingSessionID && !hasConversation && viewModel.timeline.length === 0);
  const sessionTitle = activeSession?.title || viewModel.currentProject.name || '新对话';
  const title =
    viewModel.mode === 'project' && hasProjectContext ? `我们应该在 ${viewModel.currentProject.name} 中构建什么？` : '我们该做什么？';
  const bubbleItems = viewModel.conversation.map((message) => ({
    key: message.id,
    role: message.role === 'user' ? 'user' : 'ai',
    content: <MarkdownMessage content={message.content} role={message.role} />,
    status: message.status,
    footer: shouldShowMessageActions(message.role, message.status) ? (
      <MessageActions content={message.content} createdAt={message.createdAt} messageApi={messageApi} role={message.role} />
    ) : undefined,
  }));
  const rightPanelHasTabs = rightPanelTabs.length > 0;
  const rightPanelOpen = rightPanelVisible;
  const activeRightPanelTab = rightPanelTabs.find((tab) => tab.id === activeRightPanelID) ?? rightPanelTabs[0];
  const activeSessionID = activeSession?.id ?? '';
  const hasTodos = Boolean(viewModel.todos?.items.length);
  const hasAgentTasks = Boolean(viewModel.agentTasks?.length);
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
    if (kind !== 'tasks' && !canUseProjectSideTools) {
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
      title: kind === 'review' ? '审查' : kind === 'tasks' ? '任务' : '文件',
    };
    setRightPanelTabs((current) => [...current, tab]);
    setActiveRightPanelID(tab.id);
  };
  const openAgentTask = (taskID: string) => {
    setSelectedAgentTaskID(taskID);
    openSingletonPanel('tasks');
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
      const terminal = await onTerminalCreate({
        sessionId: activeSession.id,
        profileId: viewModel.settings.terminalProfile,
        columns: 100,
        rows: 24,
      });
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
        disposeTerminalRuntime(closingTerminalID);
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
  const startRightPanelWidthDrag = (event: ReactPointerEvent<HTMLDivElement>) => {
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
    const stopRightPanelWidthDrag = () => {
      target.releasePointerCapture(pointerID);
      window.removeEventListener('pointermove', updateRightPanelWidth);
      window.removeEventListener('pointerup', stopRightPanelWidthDrag);
      window.removeEventListener('pointercancel', stopRightPanelWidthDrag);
      nudgeCursorRecompute();
    };

    window.addEventListener('pointermove', updateRightPanelWidth);
    window.addEventListener('pointerup', stopRightPanelWidthDrag);
    window.addEventListener('pointercancel', stopRightPanelWidthDrag);
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
      const frame = window.requestAnimationFrame(() => {
        if (!cancelled) {
          replaceTerminalTabs([]);
        }
      });
      return () => {
        cancelled = true;
        window.cancelAnimationFrame(frame);
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
    // Opening/switching a conversation always lands pinned at the bottom.
    pinAndScrollToBottom('auto');
  }, [activeSession?.id, switchingSessionID, pinAndScrollToBottom]);
  useEffect(() => {
    const activeTab = rightPanelTabsRef.current?.querySelector<HTMLElement>('[data-active="true"]');
    activeTab?.scrollIntoView({ block: 'nearest', inline: 'center' });
  }, [activeRightPanelID, rightPanelTabs.length]);
  useEffect(() => {
    if (canUseProjectSideTools) {
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      setRightPanelTabs((current) => current.filter((tab) => tab.kind === 'terminal'));
    });
    return () => window.cancelAnimationFrame(frame);
  }, [canUseProjectSideTools]);
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (rightPanelTabs.length === 0) {
        if (activeRightPanelID) {
          setActiveRightPanelID('');
        }
        return;
      }
      if (!rightPanelTabs.some((tab) => tab.id === activeRightPanelID)) {
        setActiveRightPanelID(rightPanelTabs[0].id);
      }
    });
    return () => window.cancelAnimationFrame(frame);
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
      const maxWidth = rightPanelMaxWidth();
      setRightPanelMaxValue(maxWidth);
      setRightPanelWidth((current) => clampRightPanelWidth(current));
    };
    window.addEventListener('resize', updateRightPanelBounds);
    return () => window.removeEventListener('resize', updateRightPanelBounds);
  }, [clampRightPanelWidth, rightPanelMaxWidth]);
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
    const frame = window.requestAnimationFrame(() => {
      setRightPanelMaxValue(rightPanelMaxWidth());
      setRightPanelWidth((current) => clampRightPanelWidth(current));
    });
    return () => window.cancelAnimationFrame(frame);
  }, [clampRightPanelWidth, rightPanelDocked, rightPanelMaxWidth, rightPanelOpen, sidebarCollapsed]);
  useEffect(() => {
    onMinimumWorkspaceWidthChange?.(rightPanelOpen ? RIGHT_PANEL_DOCKED_MIN_WORKSPACE_WIDTH : 0);
    return () => onMinimumWorkspaceWidthChange?.(0);
  }, [onMinimumWorkspaceWidthChange, rightPanelOpen]);
  const handlePromptSubmit = useCallback(
    async (prompt: string) => {
      const compactMatch = prompt.trim().match(/^\/compact(?:\s+([\s\S]*))?$/);
      if (compactMatch) {
        try {
          await onManualCompact?.(compactMatch[1]?.trim());
        } catch (error) {
          void messageApi.error(runtimePanelErrorMessage(error, '当前没有可压缩的对话轮次'));
        }
        return;
      }
      pinAndScrollToBottom('auto');
      await onPromptSubmit(prompt);
    },
    [onManualCompact, onPromptSubmit, pinAndScrollToBottom],
  );
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
          onKeyDown={hasConversation || isSessionSwitching ? handleScrollKeyDown : undefined}
          onPointerDown={hasConversation || isSessionSwitching ? handleScrollPointerDown : undefined}
          onScroll={hasConversation || isSessionSwitching ? handleScroll : undefined}
        >
          {hasTimeline ? (
            <div className={styles.timelineLayout}>
              <div className={styles.timelineColumn}>
                <Timeline items={viewModel.timeline} hookExecutions={viewModel.hookExecutions} onAgentTaskOpen={openAgentTask} onHookExecutionLoad={onHookExecutionLoad} />
              </div>
            </div>
          ) : viewModel.conversation.length > 0 ? (
            <Bubble.List
              autoScroll={false}
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
          <TodoTaskBar todos={viewModel.todos} />
          {hasConversation && showJumpToBottom && (
            <button
              aria-label="跳到底部"
              className={styles.jumpToBottomButton}
              type="button"
              onClick={jumpToBottom}
            >
              <ArrowDownOutlined />
            </button>
          )}
          {activePendingPermission ? (
            <div className={styles.permissionDock} data-testid="permission-dock">
              <PermissionGate permission={activePendingPermission} onDecide={onPermissionDecide} />
            </div>
          ) : (
            <Composer
              composer={viewModel.composer}
              project={viewModel.currentProject}
              projects={viewModel.projects}
              newConversationDraft={composerDraftTarget}
              showProjectContext={Boolean(composerDraftTarget)}
              onNewConversationDraftChange={onNewConversationDraftChange}
              onModelSelect={onModelSelect}
              onPermissionModeSelect={onPermissionModeSelect}
              onCancel={onPromptCancel}
              onSubmit={handlePromptSubmit}
              onManualCompact={onManualCompact}
            />
          )}
        </div>
        {rightPanelOpen && (
          <aside className={`${styles.rightPanel} ${rightPanelHasTabs ? '' : styles.rightPanelNoTabs}`} aria-label="右侧工作区">
            <div
              aria-label="调整右侧栏宽度"
              aria-orientation="vertical"
              aria-valuemax={rightPanelMaxValue}
              aria-valuemin={Math.min(RIGHT_PANEL_MIN_WIDTH, rightPanelMaxValue)}
              aria-valuenow={rightPanelWidth}
              className={styles.rightPanelSplitter}
              role="separator"
              tabIndex={0}
              onPointerDown={startRightPanelWidthDrag}
              onPointerLeave={nudgeCursorRecompute}
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
                      {tab.kind === 'review' ? <AuditOutlined /> : tab.kind === 'files' ? <FolderOpenOutlined /> : tab.kind === 'tasks' ? <UnorderedListOutlined /> : <ConsoleSqlOutlined />}
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
                  hasTasksTab={rightPanelTabs.some((tab) => tab.kind === 'tasks')}
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
                {(hasTodos || hasAgentTasks) ? (
                  <button className={styles.rightPanelLauncherItem} type="button" onClick={() => openRightPanelTool('tasks')}>
                    <UnorderedListOutlined />
                    <span>任务</span>
                    <kbd>Tasks</kbd>
                  </button>
                ) : null}
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
                  <AgentTaskPanel tasks={viewModel.agentTasks} onCancelTask={onAgentTaskCancel} onFollowUp={onAgentTaskFollowUp} />
                  <RecoveryCenter
                    recovery={viewModel.recovery}
                    onResumeTurn={onInterruptedResume}
                    onMarkDone={onInterruptedDone}
                    onDiscardTurn={onInterruptedDiscard}
                    onRetryError={onRecoverableErrorRetry}
                    onResumeCheckpoint={(runID, checkpointID) => onRunCheckpointResume?.(runID, checkpointID) ?? Promise.resolve()}
                  />
                  <RunProjectionPreview run={viewModel.runProjection} onResumeCheckpoint={onRunCheckpointResume} onExecuteTask={onRunTaskExecute} />
                  <ReactCallchainInspector callchain={viewModel.reactCallchain} onHookExecutionLoad={onHookExecutionLoad} />
                  <ContextDiagnosticsPanel diagnostics={viewModel.contextDiagnostics} contextUsage={viewModel.composer.contextUsage} onManualCompact={onManualCompact} onManualSnip={onManualSnip} />
                  <TurnDiagnosticsPanel
                    diagnostics={viewModel.turnDiagnostics}
                    hookExecutions={viewModel.hookExecutions}
                    onHookExecutionLoad={onHookExecutionLoad}
                  />
                  {!viewModel.agentTasks?.length &&
                  !viewModel.runProjection &&
                  !viewModel.reactCallchain &&
                  !viewModel.contextDiagnostics &&
                  !viewModel.turnDiagnostics &&
                  !viewModel.recovery?.interruptedTurns.length &&
                  !viewModel.recovery?.recoverableErrors.length &&
                  !viewModel.hookExecutions ? (
                    <>
                      <div className={styles.sideToolTitle}>审查</div>
                      <div className={styles.sideToolEmpty}>Review findings, diffs, and approvals will appear here.</div>
                    </>
                  ) : null}
                </div>
              </div>
            ) : activeRightPanelTab?.kind === 'tasks' ? (
              <div className={styles.sideToolPane} role="tabpanel">
                <div className={styles.taskStack}>
                  <TodoPanel todos={viewModel.todos} />
                  <AgentTaskPanel
                    agentRoles={viewModel.agentRoles}
                    selectedTaskID={selectedAgentTaskID}
                    tasks={viewModel.agentTasks}
                    onCancelTask={onAgentTaskCancel}
                    onFollowUp={onAgentTaskFollowUp}
                    onSelectTask={setSelectedAgentTaskID}
                  />
                  {!hasTodos && !hasAgentTasks ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No tasks" /> : null}
                </div>
              </div>
            ) : null}
          </aside>
        )}
    </section>
  );
}

function defaultComposerDraftTarget(viewModel: WorkbenchViewModel): NewConversationDraftViewModel {
  if (viewModel.currentProject.id) {
    return { active: true, scope: 'project', projectId: viewModel.currentProject.id };
  }
  return { active: true, scope: 'standalone' };
}

function RightPanelAddMenu({
  className,
  canUseProjectSideTools,
  hasFilesTab,
  hasReviewTab,
  hasTasksTab,
  onOpenTool,
}: {
  className: string;
  canUseProjectSideTools: boolean;
  hasFilesTab: boolean;
  hasReviewTab: boolean;
  hasTasksTab: boolean;
  onOpenTool: (kind: RightPanelKind) => void;
}) {
  const items = [
    { key: 'terminal', icon: <ConsoleSqlOutlined />, label: '新建终端' },
    !hasTasksTab ? { key: 'tasks', icon: <UnorderedListOutlined />, label: '打开任务' } : null,
    canUseProjectSideTools && !hasFilesTab ? { key: 'files', icon: <FolderOpenOutlined />, label: '打开文件' } : null,
    canUseProjectSideTools && !hasReviewTab ? { key: 'review', icon: <AuditOutlined />, label: '打开审查' } : null,
  ].filter((item): item is Exclude<typeof item, null> => Boolean(item));

  return (
    <Dropdown
      menu={{
        items,
        onClick: ({ key }) => {
          if (key === 'terminal' || key === 'files' || key === 'review' || key === 'tasks') {
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

function shouldShowMessageActions(role: string, status?: string) {
  if (role === 'user') {
    return true;
  }
  return status === 'success' || status === 'error';
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
