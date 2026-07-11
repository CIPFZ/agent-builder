import type {
  AgentTaskMessageViewModel,
  AgentTaskResultViewModel,
  AgentTaskViewModel,
  AgentRoleViewModel,
  ConfiguredProviderViewModel,
  CompactBoundaryViewModel,
  ContextDiagnosticsViewModel,
  ContextGovernanceProviderOverrideViewModel,
  ContextGovernanceSettingsViewModel,
  ContextUsageViewModel,
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  CreateProjectRequestViewModel,
  HookExecutionSummaryViewModel,
  HookExecutionViewModel,
  HookViewModel,
  RecoveryActionViewModel,
  RecoverableErrorViewModel,
  RecoveredRuntimeTurnViewModel,
  RecoveredTurnViewModel,
  RecoveryStatusViewModel,
  RuntimeMCPRequestViewModel,
  PermissionModeOptionViewModel,
  PermissionRequestViewModel,
  ProviderDraftDiscoveryRequestViewModel,
  ProviderModelDiscoveryViewModel,
  ProviderModelViewModel,
  ProviderTestViewModel,
  ProviderCatalogItemViewModel,
  ProjectActionRequestViewModel,
  ProjectMemoryCreateViewModel,
  ProjectMemoryIndexViewModel,
  ProjectMemoryListViewModel,
  ProjectMemoryRecordViewModel,
  ProjectMemoryUpdateViewModel,
  ProviderTypeViewModel,
  OpenProjectRequestViewModel,
  RenameProjectRequestViewModel,
  RuntimeMCPPromptViewModel,
  RuntimeMCPResourceViewModel,
  RuntimeMCPServerViewModel,
  RuntimeMCPToolViewModel,
  RuntimeModelOptionViewModel,
  RuntimePluginViewModel,
  ReactCallchainViewModel,
  RuntimeEventViewModel,
  RunCheckpointViewModel,
  TerminalEventViewModel,
  RunProjectionViewModel,
  RunSchedulerPlanRequestViewModel,
  RunSchedulerTaskCandidateViewModel,
  ToolCallViewModel,
  TodoItemViewModel,
  TodoSummaryViewModel,
  RuntimeUserInputRequestViewModel,
  RuntimeSkillViewModel,
  SettingsOptionViewModel,
  SettingsPermissionViewModel,
  TerminalViewModel,
  WorkbenchAdapter,
  WorkbenchViewModel,
} from './workbenchTypes.ts';
import { runtimeActionRefreshTargets, type RuntimeActionRefreshTarget, type RuntimeWriteActionResponseDTO } from './actionRefreshSelector.ts';
import { hydrateOutputStore } from './outputReducer.ts';
import { selectConversationMessages, selectConversationTimeline, selectPendingPermissions } from './outputSelectors.ts';
import { createOutputStore } from './outputStore.ts';
import type { RuntimeOutputEventsResponse, RuntimeOutputSnapshot } from './outputTypes.ts';
import { getInitialWorkbenchViewModel, staticWorkbenchAdapter } from './staticWorkbenchAdapter.tsx';

