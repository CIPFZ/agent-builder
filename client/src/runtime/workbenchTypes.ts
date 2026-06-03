import type { ReactNode } from 'react';

export type WorkbenchMode = 'project' | 'new-chat' | 'settings' | 'plugins';

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
  active?: boolean;
  busy?: boolean;
  activeTurnId?: string;
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
  permissionMode?: PermissionModeViewModel;
  permissionOptions: PermissionModeOptionViewModel[];
  modelLabel: string;
  capabilityLabel: string;
  selectedModel?: RuntimeModelOptionViewModel;
  modelOptions: RuntimeModelOptionViewModel[];
  busy?: boolean;
  activeTurnId?: string;
}

export interface RuntimeModelOptionViewModel {
  id: string;
  name: string;
  provider: string;
  providerId?: string;
  configuredProviderId?: string;
  configuredProvider?: string;
  selected?: boolean;
}

export interface ConversationMessageViewModel {
  id: string;
  role: 'user' | 'assistant' | 'tool' | 'system';
  content: string;
  createdAt?: number;
  provider?: string;
  model?: string;
  status?: 'loading' | 'success' | 'error';
  error?: string;
}

export type ConversationTimelineKind = 'message' | 'thinking' | 'tool_call' | 'permission' | 'progress';

export interface ConversationTimelineItemViewModel {
  id: string;
  kind: ConversationTimelineKind;
  sessionId?: string;
  turnId?: string;
  messageId?: string;
  toolCallId?: string;
  role?: 'user' | 'assistant' | 'tool' | 'system';
  title?: string;
  content?: string;
  summary?: string;
  status?: string;
  createdAt?: number;
  updatedAt?: number;
  provider?: string;
  model?: string;
  error?: string;
  toolCall?: ToolCallViewModel;
  permission?: PermissionRequestViewModel;
}

export interface ToolCallViewModel {
  id: string;
  sessionId: string;
  turnId: string;
  name: string;
  source: string;
  command?: string;
  risk?: string;
  status: string;
  inputSummary?: string;
  outputSummary?: string;
  stdout?: string;
  stderr?: string;
  error?: string;
  policyMode?: string;
  policyReason?: string;
  policyTargetSummary?: string;
  startedAt?: number;
  finishedAt?: number;
}

export interface PermissionRequestViewModel {
  id: string;
  sessionId: string;
  turnId?: string;
  toolCallId: string;
  toolName: string;
  action: string;
  risk?: string;
  status: string;
  target?: string;
  reason?: string;
  policyMode?: string;
  createdAt?: number;
  decidedAt?: number;
}

export interface PermissionModeViewModel {
  mode: string;
  label: string;
  description?: string;
}

