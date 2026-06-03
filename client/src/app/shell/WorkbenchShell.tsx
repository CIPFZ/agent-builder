import { useEffect, useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react';
import type { WorkbenchAdapter, WorkbenchMode, WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Sidebar } from '../../features/sidebar/Sidebar.tsx';
import { SettingsPanel } from '../../features/settings/SettingsPanel.tsx';
import { Workspace } from '../../features/workspace/Workspace.tsx';
import { DesktopChrome } from './DesktopChrome.tsx';
import styles from './WorkbenchShell.module.css';

interface WorkbenchShellProps {
  adapter: WorkbenchAdapter;
  viewModel: WorkbenchViewModel;
}

export function WorkbenchShell({ adapter, viewModel: initialViewModel }: WorkbenchShellProps) {
  const [viewModel, setViewModel] = useState<WorkbenchViewModel>(initialViewModel);
  const [mode, setMode] = useState<WorkbenchMode>(initialViewModel.mode);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(256);
  const viewModelRef = useRef(viewModel);

  useEffect(() => {
    viewModelRef.current = viewModel;
  }, [viewModel]);

  useEffect(() => {
    const hasActiveSession = viewModel.sessions.some((session) => session.busy);
    if (!viewModel.composer.busy && !hasActiveSession) {
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
  }, [adapter, mode, viewModel.composer.busy, viewModel.sessions]);

  const changeMode = (nextMode: WorkbenchMode) => {
    setMode(nextMode);
    setViewModel((current) => ({ ...current, mode: nextMode }));
    if (nextMode === 'settings') {
      void adapter.refresh({ ...viewModel, mode: nextMode }).then((nextViewModel) => {
        setMode(nextViewModel.mode);
        setViewModel(nextViewModel);
      });
    }
  };

  const createSession = () => {
    void adapter.createSession({ ...viewModel, mode }).then((nextViewModel) => {
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    });
  };

  const createProject = () => {
    changeMode('project');
  };

  const selectSession = (sessionID: string) => {
    void adapter.selectSession({ ...viewModel, mode }, sessionID).then((nextViewModel) => {
      setMode(nextViewModel.mode);
      setViewModel(nextViewModel);
    });
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
    const nextViewModel = await adapter.sendPrompt(optimisticViewModel, prompt);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const cancelTurn = async () => {
    const nextViewModel = await adapter.cancelTurn({ ...viewModel, mode }, viewModel.composer.activeTurnId);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
  };

  const selectModel = async (configuredProviderID: string, model: string) => {
    const nextViewModel = await adapter.selectModel({ ...viewModel, mode }, configuredProviderID, model);
    setViewModel(nextViewModel);
  };

  const selectPermissionMode = async (permissionMode: string) => {
    const nextViewModel = await adapter.selectPermissionMode({ ...viewModel, mode }, permissionMode);
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

  const deleteConfiguredProvider = async (providerID: string) => {
    const nextViewModel = await adapter.deleteConfiguredProvider({ ...viewModel, mode }, providerID);
    setMode(nextViewModel.mode);
    setViewModel(nextViewModel);
    return nextViewModel.settings.configuredProviders;
  };

  const discoverConfiguredProviderModels = (providerID: string) => adapter.discoverConfiguredProviderModels(providerID);

  const testConfiguredProvider = (providerID: string) => adapter.testConfiguredProvider(providerID);

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

  const startSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const pointerId = event.pointerId;
    const startX = event.clientX;
    const startWidth = sidebarWidth;
    const target = event.currentTarget;

    target.setPointerCapture(pointerId);

    const updateSidebarWidth = (moveEvent: PointerEvent) => {
      const nextWidth = Math.min(360, Math.max(220, startWidth + moveEvent.clientX - startX));
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
        <DesktopChrome />
        <SettingsPanel
          settings={workbenchViewModel.settings}
          onModeChange={changeMode}
          onProviderDelete={deleteConfiguredProvider}
          onProviderDiscoverModels={discoverConfiguredProviderModels}
          onProviderLatency={measureConfiguredProviderLatency}
          onProviderSave={saveConfiguredProvider}
          onProviderTest={testConfiguredProvider}
          onMCPServerDetailsLoad={loadMCPServerDetails}
          onMCPServerRefresh={refreshMCPServer}
          onMCPServerSave={saveMCPServer}
          onMCPServerToggle={setMCPServerEnabled}
          onMCPToolToggle={setMCPToolEnabled}
          onPermissionModeSelect={selectPermissionMode}
          onSettingsRefresh={refreshSettings}
          onSkillRefresh={refreshSkills}
          onSkillToggle={setSkillEnabled}
        />
      </main>
    );
  }

  return (
    <main
      className={`${styles.shell} ${sidebarCollapsed ? styles.sidebarCollapsed : ''}`}
      data-testid="workbench-shell"
      style={{ '--sidebar-width': `${sidebarWidth}px` } as CSSProperties}
    >
      <DesktopChrome
        sidebarCollapsed={sidebarCollapsed}
        onSidebarToggle={() => setSidebarCollapsed((collapsed) => !collapsed)}
      />
      {!sidebarCollapsed && (
        <Sidebar
          viewModel={workbenchViewModel}
          onModeChange={changeMode}
          onProjectCreate={createProject}
          onSessionCreate={createSession}
          onSessionDelete={deleteSession}
          onSessionSelect={selectSession}
        />
      )}
      {!sidebarCollapsed && (
        <div
          aria-label="调整侧栏宽度"
          className={styles.sidebarResizer}
          role="separator"
          aria-orientation="vertical"
          aria-valuemin={220}
          aria-valuemax={360}
          aria-valuenow={sidebarWidth}
          tabIndex={0}
          onPointerDown={startSidebarResize}
        />
      )}
      <Workspace
        viewModel={workbenchViewModel}
        onModelSelect={selectModel}
        onPermissionDecide={decidePermission}
        onPermissionModeSelect={selectPermissionMode}
        onPromptCancel={cancelTurn}
        onPromptSubmit={sendPrompt}
      />
    </main>
  );
}
