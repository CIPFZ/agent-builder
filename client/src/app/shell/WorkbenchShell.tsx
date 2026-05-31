import { useState } from 'react';
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react';
import type { ProjectViewModel, SessionViewModel, WorkbenchMode, WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Sidebar } from '../../features/sidebar/Sidebar.tsx';
import { SettingsPanel } from '../../features/settings/SettingsPanel.tsx';
import { Workspace } from '../../features/workspace/Workspace.tsx';
import { DesktopChrome } from './DesktopChrome.tsx';
import styles from './WorkbenchShell.module.css';

interface WorkbenchShellProps {
  viewModel: WorkbenchViewModel;
}

export function WorkbenchShell({ viewModel }: WorkbenchShellProps) {
  const [mode, setMode] = useState<WorkbenchMode>(viewModel.mode);
  const [projects, setProjects] = useState<ProjectViewModel[]>(viewModel.projects);
  const [sessions, setSessions] = useState<SessionViewModel[]>(viewModel.sessions);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(256);

  const createProject = () => {
    setProjects((items) => [
      ...items,
      {
        id: `mock-project-${items.length + 1}`,
        name: `模拟项目 ${items.length + 1}`,
        path: '',
        isGitRepository: false,
      },
    ]);
  };

  const createSession = () => {
    setSessions((items) => [
      ...items,
      {
        id: `mock-session-${items.length + 1}`,
        title: `模拟对话 ${items.length + 1}`,
        updatedLabel: '刚刚',
      },
    ]);
  };

  const workbenchViewModel = {
    ...viewModel,
    mode,
    projects,
    sessions,
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
        <SettingsPanel settings={workbenchViewModel.settings} onModeChange={setMode} />
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
          onModeChange={setMode}
          onProjectCreate={createProject}
          onSessionCreate={createSession}
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
      <Workspace viewModel={workbenchViewModel} />
    </main>
  );
}
