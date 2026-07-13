import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { createConversationSubmitQueue } from '../../runtime/conversationSubmitQueue.ts';
import { createCanonicalConversationCoordinator } from '../../runtime/canonicalConversationCoordinator.ts';
import { selectOwnedClientRequestIds } from '../../runtime/canonicalConversationSelectors.ts';
import { installWebviewCursorRecovery, nudgeCursorRecompute } from '../../lib/webviewCursor.ts';
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
const SHELL_SPLITTER_WIDTH = 2;
const SHELL_MIN_WIDTH = 1080;

function getLayoutWidth() {
  if (typeof window === 'undefined') {
    return SHELL_MIN_WIDTH;
  }
  return Math.max(window.innerWidth, SHELL_MIN_WIDTH);
}

function preserveCanonicalConversation(nextViewModel: WorkbenchViewModel, current: WorkbenchViewModel): WorkbenchViewModel {
  return { ...nextViewModel, canonicalConversationStore: current.canonicalConversationStore };
}

function getSidebarMaxWidth(workspaceMinVisibleWidth = WORKSPACE_MIN_VISIBLE_WIDTH) {
  return Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, getLayoutWidth() - workspaceMinVisibleWidth - SHELL_SPLITTER_WIDTH));
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
  const promptSubmitQueueRef = useRef(createConversationSubmitQueue());
  const canonicalCoordinator = useMemo(() => {
    if (!adapter.fetchCanonicalConversationSnapshot || !adapter.subscribeCanonicalConversation) return undefined;
    return createCanonicalConversationCoordinator({
      fetchSnapshot: adapter.fetchCanonicalConversationSnapshot,
      subscribe: adapter.subscribeCanonicalConversation,
      onStore: (store) => {
        // The canonical snapshot is the readiness boundary for conversation
        // rendering. Do not keep the loading placeholder coupled to the
        // slower diagnostics/status hydration performed by selectSession.
        setSwitchingSessionID((current) => current === store.sessionId ? '' : current);
        setViewModel((current) => (
          current.conversationTarget.kind === 'session' && current.conversationTarget.sessionId === store.sessionId
            ? { ...current, canonicalConversationStore: store, optimisticConversationByClientRequestId: pruneEchoedOptimisticSubmits(current.optimisticConversationByClientRequestId, store) }
            : current
        ));
      },
    });
  }, [adapter.fetchCanonicalConversationSnapshot, adapter.subscribeCanonicalConversation]);
  const hasBusySession = viewModel.sessions.some((session) => session.busy);
  const sidebarForceCollapsed =
    !sidebarCollapsed &&
    workspaceMinVisibleWidth > WORKSPACE_MIN_VISIBLE_WIDTH &&
    viewportWidth > 0 &&
    getLayoutWidth() < SIDEBAR_MIN_WIDTH + SHELL_SPLITTER_WIDTH + workspaceMinVisibleWidth;
  const effectiveSidebarCollapsed = sidebarCollapsed || sidebarForceCollapsed;

  useEffect(() => {
    viewModelRef.current = viewModel;
  }, [viewModel]);


  useEffect(() => {
    installWebviewCursorRecovery();
  }, []);

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
      const epoch = sessionMutationSeqRef.current;
      try {
        const nextViewModel = await adapter.refresh({ ...viewModelRef.current, mode: modeRef.current });
        if (!cancelled && sessionMutationSeqRef.current === epoch) {
          setMode(nextViewModel.mode);
          setViewModel((current) => preserveCanonicalConversation(nextViewModel, current));
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

    const streamHandlesEvent = Boolean(adapter.subscribeCanonicalConversation);
    void Promise.resolve(adapter.subscribeRuntimeEvents((event) => {
      // The canonical per-session stream owns conversation changes; general
      // Runtime refreshes only update non-conversation workbench state.
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

  const activeSessionID = viewModel.conversationTarget.kind === 'session' ? viewModel.conversationTarget.sessionId : undefined;

  useEffect(() => {
    const coordinator = canonicalCoordinator;
    if (!coordinator) return undefined;
    coordinator.activate(activeSessionID ?? '');
    return () => coordinator.stop();
  }, [activeSessionID, canonicalCoordinator]);

  // WP5 refreshNonce: contextUsage.compactCount is only known once a
  // refresh has already delivered it, so the general event-driven refresh
  // (350ms-coalesced for compact.* events) is what first surfaces a bump.
  // Once observed, force one more *immediate* SessionContextUsage read
  // instead of waiting for the next coalesced cycle — a compact just
  // finished, so the composer indicator should reflect the post-compact
  // numbers as soon as possible. contextUsageRefreshSeqRef discards a
  // slower, still in-flight forced fetch if a newer compact bumps the
  // count again before the first one resolves.
  const contextUsageRefreshSeqRef = useRef(0);
  const lastCompactCountRef = useRef<{ sessionId: string; count: number } | undefined>(undefined);
  useEffect(() => {
    const usage = viewModel.composer.contextUsage;
    if (!usage || !adapter.fetchContextUsage) {
      return;
    }
    const previous = lastCompactCountRef.current;
    lastCompactCountRef.current = { sessionId: usage.sessionId, count: usage.compactCount };
    if (!previous || previous.sessionId !== usage.sessionId || previous.count === usage.compactCount) {
      return;
    }
    const seq = ++contextUsageRefreshSeqRef.current;
    void adapter.fetchContextUsage(usage.sessionId).then((fresh) => {
      if (!fresh || contextUsageRefreshSeqRef.current !== seq) {
        return;
      }
      setViewModel((current) => (
        current.composer.contextUsage?.sessionId === fresh.sessionId
          ? { ...current, composer: { ...current.composer, contextUsage: fresh } }
          : current
      ));
    }).catch(() => undefined);
  }, [adapter, viewModel.composer.contextUsage]);


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
    const hasConversationStream = Boolean(adapter.subscribeCanonicalConversation);
    const busyIntervalMs = hasConversationStream ? 3000 : 1200;
    const backoffMs = hasConversationStream ? 4000 : 2000;

    const refreshUntilIdle = async () => {
      const epoch = sessionMutationSeqRef.current;
      try {
        const nextViewModel = await adapter.refresh({ ...viewModelRef.current, mode });
        if (cancelled) {
          return;
        }
        if (sessionMutationSeqRef.current !== epoch) {
          // The conversation context changed while this refresh was in
          // flight (session switch / draft / new prompt); its snapshot
          // describes a stale context and must not clobber the new one.
          timer = window.setTimeout(refreshUntilIdle, busyIntervalMs);
          return;
        }
        setMode(nextViewModel.mode);
        setViewModel((current) => preserveCanonicalConversation(nextViewModel, current));
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

  const loadEarlierConversation = useCallback(
    (sessionID: string) => canonicalCoordinator?.loadEarlier(sessionID) ?? Promise.resolve(false),
    [canonicalCoordinator],
  );

  const loadCanonicalMessageContent = useCallback(
    (sessionID: string, messageID: string) => adapter.fetchCanonicalMessageContent?.(sessionID, messageID) ?? Promise.reject(new Error('canonical message content API is unavailable')),
    [adapter],
  );

  const searchConversation = useCallback(
    (sessionID: string, query: string) => adapter.searchConversation?.(sessionID, query) ?? Promise.resolve([]),
    [adapter],
  );

  const openConversationSearchResult = useCallback(
    (sessionID: string, turnID: string) => canonicalCoordinator?.loadAround(sessionID, turnID) ?? Promise.resolve(false),
    [canonicalCoordinator],
  );

  const startConversationDraft = (target?: NewConversationDraftViewModel) => {
    sessionMutationSeqRef.current += 1;
    const currentViewModel = viewModelRef.current;
    const draftTarget = target ?? defaultDraftTarget(currentViewModel);
    const draftViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: 'new-chat',
      conversationTarget: { kind: 'draft', scope: draftTarget.scope, projectId: draftTarget.projectId },
      sessions: currentViewModel.sessions.map((session) => ({ ...session, active: false })),
      conversation: [],
      canonicalConversationStore: undefined,
      optimisticConversationByClientRequestId: undefined,
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
  };

  const updateNewConversationDraft = (target: NewConversationDraftViewModel) => {
    sessionMutationSeqRef.current += 1;
    const current = viewModelRef.current;
    const leavingActiveSession = current.sessions.some((session) => session.active);
    const next: WorkbenchViewModel = {
      ...current,
      mode: 'new-chat',
      conversationTarget: { kind: 'draft', scope: target.scope, projectId: target.projectId },
      sessions: current.sessions.map((session) => ({ ...session, active: false })),
      conversation: leavingActiveSession ? [] : current.conversation,
      canonicalConversationStore: leavingActiveSession ? undefined : current.canonicalConversationStore,
      optimisticConversationByClientRequestId: leavingActiveSession ? undefined : current.optimisticConversationByClientRequestId,
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
    const mutationSeq = ++sessionMutationSeqRef.current;
    const nextViewModel = await adapter.openProject({ ...viewModelRef.current, mode: modeRef.current }, request);
    if (sessionMutationSeqRef.current !== mutationSeq) return;
    modeRef.current = nextViewModel.mode;
    viewModelRef.current = nextViewModel;
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const createProject = async (request: CreateProjectRequestViewModel) => {
    const mutationSeq = ++sessionMutationSeqRef.current;
    const nextViewModel = await adapter.createProject({ ...viewModelRef.current, mode: modeRef.current }, request);
    if (sessionMutationSeqRef.current !== mutationSeq) return;
    modeRef.current = nextViewModel.mode;
    viewModelRef.current = nextViewModel;
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
    const mutationSeq = ++sessionMutationSeqRef.current;
    const previous = viewModelRef.current;
    const removedSessionIDs = new Set(previous.sessions.filter((session) => session.projectId === request.projectId).map((session) => session.id));
    for (const sessionID of removedSessionIDs) canonicalCoordinator?.evict(sessionID);
    const selectedSessionRemoved = previous.conversationTarget.kind === 'session' && removedSessionIDs.has(previous.conversationTarget.sessionId);
    const selectedProjectRemoved = previous.conversationTarget.kind === 'draft' && previous.conversationTarget.scope === 'project' && previous.conversationTarget.projectId === request.projectId;
    const currentProjectRemoved = previous.currentProject.id === request.projectId;
    const clearsConversation = selectedSessionRemoved || selectedProjectRemoved || currentProjectRemoved;
    const optimistic: WorkbenchViewModel = {
      ...previous,
      mode: clearsConversation ? 'new-chat' : modeRef.current,
      currentProject: currentProjectRemoved ? { id: '', name: '', path: '', isGitRepository: false } : previous.currentProject,
      projects: previous.projects.filter((project) => project.id !== request.projectId),
      sessions: previous.sessions.filter((session) => !removedSessionIDs.has(session.id)),
      conversationTarget: clearsConversation ? { kind: 'draft', scope: 'standalone' } : previous.conversationTarget,
      conversation: clearsConversation ? [] : previous.conversation,
      canonicalConversationStore: clearsConversation ? undefined : previous.canonicalConversationStore,
      optimisticConversationByClientRequestId: clearsConversation ? undefined : previous.optimisticConversationByClientRequestId,
      turnDiagnostics: clearsConversation ? undefined : previous.turnDiagnostics,
      runProjection: clearsConversation ? undefined : previous.runProjection,
      agentTasks: clearsConversation ? [] : previous.agentTasks,
      reactCallchain: clearsConversation ? undefined : previous.reactCallchain,
      contextDiagnostics: clearsConversation ? undefined : previous.contextDiagnostics,
      pendingPermissions: clearsConversation ? [] : previous.pendingPermissions,
      composer: clearsConversation ? { ...previous.composer, busy: false, activeTurnId: undefined, contextUsage: undefined } : previous.composer,
    };
    viewModelRef.current = optimistic;
    modeRef.current = optimistic.mode;
    setSwitchingSessionID('');
    setMode(optimistic.mode);
    setViewModel(optimistic);
    try {
      const settled = await adapter.removeProject(optimistic, request);
      if (sessionMutationSeqRef.current !== mutationSeq) return;
      viewModelRef.current = settled;
      modeRef.current = settled.mode;
      setMode(settled.mode);
      setViewModel(settled);
    } catch (error) {
      if (sessionMutationSeqRef.current === mutationSeq) {
        viewModelRef.current = previous;
        modeRef.current = previous.mode;
        setMode(previous.mode);
        setViewModel(previous);
      }
      throw error;
    }
  };

  const selectProjectDirectory = () => adapter.selectProjectDirectory();

  const selectSession = (sessionID: string) => {
    const mutationSeq = ++sessionMutationSeqRef.current;
    const currentViewModel = viewModelRef.current;
    const optimisticViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: 'new-chat',
      conversationTarget: { kind: 'session', sessionId: sessionID },
      sessions: currentViewModel.sessions.map((session) => ({ ...session, active: session.id === sessionID })),
      conversation: [],
      canonicalConversationStore: canonicalCoordinator?.cached(sessionID),
      optimisticConversationByClientRequestId: undefined,
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
        modeRef.current = nextViewModel.mode;
        // A canonical snapshot may have arrived while the ancillary session
        // hydration was in flight. Preserve it instead of replacing the
        // rendered conversation with the hydration request's stale base.
        const canonicalStore = canonicalCoordinator?.cached(sessionID);
        viewModelRef.current = {
          ...nextViewModel,
          conversationTarget: { kind: 'session', sessionId: sessionID },
          canonicalConversationStore: canonicalStore,
        };
        if (canonicalStore) setSwitchingSessionID('');
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
    canonicalCoordinator?.evict(sessionID);
    const mutationSeq = ++sessionMutationSeqRef.current;
    const currentViewModel = viewModelRef.current;
    const currentMode = modeRef.current;
    // conversationTarget is the UI selection authority. The sidebar's active
    // flag is a projection and can briefly lag during selection/hydration.
    const wasSelected =
      (currentViewModel.conversationTarget.kind === 'session' && currentViewModel.conversationTarget.sessionId === sessionID) ||
      currentViewModel.sessions.some((session) => session.id === sessionID && session.active);
    const draftAfterDelete = defaultDraftTarget(currentViewModel);
    const optimisticViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: wasSelected ? 'new-chat' : currentMode,
      conversationTarget: wasSelected
        ? { kind: 'draft' as const, scope: draftAfterDelete.scope, projectId: draftAfterDelete.projectId }
        : currentViewModel.conversationTarget,
      sessions: currentViewModel.sessions.filter((session) => session.id !== sessionID),
      conversation: wasSelected ? [] : currentViewModel.conversation,
      canonicalConversationStore: wasSelected ? undefined : currentViewModel.canonicalConversationStore,
      optimisticConversationByClientRequestId: wasSelected ? undefined : currentViewModel.optimisticConversationByClientRequestId,
      turnDiagnostics: wasSelected ? undefined : currentViewModel.turnDiagnostics,
      runProjection: wasSelected ? undefined : currentViewModel.runProjection,
      agentTasks: wasSelected ? [] : currentViewModel.agentTasks,
      reactCallchain: wasSelected ? undefined : currentViewModel.reactCallchain,
      contextDiagnostics: wasSelected ? undefined : currentViewModel.contextDiagnostics,
      pendingPermissions: wasSelected ? [] : currentViewModel.pendingPermissions,
      composer: wasSelected ? { ...currentViewModel.composer, busy: false, activeTurnId: undefined, contextUsage: undefined } : currentViewModel.composer,
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
        const settledViewModel = wasSelected
          ? {
              ...nextViewModel,
              mode: 'new-chat' as const,
              conversationTarget: { kind: 'draft' as const, scope: draftAfterDelete.scope, projectId: draftAfterDelete.projectId },
              conversation: [],
              canonicalConversationStore: undefined,
              optimisticConversationByClientRequestId: undefined,
            }
          : nextViewModel;
        modeRef.current = settledViewModel.mode;
        viewModelRef.current = settledViewModel;
        setMode(settledViewModel.mode);
        setViewModel(settledViewModel);
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

  const sendPrompt = (prompt: string) => {
    // Serialize the draft -> session transition. Each queued submit re-reads
    // viewModelRef only after the preceding submit has stored its session id.
    return promptSubmitQueueRef.current.enqueue(async () => {
    // Give this submit an exclusive mutation generation. Refreshes that
    // started before the submit, or while the runtime is adopting a draft
    // into its new session, must not overwrite the transition with a stale
    // draft projection.
    const conversationEpoch = ++sessionMutationSeqRef.current;
    const currentViewModel = viewModelRef.current;
    const currentMode = modeRef.current;
    const createdAt = Date.now();
    const clientRequestId = `prompt-${createdAt}`;
    const userID = `local-${createdAt}`;
    const activeSessionId = currentViewModel.conversationTarget.kind === 'session' ? currentViewModel.conversationTarget.sessionId : undefined;
    const optimisticSubmit = { clientRequestId, sessionId: activeSessionId, prompt, createdAt, status: 'submitting' as const };
    const optimisticViewModel: WorkbenchViewModel = {
      ...currentViewModel,
      mode: currentMode,
      composer: { ...currentViewModel.composer, busy: true },
      optimisticConversationByClientRequestId: { ...(currentViewModel.optimisticConversationByClientRequestId ?? {}), [clientRequestId]: optimisticSubmit },
      conversation: [
        ...currentViewModel.conversation,
        { id: userID, role: 'user', content: prompt, createdAt, clientRequestId, status: 'success' as const },
      ],
    };
    viewModelRef.current = optimisticViewModel;
    setViewModel(optimisticViewModel);
    try {
      const nextViewModel = await adapter.sendPrompt(optimisticViewModel, prompt, { clientRequestId });
      if (sessionMutationSeqRef.current !== conversationEpoch) {
        return;
      }
      sessionMutationSeqRef.current += 1;
      const adoptedSessionId = nextViewModel.conversationTarget.kind === 'session' ? nextViewModel.conversationTarget.sessionId : undefined;
      nextViewModel.optimisticConversationByClientRequestId = {
        ...(nextViewModel.optimisticConversationByClientRequestId ?? {}),
        [clientRequestId]: { ...optimisticSubmit, sessionId: adoptedSessionId },
      };
      modeRef.current = nextViewModel.mode;
      viewModelRef.current = nextViewModel;
      setSwitchingSessionID('');
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    } catch (error) {
      if (sessionMutationSeqRef.current !== conversationEpoch) {
        return;
      }
      sessionMutationSeqRef.current += 1;
      const errorMessage = sendPromptErrorMessage(error);
      const failedViewModel: WorkbenchViewModel = {
        ...optimisticViewModel,
        composer: { ...optimisticViewModel.composer, busy: false },
        optimisticConversationByClientRequestId: {
          ...(optimisticViewModel.optimisticConversationByClientRequestId ?? {}),
          [clientRequestId]: { ...optimisticSubmit, status: 'error', error: errorMessage },
        },
        conversation: optimisticViewModel.conversation.map((message) => message.clientRequestId === clientRequestId ? { ...message, status: 'error', error: errorMessage } : message),
      };
      viewModelRef.current = failedViewModel;
      setSwitchingSessionID('');
      setViewModel(failedViewModel);
    }
    });
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

  const manualCompact = async (instructions?: string) => {
    const nextViewModel = await adapter.manualCompact({ ...viewModelRef.current, mode: modeRef.current }, instructions);
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

  const subscribeTerminalEvents = adapter.subscribeTerminalEvents;

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

  const getContextGovernanceSettings = () => {
    if (!adapter.getContextGovernanceSettings) {
      throw new Error('Context governance settings are unavailable');
    }
    return adapter.getContextGovernanceSettings();
  };

  const saveContextGovernanceSettings = (settings: Parameters<NonNullable<WorkbenchAdapter['saveContextGovernanceSettings']>>[0]) => {
    if (!adapter.saveContextGovernanceSettings) {
      throw new Error('Context governance settings are unavailable');
    }
    return adapter.saveContextGovernanceSettings(settings);
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

  const startSidebarWidthDrag = (event: ReactPointerEvent<HTMLDivElement>) => {
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

    const stopSidebarWidthDrag = () => {
      target.releasePointerCapture(pointerId);
      window.removeEventListener('pointermove', updateSidebarWidth);
      window.removeEventListener('pointerup', stopSidebarWidthDrag);
      window.removeEventListener('pointercancel', stopSidebarWidthDrag);
      nudgeCursorRecompute();
    };

    window.addEventListener('pointermove', updateSidebarWidth);
    window.addEventListener('pointerup', stopSidebarWidthDrag);
    window.addEventListener('pointercancel', stopSidebarWidthDrag);
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
          contextUsage={workbenchViewModel.composer.contextUsage}
          onModelSelect={selectModel}
          onTerminalProfileSelect={selectTerminalProfile}
          onContextGovernanceLoad={getContextGovernanceSettings}
          onContextGovernanceSave={saveContextGovernanceSettings}
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
        onSessionCreate={startConversationDraft}
        onSessionRename={renameSession}
        onSessionDelete={deleteSession}
        onSessionSelect={selectSession}
      />
      {!effectiveSidebarCollapsed && (
        <div
          aria-label="调整侧栏宽度"
          className={styles.sidebarSplitter}
          role="separator"
          aria-orientation="vertical"
          aria-valuemin={SIDEBAR_MIN_WIDTH}
          aria-valuemax={sidebarMaxWidth}
          aria-valuenow={sidebarWidth}
          tabIndex={0}
          onPointerDown={startSidebarWidthDrag}
          onPointerLeave={nudgeCursorRecompute}
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
          onConversationLoadEarlier={loadEarlierConversation}
          onMessageContentLoad={loadCanonicalMessageContent}
          onConversationSearch={searchConversation}
          onConversationSearchResultOpen={openConversationSearchResult}
        />
      )}
    </main>
  );
}

function pruneEchoedOptimisticSubmits(submits: WorkbenchViewModel['optimisticConversationByClientRequestId'], store: import('../../runtime/canonicalConversationStore.ts').CanonicalConversationStore) {
  if (!submits) return undefined;
  const echoed = selectOwnedClientRequestIds(store);
  const next = Object.fromEntries(Object.entries(submits).filter(([id, submit]) => !echoed.has(id) && (!submit.sessionId || submit.sessionId === store.sessionId)));
  return Object.keys(next).length ? next : undefined;
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
