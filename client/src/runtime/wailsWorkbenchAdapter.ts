import type {
  ConfiguredProviderViewModel,
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  InterruptedTurnViewModel,
  PermissionModeOptionViewModel,
  PermissionRequestViewModel,
  ProviderModelDiscoveryViewModel,
  ProviderTestViewModel,
  ProviderCatalogItemViewModel,
  ProviderTypeViewModel,
  RuntimeMCPPromptViewModel,
  RuntimeMCPResourceViewModel,
  RuntimeMCPServerViewModel,
  RuntimeMCPToolViewModel,
  RuntimeModelOptionViewModel,
  RuntimePluginViewModel,
  RuntimeEventViewModel,
  RunProjectionViewModel,
  RunSchedulerPlanRequestViewModel,
  RunSchedulerTaskCandidateViewModel,
  RuntimeSkillViewModel,
  SettingsPermissionViewModel,
  WorkbenchAdapter,
  WorkbenchViewModel,
} from './workbenchTypes.ts';
import { getInitialWorkbenchViewModel, staticWorkbenchAdapter } from './staticWorkbenchAdapter.tsx';

interface RuntimeStatusDTO {
  sessionId?: string;
  workingDir?: string;
  model?: string;
  provider?: string;
  busy?: boolean;
  requests?: {
    activeRequestId?: string;
    sessionRequestId?: string;
    sessionBusy?: boolean;
    running?: number;
  };
}

interface RuntimeSessionDTO {
  id: string;
  title: string;
  updatedAt?: number;
  active?: boolean;
}

interface RuntimeSessionsResponseDTO {
  sessions: RuntimeSessionDTO[];
}

interface RuntimeModelsResponseDTO {
  models: Array<{
    id: string;
    name: string;
    provider: string;
    providerId?: string;
    configuredProviderId?: string;
    configuredProvider?: string;
    selected: boolean;
  }>;
}

interface RuntimeSelectedModelResponseDTO {
  selectedModel: {
    configuredProviderId: string;
    model: string;
  };
  status?: RuntimeStatusDTO;
}

interface RuntimeProviderCatalogResponseDTO {
  providerTypes?: ProviderTypeViewModel[];
  providers?: RuntimeProviderCatalogItemDTO[];
}

interface RuntimeProviderCatalogItemDTO {
  id?: string;
  name?: string;
  type?: string;
  apiEndpoint?: string;
  apiKeyTemplate?: string;
  modelCount?: number;
  defaultLargeModel?: string;
  defaultSmallModel?: string;
  requiredFields?: string[];
  notes?: string[];
  configurable?: boolean;
}

interface RuntimeConfiguredProviderDTO {
  id: string;
  providerId: string;
  name: string;
  remark?: string;
  protocol: string;
  apiEndpoint: string;
  hasApiKey?: boolean;
  proxy?: string;
  defaultModel?: string;
  enabled: boolean;
}

interface RuntimeConfiguredProviderRequestDTO {
  id?: string;
  providerId: string;
  name: string;
  remark?: string;
  protocol: string;
  apiEndpoint?: string;
  apiKey?: string;
  proxy?: string;
  defaultModel?: string;
  enabled: boolean;
}

interface RuntimeConfiguredProvidersResponseDTO {
  providers: RuntimeConfiguredProviderDTO[];
}

interface RuntimeConfiguredProviderResponseDTO {
  provider: RuntimeConfiguredProviderDTO;
}

type RuntimeProviderModelDiscoveryResponseDTO = ProviderModelDiscoveryViewModel;

type RuntimeProviderTestResponseDTO = ProviderTestViewModel;

interface RuntimeChatResponseDTO {
  requestId?: string;
  turnId?: string;
  status: RuntimeStatusDTO;
}

interface RuntimeEventsEndpointDTO {
  url: string;
  token?: string;
}

interface RuntimeEventDTO {
  sequence?: number;
  type?: string;
  created_at?: string;
  createdAt?: string;
  session_id?: string;
  sessionId?: string;
  turn_id?: string;
  turnId?: string;
  tool_call_id?: string;
  toolCallId?: string;
}

interface RuntimeEventsResponseDTO {
  events?: RuntimeEventDTO[];
}

interface RuntimeMessageDTO {
  id: string;
  role: 'user' | 'assistant' | 'tool' | 'system';
  content?: string;
  parts?: Array<{
    type: string;
    text?: string;
    content?: string;
    data?: string;
    thinking?: string;
    startedAt?: number;
    finishedAt?: number;
    message?: string;
    details?: string;
    toolCallId?: string;
    name?: string;
    input?: string;
    isError?: boolean;
  }>;
  provider?: string;
  model?: string;
  createdAt?: number;
  error?: string;
}

interface RuntimeMessagesResponseDTO {
  messages: RuntimeMessageDTO[];
}

interface RuntimeTurnResponseDTO {
  turn: {
    id: string;
    status: string;
    sessionId?: string;
    error?: string;
    latestAssistant?: RuntimeMessageDTO;
    diagnostics?: RuntimeTurnDiagnosticsDTO;
  };
}

interface RuntimeTurnDTO {
  id: string;
  sessionId: string;
  status: string;
  userMessageId?: string;
  latestAssistantMessageId?: string;
  startedAt?: number;
  finishedAt?: number;
  error?: string;
  diagnostics?: RuntimeTurnDiagnosticsDTO;
  interrupted?: RuntimeInterruptedSummaryDTO;
}

type RuntimeInterruptedSummaryDTO = InterruptedTurnViewModel;

interface RuntimeTurnDiagnosticsDTO {
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
  artifactCounts?: {
    expected?: number;
    produced?: number;
    verified?: number;
    missing?: number;
    localDeliverables?: number;
    runtimeRefs?: number;
    producedMetadataRefs?: number;
    structuredRefs?: number;
  };
  artifactConfidenceSummary?: {
    localVerifiedFile?: number;
    producedToolMetadata?: number;
    runtimeOutputRefs?: number;
    structuredMcpCustomRefs?: number;
    unknownNotDetected?: number;
  };
  toolCountsByStatus?: Record<string, number>;
  toolCountsByKind?: Record<string, number>;
  failedToolCount?: number;
  deniedToolCount?: number;
  cancelledToolCount?: number;
  nonzeroExitShellCount?: number;
  permissionCounts?: {
    pending?: number;
    allowed?: number;
    denied?: number;
    expired?: number;
    cancelled?: number;
  };
  lastToolId?: string;
  lastToolStatus?: string;
  lastToolTitle?: string;
  lastRuntimeEventAt?: number;
  lastRuntimeEventSequence?: number;
  warning?: string;
  warningReason?: string;
  warningSource?: string;
}

interface RuntimeTurnsResponseDTO {
  turns: RuntimeTurnDTO[];
}

interface RuntimeToolCallDTO {
  id: string;
  sessionId: string;
  turnId: string;
  name: string;
  source: string;
  command?: string;
  risk?: string;
  policyMode?: string;
  policyReason?: string;
  policyTargetSummary?: string;
  display?: RuntimeToolCallDisplayDTO;
  exitCode?: number;
  outputRefs?: string[];
  artifactRefs?: string[];
  diffRefs?: string[];
  status: string;
  inputSummary?: string;
  outputSummary?: string;
  stdout?: string;
  stderr?: string;
  error?: string;
  startedAt?: number;
  finishedAt?: number;
}

