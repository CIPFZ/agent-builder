import { useCallback, useEffect, useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react';
import type {
  CreateProjectRequestViewModel,
  OpenProjectRequestViewModel,
  ProjectActionRequestViewModel,
  RenameProjectRequestViewModel,
  WorkbenchAdapter,
  WorkbenchMode,
  WorkbenchViewModel,
} from '../../runtime/workbenchTypes.ts';
import { runtimeEventRefreshDelay } from '../../runtime/runtimeEventRefresh.ts';
import { PluginCenter } from '../../features/plugins/PluginCenter.tsx';
import { Sidebar } from '../../features/sidebar/Sidebar.tsx';
import { SettingsPanel } from '../../features/settings/SettingsPanel.tsx';
import { Workspace } from '../../features/workspace/Workspace.tsx';
import styles from './WorkbenchShell.module.css';

interface WorkbenchShellProps {
  adapter: WorkbenchAdapter;
  viewModel: WorkbenchViewModel;
}

const SIDEBAR_DEFAULT_WIDTH = 280;
const SIDEBAR_MIN_WIDTH = 260;
const SIDEBAR_MAX_WIDTH = 380;
const WORKSPACE_MIN_VISIBLE_WIDTH = 360;
const SHELL_RESIZE_GUTTER_WIDTH = 2;
const SHELL_MIN_WIDTH = 1080;

function getLayoutWidth() {
  if (typeof window === 'undefined') {
    return SHELL_MIN_WIDTH;
  }
  return Math.max(window.innerWidth, SHELL_MIN_WIDTH);
}

function getSidebarMaxWidth(workspaceMinVisibleWidth = WORKSPACE_MIN_VISIBLE_WIDTH) {
  return Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, getLayoutWidth() - workspaceMinVisibleWidth - SHELL_RESIZE_GUTTER_WIDTH));
}

function clampSidebarWidth(width: number, maxWidth = getSidebarMaxWidth()) {
  return Math.min(maxWidth, Math.max(SIDEBAR_MIN_WIDTH, width));
}