interface RuntimeStatusDTO extends RuntimeWriteActionResponseDTO {
  sessionId?: string;
  workingDir?: string;
  workspaceId?: string;
  explicitProject?: boolean;
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

interface RuntimeProjectDTO {
  id: string;
  name: string;
  path: string;
  canonicalPath?: string;
  isGitRepository?: boolean;
  branch?: string;
  current?: boolean;
  existsOnDisk?: boolean;
  createdAt?: number;
  updatedAt?: number;
  lastOpenedAt?: number;
  deletedAt?: number;
}

interface RuntimeProjectsResponseDTO {
  projects?: RuntimeProjectDTO[];
}

interface RuntimeMemoryRecordDTO {
  id: string;
  projectId: string;
  relativePath: string;
  absolutePath?: string;
  type: string;
  title: string;
  description: string;
  tags?: string[];
  enabled: boolean;
  deletedAt?: string;
  contentHash: string;
  tokenEstimate: number;
  createdAt: string;
  updatedAt: string;
  lastIndexedAt: string;
  lastInjectedAt?: string;
  preview?: string;
  content?: string;
}

interface RuntimeMemoryListResponseDTO {
  projectId: string;
  root?: string;
  records?: RuntimeMemoryRecordDTO[];
}

interface RuntimeMemoryDetailResponseDTO {
  record: RuntimeMemoryRecordDTO;
}

interface RuntimeMemoryIndexResponseDTO {
  projectId: string;
  indexed: number;
  deleted: number;
  failed: number;
  startedAt: string;
  endedAt: string;
}

interface RuntimeOpenProjectResponseDTO {
  project: RuntimeProjectDTO;
  status: RuntimeStatusDTO;
}

interface RuntimeSessionDTO {
  id: string;
  title: string;
  projectId?: string;
  scope?: 'project' | 'standalone' | string;
  updatedAt?: number;
  active?: boolean;
}

interface RuntimeSessionsResponseDTO {
  sessions: RuntimeSessionDTO[];
}

interface RuntimeSidebarProjectionResponseDTO {
  projects?: RuntimeProjectDTO[];
  sessions?: RuntimeSessionDTO[];
  currentProjectId?: string;
  activeSessionId?: string;
}

interface RuntimeSessionResponseDTO {
  session: RuntimeSessionDTO;
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

interface RuntimeHooksResponseDTO {
  hooks?: RuntimeHookDTO[];
  diagnostics?: string[];
}

interface RuntimeHookDTO {
  id?: string;
  name?: string;
  source?: string;
  event?: string;
  matcher?: string;
  command?: string;
  timeout?: number;
  timeoutMs?: number;
  timeout_ms?: number;
  enabled?: boolean;
  status?: string;
  diagnostics?: string;
  reason?: string;
}

interface RuntimeHookExecutionDTO {
  id?: string;
  hookId?: string;
  hook_id?: string;
  hookName?: string;
  hook_name?: string;
  hookSource?: string;
  hook_source?: string;
  event?: string;
  status?: string;
  sessionId?: string;
  session_id?: string;
  turnId?: string;
  turn_id?: string;
  toolCallId?: string;
  tool_call_id?: string;
  taskId?: string;
  task_id?: string;
  capabilityId?: string;
  capability_id?: string;
  mcpServer?: string;
  mcp_server?: string;
  skill?: string;
  contextRef?: string;
  context_ref?: string;
  policyMode?: string;
  policy_mode?: string;
  policyProfile?: string;
  policy_profile?: string;
  policyRule?: string;
  policy_rule?: string;
  policyDecision?: string;
  policy_decision?: string;
  policyReason?: string;
  policy_reason?: string;
  headless?: boolean;
  headlessReason?: string;
  headless_reason?: string;
  sandboxDecisionId?: string;
  sandbox_decision_id?: string;
  sandboxStatus?: string;
  sandbox_status?: string;
  scopeKind?: string;
  scope_kind?: string;
  scopeValue?: string;
  scope_value?: string;
  reason?: string;
  error?: string;
  inputSummary?: string;
  input_summary?: string;
  outputSummary?: string;
  output_summary?: string;
  contextSummary?: string;
  context_summary?: string;
  inputRewritten?: boolean;
  input_rewritten?: boolean;
  contextInjected?: boolean;
  context_injected?: boolean;
  redacted?: boolean;
  startedAt?: number;
  started_at?: number;
  completedAt?: number;
  completed_at?: number;
  durationMs?: number;
  duration_ms?: number;
}

interface RuntimeHookExecutionsRequestDTO {
  sessionId?: string;
  turnId?: string;
  toolCallId?: string;
  taskId?: string;
  event?: string;
  status?: string;
  limit?: number;
}

interface RuntimeHookExecutionsResponseDTO {
  executions?: RuntimeHookExecutionDTO[];
}

interface RuntimeHookExecutionResponseDTO {
  execution?: RuntimeHookExecutionDTO;
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
  apiKey?: string;
  hasApiKey?: boolean;
  proxy?: string;
  defaultModel?: string;
  models?: ProviderModelViewModel[];
  defaultContextWindow?: number;
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
  models?: ProviderModelViewModel[];
  defaultContextWindow?: number;
  enabled: boolean;
}

interface RuntimeConfiguredProvidersResponseDTO {
  providers: RuntimeConfiguredProviderDTO[];
}

interface RuntimeConfiguredProviderResponseDTO {
  provider: RuntimeConfiguredProviderDTO;
}

interface RuntimeModelConfigRequestDTO {
  protocol?: string;
  url?: string;
  apiKey?: string;
  model?: string;
  proxy?: string;
}

interface RuntimeModelDiscoveryResponseDTO {
  protocol?: string;
  model?: string;
  models?: string[];
  error?: string;
}

interface RuntimeModelVerifyResponseDTO {
  ok: boolean;
  protocol?: string;
  model?: string;
  models?: string[];
  error?: string;
}

type RuntimeProviderModelDiscoveryResponseDTO = ProviderModelDiscoveryViewModel;

type RuntimeProviderTestResponseDTO = ProviderTestViewModel;

interface RuntimeChatResponseDTO {
  requestId?: string;
  turnId?: string;
  status: RuntimeStatusDTO;
  normalizedInput?: RuntimeNormalizedInputDTO;
}

interface RuntimeUserInputRequestDTO {
  sessionId?: string;
  projectId?: string;
  scope?: 'project' | 'standalone' | string;
  mode: string;
  items: RuntimeUserInputItemDTO[];
  options?: RuntimeUserInputOptionsDTO;
}

interface RuntimeUserInputItemDTO {
  type: string;
  text?: string;
  data?: string;
  mimeType?: string;
  fileName?: string;
  sourcePath?: string;
  metadata?: Record<string, string>;
}

interface RuntimeUserInputOptionsDTO {
  isMeta?: boolean;
  skipSlashCommands?: boolean;
  bridgeOrigin?: boolean;
  voiceSource?: string;
  clientRequestId?: string;
}

interface RuntimeNormalizedInputDTO {
  id: string;
  sessionId?: string;
  projectId?: string;
  scope?: string;
  mode?: string;
  prompt?: string;
  shouldQuery?: boolean;
  createdAt?: number;
  command?: {
    name?: string;
    known?: boolean;
    runtime?: boolean;
    shouldQuery?: boolean;
    resultText?: string;
    strategy?: string;
    metadata?: Record<string, string>;
  };
  messages?: Array<{
    role?: string;
    content?: string;
    hidden?: boolean;
    mode?: string;
    itemTypes?: string[];
    metadata?: Record<string, string>;
    attachmentIds?: string[];
  }>;
  attachments?: Array<{
    id?: string;
    type?: string;
    mimeType?: string;
    fileName?: string;
    sourcePath?: string;
    metadata?: Record<string, string>;
    sizeBytes?: number;
  }>;
  hookOutcome?: {
    status?: string;
    preventContinuation?: boolean;
    blocking?: boolean;
    reason?: string;
    metadata?: Record<string, string>;
  };
}

interface RuntimeTerminalDTO {
  id: string;
  projectId?: string;
  sessionId?: string;
  title?: string;
  cwd: string;
  initialCwd?: string;
  shell: string;
  shellPath?: string;
  shellArgs?: string[];
  columns?: number;
  rows?: number;
  status?: string;
  exitCode?: number;
  createdAt?: number;
  updatedAt?: number;
}

interface RuntimeTerminalResponseDTO {
  terminal: RuntimeTerminalDTO;
}

interface RuntimeSessionTerminalsResponseDTO {
  sessionId: string;
  terminals?: RuntimeTerminalDTO[];
}

interface RuntimeTerminalProfileDTO {
  id: string;
  label: string;
}

interface RuntimeTerminalSettingsDTO {
  profileId?: string;
  profiles?: RuntimeTerminalProfileDTO[];
}

interface RuntimeTerminalSettingsResponseDTO {
  settings: RuntimeTerminalSettingsDTO;
}

interface RuntimeTerminalEventDTO {
  terminalId?: string;
  terminal_id?: string;
  sequence?: number;
  data?: string;
  binaryBase64?: string;
  binary_base64?: string;
  final?: boolean;
  status?: string;
  exitCode?: number;
  exit_code?: number;
  error?: string;
}

interface RuntimeTerminalStreamMessageDTO {
  type?: string;
  streamId?: string;
  terminalId?: string;
  events?: RuntimeTerminalEventDTO[];
  error?: string;
}

interface RuntimeTerminalStreamResponseDTO {
  streamId: string;
  eventName: string;
}

interface RuntimeTerminalStreamStartRequestDTO {
  terminalId: string;
  streamId: string;
  after?: number;
}

interface RuntimeTerminalStreamAckRequestDTO {
  streamId: string;
  sequence: number;
}

interface RuntimeTerminalStreamStopRequestDTO {
  streamId: string;
}

interface WailsRuntimeModule {
  Events: {
    On: (eventName: string, callback: (event: { data: unknown }) => void) => () => void;
  };
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
  updatedAt?: number;
  error?: string;
  metadata?: Record<string, string>;
  clientRequestId?: string;
  inputMode?: string;
  hidden?: boolean;
}

interface RuntimeMessagesResponseDTO {
  messages: RuntimeMessageDTO[];
}

interface RuntimeTurnResponseDTO extends RuntimeWriteActionResponseDTO {
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
}

interface RuntimeRecoveryStatusDTO {
  runtime_started_at?: string;
  last_event_sequence?: number;
  active_turns?: RuntimeTurnDTO[];
  interrupted_turns?: RuntimeRecoveredTurnDTO[];
  recoverable_errors?: RuntimeRecoverableErrorDTO[];
  compact_boundaries?: RuntimeCompactBoundaryDTO[];
  pending_permissions?: RuntimePermissionDTO[];
  pending_mcp_requests?: RuntimeMCPRequestDTO[];
  actions?: RuntimeRecoveryActionDTO[];
  snapshot_required?: boolean;
}

interface RuntimeRecoveredTurnDTO extends RuntimeTurnDTO {
  interruption_kind?: string;
  resume_eligible?: boolean;
  discard_eligible?: boolean;
  mark_done_eligible?: boolean;
  reason?: string;
  resume_hint?: string;
  open_tool_calls?: RuntimeToolCallDTO[];
  checkpoints?: RuntimeRunCheckpointDTO[];
}

interface RuntimeRecoverableErrorDTO {
  id: string;
  kind?: string;
  severity?: string;
  session_id?: string;
  turn_id?: string;
  run_id?: string;
  provider?: string;
  model?: string;
  message?: string;
  retry_eligible?: boolean;
  compact_eligible?: boolean;
  user_action?: string;
  details?: Record<string, unknown>;
  created_at?: string;
}

interface RuntimeRecoveryActionDTO {
  id: string;
  label?: string;
  kind?: string;
  session_id?: string;
  turn_id?: string;
  run_id?: string;
  checkpoint_id?: string;
  destructive?: boolean;
  starts_worker?: boolean;
  evidence?: string[];
}

interface RuntimeRecoveryRetryResponseDTO extends RuntimeWriteActionResponseDTO {
  error_id?: string;
  error?: RuntimeRecoverableErrorDTO;
  chat?: RuntimeChatResponseDTO;
}

interface RuntimeMCPRequestDTO {
  id?: string;
  sessionId?: string;
  session_id?: string;
  turnId?: string;
  turn_id?: string;
  kind?: string;
  status?: string;
  server?: string;
  tool?: string;
  prompt?: string;
  reason?: string;
  createdAt?: number;
  created_at?: number;
}

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
  stopReason?: string;
  stopReasonMessage?: string;
  hasFinalAssistant?: boolean;
  finalAssistantMessageId?: string;
  finalAssistantEmpty?: boolean;
  lastAssistantFinishReason?: string;
  missingFinalAssistant?: boolean;
  toolResultDeliveries?: RuntimeToolResultDeliveryDTO[];
  deliveredToolResultCount?: number;
  undeliveredToolResultCount?: number;
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
  messageId?: string;
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
  description?: string;
  action: string;
  risk?: string;
  status: string;
  target?: string;
  path?: string;
  reason?: string;
  policyReason?: string;
  policyMode?: string;
  policyRuleId?: string;
  policyRuleSource?: string;
  policyScopeKind?: string;
  policyScopeValue?: string;
  policyTargetSummary?: string;
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

interface RuntimeContextGovernanceModelOverrideDTO {
  autoCompactPercent?: number;
}

interface RuntimeContextGovernanceProviderOverrideDTO {
  autoCompactPercent?: number;
  models?: Record<string, RuntimeContextGovernanceModelOverrideDTO>;
}

interface RuntimeContextGovernanceSettingsDTO {
  autoCompactEnabled?: boolean;
  autoCompactPercent?: number;
  microcompactEnabled?: boolean;
  microcompactKeepRecent?: number;
  summaryModel?: string;
  providerOverrides?: Record<string, RuntimeContextGovernanceProviderOverrideDTO>;
}

interface RuntimeContextGovernanceSettingsResponseDTO {
  settings: RuntimeContextGovernanceSettingsDTO;
}

function mapContextGovernanceProviderOverride(
  override: RuntimeContextGovernanceProviderOverrideDTO,
): ContextGovernanceProviderOverrideViewModel {
  const models = override.models;
  return {
    autoCompactPercent: override.autoCompactPercent,
    models: models
      ? Object.fromEntries(Object.entries(models).map(([modelID, modelOverride]) => [modelID, { autoCompactPercent: modelOverride.autoCompactPercent }]))
      : undefined,
  };
}

function mapContextGovernanceSettings(dto?: RuntimeContextGovernanceSettingsDTO): ContextGovernanceSettingsViewModel {
  if (!dto) {
    return {};
  }
  const providerOverrides = dto.providerOverrides;
  return {
    autoCompactEnabled: dto.autoCompactEnabled,
    autoCompactPercent: dto.autoCompactPercent,
    microcompactEnabled: dto.microcompactEnabled,
    microcompactKeepRecent: dto.microcompactKeepRecent,
    summaryModel: dto.summaryModel,
    providerOverrides: providerOverrides
      ? Object.fromEntries(Object.entries(providerOverrides).map(([providerID, override]) => [providerID, mapContextGovernanceProviderOverride(override)]))
      : undefined,
  };
}

function toContextGovernanceSettingsRequest(settings: ContextGovernanceSettingsViewModel): RuntimeContextGovernanceSettingsDTO {
  return {
    autoCompactEnabled: settings.autoCompactEnabled,
    autoCompactPercent: settings.autoCompactPercent,
    microcompactEnabled: settings.microcompactEnabled,
    microcompactKeepRecent: settings.microcompactKeepRecent,
    summaryModel: settings.summaryModel,
    providerOverrides: settings.providerOverrides,
  };
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

interface RuntimeReactCallchainDTO {
  sessionId: string;
  turnId?: string;
  nodes?: RuntimeReactCallNodeDTO[];
  summary?: RuntimeReactCallSummaryDTO;
  source?: RuntimeReactCallSourceDTO;
  toolResultDeliveries?: RuntimeToolResultDeliveryDTO[];
}

interface RuntimeToolResultDeliveryDTO {
  toolCallId: string;
  toolResultMessageId?: string;
  deliveredToModel: boolean;
  deliveredAtStep?: number;
  synthetic?: boolean;
  reason?: string;
}

interface RuntimeReactCallNodeDTO {
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

interface RuntimeReactCallSummaryDTO {
  hasFinalAssistant?: boolean;
  finalAssistantMessageId?: string;
  finalAssistantEmpty?: boolean;
  lastAssistantFinishReason?: string;
  toolCallCount?: number;
  permissionCount?: number;
  hookCount?: number;
  stopReason?: string;
  stopReasonMessage?: string;
  missingEvidence?: string[];
  toolResultDeliveries?: RuntimeToolResultDeliveryDTO[];
  deliveredToolResultCount?: number;
  undeliveredToolResultCount?: number;
}

interface RuntimeReactCallSourceDTO {
  sessionActivityParity?: boolean;
  usesMessages?: boolean;
  usesToolCalls?: boolean;
  usesPermissions?: boolean;
  usesHooks?: boolean;
  eventsAreRefreshOnly?: boolean;
}

interface RuntimePromptAssembliesResponseDTO {
  assemblies?: RuntimePromptAssemblyDTO[];
}

interface RuntimeContextActionRequestDTO {
  sessionId?: string;
  turnId: string;
  projectionId?: string;
  reason?: string;
  instructions?: string;
}

interface RuntimePromptAssemblyDTO {
  id?: string;
  sessionId?: string;
  turnId?: string;
  projectionId?: string;
  step?: number;
  model?: string;
  provider?: string;
  sections?: RuntimePromptSectionDTO[];
  system?: {
    source?: string;
    hash?: string;
    tokenEstimate?: number;
    promptPrefix?: boolean;
    promptPrefixHash?: string;
    sourceRefs?: string[];
    redacted?: boolean;
  };
  messages?: {
    count?: number;
    byRole?: Record<string, number>;
    toolResultCount?: number;
    deliveredToolResults?: number;
    syntheticToolResults?: number;
    attachmentCount?: number;
    imageCount?: number;
    tokenEstimate?: number;
    rawPromptStored?: boolean;
  };
  tools?: {
    selected?: string[];
    omitted?: string[];
    selectedCount?: number;
    omittedCount?: number;
    resultCount?: number;
    persistedResults?: number;
    compactedResults?: number;
  };
  skills?: {
    availableCount?: number;
    loadedCount?: number;
    names?: string[];
    loadedNames?: string[];
    xmlPresent?: boolean;
    xmlHash?: string;
    tokenEstimate?: number;
    rawContentStored?: boolean;
  };
  mcp?: {
    serverCount?: number;
    instructionCount?: number;
    servers?: string[];
    serverListHash?: string;
    instructionHash?: string;
    tokenEstimate?: number;
    rawContentStored?: boolean;
  };
  contextSources?: RuntimeContextSourceDTO[];
  compact?: RuntimeCompactBoundaryDTO[];
  contextBoundaries?: RuntimeCompactBoundaryDTO[];
  snipBoundaries?: RuntimeSnipBoundaryDTO[];
  replacements?: RuntimeContentReplacementDTO[];
  reactiveAttempts?: RuntimeReactiveCompactAttemptDTO[];
  budget?: RuntimeBudgetReportDTO;
  createdAt?: number;
}

interface RuntimePromptSectionDTO {
  id?: string;
  name?: string;
  kind?: string;
  role?: string;
  order?: number;
  cachePolicy?: string;
  source?: string;
  sourceRefs?: string[];
  scope?: string;
  hash?: string;
  length?: number;
  tokenEstimate?: number;
  redacted?: boolean;
  rawStored?: boolean;
  diagnostics?: string;
}

interface RuntimeSnipBoundaryDTO {
  id?: string;
  removedMessageRefs?: string[];
  summaryRef?: string;
  reason?: string;
  createdAt?: number;
}

interface RuntimeContentReplacementDTO {
  id?: string;
  toolCallId?: string;
  kind?: string;
  reason?: string;
  originalRef?: string;
  createdAt?: number;
}

interface RuntimeReactiveCompactAttemptDTO {
  id?: string;
  attempt?: number;
  action?: string;
  status?: string;
  error?: string;
  createdAt?: number;
}

interface RuntimeContextSourceDTO {
  id?: string;
  kind?: string;
  name?: string;
  path?: string;
  uri?: string;
  scope?: string;
  enabled?: boolean;
  state?: string;
  reason?: string;
  diagnostics?: string;
  error?: string;
  token_estimate?: number;
  tokenEstimate?: number;
  provenance?: string;
  content_hash?: string;
  contentHash?: string;
}

interface RuntimeCompactBoundaryDTO {
  id?: string;
  kind?: string;
  trigger?: string;
  status?: string;
  summaryRef?: string;
  messageRefs?: string[];
  toolCallRefs?: unknown[];
  reinjectedRefs?: unknown[];
  error?: string;
  createdAt?: number;
  completedAt?: number;
}

interface RuntimeRunCheckpointDTO {
  id?: string;
  runId?: string;
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

interface RuntimeBudgetReportDTO {
  contextWindow?: number;
  inputBudget?: RuntimeBudgetBucketDTO;
  messages?: RuntimeBudgetBucketDTO;
  contextSources?: RuntimeBudgetBucketDTO;
  toolSchemas?: RuntimeBudgetBucketDTO;
  skills?: RuntimeBudgetBucketDTO;
  mcp?: RuntimeBudgetBucketDTO;
  toolOutputs?: RuntimeBudgetBucketDTO;
  selectedToolSchemas?: RuntimeBudgetBucketDTO;
  omittedToolSchemas?: RuntimeBudgetBucketDTO;
  totalEstimatedTokens?: number;
  updatedAt?: number;
}

interface RuntimeBudgetBucketDTO {
  count?: number;
  estimatedTokens?: number;
}

interface RuntimeContextUsageDTO {
  sessionId?: string;
  model?: string;
  contextWindow?: number;
  usedTokens?: number;
  percentUsed?: number;
  autoCompactAt?: number;
  percentLeft?: number;
  level?: string;
  estimated?: boolean;
  autoCompactEnabled?: boolean;
  outputReserve?: number;
  autoCompactBuffer?: number;
  breakdown?: RuntimeContextCategoryDTO[];
  compactCount?: number;
  updatedAt?: number;
}

interface RuntimeContextCategoryDTO {
  key?: string;
  label?: string;
  tokens?: number;
  estimated?: boolean;
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
    checkpoints?: RuntimeRunCheckpointDTO[];
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

interface RuntimeRunSummaryDTO {
  id: string;
  workspaceId?: string;
  primarySessionId?: string;
  sessionIds?: string[];
  objective?: string;
  source?: string;
  createdAt?: number;
  updatedAt?: number;
}

interface RuntimeRunSummarySourceDTO {
  kind?: string;
  readOnly?: boolean;
  summaryOnly?: boolean;
  persistedRunAuthority?: boolean;
  projectionRequiredForLifecycle?: boolean;
  excludedEvidence?: string[];
}

interface RuntimeRunSummariesResponseDTO {
  runs?: RuntimeRunSummaryDTO[];
  source?: RuntimeRunSummarySourceDTO;
}

interface RuntimeRunSummaryResponseDTO {
  run?: RuntimeRunSummaryDTO;
  source?: RuntimeRunSummarySourceDTO;
}

interface RuntimeRunCheckpointMarkerDTO {
  runId: string;
  checkpointId: string;
  turnId?: string;
  acknowledgedAt?: number;
  discardedAt?: number;
  resumedTurnIds?: string[];
}

interface RuntimeRunCheckpointMarkerSourceDTO {
  kind?: string;
  readOnly?: boolean;
  markerOnly?: boolean;
  persistedRunAuthority?: boolean;
  projectionRequiredForEligibility?: boolean;
  excludedEvidence?: string[];
}

interface RuntimeRunCheckpointMarkersResponseDTO {
  markers?: RuntimeRunCheckpointMarkerDTO[];
  source?: RuntimeRunCheckpointMarkerSourceDTO;
}

interface RuntimeRunCheckpointMarkerResponseDTO {
  marker?: RuntimeRunCheckpointMarkerDTO;
  source?: RuntimeRunCheckpointMarkerSourceDTO;
}

interface RuntimeRunResumeResponseDTO extends RuntimeWriteActionResponseDTO {
  runId?: string;
  checkpointId?: string;
  sessionId?: string;
  turnId?: string;
}

interface RuntimeRunSchedulerExecuteTaskResponseDTO extends RuntimeWriteActionResponseDTO {
  accepted?: boolean;
  executionStarted?: boolean;
  reason?: string;
  refreshTargets?: string[];
}

interface RuntimeAgentTaskDTO {
  id: string;
  parentSessionId?: string;
  parentTurnId?: string;
  parentToolCallId?: string;
  childSessionId?: string;
  title?: string;
  kind?: string;
  role?: string;
  name?: string;
  promptSummary?: string;
  model?: string;
  provider?: string;
  allowedTools?: string[];
  capabilityScope?: string[];
  cwd?: string;
  worktree?: string;
  status?: string;
  progress?: number;
  resultSummary?: string;
  artifactRefs?: string[];
  startedAt?: number;
  updatedAt?: number;
  finishedAt?: number;
  error?: string;
  cancellationDetail?: string;
  result?: RuntimeAgentTaskResultDTO;
}

interface RuntimeAgentTasksResponseDTO {
  tasks?: RuntimeAgentTaskDTO[];
}

interface RuntimeAgentTaskMessageDTO {
  id: string;
  taskId?: string;
  direction?: string;
  kind?: string;
  status?: string;
  sequence?: number;
  contentSummary?: string;
  relatedToolCallId?: string;
  relatedMessageId?: string;
  artifactRefs?: string[];
  createdAt?: number;
  deliveredAt?: number;
  processedAt?: number;
  error?: string;
}

interface RuntimeAgentTaskResultDTO {
  taskId?: string;
  status?: string;
  summary?: string;
  errorDetail?: string;
  cancellationDetail?: string;
  artifactRefs?: string[];
  relatedMessageRefs?: string[];
  relatedToolCallRefs?: string[];
  compactBoundaryRefs?: string[];
  createdAt?: number;
  updatedAt?: number;
}

interface RuntimeAgentTaskResponseDTO extends RuntimeWriteActionResponseDTO {
  task?: RuntimeAgentTaskDTO;
  messages?: RuntimeAgentTaskMessageDTO[];
  result?: RuntimeAgentTaskResultDTO;
}

interface RuntimeAgentTaskMessageResponseDTO {
  message?: RuntimeAgentTaskMessageDTO;
}

interface RuntimeAgentTaskOutputResponseDTO {
  taskId?: string;
  status?: string;
  summary?: string;
  error?: string;
  cancellationDetail?: string;
  artifactRefs?: string[];
  outputRefs?: string[];
  relatedMessageRefs?: string[];
  relatedToolCallRefs?: string[];
  compactBoundaryRefs?: string[];
  messages?: RuntimeAgentTaskMessageDTO[];
  updatedAt?: number;
}

interface RuntimeAgentRoleDTO {
  id?: string;
  name?: string;
  title?: string;
  description?: string;
  promptSummary?: string;
  allowedTools?: string[];
  capabilityScope?: string[];
  model?: string;
  provider?: string;
  cwd?: string;
  worktree?: string;
  source?: string;
}

interface RuntimeAgentRolesResponseDTO {
  roles?: RuntimeAgentRoleDTO[];
}

interface RuntimeTodoDTO {
  id?: string;
  content?: string;
  status?: string;
  activeForm?: string;
  active_form?: string;
  createdAt?: number;
  updatedAt?: number;
  source?: {
    kind?: string;
    label?: string;
    ref?: string;
  };
}

interface RuntimeTodoSummaryDTO {
  sessionId?: string;
  turnId?: string;
  todos?: RuntimeTodoDTO[];
  pending?: number;
  inProgress?: number;
  completed?: number;
  total?: number;
  updatedAt?: number;
}

interface RuntimeTodosResponseDTO {
  summary?: RuntimeTodoSummaryDTO;
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
  RecoveryStatus?: () => Promise<RuntimeRecoveryStatusDTO>;
  ResumeInterruptedTurn?: (turnID: string, req: { mode?: string; prompt?: string; metadata?: Record<string, string> }) => Promise<RuntimeTurnResponseDTO>;
  DiscardInterruptedTurn?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  RetryRecoverableError?: (errorID: string) => Promise<RuntimeRecoveryRetryResponseDTO>;
  Projects?: () => Promise<RuntimeProjectsResponseDTO>;
  SidebarProjection?: () => Promise<RuntimeSidebarProjectionResponseDTO>;
  OpenProject?: (req: OpenProjectRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  CreateProject?: (req: CreateProjectRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  RenameProject?: (req: RenameProjectRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  OpenProjectInExplorer?: (req: ProjectActionRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  RemoveProject?: (req: ProjectActionRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  ProjectMemories?: (projectID: string) => Promise<RuntimeMemoryListResponseDTO>;
  ProjectMemory?: (memoryID: string) => Promise<RuntimeMemoryDetailResponseDTO>;
  CreateProjectMemory?: (req: ProjectMemoryCreateViewModel) => Promise<RuntimeMemoryRecordDTO>;
  UpdateProjectMemory?: (memoryID: string, req: ProjectMemoryUpdateViewModel) => Promise<RuntimeMemoryRecordDTO>;
  DisableProjectMemory?: (memoryID: string, req: { enabled: boolean }) => Promise<RuntimeMemoryRecordDTO>;
  DeleteProjectMemory?: (memoryID: string, req: { reason?: string }) => Promise<RuntimeMemoryRecordDTO>;
  RefreshProjectMemoryIndex?: (projectID: string) => Promise<RuntimeMemoryIndexResponseDTO>;
  SelectProjectDirectory?: () => Promise<string>;
  Sessions: () => Promise<RuntimeSessionsResponseDTO>;
  Models: () => Promise<RuntimeModelsResponseDTO>;
  SelectedModel?: () => Promise<RuntimeSelectedModelResponseDTO>;
  SaveSelectedModel?: (req: { configuredProviderId: string; model: string; scope?: string }) => Promise<RuntimeSelectedModelResponseDTO>;
  ProviderCatalog?: () => Promise<RuntimeProviderCatalogResponseDTO>;
  ConfiguredProviders?: () => Promise<RuntimeConfiguredProvidersResponseDTO>;
  SaveConfiguredProvider?: (req: RuntimeConfiguredProviderRequestDTO) => Promise<RuntimeConfiguredProviderResponseDTO>;
  DeleteConfiguredProvider?: (providerID: string) => Promise<RuntimeConfiguredProvidersResponseDTO>;
  DiscoverModelConfig?: (req: RuntimeModelConfigRequestDTO) => Promise<RuntimeModelDiscoveryResponseDTO>;
  VerifyModelConfig?: (req: RuntimeModelConfigRequestDTO) => Promise<RuntimeModelVerifyResponseDTO>;
  DiscoverConfiguredProviderModels?: (providerID: string) => Promise<RuntimeProviderModelDiscoveryResponseDTO>;
  TestConfiguredProvider?: (providerID: string) => Promise<RuntimeProviderTestResponseDTO>;
  MeasureConfiguredProviderLatency?: (providerID: string) => Promise<RuntimeProviderTestResponseDTO>;
  NewChat: (title: string) => Promise<RuntimeStatusDTO>;
  CreateSession?: (req: { title?: string; projectId?: string; scope?: 'project' | 'standalone' }) => Promise<RuntimeSessionResponseDTO>;
  SelectSession: (sessionID: string) => Promise<RuntimeStatusDTO>;
  RenameSession?: (req: { sessionId: string; title: string }) => Promise<RuntimeSessionsResponseDTO>;
  DeleteSession?: (sessionID: string) => Promise<RuntimeSessionsResponseDTO>;
  Chat: (req: { prompt: string; sessionId?: string; projectId?: string; scope?: 'project' | 'standalone' }) => Promise<RuntimeChatResponseDTO>;
  SubmitUserInput?: (req: RuntimeUserInputRequestDTO) => Promise<RuntimeChatResponseDTO>;
  UserInput?: (inputID: string) => Promise<RuntimeNormalizedInputDTO>;
  SessionTerminals?: (sessionID: string) => Promise<RuntimeSessionTerminalsResponseDTO>;
  CreateTerminal?: (req: { sessionId: string; id?: string; cwd?: string; profileId?: string; columns?: number; rows?: number }) => Promise<RuntimeTerminalResponseDTO>;
  TerminalSettings?: () => Promise<RuntimeTerminalSettingsResponseDTO>;
  SaveTerminalSettings?: (req: { profileId: string }) => Promise<RuntimeTerminalSettingsResponseDTO>;
  WriteTerminalInput?: (terminalID: string, req: { data?: string; binaryBase64?: string }) => Promise<RuntimeTerminalResponseDTO>;
  ResizeTerminal?: (terminalID: string, req: { columns: number; rows: number }) => Promise<RuntimeTerminalResponseDTO>;
  StartTerminalEventStream?: (req: RuntimeTerminalStreamStartRequestDTO) => Promise<RuntimeTerminalStreamResponseDTO>;
  AckTerminalEventStream?: (req: RuntimeTerminalStreamAckRequestDTO) => Promise<boolean>;
  StopTerminalEventStream?: (req: RuntimeTerminalStreamStopRequestDTO) => Promise<boolean>;
  DeleteTerminal?: (terminalID: string) => Promise<RuntimeTerminalResponseDTO>;
  CancelTurn?: (turnID: string) => Promise<RuntimeStatusDTO>;
  MarkInterruptedDone?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  Messages?: () => Promise<RuntimeMessagesResponseDTO>;
  SessionMessages?: (sessionID: string) => Promise<RuntimeMessagesResponseDTO>;
  SessionContextUsage?: (sessionID: string) => Promise<RuntimeContextUsageDTO>;
  SessionOutput?: (sessionID: string, req: { snapshot?: boolean; cursor?: string; limit?: number }) => Promise<RuntimeOutputSnapshot>;
  SessionOutputEvents?: (sessionID: string, after: string) => Promise<RuntimeOutputEventsResponse>;
  StartSessionOutputStream?: (req: { sessionId: string; streamId?: string; after?: string }) => Promise<{ streamId: string; eventName: string }>;
  StopSessionOutputStream?: (req: { streamId: string }) => Promise<boolean>;
  SessionActivity?: (sessionID: string) => Promise<RuntimeSessionActivityDTO>;
  SessionActivityWindow?: (sessionID: string, limit: number) => Promise<RuntimeSessionActivityWindowDTO>;
  SessionActivityCursorWindow?: (sessionID: string, cursor: string, limit: number) => Promise<RuntimeSessionActivityWindowDTO>;
  TurnActivity?: (turnID: string) => Promise<RuntimeTurnActivityDTO>;
  ReactCallchain?: (turnID: string) => Promise<RuntimeReactCallchainDTO>;
  SessionReactCallchain?: (sessionID: string, limit: number) => Promise<RuntimeReactCallchainDTO>;
  TurnPromptAssemblies?: (turnID: string) => Promise<RuntimePromptAssembliesResponseDTO>;
  SessionPromptAssemblies?: (sessionID: string, limit: number) => Promise<RuntimePromptAssembliesResponseDTO>;
  Hooks?: () => Promise<RuntimeHooksResponseDTO>;
  HookExecutions?: (req: RuntimeHookExecutionsRequestDTO) => Promise<RuntimeHookExecutionsResponseDTO>;
  HookExecution?: (executionID: string) => Promise<RuntimeHookExecutionResponseDTO>;
  ManualCompact?: (req: RuntimeContextActionRequestDTO) => Promise<unknown>;
  ManualSnip?: (req: RuntimeContextActionRequestDTO) => Promise<unknown>;
  RunProjection?: (req: RuntimeRunProjectionRequestDTO) => Promise<RuntimeRunProjectionResponseDTO>;
  RunSummaries?: () => Promise<RuntimeRunSummariesResponseDTO>;
  RunSummary?: (runID: string) => Promise<RuntimeRunSummaryResponseDTO>;
  RunCheckpointMarkers?: (runID: string) => Promise<RuntimeRunCheckpointMarkersResponseDTO>;
  RunCheckpointMarker?: (runID: string, checkpointID: string) => Promise<RuntimeRunCheckpointMarkerResponseDTO>;
  RunSchedulerPlan?: (req: RuntimeRunSchedulerPlanRequestDTO) => Promise<RuntimeRunSchedulerPlanResponseDTO>;
  ResumeRunCheckpoint?: (runID: string, checkpointID: string) => Promise<RuntimeRunResumeResponseDTO>;
  ExecuteRunTask?: (runID: string, taskID: string) => Promise<RuntimeRunSchedulerExecuteTaskResponseDTO>;
  SessionAgentTasks?: (sessionID: string) => Promise<RuntimeAgentTasksResponseDTO>;
  SessionTodos?: (sessionID: string) => Promise<RuntimeTodosResponseDTO>;
  TurnTodos?: (turnID: string) => Promise<RuntimeTodosResponseDTO>;
  AgentTask?: (taskID: string) => Promise<RuntimeAgentTaskResponseDTO>;
  AgentRoles?: () => Promise<RuntimeAgentRolesResponseDTO>;
  AgentTaskFollowUp?: (taskID: string, req: { direction: string; kind: string; contentSummary: string }) => Promise<RuntimeAgentTaskMessageResponseDTO>;
  CancelAgentTask?: (taskID: string) => Promise<RuntimeAgentTaskResponseDTO>;
  AgentTaskOutput?: (taskID: string) => Promise<RuntimeAgentTaskOutputResponseDTO>;
  Turn?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  Turns?: (status: string) => Promise<RuntimeTurnsResponseDTO>;
  Permissions?: () => Promise<{ permissions: RuntimePermissionDTO[] }>;
  GetPolicy?: () => Promise<RuntimePolicyResponseDTO>;
  UpdatePolicy?: (req: { mode: string }) => Promise<RuntimePolicyResponseDTO>;
  ContextGovernanceSettings?: () => Promise<RuntimeContextGovernanceSettingsResponseDTO>;
  SaveContextGovernanceSettings?: (req: RuntimeContextGovernanceSettingsDTO) => Promise<RuntimeContextGovernanceSettingsResponseDTO>;
  DecidePermission?: (req: { permissionId: string; action: string; guidance?: string }) => Promise<RuntimeStatusDTO>;
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
const runtimeBridgePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const wailsRuntimePath = '/wails/runtime.js';
const runtimeBridgeTimeoutMS = 750;
let runtimeLatestEventSequence = 0;
let runtimeActivityRefreshHint: RuntimeEventViewModel | undefined;
let forceDraftChatSubmit = false;
let wailsRuntimePromise: Promise<WailsRuntimeModule> | undefined;

function loadRuntimeBridge() {
  if (typeof window === 'undefined') {
    return Promise.resolve(null);
  }

  // Wails dev also serves generated bindings through Vite. Try them first so
  // the desktop WebView does not depend on the standalone HTTP runtime.
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

function loadWailsRuntime() {
  wailsRuntimePromise ??= import(
    /* @vite-ignore */
    wailsRuntimePath
  ).then((module) => module as WailsRuntimeModule);
  return wailsRuntimePromise;
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
    projectId: session.scope === 'standalone' ? undefined : session.projectId,
    scope: session.scope === 'standalone' ? 'standalone' as const : 'project' as const,
    updatedLabel: formatUpdatedLabel(session.updatedAt),
    active: session.active || session.id === activeSessionID,
    busy: activeTurnBySession.has(session.id),
    activeTurnId: activeTurnBySession.get(session.id)?.id,
  }));
}

function mapProjects(response?: RuntimeProjectsResponseDTO, currentProjectID?: string): WorkbenchViewModel['projects'] {
  const projects = Array.isArray(response?.projects) ? response.projects : [];
  return projects
    .filter((project) => project.id && project.path)
    .map((project) => ({
      id: project.id,
      name: project.name || projectNameFromPath(project.path),
      path: project.path,
      canonicalPath: project.canonicalPath,
      isGitRepository: Boolean(project.isGitRepository),
      branch: project.branch,
      current: Boolean(project.current || (currentProjectID && project.id === currentProjectID)),
      existsOnDisk: project.existsOnDisk ?? true,
      createdAt: project.createdAt,
      updatedAt: project.updatedAt,
      lastOpenedAt: project.lastOpenedAt,
      deletedAt: project.deletedAt,
    }));
}

function mapProjectFromStatus(status?: RuntimeStatusDTO, current?: WorkbenchViewModel['currentProject']): WorkbenchViewModel['currentProject'] {
  if (!status?.explicitProject) {
    return {
      id: '',
      name: '',
      path: '',
      isGitRepository: false,
      branch: undefined,
      current: false,
    };
  }
  const path = status?.workingDir || current?.path || '';
  const id = status?.workspaceId || current?.id || '';
  return {
    id,
    name: path ? projectNameFromPath(path) : (current?.name ?? ''),
    path,
    isGitRepository: current?.path === path ? Boolean(current?.isGitRepository) : false,
    branch: current?.path === path ? current?.branch : undefined,
    current: Boolean(path),
  };
}

function defaultDraftTarget(current: WorkbenchViewModel): NonNullable<WorkbenchViewModel['newConversationDraft']> {
  if (current.currentProject.id) {
    return { active: true, scope: 'project', projectId: current.currentProject.id };
  }
  return { active: true, scope: 'standalone' };
}

function optimisticDraftSessionTitle(prompt: string) {
  const title = prompt.trim().replace(/\s+/g, ' ');
  return title.length > 32 ? `${title.slice(0, 32)}...` : title || 'New chat';
}

function sessionsAfterDraftSubmit(
  current: WorkbenchViewModel,
  sessionID: string | undefined,
  prompt: string,
  draftTarget: WorkbenchViewModel['newConversationDraft'],
  turnID: string | undefined,
) {
  if (!sessionID) {
    return current.sessions;
  }
  const busyAfterSubmit = Boolean(turnID);
  const existing = current.sessions.find((session) => session.id === sessionID);
  if (existing) {
    return current.sessions.map((session) => ({
      ...session,
      active: session.id === sessionID,
      busy: session.id === sessionID ? busyAfterSubmit : session.busy,
      activeTurnId: session.id === sessionID && busyAfterSubmit ? turnID : session.activeTurnId,
    }));
  }
  const scope = draftTarget?.scope === 'standalone' ? 'standalone' as const : 'project' as const;
  const projectId = scope === 'project' ? (draftTarget?.projectId || current.currentProject.id) : undefined;
  return [
    {
      id: sessionID,
      title: optimisticDraftSessionTitle(prompt),
      updatedLabel: '刚刚',
      scope,
      projectId,
      active: true,
      busy: busyAfterSubmit,
      activeTurnId: busyAfterSubmit ? turnID : undefined,
    },
    ...current.sessions.map((session) => ({ ...session, active: false })),
  ];
}

function toRuntimeUserInputRequest(
  input: RuntimeUserInputRequestViewModel,
  activeSessionID: string | undefined,
  draftTarget: WorkbenchViewModel['newConversationDraft'],
): RuntimeUserInputRequestDTO {
  return {
    sessionId: activeSessionID,
    projectId: draftTarget?.scope === 'project' ? draftTarget.projectId : input.projectId,
    scope: draftTarget?.scope ?? input.scope,
    mode: input.mode || 'prompt',
    items: input.items.map((item) => ({
      type: item.type,
      text: item.text,
      data: item.data,
      mimeType: item.mimeType,
      fileName: item.fileName,
      sourcePath: item.sourcePath,
      metadata: item.metadata,
    })),
    options: input.options,
  };
}

function promptToUserInput(prompt: string, clientRequestId?: string): RuntimeUserInputRequestViewModel {
  const trimmed = prompt.trim();
  return {
    mode: trimmed.startsWith('/') ? 'slash' : 'prompt',
    items: [
      {
        type: 'text',
        text: prompt,
      },
    ],
    options: clientRequestId ? { clientRequestId } : undefined,
  };
}

function resolveContextActionTurnId(current: WorkbenchViewModel): string | undefined {
  return current.contextDiagnostics?.turnId || current.composer.activeTurnId || current.turnDiagnostics?.turnId;
}

function contextActionRequest(current: WorkbenchViewModel, reason: string): RuntimeContextActionRequestDTO {
  const sessionId = current.contextDiagnostics?.sessionId || current.sessions.find((session) => session.active)?.id;
  const turnId = resolveContextActionTurnId(current);
  if (!turnId) {
    throw new Error('No turn is available for context governance action');
  }
  return { sessionId, turnId, projectionId: current.contextDiagnostics?.projectionId, reason };
}

function projectNameFromPath(path: string) {
  const normalized = path.trim().replace(/[\\/]+$/, '');
  if (!normalized) {
    return '';
  }
  const parts = normalized.split(/[\\/]+/);
  return parts[parts.length - 1] || normalized;
}

function isFinalTurnStatus(status?: string) {
  return ['completed', 'failed', 'cancelled', 'interrupted'].includes(status || '');
}

function mapConfiguredProviders(response?: RuntimeConfiguredProvidersResponseDTO): ConfiguredProviderViewModel[] | undefined {
  if (!response) {
    return undefined;
  }
  if (!Array.isArray(response?.providers)) {
    return [];
  }

  return response.providers.map((provider) => ({
    id: provider.id,
    providerId: provider.providerId,
    name: provider.name,
    remark: provider.remark,
    apiEndpoint: provider.apiEndpoint,
    protocol: provider.protocol,
    defaultModel: provider.defaultModel,
    models: provider.models?.map(mapProviderModel),
    defaultContextWindow: provider.defaultContextWindow,
    tokenConfigured: provider.hasApiKey,
    token: provider.apiKey,
    proxy: provider.proxy,
    enabled: provider.enabled,
  }));
}

function isConfiguredProviderNotFound(error: unknown) {
  return error instanceof Error && error.message.toLowerCase().includes('configured provider not found');
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

function mapHook(dto: RuntimeHookDTO, index = 0): HookViewModel {
  const event = dto.event || 'unknown';
  const timeoutSeconds = dto.timeout;
  const timeoutMs = dto.timeoutMs ?? dto.timeout_ms ?? (typeof timeoutSeconds === 'number' && timeoutSeconds > 0 ? timeoutSeconds * 1000 : undefined);
  const rawCommand = dto.command || dto.name || '';
  const status = normalizeHookStatus(dto.status, dto.enabled);
  return {
    id: dto.id || `hook:${event}:${index}`,
    name: dto.name || event,
    source: dto.source || 'unknown',
    event,
    matcher: dto.matcher || undefined,
    commandPreview: previewText(rawCommand || dto.name || event, 140),
    enabled: dto.enabled !== false && status !== 'disabled',
    status,
    diagnostics: dto.diagnostics,
    reason: dto.reason,
    timeoutMs,
  };
}

function mapHooks(response?: RuntimeHooksResponseDTO): HookViewModel[] | undefined {
  if (!Array.isArray(response?.hooks)) {
    return undefined;
  }
  return response.hooks.map(mapHook);
}

function mapHookExecution(dto: RuntimeHookExecutionDTO): HookExecutionViewModel {
  return {
    id: dto.id || '',
    hookId: dto.hookId ?? dto.hook_id ?? '',
    hookName: dto.hookName ?? dto.hook_name,
    hookSource: dto.hookSource ?? dto.hook_source,
    event: dto.event || 'unknown',
    status: dto.status || 'unknown',
    sessionId: dto.sessionId ?? dto.session_id,
    turnId: dto.turnId ?? dto.turn_id,
    toolCallId: dto.toolCallId ?? dto.tool_call_id,
    taskId: dto.taskId ?? dto.task_id,
    capabilityId: dto.capabilityId ?? dto.capability_id,
    mcpServer: dto.mcpServer ?? dto.mcp_server,
    skill: dto.skill,
    contextRef: dto.contextRef ?? dto.context_ref,
    policyMode: dto.policyMode ?? dto.policy_mode,
    policyProfile: dto.policyProfile ?? dto.policy_profile,
    policyRule: dto.policyRule ?? dto.policy_rule,
    policyDecision: dto.policyDecision ?? dto.policy_decision,
    policyReason: dto.policyReason ?? dto.policy_reason,
    headless: dto.headless,
    headlessReason: dto.headlessReason ?? dto.headless_reason,
    sandboxDecisionId: dto.sandboxDecisionId ?? dto.sandbox_decision_id,
    sandboxStatus: dto.sandboxStatus ?? dto.sandbox_status,
    scopeKind: dto.scopeKind ?? dto.scope_kind,
    scopeValue: dto.scopeValue ?? dto.scope_value,
    reason: dto.reason,
    error: dto.error,
    inputSummary: dto.inputSummary ?? dto.input_summary,
    outputSummary: dto.outputSummary ?? dto.output_summary,
    contextSummary: dto.contextSummary ?? dto.context_summary,
    inputRewritten: Boolean(dto.inputRewritten ?? dto.input_rewritten),
    contextInjected: Boolean(dto.contextInjected ?? dto.context_injected),
    redacted: Boolean(dto.redacted),
    startedAt: dto.startedAt ?? dto.started_at,
    completedAt: dto.completedAt ?? dto.completed_at,
    durationMs: dto.durationMs ?? dto.duration_ms,
  };
}

function mapHookExecutions(response?: RuntimeHookExecutionsResponseDTO): HookExecutionViewModel[] | undefined {
  if (!Array.isArray(response?.executions)) {
    return undefined;
  }
  return response.executions.map(mapHookExecution).filter((item) => item.id);
}

function summarizeHookExecutions(items: HookExecutionViewModel[], sessionId?: string): HookExecutionSummaryViewModel {
  const summary: HookExecutionSummaryViewModel = {
    sessionId,
    items: [...items].sort((left, right) => (right.startedAt ?? 0) - (left.startedAt ?? 0)),
    total: items.length,
    started: 0,
    completed: 0,
    blocked: 0,
    failed: 0,
    skipped: 0,
    rewritten: 0,
    contextInjected: 0,
    lastUpdatedAt: Date.now(),
  };
  for (const item of items) {
    if (item.status === 'started' || item.status === 'running') {
      summary.started += 1;
    } else if (item.status === 'completed') {
      summary.completed += 1;
    } else if (item.status === 'blocked' || item.status === 'denied') {
      summary.blocked += 1;
    } else if (item.status === 'failed') {
      summary.failed += 1;
    } else if (item.status === 'skipped') {
      summary.skipped += 1;
    }
    if (item.inputRewritten) {
      summary.rewritten += 1;
    }
    if (item.contextInjected) {
      summary.contextInjected += 1;
    }
  }
  return summary;
}

async function hydrateHooks(bridge: RuntimeBridgeModule): Promise<HookViewModel[] | undefined> {
  return mapHooks(await optionalRuntimeRequest(() => bridge.Hooks?.() ?? Promise.resolve(undefined)));
}

async function hydrateHookExecutions(bridge: RuntimeBridgeModule, sessionId?: string): Promise<HookExecutionSummaryViewModel | undefined> {
  if (!bridge.HookExecutions) {
    return undefined;
  }
  if (!sessionId) {
    return summarizeHookExecutions([]);
  }
  const response = await optionalRuntimeRequest(() => bridge.HookExecutions?.({ sessionId, limit: 200 }) ?? Promise.resolve(undefined));
  return summarizeHookExecutions(mapHookExecutions(response) ?? [], sessionId);
}

function normalizeHookStatus(status?: string, enabled?: boolean) {
  if (status === 'enabled' || status === 'configured' || status === 'discovered') {
    return 'active';
  }
  if (status === 'disabled') {
    return 'invalid';
  }
  if (status) {
    return status;
  }
  if (enabled === true) {
    return 'active';
  }
  if (enabled === false) {
    return 'invalid';
  }
  return 'unknown';
}

function previewText(value: string, limit: number) {
  const normalized = value.replace(/\s+/g, ' ').trim();
  if (normalized.length <= limit) {
    return normalized;
  }
  return `${normalized.slice(0, Math.max(0, limit - 3))}...`;
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
    .filter((message) => !message.hidden && (message.role === 'user' || message.role === 'assistant'))
    .map((message) => ({
      id: message.id,
      role: message.role,
      content: runtimeMessageContent(message),
      createdAt: message.createdAt,
      clientRequestId: message.clientRequestId ?? message.metadata?.clientRequestId,
      provider: message.provider,
      model: message.model,
      status: message.error ? 'error' : 'success',
      error: message.error,
    }));
}

function mapNormalizedInputConversation(response: RuntimeChatResponseDTO, fallbackPrompt: string): ConversationMessageViewModel[] {
  const normalized = response.normalizedInput;
  const hookPrevented = normalized?.hookOutcome?.preventContinuation || normalized?.hookOutcome?.status === 'blocked';
  if (!normalized || response.turnId || (normalized.shouldQuery === true && !hookPrevented)) {
    return [];
  }
  const messages = Array.isArray(normalized.messages) ? normalized.messages : [];
  const clientRequestId = normalizedClientRequestId(normalized);
  const visibleMessages = messages.filter((message) => {
    if (message.hidden) {
      return false;
    }
    if (hookPrevented && message.role === 'user') {
      return false;
    }
    return message.role === 'user' || message.role === 'assistant' || message.role === 'system';
  });
  const conversation: ConversationMessageViewModel[] = visibleMessages.map((message, index) => ({
    id: `${normalized.id || response.requestId || 'input'}-${index}`,
    role: message.role === 'system' || message.role === 'assistant' ? 'assistant' as const : 'user' as const,
    content: message.content || (message.role === 'user' ? fallbackPrompt : ''),
    createdAt: normalized.createdAt,
    clientRequestId: message.role === 'user' ? clientRequestId : undefined,
    status: 'success' as const,
  }));
  if (!conversation.some((message) => message.role === 'assistant') && normalized.command?.resultText) {
    conversation.push({
      id: `${normalized.id || response.requestId || 'input'}-command`,
      role: 'assistant',
      content: normalized.command.resultText,
      createdAt: normalized.createdAt,
      clientRequestId,
      status: 'success',
    });
  }
  if (hookPrevented) {
    const hookOutcome = normalized.hookOutcome;
    const reason = hookOutcome?.reason?.trim();
    const event = hookOutcome?.metadata?.event || 'UserPromptSubmit';
    conversation.push({
      id: `${normalized.id || response.requestId || 'input'}-hook-blocked`,
      role: 'assistant',
      content: reason
        ? `Prompt blocked by ${event} hook: ${reason}`
        : `Prompt blocked by ${event} hook.`,
      createdAt: normalized.createdAt,
      clientRequestId,
      status: 'error',
      error: reason || 'Prompt blocked by hook.',
    });
  }
  return conversation;
}

function mapNormalizedInputTimeline(response: RuntimeChatResponseDTO, fallbackPrompt: string): ConversationTimelineItemViewModel[] {
  return mapNormalizedInputConversation(response, fallbackPrompt).map((message) => ({
    id: `timeline-${message.id}`,
    kind: 'message',
    role: message.role === 'assistant' || message.role === 'user' ? message.role : undefined,
    content: message.content,
    createdAt: message.createdAt,
    clientRequestId: message.clientRequestId,
    status: message.status,
  }));
}

function normalizedClientRequestId(normalized?: RuntimeNormalizedInputDTO) {
  if (!normalized) {
    return undefined;
  }
  return normalized.messages?.find((message) => message.metadata?.clientRequestId)?.metadata?.clientRequestId;
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
    description: permission.description,
    action: permission.action,
    risk: permission.risk,
    status: permission.status,
    path: permission.path,
    target: permission.target || permission.path,
    reason: permission.reason || permission.policyReason,
    policyReason: permission.policyReason,
    policyMode: permission.policyMode,
    policyRuleId: permission.policyRuleId,
    policyRuleSource: permission.policyRuleSource,
    policyScopeKind: permission.policyScopeKind,
    policyScopeValue: permission.policyScopeValue,
    policyTargetSummary: permission.policyTargetSummary,
    createdAt: permission.createdAt,
    decidedAt: permission.decidedAt,
  };
}

function mapToolCall(toolCall: RuntimeToolCallDTO): ToolCallViewModel {
  const display = toolCall.display;
  return {
    id: toolCall.id,
    sessionId: toolCall.sessionId,
    turnId: toolCall.turnId,
    name: toolCall.name,
    source: toolCall.source,
    command: toolCall.command,
    risk: toolCall.risk,
    status: toolCall.status,
    inputSummary: toolCall.inputSummary,
    outputSummary: toolCall.outputSummary,
    error: toolCall.error,
    policyMode: toolCall.policyMode,
    policyReason: toolCall.policyReason,
    policyTargetSummary: toolCall.policyTargetSummary,
    display: display
      ? {
          kind: display.kind,
          title: display.title,
          detail: display.detail,
          target: display.target,
          primaryTarget: display.primaryTarget,
          targets: display.targets,
          workingDir: display.workingDir,
          command: display.command,
          exitCode: display.exitCode,
          durationMs: display.durationMs,
          stdoutExcerpt: display.stdoutExcerpt,
          stderrExcerpt: display.stderrExcerpt,
          inputExcerpt: display.inputExcerpt,
          outputExcerpt: display.outputExcerpt,
          failureReason: display.failureReason,
          artifactCount: display.artifactCount,
          diffCount: display.diffCount,
          artifactRefs: display.artifactRefs,
          diffRefs: display.diffRefs,
          artifactSummary: display.artifactSummary,
          diffSummary: display.diffSummary,
        }
      : undefined,
    exitCode: toolCall.exitCode,
    outputRefs: toolCall.outputRefs,
    artifactRefs: toolCall.artifactRefs,
    diffRefs: toolCall.diffRefs,
    startedAt: toolCall.startedAt,
    finishedAt: toolCall.finishedAt,
  };
}


export function attachAgentTasksToTimeline(items: ConversationTimelineItemViewModel[], tasks?: AgentTaskViewModel[]): ConversationTimelineItemViewModel[] {
  if (!tasks?.length) {
    return items;
  }
  const taskByTool = new Map(tasks.filter((task) => task.parentToolCallId).map((task) => [task.parentToolCallId!, task]));
  const taskItems = tasks.map((task) => ({
    id: `agent-task:${task.id}`,
    kind: 'agent_task' as const,
    sessionId: task.parentSessionId,
    turnId: task.parentTurnId,
    toolCallId: task.parentToolCallId,
    title: task.title,
    status: task.status,
    summary: task.resultSummary || task.promptSummary,
    createdAt: task.startedAt,
    updatedAt: task.finishedAt || task.updatedAt,
    error: task.error,
    agentTask: task,
  }));
  const byTool = new Map<string, ConversationTimelineItemViewModel[]>();
  const unplaced: ConversationTimelineItemViewModel[] = [];
  taskItems.forEach((item) => {
    if (item.toolCallId) {
      byTool.set(item.toolCallId, [...(byTool.get(item.toolCallId) ?? []), item]);
    } else {
      unplaced.push(item);
    }
  });
  const merged: ConversationTimelineItemViewModel[] = [];
  items.forEach((item) => {
    if (item.kind === 'tool_call' && item.toolCallId && item.toolCall) {
      const agentTask = taskByTool.get(item.toolCallId);
      merged.push(agentTask ? { ...item, toolCall: { ...item.toolCall, agentTask } } : item);
    } else {
      merged.push(item);
    }
    if (item.kind === 'tool_call' && item.toolCallId) {
      merged.push(...(byTool.get(item.toolCallId) ?? []));
      byTool.delete(item.toolCallId);
    }
  });
  byTool.forEach((values) => unplaced.push(...values));
  unplaced.sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
  return [...merged, ...unplaced];
}


export function mergeConversationMessages(current: ConversationMessageViewModel[], response?: RuntimeMessagesResponseDTO) {
  const incoming = mapConversation(response);
  if (incoming.length === 0) {
    return current;
  }
  const incomingIDs = new Set(incoming.map((message) => message.id));
  return [...current.filter((message) => !incomingIDs.has(message.id) && !hasRuntimeReplacementForOptimisticConversation(message, incoming)), ...incoming].sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function mergeNormalizedConversation(current: ConversationMessageViewModel[], incoming: ConversationMessageViewModel[]) {
  if (incoming.length === 0) {
    return current.filter((message) => message.status !== 'loading');
  }
  const incomingIDs = new Set(incoming.map((message) => message.id));
  return [
    ...current.filter((message) => message.status !== 'loading' && !incomingIDs.has(message.id) && !hasRuntimeReplacementForOptimisticConversation(message, incoming)),
    ...incoming,
  ].sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function mergeNormalizedTimeline(current: ConversationTimelineItemViewModel[], incoming: ConversationTimelineItemViewModel[]) {
  if (incoming.length === 0) {
    return current.filter((item) => item.status !== 'loading');
  }
  const incomingIDs = new Set(incoming.map((item) => item.id));
  return dedupeTimelineItems([
    ...current.filter((item) => item.status !== 'loading' && !incomingIDs.has(item.id) && !hasRuntimeReplacementForOptimisticTimeline(item, incoming)),
    ...incoming,
  ]).sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function dedupeTimelineItems(items: ConversationTimelineItemViewModel[]) {
  const byID = new Map<string, ConversationTimelineItemViewModel>();
  items.forEach((item) => {
    byID.set(item.id, item);
  });
  return [...byID.values()];
}

function isOptimisticConversationMessage(message: ConversationMessageViewModel) {
  return message.status === 'loading' || message.id.startsWith('local-') || message.id.startsWith('loading-');
}


function hasRuntimeReplacementForOptimisticConversation(message: ConversationMessageViewModel, incoming: ConversationMessageViewModel[]) {
  if (!isOptimisticConversationMessage(message)) {
    return false;
  }
  if (message.clientRequestId && incoming.some((candidate) => candidate.clientRequestId === message.clientRequestId && candidate.role === message.role)) {
    return true;
  }
  if (message.role === 'user') {
    return incoming.some((candidate) => candidate.role === 'user' && sameDisplayContent(candidate.content, message.content));
  }
  if (message.status === 'loading' && message.role === 'assistant') {
    return incoming.some((candidate) => candidate.role === 'assistant');
  }
  return incoming.some((candidate) => candidate.role === message.role && sameDisplayContent(candidate.content, message.content));
}

function hasRuntimeReplacementForOptimisticTimeline(item: ConversationTimelineItemViewModel, replacement: ConversationTimelineItemViewModel[]) {
  if (item.clientRequestId && replacement.some((candidate) => candidate.clientRequestId === item.clientRequestId && candidate.kind === item.kind && candidate.role === item.role)) {
    return true;
  }
  if (item.kind === 'message' && item.role === 'user') {
    return replacement.some((candidate) => candidate.kind === 'message' && candidate.role === 'user' && sameDisplayContent(candidate.content, item.content));
  }
  if (item.kind === 'message' && item.role === 'assistant' && item.status === 'loading') {
    return replacement.some((candidate) =>
      candidate.role === 'assistant' ||
      candidate.kind === 'thinking' ||
      candidate.kind === 'tool_call' ||
      candidate.kind === 'permission' ||
      candidate.kind === 'progress',
    );
  }
  return replacement.some((candidate) => candidate.kind === item.kind && candidate.role === item.role && sameDisplayContent(candidate.content || candidate.summary, item.content || item.summary));
}

function sameDisplayContent(left?: string, right?: string) {
  return normalizeDisplayContent(left) !== '' && normalizeDisplayContent(left) === normalizeDisplayContent(right);
}

function normalizeDisplayContent(value?: string) {
  return (value ?? '').replace(/\s+/g, ' ').trim();
}

export function mergePendingPermissions(current: PermissionRequestViewModel[], permissions: PermissionRequestViewModel[]) {
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

function mapRecoveryStatus(response?: RuntimeRecoveryStatusDTO): RecoveryStatusViewModel {
  return {
    runtimeStartedAt: response?.runtime_started_at,
    lastEventSequence: response?.last_event_sequence,
    activeTurns: (response?.active_turns ?? []).map(mapRecoveredRuntimeTurn),
    interruptedTurns: (response?.interrupted_turns ?? []).map(mapRecoveredTurn),
    recoverableErrors: (response?.recoverable_errors ?? []).map(mapRecoverableError),
    pendingPermissions: (response?.pending_permissions ?? []).map(mapPermission),
    pendingMCPRequests: (response?.pending_mcp_requests ?? []).map(mapMCPRequest),
    compactBoundaries: (response?.compact_boundaries ?? []).map(mapCompactBoundary),
    actions: (response?.actions ?? []).map(mapRecoveryAction),
    snapshotRequired: response?.snapshot_required,
  };
}

function mapRecoveredRuntimeTurn(turn: RuntimeTurnDTO): RecoveredRuntimeTurnViewModel {
  return {
    id: turn.id,
    sessionId: turn.sessionId,
    status: turn.status,
    error: turn.error,
    startedAt: turn.startedAt,
    finishedAt: turn.finishedAt,
  };
}

function mapRecoveredTurn(turn: RuntimeRecoveredTurnDTO): RecoveredTurnViewModel {
  return {
    ...mapRecoveredRuntimeTurn(turn),
    interruptionKind: turn.interruption_kind || 'unknown',
    resumeEligible: Boolean(turn.resume_eligible),
    discardEligible: Boolean(turn.discard_eligible),
    markDoneEligible: Boolean(turn.mark_done_eligible),
    reason: turn.reason,
    resumeHint: turn.resume_hint,
    openToolCalls: (turn.open_tool_calls ?? []).map(mapToolCall),
    checkpoints: (turn.checkpoints ?? []).map(mapRunCheckpoint),
  };
}

function mapRecoverableError(error: RuntimeRecoverableErrorDTO): RecoverableErrorViewModel {
  return {
    id: error.id,
    kind: error.kind || 'unknown',
    severity: error.severity || 'error',
    sessionId: error.session_id,
    turnId: error.turn_id,
    runId: error.run_id,
    provider: error.provider,
    model: error.model,
    message: error.message || '',
    retryEligible: Boolean(error.retry_eligible),
    compactEligible: Boolean(error.compact_eligible),
    userAction: error.user_action,
    details: error.details,
    createdAt: error.created_at,
  };
}

function mapRecoveryAction(action: RuntimeRecoveryActionDTO): RecoveryActionViewModel {
  return {
    id: action.id,
    label: action.label || action.kind || action.id,
    kind: action.kind || '',
    sessionId: action.session_id,
    turnId: action.turn_id,
    runId: action.run_id,
    checkpointId: action.checkpoint_id,
    destructive: Boolean(action.destructive),
    startsWorker: Boolean(action.starts_worker),
    evidence: action.evidence ?? [],
  };
}

function mapCompactBoundary(boundary: RuntimeCompactBoundaryDTO): CompactBoundaryViewModel {
  return {
    id: boundary.id || `${boundary.kind || 'compact'}:${boundary.createdAt ?? 0}`,
    kind: boundary.kind || 'compact',
    trigger: boundary.trigger || 'unknown',
    status: boundary.status || 'unknown',
    summaryRef: boundary.summaryRef,
    messageRefs: Array.isArray(boundary.messageRefs) ? boundary.messageRefs : [],
    toolCallRefCount: Array.isArray(boundary.toolCallRefs) ? boundary.toolCallRefs.length : 0,
    reinjectedRefCount: Array.isArray(boundary.reinjectedRefs) ? boundary.reinjectedRefs.length : 0,
    error: boundary.error,
    createdAt: boundary.createdAt,
    completedAt: boundary.completedAt,
  };
}

function mapRunCheckpoint(checkpoint: RuntimeRunCheckpointDTO): RunCheckpointViewModel {
  return {
    id: checkpoint.id || '',
    runId: checkpoint.runId,
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
  };
}

function mapMCPRequest(request: RuntimeMCPRequestDTO): RuntimeMCPRequestViewModel {
  return {
    id: request.id || '',
    sessionId: request.sessionId ?? request.session_id,
    turnId: request.turnId ?? request.turn_id,
    kind: request.kind,
    status: request.status,
    server: request.server,
    tool: request.tool,
    prompt: request.prompt,
    reason: request.reason,
    createdAt: request.createdAt ?? request.created_at,
  };
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

function mapReactCallchain(callchain?: RuntimeReactCallchainDTO, hookExecutions?: HookExecutionSummaryViewModel): ReactCallchainViewModel | undefined {
  if (!callchain?.sessionId) {
    return undefined;
  }
  const nodes = (Array.isArray(callchain.nodes) ? callchain.nodes : [])
    .filter((node) => node.id && node.kind && typeof node.sequence === 'number')
    .map((node) => {
      const hook = node.hookExecutionId ? hookExecutions?.items.find((item) => item.id === node.hookExecutionId) : undefined;
      return {
        id: node.id,
        parentId: node.parentId,
        kind: node.kind,
        sessionId: node.sessionId || callchain.sessionId,
        turnId: node.turnId,
        messageId: node.messageId,
        toolCallId: node.toolCallId,
        permissionId: node.permissionId,
        hookExecutionId: node.hookExecutionId,
        sequence: node.sequence,
        status: node.status,
        finishReason: node.finishReason,
        title: node.title,
        summary: node.summary,
        error: node.error,
        startedAt: node.startedAt,
        finishedAt: node.finishedAt,
        evidence: node.evidence,
        hook: hook
          ? {
              executionId: hook.id,
              event: hook.event,
              status: hook.status,
              reason: hook.reason,
              durationMs: hook.durationMs,
              inputRewritten: hook.inputRewritten,
              contextInjected: hook.contextInjected,
            }
          : node.kind === 'hook_execution'
            ? {
                executionId: node.hookExecutionId,
                event: node.evidence?.event,
                status: node.status,
                reason: node.summary,
                durationMs: node.startedAt && node.finishedAt ? Math.max(0, normalizeTimestamp(node.finishedAt) - normalizeTimestamp(node.startedAt)) : undefined,
              }
            : undefined,
      };
    })
    .sort((left, right) => left.sequence - right.sequence || left.id.localeCompare(right.id));
  const summary = callchain.summary ?? {};
  const source = callchain.source ?? {};
  const topLevelDeliveries = mapToolResultDeliveries(callchain.toolResultDeliveries);
  const summaryDeliveries = mapToolResultDeliveries(summary.toolResultDeliveries);
  const toolResultDeliveries = topLevelDeliveries.length ? topLevelDeliveries : summaryDeliveries;
  return {
    sessionId: callchain.sessionId,
    turnId: callchain.turnId,
    nodes,
    summary: {
      hasFinalAssistant: Boolean(summary.hasFinalAssistant),
      finalAssistantMessageId: summary.finalAssistantMessageId,
      finalAssistantEmpty: Boolean(summary.finalAssistantEmpty),
      lastAssistantFinishReason: summary.lastAssistantFinishReason,
      toolCallCount: summary.toolCallCount ?? 0,
      permissionCount: summary.permissionCount ?? 0,
      hookCount: summary.hookCount ?? 0,
      stopReason: summary.stopReason,
      stopReasonMessage: summary.stopReasonMessage,
      missingEvidence: Array.isArray(summary.missingEvidence) ? summary.missingEvidence : [],
      toolResultDeliveries,
      deliveredToolResultCount: summary.deliveredToolResultCount,
      undeliveredToolResultCount: summary.undeliveredToolResultCount,
    },
    source: {
      sessionActivityParity: Boolean(source.sessionActivityParity),
      usesMessages: Boolean(source.usesMessages),
      usesToolCalls: Boolean(source.usesToolCalls),
      usesPermissions: Boolean(source.usesPermissions),
      usesHooks: Boolean(source.usesHooks),
      eventsAreRefreshOnly: Boolean(source.eventsAreRefreshOnly),
    },
    toolResultDeliveries,
  };
}

function mapToolResultDeliveries(deliveries?: RuntimeToolResultDeliveryDTO[]) {
  if (!Array.isArray(deliveries)) {
    return [];
  }
  return deliveries
    .filter((delivery) => Boolean(delivery.toolCallId))
    .map((delivery) => ({
      toolCallId: delivery.toolCallId || '',
      toolResultMessageId: delivery.toolResultMessageId,
      deliveredToModel: Boolean(delivery.deliveredToModel),
      deliveredAtStep: delivery.deliveredAtStep,
      synthetic: Boolean(delivery.synthetic),
      reason: delivery.reason,
    }));
}

function mapContextDiagnostics(response?: RuntimePromptAssembliesResponseDTO): ContextDiagnosticsViewModel | undefined {
  const assemblies = Array.isArray(response?.assemblies) ? response.assemblies : [];
  const assembly = assemblies
    .filter((item) => item?.sessionId)
    .sort((left, right) => (right.step ?? 0) - (left.step ?? 0) || (right.createdAt ?? 0) - (left.createdAt ?? 0))[0];
  if (!assembly?.sessionId) {
    return undefined;
  }

  const system = assembly.system ?? {};
  const messages = assembly.messages ?? {};
  const tools = assembly.tools ?? {};
  const skills = assembly.skills ?? {};
  const mcp = assembly.mcp ?? {};
  const sections = (Array.isArray(assembly.sections) ? assembly.sections : [])
    .filter((section) => section.id || section.name)
    .sort((left, right) => (left.order ?? 0) - (right.order ?? 0))
    .slice(0, 48)
    .map((section, index) => ({
      id: section.id || `${section.kind || 'section'}:${index}`,
      name: section.name || section.id || 'Prompt section',
      kind: section.kind || 'prompt_section',
      role: section.role || 'system',
      order: section.order ?? index + 1,
      cachePolicy: section.cachePolicy || 'turn_dynamic',
      source: section.source,
      sourceRefs: Array.isArray(section.sourceRefs) ? section.sourceRefs : [],
      scope: section.scope,
      hash: section.hash,
      length: section.length,
      tokenEstimate: section.tokenEstimate,
      redacted: section.redacted !== false,
      rawStored: Boolean(section.rawStored),
      diagnostics: section.diagnostics,
    }));
  const contextSources = (Array.isArray(assembly.contextSources) ? assembly.contextSources : [])
    .filter((source) => source.id || source.name)
    .slice(0, 24)
    .map((source) => ({
      id: source.id || source.name || 'context-source',
      kind: source.kind || 'context',
      name: source.name || source.id || 'Context source',
      path: source.path,
      uri: source.uri,
      scope: source.scope,
      enabled: source.enabled !== false,
      state: source.state || 'unknown',
      reason: source.reason,
      diagnostics: source.diagnostics,
      error: source.error,
      tokenEstimate: source.tokenEstimate ?? source.token_estimate,
      provenance: source.provenance,
      contentHash: source.contentHash ?? source.content_hash,
    }));
  const compactBoundaryDTOs = [
    ...(Array.isArray(assembly.compact) ? assembly.compact : []),
    ...(Array.isArray(assembly.contextBoundaries) ? assembly.contextBoundaries : []),
  ];
  const compactBoundaries = compactBoundaryDTOs
    .filter((boundary) => boundary.id || boundary.kind)
    .slice(0, 12)
    .map((boundary) => ({
      id: boundary.id || `${boundary.kind || 'compact'}:${boundary.createdAt ?? 0}`,
      kind: boundary.kind || 'compact',
      trigger: boundary.trigger || 'unknown',
      status: boundary.status || 'unknown',
      summaryRef: boundary.summaryRef,
      messageRefs: Array.isArray(boundary.messageRefs) ? boundary.messageRefs : [],
      toolCallRefCount: Array.isArray(boundary.toolCallRefs) ? boundary.toolCallRefs.length : 0,
      reinjectedRefCount: Array.isArray(boundary.reinjectedRefs) ? boundary.reinjectedRefs.length : 0,
      error: boundary.error,
      createdAt: boundary.createdAt,
      completedAt: boundary.completedAt,
    }));
  const snipBoundaries = (Array.isArray(assembly.snipBoundaries) ? assembly.snipBoundaries : [])
    .filter((boundary) => boundary.id)
    .slice(0, 12)
    .map((boundary) => ({
      id: boundary.id || 'snip',
      reason: boundary.reason,
      removedMessageCount: Array.isArray(boundary.removedMessageRefs) ? boundary.removedMessageRefs.length : 0,
      summaryRef: boundary.summaryRef,
      createdAt: boundary.createdAt,
    }));
  const replacements = (Array.isArray(assembly.replacements) ? assembly.replacements : [])
    .filter((replacement) => replacement.id || replacement.toolCallId)
    .slice(0, 24)
    .map((replacement) => ({
      id: replacement.id || replacement.toolCallId || 'replacement',
      toolCallId: replacement.toolCallId,
      kind: replacement.kind || 'tool_result',
      reason: replacement.reason,
      originalRef: replacement.originalRef,
      createdAt: replacement.createdAt,
    }));
  const reactiveAttempts = (Array.isArray(assembly.reactiveAttempts) ? assembly.reactiveAttempts : [])
    .filter((attempt) => attempt.id)
    .slice(0, 12)
    .map((attempt) => ({
      id: attempt.id || 'reactive',
      attempt: attempt.attempt ?? 0,
      action: attempt.action || 'unknown',
      status: attempt.status || 'unknown',
      error: attempt.error,
      createdAt: attempt.createdAt,
    }));
  const warnings = [
    ...contextSources
      .filter((source) => source.state === 'failed' || source.state === 'skipped' || Boolean(source.error))
      .map((source) => `${source.name}: ${source.error || source.reason || source.state}`),
    ...sections
      .filter((section) => section.rawStored)
      .map((section) => `${section.name}: raw section content storage reported`),
    ...(messages.rawPromptStored ? ['Runtime reported raw prompt storage in prompt assembly metadata.'] : []),
    ...(skills.rawContentStored ? ['Runtime reported raw skill content storage in prompt assembly metadata.'] : []),
    ...(mcp.rawContentStored ? ['Runtime reported raw MCP instruction storage in prompt assembly metadata.'] : []),
  ];

  return {
    sessionId: assembly.sessionId,
    turnId: assembly.turnId,
    projectionId: assembly.projectionId,
    step: assembly.step,
    provider: assembly.provider,
    model: assembly.model,
    createdAt: assembly.createdAt,
    sections,
    system: {
      source: system.source,
      hash: system.hash,
      tokenEstimate: system.tokenEstimate,
      promptPrefix: Boolean(system.promptPrefix),
      promptPrefixHash: system.promptPrefixHash,
      sourceRefs: Array.isArray(system.sourceRefs) ? system.sourceRefs : [],
      redacted: system.redacted !== false,
    },
    messages: {
      count: messages.count ?? 0,
      byRole: messages.byRole ?? {},
      toolResultCount: messages.toolResultCount ?? 0,
      deliveredToolResults: messages.deliveredToolResults ?? 0,
      syntheticToolResults: messages.syntheticToolResults ?? 0,
      attachmentCount: messages.attachmentCount ?? 0,
      imageCount: messages.imageCount ?? 0,
      tokenEstimate: messages.tokenEstimate,
      rawPromptStored: Boolean(messages.rawPromptStored),
    },
    tools: {
      selected: Array.isArray(tools.selected) ? tools.selected : [],
      omitted: Array.isArray(tools.omitted) ? tools.omitted : [],
      selectedCount: tools.selectedCount ?? tools.selected?.length ?? 0,
      omittedCount: tools.omittedCount ?? tools.omitted?.length ?? 0,
      resultCount: tools.resultCount ?? 0,
      persistedResults: tools.persistedResults ?? 0,
      compactedResults: tools.compactedResults ?? 0,
    },
    skills: {
      availableCount: skills.availableCount ?? 0,
      loadedCount: skills.loadedCount ?? skills.loadedNames?.length ?? 0,
      names: Array.isArray(skills.names) ? skills.names : [],
      loadedNames: Array.isArray(skills.loadedNames) ? skills.loadedNames : [],
      xmlPresent: Boolean(skills.xmlPresent),
      xmlHash: skills.xmlHash,
      tokenEstimate: skills.tokenEstimate,
      rawContentStored: Boolean(skills.rawContentStored),
    },
    mcp: {
      serverCount: mcp.serverCount ?? mcp.servers?.length ?? 0,
      instructionCount: mcp.instructionCount ?? 0,
      servers: Array.isArray(mcp.servers) ? mcp.servers : [],
      serverListHash: mcp.serverListHash,
      instructionHash: mcp.instructionHash,
      tokenEstimate: mcp.tokenEstimate,
      rawContentStored: Boolean(mcp.rawContentStored),
    },
    contextSources,
    compactBoundaries,
    snipBoundaries,
    replacements,
    reactiveAttempts,
    budget: mapPromptBudget(assembly.budget),
    warnings,
  };
}

function mapPromptBudget(budget?: RuntimeBudgetReportDTO) {
  return {
    contextWindow: budget?.contextWindow,
    inputBudget: mapBudgetBucket(budget?.inputBudget),
    messages: mapBudgetBucket(budget?.messages),
    contextSources: mapBudgetBucket(budget?.contextSources),
    toolSchemas: mapBudgetBucket(budget?.toolSchemas),
    skills: mapBudgetBucket(budget?.skills),
    mcp: mapBudgetBucket(budget?.mcp),
    toolOutputs: mapBudgetBucket(budget?.toolOutputs),
    selectedToolSchemas: mapBudgetBucket(budget?.selectedToolSchemas),
    omittedToolSchemas: mapBudgetBucket(budget?.omittedToolSchemas),
    totalEstimatedTokens: budget?.totalEstimatedTokens ?? 0,
    updatedAt: budget?.updatedAt,
  };
}

function mapBudgetBucket(bucket?: RuntimeBudgetBucketDTO) {
  if (!bucket) {
    return undefined;
  }
  return {
    count: bucket.count ?? 0,
    estimatedTokens: bucket.estimatedTokens ?? 0,
  };
}

function mapContextUsage(usage?: RuntimeContextUsageDTO): ContextUsageViewModel | undefined {
  if (!usage) {
    return undefined;
  }
  return {
    sessionId: usage.sessionId ?? '',
    model: usage.model ?? '',
    contextWindow: usage.contextWindow ?? 0,
    usedTokens: usage.usedTokens ?? 0,
    percentUsed: usage.percentUsed ?? 0,
    autoCompactAt: usage.autoCompactAt ?? 0,
    percentLeft: usage.percentLeft ?? 0,
    level: usage.level ?? 'ok',
    estimated: Boolean(usage.estimated),
    autoCompactEnabled: usage.autoCompactEnabled ?? true,
    outputReserve: usage.outputReserve ?? 0,
    autoCompactBuffer: usage.autoCompactBuffer ?? 0,
    compactCount: usage.compactCount ?? 0,
    updatedAt: usage.updatedAt ?? 0,
    breakdown: Array.isArray(usage.breakdown)
      ? usage.breakdown.map((category) => ({
          key: category.key ?? '',
          label: category.label ?? category.key ?? '',
          tokens: category.tokens ?? 0,
          estimated: Boolean(category.estimated),
        }))
      : [],
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
    models: provider.models?.map((model) => ({
      id: model.id,
      displayName: model.displayName,
      contextWindow: model.contextWindow,
      maxOutputTokens: model.maxOutputTokens,
    })),
    defaultContextWindow: provider.defaultContextWindow,
    enabled: true,
  };
}

function toRuntimeModelConfigRequest(request: ProviderDraftDiscoveryRequestViewModel): RuntimeModelConfigRequestDTO {
  return {
    protocol: request.protocol === 'anthropic' ? 'anthropic' : 'openai',
    url: request.apiEndpoint,
    apiKey: request.token,
    model: request.defaultModel,
    proxy: request.proxy,
  };
}

function mapDraftModelDiscovery(response: RuntimeModelDiscoveryResponseDTO): ProviderModelDiscoveryViewModel {
  return {
    providerId: 'draft',
    models: providerModelsFromIDs(response.models),
    error: response.error,
  };
}

function providerModelsFromIDs(models?: string[]): ProviderModelViewModel[] {
  return Array.isArray(models)
    ? models
        .map((id) => id.trim())
        .filter(Boolean)
        .map((id) => ({ id }))
    : [];
}

function mapProviderModel(model: ProviderModelViewModel): ProviderModelViewModel {
  // Backend contract: contextWindow / maxOutputTokens carry the user's
  // explicit values only; resolved values arrive in dedicated fields.
  // Discovery responses report provider-discovered values, which are never
  // user input: keep them on the resolved side only.
  const discovered = model.source === 'discovered';
  return {
    id: model.id,
    displayName: model.displayName,
    contextWindow: discovered ? undefined : model.contextWindow || undefined,
    maxOutputTokens: discovered ? undefined : model.maxOutputTokens || undefined,
    resolvedContextWindow: model.resolvedContextWindow ?? model.contextWindow ?? undefined,
    resolvedMaxOutputTokens: model.resolvedMaxOutputTokens ?? model.maxOutputTokens ?? undefined,
    source: model.source,
  };
}

function mapDraftProviderTest(response: RuntimeModelVerifyResponseDTO, durationMs?: number): ProviderTestViewModel {
  return {
    ok: Boolean(response.ok),
    providerId: 'draft',
    model: response.model,
    durationMs,
    error: response.error,
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

async function hydrateAgentTasks(bridge: RuntimeBridgeModule, sessionID?: string): Promise<AgentTaskViewModel[] | undefined> {
  if (!sessionID || !bridge.SessionAgentTasks) {
    return undefined;
  }
  const response = await optionalRuntimeRequest(() => bridge.SessionAgentTasks?.(sessionID) ?? Promise.resolve(undefined));
  const tasks = Array.isArray(response?.tasks) ? response.tasks : [];
  if (!tasks.length) {
    return [];
  }
  const detailed = await Promise.all(
    tasks.map(async (task) => {
      const [detail, output] = await Promise.all([
        optionalRuntimeRequest(() => bridge.AgentTask?.(task.id) ?? Promise.resolve(undefined)),
        optionalRuntimeRequest(() => bridge.AgentTaskOutput?.(task.id) ?? Promise.resolve(undefined)),
      ]);
      return mapAgentTask(detail?.task ?? task, detail?.messages, detail?.result, output);
    }),
  );
  return detailed.filter((task): task is AgentTaskViewModel => Boolean(task));
}

async function hydrateTodos(bridge: RuntimeBridgeModule, sessionID?: string): Promise<TodoSummaryViewModel | undefined> {
  if (!sessionID || !bridge.SessionTodos) {
    return undefined;
  }
  const response = await optionalRuntimeRequest(() => bridge.SessionTodos?.(sessionID) ?? Promise.resolve(undefined));
  return mapTodoSummary(response?.summary, sessionID);
}

function mapTodoSummary(summary?: RuntimeTodoSummaryDTO, fallbackSessionID = ''): TodoSummaryViewModel | undefined {
  if (!summary) {
    return undefined;
  }
  const items = (Array.isArray(summary.todos) ? summary.todos : []).map(mapTodoItem).filter((item): item is TodoItemViewModel => Boolean(item));
  const pending = typeof summary.pending === 'number' ? summary.pending : items.filter((item) => item.status === 'pending').length;
  const inProgress = typeof summary.inProgress === 'number' ? summary.inProgress : items.filter((item) => item.status === 'in_progress').length;
  const completed = typeof summary.completed === 'number' ? summary.completed : items.filter((item) => item.status === 'completed').length;
  return {
    sessionId: summary.sessionId || fallbackSessionID,
    turnId: summary.turnId,
    items,
    pending,
    inProgress,
    completed,
    total: typeof summary.total === 'number' ? summary.total : items.length,
    updatedAt: summary.updatedAt,
  };
}

function mapTodoItem(todo: RuntimeTodoDTO, index: number): TodoItemViewModel | undefined {
  const content = todo.content?.trim();
  if (!content) {
    return undefined;
  }
  const source = todo.source?.kind
    ? {
        kind: todo.source.kind,
        label: todo.source.label,
        ref: todo.source.ref,
      }
    : undefined;
  return {
    id: todo.id || `todo:${index + 1}:${content}`,
    content,
    status: todo.status || 'pending',
    activeForm: todo.activeForm || todo.active_form,
    createdAt: todo.createdAt,
    updatedAt: todo.updatedAt,
    source,
  };
}

async function hydrateAgentRoles(bridge: RuntimeBridgeModule): Promise<AgentRoleViewModel[] | undefined> {
  if (!bridge.AgentRoles) {
    return undefined;
  }
  const response = await optionalRuntimeRequest(() => bridge.AgentRoles?.() ?? Promise.resolve(undefined));
  const roles = Array.isArray(response?.roles) ? response.roles : undefined;
  return roles?.map(mapAgentRole).filter((role): role is AgentRoleViewModel => Boolean(role));
}

function mapAgentRole(role?: RuntimeAgentRoleDTO): AgentRoleViewModel | undefined {
  if (!role?.id) {
    return undefined;
  }
  return {
    id: role.id,
    name: role.name,
    title: role.title,
    description: role.description,
    promptSummary: role.promptSummary,
    allowedTools: role.allowedTools,
    capabilityScope: role.capabilityScope,
    model: role.model,
    provider: role.provider,
    cwd: role.cwd,
    worktree: role.worktree,
    source: role.source,
  };
}

function mapAgentTask(
  task?: RuntimeAgentTaskDTO,
  messages?: RuntimeAgentTaskMessageDTO[],
  result?: RuntimeAgentTaskResultDTO,
  output?: RuntimeAgentTaskOutputResponseDTO,
): AgentTaskViewModel | undefined {
  if (!task?.id) {
    return undefined;
  }
  const mappedResult = mapAgentTaskResult(result);
  return {
    id: task.id,
    parentSessionId: task.parentSessionId ?? '',
    parentTurnId: task.parentTurnId,
    parentToolCallId: task.parentToolCallId,
    childSessionId: task.childSessionId,
    title: task.title || task.name || task.id,
    kind: task.kind || 'subagent',
    role: task.role,
    name: task.name,
    promptSummary: task.promptSummary,
    model: task.model,
    provider: task.provider,
    allowedTools: task.allowedTools,
    capabilityScope: task.capabilityScope,
    cwd: task.cwd,
    worktree: task.worktree,
    status: output?.status || task.status || mappedResult?.status || 'unknown',
    progress: typeof task.progress === 'number' ? task.progress : 0,
    resultSummary: output?.summary || mappedResult?.summary || task.resultSummary,
    artifactRefs: uniqueStrings([...(task.artifactRefs ?? []), ...(mappedResult?.artifactRefs ?? []), ...(output?.artifactRefs ?? [])]),
    outputRefs: output?.outputRefs,
    compactBoundaryRefs: output?.compactBoundaryRefs ?? mappedResult?.compactBoundaryRefs,
    cancellationDetail: output?.cancellationDetail ?? mappedResult?.cancellationDetail ?? task.cancellationDetail,
    messages: (output?.messages ?? messages)?.map(mapAgentTaskMessage).filter((message): message is AgentTaskMessageViewModel => Boolean(message)),
    result: mappedResult,
    startedAt: task.startedAt,
    updatedAt: output?.updatedAt ?? task.updatedAt,
    finishedAt: task.finishedAt,
    error: output?.error || mappedResult?.errorDetail || task.error,
  };
}

function mapAgentTaskMessage(message?: RuntimeAgentTaskMessageDTO): AgentTaskMessageViewModel | undefined {
  if (!message?.id) {
    return undefined;
  }
  return {
    id: message.id,
    taskId: message.taskId ?? '',
    direction: message.direction ?? '',
    kind: message.kind ?? '',
    status: message.status ?? '',
    sequence: message.sequence,
    contentSummary: message.contentSummary,
    relatedToolCallId: message.relatedToolCallId,
    relatedMessageId: message.relatedMessageId,
    artifactRefs: message.artifactRefs,
    createdAt: message.createdAt,
    deliveredAt: message.deliveredAt,
    processedAt: message.processedAt,
    error: message.error,
  };
}

function mapAgentTaskResult(result?: RuntimeAgentTaskResultDTO): AgentTaskResultViewModel | undefined {
  if (!result?.taskId) {
    return undefined;
  }
  return {
    taskId: result.taskId,
    status: result.status ?? '',
    summary: result.summary,
    errorDetail: result.errorDetail,
    cancellationDetail: result.cancellationDetail,
    artifactRefs: result.artifactRefs,
    relatedMessageRefs: result.relatedMessageRefs,
    relatedToolCallRefs: result.relatedToolCallRefs,
    compactBoundaryRefs: result.compactBoundaryRefs,
    createdAt: result.createdAt,
    updatedAt: result.updatedAt,
  };
}

function uniqueStrings(values: Array<string | undefined>): string[] | undefined {
  const out = Array.from(new Set(values.map((value) => value?.trim()).filter((value): value is string => Boolean(value))));
  return out.length ? out : undefined;
}

interface RuntimeHydrateOptions {
  refreshTargets?: RuntimeActionRefreshTarget[];
}

function actionTargetsInclude(targets: RuntimeActionRefreshTarget[] | undefined, ...candidates: RuntimeActionRefreshTarget[]) {
  if (!targets || targets.length === 0) {
    return true;
  }
  return candidates.some((candidate) => targets.includes(candidate));
}

async function hydrateWorkbenchForAction(current: WorkbenchViewModel, bridge: RuntimeBridgeModule, response: unknown) {
  const refreshTargets = runtimeActionRefreshTargets(response);
  if (!refreshTargets) {
    return hydrateWorkbench(current, bridge);
  }
  return hydrateWorkbench(current, bridge, { refreshTargets });
}

async function hydrateWorkbench(current: WorkbenchViewModel, bridge: RuntimeBridgeModule, hydrateOptions: RuntimeHydrateOptions = {}) {
  const refreshTargets = hydrateOptions.refreshTargets;
  const fullHydration = !refreshTargets || refreshTargets.length === 0;
  const refreshActivity = actionTargetsInclude(
    refreshTargets,
    'recovery',
    'turn_activity',
    'session_activity_window',
    'session_activity',
    'tool_calls',
    'diagnostics',
    'permissions',
    'mcp_requests',
  );
  const refreshRuns = actionTargetsInclude(refreshTargets, 'run', 'run_projection', 'run_transition_history', 'run_scheduler_plan');
  const refreshPolicy = actionTargetsInclude(refreshTargets, 'permissions');
  const [
    status,
    sidebarProjection,
    recoveryStatus,
    sessionsResponse,
    projectsResponse,
    modelsResponse,
    providerCatalog,
    configuredProvidersResponse,
    activeTurnsResponse,
    skillsResponse,
    pluginsResponse,
    mcpServersResponse,
    terminalSettingsResponse,
  ] = await Promise.all([
    optionalRuntimeRequest(() => bridge.Status()),
    fullHydration ? optionalRuntimeRequest(() => bridge.SidebarProjection?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration || actionTargetsInclude(refreshTargets, 'recovery', 'turn_activity', 'permissions', 'mcp_requests', 'run')
      ? optionalRuntimeRequest(() => bridge.RecoveryStatus?.() ?? Promise.resolve(undefined))
      : Promise.resolve(undefined),
    optionalRuntimeRequest(() => bridge.Sessions()),
    fullHydration ? optionalRuntimeRequest(() => bridge.Projects?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration ? bridge.Models().catch(() => undefined) : Promise.resolve(undefined),
    fullHydration ? bridge.ProviderCatalog?.().catch(() => undefined) : Promise.resolve(undefined),
    fullHydration ? bridge.ConfiguredProviders?.().catch(() => undefined) : Promise.resolve(undefined),
    optionalRuntimeRequest(() => bridge.Turns?.('active') ?? Promise.resolve(undefined)),
    fullHydration ? optionalRuntimeRequest(() => bridge.Skills?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration ? optionalRuntimeRequest(() => bridge.Plugins?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration ? optionalRuntimeRequest(() => bridge.MCPServers?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration ? optionalRuntimeRequest(() => bridge.TerminalSettings?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
  ]);
  const projectedSessionsResponse: RuntimeSessionsResponseDTO | undefined = Array.isArray(sidebarProjection?.sessions)
    ? { sessions: sidebarProjection.sessions }
    : sessionsResponse;
  const projectedProjectsResponse: RuntimeProjectsResponseDTO | undefined = Array.isArray(sidebarProjection?.projects)
    ? { projects: sidebarProjection.projects }
    : projectsResponse;
  const activeSessionID = sidebarProjection?.activeSessionId || status?.sessionId || projectedSessionsResponse?.sessions?.find((session) => session.active)?.id;
  // Batch A: independent hydration requests that only depend on activeSessionID /
  // fullHydration / refreshActivity / refreshRuns — fired together to cut round trips.
  const [
    hooks,
    hookExecutions,
    outputSnapshot,
    activity,
    runProjection,
    agentTasks,
    todos,
    agentRoles,
    contextUsage,
  ] = await Promise.all([
    fullHydration ? hydrateHooks(bridge) : Promise.resolve(undefined),
    activeSessionID ? hydrateHookExecutions(bridge, activeSessionID) : Promise.resolve(summarizeHookExecutions([])),
    activeSessionID && refreshActivity
      ? optionalRuntimeRequest(() => bridge.SessionOutput?.(activeSessionID, { snapshot: true, limit: fullHydration ? undefined : 64 }) ?? Promise.resolve(undefined))
      : Promise.resolve(undefined),
    // Narrow-then-wide activity fallback stays sequential internally (narrow hint
    // must fail before we fall back to the full SessionActivity request).
    activeSessionID && refreshActivity
      ? (async () => {
          const narrowActivity = await hydrateNarrowActivityFromHint(bridge, activeSessionID)
          return narrowActivity ?? (await optionalRuntimeRequest(() => bridge.SessionActivity?.(activeSessionID) ?? Promise.resolve(undefined)))
        })()
      : Promise.resolve(undefined),
    activeSessionID && bridge.RunProjection && refreshRuns
      ? optionalRuntimeRequest(() => bridge.RunProjection?.({ sessionId: activeSessionID, limit: 24 }) ?? Promise.resolve(undefined))
      : Promise.resolve(undefined),
    activeSessionID ? hydrateAgentTasks(bridge, activeSessionID) : Promise.resolve(undefined),
    activeSessionID ? hydrateTodos(bridge, activeSessionID) : Promise.resolve(undefined),
    fullHydration ? hydrateAgentRoles(bridge) : Promise.resolve(undefined),
    activeSessionID ? optionalRuntimeRequest(() => bridge.SessionContextUsage?.(activeSessionID) ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
  ])
  const outputStore = outputSnapshot ? hydrateOutputStore(outputSnapshot, current.outputStore) : current.outputStore;
  const modelOptionList = modelsResponse ? modelOptions(modelsResponse) : current.composer.modelOptions;
  const selectedModel = modelsResponse ? modelOptionList.find((model) => model.selected) : current.composer.selectedModel;
  const currentProjectID = sidebarProjection?.currentProjectId || projectedProjectsResponse?.projects?.find((project) => project.current)?.id;
  const mappedProjects = mapProjects(projectedProjectsResponse, currentProjectID);
  const projects = projectedProjectsResponse ? mappedProjects : current.projects;
  const currentProject = projects.find((project) => project.current || project.id === currentProjectID) ?? mapProjectFromStatus(status, current.currentProject);
  const activeTurns = Array.isArray(activeTurnsResponse?.turns) ? activeTurnsResponse.turns : [];
  const activityTurns = Array.isArray(activity?.turns) ? activity.turns : [];
  const sessionActiveTurn =
    activeTurns.find((turn) => turn.sessionId === activeSessionID && !isFinalTurnStatus(turn.status)) ||
    activityTurns.find((turn) => !isFinalTurnStatus(turn.status));
  const busy =
    typeof status?.requests?.sessionBusy === 'boolean'
      ? status.requests.sessionBusy
      : Boolean(sessionActiveTurn);
  const activeTurnId = status?.requests?.sessionRequestId || sessionActiveTurn?.id || (busy ? current.composer.activeTurnId : undefined);
  // Batch B: requests that depend on Batch A results (runProjection / activity)
  // or on activeTurnId (derived synchronously above) — independent of each other.
  const [
    schedulerTaskCandidates,
    reactCallchainDTO,
    promptAssembliesDTO,
    policy,
  ] = await Promise.all([
    actionTargetsInclude(refreshTargets, 'run_scheduler_plan') ? hydrateRunSchedulerTaskCandidates(bridge, runProjection) : Promise.resolve([]),
    activeSessionID && refreshActivity
      ? hydrateReactCallchain(bridge, activeSessionID, activeTurnId)
      : Promise.resolve(undefined),
    activeSessionID && refreshActivity
      ? hydratePromptAssemblies(bridge, activeSessionID, activeTurnId)
      : Promise.resolve(undefined),
    activity?.policy ?? (refreshPolicy
      ? optionalRuntimeRequest(() => bridge.GetPolicy?.() ?? Promise.resolve(undefined)).then((response) => response?.policy)
      : undefined),
  ]);
  const contextDiagnostics = mapContextDiagnostics(promptAssembliesDTO) ?? (current.contextDiagnostics?.sessionId === activeSessionID ? current.contextDiagnostics : undefined);
  const activeOutputStore = outputStore?.sessionId === activeSessionID ? outputStore : undefined;
  const outputTimeline = activeOutputStore ? selectConversationTimeline(activeOutputStore) : undefined;
  const outputConversation = activeOutputStore ? selectConversationMessages(activeOutputStore) : undefined;
  const outputPendingPermissions = activeOutputStore ? selectPendingPermissions(activeOutputStore) : undefined;
  // Primary conversation rendering is owned by SessionOutput. SessionActivity,
  // prompt assemblies, and context diagnostics remain diagnostics-only and must
  // not reconstruct the main timeline if a runtime output snapshot is absent.
  const timeline = outputTimeline ?? (activeSessionID ? [] : current.timeline);
  const conversation = outputConversation ?? (activeSessionID ? [] : current.conversation);
  const pendingPermissions = outputPendingPermissions ?? current.pendingPermissions;
  const skills = mapSkills(skillsResponse) ?? current.settings.skills;
  const plugins = mapPlugins(pluginsResponse) ?? current.settings.plugins;
  const mcpServers = mapMCPServers(mcpServersResponse) ?? current.settings.mcpServers;
  const providers = mapProviderCatalogItems(providerCatalog) ?? current.settings.providers;
  const nextPermissionMode = policy ? permissionMode(policy) : (current.composer.permissionMode ?? permissionMode());
  const nextSettingsPermissionMode = policy ? permissionMode(policy) : (current.settings.permissionMode ?? permissionMode());

  return {
    ...current,
    currentProject,
    projects,
    sessions: mapSessions(projectedSessionsResponse, activeSessionID, activeTurns),
    conversation,
    timeline,
    outputStore,
    turnDiagnostics: activity ? selectTurnDiagnostics(activity, sessionActiveTurn?.id) : current.turnDiagnostics,
    runProjection: mapRunProjection(runProjection, schedulerTaskCandidates) ?? (current.runProjection?.primarySessionId === activeSessionID ? current.runProjection : undefined),
    agentTasks: agentTasks ?? (activeSessionID ? current.agentTasks?.filter((task) => task.parentSessionId === activeSessionID || task.childSessionId === activeSessionID) : []),
    agentRoles: agentRoles ?? current.agentRoles,
    todos: todos ?? (current.todos?.sessionId === activeSessionID ? current.todos : undefined),
    reactCallchain: mapReactCallchain(reactCallchainDTO, hookExecutions) ?? (current.reactCallchain?.sessionId === activeSessionID ? current.reactCallchain : undefined),
    contextDiagnostics,
    recovery: recoveryStatus ? mapRecoveryStatus(recoveryStatus) : current.recovery,
    hooks: hooks ?? current.hooks ?? [],
    hookExecutions: hookExecutions ?? (activeSessionID ? undefined : summarizeHookExecutions([])),
    pendingPermissions,
    composer: {
      ...current.composer,
      permissionLabel: nextPermissionMode.label,
      permissionMode: nextPermissionMode,
      permissionOptions: permissionModeOptions,
      modelLabel: modelLabel(status, modelsResponse),
      capabilityLabel: capabilityLabel(skills, mcpServers),
      contextUsage: mapContextUsage(contextUsage) ?? (current.composer.contextUsage?.sessionId === activeSessionID ? current.composer.contextUsage : undefined),
      selectedModel,
      modelOptions: modelOptionList,
      busy,
      activeTurnId,
    },
    settings: {
      ...current.settings,
      permissionMode: nextSettingsPermissionMode,
      permissionOptions: permissionModeOptions,
      permissions: policy ? settingsPermissions(policy) : current.settings.permissions,
      providerTypes: providerCatalog?.providerTypes ?? current.settings.providerTypes,
      providers,
      configuredProviders: mapConfiguredProviders(configuredProvidersResponse) ?? current.settings.configuredProviders,
      terminalProfile: terminalSettingsResponse?.settings?.profileId ?? current.settings.terminalProfile,
      terminalOptions: mapTerminalProfileOptions(terminalSettingsResponse) ?? current.settings.terminalOptions,
      plugins,
      skills,
      mcpServers,
      mcpToolsByServer: current.settings.mcpToolsByServer,
      mcpResourcesByServer: current.settings.mcpResourcesByServer,
      mcpPromptsByServer: current.settings.mcpPromptsByServer,
    },
  };
}

async function hydrateReactCallchain(bridge: RuntimeBridgeModule, sessionID: string, turnID?: string) {
  return readReactCallchain(bridge, sessionID, turnID);
}

async function readReactCallchain(bridge: RuntimeBridgeModule, sessionID: string, turnID?: string) {
  return optionalRuntimeRequest(() => {
    if (turnID && bridge.ReactCallchain) {
      return bridge.ReactCallchain(turnID);
    }
    return bridge.SessionReactCallchain?.(sessionID, 6) ?? Promise.resolve(undefined);
  });
}

async function hydratePromptAssemblies(bridge: RuntimeBridgeModule, sessionID: string, turnID?: string) {
  return readPromptAssemblies(bridge, sessionID, turnID);
}

async function readPromptAssemblies(bridge: RuntimeBridgeModule, sessionID: string, turnID?: string) {
  return optionalRuntimeRequest(() => {
    if (turnID && bridge.TurnPromptAssemblies) {
      return bridge.TurnPromptAssemblies(turnID);
    }
    return bridge.SessionPromptAssemblies?.(sessionID, 6) ?? Promise.resolve(undefined);
  });
}

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

async function subscribeRuntimeBridgeEvents(bridge: RuntimeBridgeModule, onEvent: (event: RuntimeEventViewModel) => void) {
  return subscribeRuntimeEventsByPolling(bridge, onEvent);
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
      if (!bridge.Events) {
        return;
      }
      const response = await bridge.Events(lastSequence);
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

async function subscribeWailsRuntimeTerminalStream(
  bridge: RuntimeBridgeModule,
  terminalID: string,
  onEvent: (event: TerminalEventViewModel) => void,
) {
  if (!bridge.StartTerminalEventStream || !bridge.AckTerminalEventStream || !bridge.StopTerminalEventStream) {
    throw new Error('terminal Wails stream bindings are unavailable');
  }

  let lastSequence = 0;
  let closed = false;
  const requestedStreamID = createTerminalStreamID(terminalID);
  const eventName = 'agent-builder:terminal-stream';
  const wailsRuntime = await loadWailsRuntime();
  const off = wailsRuntime.Events.On(eventName, (event) => {
    if (closed) {
      return;
    }
    const message = event.data as RuntimeTerminalStreamMessageDTO | undefined;
    if (!message || message.streamId !== requestedStreamID || (message.terminalId && message.terminalId !== terminalID)) {
      return;
    }
    const events = Array.isArray(message.events) ? message.events : [];
    if (events.length === 0) {
      return;
    }
    const mapped = mapTerminalEventBatch(events);
    if (mapped.sequence > lastSequence) {
      lastSequence = mapped.sequence;
    }
    onEvent({
      ...mapped,
      acknowledge: () => {
        void bridge.AckTerminalEventStream?.({ streamId: requestedStreamID, sequence: mapped.sequence });
      },
    });
  });
  let stream: RuntimeTerminalStreamResponseDTO;
  try {
    stream = await bridge.StartTerminalEventStream({ terminalId: terminalID, streamId: requestedStreamID, after: lastSequence });
  } catch (error) {
    closed = true;
    off();
    throw error;
  }

  return () => {
    closed = true;
    off();
    void bridge.StopTerminalEventStream?.({ streamId: stream.streamId || requestedStreamID });
  };
}

function createTerminalStreamID(terminalID: string) {
  const random =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `terminal-stream:${terminalID}:${random}`;
}

function mapTerminalEventBatch(events: RuntimeTerminalEventDTO[]): TerminalEventViewModel {
  const mapped = events.map(mapTerminalEvent);
  if (mapped.length === 0) {
    return {
      terminalId: '',
      sequence: 0,
    };
  }
  const last = mapped[mapped.length - 1];
  return {
    terminalId: last.terminalId,
    sequence: mapped.reduce((max, event) => Math.max(max, event.sequence), 0),
    chunks: mapped.map((event) => ({
      data: event.data,
      binaryBase64: event.binaryBase64,
    })),
    final: mapped.some((event) => event.final),
    status: last.status,
    exitCode: last.exitCode,
    error: mapped.map((event) => event.error).filter(Boolean).join('\n') || undefined,
  };
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

function mapTerminalDTO(terminal: RuntimeTerminalDTO): TerminalViewModel {
  return {
    id: terminal.id,
    projectId: terminal.projectId,
    sessionId: terminal.sessionId ?? '',
    title: terminal.title,
    cwd: terminal.cwd,
    initialCwd: terminal.initialCwd,
    shell: terminal.shell,
    shellPath: terminal.shellPath,
    shellArgs: terminal.shellArgs,
    columns: terminal.columns,
    rows: terminal.rows,
    status: terminal.status || 'running',
    exitCode: terminal.exitCode,
    createdAt: terminal.createdAt,
    updatedAt: terminal.updatedAt,
  };
}

function mapTerminal(response: RuntimeTerminalResponseDTO): TerminalViewModel {
  return mapTerminalDTO(response.terminal);
}

function mapSessionTerminals(response?: RuntimeSessionTerminalsResponseDTO): TerminalViewModel[] {
  return Array.isArray(response?.terminals) ? response.terminals.map(mapTerminalDTO) : [];
}

function mapTerminalProfileOptions(response?: RuntimeTerminalSettingsResponseDTO): SettingsOptionViewModel[] | undefined {
  const profiles = response?.settings?.profiles;
  if (!Array.isArray(profiles)) {
    return undefined;
  }
  return profiles.map((profile) => ({
    label: profile.label || profile.id,
    value: profile.id,
  }));
}

function mapTerminalEvent(event: RuntimeTerminalEventDTO): TerminalEventViewModel {
  return {
    terminalId: event.terminalId ?? event.terminal_id ?? '',
    sequence: event.sequence ?? 0,
    data: event.data,
    binaryBase64: event.binaryBase64 ?? event.binary_base64,
    final: event.final,
    status: event.status,
    exitCode: event.exitCode ?? event.exit_code,
    error: event.error,
  };
}

async function withBridge(
  run: (bridge: RuntimeBridgeModule) => Promise<WorkbenchViewModel>,
  fallback: () => Promise<WorkbenchViewModel>,
) {
  const bridge = await loadRuntimeBridge();
  if (!bridge?.Status) {
    throw new Error('runtime Wails bindings are unavailable');
  }
  try {
    return await run(bridge);
  } catch (error) {
    console.warn('[runtime] Wails bridge failed', error);
    if (hasProviderSettingsBridge(bridge)) {
      try {
        return await hydrateSettingsOnly(await fallback(), bridge);
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
  const hooks = await hydrateHooks(bridge);
  const providers = mapProviderCatalogItems(providerCatalog) ?? current.settings.providers;

  return {
    ...current,
    hooks: hooks ?? current.hooks ?? [],
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

function resetConversationRuntimeState(current: WorkbenchViewModel): WorkbenchViewModel {
  return {
    ...current,
    conversation: [],
    timeline: [],
    turnDiagnostics: undefined,
    runProjection: undefined,
    reactCallchain: undefined,
    contextDiagnostics: undefined,
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
  async openProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.OpenProject) {
      await bridge.OpenProject(request);
      return hydrateWorkbench(nextBase, bridge);
    }
    throw new Error('open project Wails binding is unavailable');
  },
  async createProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.CreateProject) {
      await bridge.CreateProject(request);
      return hydrateWorkbench(nextBase, bridge);
    }
    throw new Error('create project Wails binding is unavailable');
  },
  async renameProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.RenameProject) {
      await bridge.RenameProject(request);
      return hydrateWorkbench(nextBase, bridge);
    }
    throw new Error('rename project Wails binding is unavailable');
  },
  async openProjectInExplorer(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.OpenProjectInExplorer) {
      await bridge.OpenProjectInExplorer(request);
      return;
    }
    throw new Error('open project in explorer Wails binding is unavailable');
  },
  async removeProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.RemoveProject) {
      await bridge.RemoveProject(request);
      return hydrateWorkbench(nextBase, bridge);
    }
    throw new Error('remove project Wails binding is unavailable');
  },
  async selectProjectDirectory() {
    const bridge = await loadRuntimeBridge();
    if (bridge?.SelectProjectDirectory) {
      return bridge.SelectProjectDirectory();
    }
    return staticWorkbenchAdapter.selectProjectDirectory();
  },
  async fetchContextUsage(sessionID) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.SessionContextUsage) {
      return undefined;
    }
    try {
      return mapContextUsage(await bridge.SessionContextUsage(sessionID));
    } catch (error) {
      console.warn('[runtime] fetchContextUsage failed', error);
      return undefined;
    }
  },
  async subscribeRuntimeEvents(onEvent) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.Events || bridge?.EventsEndpoint) {
      return subscribeRuntimeBridgeEvents(bridge, onEvent);
    }
    return () => undefined;
  },
  async subscribeSessionOutput(sessionID, handlers, after) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.SessionOutputEvents && !bridge?.StartSessionOutputStream) {
      return () => undefined;
    }
    const { subscribeSessionOutput } = await import('./outputStream.ts');
    return subscribeSessionOutput({
      sessionId: sessionID,
      after,
      bridge: bridge as unknown as Parameters<typeof subscribeSessionOutput>[0]['bridge'],
      loadWailsEvents: async () => {
        try {
          return (await loadWailsRuntime()) as unknown as Parameters<typeof subscribeSessionOutput>[0]['loadWailsEvents'] extends (() => infer R) ? Awaited<R> : never;
        } catch { return null; }
      },
      onBatch: handlers.onEvents,
      onSnapshotRequired: handlers.onSnapshotRequired,
    });
  },
  async loadHookExecution(executionID) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.HookExecution) {
      throw new Error('runtime hook execution API is unavailable');
    }
    const response = await bridge.HookExecution(executionID);
    if (!response.execution) {
      throw new Error('runtime hook execution was not found');
    }
    return mapHookExecution(response.execution);
  },
  async createSession(current, target) {
    forceDraftChatSubmit = false;
    const draft = target ?? current.newConversationDraft ?? defaultDraftTarget(current);
    const draftViewModel = resetConversationRuntimeState({
      ...current,
      mode: 'new-chat',
      newConversationDraft: draft,
      sessions: current.sessions.map((session) => ({ ...session, active: false })),
    });
    return withBridge(
      async (bridge) => {
        // A new conversation is intentionally only a draft until its first
        // prompt is submitted, but the runtime must still clear its active
        // session. Otherwise the next event-driven hydration reads the old
        // activeSessionId and visibly switches the UI back to that session.
        await bridge.NewChat('');
        return draftViewModel;
      },
      () => staticWorkbenchAdapter.createSession(draftViewModel, draft),
    );
  },
  async selectSession(current, sessionID) {
    forceDraftChatSubmit = false;
    return withBridge(
      async (bridge) => {
        await bridge.SelectSession(sessionID);
        const hydrated = await hydrateWorkbench(
          resetConversationRuntimeState({ ...current, mode: 'new-chat' }),
          bridge,
          { refreshTargets: ['session_activity', 'run_projection'] },
        );
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
          { refreshTargets: ['status'] },
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
  async decidePermission(current, permissionID, action, guidance) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DecidePermission) {
          return staticWorkbenchAdapter.decidePermission(current, permissionID, action, guidance);
        }
        const response = await bridge.DecidePermission({ permissionId: permissionID, action, guidance });
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.decidePermission(current, permissionID, action, guidance),
    );
  },
  async sendPrompt(current, prompt, options) {
    return wailsWorkbenchAdapter.submitUserInput?.(current, promptToUserInput(prompt, options?.clientRequestId)) ?? staticWorkbenchAdapter.sendPrompt(current, prompt);
  },
  async submitUserInput(current, input) {
    const prompt = input.items.map((item) => item.text).filter(Boolean).join('\n\n').trim();
    const activeSessionID = forceDraftChatSubmit || current.newConversationDraft ? undefined : current.sessions.find((session) => session.active)?.id;
    const draftTarget = activeSessionID ? undefined : (current.newConversationDraft ?? defaultDraftTarget(current));

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
            () =>
              bridge.SubmitUserInput
                ? bridge.SubmitUserInput(toRuntimeUserInputRequest(input, activeSessionID, draftTarget))
                : bridge.Chat({
                    prompt,
                    sessionId: activeSessionID,
                    projectId: draftTarget?.scope === 'project' ? draftTarget.projectId : undefined,
                    scope: draftTarget?.scope,
                  }),
            runtimeMutationTimeoutMS,
            '运行时响应超时，请稍后刷新会话查看结果。',
          );
          const responseSessionID = response.status.sessionId || activeSessionID;
          forceDraftChatSubmit = false;
          const busyAfterSubmit = Boolean(response.turnId);
          const normalizedConversation = mapNormalizedInputConversation(response, prompt);
          const normalizedTimeline = mapNormalizedInputTimeline(response, prompt);
          const normalizedOnly = !response.turnId && response.normalizedInput;
          const nextOutputStore =
            responseSessionID && current.outputStore?.sessionId !== responseSessionID
              ? createOutputStore(responseSessionID)
              : current.outputStore;
          return {
            ...optimistic,
            mode: 'new-chat',
            newConversationDraft: undefined,
            sessions: sessionsAfterDraftSubmit(current, responseSessionID, prompt, draftTarget, response.turnId),
            outputStore: nextOutputStore,
            conversation:
              normalizedOnly
                ? mergeNormalizedConversation(current.conversation, normalizedConversation)
                : optimistic.conversation,
            timeline: normalizedOnly ? mergeNormalizedTimeline(current.timeline, normalizedTimeline) : optimistic.timeline,
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
            newConversationDraft: draftTarget,
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
        let response: RuntimeStatusDTO | undefined;
        if (targetTurnID && bridge.CancelTurn) {
          response = await bridge.CancelTurn(targetTurnID);
        }
        const hydrated = await hydrateWorkbenchForAction(
          {
            ...current,
            composer: { ...current.composer, busy: false, activeTurnId: undefined },
          },
          bridge,
          response,
        );
        return {
          ...hydrated,
          composer: { ...hydrated.composer, busy: false, activeTurnId: undefined },
        };
      },
      () => staticWorkbenchAdapter.cancelTurn(current, turnID),
    );
  },
  async resumeInterruptedTurn(current, turnID, request) {
    return withBridge(
      async (bridge) => {
        if (!bridge.ResumeInterruptedTurn) {
          return staticWorkbenchAdapter.resumeInterruptedTurn(current, turnID, request);
        }
        const response = await bridge.ResumeInterruptedTurn(turnID, {
          mode: request?.mode ?? 'continue',
          prompt: request?.prompt,
        });
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.resumeInterruptedTurn(current, turnID, request),
    );
  },
  async markInterruptedDone(current, turnID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.MarkInterruptedDone) {
          return staticWorkbenchAdapter.markInterruptedDone(current, turnID);
        }
        const response = await bridge.MarkInterruptedDone(turnID);
        return hydrateWorkbenchForAction(
          {
            ...current,
          },
          bridge,
          response,
        );
      },
      () => staticWorkbenchAdapter.markInterruptedDone(current, turnID),
    );
  },
  async discardInterruptedTurn(current, turnID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DiscardInterruptedTurn) {
          return staticWorkbenchAdapter.discardInterruptedTurn(current, turnID);
        }
        const response = await bridge.DiscardInterruptedTurn(turnID);
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.discardInterruptedTurn(current, turnID),
    );
  },
  async retryRecoverableError(current, errorID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.RetryRecoverableError) {
          return staticWorkbenchAdapter.retryRecoverableError(current, errorID);
        }
        const response = await bridge.RetryRecoverableError(errorID);
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.retryRecoverableError(current, errorID),
    );
  },
  async manualCompact(current, instructions) {
    // B12: when there is no active/known turn, reject up front with a
    // friendly message instead of letting contextActionRequest's throw fall
    // into withBridge's generic catch — that path swallows the real error
    // and replaces it with the static adapter's "runtime unavailable", which
    // is meaningless to the user here (see runtime unavailable() fallback
    // below for other bridge failures, which is the case that message is
    // actually meant for).
    if (!resolveContextActionTurnId(current)) {
      return Promise.reject(new Error('当前没有可压缩的对话轮次'));
    }
    return withBridge(
      async (bridge) => {
        if (!bridge.ManualCompact) {
          return staticWorkbenchAdapter.manualCompact(current, instructions);
        }
        const req = contextActionRequest(current, 'manual');
        if (instructions?.trim()) {
          req.instructions = instructions.trim();
        }
        await bridge.ManualCompact(req);
        return hydrateWorkbench(current, bridge, { refreshTargets: ['session_activity'] });
      },
      () => staticWorkbenchAdapter.manualCompact(current, instructions),
    );
  },
  async manualSnip(current, reason) {
    return withBridge(
      async (bridge) => {
        if (!bridge.ManualSnip) {
          return staticWorkbenchAdapter.manualSnip(current, reason);
        }
        const req = contextActionRequest(current, reason || 'manual_snip');
        await bridge.ManualSnip(req);
        return hydrateWorkbench(current, bridge, { refreshTargets: ['session_activity'] });
      },
      () => staticWorkbenchAdapter.manualSnip(current, reason),
    );
  },
  async resumeRunCheckpoint(current, runID, checkpointID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.ResumeRunCheckpoint) {
          return staticWorkbenchAdapter.resumeRunCheckpoint(current, runID, checkpointID);
        }
        const response = await bridge.ResumeRunCheckpoint(runID, checkpointID);
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.resumeRunCheckpoint(current, runID, checkpointID),
    );
  },
  async readRunSchedulerPlan(current, request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.RunSchedulerPlan) {
      return mapRunSchedulerPlanCandidates(await bridge.RunSchedulerPlan(toRunSchedulerPlanRequestDTO(request)));
    }
    return staticWorkbenchAdapter.readRunSchedulerPlan(current, request);
  },
  async executeRunTask(current, runID, taskID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.ExecuteRunTask) {
          return staticWorkbenchAdapter.executeRunTask(current, runID, taskID);
        }
        const response = await bridge.ExecuteRunTask(runID, taskID);
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.executeRunTask(current, runID, taskID),
    );
  },
  async sendAgentTaskFollowUp(current, taskID, taskMessage) {
    return withBridge(
      async (bridge) => {
        if (!bridge.AgentTaskFollowUp) {
          return staticWorkbenchAdapter.sendAgentTaskFollowUp(current, taskID, taskMessage);
        }
        await bridge.AgentTaskFollowUp(taskID, {
          direction: 'parent_to_child',
          kind: 'instruction',
          contentSummary: taskMessage,
        });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.sendAgentTaskFollowUp(current, taskID, taskMessage),
    );
  },
  async cancelAgentTask(current, taskID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.CancelAgentTask) {
          return staticWorkbenchAdapter.cancelAgentTask(current, taskID);
        }
        const response = await bridge.CancelAgentTask(taskID);
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.cancelAgentTask(current, taskID),
    );
  },
  async listSessionTerminals(sessionID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.SessionTerminals) {
      return mapSessionTerminals(await bridge.SessionTerminals(sessionID));
    }
    throw new Error('terminal Wails bindings are unavailable');
  },
  async createTerminal(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.CreateTerminal) {
      return mapTerminal(await bridge.CreateTerminal(request));
    }
    throw new Error('terminal Wails bindings are unavailable');
  },
  async writeTerminalInput(terminalID, data) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.WriteTerminalInput) {
      return mapTerminal(await bridge.WriteTerminalInput(terminalID, { data }));
    }
    throw new Error('terminal Wails input binding is unavailable');
  },
  async resizeTerminal(terminalID, columns, rows) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.ResizeTerminal) {
      return mapTerminal(await bridge.ResizeTerminal(terminalID, { columns, rows }));
    }
    throw new Error('terminal Wails resize binding is unavailable');
  },
  async subscribeTerminalEvents(terminalID, onEvent) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.StartTerminalEventStream) {
      return subscribeWailsRuntimeTerminalStream(bridge, terminalID, onEvent);
    }
    throw new Error('terminal Wails stream binding is unavailable');
  },
  async deleteTerminal(terminalID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DeleteTerminal) {
      await bridge.DeleteTerminal(terminalID);
      return;
    }
    throw new Error('terminal Wails bindings are unavailable');
  },
  async selectTerminalProfile(current, profileID) {
    return withBridge(
      async (bridge) => {
        if (!bridge.SaveTerminalSettings) {
          throw new Error('terminal settings Wails binding is unavailable');
        }
        await bridge.SaveTerminalSettings({ profileId: profileID });
        return hydrateWorkbench(current, bridge);
      },
      () => staticWorkbenchAdapter.selectTerminalProfile(current, profileID),
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
        let response: RuntimeConfiguredProvidersResponseDTO;
        try {
          response = await bridge.DeleteConfiguredProvider(providerID);
        } catch (error) {
          if (!isConfiguredProviderNotFound(error) || !bridge.ConfiguredProviders) {
            throw error;
          }
          response = await bridge.ConfiguredProviders();
        }
        const configuredProviders = mapConfiguredProviders(response);
        const hydrated = await hydrateWorkbench(current, bridge);
        return configuredProviders
          ? {
              ...hydrated,
              settings: {
                ...hydrated.settings,
                configuredProviders,
              },
            }
          : hydrated;
      },
      () => staticWorkbenchAdapter.deleteConfiguredProvider(current, providerID),
    );
  },
  async discoverProviderDraftModels(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DiscoverModelConfig) {
      return mapDraftModelDiscovery(await bridge.DiscoverModelConfig(toRuntimeModelConfigRequest(request)));
    }
    return staticWorkbenchAdapter.discoverProviderDraftModels(request);
  },
  async discoverConfiguredProviderModels(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DiscoverConfiguredProviderModels) {
      const response = await bridge.DiscoverConfiguredProviderModels(providerID);
      return {
        ...response,
        models: response.models.map(mapProviderModel),
      };
    }
    return staticWorkbenchAdapter.discoverConfiguredProviderModels(providerID);
  },
  async testProviderDraft(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.VerifyModelConfig) {
      return mapDraftProviderTest(await bridge.VerifyModelConfig(toRuntimeModelConfigRequest(request)));
    }
    return staticWorkbenchAdapter.testProviderDraft(request);
  },
  async testConfiguredProvider(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.TestConfiguredProvider) {
      return bridge.TestConfiguredProvider(providerID);
    }
    return staticWorkbenchAdapter.testConfiguredProvider(providerID);
  },
  async measureProviderDraftLatency(request) {
    const startedAt = performance.now();
    const bridge = await loadRuntimeBridge();
    if (bridge?.VerifyModelConfig) {
      return mapDraftProviderTest(await bridge.VerifyModelConfig(toRuntimeModelConfigRequest(request)), Math.round(performance.now() - startedAt));
    }
    return staticWorkbenchAdapter.measureProviderDraftLatency(request);
  },
  async measureConfiguredProviderLatency(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.MeasureConfiguredProviderLatency) {
      return bridge.MeasureConfiguredProviderLatency(providerID);
    }
    return staticWorkbenchAdapter.measureConfiguredProviderLatency(providerID);
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
  async listProjectMemories(projectID) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.ProjectMemories) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryList(await bridge.ProjectMemories(projectID));
  },
  async getProjectMemory(memoryID) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.ProjectMemory) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryRecord((await bridge.ProjectMemory(memoryID)).record);
  },
  async createProjectMemory(request) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.CreateProjectMemory) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryRecord(await bridge.CreateProjectMemory(request));
  },
  async updateProjectMemory(memoryID, request) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.UpdateProjectMemory) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryRecord(await bridge.UpdateProjectMemory(memoryID, request));
  },
  async setProjectMemoryEnabled(memoryID, enabled) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.DisableProjectMemory) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryRecord(await bridge.DisableProjectMemory(memoryID, { enabled }));
  },
  async deleteProjectMemory(memoryID, reason) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.DeleteProjectMemory) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryRecord(await bridge.DeleteProjectMemory(memoryID, { reason }));
  },
  async refreshProjectMemoryIndex(projectID) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.RefreshProjectMemoryIndex) {
      throw new Error('project memory runtime binding is unavailable');
    }
    return mapProjectMemoryIndex(await bridge.RefreshProjectMemoryIndex(projectID));
  },
  async getContextGovernanceSettings() {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.ContextGovernanceSettings) {
      throw new Error('context governance runtime binding is unavailable');
    }
    return mapContextGovernanceSettings((await bridge.ContextGovernanceSettings()).settings);
  },
  async saveContextGovernanceSettings(settings) {
    const bridge = await loadRuntimeBridge();
    if (!bridge?.SaveContextGovernanceSettings) {
      throw new Error('context governance runtime binding is unavailable');
    }
    const response = await bridge.SaveContextGovernanceSettings(toContextGovernanceSettingsRequest(settings));
    return mapContextGovernanceSettings(response.settings);
  },
};