interface RuntimeToolCallDisplayDTO {
  kind?: string;
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

interface RuntimePermissionDTO {
  id: string;
  sessionId: string;
  turnId?: string;
  toolCallId: string;
  toolName: string;
  action: string;
  risk?: string;
  status: string;
  target?: string;
  path?: string;
  reason?: string;
  policyReason?: string;
  policyMode?: string;
  createdAt?: number;
  decidedAt?: number;
}

interface RuntimePolicyDTO {
  mode: string;
  modes?: string[];
  description?: string;
}

interface RuntimePolicyResponseDTO {
  policy: RuntimePolicyDTO;
}

interface RuntimeSessionActivityDTO {
  sessionId: string;
  messages: RuntimeMessageDTO[];
  turns: RuntimeTurnDTO[];
  toolCalls: RuntimeToolCallDTO[];
  permissions: RuntimePermissionDTO[];
  events?: RuntimeEventDTO[];
  policy: RuntimePolicyDTO;
}

interface RuntimeSessionActivityWindowDTO extends RuntimeSessionActivityDTO {
  window?: {
    limit?: number;
    cursor?: string;
    firstCursor?: string;
    lastCursor?: string;
    hasMoreBefore?: boolean;
    hasMoreAfter?: boolean;
    evidenceCount?: number;
    fromStart?: boolean;
    toEnd?: boolean;
  };
}

interface RuntimeTurnActivityDTO extends RuntimeSessionActivityDTO {
  turnId: string;
}

interface RuntimeRunProjectionRequestDTO {
  sessionId: string;
  cursor?: string;
  limit?: number;
}

interface RuntimeRunProjectionResponseDTO {
  run: {
    id: string;
    workspaceId?: string;
    primarySessionId?: string;
    sessionIds?: string[];
    objective?: string;
    status?: string;
    turnIds?: string[];
    taskIds?: string[];
    toolCallIds?: string[];
    permissionRequestIds?: string[];
    expectedArtifacts?: string[];
    producedArtifacts?: string[];
    verifiedArtifacts?: string[];
    checkpoints?: Array<{
      id?: string;
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
    }>;
    diagnostics?: {
      turnCount?: number;
      taskCount?: number;
      toolCallCount?: number;
      permissionRequestCount?: number;
      interruptedTurnCount?: number;
      failedTurnCount?: number;
      cancelledTurnCount?: number;
      runningTurnCount?: number;
      waitingPermissionTurnCount?: number;
      artifactCounts?: {
        expected?: number;
        produced?: number;
        verified?: number;
        missing?: number;
      };
    };
    evidenceCursor?: string;
    activityWindow?: RuntimeSessionActivityWindowDTO['window'];
    source?: {
      kind?: string;
      readOnly?: boolean;
      sessionActivityParity?: boolean;
      evidence?: string[];
    };
    createdAt?: number;
    updatedAt?: number;
    finishedAt?: number;
  };
}

interface RuntimeRunResumeResponseDTO {
  runId?: string;
  checkpointId?: string;
  sessionId?: string;
  turnId?: string;
}

interface RuntimeRunSchedulerExecuteTaskResponseDTO {
  accepted?: boolean;
  executionStarted?: boolean;
  reason?: string;
  refreshTargets?: string[];
}

interface RuntimeRunSchedulerPlanRequestDTO {
  runId?: string;
  sessionId?: string;
  mode?: string;
  turnId?: string;
  checkpointId?: string;
  taskId?: string;
  cursor?: string;
  limit?: number;
}

interface RuntimeRunSchedulerPlanResponseDTO {
  plan?: {
    runId?: string;
    primarySessionId?: string;
    sessionIds?: string[];
    objective?: string;
    statusFromRunDetail?: string;
    items?: RuntimeRunSchedulerPlanItemDTO[];
    cancellationScope?: string;
    diagnosticsRoute?: string;
    refreshTargets?: string[];
  };
  source?: {
    kind?: string;
    readOnly?: boolean;
    startsWorker?: boolean;
    sessionActivityParity?: boolean;
    evidence?: string[];
  };
}

interface RuntimeRunSchedulerPlanItemDTO {
  id?: string;
  kind?: string;
  orderKey?: string;
  sessionId?: string;
  turnId?: string;
  checkpointId?: string;
  taskId?: string;
  canSchedule?: boolean;
  preflightReason?: string;
  ownershipVerified?: boolean;
  requiredPreflight?: boolean;
  refreshTargets?: string[];
  cancellationScope?: string;
  diagnosticsRoute?: string;
  taskScope?: {
    allowedTools?: string[];
    capabilityScope?: string[];
    cwd?: string;
    worktree?: string;
    role?: string;
    provider?: string;
    model?: string;
    parentToolCallId?: string;
    childSessionId?: string;
  };
}

interface RuntimeSkillDTO {
  name: string;
  description?: string;
  builtin: boolean;
  enabled: boolean;
  path?: string;
  skill_file_path?: string;
  state: string;
  diagnostics?: string;
  error?: string;
  reason?: string;
  allowed_tools?: string[];
  capability_id?: string;
  policy_mode?: string;
  policy_risk?: string;
  policy_reason?: string;
}

interface RuntimeSkillsResponseDTO {
  skills: RuntimeSkillDTO[];
}

interface RuntimePluginDTO {
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
  skills?: string[];
  mcp_servers?: string[];
  tool_count?: number;
  resource_count?: number;
  prompt_count?: number;
}

interface RuntimePluginsResponseDTO {
  plugins: RuntimePluginDTO[];
}

interface RuntimeMCPCountsDTO {
  tools?: number;
  prompts?: number;
  resources?: number;
}

interface RuntimeMCPServerDTO {
  name: string;
  type: string;
  url?: string;
  command?: string;
  args?: string[];
  disabled: boolean;
  state: string;
  counts?: RuntimeMCPCountsDTO;
  diagnostics?: string;
  reason?: string;
  error?: string;
  enabled_tools?: string[];
  disabled_tools?: string[];
}

interface RuntimeMCPServersResponseDTO {
  servers: RuntimeMCPServerDTO[];
}

interface RuntimeMCPToolDTO {
  server: string;
  name: string;
  description?: string;
  enabled: boolean;
}

interface RuntimeMCPToolsResponseDTO {
  tools: RuntimeMCPToolDTO[];
}

interface RuntimeMCPResourceDTO {
  server: string;
  uri: string;
  name?: string;
  description?: string;
  mime_type?: string;
}

interface RuntimeMCPResourcesResponseDTO {
  resources: RuntimeMCPResourceDTO[];
}

interface RuntimeMCPPromptDTO {
  server: string;
  name: string;
  description?: string;
}

interface RuntimeMCPPromptsResponseDTO {
  prompts: RuntimeMCPPromptDTO[];
}

interface RuntimeBridgeModule {
  Status: () => Promise<RuntimeStatusDTO>;
  Sessions: () => Promise<RuntimeSessionsResponseDTO>;
  Models: () => Promise<RuntimeModelsResponseDTO>;
  SelectedModel?: () => Promise<RuntimeSelectedModelResponseDTO>;
  SaveSelectedModel?: (req: { configuredProviderId: string; model: string; scope?: string }) => Promise<RuntimeSelectedModelResponseDTO>;
  ProviderCatalog?: () => Promise<RuntimeProviderCatalogResponseDTO>;
  ConfiguredProviders?: () => Promise<RuntimeConfiguredProvidersResponseDTO>;
  SaveConfiguredProvider?: (req: RuntimeConfiguredProviderRequestDTO) => Promise<RuntimeConfiguredProviderResponseDTO>;
  DeleteConfiguredProvider?: (providerID: string) => Promise<RuntimeConfiguredProvidersResponseDTO>;
  DiscoverConfiguredProviderModels?: (providerID: string) => Promise<RuntimeProviderModelDiscoveryResponseDTO>;
  TestConfiguredProvider?: (providerID: string) => Promise<RuntimeProviderTestResponseDTO>;
  MeasureConfiguredProviderLatency?: (providerID: string) => Promise<RuntimeProviderTestResponseDTO>;
  NewChat: (title: string) => Promise<RuntimeStatusDTO>;
  SelectSession: (sessionID: string) => Promise<RuntimeStatusDTO>;
  RenameSession?: (req: { sessionId: string; title: string }) => Promise<RuntimeSessionsResponseDTO>;
  DeleteSession?: (sessionID: string) => Promise<RuntimeSessionsResponseDTO>;
  Chat: (req: { prompt: string; sessionId?: string }) => Promise<RuntimeChatResponseDTO>;
  CancelTurn?: (turnID: string) => Promise<RuntimeStatusDTO>;
  MarkInterruptedDone?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  Messages?: () => Promise<RuntimeMessagesResponseDTO>;
  SessionMessages?: (sessionID: string) => Promise<RuntimeMessagesResponseDTO>;
  SessionActivity?: (sessionID: string) => Promise<RuntimeSessionActivityDTO>;
  SessionActivityWindow?: (sessionID: string, limit: number) => Promise<RuntimeSessionActivityWindowDTO>;
  SessionActivityCursorWindow?: (sessionID: string, cursor: string, limit: number) => Promise<RuntimeSessionActivityWindowDTO>;
  TurnActivity?: (turnID: string) => Promise<RuntimeTurnActivityDTO>;
  RunProjection?: (req: RuntimeRunProjectionRequestDTO) => Promise<RuntimeRunProjectionResponseDTO>;
  RunSchedulerPlan?: (req: RuntimeRunSchedulerPlanRequestDTO) => Promise<RuntimeRunSchedulerPlanResponseDTO>;
  ResumeRunCheckpoint?: (runID: string, checkpointID: string) => Promise<RuntimeRunResumeResponseDTO>;
  ExecuteRunTask?: (runID: string, taskID: string) => Promise<RuntimeRunSchedulerExecuteTaskResponseDTO>;
  Turn?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  Turns?: (status: string) => Promise<RuntimeTurnsResponseDTO>;
  Permissions?: () => Promise<{ permissions: RuntimePermissionDTO[] }>;
  GetPolicy?: () => Promise<RuntimePolicyResponseDTO>;
  UpdatePolicy?: (req: { mode: string }) => Promise<RuntimePolicyResponseDTO>;
  DecidePermission?: (req: { permissionId: string; action: string }) => Promise<RuntimeStatusDTO>;
  Skills?: () => Promise<RuntimeSkillsResponseDTO>;
  Plugins?: () => Promise<RuntimePluginsResponseDTO>;
  RefreshSkills?: () => Promise<RuntimeSkillsResponseDTO>;
  SetSkillEnabled?: (req: { name: string; enabled: boolean }) => Promise<RuntimeSkillsResponseDTO>;
  MCPServers?: () => Promise<RuntimeMCPServersResponseDTO>;
  SaveMCPServer?: (req: RuntimeMCPServerDTO) => Promise<RuntimeMCPServersResponseDTO>;
  SetMCPServerEnabled?: (req: { name: string; enabled: boolean }) => Promise<RuntimeMCPServersResponseDTO>;
  RefreshMCPServer?: (name: string) => Promise<RuntimeMCPServersResponseDTO>;
  RetryMCPServer?: (name: string) => Promise<RuntimeMCPServersResponseDTO>;
  SetMCPToolEnabled?: (req: { server: string; tool: string; enabled: boolean }) => Promise<RuntimeMCPToolsResponseDTO>;
  MCPTools?: (name: string) => Promise<RuntimeMCPToolsResponseDTO>;
  MCPResources?: (name: string) => Promise<RuntimeMCPResourcesResponseDTO>;
  MCPPrompts?: (name: string) => Promise<RuntimeMCPPromptsResponseDTO>;
  EventsEndpoint?: () => Promise<RuntimeEventsEndpointDTO>;
  Events?: (after?: number) => Promise<RuntimeEventsResponseDTO>;
}

let runtimeBridgePromise: Promise<RuntimeBridgeModule | null> | undefined;
const runtimeBridgePath = '/bindings/github.com/charmbracelet/crush/desktop/runtimebridge.js';
const runtimeBridgeTimeoutMS = 750;
let runtimeLatestEventSequence = 0;
let runtimeActivityRefreshHint: RuntimeEventViewModel | undefined;
let forceDraftChatSubmit = false;

function loadRuntimeBridge() {
  if (import.meta.env.DEV && typeof window !== 'undefined') {
    return Promise.resolve(null);
  }

  // Wails generates JavaScript bindings without TypeScript declarations.
  runtimeBridgePromise ??= Promise.race([
    import(
      /* @vite-ignore */
      runtimeBridgePath
    ).then((module) => module as RuntimeBridgeModule),
    new Promise<null>((resolve) => {
      window.setTimeout(() => resolve(null), runtimeBridgeTimeoutMS);
    }),
  ]).catch(() => null);

  return runtimeBridgePromise;
}

function hasProviderSettingsBridge(bridge: RuntimeBridgeModule | null): bridge is RuntimeBridgeModule {
  return Boolean(bridge?.ProviderCatalog && bridge.ConfiguredProviders && bridge.SaveConfiguredProvider && bridge.DeleteConfiguredProvider);
}

function formatUpdatedLabel(updatedAt?: number) {
  if (!updatedAt) {
    return '';
  }

  const normalizedUpdatedAt = normalizeTimestamp(updatedAt);
  const elapsed = Date.now() - normalizedUpdatedAt;
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;

  if (elapsed < 0) {
    return '刚刚';
  }
  if (elapsed < minute) {
    return '刚刚';
  }
  if (elapsed < hour) {
    return `${Math.max(1, Math.floor(elapsed / minute))} 分钟前`;
  }
  if (elapsed < day) {
    return `${Math.floor(elapsed / hour)} 小时前`;
  }

  return `${Math.floor(elapsed / day)} 天前`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function modelOptions(modelsResponse?: RuntimeModelsResponseDTO): RuntimeModelOptionViewModel[] {
  const models = Array.isArray(modelsResponse?.models) ? modelsResponse.models : [];
  return models.map((model) => ({
    id: model.id,
    name: model.name || model.id,
    provider: model.provider,
    providerId: model.providerId,
    configuredProviderId: model.configuredProviderId,
    configuredProvider: model.configuredProvider,
    selected: model.selected,
  }));
}

function modelLabel(status?: RuntimeStatusDTO, modelsResponse?: RuntimeModelsResponseDTO) {
  const models = modelOptions(modelsResponse);
  const selectedModel = models.find((model) => model.selected);
  const model = selectedModel?.name || status?.model;

  return model || '未配置模型';
}

function mapSessions(response?: RuntimeSessionsResponseDTO, activeSessionID?: string, activeTurns?: RuntimeTurnDTO[]) {
  const sessions = Array.isArray(response?.sessions) ? response.sessions : [];
  const activeTurnBySession = new Map(
    (Array.isArray(activeTurns) ? activeTurns : [])
      .filter((turn) => turn.sessionId && !isFinalTurnStatus(turn.status))
      .map((turn) => [turn.sessionId, turn]),
  );

  return sessions.map((session) => ({
    id: session.id,
    title: session.title || '新对话',
    updatedLabel: formatUpdatedLabel(session.updatedAt),
    active: session.active || session.id === activeSessionID,
    busy: activeTurnBySession.has(session.id),
    activeTurnId: activeTurnBySession.get(session.id)?.id,
  }));
}

function isFinalTurnStatus(status?: string) {
  return ['completed', 'failed', 'cancelled', 'interrupted'].includes(status || '');
}

function mapConfiguredProviders(response?: RuntimeConfiguredProvidersResponseDTO): ConfiguredProviderViewModel[] | undefined {
  if (!Array.isArray(response?.providers)) {
    return undefined;
  }

  return response.providers.map((provider) => ({
    id: provider.id,
    providerId: provider.providerId,
    name: provider.name,
    remark: provider.remark,
    apiEndpoint: provider.apiEndpoint,
    protocol: provider.protocol,
    defaultModel: provider.defaultModel,
    tokenConfigured: provider.hasApiKey,
    proxy: provider.proxy,
    enabled: provider.enabled,
  }));
}

function mapProviderCatalogItems(response?: RuntimeProviderCatalogResponseDTO): ProviderCatalogItemViewModel[] | undefined {
  if (!Array.isArray(response?.providers)) {
    return undefined;
  }

  return response.providers.map(mapProviderCatalogItem);
}

function mapProviderCatalogItem(item: RuntimeProviderCatalogItemDTO): ProviderCatalogItemViewModel {
  return {
    id: item.id ?? '',
    name: item.name ?? item.id ?? '',
    type: item.type ?? '',
    apiEndpoint: item.apiEndpoint,
    apiKeyTemplate: item.apiKeyTemplate,
    modelCount: item.modelCount ?? 0,
    defaultLargeModel: item.defaultLargeModel,
    defaultSmallModel: item.defaultSmallModel,
    requiredFields: Array.isArray(item.requiredFields) ? item.requiredFields : [],
    notes: Array.isArray(item.notes) ? item.notes : [],
    configurable: item.configurable ?? false,
  };
}

function mapSkills(response?: RuntimeSkillsResponseDTO): RuntimeSkillViewModel[] | undefined {
  if (!Array.isArray(response?.skills)) {
    return undefined;
  }

  return response.skills.map((skill) => ({
    name: skill.name,
    description: skill.description,
    builtin: skill.builtin,
    enabled: skill.enabled,
    path: skill.path,
    skillFilePath: skill.skill_file_path,
    state: skill.state,
    diagnostics: skill.diagnostics,
    error: skill.error,
    reason: skill.reason,
    allowedTools: Array.isArray(skill.allowed_tools) ? skill.allowed_tools : [],
    capabilityId: skill.capability_id,
    policyMode: skill.policy_mode,
    policyRisk: skill.policy_risk,
    policyReason: skill.policy_reason,
  }));
}

function mapPlugins(response?: RuntimePluginsResponseDTO): RuntimePluginViewModel[] | undefined {
  if (!Array.isArray(response?.plugins)) {
    return undefined;
  }

  return response.plugins.map((plugin) => ({
    id: plugin.id,
    name: plugin.name,
    description: plugin.description,
    category: plugin.category,
    source: plugin.source,
    kind: plugin.kind,
    icon: plugin.icon,
    enabled: plugin.enabled,
    state: plugin.state,
    diagnostics: plugin.diagnostics,
    reason: plugin.reason,
    error: plugin.error,
    skills: Array.isArray(plugin.skills) ? plugin.skills : [],
    mcpServers: Array.isArray(plugin.mcp_servers) ? plugin.mcp_servers : [],
    toolCount: plugin.tool_count ?? 0,
    resourceCount: plugin.resource_count ?? 0,
    promptCount: plugin.prompt_count ?? 0,
  }));
}

function mapMCPServers(response?: RuntimeMCPServersResponseDTO): RuntimeMCPServerViewModel[] | undefined {
  if (!Array.isArray(response?.servers)) {
    return undefined;
  }

  return response.servers.map((server) => ({
    name: server.name,
    type: server.type,
    url: server.url,
    command: server.command,
    args: Array.isArray(server.args) ? server.args : [],
    disabled: server.disabled,
    enabled: !server.disabled,
    state: server.state,
    counts: {
      tools: server.counts?.tools ?? 0,
      prompts: server.counts?.prompts ?? 0,
      resources: server.counts?.resources ?? 0,
    },
    diagnostics: server.diagnostics,
    reason: server.reason,
    error: server.error,
    enabledTools: Array.isArray(server.enabled_tools) ? server.enabled_tools : [],
    disabledTools: Array.isArray(server.disabled_tools) ? server.disabled_tools : [],
  }));
}

function mapMCPTools(response?: RuntimeMCPToolsResponseDTO): RuntimeMCPToolViewModel[] {
  return (Array.isArray(response?.tools) ? response.tools : []).map((tool) => ({
    server: tool.server,
    name: tool.name,
    description: tool.description,
    enabled: tool.enabled,
  }));
}

function mapMCPResources(response?: RuntimeMCPResourcesResponseDTO): RuntimeMCPResourceViewModel[] {
  return (Array.isArray(response?.resources) ? response.resources : []).map((resource) => ({
    server: resource.server,
    uri: resource.uri,
    name: resource.name,
    description: resource.description,
    mimeType: resource.mime_type,
  }));
}

function mapMCPPrompts(response?: RuntimeMCPPromptsResponseDTO): RuntimeMCPPromptViewModel[] {
  return (Array.isArray(response?.prompts) ? response.prompts : []).map((prompt) => ({
    server: prompt.server,
    name: prompt.name,
    description: prompt.description,
  }));
}

function capabilityLabel(skills: RuntimeSkillViewModel[], servers: RuntimeMCPServerViewModel[]) {
  const enabledSkills = skills.filter((skill) => skill.enabled).length;
  const enabledServers = servers.filter((server) => server.enabled).length;
  const enabledMCPTools = servers.filter((server) => server.enabled).reduce((total, server) => total + server.counts.tools, 0);
  return `${enabledSkills} skills / ${enabledServers} MCP / ${enabledMCPTools} tools`;
}

function toMCPServerRequest(server: RuntimeMCPServerViewModel): RuntimeMCPServerDTO {
  return {
    name: server.name,
    type: server.type,
    url: server.url,
    command: server.command,
    args: server.args,
    disabled: !server.enabled,
    state: server.state,
    enabled_tools: server.enabledTools,
    disabled_tools: server.disabledTools,
  };
}

function mapConversation(response?: RuntimeMessagesResponseDTO): ConversationMessageViewModel[] {
  const messages = Array.isArray(response?.messages) ? response.messages : [];
  return messages
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .map((message) => ({
      id: message.id,
      role: message.role,
      content: runtimeMessageContent(message),
      createdAt: message.createdAt,
      provider: message.provider,
      model: message.model,
      status: message.error ? 'error' : 'success',
      error: message.error,
    }));
}

const permissionModeOptions: PermissionModeOptionViewModel[] = [
  { value: 'ask', mode: 'ask', label: '\u9ed8\u8ba4\u6a21\u5f0f', description: '\u5de5\u5177\u8c03\u7528\u6309 runtime \u89c4\u5219\u8bf7\u6c42\u5ba1\u6279\u3002' },
  { value: 'auto_read', mode: 'auto_read', label: '\u81ea\u52a8\u5ba1\u67e5', description: '\u53ea\u8bfb\u5de5\u5177\u81ea\u52a8\u6267\u884c\uff0c\u5176\u4f59\u64cd\u4f5c\u8bf7\u6c42\u5ba1\u6279\u3002' },
  {
    value: 'full_access',
    mode: 'full_access',
    label: '\u5b8c\u5168\u8bbf\u95ee\u6743\u9650',
    description: '\u5de5\u5177\u8c03\u7528\u81ea\u52a8\u6267\u884c\uff0c\u4ecd\u53d7 runtime \u5b89\u5168\u8fb9\u754c\u548c\u663e\u5f0f\u62d2\u7edd\u89c4\u5219\u7ea6\u675f\u3002',
  },
];

function permissionMode(policy?: RuntimePolicyDTO) {
  const option = permissionModeOptions.find((item) => item.mode === policy?.mode) ?? permissionModeOptions[0];
  const runtimeDescription = policy?.description?.trim();
  return {
    mode: option.mode,
    label: option.label,
    description: runtimeDescription && !isEnglishRuntimeDescription(runtimeDescription) ? runtimeDescription : option.description,
  };
}

function isEnglishRuntimeDescription(description: string) {
  return [...description].every((char) => char.charCodeAt(0) <= 127);
}

function settingsPermissions(policy?: RuntimePolicyDTO): SettingsPermissionViewModel[] {
  const selected = permissionMode(policy).mode;
  return permissionModeOptions.map((option) => ({
    key: option.mode,
    title: option.label,
    description: option.description ?? '',
    enabled: option.mode === selected,
  }));
}

function mapPermission(permission: RuntimePermissionDTO): PermissionRequestViewModel {
  return {
    id: permission.id,
    sessionId: permission.sessionId,
    turnId: permission.turnId,
    toolCallId: permission.toolCallId,
    toolName: permission.toolName,
    action: permission.action,
    risk: permission.risk,
    status: permission.status,
    target: permission.target || permission.path,
    reason: permission.reason || permission.policyReason,
    policyMode: permission.policyMode,
    createdAt: permission.createdAt,
    decidedAt: permission.decidedAt,
  };
}

function mapActivityTimeline(activity?: RuntimeSessionActivityDTO): ConversationTimelineItemViewModel[] {
  if (!activity) {
    return [];
  }
  const messagesDTO = Array.isArray(activity.messages) ? activity.messages : [];
  const toolCallsDTO = Array.isArray(activity.toolCalls) ? activity.toolCalls : [];
  const permissionsDTO = Array.isArray(activity.permissions) ? activity.permissions : [];
  const turnsDTO = Array.isArray(activity.turns) ? activity.turns : [];
  const turnContext = buildTurnContext(messagesDTO, turnsDTO);
  const messageOrder = new Map(messagesDTO.map((message, index) => [message.id, index]));
  const messages: ConversationTimelineItemViewModel[] = messagesDTO
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .flatMap((message) => {
      const timelineItems: ConversationTimelineItemViewModel[] = [];
      const content = runtimeMessageContent(message);
      if (content.trim() || message.error || message.role === 'user') {
        timelineItems.push({
          id: `message:${message.id}`,
          kind: 'message',
          sessionId: activity.sessionId,
          turnId: turnIDForMessage(turnContext, message.id),
          messageId: message.id,
          role: message.role,
          content,
          status: message.error ? 'error' : 'success',
          createdAt: message.createdAt,
          provider: message.provider,
          model: message.model,
          error: message.error,
        });
      }
      return timelineItems;
    });
  const thinking = runtimeGroupedThinkingItems(messagesDTO, activity.sessionId, turnContext);
  const toolCalls: ConversationTimelineItemViewModel[] = toolCallsDTO.map((toolCall) => ({
    id: `tool:${toolCall.id}`,
    kind: 'tool_call',
    sessionId: toolCall.sessionId,
    turnId: toolCall.turnId,
    toolCallId: toolCall.id,
    title: toolCall.name,
    status: toolCall.status,
    summary: toolCall.outputSummary || toolCall.inputSummary,
    createdAt: toolCall.startedAt,
    updatedAt: toolCall.finishedAt,
    error: toolCall.error,
    toolCall: {
      ...toolCall,
    },
  }));
  const permissions: ConversationTimelineItemViewModel[] = permissionsDTO.map((permission) => ({
    id: `permission:${permission.id}`,
    kind: 'permission',
    sessionId: permission.sessionId,
    turnId: permission.turnId,
    toolCallId: permission.toolCallId,
    title: permission.toolName,
    status: permission.status,
    summary: permission.reason || permission.policyReason,
    createdAt: permission.createdAt,
    updatedAt: permission.decidedAt,
    permission: mapPermission(permission),
  }));
  const progress: ConversationTimelineItemViewModel[] = turnsDTO
    .filter((turn) => !['completed'].includes(turn.status))
    .map((turn) => ({
      id: `turn:${turn.id}`,
      kind: 'progress',
      sessionId: turn.sessionId,
      turnId: turn.id,
      title: '运行进度',
      status: turn.status,
      summary: turn.error,
      createdAt: turn.startedAt,
      updatedAt: turn.finishedAt,
      error: turn.error,
    }));
  const diagnostics: ConversationTimelineItemViewModel[] = turnsDTO
    .filter((turn) => Boolean(turn.diagnostics?.warning))
    .map((turn) => ({
      id: `turn-diagnostics:${turn.id}`,
      kind: 'diagnostic',
      sessionId: turn.sessionId,
      turnId: turn.id,
      title: 'Turn diagnostics',
      status: 'warning',
      summary: turn.diagnostics?.warning,
      createdAt: turn.finishedAt || turn.startedAt,
      updatedAt: turn.finishedAt,
      diagnostics: turn.diagnostics,
    }));

  return [...messages, ...thinking, ...toolCalls, ...permissions, ...progress, ...diagnostics].sort((left, right) => {
    const leftTurn = left.turnId || turnIDForMessage(turnContext, left.messageId);
    const rightTurn = right.turnId || turnIDForMessage(turnContext, right.messageId);
    const leftTime = timelineSortTime(left, leftTurn, turnContext);
    const rightTime = timelineSortTime(right, rightTurn, turnContext);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    if (leftTurn && rightTurn && leftTurn === rightTurn) {
      const leftRank = timelineKindRank(left);
      const rightRank = timelineKindRank(right);
      if (leftRank !== rightRank) {
        return leftRank - rightRank;
      }
    }
    const leftMessageOrder = timelineMessageOrder(left, messageOrder);
    const rightMessageOrder = timelineMessageOrder(right, messageOrder);
    if (leftMessageOrder !== rightMessageOrder) {
      return leftMessageOrder - rightMessageOrder;
    }
    return left.id.localeCompare(right.id);
  });
}

function mergeActivityTimeline(current: ConversationTimelineItemViewModel[], activity: RuntimeSessionActivityDTO) {
  const replacement = mapActivityTimeline(activity);
  const turnIDs = new Set((Array.isArray(activity.turns) ? activity.turns : []).map((turn) => turn.id).filter(Boolean));
  const messageIDs = new Set((Array.isArray(activity.messages) ? activity.messages : []).map((message) => message.id).filter(Boolean));
  const toolCallIDs = new Set((Array.isArray(activity.toolCalls) ? activity.toolCalls : []).map((toolCall) => toolCall.id).filter(Boolean));
  const permissionIDs = new Set((Array.isArray(activity.permissions) ? activity.permissions : []).map((permission) => permission.id).filter(Boolean));
  const kept = current.filter((item) => {
    if (item.turnId && turnIDs.has(item.turnId)) {
      return false;
    }
    if (item.messageId && messageIDs.has(item.messageId)) {
      return false;
    }
    if (item.toolCallId && toolCallIDs.has(item.toolCallId)) {
      return false;
    }
    if (item.permission?.id && permissionIDs.has(item.permission.id)) {
      return false;
    }
    return true;
  });
  return [...kept, ...replacement].sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? left.updatedAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? right.updatedAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function mergeConversationMessages(current: ConversationMessageViewModel[], response?: RuntimeMessagesResponseDTO) {
  const incoming = mapConversation(response);
  if (incoming.length === 0) {
    return current;
  }
  const incomingIDs = new Set(incoming.map((message) => message.id));
  return [...current.filter((message) => !incomingIDs.has(message.id)), ...incoming].sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function mergePendingPermissions(current: PermissionRequestViewModel[], permissions: PermissionRequestViewModel[]) {
  const incomingIDs = new Set(permissions.map((permission) => permission.id));
  return [...current.filter((permission) => !incomingIDs.has(permission.id)), ...permissions].filter((permission) => permission.status === 'pending');
}

function selectTurnDiagnostics(activity?: RuntimeSessionActivityDTO, activeTurnId?: string) {
  const turns = Array.isArray(activity?.turns) ? activity.turns : [];
  if (turns.length === 0) {
    return undefined;
  }
  const selected =
    (activeTurnId ? turns.find((turn) => turn.id === activeTurnId) : undefined) ??
    turns.find((turn) => !isFinalTurnStatus(turn.status)) ??
    [...turns].sort((left, right) => {
      const leftTime = left.finishedAt || left.startedAt || 0;
      const rightTime = right.finishedAt || right.startedAt || 0;
      return rightTime - leftTime;
    })[0];
  return selected?.diagnostics;
}

function selectInterruptedTurn(activity?: RuntimeSessionActivityDTO, activeTurnId?: string): InterruptedTurnViewModel | undefined {
  const turns = Array.isArray(activity?.turns) ? activity.turns : [];
  if (turns.length === 0) {
    return undefined;
  }
  const interruptedTurns = turns.filter((turn) => turn.status === 'interrupted' && turn.interrupted?.turnId);
  if (interruptedTurns.length === 0) {
    return undefined;
  }
  const selected =
    (activeTurnId ? interruptedTurns.find((turn) => turn.id === activeTurnId) : undefined) ??
    [...interruptedTurns].sort((left, right) => {
      const leftTime = left.interrupted?.interruptedAt || left.finishedAt || left.startedAt || 0;
      const rightTime = right.interrupted?.interruptedAt || right.finishedAt || right.startedAt || 0;
      return rightTime - leftTime;
    })[0];
  return selected?.interrupted;
}

interface TimelineTurnContext {
  turnByID: Map<string, RuntimeTurnDTO>;
  turnIDByMessageID: Map<string, string>;
}

function buildTurnContext(messages: RuntimeMessageDTO[], turns: RuntimeTurnDTO[]): TimelineTurnContext {
  const turnByID = new Map(turns.map((turn) => [turn.id, turn]));
  const turnIDByMessageID = new Map<string, string>();
  const messageIndex = new Map(messages.map((message, index) => [message.id, index]));
  for (const turn of turns) {
    if (turn.userMessageId) {
      turnIDByMessageID.set(turn.userMessageId, turn.id);
    }
    if (turn.latestAssistantMessageId) {
      turnIDByMessageID.set(turn.latestAssistantMessageId, turn.id);
      if (!turn.userMessageId) {
        const assistantIndex = messageIndex.get(turn.latestAssistantMessageId);
        if (typeof assistantIndex === 'number') {
          for (let index = assistantIndex - 1; index >= 0; index -= 1) {
            const candidate = messages[index];
            if (candidate.role === 'user' && !turnIDByMessageID.has(candidate.id)) {
              turnIDByMessageID.set(candidate.id, turn.id);
              for (let relatedIndex = index + 1; relatedIndex <= assistantIndex; relatedIndex += 1) {
                const related = messages[relatedIndex];
                if (related.role === 'assistant' && !turnIDByMessageID.has(related.id)) {
                  turnIDByMessageID.set(related.id, turn.id);
                }
              }
              break;
            }
          }
        }
      }
    }
  }
  return { turnByID, turnIDByMessageID };
}

function timelineMessageOrder(item: ConversationTimelineItemViewModel, messageOrder: Map<string, number>) {
  if (!item.messageId) {
    return Number.MAX_SAFE_INTEGER;
  }
  return messageOrder.get(item.messageId) ?? Number.MAX_SAFE_INTEGER;
}

function turnIDForMessage(context: TimelineTurnContext, messageID?: string) {
  if (!messageID) {
    return undefined;
  }
  return context.turnIDByMessageID.get(messageID);
}

function timelineSortTime(item: ConversationTimelineItemViewModel, turnID: string | undefined, context: TimelineTurnContext) {
  const itemTime = normalizeTimestamp(item.createdAt ?? 0);
  if (itemTime > 0) {
    return itemTime;
  }
  if (turnID) {
    const turn = context.turnByID.get(turnID);
    if (turn?.startedAt) {
      return turn.startedAt;
    }
  }
  return 0;
}

function timelineKindRank(item: ConversationTimelineItemViewModel) {
  if (item.kind === 'message' && item.role === 'user') {
    return 0;
  }
  if (item.kind === 'thinking') {
    return 1;
  }
  if (item.kind === 'tool_call') {
    return 2;
  }
  if (item.kind === 'permission') {
    return 3;
  }
  if (item.kind === 'progress') {
    return 4;
  }
  if (item.kind === 'diagnostic') {
    return 5;
  }
  return 6;
}

function mapRunProjection(
  response?: RuntimeRunProjectionResponseDTO,
  schedulerTaskCandidates?: RunSchedulerTaskCandidateViewModel[],
): RunProjectionViewModel | undefined {
  const run = response?.run;
  if (!run?.id) {
    return undefined;
  }
  const diagnostics = run.diagnostics;
  const artifactCounts = diagnostics?.artifactCounts;
  return {
    id: run.id,
    primarySessionId: run.primarySessionId,
    status: run.status,
    objective: run.objective,
    turnCount: diagnostics?.turnCount ?? run.turnIds?.length,
    taskCount: diagnostics?.taskCount ?? run.taskIds?.length,
    toolCallCount: diagnostics?.toolCallCount ?? run.toolCallIds?.length,
    permissionRequestCount: diagnostics?.permissionRequestCount ?? run.permissionRequestIds?.length,
    waitingPermissionTurnCount: diagnostics?.waitingPermissionTurnCount,
    runningTurnCount: diagnostics?.runningTurnCount,
    interruptedTurnCount: diagnostics?.interruptedTurnCount,
    failedTurnCount: diagnostics?.failedTurnCount,
    cancelledTurnCount: diagnostics?.cancelledTurnCount,
    expectedArtifactCount: artifactCounts?.expected ?? run.expectedArtifacts?.length,
    producedArtifactCount: artifactCounts?.produced ?? run.producedArtifacts?.length,
    verifiedArtifactCount: artifactCounts?.verified ?? run.verifiedArtifacts?.length,
    missingArtifactCount: artifactCounts?.missing,
    checkpointCount: run.checkpoints?.length,
    checkpoints: run.checkpoints
      ?.filter((checkpoint) => checkpoint.id)
      .map((checkpoint) => ({
        id: checkpoint.id || '',
        turnId: checkpoint.turnId,
        taskId: checkpoint.taskId,
        status: checkpoint.status,
        summary: checkpoint.summary,
        artifactRefs: checkpoint.artifactRefs,
        createdAt: checkpoint.createdAt,
        acknowledgedAt: checkpoint.acknowledgedAt,
        discardedAt: checkpoint.discardedAt,
        resumedTurnIds: checkpoint.resumedTurnIds,
        resumeEligible: checkpoint.resumeEligible,
      })),
    evidenceCursor: run.evidenceCursor || run.activityWindow?.lastCursor,
    sourceKind: run.source?.kind,
    sourceReadOnly: run.source?.readOnly,
    sessionActivityParity: run.source?.sessionActivityParity,
    schedulerTaskCandidates,
    updatedAt: run.updatedAt,
    finishedAt: run.finishedAt,
  };
}

function mapRunSchedulerPlanCandidates(response?: RuntimeRunSchedulerPlanResponseDTO): RunSchedulerTaskCandidateViewModel[] {
  const plan = response?.plan;
  const runID = plan?.runId;
  if (!runID || !Array.isArray(plan?.items)) {
    return [];
  }
  return plan.items
    .map((item) => mapRunSchedulerPlanItem(runID, item))
    .filter((item): item is RunSchedulerTaskCandidateViewModel => Boolean(item));
}

function mapRunSchedulerPlanItem(runID: string, item: RuntimeRunSchedulerPlanItemDTO): RunSchedulerTaskCandidateViewModel | undefined {
  const taskID = item.taskId?.trim();
  if (!taskID) {
    return undefined;
  }
  const scope = item.taskScope;
  return {
    id: item.id || `task:${taskID}`,
    runID,
    taskID,
    kind: item.kind || 'task_turn',
    orderKey: item.orderKey,
    sessionID: item.sessionId,
    turnID: item.turnId,
    title: taskID,
    source: scope?.role,
    status: item.canSchedule ? 'ready' : 'blocked',
    executeEligible: item.canSchedule === true,
    disabledReason: item.canSchedule ? undefined : item.preflightReason,
    ownershipVerified: item.ownershipVerified === true,
    requiredPreflight: item.requiredPreflight !== false,
    refreshTargets: item.refreshTargets,
    cancellationScope: item.cancellationScope,
    diagnosticsRoute: item.diagnosticsRoute,
    taskScope: scope
      ? {
          allowedTools: scope.allowedTools,
          capabilityScope: scope.capabilityScope,
          cwd: scope.cwd,
          worktree: scope.worktree,
          role: scope.role,
          provider: scope.provider,
          model: scope.model,
          parentToolCallID: scope.parentToolCallId,
          childSessionID: scope.childSessionId,
        }
      : undefined,
  };
}

function toRunSchedulerPlanRequestDTO(request: RunSchedulerPlanRequestViewModel): RuntimeRunSchedulerPlanRequestDTO {
  return {
    runId: request.runID,
    sessionId: request.sessionID,
    mode: request.mode,
    turnId: request.turnID,
    checkpointId: request.checkpointID,
    taskId: request.taskID,
    cursor: request.cursor,
    limit: request.limit,
  };
}

function runtimeMessageContent(message: RuntimeMessageDTO) {
  const content = message.content || message.error || '';
  if (content.trim() || !Array.isArray(message.parts)) {
    return content;
  }

  return message.parts
    .map((part) => part.text || part.content || part.data || [part.message, part.details].filter(Boolean).join(': '))
    .filter(Boolean)
    .join('\n\n');
}

function runtimeGroupedThinkingItems(messages: RuntimeMessageDTO[], sessionId: string, turnContext: TimelineTurnContext): ConversationTimelineItemViewModel[] {
  const groups = new Map<
    string,
    {
      content: string[];
      createdAt?: number;
      updatedAt?: number;
      messageId?: string;
      provider?: string;
      model?: string;
      running: boolean;
    }
  >();

  messages.forEach((message) => {
    if (!Array.isArray(message.parts)) {
      return;
    }
    message.parts.forEach((part) => {
      if (part.type !== 'reasoning' || !part.thinking?.trim()) {
        return;
      }
      const turnId = turnIDForMessage(turnContext, message.id) || `message:${message.id}`;
      const group = groups.get(turnId) ?? {
        content: [],
        messageId: message.id,
        provider: message.provider,
        model: message.model,
        running: false,
      };
      group.content.push(part.thinking.trim());
      group.createdAt = minTimestamp(group.createdAt, part.startedAt || message.createdAt);
      group.updatedAt = maxTimestamp(group.updatedAt, part.finishedAt);
      group.running ||= !part.finishedAt;
      groups.set(turnId, group);
    });
  });

  return Array.from(groups.entries()).map(([turnId, group]) => ({
    id: `thinking:${turnId}`,
    kind: 'thinking' as const,
    sessionId,
    turnId: turnId.startsWith('message:') ? undefined : turnId,
    messageId: group.messageId,
    role: 'assistant' as const,
    title: '思考',
    content: group.content.join('\n\n'),
    status: group.running ? 'running' : 'completed',
    createdAt: group.createdAt,
    updatedAt: group.updatedAt,
    provider: group.provider,
    model: group.model,
  }));
}

function minTimestamp(left?: number, right?: number) {
  if (!left) {
    return right;
  }
  if (!right) {
    return left;
  }
  return Math.min(left, right);
}

function maxTimestamp(left?: number, right?: number) {
  if (!left) {
    return right;
  }
  if (!right) {
    return left;
  }
  return Math.max(left, right);
}

function toConfiguredProviderRequest(provider: ConfiguredProviderViewModel & { token?: string }): RuntimeConfiguredProviderRequestDTO {
  return {
    id: provider.id,
    providerId: provider.providerId,
    name: provider.name,
    remark: provider.remark,
    protocol: provider.protocol || 'openai-compat',
    apiEndpoint: provider.apiEndpoint,
    apiKey: provider.token,
    proxy: provider.proxy,
    defaultModel: provider.defaultModel,
    enabled: provider.enabled ?? true,
  };
}

async function hydrateNarrowActivityFromHint(bridge: RuntimeBridgeModule, activeSessionID: string): Promise<RuntimeSessionActivityDTO | undefined> {
  const hint = runtimeActivityRefreshHint;
  runtimeActivityRefreshHint = undefined;
  if (!hint) {
    return undefined;
  }
  if (hint.sessionId && hint.sessionId !== activeSessionID) {
    return undefined;
  }
  if (hint.turnId && bridge.TurnActivity) {
    const turnActivity = await optionalRuntimeRequest(() => bridge.TurnActivity?.(hint.turnId ?? '') ?? Promise.resolve(undefined));
    if (turnActivity?.sessionId === activeSessionID) {
      return turnActivity;
    }
  }
  if (bridge.SessionActivityCursorWindow) {
    return optionalRuntimeRequest(() => bridge.SessionActivityCursorWindow?.(activeSessionID, '', 8) ?? Promise.resolve(undefined));
  }
  if (bridge.SessionActivityWindow) {
    return optionalRuntimeRequest(() => bridge.SessionActivityWindow?.(activeSessionID, 8) ?? Promise.resolve(undefined));
  }
  return undefined;
}

async function hydrateRunSchedulerTaskCandidates(
  bridge: RuntimeBridgeModule,
  runProjection?: RuntimeRunProjectionResponseDTO,
): Promise<RunSchedulerTaskCandidateViewModel[] | undefined> {
  const runID = runProjection?.run?.id;
  const taskIDs = runProjection?.run?.taskIds?.filter((taskID) => taskID.trim());
  if (!runID || !taskIDs?.length || !bridge.RunSchedulerPlan) {
    return undefined;
  }
  const plans = await Promise.all(
    taskIDs.map((taskID) =>
      optionalRuntimeRequest(() =>
        bridge.RunSchedulerPlan?.({
          runId: runID,
          taskId: taskID,
          mode: 'task_turn',
        }) ?? Promise.resolve(undefined),
      ),
    ),
  );
  const candidates = plans.flatMap((plan) => mapRunSchedulerPlanCandidates(plan));
  const byKey = new Map<string, RunSchedulerTaskCandidateViewModel>();
  candidates.forEach((candidate) => {
    byKey.set(`${candidate.runID}:${candidate.taskID}`, candidate);
  });
  return Array.from(byKey.values()).sort((left, right) => (left.orderKey || left.taskID).localeCompare(right.orderKey || right.taskID));
}

async function hydrateWorkbench(current: WorkbenchViewModel, bridge: RuntimeBridgeModule) {
  const [status, sessionsResponse, modelsResponse, providerCatalog, configuredProvidersResponse, activeTurnsResponse, skillsResponse, pluginsResponse, mcpServersResponse] = await Promise.all([
    optionalRuntimeRequest(() => bridge.Status()),
    optionalRuntimeRequest(() => bridge.Sessions()),
    bridge.Models().catch(() => undefined),
    bridge.ProviderCatalog?.().catch(() => undefined),
    bridge.ConfiguredProviders?.().catch(() => undefined),
    optionalRuntimeRequest(() => bridge.Turns?.('active') ?? Promise.resolve(undefined)),
    optionalRuntimeRequest(() => bridge.Skills?.() ?? Promise.resolve(undefined)),
    optionalRuntimeRequest(() => bridge.Plugins?.() ?? Promise.resolve(undefined)),
    optionalRuntimeRequest(() => bridge.MCPServers?.() ?? Promise.resolve(undefined)),
  ]);
  const activeSessionID = status?.sessionId || sessionsResponse?.sessions?.find((session) => session.active)?.id;
  const narrowActivity = activeSessionID ? await hydrateNarrowActivityFromHint(bridge, activeSessionID) : undefined;
  const activity = narrowActivity ?? (activeSessionID ? await optionalRuntimeRequest(() => bridge.SessionActivity?.(activeSessionID) ?? Promise.resolve(undefined)) : undefined);
  const runProjection = activeSessionID && bridge.RunProjection
    ? await optionalRuntimeRequest(() => bridge.RunProjection?.({ sessionId: activeSessionID, limit: 24 }) ?? Promise.resolve(undefined))
    : undefined;
  const schedulerTaskCandidates = await hydrateRunSchedulerTaskCandidates(bridge, runProjection);
  const messagesResponse = activity
    ? { messages: Array.isArray(activity.messages) ? activity.messages : [] }
    : activeSessionID
      ? await optionalRuntimeRequest(() => bridge.SessionMessages?.(activeSessionID) ?? Promise.resolve(undefined))
    : await optionalRuntimeRequest(() => bridge.Messages?.() ?? Promise.resolve(undefined));
  const options = modelOptions(modelsResponse);
  const selectedModel = options.find((model) => model.selected);
  const workingDir = status?.workingDir || current.currentProject.path;
  const activeTurns = Array.isArray(activeTurnsResponse?.turns) ? activeTurnsResponse.turns : [];
  const sessionActiveTurn =
    activeTurns.find((turn) => turn.sessionId === activeSessionID && !isFinalTurnStatus(turn.status)) ||
    activity?.turns.find((turn) => !isFinalTurnStatus(turn.status));
  const busy =
    typeof status?.requests?.sessionBusy === 'boolean'
      ? status.requests.sessionBusy
      : Boolean(sessionActiveTurn);
  const activeTurnId = status?.requests?.sessionRequestId || sessionActiveTurn?.id || (busy ? current.composer.activeTurnId : undefined);
  const policy = activity?.policy ?? (await optionalRuntimeRequest(() => bridge.GetPolicy?.() ?? Promise.resolve(undefined)))?.policy;
  const permissions = (Array.isArray(activity?.permissions) ? activity.permissions : []).map(mapPermission);
  const timeline = activity
    ? narrowActivity
      ? mergeActivityTimeline(current.timeline, activity)
      : mapActivityTimeline(activity)
    : current.timeline;
  const conversation = activity && narrowActivity
    ? mergeConversationMessages(current.conversation, messagesResponse)
    : mapConversation(messagesResponse);
  const pendingPermissions = activity && narrowActivity
    ? mergePendingPermissions(current.pendingPermissions, permissions)
    : permissions.filter((permission) => permission.status === 'pending');
  const skills = mapSkills(skillsResponse) ?? current.settings.skills;
  const plugins = mapPlugins(pluginsResponse) ?? current.settings.plugins;
  const mcpServers = mapMCPServers(mcpServersResponse) ?? current.settings.mcpServers;
  const providers = mapProviderCatalogItems(providerCatalog) ?? current.settings.providers;

  return {
    ...current,
    currentProject: {
      ...current.currentProject,
      path: workingDir,
    },
    sessions: mapSessions(sessionsResponse, status?.sessionId, activeTurns),
    conversation,
    timeline,
    turnDiagnostics: activity ? selectTurnDiagnostics(activity, sessionActiveTurn?.id) : current.turnDiagnostics,
    interruptedTurn: activity ? selectInterruptedTurn(activity, sessionActiveTurn?.id) : current.interruptedTurn,
    runProjection: mapRunProjection(runProjection, schedulerTaskCandidates) ?? (current.runProjection?.primarySessionId === activeSessionID ? current.runProjection : undefined),
    pendingPermissions,
    composer: {
      ...current.composer,
      permissionLabel: permissionMode(policy).label,
      permissionMode: permissionMode(policy),
      permissionOptions: permissionModeOptions,
      modelLabel: modelLabel(status, modelsResponse),
      capabilityLabel: capabilityLabel(skills, mcpServers),
      selectedModel,
      modelOptions: options,
      busy,
      activeTurnId,
    },
    settings: {
      ...current.settings,
      permissionMode: permissionMode(policy),
      permissionOptions: permissionModeOptions,
      permissions: settingsPermissions(policy),
      providerTypes: providerCatalog?.providerTypes ?? current.settings.providerTypes,
      providers,
      configuredProviders: mapConfiguredProviders(configuredProvidersResponse) ?? current.settings.configuredProviders,
      plugins,
      skills,
      mcpServers,
      mcpToolsByServer: current.settings.mcpToolsByServer,
      mcpResourcesByServer: current.settings.mcpResourcesByServer,
      mcpPromptsByServer: current.settings.mcpPromptsByServer,
    },
  };
}

const runtimeHTTPURL = import.meta.env.DEV ? '/runtime-api' : import.meta.env.VITE_AGENT_BUILDER_RUNTIME_URL || 'http://127.0.0.1:5183';
const runtimeHTTPToken = import.meta.env.VITE_AGENT_BUILDER_RUNTIME_TOKEN || 'agent-builder-dev';
const runtimeOptionalRequestTimeoutMS = 3000;
const runtimeMutationTimeoutMS = 15000;

async function optionalRuntimeRequest<T>(request: () => Promise<T>): Promise<T | undefined> {
  return Promise.race([
    request(),
    new Promise<undefined>((resolve) => {
      window.setTimeout(() => resolve(undefined), runtimeOptionalRequestTimeoutMS);
    }),
  ]).catch(() => undefined);
}

async function runtimeRequestWithTimeout<T>(request: () => Promise<T>, timeoutMS: number, message: string): Promise<T> {
  return Promise.race([
    request(),
    new Promise<never>((_, reject) => {
      window.setTimeout(() => reject(new Error(message)), timeoutMS);
    }),
  ]);
}

interface RuntimeHTTPInit {
  body?: string;
  headers?: Record<string, string>;
  method?: string;
}

async function runtimeFetch<T>(path: string, init?: RuntimeHTTPInit): Promise<T> {
  const url = `${runtimeHTTPURL}${path}`;
  const runtimeGlobal = getRuntimeGlobal();
  const headers = {
    Authorization: `Bearer ${runtimeHTTPToken}`,
    'Content-Type': 'application/json',
    ...init?.headers,
  };

  if (typeof runtimeGlobal?.fetch !== 'function') {
    if (typeof runtimeGlobal?.XMLHttpRequest !== 'function') {
      return runtimeModule<T>(path, init);
    }
    return runtimeXHR<T>(url, {
      body: init?.body,
      headers,
      method: init?.method,
    });
  }

  const response = await runtimeGlobal.fetch(url, {
    body: init?.body,
    headers,
    method: init?.method,
  });
  if (!response.ok) {
    const detail = await runtimeHTTPErrorDetail(response);
    throw new Error(detail || `runtime HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

async function runtimeHTTPErrorDetail(response: Response) {
  try {
    const payload = (await response.json()) as { error?: string };
    return payload.error;
  } catch {
    return '';
  }
}

function getRuntimeGlobal(): (Window & typeof globalThis) | undefined {
  if (typeof globalThis !== 'undefined') {
    return globalThis as Window & typeof globalThis;
  }
  if (typeof window !== 'undefined') {
    return window as Window & typeof globalThis;
  }
  return undefined;
}

async function runtimeModule<T>(path: string, init?: RuntimeHTTPInit): Promise<T> {
  try {
    const params = new URLSearchParams({
      path,
      token: runtimeHTTPToken,
      t: String(Date.now()),
    });
    if (init?.method) {
      params.set('method', init.method);
    }
    if (init?.body) {
      params.set('body', init.body);
    }
    const module = (await import(
      /* @vite-ignore */
      `${runtimeHTTPURL}/v1/dev/module?${params.toString()}`
    )) as { default: T | { error?: string } };
    if (typeof module.default === 'object' && module.default && 'error' in module.default) {
      throw new Error(String(module.default.error));
    }
    return module.default as T;
  } catch (error) {
    console.warn('[runtime] module fallback failed', path, error);
    return runtimeJSONP<T>(path);
  }
}

function runtimeJSONP<T>(path: string): Promise<T> {
  if (typeof document === 'undefined') {
    return Promise.reject(new Error('runtime HTTP request is unavailable'));
  }
  const runtimeGlobal = getRuntimeGlobal();
  if (!runtimeGlobal) {
    return Promise.reject(new Error('runtime HTTP request is unavailable'));
  }
  return new Promise((resolve, reject) => {
    const callback = `__agentBuilderRuntimeJSONP_${Date.now()}_${Math.random().toString(36).slice(2)}`;
    const script = document.createElement('script');
    const cleanup = () => {
      delete (runtimeGlobal as unknown as Record<string, unknown>)[callback];
      if (script.parentNode) {
        script.parentNode.removeChild(script);
      }
    };
    (runtimeGlobal as unknown as Record<string, unknown>)[callback] = (value: T | { error?: string }) => {
      cleanup();
      if (typeof value === 'object' && value && 'error' in value) {
        reject(new Error(String(value.error)));
        return;
      }
      resolve(value as T);
    };
    script.onerror = () => {
      cleanup();
      reject(new Error('runtime HTTP JSONP request failed'));
    };
    script.src = `${runtimeHTTPURL}/v1/dev/jsonp?path=${encodeURIComponent(path)}&token=${encodeURIComponent(runtimeHTTPToken)}&callback=${encodeURIComponent(callback)}`;
    document.head.appendChild(script);
  });
}

function runtimeXHR<T>(
  url: string,
  init: {
    body?: string | null;
    headers: Record<string, string>;
    method?: string;
  },
): Promise<T> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open(init.method ?? 'GET', url, true);
    Object.entries(init.headers).forEach(([key, value]) => request.setRequestHeader(key, value));
    request.onload = () => {
      if (request.status < 200 || request.status >= 300) {
        reject(new Error(`runtime HTTP ${request.status}`));
        return;
      }
      try {
        resolve(JSON.parse(request.responseText) as T);
      } catch (error) {
        reject(error instanceof Error ? error : new Error('failed to decode runtime response'));
      }
    };
    request.onerror = () => reject(new Error('runtime HTTP request failed'));
    request.send(init.body ?? null);
  });
}

function runtimeHTTPEventsEndpoint(): RuntimeEventsEndpointDTO {
  return {
    url: `${runtimeHTTPURL}/v1/events`,
    token: runtimeHTTPToken,
  };
}

function runtimeEventSourceURL(endpoint: RuntimeEventsEndpointDTO, after?: number) {
  const baseURL = typeof window !== 'undefined' ? window.location.href : 'http://127.0.0.1/';
  try {
    const url = new URL(endpoint.url, baseURL);
    if (endpoint.token) {
      url.searchParams.set('token', endpoint.token);
    }
    if (after && after > 0) {
      url.searchParams.set('after', String(after));
    }
    return url.toString();
  } catch {
    const separator = endpoint.url.includes('?') ? '&' : '?';
    const params = new URLSearchParams();
    if (endpoint.token) {
      params.set('token', endpoint.token);
    }
    if (after && after > 0) {
      params.set('after', String(after));
    }
    return `${endpoint.url}${separator}${params.toString()}`;
  }
}

async function subscribeRuntimeBridgeEvents(bridge: RuntimeBridgeModule, onEvent: (event: RuntimeEventViewModel) => void) {
  if (typeof window === 'undefined' || typeof window.EventSource === 'undefined') {
    return subscribeRuntimeEventsByPolling(bridge, onEvent);
  }

  let closed = false;
  let reconnectTimer: number | undefined;
  let source: EventSource | undefined;
  let lastSequence = runtimeLatestEventSequence;

  const closeSource = () => {
    if (source) {
      source.close();
      source = undefined;
    }
  };

  const connect = async () => {
    try {
      const endpoint = bridge.EventsEndpoint ? await bridge.EventsEndpoint() : runtimeHTTPEventsEndpoint();
      if (closed) {
        return;
      }
      closeSource();
      source = new window.EventSource(runtimeEventSourceURL(endpoint, lastSequence));
      const handleMessage = (message: MessageEvent<string>) => {
        const event = parseRuntimeEventMessage(message.data);
        const nextSequence = nextRuntimeEventCursor(lastSequence, event);
        if (nextSequence > lastSequence) {
          lastSequence = nextSequence;
          runtimeLatestEventSequence = nextRuntimeEventCursor(runtimeLatestEventSequence, event);
          runtimeActivityRefreshHint = event;
          onEvent(event);
        }
      };
      source.addEventListener('runtime-event', handleMessage);
      source.onmessage = handleMessage;
      source.onerror = () => {
        closeSource();
        if (!closed) {
          reconnectTimer = window.setTimeout(connect, 2000);
        }
      };
    } catch {
      if (!closed) {
        reconnectTimer = window.setTimeout(connect, 2000);
      }
    }
  };

  await connect();

  return () => {
    closed = true;
    if (reconnectTimer) {
      window.clearTimeout(reconnectTimer);
    }
    closeSource();
  };
}

function subscribeRuntimeEventsByPolling(bridge: RuntimeBridgeModule, onEvent: (event: RuntimeEventViewModel) => void) {
  if (typeof window === 'undefined') {
    return () => undefined;
  }

  let closed = false;
  let timer: number | undefined;
  let lastSequence = runtimeLatestEventSequence;

  const poll = async () => {
    try {
      const response = bridge.Events ? await bridge.Events(lastSequence) : await runtimeFetch<RuntimeEventsResponseDTO>(runtimeEventsPath(lastSequence));
      if (closed) {
        return;
      }
      const events = Array.isArray(response.events) ? response.events : [];
      for (const event of events) {
        const viewEvent = mapRuntimeEvent(event);
        const nextSequence = nextRuntimeEventCursor(lastSequence, viewEvent);
        if (nextSequence > lastSequence) {
          lastSequence = nextSequence;
          runtimeLatestEventSequence = nextRuntimeEventCursor(runtimeLatestEventSequence, viewEvent);
          runtimeActivityRefreshHint = viewEvent;
          onEvent(viewEvent);
        }
      }
    } catch {
      // Keep polling; the existing refresh loop still covers active turns.
    } finally {
      if (!closed) {
        timer = window.setTimeout(poll, 1200);
      }
    }
  };

  timer = window.setTimeout(poll, 300);

  return () => {
    closed = true;
    if (timer) {
      window.clearTimeout(timer);
    }
  };
}

function parseRuntimeEventMessage(data: string): RuntimeEventViewModel {
  try {
    const event = JSON.parse(data) as RuntimeEventDTO;
    return mapRuntimeEvent(event);
  } catch {
    return {};
  }
}

function mapRuntimeEvent(event: RuntimeEventDTO): RuntimeEventViewModel {
  return {
    sequence: event.sequence,
    type: event.type,
    sessionId: event.sessionId ?? event.session_id,
    turnId: event.turnId ?? event.turn_id,
    toolCallId: event.toolCallId ?? event.tool_call_id,
    createdAt: event.createdAt ?? event.created_at,
  };
}

function nextRuntimeEventCursor(current: number, event: RuntimeEventViewModel) {
  return typeof event.sequence === 'number' && Number.isFinite(event.sequence) && event.sequence > current ? event.sequence : current;
}

function runtimeEventsPath(after?: number) {
  if (!after || after <= 0) {
    return '/v1/events';
  }
  return `/v1/events?after=${encodeURIComponent(String(after))}`;
}

const runtimeHTTPBridge: RuntimeBridgeModule = {
  Status: () => runtimeFetch<RuntimeStatusDTO>('/v1/runtime/status'),
  Sessions: () => runtimeFetch<RuntimeSessionsResponseDTO>('/v1/sessions'),
  Models: () => runtimeFetch<RuntimeModelsResponseDTO>('/v1/config/models'),
  SelectedModel: () => runtimeFetch<RuntimeSelectedModelResponseDTO>('/v1/config/selected-model'),
  SaveSelectedModel: (req) =>
    runtimeFetch<RuntimeSelectedModelResponseDTO>('/v1/config/selected-model', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  ProviderCatalog: () => runtimeFetch<RuntimeProviderCatalogResponseDTO>('/v1/config/providers'),
  ConfiguredProviders: () => runtimeFetch<RuntimeConfiguredProvidersResponseDTO>('/v1/config/configured-providers'),
  async SaveConfiguredProvider(req) {
    const method = req.id ? 'PUT' : 'POST';
    const path = req.id ? `/v1/config/configured-providers/${encodeURIComponent(req.id)}` : '/v1/config/configured-providers';
    return runtimeFetch<RuntimeConfiguredProviderResponseDTO>(path, {
      method,
      body: JSON.stringify(req),
    });
  },
  DeleteConfiguredProvider: (providerID) =>
    runtimeFetch<RuntimeConfiguredProvidersResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}`, {
      method: 'DELETE',
    }),
  DiscoverConfiguredProviderModels: (providerID) =>
    runtimeFetch<RuntimeProviderModelDiscoveryResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}/models`, {
      method: 'POST',
    }),
  TestConfiguredProvider: (providerID) =>
    runtimeFetch<RuntimeProviderTestResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}/test`, {
      method: 'POST',
    }),
  MeasureConfiguredProviderLatency: (providerID) =>
    runtimeFetch<RuntimeProviderTestResponseDTO>(`/v1/config/configured-providers/${encodeURIComponent(providerID)}/latency`, {
      method: 'POST',
    }),
  async NewChat(title) {
    return runtimeFetch<RuntimeStatusDTO>('/v1/sessions', {
      method: 'POST',
      body: JSON.stringify({ title }),
    });
  },
  SelectSession: (sessionID) =>
    runtimeFetch<RuntimeStatusDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/select`, {
      method: 'POST',
    }),
  RenameSession: (req) =>
    runtimeFetch<RuntimeSessionsResponseDTO>(`/v1/sessions/${encodeURIComponent(req.sessionId)}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  DeleteSession: (sessionID) =>
    runtimeFetch<RuntimeSessionsResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}`, {
      method: 'DELETE',
    }),
  SessionMessages: (sessionID) => runtimeFetch<RuntimeMessagesResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/messages`),
  SessionActivity: (sessionID) => runtimeFetch<RuntimeSessionActivityDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/activity`),
  SessionActivityWindow: (sessionID, limit) =>
    runtimeFetch<RuntimeSessionActivityWindowDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/activity-window?limit=${encodeURIComponent(String(limit))}`),
  SessionActivityCursorWindow: (sessionID, cursor, limit) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) {
      params.set('cursor', cursor);
    }
    return runtimeFetch<RuntimeSessionActivityWindowDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/activity-window?${params.toString()}`);
  },
  RunProjection: (req) => {
    const params = new URLSearchParams();
    if (typeof req.limit === 'number') {
      params.set('limit', String(req.limit));
    }
    if (req.cursor) {
      params.set('cursor', req.cursor);
    }
    const query = params.toString();
    return runtimeFetch<RuntimeRunProjectionResponseDTO>(
      `/v1/sessions/${encodeURIComponent(req.sessionId)}/run-projection${query ? `?${query}` : ''}`,
    );
  },
  RunSchedulerPlan: (req) => {
    const params = new URLSearchParams();
    if (req.runId) {
      params.set('run_id', req.runId);
    }
    if (req.sessionId) {
      params.set('session_id', req.sessionId);
    }
    if (req.mode) {
      params.set('mode', req.mode);
    }
    if (req.turnId) {
      params.set('turn_id', req.turnId);
    }
    if (req.checkpointId) {
      params.set('checkpoint_id', req.checkpointId);
    }
    if (req.taskId) {
      params.set('task_id', req.taskId);
    }
    if (req.cursor) {
      params.set('cursor', req.cursor);
    }
    if (typeof req.limit === 'number') {
      params.set('limit', String(req.limit));
    }
    const query = params.toString();
    return runtimeFetch<RuntimeRunSchedulerPlanResponseDTO>(`/v1/run-scheduler-plan${query ? `?${query}` : ''}`);
  },
  ResumeRunCheckpoint: (runID, checkpointID) =>
    runtimeFetch<RuntimeRunResumeResponseDTO>(
      `/v1/runs/${encodeURIComponent(runID)}/checkpoints/${encodeURIComponent(checkpointID)}/resume`,
      {
        method: 'POST',
      },
    ),
  ExecuteRunTask: (runID, taskID) =>
    runtimeFetch<RuntimeRunSchedulerExecuteTaskResponseDTO>(
      `/v1/runs/${encodeURIComponent(runID)}/tasks/${encodeURIComponent(taskID)}/execute`,
      {
        method: 'POST',
      },
    ),
  TurnActivity: (turnID) => runtimeFetch<RuntimeTurnActivityDTO>(`/v1/turns/${encodeURIComponent(turnID)}/activity`),
  Turn: (turnID) => runtimeFetch<RuntimeTurnResponseDTO>(`/v1/turns/${encodeURIComponent(turnID)}`),
  Turns: (status) => runtimeFetch<RuntimeTurnsResponseDTO>(`/v1/turns?status=${encodeURIComponent(status)}`),
  Permissions: () => runtimeFetch<{ permissions: RuntimePermissionDTO[] }>('/v1/permissions'),
  GetPolicy: () => runtimeFetch<RuntimePolicyResponseDTO>('/v1/policy'),
  UpdatePolicy: (req) =>
    runtimeFetch<RuntimePolicyResponseDTO>('/v1/policy', {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  DecidePermission: (req) =>
    runtimeFetch<RuntimeStatusDTO>(`/v1/permissions/${encodeURIComponent(req.permissionId)}/decision`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  Skills: () => runtimeFetch<RuntimeSkillsResponseDTO>('/v1/skills'),
  Plugins: () => runtimeFetch<RuntimePluginsResponseDTO>('/v1/plugins'),
  RefreshSkills: () =>
    runtimeFetch<RuntimeSkillsResponseDTO>('/v1/skills/refresh', {
      method: 'POST',
    }),
  SetSkillEnabled: (req) =>
    runtimeFetch<RuntimeSkillsResponseDTO>(`/v1/skills/${encodeURIComponent(req.name)}/enabled`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  MCPServers: () => runtimeFetch<RuntimeMCPServersResponseDTO>('/v1/mcp/servers'),
  SaveMCPServer: (req) =>
    runtimeFetch<RuntimeMCPServersResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(req.name)}`, {
      method: 'PUT',
      body: JSON.stringify(req),
    }),
  SetMCPServerEnabled: (req) =>
    runtimeFetch<RuntimeMCPServersResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(req.name)}/enabled`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  RefreshMCPServer: (name) =>
    runtimeFetch<RuntimeMCPServersResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(name)}/refresh`, {
      method: 'POST',
    }),
  RetryMCPServer: (name) =>
    runtimeFetch<RuntimeMCPServersResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(name)}/retry`, {
      method: 'POST',
    }),
  SetMCPToolEnabled: (req) =>
    runtimeFetch<RuntimeMCPToolsResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(req.server)}/tools/${encodeURIComponent(req.tool)}/enabled`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  MCPTools: (name) => runtimeFetch<RuntimeMCPToolsResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(name)}/tools`),
  MCPResources: (name) => runtimeFetch<RuntimeMCPResourcesResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(name)}/resources`),
  MCPPrompts: (name) => runtimeFetch<RuntimeMCPPromptsResponseDTO>(`/v1/mcp/servers/${encodeURIComponent(name)}/prompts`),
  EventsEndpoint: () => Promise.resolve(runtimeHTTPEventsEndpoint()),
  Events: (after) => runtimeFetch<RuntimeEventsResponseDTO>(runtimeEventsPath(after)),
  CancelTurn: (turnID) =>
    runtimeFetch<RuntimeStatusDTO>(`/v1/turns/${encodeURIComponent(turnID)}/cancel`, {
      method: 'POST',
    }),
  MarkInterruptedDone: (turnID) =>
    runtimeFetch<RuntimeTurnResponseDTO>(`/v1/turns/${encodeURIComponent(turnID)}/interrupted/done`, {
      method: 'POST',
    }),
  Chat: (req) => {
    if (req.sessionId) {
      return runtimeFetch<RuntimeChatResponseDTO>(`/v1/sessions/${encodeURIComponent(req.sessionId)}/turns`, {
        method: 'POST',
        body: JSON.stringify(req),
      });
    }
    return runtimeFetch<RuntimeChatResponseDTO>('/v1/turns', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },
};

