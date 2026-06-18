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

export interface OpenProjectRequestViewModel {
  path: string;
  createMissing?: boolean;
}

export interface CreateProjectRequestViewModel {
  name: string;
}

export interface RenameProjectRequestViewModel {
  projectId: string;
  name: string;
}

export interface ProjectActionRequestViewModel {
  projectId: string;
}

export interface SessionViewModel {
  id: string;
  title: string;
  updatedLabel: string;
  projectId?: string;
  scope: 'project' | 'standalone';
  active?: boolean;
  busy?: boolean;
  activeTurnId?: string;
}

export interface NewConversationDraftViewModel {
  active: boolean;
  scope: 'project' | 'standalone';
  projectId?: string;
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

export interface RuntimeUserInputRequestViewModel {
  sessionId?: string;
  projectId?: string;
  scope?: 'project' | 'standalone';
  mode: 'prompt' | 'slash' | 'shell' | 'voice' | 'meta' | string;
  items: RuntimeUserInputItemViewModel[];
  options?: RuntimeUserInputOptionsViewModel;
}

export interface RuntimeUserInputItemViewModel {
  type: 'text' | 'image' | 'audio_transcript' | 'file_ref' | 'ide_selection' | 'pasted_text' | string;
  text?: string;
  data?: string;
  mimeType?: string;
  fileName?: string;
  sourcePath?: string;
  metadata?: Record<string, string>;
}

export interface RuntimeUserInputOptionsViewModel {
  isMeta?: boolean;
  skipSlashCommands?: boolean;
  bridgeOrigin?: boolean;
  voiceSource?: string;
  clientRequestId?: string;
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

export type ConversationTimelineKind = 'message' | 'thinking' | 'tool_call' | 'permission' | 'progress' | 'diagnostic';

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
  diagnostics?: TurnDiagnosticsViewModel;
}

export interface TurnDiagnosticsViewModel {
  turnId?: string;
  sessionId?: string;
  status?: string;
  startedAt?: number;
  finishedAt?: number;
  durationMs?: number;
  runningDurationMs?: number;
  computedAt?: number;
  expectedArtifacts?: string[];
  producedArtifacts?: string[];
  verifiedArtifacts?: string[];
  unverifiedArtifacts?: string[];
  missingArtifacts?: string[];
  artifactVerificationAt?: number;
  artifactCounts?: ArtifactCountsViewModel;
  artifactConfidenceSummary?: ArtifactConfidenceViewModel;
  toolCountsByStatus?: Record<string, number>;
  toolCountsByKind?: Record<string, number>;
  failedToolCount?: number;
  deniedToolCount?: number;
  cancelledToolCount?: number;
  nonzeroExitShellCount?: number;
  permissionCounts?: PermissionCountsViewModel;
  lastToolId?: string;
  lastToolStatus?: string;
  lastToolTitle?: string;
  lastRuntimeEventAt?: number;
  lastRuntimeEventSequence?: number;
  warning?: string;
  warningReason?: string;
  warningSource?: string;
}

export interface InterruptedTurnViewModel {
  turnId?: string;
  sessionId?: string;
  status?: string;
  startedAt?: number;
  interruptedAt?: number;
  durationMs?: number;
  reason?: string;
  source?: string;
  lastCompletedTool?: InterruptedToolViewModel;
  lastFailedTool?: InterruptedToolViewModel;
  pendingTool?: InterruptedToolViewModel;
  expectedArtifacts?: string[];
  producedArtifacts?: string[];
  verifiedArtifacts?: string[];
  missingArtifacts?: string[];
  artifactCounts?: ArtifactCountsViewModel;
  permissionCounts?: PermissionCountsViewModel;
  failedToolCount?: number;
  deniedToolCount?: number;
  cancelledToolCount?: number;
  nonzeroExitShellCount?: number;
  lastRuntimeEventAt?: number;
  lastRuntimeEventSequence?: number;
  summaryText?: string;
}

export interface InterruptedToolViewModel {
  id?: string;
  name?: string;
  source?: string;
  status?: string;
  startedAt?: number;
  finishedAt?: number;
  command?: string;
  workingDir?: string;
  exitCode?: number;
  target?: string;
  targets?: string[];
  stdoutExcerpt?: string;
  stderrExcerpt?: string;
  failureReason?: string;
  artifactRefs?: string[];
  diffRefs?: string[];
  display?: ToolCallDisplayViewModel;
}

export interface ArtifactCountsViewModel {
  expected?: number;
  produced?: number;
  verified?: number;
  missing?: number;
  localDeliverables?: number;
  runtimeRefs?: number;
  producedMetadataRefs?: number;
  structuredRefs?: number;
}

export interface ArtifactConfidenceViewModel {
  localVerifiedFile?: number;
  producedToolMetadata?: number;
  runtimeOutputRefs?: number;
  structuredMcpCustomRefs?: number;
  unknownNotDetected?: number;
}

export interface PermissionCountsViewModel {
  pending?: number;
  allowed?: number;
  denied?: number;
  expired?: number;
  cancelled?: number;
}

export interface RunProjectionViewModel {
  id: string;
  primarySessionId?: string;
  status?: string;
  objective?: string;
  turnCount?: number;
  taskCount?: number;
  toolCallCount?: number;
  permissionRequestCount?: number;
  waitingPermissionTurnCount?: number;
  runningTurnCount?: number;
  interruptedTurnCount?: number;
  failedTurnCount?: number;
  cancelledTurnCount?: number;
  expectedArtifactCount?: number;
  producedArtifactCount?: number;
  verifiedArtifactCount?: number;
  missingArtifactCount?: number;
  checkpointCount?: number;
  evidenceCursor?: string;
  sourceKind?: string;
  sourceReadOnly?: boolean;
  sessionActivityParity?: boolean;
  checkpoints?: RunCheckpointViewModel[];
  schedulerTaskCandidates?: RunSchedulerTaskCandidateViewModel[];
  updatedAt?: number;
  finishedAt?: number;
}

export interface ReactCallchainViewModel {
  sessionId: string;
  turnId?: string;
  nodes: ReactCallchainNodeViewModel[];
  summary: ReactCallchainSummaryViewModel;
  source: ReactCallchainSourceViewModel;
  toolResultDeliveries?: ToolResultDeliveryViewModel[];
}

export interface ToolResultDeliveryViewModel {
  toolCallId: string;
  toolResultMessageId?: string;
  deliveredToModel: boolean;
  deliveredAtStep?: number;
  synthetic?: boolean;
  reason?: string;
}

export interface ReactCallchainNodeViewModel {
  id: string;
  parentId?: string;
  kind: string;
  sessionId: string;
  turnId?: string;
  messageId?: string;
  toolCallId?: string;
  permissionId?: string;
  hookExecutionId?: string;
  sequence: number;
  status?: string;
  finishReason?: string;
  title?: string;
  summary?: string;
  error?: string;
  startedAt?: number;
  finishedAt?: number;
  evidence?: Record<string, string>;
}

export interface ReactCallchainSummaryViewModel {
  hasFinalAssistant: boolean;
  finalAssistantMessageId?: string;
  finalAssistantEmpty?: boolean;
  lastAssistantFinishReason?: string;
  toolCallCount: number;
  permissionCount: number;
  hookCount: number;
  stopReason?: string;
  stopReasonMessage?: string;
  missingEvidence: string[];
  toolResultDeliveries?: ToolResultDeliveryViewModel[];
  deliveredToolResultCount?: number;
  undeliveredToolResultCount?: number;
}

export interface ReactCallchainSourceViewModel {
  sessionActivityParity: boolean;
  usesMessages: boolean;
  usesToolCalls: boolean;
  usesPermissions: boolean;
  usesHooks: boolean;
  eventsAreRefreshOnly: boolean;
}

export interface RunSchedulerPlanRequestViewModel {
  runID: string;
  sessionID?: string;
  mode?: string;
  turnID?: string;
  checkpointID?: string;
  taskID?: string;
  cursor?: string;
  limit?: number;
}

export interface RunSchedulerTaskCandidateViewModel {
  id: string;
  runID: string;
  taskID: string;
  kind: string;
  orderKey?: string;
  sessionID?: string;
  turnID?: string;
  title?: string;
  source?: string;
  status?: string;
  executeEligible: boolean;
  disabledReason?: string;
  ownershipVerified: boolean;
  requiredPreflight: boolean;
  refreshTargets?: string[];
  cancellationScope?: string;
  diagnosticsRoute?: string;
  taskScope?: RunSchedulerTaskScopeViewModel;
}

export interface RunSchedulerTaskScopeViewModel {
  allowedTools?: string[];
  capabilityScope?: string[];
  cwd?: string;
  worktree?: string;
  role?: string;
  provider?: string;
  model?: string;
  parentToolCallID?: string;
  childSessionID?: string;
}

export interface RunCheckpointViewModel {
  id: string;
  turnId?: string;
  taskId?: string;
  status?: string;
  summary?: string;
  artifactRefs?: string[];
  createdAt?: number;
  acknowledgedAt?: number;
  discardedAt?: number;
  resumedTurnIds?: string[];
  resumeEligible?: boolean;
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
  display?: ToolCallDisplayViewModel;
  exitCode?: number;
  outputRefs?: string[];
  artifactRefs?: string[];
  diffRefs?: string[];
  startedAt?: number;
  finishedAt?: number;
}

export interface ToolCallDisplayViewModel {
  kind?: 'file_read' | 'file_write' | 'file_edit' | 'file_search' | 'shell' | 'generic' | string;
  title?: string;
  detail?: string;
  target?: string;
  primaryTarget?: string;
  targets?: string[];
  workingDir?: string;
  command?: string;
  exitCode?: number;
  durationMs?: number;
  stdoutExcerpt?: string;
  stderrExcerpt?: string;
  inputExcerpt?: string;
  outputExcerpt?: string;
  failureReason?: string;
  artifactCount?: number;
  diffCount?: number;
  artifactRefs?: string[];
  diffRefs?: string[];
  artifactSummary?: string;
  diffSummary?: string;
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
  models?: string[];
  tokenConfigured?: boolean;
  token?: string;
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

export interface ProviderDraftDiscoveryRequestViewModel {
  protocol?: string;
  apiEndpoint?: string;
  token?: string;
  defaultModel?: string;
  proxy?: string;
}

export interface TerminalViewModel {
  id: string;
  projectId?: string;
  sessionId: string;
  title?: string;
  cwd: string;
  initialCwd?: string;
  shell: string;
  shellPath?: string;
  shellArgs?: string[];
  columns?: number;
  rows?: number;
  status: string;
  exitCode?: number;
  createdAt?: number;
  updatedAt?: number;
}

export interface TerminalEventViewModel {
  terminalId: string;
  sequence: number;
  chunks?: TerminalEventChunkViewModel[];
  data?: string;
  binaryBase64?: string;
  final?: boolean;
  status?: string;
  exitCode?: number;
  error?: string;
  acknowledge?: () => void;
}

export interface TerminalEventChunkViewModel {
  data?: string;
  binaryBase64?: string;
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

export interface RuntimePluginViewModel {
  id: string;
  name: string;
  description?: string;
  category: string;
  source: string;
  kind: string;
  icon?: string;
  enabled: boolean;
  state: string;
  diagnostics?: string;
  reason?: string;
  error?: string;
  skills: string[];
  mcpServers: string[];
  toolCount: number;
  resourceCount: number;
  promptCount: number;
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
  plugins: RuntimePluginViewModel[];
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
  newConversationDraft?: NewConversationDraftViewModel;
  sidebarActions: SidebarActionViewModel[];
  conversation: ConversationMessageViewModel[];
  timeline: ConversationTimelineItemViewModel[];
  turnDiagnostics?: TurnDiagnosticsViewModel;
  interruptedTurn?: InterruptedTurnViewModel;
  runProjection?: RunProjectionViewModel;
  reactCallchain?: ReactCallchainViewModel;
  pendingPermissions: PermissionRequestViewModel[];
  composer: ComposerViewModel;
  settings: SettingsViewModel;
}

export interface RuntimeEventViewModel {
  sequence?: number;
  type?: string;
  sessionId?: string;
  turnId?: string;
  toolCallId?: string;
  createdAt?: string;
}

export interface WorkbenchAdapter {
  loadInitialViewModel: (mode?: WorkbenchMode) => Promise<WorkbenchViewModel>;
  refresh: (current: WorkbenchViewModel) => Promise<WorkbenchViewModel>;
  subscribeRuntimeEvents?: (onEvent: (event: RuntimeEventViewModel) => void) => Promise<() => void> | (() => void);
  openProject: (current: WorkbenchViewModel, request: OpenProjectRequestViewModel) => Promise<WorkbenchViewModel>;
  createProject: (current: WorkbenchViewModel, request: CreateProjectRequestViewModel) => Promise<WorkbenchViewModel>;
  renameProject: (current: WorkbenchViewModel, request: RenameProjectRequestViewModel) => Promise<WorkbenchViewModel>;
  openProjectInExplorer: (request: ProjectActionRequestViewModel) => Promise<void>;
  removeProject: (current: WorkbenchViewModel, request: ProjectActionRequestViewModel) => Promise<WorkbenchViewModel>;
  selectProjectDirectory: () => Promise<string>;
  createSession: (current: WorkbenchViewModel, target?: NewConversationDraftViewModel) => Promise<WorkbenchViewModel>;
  selectSession: (current: WorkbenchViewModel, sessionID: string) => Promise<WorkbenchViewModel>;
  renameSession: (current: WorkbenchViewModel, sessionID: string, title: string) => Promise<WorkbenchViewModel>;
  deleteSession: (current: WorkbenchViewModel, sessionID: string) => Promise<WorkbenchViewModel>;
  selectModel: (current: WorkbenchViewModel, configuredProviderID: string, model: string) => Promise<WorkbenchViewModel>;
  selectPermissionMode: (current: WorkbenchViewModel, mode: string) => Promise<WorkbenchViewModel>;
  decidePermission: (current: WorkbenchViewModel, permissionID: string, action: 'allow' | 'allow_session' | 'deny') => Promise<WorkbenchViewModel>;
  sendPrompt: (current: WorkbenchViewModel, prompt: string) => Promise<WorkbenchViewModel>;
  submitUserInput?: (current: WorkbenchViewModel, input: RuntimeUserInputRequestViewModel) => Promise<WorkbenchViewModel>;
  cancelTurn: (current: WorkbenchViewModel, turnID?: string) => Promise<WorkbenchViewModel>;
  markInterruptedDone: (current: WorkbenchViewModel, turnID: string) => Promise<WorkbenchViewModel>;
  resumeRunCheckpoint: (current: WorkbenchViewModel, runID: string, checkpointID: string) => Promise<WorkbenchViewModel>;
  readRunSchedulerPlan: (
    current: WorkbenchViewModel,
    request: RunSchedulerPlanRequestViewModel,
  ) => Promise<RunSchedulerTaskCandidateViewModel[]>;
  executeRunTask: (current: WorkbenchViewModel, runID: string, taskID: string) => Promise<WorkbenchViewModel>;
  listSessionTerminals: (sessionID: string) => Promise<TerminalViewModel[]>;
  createTerminal: (request: { sessionId: string; cwd?: string; columns?: number; rows?: number }) => Promise<TerminalViewModel>;
  writeTerminalInput: (terminalID: string, data: string) => Promise<TerminalViewModel>;
  resizeTerminal: (terminalID: string, columns: number, rows: number) => Promise<TerminalViewModel>;
  subscribeTerminalEvents: (terminalID: string, onEvent: (event: TerminalEventViewModel) => void) => Promise<() => void> | (() => void);
  deleteTerminal: (terminalID: string) => Promise<void>;
  saveConfiguredProvider: (
    current: WorkbenchViewModel,
    provider: ConfiguredProviderViewModel & { token?: string },
  ) => Promise<WorkbenchViewModel>;
  deleteConfiguredProvider: (current: WorkbenchViewModel, providerID: string) => Promise<WorkbenchViewModel>;
  discoverProviderDraftModels: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderModelDiscoveryViewModel>;
  discoverConfiguredProviderModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  testProviderDraft: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  testConfiguredProvider: (providerID: string) => Promise<ProviderTestViewModel>;
  measureProviderDraftLatency: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  measureConfiguredProviderLatency: (providerID: string) => Promise<ProviderTestViewModel>;
  refreshSkills: (current: WorkbenchViewModel) => Promise<WorkbenchViewModel>;
  setSkillEnabled: (current: WorkbenchViewModel, name: string, enabled: boolean) => Promise<WorkbenchViewModel>;
  refreshMCPServer: (current: WorkbenchViewModel, name: string) => Promise<WorkbenchViewModel>;
  saveMCPServer: (current: WorkbenchViewModel, server: RuntimeMCPServerViewModel) => Promise<WorkbenchViewModel>;
  setMCPServerEnabled: (current: WorkbenchViewModel, name: string, enabled: boolean) => Promise<WorkbenchViewModel>;
  setMCPToolEnabled: (current: WorkbenchViewModel, server: string, tool: string, enabled: boolean) => Promise<WorkbenchViewModel>;
  loadMCPServerDetails: (current: WorkbenchViewModel, name: string) => Promise<WorkbenchViewModel>;
}