export function WorkbenchShell({ adapter, viewModel: initialViewModel }: WorkbenchShellProps) {
  const [viewModel, setViewModel] = useState<WorkbenchViewModel>(initialViewModel);
  const [mode, setMode] = useState<WorkbenchMode>(initialViewModel.mode);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(() => clampSidebarWidth(SIDEBAR_DEFAULT_WIDTH));
  const [workspaceMinVisibleWidth, setWorkspaceMinVisibleWidth] = useState(WORKSPACE_MIN_VISIBLE_WIDTH);
  const [sidebarMaxWidth, setSidebarMaxWidth] = useState(() => getSidebarMaxWidth());
  const [viewportWidth, setViewportWidth] = useState(() => (typeof window === 'undefined' ? 0 : window.innerWidth));
  const viewModelRef = useRef(viewModel);
  const modeRef = useRef(mode);
  const hasBusySession = viewModel.sessions.some((session) => session.busy);
  const sidebarForceCollapsed =
    !sidebarCollapsed &&
    workspaceMinVisibleWidth > WORKSPACE_MIN_VISIBLE_WIDTH &&
    viewportWidth > 0 &&
    getLayoutWidth() < SIDEBAR_MIN_WIDTH + SHELL_RESIZE_GUTTER_WIDTH + workspaceMinVisibleWidth;
  const effectiveSidebarCollapsed = sidebarCollapsed || sidebarForceCollapsed;

  useEffect(() => {
    viewModelRef.current = viewModel;
  }, [viewModel]);

  useEffect(() => {
    const updateViewportWidth = () => setViewportWidth(window.innerWidth);
    updateViewportWidth();
    window.addEventListener('resize', updateViewportWidth);
    return () => window.removeEventListener('resize', updateViewportWidth);
  }, []);

  useEffect(() => {
    const updateSidebarBounds = () => {
      const nextMaxWidth = getSidebarMaxWidth(workspaceMinVisibleWidth);
      setSidebarMaxWidth(nextMaxWidth);
      setSidebarWidth((current) => clampSidebarWidth(current, nextMaxWidth));
    };

    updateSidebarBounds();
    window.addEventListener('resize', updateSidebarBounds);
    return () => window.removeEventListener('resize', updateSidebarBounds);
  }, [workspaceMinVisibleWidth]);

  useEffect(() => {
    modeRef.current = mode;
  }, [mode]);

  useEffect(() => {
    if (!adapter.subscribeRuntimeEvents) {
      return undefined;
    }
    if (!viewModel.composer.busy && !hasBusySession) {
      return undefined;
    }

    let cancelled = false;
    let refreshTimer: number | undefined;
    let refreshing = false;
    let queued = false;
    let unsubscribe: (() => void) | undefined;

    const refreshFromRuntimeEvent = async () => {
      if (cancelled) {
        return;
      }
      if (refreshing) {
        queued = true;
        return;
      }
      refreshing = true;
      queued = false;
      try {
        const nextViewModel = await adapter.refresh({ ...viewModelRef.current, mode: modeRef.current });
        if (!cancelled) {
          setMode(nextViewModel.mode);
          setViewModel(nextViewModel);
        }
      } catch {
        // Polling remains active while busy; event refresh is an opportunistic fast path.
      } finally {
        refreshing = false;
        if (queued && !cancelled) {
          scheduleRuntimeRefresh(120);
        }
      }
    };

    const scheduleRuntimeRefresh = (delay = 180) => {
      if (cancelled) {
        return;
      }
      if (refreshTimer) {
        window.clearTimeout(refreshTimer);
      }
      refreshTimer = window.setTimeout(refreshFromRuntimeEvent, delay);
    };

    void Promise.resolve(adapter.subscribeRuntimeEvents((event) => scheduleRuntimeRefresh(runtimeEventRefreshDelay(event)))).then((cleanup) => {
      if (cancelled) {
        cleanup();
        return;
      }
      unsubscribe = cleanup;
    });

    return () => {
      cancelled = true;
      if (refreshTimer) {
        window.clearTimeout(refreshTimer);
      }
      unsubscribe?.();
    };
  }, [adapter, viewModel.composer.busy, hasBusySession]);

  useEffect(() => {
    if (!viewModel.composer.busy && !hasBusySession) {
      return undefined;
    }

    let cancelled = false;
    let timer: number | undefined;

    const refreshUntilIdle = async () => {
      try {
        const nextViewModel = await adapter.refresh({ ...viewModelRef.current, mode });
        if (cancelled) {
          return;
        }
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
        if (nextViewModel.composer.busy || nextViewModel.sessions.some((session) => session.busy)) {
          timer = window.setTimeout(refreshUntilIdle, 1200);
        }
      } catch {
        if (!cancelled) {
          timer = window.setTimeout(refreshUntilIdle, 2000);
        }
      }
    };

    timer = window.setTimeout(refreshUntilIdle, 800);

    return () => {
      cancelled = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [adapter, mode, viewModel.composer.busy, hasBusySession]);

  const changeMode = (nextMode: WorkbenchMode) => {
    setMode(nextMode);
    setViewModel((current) => ({ ...current, mode: nextMode }));
    if (nextMode === 'settings' || nextMode === 'plugins') {
      void adapter.refresh({ ...viewModel, mode: nextMode }).then((nextViewModel) => {
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
      });
    }
  };

  const createSession = () => {
    const draftViewModel: WorkbenchViewModel = {
      ...viewModel,
      mode: 'new-chat',
      sessions: viewModel.sessions.map((session) => ({ ...session, active: false })),
      conversation: [],
      timeline: [],
      turnDiagnostics: undefined,
      interruptedTurn: undefined,
      runProjection: undefined,
      pendingPermissions: [],
      composer: { ...viewModel.composer, busy: false, activeTurnId: undefined },
    };
    setMode('new-chat');
    setViewModel(draftViewModel);
    void adapter.createSession(draftViewModel).then((nextViewModel) => {
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    });
  };

  const openProject = async (request: OpenProjectRequestViewModel) => {
    const nextViewModel = await adapter.openProject({ ...viewModel, mode }, request);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const createProject = async (request: CreateProjectRequestViewModel) => {
    const nextViewModel = await adapter.createProject({ ...viewModel, mode }, request);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const renameProject = async (request: RenameProjectRequestViewModel) => {
    const nextViewModel = await adapter.renameProject({ ...viewModel, mode }, request);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const openProjectInExplorer = async (request: ProjectActionRequestViewModel) => {
    await adapter.openProjectInExplorer(request);
  };

  const removeProject = async (request: ProjectActionRequestViewModel) => {
    const nextViewModel = await adapter.removeProject({ ...viewModel, mode }, request);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const selectProjectDirectory = () => adapter.selectProjectDirectory();

  const selectSession = (sessionID: string) => {
    void adapter.selectSession({ ...viewModel, mode }, sessionID).then((nextViewModel) => {
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    });
  };

  const renameSession = async (sessionID: string, title: string) => {
    const nextViewModel = await adapter.renameSession({ ...viewModel, mode }, sessionID, title);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const deleteSession = (sessionID: string) => {
    void adapter.deleteSession({ ...viewModel, mode }, sessionID).then((nextViewModel) => {
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    });
  };

  const sendPrompt = async (prompt: string) => {
    const createdAt = Date.now();
    const userID = `local-${createdAt}`;
    const loadingID = `loading-${createdAt}`;
    const optimisticViewModel: WorkbenchViewModel = {
      ...viewModel,
      mode,
      composer: { ...viewModel.composer, busy: true },
      conversation: [
        ...viewModel.conversation,
        {
          id: userID,
          role: 'user',
          content: prompt,
          createdAt,
          status: 'success',
        },
        {
          id: loadingID,
          role: 'assistant',
          content: '正在生成回复...',
          status: 'loading',
        },
      ],
      timeline: [
        ...viewModel.timeline,
        {
          id: userID,
          kind: 'message',
          role: 'user',
          content: prompt,
          createdAt,
          status: 'success',
        },
        {
          id: loadingID,
          kind: 'message',
          role: 'assistant',
          content: '正在生成回复...',
          status: 'loading',
        },
      ],
    };
    setViewModel(optimisticViewModel);
    try {
      const nextViewModel = await adapter.sendPrompt(optimisticViewModel, prompt);
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    } catch (error) {
      const failedViewModel: WorkbenchViewModel = {
        ...optimisticViewModel,
        composer: { ...optimisticViewModel.composer, busy: false },
        conversation: optimisticViewModel.conversation.map((message) =>
          message.id === loadingID ? { ...message, content: sendPromptErrorMessage(error), status: 'error', error: sendPromptErrorMessage(error) } : message,
        ),
        timeline: optimisticViewModel.timeline.map((item) =>
          item.id === loadingID ? { ...item, content: sendPromptErrorMessage(error), status: 'error', error: sendPromptErrorMessage(error) } : item,
        ),
      };
      setViewModel(failedViewModel);
    }
  };

  const cancelTurn = async () => {
    const nextViewModel = await adapter.cancelTurn({ ...viewModel, mode }, viewModel.composer.activeTurnId);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const markInterruptedDone = async (turnID: string) => {
    const nextViewModel = await adapter.markInterruptedDone({ ...viewModel, mode }, turnID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const resumeRunCheckpoint = async (runID: string, checkpointID: string) => {
    const nextViewModel = await adapter.resumeRunCheckpoint({ ...viewModelRef.current, mode: modeRef.current }, runID, checkpointID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const executeRunTask = async (runID: string, taskID: string) => {
    const nextViewModel = await adapter.executeRunTask({ ...viewModelRef.current, mode: modeRef.current }, runID, taskID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const listSessionTerminals = useCallback((sessionID: string) => adapter.listSessionTerminals(sessionID), [adapter]);

  const createTerminal = useCallback(
    (request: { sessionId: string; cwd?: string; columns?: number; rows?: number }) => adapter.createTerminal(request),
    [adapter],
  );

  const writeTerminalInput = useCallback((terminalID: string, data: string) => adapter.writeTerminalInput(terminalID, data), [adapter]);

  const resizeTerminal = useCallback((terminalID: string, columns: number, rows: number) => adapter.resizeTerminal(terminalID, columns, rows), [adapter]);

  const subscribeTerminalEvents = useCallback(
    (terminalID: string, onEvent: Parameters<typeof adapter.subscribeTerminalEvents>[1]) => adapter.subscribeTerminalEvents(terminalID, onEvent),
    [adapter],
  );

  const deleteTerminal = useCallback((terminalID: string) => adapter.deleteTerminal(terminalID), [adapter]);

  const selectModel = async (configuredProviderID: string, model: string) => {
    const nextViewModel = await adapter.selectModel({ ...viewModel, mode }, configuredProviderID, model);
    setViewModel(nextViewModel);
  };

  const selectPermissionMode = async (permissionMode: string) => {
    const currentViewModel = { ...viewModel, mode };
    const optimisticViewModel = applyPermissionMode(currentViewModel, permissionMode);
    setViewModel(optimisticViewModel);
    const nextViewModel = await adapter.selectPermissionMode(optimisticViewModel, permissionMode);
    setViewModel(nextViewModel);
  };

  const decidePermission = async (permissionID: string, action: 'allow' | 'allow_for_session' | 'deny') => {
    const nextViewModel = await adapter.decidePermission({ ...viewModel, mode }, permissionID, action);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const saveConfiguredProvider = async (provider: Parameters<WorkbenchAdapter['saveConfiguredProvider']>[1]) => {
    const nextViewModel = await adapter.saveConfiguredProvider({ ...viewModel, mode }, provider);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
    return nextViewModel.settings.configuredProviders;
  };

  const refreshSettings = async () => {
    const nextViewModel = await adapter.refresh({ ...viewModel, mode: 'settings' });
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const refreshCurrentSettings = async () => {
    const nextViewModel = await adapter.refresh({ ...viewModel, mode });
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const deleteConfiguredProvider = async (providerID: string) => {
    const nextViewModel = await adapter.deleteConfiguredProvider({ ...viewModel, mode }, providerID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
    return nextViewModel.settings.configuredProviders;
  };

  const discoverConfiguredProviderModels = (providerID: string) => adapter.discoverConfiguredProviderModels(providerID);

  const discoverProviderDraftModels = (request: Parameters<WorkbenchAdapter['discoverProviderDraftModels']>[0]) =>
    adapter.discoverProviderDraftModels(request);

  const testProviderDraft = (request: Parameters<WorkbenchAdapter['testProviderDraft']>[0]) => adapter.testProviderDraft(request);

  const testConfiguredProvider = (providerID: string) => adapter.testConfiguredProvider(providerID);

  const measureProviderDraftLatency = (request: Parameters<WorkbenchAdapter['measureProviderDraftLatency']>[0]) =>
    adapter.measureProviderDraftLatency(request);

  const measureConfiguredProviderLatency = (providerID: string) => adapter.measureConfiguredProviderLatency(providerID);

  const refreshSkills = async () => {
    const nextViewModel = await adapter.refreshSkills({ ...viewModel, mode });
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const setSkillEnabled = async (name: string, enabled: boolean) => {
    const nextViewModel = await adapter.setSkillEnabled({ ...viewModel, mode }, name, enabled);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const refreshMCPServer = async (name: string) => {
    const nextViewModel = await adapter.refreshMCPServer({ ...viewModel, mode }, name);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const saveMCPServer = async (server: Parameters<WorkbenchAdapter['saveMCPServer']>[1]) => {
    const nextViewModel = await adapter.saveMCPServer({ ...viewModel, mode }, server);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const setMCPServerEnabled = async (name: string, enabled: boolean) => {
    const nextViewModel = await adapter.setMCPServerEnabled({ ...viewModel, mode }, name, enabled);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const setMCPToolEnabled = async (server: string, tool: string, enabled: boolean) => {
    const nextViewModel = await adapter.setMCPToolEnabled({ ...viewModel, mode }, server, tool, enabled);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const loadMCPServerDetails = async (name: string) => {
    const nextViewModel = await adapter.loadMCPServerDetails({ ...viewModel, mode }, name);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const workbenchViewModel = {
    ...viewModel,
    mode,
  };
  const handleMinimumWorkspaceWidthChange = useCallback((width: number) => {
    setWorkspaceMinVisibleWidth(Math.max(WORKSPACE_MIN_VISIBLE_WIDTH, Math.ceil(width)));
  }, []);

  const startSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const pointerId = event.pointerId;
    const startX = event.clientX;
    const startWidth = sidebarWidth;
    const target = event.currentTarget;

    target.setPointerCapture(pointerId);

    const updateSidebarWidth = (moveEvent: PointerEvent) => {
      const nextWidth = clampSidebarWidth(startWidth + moveEvent.clientX - startX, sidebarMaxWidth);
      setSidebarWidth(nextWidth);
    };

    const stopSidebarResize = () => {
      target.releasePointerCapture(pointerId);
      window.removeEventListener('pointermove', updateSidebarWidth);
      window.removeEventListener('pointerup', stopSidebarResize);
      window.removeEventListener('pointercancel', stopSidebarResize);
    };

    window.addEventListener('pointermove', updateSidebarWidth);
    window.addEventListener('pointerup', stopSidebarResize);
    window.addEventListener('pointercancel', stopSidebarResize);
  };

  if (mode === 'settings') {
    return (
      <main className={`${styles.shell} ${styles.settingsShell}`} data-testid="workbench-shell">
        <SettingsPanel
          settings={workbenchViewModel.settings}
          onModeChange={changeMode}
          onProviderDelete={deleteConfiguredProvider}
          onProviderDiscoverDraftModels={discoverProviderDraftModels}
          onProviderDiscoverModels={discoverConfiguredProviderModels}
          onProviderDraftLatency={measureProviderDraftLatency}
          onProviderDraftTest={testProviderDraft}
          onProviderLatency={measureConfiguredProviderLatency}
          onProviderSave={saveConfiguredProvider}
          onProviderTest={testConfiguredProvider}
          selectedModel={workbenchViewModel.composer.selectedModel}
          onModelSelect={selectModel}
          onMCPServerDetailsLoad={loadMCPServerDetails}
          onMCPServerRefresh={refreshMCPServer}
          onMCPServerSave={saveMCPServer}
          onMCPServerToggle={setMCPServerEnabled}
          onMCPToolToggle={setMCPToolEnabled}
          onSettingsRefresh={refreshSettings}
          onSkillRefresh={refreshSkills}
          onSkillToggle={setSkillEnabled}
        />
      </main>
    );
  }

  return (
    <main
      className={`${styles.shell} ${effectiveSidebarCollapsed ? styles.sidebarCollapsed : ''}`}
      data-testid="workbench-shell"
      style={{ '--sidebar-width': `${sidebarWidth}px` } as CSSProperties}
    >
      <Sidebar
        collapsed={effectiveSidebarCollapsed}
        viewModel={workbenchViewModel}
        onCollapsedChange={setSidebarCollapsed}
        onModeChange={changeMode}
        onProjectCreate={createProject}
        onProjectOpen={openProject}
        onProjectRename={renameProject}
        onProjectOpenInExplorer={openProjectInExplorer}
        onProjectRemove={removeProject}
        onProjectDirectorySelect={selectProjectDirectory}
        onSessionCreate={createSession}
        onSessionDelete={deleteSession}
        onSessionSelect={selectSession}
      />
      {!effectiveSidebarCollapsed && (
        <div
          aria-label="调整侧栏宽度"
          className={styles.sidebarResizer}
          role="separator"
          aria-orientation="vertical"
          aria-valuemin={SIDEBAR_MIN_WIDTH}
          aria-valuemax={sidebarMaxWidth}
          aria-valuenow={sidebarWidth}
          tabIndex={0}
          onPointerDown={startSidebarResize}
        />
      )}
      {mode === 'plugins' ? (
        <PluginCenter
          sidebarCollapsed={effectiveSidebarCollapsed}
          settings={workbenchViewModel.settings}
          onSettingsRefresh={refreshCurrentSettings}
          onSkillToggle={setSkillEnabled}
        />
      ) : (
        <Workspace
          sidebarCollapsed={effectiveSidebarCollapsed}
          viewModel={workbenchViewModel}
          onMinimumWorkspaceWidthChange={handleMinimumWorkspaceWidthChange}
          onModelSelect={selectModel}
          onPermissionDecide={decidePermission}
          onPermissionModeSelect={selectPermissionMode}
          onPromptCancel={cancelTurn}
          onSessionRename={renameSession}
          onPromptSubmit={sendPrompt}
          onInterruptedDone={markInterruptedDone}
          onRunCheckpointResume={resumeRunCheckpoint}
          onRunTaskExecute={executeRunTask}
          onSessionTerminalsList={listSessionTerminals}
          onTerminalCreate={createTerminal}
          onTerminalDelete={deleteTerminal}
          onTerminalInput={writeTerminalInput}
          onTerminalResize={resizeTerminal}
          onTerminalSubscribe={subscribeTerminalEvents}
        />
      )}
    </main>
  );
}

function applyPermissionMode(current: WorkbenchViewModel, mode: string): WorkbenchViewModel {
  const option =
    current.composer.permissionOptions.find((item) => item.mode === mode || item.value === mode) ??
    current.settings.permissionOptions.find((item) => item.mode === mode || item.value === mode);
  if (!option) {
    return current;
  }
  const permissionMode = {
    mode: option.mode,
    label: option.label,
    description: option.description,
  };
  return {
    ...current,
    composer: {
      ...current.composer,
      permissionLabel: option.label,
      permissionMode,
    },
    settings: {
      ...current.settings,
      permissionMode,
    },
  };
}

function sendPromptErrorMessage(error: unknown) {
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return '发送失败';
}