function mapProjectMemoryList(response: RuntimeMemoryListResponseDTO): ProjectMemoryListViewModel {
  return {
    projectId: response.projectId,
    root: response.root,
    records: (response.records ?? []).map(mapProjectMemoryRecord),
  };
}

function mapProjectMemoryRecord(record: RuntimeMemoryRecordDTO): ProjectMemoryRecordViewModel {
  return {
    id: record.id,
    projectId: record.projectId,
    relativePath: record.relativePath,
    absolutePath: record.absolutePath,
    type: record.type,
    title: record.title,
    description: record.description,
    tags: record.tags ?? [],
    enabled: Boolean(record.enabled),
    deletedAt: record.deletedAt,
    contentHash: record.contentHash,
    tokenEstimate: record.tokenEstimate ?? 0,
    createdAt: record.createdAt,
    updatedAt: record.updatedAt,
    lastIndexedAt: record.lastIndexedAt,
    lastInjectedAt: record.lastInjectedAt,
    preview: record.preview,
    content: record.content,
  };
}

function mapProjectMemoryIndex(response: RuntimeMemoryIndexResponseDTO): ProjectMemoryIndexViewModel {
  return {
    projectId: response.projectId,
    indexed: response.indexed,
    deleted: response.deleted,
    failed: response.failed,
    startedAt: response.startedAt,
    endedAt: response.endedAt,
  };
}

function runtimeErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : '运行时请求失败';
}