async function loadRuntimeHTTPBridge() {
  try {
    await runtimeHTTPBridge.ProviderCatalog?.();
    return runtimeHTTPBridge;
  } catch (error) {
    console.warn('[runtime] provider catalog unavailable', error);
    return null;
  }
}

async function withBridge(
  run: (bridge: RuntimeBridgeModule) => Promise<WorkbenchViewModel>,
  fallback: () => Promise<WorkbenchViewModel>,
) {
  const bridge = await loadRuntimeBridge();
  if (hasProviderSettingsBridge(bridge)) {
    try {
      return await run(bridge);
    } catch (error) {
      console.warn('[runtime] wails bridge failed, trying HTTP bridge', error);
      // Continue to the HTTP runtime bridge below.
    }
  }

  const httpBridge = await loadRuntimeHTTPBridge();
  if (!httpBridge) {
    return fallback();
  }
  try {
    return await run(httpBridge);
  } catch (error) {
    console.warn('[runtime] HTTP bridge failed, using static fallback', error);
    if (httpBridge.ProviderCatalog && httpBridge.ConfiguredProviders) {
      try {
        return await hydrateSettingsOnly(await fallback(), httpBridge);
      } catch (settingsError) {
        console.warn('[runtime] provider settings fallback failed', settingsError);
      }
    }
    return fallback();
  }
}