export interface PermissionModeOptionViewModel extends PermissionModeViewModel {
  value: string;
  disabled?: boolean;
  disabledReason?: string;
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

export interface ProviderTypeViewModel {
  id: string;
  name: string;
  description?: string;
}

export interface ProviderCatalogItemViewModel {
  id: string;
  name: string;
  type: string;
  apiEndpoint?: string;
  apiKeyTemplate?: string;
  modelCount: number;
  defaultLargeModel?: string;
  defaultSmallModel?: string;
  requiredFields: string[];
  notes: string[];
  configurable: boolean;
}

export interface ConfiguredProviderViewModel {
  id: string;
  providerId: string;
  name: string;
  remark?: string;
  apiEndpoint?: string;
  authVariable?: string;
  protocol?: string;
  defaultModel?: string;
  tokenConfigured?: boolean;
  proxy?: string;
  enabled?: boolean;
  mainModel?: string;
  haikuModel?: string;
  sonnetModel?: string;
  opusModel?: string;
}

export interface ProviderModelDiscoveryViewModel {
  providerId: string;
  models: string[];
  error?: string;
}

export interface ProviderTestViewModel {
  ok: boolean;
  providerId: string;
  model?: string;
  durationMs?: number;
  error?: string;
}

export interface RuntimeSkillViewModel {
  name: string;
  description?: string;
  builtin: boolean;
  enabled: boolean;
  path?: string;
  skillFilePath?: string;
  state: string;
  diagnostics?: string;
  error?: string;
  reason?: string;
  allowedTools: string[];
  capabilityId?: string;
  policyMode?: string;
  policyRisk?: string;
  policyReason?: string;
}

export interface RuntimeMCPCountsViewModel {
  tools: number;
  prompts: number;
  resources: number;
}

export interface RuntimeMCPServerViewModel {
  name: string;
  type: string;
  url?: string;
  command?: string;
  args: string[];
  disabled: boolean;
  enabled: boolean;
  state: string;
  counts: RuntimeMCPCountsViewModel;
  diagnostics?: string;
  reason?: string;
  error?: string;
  enabledTools: string[];
  disabledTools: string[];
}

export interface RuntimeMCPToolViewModel {
  server: string;
  name: string;
  description?: string;
  enabled: boolean;
}

export interface RuntimeMCPResourceViewModel {
  server: string;
  uri: string;
  name?: string;
  description?: string;
  mimeType?: string;
}

export interface RuntimeMCPPromptViewModel {
  server: string;
  name: string;
  description?: string;
}

export interface SettingsViewModel {
  activeKey: string;
  navItems: SettingsNavItemViewModel[];
  permissions: SettingsPermissionViewModel[];
  permissionMode?: PermissionModeViewModel;
  permissionOptions: PermissionModeOptionViewModel[];
  defaultEditor: string;
  terminalProfile: string;
  editorOptions: SettingsOptionViewModel[];
  terminalOptions: SettingsOptionViewModel[];
  providerTypes: ProviderTypeViewModel[];
  providers: ProviderCatalogItemViewModel[];
  configuredProviders: ConfiguredProviderViewModel[];
  skills: RuntimeSkillViewModel[];
  mcpServers: RuntimeMCPServerViewModel[];
  mcpToolsByServer: Record<string, RuntimeMCPToolViewModel[]>;
  mcpResourcesByServer: Record<string, RuntimeMCPResourceViewModel[]>;
  mcpPromptsByServer: Record<string, RuntimeMCPPromptViewModel[]>;
}

export interface WorkbenchViewModel {
  mode: WorkbenchMode;
  currentProject: ProjectViewModel;
  projects: ProjectViewModel[];
  sessions: SessionViewModel[];
  sidebarActions: SidebarActionViewModel[];
  conversation: ConversationMessageViewModel[];
  timeline: ConversationTimelineItemViewModel[];
  pendingPermissions: PermissionRequestViewModel[];
  composer: ComposerViewModel;
  settings: SettingsViewModel;
}

export interface WorkbenchAdapter {
  loadInitialViewModel: (mode?: WorkbenchMode) => Promise<WorkbenchViewModel>;
  refresh: (current: WorkbenchViewModel) => Promise<WorkbenchViewModel>;
  createSession: (current: WorkbenchViewModel) => Promise<WorkbenchViewModel>;
  selectSession: (current: WorkbenchViewModel, sessionID: string) => Promise<WorkbenchViewModel>;
  deleteSession: (current: WorkbenchViewModel, sessionID: string) => Promise<WorkbenchViewModel>;
  selectModel: (current: WorkbenchViewModel, configuredProviderID: string, model: string) => Promise<WorkbenchViewModel>;
  selectPermissionMode: (current: WorkbenchViewModel, mode: string) => Promise<WorkbenchViewModel>;
  decidePermission: (current: WorkbenchViewModel, permissionID: string, action: 'allow' | 'allow_for_session' | 'deny') => Promise<WorkbenchViewModel>;
  sendPrompt: (current: WorkbenchViewModel, prompt: string) => Promise<WorkbenchViewModel>;
  cancelTurn: (current: WorkbenchViewModel, turnID?: string) => Promise<WorkbenchViewModel>;
  saveConfiguredProvider: (
    current: WorkbenchViewModel,
    provider: ConfiguredProviderViewModel & { token?: string },
  ) => Promise<WorkbenchViewModel>;
  deleteConfiguredProvider: (current: WorkbenchViewModel, providerID: string) => Promise<WorkbenchViewModel>;
  discoverConfiguredProviderModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  testConfiguredProvider: (providerID: string) => Promise<ProviderTestViewModel>;
  measureConfiguredProviderLatency: (providerID: string) => Promise<ProviderTestViewModel>;
  refreshSkills: (current: WorkbenchViewModel) => Promise<WorkbenchViewModel>;
  setSkillEnabled: (current: WorkbenchViewModel, name: string, enabled: boolean) => Promise<WorkbenchViewModel>;
  refreshMCPServer: (current: WorkbenchViewModel, name: string) => Promise<WorkbenchViewModel>;
  saveMCPServer: (current: WorkbenchViewModel, server: RuntimeMCPServerViewModel) => Promise<WorkbenchViewModel>;
  setMCPServerEnabled: (current: WorkbenchViewModel, name: string, enabled: boolean) => Promise<WorkbenchViewModel>;
  setMCPToolEnabled: (current: WorkbenchViewModel, server: string, tool: string, enabled: boolean) => Promise<WorkbenchViewModel>;
  loadMCPServerDetails: (current: WorkbenchViewModel, name: string) => Promise<WorkbenchViewModel>;
}
