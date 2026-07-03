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
  NewConversationDraftViewModel,
} from '../../runtime/workbenchTypes.ts';
import { addOptimisticUserSubmit, applyOutputEvents } from '../../runtime/outputReducer.ts';
import { createOutputStore } from '../../runtime/outputStore.ts';
import { selectConversationMessages, selectConversationTimeline, selectPendingPermissions } from '../../runtime/outputSelectors.ts';
import { runtimeEventCoveredByOutputStream, runtimeEventRefreshDelay } from '../../runtime/runtimeEventRefresh.ts';
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
  const [switchingSessionID, setSwitchingSessionID] = useState('');
  const viewModelRef = useRef(viewModel);
  const modeRef = useRef(mode);
  const sessionMutationSeqRef = useRef(0);
  const newConversationDraftSeqRef = useRef(0);
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

    const streamHandlesEvent = Boolean(adapter.subscribeSessionOutput);
    void Promise.resolve(adapter.subscribeRuntimeEvents((event) => {
      // When the per-session output stream is available it already
      // materializes the message / tool / permission diffs into the
      // outputStore; the full-workbench refresh is redundant for these
      // events and would just double the render cost.
      if (streamHandlesEvent && runtimeEventCoveredByOutputStream(event)) {
        return;
      }
      scheduleRuntimeRefresh(runtimeEventRefreshDelay(event));
    })).then((cleanup) => {
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
  }, [adapter]);

  // Session output stream: when the adapter exposes subscribeSessionOutput
  // the live text/tool/permission deltas flow directly into outputStore
  // without a full refresh cycle. Snapshot_required signals fall back to
  // the refresh path via requestFullRefresh.
  const activeSessionID = viewModel.sessions.find((session) => session.active)?.id;
  useEffect(() => {
    if (!adapter.subscribeSessionOutput || !activeSessionID) {
      return undefined;
    }
    let cancelled = false;
    let closer: (() => void) | undefined;
    const requestFullRefresh = () => {
      if (cancelled) return;
      void adapter.refresh({ ...viewModelRef.current, mode: modeRef.current }).then((nextViewModel) => {
        if (cancelled) return;
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
      }).catch(() => undefined);
    };
    const onEvents = (events: import('../../runtime/outputTypes.ts').RuntimeOutputEvent[]) => {
      if (cancelled || events.length === 0) return;
      setViewModel((current) => {
        const store = current.outputStore && current.outputStore.sessionId === activeSessionID
          ? current.outputStore
          : createOutputStore(activeSessionID);
        const nextStore = applyOutputEvents(store, events);
        return {
          ...current,
          outputStore: nextStore,
          conversation: selectConversationMessages(nextStore),
          timeline: selectConversationTimeline(nextStore),
          pendingPermissions: selectPendingPermissions(nextStore),
        };
      });
    };
    void Promise.resolve(
      adapter.subscribeSessionOutput(activeSessionID, {
        onEvents,
        onSnapshotRequired: requestFullRefresh,
      }, viewModel.outputStore?.cursor),
    ).then((cleanup) => {
      if (cancelled) {
        cleanup?.();
        return;
      }
      closer = cleanup;
    }).catch(() => undefined);
    return () => {
      cancelled = true;
      if (closer) {
        try { closer(); } catch { /* ignore */ }
      }
    };
    // We intentionally do not depend on viewModel.outputStore.cursor —
    // subscribing once per (session, adapter) is enough; the stream itself
    // carries its own cursor state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [adapter, activeSessionID]);

  useEffect(() => {
    if (!viewModel.composer.busy && !hasBusySession) {
      return undefined;
    }

    let cancelled = false;
    let timer: number | undefined;

    // When the streaming channel is available, the shell no longer needs
    // aggressive full-workbench refresh loops to render the assistant's
    // live output. We slow the polling to 3s so hooks/tasks/context still
    // pick up eventual state changes.
    const busyIntervalMs = adapter.subscribeSessionOutput ? 3000 : 1200;
    const backoffMs = adapter.subscribeSessionOutput ? 4000 : 2000;

    const refreshUntilIdle = async () => {
      try {
        const nextViewModel = await adapter.refresh({ ...viewModelRef.current, mode });
        if (cancelled) {
          return;
        }
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
        if (nextViewModel.composer.busy || nextViewModel.sessions.some((session) => session.busy)) {
          timer = window.setTimeout(refreshUntilIdle, busyIntervalMs);
        }
      } catch {
        if (!cancelled) {
          timer = window.setTimeout(refreshUntilIdle, backoffMs);
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

  const loadHookExecution = useCallback(
    (executionID: string) => {
      if (!adapter.loadHookExecution) {
        return Promise.reject(new Error('runtime hook execution API is unavailable'));
      }
      return adapter.loadHookExecution(executionID);
    },
    [adapter],
  );

  const createSession = (target?: NewConversationDraftViewModel) => {
    const draftSeq = ++newConversationDraftSeqRef.current;
    const currentViewModel = viewModelRef.current;
    const currentMode = modeRef.current;
    const draftTarget = target ?? defaultDraftTarget(currentViewModel);
    const draftViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: 'new-chat',
      newConversationDraft: draftTarget,
      sessions: currentViewModel.sessions.map((session) => ({ ...session, active: false })),
      conversation: [],
      timeline: [],
      turnDiagnostics: undefined,
      runProjection: undefined,
      reactCallchain: undefined,
      contextDiagnostics: undefined,
      pendingPermissions: [],
      composer: { ...currentViewModel.composer, busy: false, activeTurnId: undefined },
    };
    modeRef.current = 'new-chat';
    viewModelRef.current = draftViewModel;
    setSwitchingSessionID('');
    setMode('new-chat');
    setViewModel(draftViewModel);
    void adapter
      .createSession(draftViewModel, draftTarget)
      .then((nextViewModel) => {
        if (newConversationDraftSeqRef.current !== draftSeq) {
          return;
        }
        modeRef.current = nextViewModel.mode;
        viewModelRef.current = nextViewModel;
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
      })
      .catch((error) => {
        if (newConversationDraftSeqRef.current !== draftSeq) {
          return;
        }
        console.warn('[workbench] start new conversation draft failed', error);
        modeRef.current = currentMode;
        viewModelRef.current = currentViewModel;
        setMode(currentMode);
        setViewModel(currentViewModel);
      });
  };

  const updateNewConversationDraft = (target: NewConversationDraftViewModel) => {
    newConversationDraftSeqRef.current += 1;
    const current = viewModelRef.current;
    const next: WorkbenchViewModel = {
      ...current,
      mode: 'new-chat',
      newConversationDraft: target,
      sessions: current.sessions.map((session) => ({ ...session, active: false })),
      conversation: current.sessions.some((session) => session.active) ? [] : current.conversation,
      timeline: current.sessions.some((session) => session.active) ? [] : current.timeline,
      turnDiagnostics: undefined,
      runProjection: undefined,
      reactCallchain: undefined,
      contextDiagnostics: undefined,
      pendingPermissions: [],
      composer: { ...current.composer, busy: false, activeTurnId: undefined },
    };
    modeRef.current = 'new-chat';
    viewModelRef.current = next;
    setSwitchingSessionID('');
    setMode('new-chat');
    setViewModel(next);
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
    newConversationDraftSeqRef.current += 1;
    const mutationSeq = ++sessionMutationSeqRef.current;
    const currentViewModel = viewModelRef.current;
    const optimisticViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: 'new-chat',
      newConversationDraft: undefined,
      sessions: currentViewModel.sessions.map((session) => ({ ...session, active: session.id === sessionID })),
      conversation: [],
      timeline: [],
      turnDiagnostics: undefined,
      runProjection: undefined,
      reactCallchain: undefined,
      contextDiagnostics: undefined,
      pendingPermissions: [],
      composer: { ...currentViewModel.composer, busy: false, activeTurnId: undefined },
    };
    modeRef.current = 'new-chat';
    viewModelRef.current = optimisticViewModel;
    setSwitchingSessionID(sessionID);
    setMode('new-chat');
    setViewModel(optimisticViewModel);
    void adapter
      .selectSession(optimisticViewModel, sessionID)
      .then((nextViewModel) => {
        if (sessionMutationSeqRef.current !== mutationSeq) {
          return;
        }
        setSwitchingSessionID('');
        modeRef.current = nextViewModel.mode;
        viewModelRef.current = { ...nextViewModel, newConversationDraft: undefined };
        setMode(nextViewModel.mode);
        setViewModel(viewModelRef.current);
      })
      .catch((error) => {
        if (sessionMutationSeqRef.current !== mutationSeq) {
          return;
        }
        console.warn('[workbench] select session failed', error);
        setSwitchingSessionID('');
        modeRef.current = mode;
        viewModelRef.current = viewModel;
        setMode(mode);
        setViewModel(viewModel);
      });
  };

  const renameSession = async (sessionID: string, title: string) => {
    const nextViewModel = await adapter.renameSession({ ...viewModel, mode }, sessionID, title);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const deleteSession = (sessionID: string) => {
    newConversationDraftSeqRef.current += 1;
    const mutationSeq = ++sessionMutationSeqRef.current;
    const currentViewModel = viewModelRef.current;
    const currentMode = modeRef.current;
    const wasActive = currentViewModel.sessions.some((session) => session.id === sessionID && session.active);
    const optimisticViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: wasActive ? 'new-chat' : currentMode,
      newConversationDraft: wasActive ? defaultDraftTarget(currentViewModel) : currentViewModel.newConversationDraft,
      sessions: currentViewModel.sessions.filter((session) => session.id !== sessionID),
      conversation: wasActive ? [] : currentViewModel.conversation,
      timeline: wasActive ? [] : currentViewModel.timeline,
      turnDiagnostics: wasActive ? undefined : currentViewModel.turnDiagnostics,
      runProjection: wasActive ? undefined : currentViewModel.runProjection,
      reactCallchain: wasActive ? undefined : currentViewModel.reactCallchain,
      contextDiagnostics: wasActive ? undefined : currentViewModel.contextDiagnostics,
      pendingPermissions: wasActive ? [] : currentViewModel.pendingPermissions,
      composer: wasActive ? { ...currentViewModel.composer, busy: false, activeTurnId: undefined } : currentViewModel.composer,
    };
    setSwitchingSessionID('');
    modeRef.current = optimisticViewModel.mode;
    viewModelRef.current = optimisticViewModel;
    setMode(optimisticViewModel.mode);
    setViewModel(optimisticViewModel);
    void adapter
      .deleteSession(optimisticViewModel, sessionID)
      .then((nextViewModel) => {
        if (sessionMutationSeqRef.current !== mutationSeq) {
          return;
        }
        modeRef.current = nextViewModel.mode;
        viewModelRef.current = nextViewModel;
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
      })
      .catch((error) => {
        if (sessionMutationSeqRef.current !== mutationSeq) {
          return;
        }
        console.warn('[workbench] delete session failed', error);
        modeRef.current = currentMode;
        viewModelRef.current = currentViewModel;
        setMode(currentMode);
        setViewModel(currentViewModel);
      });
  };

  const sendPrompt = async (prompt: string) => {
    const currentViewModel = viewModelRef.current;
    const currentMode = modeRef.current;
    const createdAt = Date.now();
    const clientRequestId = `prompt-${createdAt}`;
    const userID = `local-${createdAt}`;
    const loadingID = `loading-${createdAt}`;
    const baseOutputStore = currentViewModel.outputStore ?? createOutputStore(currentViewModel.sessions.find((session) => session.active)?.id ?? '');
    const optimisticOutputStore = addOptimisticUserSubmit(baseOutputStore, {
      clientRequestId,
      prompt,
      createdAt,
      status: 'submitting',
    });
    const outputConversation = optimisticOutputStore.sessionId ? selectConversationMessages(optimisticOutputStore) : undefined;
    const outputTimeline = optimisticOutputStore.sessionId ? selectConversationTimeline(optimisticOutputStore) : undefined;
    const optimisticViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: currentMode,
      composer: { ...currentViewModel.composer, busy: true },
      outputStore: optimisticOutputStore,
      conversation: outputConversation ?? [
        ...currentViewModel.conversation,
        {
          id: userID,
          role: 'user',
          content: prompt,
          createdAt,
          clientRequestId,
          status: 'success',
        },
        {
          id: loadingID,
          role: 'assistant',
          content: '正在生成回复...',
          createdAt,
          clientRequestId,
          status: 'loading',
        },
      ],
      timeline: outputTimeline ?? [
        ...currentViewModel.timeline,
        {
          id: userID,
          kind: 'message',
          role: 'user',
          content: prompt,
          createdAt,
          clientRequestId,
          status: 'success',
        },
        {
          id: loadingID,
          kind: 'message',
          role: 'assistant',
          content: '正在生成回复...',
          createdAt,
          clientRequestId,
          status: 'loading',
        },
      ],
    };
    viewModelRef.current = optimisticViewModel;
    setViewModel(optimisticViewModel);
    try {
      const nextViewModel = await adapter.sendPrompt(optimisticViewModel, prompt, { clientRequestId });
      modeRef.current = nextViewModel.mode;
      viewModelRef.current = nextViewModel;
      setSwitchingSessionID('');
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    } catch (error) {
      const failedOutputStore = {
        ...optimisticOutputStore,
        optimisticByClientRequestId: {
          ...optimisticOutputStore.optimisticByClientRequestId,
          [clientRequestId]: {
            ...optimisticOutputStore.optimisticByClientRequestId[clientRequestId],
            status: 'error' as const,
            error: sendPromptErrorMessage(error),
          },
        },
      };
      const failedViewModel: WorkbenchViewModel = {
        ...optimisticViewModel,
        outputStore: failedOutputStore,
        composer: { ...optimisticViewModel.composer, busy: false },
        conversation: failedOutputStore.sessionId ? selectConversationMessages(failedOutputStore) : optimisticViewModel.conversation.map((message) =>
          message.id === loadingID ? { ...message, content: sendPromptErrorMessage(error), status: 'error', error: sendPromptErrorMessage(error) } : message,
        ),
        timeline: failedOutputStore.sessionId ? selectConversationTimeline(failedOutputStore) : optimisticViewModel.timeline.map((item) =>
          item.id === loadingID ? { ...item, content: sendPromptErrorMessage(error), status: 'error', error: sendPromptErrorMessage(error) } : item,
        ),
      };
      viewModelRef.current = failedViewModel;
      setSwitchingSessionID('');
      setViewModel(failedViewModel);
    }
  };

  const cancelTurn = async () => {
    const nextViewModel = await adapter.cancelTurn({ ...viewModel, mode }, viewModel.composer.activeTurnId);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const resumeInterruptedTurn = async (turnID: string) => {
    const nextViewModel = await adapter.resumeInterruptedTurn({ ...viewModelRef.current, mode: modeRef.current }, turnID, { mode: 'continue' });
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const markInterruptedDone = async (turnID: string) => {
    const nextViewModel = await adapter.markInterruptedDone({ ...viewModel, mode }, turnID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const discardInterruptedTurn = async (turnID: string) => {
    const nextViewModel = await adapter.discardInterruptedTurn({ ...viewModelRef.current, mode: modeRef.current }, turnID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const retryRecoverableError = async (errorID: string) => {
    const nextViewModel = await adapter.retryRecoverableError({ ...viewModelRef.current, mode: modeRef.current }, errorID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const manualCompact = async () => {
    const nextViewModel = await adapter.manualCompact({ ...viewModelRef.current, mode: modeRef.current });
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const manualSnip = async () => {
    const nextViewModel = await adapter.manualSnip({ ...viewModelRef.current, mode: modeRef.current });
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

  const sendAgentTaskFollowUp = async (taskID: string, message: string) => {
    const nextViewModel = await adapter.sendAgentTaskFollowUp({ ...viewModelRef.current, mode: modeRef.current }, taskID, message);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const cancelAgentTask = async (taskID: string) => {
    const nextViewModel = await adapter.cancelAgentTask({ ...viewModelRef.current, mode: modeRef.current }, taskID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const listSessionTerminals = useCallback((sessionID: string) => adapter.listSessionTerminals(sessionID), [adapter]);

  const createTerminal = useCallback(
    (request: { sessionId: string; cwd?: string; profileId?: string; columns?: number; rows?: number }) => adapter.createTerminal(request),
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

  const selectTerminalProfile = async (profileID: string) => {
    const nextViewModel = await adapter.selectTerminalProfile({ ...viewModel, mode }, profileID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
    return nextViewModel.settings;
  };

  const decidePermission = async (permissionID: string, action: 'allow' | 'allow_session' | 'deny', guidance?: string) => {
    const nextViewModel = await adapter.decidePermission({ ...viewModel, mode }, permissionID, action, guidance);
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

  const listProjectMemories = (projectID: string) => {
    if (!adapter.listProjectMemories) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.listProjectMemories(projectID);
  };

  const getProjectMemory = (memoryID: string) => {
    if (!adapter.getProjectMemory) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.getProjectMemory(memoryID);
  };

  const createProjectMemory = (request: Parameters<NonNullable<WorkbenchAdapter['createProjectMemory']>>[0]) => {
    if (!adapter.createProjectMemory) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.createProjectMemory(request);
  };

  const updateProjectMemory = (memoryID: string, request: Parameters<NonNullable<WorkbenchAdapter['updateProjectMemory']>>[1]) => {
    if (!adapter.updateProjectMemory) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.updateProjectMemory(memoryID, request);
  };

  const setProjectMemoryEnabled = (memoryID: string, enabled: boolean) => {
    if (!adapter.setProjectMemoryEnabled) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.setProjectMemoryEnabled(memoryID, enabled);
  };

  const deleteProjectMemory = (memoryID: string, reason?: string) => {
    if (!adapter.deleteProjectMemory) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.deleteProjectMemory(memoryID, reason);
  };

  const refreshProjectMemoryIndex = (projectID: string) => {
    if (!adapter.refreshProjectMemoryIndex) {
      throw new Error('Project memory is unavailable');
    }
    return adapter.refreshProjectMemoryIndex(projectID);
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
          hooks={workbenchViewModel.hooks}
          hookExecutions={workbenchViewModel.hookExecutions}
          project={workbenchViewModel.currentProject}
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
          onTerminalProfileSelect={selectTerminalProfile}
          onMCPServerDetailsLoad={loadMCPServerDetails}
          onMCPServerRefresh={refreshMCPServer}
          onMCPServerSave={saveMCPServer}
          onMCPServerToggle={setMCPServerEnabled}
          onMCPToolToggle={setMCPToolEnabled}
          onSettingsRefresh={refreshSettings}
          onSkillRefresh={refreshSkills}
          onSkillToggle={setSkillEnabled}
          onProjectMemoryCreate={createProjectMemory}
          onProjectMemoryDelete={deleteProjectMemory}
          onProjectMemoryDetail={getProjectMemory}
          onProjectMemoryEnabledChange={setProjectMemoryEnabled}
          onProjectMemoryList={listProjectMemories}
          onProjectMemoryRefresh={refreshProjectMemoryIndex}
          onProjectMemoryUpdate={updateProjectMemory}
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
        onSessionRename={renameSession}
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
          switchingSessionID={switchingSessionID}
          onMinimumWorkspaceWidthChange={handleMinimumWorkspaceWidthChange}
          onModelSelect={selectModel}
          onPermissionDecide={decidePermission}
          onPermissionModeSelect={selectPermissionMode}
          onPromptCancel={cancelTurn}
          onSessionRename={renameSession}
          onPromptSubmit={sendPrompt}
          onNewConversationDraftChange={updateNewConversationDraft}
          onInterruptedResume={resumeInterruptedTurn}
          onInterruptedDone={markInterruptedDone}
          onInterruptedDiscard={discardInterruptedTurn}
          onRecoverableErrorRetry={retryRecoverableError}
          onManualCompact={manualCompact}
          onManualSnip={manualSnip}
          onRunCheckpointResume={resumeRunCheckpoint}
          onRunTaskExecute={executeRunTask}
          onAgentTaskFollowUp={sendAgentTaskFollowUp}
          onAgentTaskCancel={cancelAgentTask}
          onSessionTerminalsList={listSessionTerminals}
          onTerminalCreate={createTerminal}
          onTerminalDelete={deleteTerminal}
          onTerminalInput={writeTerminalInput}
          onTerminalResize={resizeTerminal}
          onTerminalSubscribe={subscribeTerminalEvents}
          onHookExecutionLoad={loadHookExecution}
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

function defaultDraftTarget(viewModel: WorkbenchViewModel): NewConversationDraftViewModel {
  if (viewModel.currentProject.id) {
    return { active: true, scope: 'project', projectId: viewModel.currentProject.id };
  }
  return { active: true, scope: 'standalone' };
}

function sendPromptErrorMessage(error: unknown) {
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return '发送失败';
}