async function hydrateSettingsOnly(current: WorkbenchViewModel, bridge: RuntimeBridgeModule) {
  const providerCatalog = await bridge.ProviderCatalog?.().catch(() => undefined);
  const configuredProvidersResponse = await bridge.ConfiguredProviders?.().catch(() => undefined);
  const skillsResponse = await bridge.Skills?.().catch(() => undefined);
  const pluginsResponse = await bridge.Plugins?.().catch(() => undefined);
  const mcpServersResponse = await bridge.MCPServers?.().catch(() => undefined);
  const providers = mapProviderCatalogItems(providerCatalog) ?? current.settings.providers;

  return {
    ...current,
    settings: {
      ...current.settings,
      providerTypes: providerCatalog?.providerTypes ?? current.settings.providerTypes,
      providers,
      configuredProviders: mapConfiguredProviders(configuredProvidersResponse) ?? current.settings.configuredProviders,
      plugins: mapPlugins(pluginsResponse) ?? current.settings.plugins,
      skills: mapSkills(skillsResponse) ?? current.settings.skills,
      mcpServers: mapMCPServers(mcpServersResponse) ?? current.settings.mcpServers,
    },
  };
}

async function hydratePluginSettings(current: WorkbenchViewModel, bridge: RuntimeBridgeModule) {
  const pluginsResponse = await bridge.Plugins?.().catch(() => undefined);
  return {
    ...current.settings,
    plugins: mapPlugins(pluginsResponse) ?? current.settings.plugins,
  };
}

