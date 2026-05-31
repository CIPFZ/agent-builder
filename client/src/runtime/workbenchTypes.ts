import type { ReactNode } from 'react';

export type WorkbenchMode = 'project' | 'new-chat' | 'settings';

export interface ProjectViewModel {
  id: string;
  name: string;
  path: string;
  isGitRepository: boolean;
  branch?: string;
  current?: boolean;
}

export interface SessionViewModel {
  id: string;
  title: string;
  updatedLabel: string;
}

export interface SidebarActionViewModel {
  id: string;
  label: string;
  icon: ReactNode;
  disabled?: boolean;
}

export interface ComposerViewModel {
  placeholder: string;
  permissionLabel: string;
  modelLabel: string;
}

export interface SettingsNavItemViewModel {
  key: string;
  label: string;
  icon: ReactNode;
}

export interface SettingsPermissionViewModel {
  key: string;
  title: string;
  description: string;
  enabled: boolean;
}

export interface SettingsOptionViewModel {
  label: string;
  value: string;
}

export interface SettingsViewModel {
  activeKey: string;
  navItems: SettingsNavItemViewModel[];
  permissions: SettingsPermissionViewModel[];
  defaultEditor: string;
  terminalProfile: string;
  editorOptions: SettingsOptionViewModel[];
  terminalOptions: SettingsOptionViewModel[];
}

export interface WorkbenchViewModel {
  mode: WorkbenchMode;
  currentProject: ProjectViewModel;
  projects: ProjectViewModel[];
  sessions: SessionViewModel[];
  sidebarActions: SidebarActionViewModel[];
  composer: ComposerViewModel;
  settings: SettingsViewModel;
}