export const wailsWorkbenchAdapter: WorkbenchAdapter = {
  async loadInitialViewModel(mode = 'project') {
    const initial = getInitialWorkbenchViewModel(mode);

    return withBridge(
      (bridge) => hydrateWorkbench(initial, bridge),
      () => staticWorkbenchAdapter.loadInitialViewModel(mode),
    );
  },
  async refresh(current) {
    return withBridge(
      (bridge) => hydrateWorkbench(current, bridge),
      () => staticWorkbenchAdapter.refresh(current),
    );
  },
  async subscribeRuntimeEvents(onEvent) {
    const bridge = await loadRuntimeBridge();
    if (hasProviderSettingsBridge(bridge)) {
      return subscribeRuntimeBridgeEvents(bridge, onEvent);
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (!httpBridge) {
      return () => undefined;
    }
    return subscribeRuntimeBridgeEvents(httpBridge, onEvent);
  },
  async createSession(current) {
    forceDraftChatSubmit = true;
    return withBridge(
      async (bridge) => {
        await bridge.NewChat('');
        const hydrated = await hydrateWorkbench({ ...current, mode: 'new-chat', conversation: [], timeline: [] }, bridge);
        return { ...hydrated, mode: 'new-chat' };
      },
      () => staticWorkbenchAdapter.createSession(current),
    );
  },
  async selectSession(current, sessionID) {
    forceDraftChatSubmit = false;
    return withBridge(
      async (bridge) => {
        await bridge.SelectSession(sessionID);
        const hydrated = await hydrateWorkbench({ ...current, mode: 'new-chat', conversation: [], timeline: [] }, bridge);
        return { ...hydrated, mode: 'new-chat' };
      },
      () => staticWorkbenchAdapter.selectSession(current, sessionID),
    );
  },
  async renameSession(current, sessionID, title) {
    const nextTitle = title.trim();
    return withBridge(
      async (bridge) => {
        if (!bridge.RenameSession) {
          return staticWorkbenchAdapter.renameSession(current, sessionID, nextTitle);
        }
        await bridge.RenameSession({ sessionId: sessionID, title: nextTitle });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.renameSession(current, sessionID, nextTitle),
    );
  },
  async deleteSession(current, sessionID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DeleteSession) {
          return staticWorkbenchAdapter.deleteSession(current, sessionID);
        }
        await bridge.DeleteSession(sessionID);
        const wasActive = current.sessions.some((session) => session.id === sessionID && session.active);
        const hydrated = await hydrateWorkbench(
          wasActive ? { ...current, mode: 'new-chat', conversation: [], timeline: [] } : current,
          bridge,
        );
        return { ...hydrated, mode: wasActive ? 'new-chat' : current.mode };
      },
      () => staticWorkbenchAdapter.deleteSession(current, sessionID),
    );
  },
  async selectModel(current, configuredProviderID, model) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SaveSelectedModel) {
          return staticWorkbenchAdapter.selectModel(current, configuredProviderID, model);
        }
        await bridge.SaveSelectedModel({ configuredProviderId: configuredProviderID, model, scope: 'global' });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.selectModel(current, configuredProviderID, model),
    );
  },
  async selectPermissionMode(current, mode) {
    return withBridge(
      async (bridge) => {
        if (!bridge.UpdatePolicy) {
          return current;
        }
        await bridge.UpdatePolicy({ mode });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.selectPermissionMode(current, mode),
    );
  },
  async decidePermission(current, permissionID, action) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DecidePermission) {
          return staticWorkbenchAdapter.decidePermission(current, permissionID, action);
        }
        await bridge.DecidePermission({ permissionId: permissionID, action });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.decidePermission(current, permissionID, action),
    );
  },
  async sendPrompt(current, prompt) {
    const activeSessionID = forceDraftChatSubmit ? undefined : current.sessions.find((session) => session.active)?.id;

    return withBridge(
      async (bridge) => {
        const existingLoading = current.conversation.findLast((message) => message.status === 'loading' && message.role === 'assistant');
        const hasOptimisticPrompt = current.conversation.some((message) => message.role === 'user' && message.content === prompt);
        const loadingID = existingLoading?.id ?? `loading-${Date.now()}`;
        const optimistic = {
          ...current,
          composer: { ...current.composer, busy: true },
          conversation: current.conversation.some((message) => message.id === loadingID)
            ? current.conversation
            : [
                ...current.conversation,
                ...(hasOptimisticPrompt
                  ? []
                  : [
                      {
                        id: `local-${Date.now()}`,
                        role: 'user' as const,
                        content: prompt,
                        createdAt: Date.now(),
                        status: 'success' as const,
                      },
                    ]),
                {
                  id: loadingID,
                  role: 'assistant' as const,
                  content: '正在生成回复...',
                  status: 'loading' as const,
                },
              ],
        };
        try {
          const response = await runtimeRequestWithTimeout(
            () => bridge.Chat({ prompt, sessionId: activeSessionID }),
            runtimeMutationTimeoutMS,
            '运行时响应超时，请稍后刷新会话查看结果。',
          );
          const responseSessionID = response.status.sessionId || activeSessionID;
          forceDraftChatSubmit = false;
          const busyAfterSubmit = Boolean(response.turnId);
          return {
            ...optimistic,
            mode: 'new-chat',
            sessions: responseSessionID
              ? current.sessions.map((session) => ({
                  ...session,
                  active: session.id === responseSessionID,
                  busy: session.id === responseSessionID ? busyAfterSubmit : session.busy,
                  activeTurnId: session.id === responseSessionID && busyAfterSubmit ? response.turnId : session.activeTurnId,
                }))
              : current.sessions,
            composer: {
              ...optimistic.composer,
              busy: busyAfterSubmit,
              activeTurnId: busyAfterSubmit ? response.turnId : undefined,
            },
          };
        } catch (error) {
          const message = runtimeErrorMessage(error);
          return {
            ...optimistic,
            mode: 'new-chat',
            conversation: optimistic.conversation.map((item) =>
              item.id === loadingID
                ? {
                    ...item,
                    content: message,
                    status: 'error' as const,
                    error: message,
                  }
                : item,
            ),
            composer: { ...current.composer, busy: false, activeTurnId: undefined },
          };
        }
      },
      () => staticWorkbenchAdapter.sendPrompt(current, prompt),
    );
  },
  async cancelTurn(current, turnID) {
    return withBridge(
      async (bridge) => {
        const targetTurnID = turnID || current.composer.activeTurnId;
        if (targetTurnID && bridge.CancelTurn) {
          await bridge.CancelTurn(targetTurnID);
        }
        const hydrated = await hydrateWorkbench(
          {
            ...current,
            composer: { ...current.composer, busy: false, activeTurnId: undefined },
          },
          bridge,
        );
        return {
          ...hydrated,
          composer: { ...hydrated.composer, busy: false, activeTurnId: undefined },
        };
      },
      () => staticWorkbenchAdapter.cancelTurn(current, turnID),
    );
  },
  async markInterruptedDone(current, turnID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.MarkInterruptedDone) {
          return staticWorkbenchAdapter.markInterruptedDone(current, turnID);
        }
        await bridge.MarkInterruptedDone(turnID);
        return hydrateWorkbench(
          {
            ...current,
            interruptedTurn: undefined,
          },
          bridge,
        );
      },
      () => staticWorkbenchAdapter.markInterruptedDone(current, turnID),
    );
  },
  async resumeRunCheckpoint(current, runID, checkpointID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.ResumeRunCheckpoint) {
          return staticWorkbenchAdapter.resumeRunCheckpoint(current, runID, checkpointID);
        }
        await bridge.ResumeRunCheckpoint(runID, checkpointID);
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.resumeRunCheckpoint(current, runID, checkpointID),
    );
  },
  async readRunSchedulerPlan(current, request) {
    const bridge = await loadRuntimeBridge();
    if (hasProviderSettingsBridge(bridge) && bridge.RunSchedulerPlan) {
      return mapRunSchedulerPlanCandidates(await bridge.RunSchedulerPlan(toRunSchedulerPlanRequestDTO(request)));
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.RunSchedulerPlan) {
      return mapRunSchedulerPlanCandidates(await httpBridge.RunSchedulerPlan(toRunSchedulerPlanRequestDTO(request)));
    }
    return staticWorkbenchAdapter.readRunSchedulerPlan(current, request);
  },
  async executeRunTask(current, runID, taskID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.ExecuteRunTask) {
          return staticWorkbenchAdapter.executeRunTask(current, runID, taskID);
        }
        await bridge.ExecuteRunTask(runID, taskID);
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.executeRunTask(current, runID, taskID),
    );
  },
  async saveConfiguredProvider(current, provider) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SaveConfiguredProvider) {
          return staticWorkbenchAdapter.saveConfiguredProvider(current, provider);
        }
        await bridge.SaveConfiguredProvider(toConfiguredProviderRequest(provider));
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.saveConfiguredProvider(current, provider),
    );
  },
  async deleteConfiguredProvider(current, providerID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DeleteConfiguredProvider) {
          return staticWorkbenchAdapter.deleteConfiguredProvider(current, providerID);
        }
        await bridge.DeleteConfiguredProvider(providerID);
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.deleteConfiguredProvider(current, providerID),
    );
  },
  async discoverConfiguredProviderModels(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DiscoverConfiguredProviderModels) {
      return bridge.DiscoverConfiguredProviderModels(providerID);
    }
    return runtimeHTTPBridge.DiscoverConfiguredProviderModels?.(providerID) ?? staticWorkbenchAdapter.discoverConfiguredProviderModels(providerID);
  },
  async testConfiguredProvider(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.TestConfiguredProvider) {
      return bridge.TestConfiguredProvider(providerID);
    }
    return runtimeHTTPBridge.TestConfiguredProvider?.(providerID) ?? staticWorkbenchAdapter.testConfiguredProvider(providerID);
  },
  async measureConfiguredProviderLatency(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.MeasureConfiguredProviderLatency) {
      return bridge.MeasureConfiguredProviderLatency(providerID);
    }
    return runtimeHTTPBridge.MeasureConfiguredProviderLatency?.(providerID) ?? staticWorkbenchAdapter.measureConfiguredProviderLatency(providerID);
  },
  async refreshSkills(current) {
    return withBridge(
      async (bridge) => {
        if (!bridge.RefreshSkills) {
          return staticWorkbenchAdapter.refreshSkills(current);
        }
        const response = await bridge.RefreshSkills();
        const settings = await hydratePluginSettings(current, bridge);
        return {
          ...current,
          settings: {
            ...settings,
            skills: mapSkills(response) ?? current.settings.skills,
          },
        };
      },
      () => staticWorkbenchAdapter.refreshSkills(current),
    );
  },
  async setSkillEnabled(current, name, enabled) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SetSkillEnabled) {
          return staticWorkbenchAdapter.setSkillEnabled(current, name, enabled);
        }
        const response = await bridge.SetSkillEnabled({ name, enabled });
        const settings = await hydratePluginSettings(current, bridge);
        return {
          ...current,
          settings: {
            ...settings,
            skills: mapSkills(response) ?? current.settings.skills,
          },
        };
      },
      () => staticWorkbenchAdapter.setSkillEnabled(current, name, enabled),
    );
  },
  async refreshMCPServer(current, name) {
    return withBridge(
      async (bridge) => {
        const refresh = bridge.RefreshMCPServer ?? bridge.RetryMCPServer;
        if (!refresh) {
          return staticWorkbenchAdapter.refreshMCPServer(current, name);
        }
        const response = await refresh(name);
        const settings = await hydratePluginSettings(current, bridge);
        return {
          ...current,
          settings: {
            ...settings,
            mcpServers: mapMCPServers(response) ?? current.settings.mcpServers,
          },
        };
      },
      () => staticWorkbenchAdapter.refreshMCPServer(current, name),
    );
  },
  async saveMCPServer(current, server) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SaveMCPServer) {
          return staticWorkbenchAdapter.saveMCPServer(current, server);
        }
        const response = await bridge.SaveMCPServer(toMCPServerRequest(server));
        const settings = await hydratePluginSettings(current, bridge);
        return {
          ...current,
          settings: {
            ...settings,
            mcpServers: mapMCPServers(response) ?? current.settings.mcpServers,
          },
        };
      },
      () => staticWorkbenchAdapter.saveMCPServer(current, server),
    );
  },
  async setMCPServerEnabled(current, name, enabled) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SetMCPServerEnabled) {
          return staticWorkbenchAdapter.setMCPServerEnabled(current, name, enabled);
        }
        const response = await bridge.SetMCPServerEnabled({ name, enabled });
        const settings = await hydratePluginSettings(current, bridge);
        return {
          ...current,
          settings: {
            ...settings,
            mcpServers: mapMCPServers(response) ?? current.settings.mcpServers,
          },
        };
      },
      () => staticWorkbenchAdapter.setMCPServerEnabled(current, name, enabled),
    );
  },
  async setMCPToolEnabled(current, server, tool, enabled) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SetMCPToolEnabled) {
          return staticWorkbenchAdapter.setMCPToolEnabled(current, server, tool, enabled);
        }
        const response = await bridge.SetMCPToolEnabled({ server, tool, enabled });
        return {
          ...current,
          settings: {
            ...current.settings,
            mcpToolsByServer: {
              ...current.settings.mcpToolsByServer,
              [server]: mapMCPTools(response),
            },
          },
        };
      },
      () => staticWorkbenchAdapter.setMCPToolEnabled(current, server, tool, enabled),
    );
  },
  async loadMCPServerDetails(current, name) {
    return withBridge(
      async (bridge) => {
        const [tools, resources, prompts] = await Promise.all([
          optionalRuntimeRequest(() => bridge.MCPTools?.(name) ?? Promise.resolve(undefined)),
          optionalRuntimeRequest(() => bridge.MCPResources?.(name) ?? Promise.resolve(undefined)),
          optionalRuntimeRequest(() => bridge.MCPPrompts?.(name) ?? Promise.resolve(undefined)),
        ]);
        return {
          ...current,
          settings: {
            ...current.settings,
            mcpToolsByServer: {
              ...current.settings.mcpToolsByServer,
              [name]: mapMCPTools(tools),
            },
            mcpResourcesByServer: {
              ...current.settings.mcpResourcesByServer,
              [name]: mapMCPResources(resources),
            },
            mcpPromptsByServer: {
              ...current.settings.mcpPromptsByServer,
              [name]: mapMCPPrompts(prompts),
            },
          },
        };
      },
      () => staticWorkbenchAdapter.loadMCPServerDetails(current, name),
    );
  },
};

function runtimeErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : '运行时请求失败';
}
