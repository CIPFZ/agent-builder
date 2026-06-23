import type {
  AgentTaskMessageViewModel,
  AgentTaskResultViewModel,
  AgentTaskViewModel,
  ConfiguredProviderViewModel,
  ContextDiagnosticsViewModel,
  ConversationMessageViewModel,
  ConversationTimelineItemViewModel,
  CreateProjectRequestViewModel,
  InterruptedTurnViewModel,
  PermissionModeOptionViewModel,
  PermissionRequestViewModel,
  ProviderDraftDiscoveryRequestViewModel,
  ProviderModelDiscoveryViewModel,
  ProviderTestViewModel,
  ProviderCatalogItemViewModel,
  ProjectActionRequestViewModel,
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
  TerminalEventViewModel,
  RunProjectionViewModel,
  RunSchedulerPlanRequestViewModel,
  RunSchedulerTaskCandidateViewModel,
  ToolCallViewModel,
  RuntimeUserInputRequestViewModel,
  RuntimeSkillViewModel,
  SettingsPermissionViewModel,
  TerminalViewModel,
  WorkbenchAdapter,
  WorkbenchViewModel,
} from './workbenchTypes.ts';
import { runtimeActionRefreshTargets, type RuntimeActionRefreshTarget, type RuntimeWriteActionResponseDTO } from './actionRefreshSelector.ts';
import { getInitialWorkbenchViewModel, staticWorkbenchAdapter } from './staticWorkbenchAdapter.tsx';

interface RuntimeStatusDTO extends RuntimeWriteActionResponseDTO {
  sessionId?: string;
  workingDir?: string;
  workspaceId?: string;
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
  isGitRepository?: boolean;
  branch?: string;
  current?: boolean;
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
  models?: string[];
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
  models?: string[];
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
  events?: RuntimeTerminalEventDTO[];
  error?: string;
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

interface RuntimeTimelineOrder {
  sequenceByMessageID: Map<string, number>;
  sequenceByToolCallID: Map<string, number>;
  sequenceByPermissionID: Map<string, number>;
  terminalByTurnID: Map<string, RuntimeReactCallNodeDTO>;
  assistantStepByMessageID: Map<string, RuntimeReactCallNodeDTO>;
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

interface RuntimePromptAssemblyDTO {
  id?: string;
  sessionId?: string;
  turnId?: string;
  step?: number;
  model?: string;
  provider?: string;
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
    instructionHash?: string;
    tokenEstimate?: number;
    rawContentStored?: boolean;
  };
  contextSources?: RuntimeContextSourceDTO[];
  compact?: RuntimeCompactBoundaryDTO[];
  budget?: RuntimeBudgetReportDTO;
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
  OpenProject?: (req: OpenProjectRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  CreateProject?: (req: CreateProjectRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  RenameProject?: (req: RenameProjectRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  OpenProjectInExplorer?: (req: ProjectActionRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
  RemoveProject?: (req: ProjectActionRequestViewModel) => Promise<RuntimeOpenProjectResponseDTO>;
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
  CreateTerminal?: (req: { sessionId: string; id?: string; cwd?: string; columns?: number; rows?: number }) => Promise<RuntimeTerminalResponseDTO>;
  DeleteTerminal?: (terminalID: string) => Promise<RuntimeTerminalResponseDTO>;
  CancelTurn?: (turnID: string) => Promise<RuntimeStatusDTO>;
  MarkInterruptedDone?: (turnID: string) => Promise<RuntimeTurnResponseDTO>;
  Messages?: () => Promise<RuntimeMessagesResponseDTO>;
  SessionMessages?: (sessionID: string) => Promise<RuntimeMessagesResponseDTO>;
  SessionActivity?: (sessionID: string) => Promise<RuntimeSessionActivityDTO>;
  SessionActivityWindow?: (sessionID: string, limit: number) => Promise<RuntimeSessionActivityWindowDTO>;
  SessionActivityCursorWindow?: (sessionID: string, cursor: string, limit: number) => Promise<RuntimeSessionActivityWindowDTO>;
  TurnActivity?: (turnID: string) => Promise<RuntimeTurnActivityDTO>;
  ReactCallchain?: (turnID: string) => Promise<RuntimeReactCallchainDTO>;
  SessionReactCallchain?: (sessionID: string, limit: number) => Promise<RuntimeReactCallchainDTO>;
  TurnPromptAssemblies?: (turnID: string) => Promise<RuntimePromptAssembliesResponseDTO>;
  SessionPromptAssemblies?: (sessionID: string, limit: number) => Promise<RuntimePromptAssembliesResponseDTO>;
  RunProjection?: (req: RuntimeRunProjectionRequestDTO) => Promise<RuntimeRunProjectionResponseDTO>;
  RunSummaries?: () => Promise<RuntimeRunSummariesResponseDTO>;
  RunSummary?: (runID: string) => Promise<RuntimeRunSummaryResponseDTO>;
  RunCheckpointMarkers?: (runID: string) => Promise<RuntimeRunCheckpointMarkersResponseDTO>;
  RunCheckpointMarker?: (runID: string, checkpointID: string) => Promise<RuntimeRunCheckpointMarkerResponseDTO>;
  RunSchedulerPlan?: (req: RuntimeRunSchedulerPlanRequestDTO) => Promise<RuntimeRunSchedulerPlanResponseDTO>;
  ResumeRunCheckpoint?: (runID: string, checkpointID: string) => Promise<RuntimeRunResumeResponseDTO>;
  ExecuteRunTask?: (runID: string, taskID: string) => Promise<RuntimeRunSchedulerExecuteTaskResponseDTO>;
  SessionAgentTasks?: (sessionID: string) => Promise<RuntimeAgentTasksResponseDTO>;
  AgentTask?: (taskID: string) => Promise<RuntimeAgentTaskResponseDTO>;
  AgentTaskFollowUp?: (taskID: string, req: { direction: string; kind: string; contentSummary: string }) => Promise<RuntimeAgentTaskMessageResponseDTO>;
  CancelAgentTask?: (taskID: string) => Promise<RuntimeAgentTaskResponseDTO>;
  AgentTaskOutput?: (taskID: string) => Promise<RuntimeAgentTaskOutputResponseDTO>;
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
const runtimeBridgePath = '/bindings/github.com/CIPFZ/agent-builder/desktop/runtimebridge.js';
const runtimeBridgeTimeoutMS = 750;
let runtimeLatestEventSequence = 0;
let runtimeActivityRefreshHint: RuntimeEventViewModel | undefined;
let forceDraftChatSubmit = false;

function loadRuntimeBridge() {
  if (typeof window === 'undefined') {
    return Promise.resolve(null);
  }
  if (import.meta.env.DEV) {
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

function mapSessions(response?: RuntimeSessionsResponseDTO, activeSessionID?: string, activeTurns?: RuntimeTurnDTO[], currentProjectID?: string) {
  const sessions = Array.isArray(response?.sessions) ? response.sessions : [];
  const activeTurnBySession = new Map(
    (Array.isArray(activeTurns) ? activeTurns : [])
      .filter((turn) => turn.sessionId && !isFinalTurnStatus(turn.status))
      .map((turn) => [turn.sessionId, turn]),
  );

  return sessions.map((session) => ({
    id: session.id,
    title: session.title || '新对话',
    projectId: session.scope === 'standalone' ? undefined : (session.projectId || currentProjectID),
    scope: session.scope === 'standalone' ? 'standalone' as const : 'project' as const,
    updatedLabel: formatUpdatedLabel(session.updatedAt),
    active: session.active || session.id === activeSessionID,
    busy: activeTurnBySession.has(session.id),
    activeTurnId: activeTurnBySession.get(session.id)?.id,
  }));
}

function mapProjectFromStatus(status?: RuntimeStatusDTO, current?: WorkbenchViewModel['currentProject']): WorkbenchViewModel['currentProject'] {
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

function promptToUserInput(prompt: string): RuntimeUserInputRequestViewModel {
  const trimmed = prompt.trim();
  return {
    mode: trimmed.startsWith('/') ? 'slash' : 'prompt',
    items: [
      {
        type: 'text',
        text: prompt,
      },
    ],
  };
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
    models: provider.models,
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

function mapNormalizedInputConversation(response: RuntimeChatResponseDTO, fallbackPrompt: string): ConversationMessageViewModel[] {
  const normalized = response.normalizedInput;
  const hookPrevented = normalized?.hookOutcome?.preventContinuation || normalized?.hookOutcome?.status === 'blocked';
  if (!normalized || response.turnId || (normalized.shouldQuery === true && !hookPrevented)) {
    return [];
  }
  const messages = Array.isArray(normalized.messages) ? normalized.messages : [];
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
    status: 'success' as const,
  }));
  if (!conversation.some((message) => message.role === 'assistant') && normalized.command?.resultText) {
    conversation.push({
      id: `${normalized.id || response.requestId || 'input'}-command`,
      role: 'assistant',
      content: normalized.command.resultText,
      createdAt: normalized.createdAt,
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
    status: message.status,
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

function buildRuntimeTimelineOrder(callchain?: RuntimeReactCallchainDTO): RuntimeTimelineOrder {
  const order: RuntimeTimelineOrder = {
    sequenceByMessageID: new Map(),
    sequenceByToolCallID: new Map(),
    sequenceByPermissionID: new Map(),
    terminalByTurnID: new Map(),
    assistantStepByMessageID: new Map(),
  };
  const nodes = Array.isArray(callchain?.nodes) ? callchain.nodes : [];
  for (const node of nodes) {
    if (typeof node.sequence !== 'number') {
      continue;
    }
    if (node.messageId && (node.kind === 'user_input' || node.kind === 'assistant_final')) {
      order.sequenceByMessageID.set(node.messageId, node.sequence);
    }
    if (node.messageId && node.kind === 'assistant_step') {
      order.assistantStepByMessageID.set(node.messageId, node);
      order.sequenceByMessageID.set(node.messageId, node.sequence);
    }
    if (node.toolCallId && node.kind === 'tool_call') {
      order.sequenceByToolCallID.set(node.toolCallId, node.sequence);
    }
    if (node.permissionId && (node.kind === 'permission_request' || node.kind === 'permission_decision')) {
      order.sequenceByPermissionID.set(node.permissionId, node.sequence);
    }
    if (node.turnId && node.kind === 'turn_terminal') {
      order.terminalByTurnID.set(node.turnId, node);
    }
  }
  return order;
}

function mapActivityTimeline(activity?: RuntimeSessionActivityDTO, callchain?: RuntimeReactCallchainDTO): ConversationTimelineItemViewModel[] {
  if (!activity) {
    return [];
  }
  const runtimeOrder = buildRuntimeTimelineOrder(callchain);
  const messagesDTO = Array.isArray(activity.messages) ? activity.messages : [];
  const toolCallsDTO = Array.isArray(activity.toolCalls) ? activity.toolCalls : [];
  const permissionsDTO = Array.isArray(activity.permissions) ? activity.permissions : [];
  const turnsDTO = Array.isArray(activity.turns) ? activity.turns : [];
  const turnContext = buildTurnContext(messagesDTO, turnsDTO);
  const messages: ConversationTimelineItemViewModel[] = messagesDTO
    .filter((message) => message.role === 'user' || message.role === 'assistant')
    .flatMap((message) => {
      const timelineItems: ConversationTimelineItemViewModel[] = [];
      const content = runtimeMessageContent(message);
      const isIntermediateAssistant = message.role === 'assistant' && runtimeMessageHasToolCalls(message);
      const assistantStep = message.role === 'assistant' ? runtimeOrder.assistantStepByMessageID.get(message.id) : undefined;
      const turnId = turnIDForMessage(turnContext, message.id);
      const messageStatus = timelineMessageStatus(message, turnContext);
      if (assistantStep?.summary?.trim()) {
        timelineItems.push({
          id: `assistant-step:${message.id}`,
          kind: 'thinking',
          sessionId: activity.sessionId,
          turnId: turnId || assistantStep.turnId,
          messageId: message.id,
          role: 'assistant',
          title: assistantStep.title || 'Assistant step',
          content: assistantStep.summary,
          summary: assistantStep.summary,
          status: assistantStep.status || messageStatus,
          createdAt: assistantStep.startedAt || message.createdAt,
          updatedAt: assistantStep.finishedAt || message.updatedAt,
          sequence: assistantStep.sequence,
          source: 'react_callchain',
          provider: message.provider,
          model: message.model,
          error: assistantStep.error || message.error,
        });
      }
      if (!isIntermediateAssistant && (content.trim() || message.error || message.role === 'user')) {
        timelineItems.push({
          id: `message:${message.id}`,
          kind: 'message',
          sessionId: activity.sessionId,
          turnId,
          messageId: message.id,
          role: message.role,
          content,
          status: messageStatus,
          createdAt: message.createdAt,
          updatedAt: message.updatedAt,
          sequence: runtimeOrder.sequenceByMessageID.get(message.id),
          source: runtimeOrder.sequenceByMessageID.has(message.id) ? 'react_callchain' : 'runtime_activity',
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
    messageId: toolCall.messageId,
    toolCallId: toolCall.id,
    title: toolCall.name,
    status: toolCall.status,
    summary: toolCall.outputSummary || toolCall.inputSummary,
    createdAt: toolCall.startedAt,
    updatedAt: toolCall.finishedAt,
    sequence: runtimeOrder.sequenceByToolCallID.get(toolCall.id),
    source: runtimeOrder.sequenceByToolCallID.has(toolCall.id) ? 'react_callchain' : 'runtime_activity',
    error: toolCall.error,
    toolCall: mapToolCall(toolCall),
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
    sequence: runtimeOrder.sequenceByPermissionID.get(permission.id),
    source: runtimeOrder.sequenceByPermissionID.has(permission.id) ? 'react_callchain' : 'runtime_activity',
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
      sequence: runtimeOrder.terminalByTurnID.get(turn.id)?.sequence,
      source: runtimeOrder.terminalByTurnID.has(turn.id) ? 'react_callchain' : 'runtime_fallback',
      error: turn.error,
    }));
  const terminals: ConversationTimelineItemViewModel[] = turnsDTO
    .filter((turn) => shouldShowTurnTerminal(turn))
    .map((turn) => {
      const terminal = runtimeOrder.terminalByTurnID.get(turn.id);
      const diagnostics = turn.diagnostics;
      const missingFinal = diagnostics?.missingFinalAssistant || diagnostics?.hasFinalAssistant === false;
      return {
        id: `turn-terminal:${turn.id}`,
        kind: 'turn_terminal' as const,
        sessionId: turn.sessionId,
        turnId: turn.id,
        title: 'Turn terminal',
        status: turn.status,
        summary: missingFinal
          ? diagnostics?.stopReasonMessage || diagnostics?.stopReason || terminal?.summary || turn.error || 'Turn ended without a final assistant message.'
          : terminal?.summary || diagnostics?.stopReasonMessage || diagnostics?.stopReason || turn.error,
        createdAt: turn.finishedAt || turn.startedAt,
        updatedAt: turn.finishedAt,
        sequence: terminal?.sequence,
        source: terminal ? 'react_callchain' as const : 'runtime_fallback' as const,
        error: turn.error,
        diagnostics: turn.diagnostics,
      };
    });
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
      source: 'runtime_activity',
      diagnostics: turn.diagnostics,
    }));

  return [...messages, ...thinking, ...toolCalls, ...permissions, ...progress, ...terminals, ...diagnostics].sort(compareActivityTimelineItems(messagesDTO, turnsDTO));
}

function mergeActivityTimeline(current: ConversationTimelineItemViewModel[], activity: RuntimeSessionActivityDTO, callchain?: RuntimeReactCallchainDTO) {
  const replacement = mapActivityTimeline(activity, callchain);
  const activityMessages = Array.isArray(activity.messages) ? activity.messages : [];
  const activityTurns = Array.isArray(activity.turns) ? activity.turns : [];
  const activityTurnContext = buildTurnContext(activityMessages, activityTurns);
  const turnIDs = new Set(activityTurns.map((turn) => turn.id).filter(Boolean));
  replacement.forEach((item) => {
    const turnID = item.turnId || turnIDForMessage(activityTurnContext, item.messageId);
    if (turnID) {
      turnIDs.add(turnID);
    }
  });
  const messageIDs = new Set((Array.isArray(activity.messages) ? activity.messages : []).map((message) => message.id).filter(Boolean));
  const toolCallIDs = new Set((Array.isArray(activity.toolCalls) ? activity.toolCalls : []).map((toolCall) => toolCall.id).filter(Boolean));
  const permissionIDs = new Set((Array.isArray(activity.permissions) ? activity.permissions : []).map((permission) => permission.id).filter(Boolean));
  const hasRuntimeActivity = replacement.length > 0 || turnIDs.size > 0 || messageIDs.size > 0 || toolCallIDs.size > 0 || permissionIDs.size > 0;
  const kept = current.filter((item) => {
    if (hasRuntimeActivity && isOptimisticTimelineItem(item)) {
      return false;
    }
    if (item.turnId && turnIDs.has(item.turnId)) {
      return false;
    }
    const inferredTurnID = turnIDForMessage(activityTurnContext, item.messageId);
    if (inferredTurnID && turnIDs.has(inferredTurnID)) {
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
  return [...kept, ...replacement].sort(compareActivityTimelineItems(activityMessages, activityTurns));
}

function attachAgentTasksToTimeline(items: ConversationTimelineItemViewModel[], tasks?: AgentTaskViewModel[]): ConversationTimelineItemViewModel[] {
  if (!tasks?.length) {
    return items;
  }
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
    merged.push(item);
    if (item.kind === 'tool_call' && item.toolCallId) {
      merged.push(...(byTool.get(item.toolCallId) ?? []));
      byTool.delete(item.toolCallId);
    }
  });
  byTool.forEach((values) => unplaced.push(...values));
  return [...merged, ...unplaced];
}

function compareActivityTimelineItems(messages: RuntimeMessageDTO[], turns: RuntimeTurnDTO[]) {
  const turnContext = buildTurnContext(messages, turns);
  const messageOrder = new Map(messages.map((message, index) => [message.id, index]));
  return (left: ConversationTimelineItemViewModel, right: ConversationTimelineItemViewModel) => {
    const leftTurn = left.turnId || turnIDForMessage(turnContext, left.messageId);
    const rightTurn = right.turnId || turnIDForMessage(turnContext, right.messageId);
    const leftTime = timelineSortTime(left, leftTurn, turnContext);
    const rightTime = timelineSortTime(right, rightTurn, turnContext);
    if (leftTurn && rightTurn && leftTurn === rightTurn) {
      const leftOrder = timelineTurnOrder(left, messageOrder, turnContext);
      const rightOrder = timelineTurnOrder(right, messageOrder, turnContext);
      if (leftOrder !== rightOrder) {
        return leftOrder - rightOrder;
      }
      const leftRank = timelineKindRank(left);
      const rightRank = timelineKindRank(right);
      if (leftRank !== rightRank) {
        return leftRank - rightRank;
      }
    }
    if (typeof left.sequence === 'number' && typeof right.sequence === 'number' && left.sequence !== right.sequence) {
      return left.sequence - right.sequence;
    }
    if (typeof left.sequence === 'number' && typeof right.sequence !== 'number') {
      return -1;
    }
    if (typeof left.sequence !== 'number' && typeof right.sequence === 'number') {
      return 1;
    }
    const leftSequence = timelineMessageSequenceOrder(left, messageOrder);
    const rightSequence = timelineMessageSequenceOrder(right, messageOrder);
    if (leftSequence !== rightSequence && leftSequence < Number.MAX_SAFE_INTEGER && rightSequence < Number.MAX_SAFE_INTEGER) {
      return leftSequence - rightSequence;
    }
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    const leftMessageOrder = timelineMessageOrder(left, messageOrder);
    const rightMessageOrder = timelineMessageOrder(right, messageOrder);
    if (leftMessageOrder !== rightMessageOrder) {
      return leftMessageOrder - rightMessageOrder;
    }
    return left.id.localeCompare(right.id);
  };
}

function mergeConversationMessages(current: ConversationMessageViewModel[], response?: RuntimeMessagesResponseDTO) {
  const incoming = mapConversation(response);
  if (incoming.length === 0) {
    return current;
  }
  const incomingIDs = new Set(incoming.map((message) => message.id));
  return [...current.filter((message) => !incomingIDs.has(message.id) && !isOptimisticConversationMessage(message)), ...incoming].sort((left, right) => {
    const leftTime = normalizeTimestamp(left.createdAt ?? 0);
    const rightTime = normalizeTimestamp(right.createdAt ?? 0);
    if (leftTime !== rightTime) {
      return leftTime - rightTime;
    }
    return left.id.localeCompare(right.id);
  });
}

function isOptimisticConversationMessage(message: ConversationMessageViewModel) {
  return message.status === 'loading' || message.id.startsWith('local-') || message.id.startsWith('loading-');
}

function isOptimisticTimelineItem(item: ConversationTimelineItemViewModel) {
  return item.status === 'loading' || item.id.startsWith('local-') || item.id.startsWith('loading-');
}

function timelineMessageStatus(message: RuntimeMessageDTO, turnContext: TimelineTurnContext): 'loading' | 'success' | 'error' {
  if (message.error) {
    return 'error';
  }
  if (message.role !== 'assistant') {
    return 'success';
  }
  const turnId = turnIDForMessage(turnContext, message.id);
  const turn = turnId ? turnContext.turnByID.get(turnId) : undefined;
  if (turn && !isFinalTurnStatus(turn.status)) {
    return 'loading';
  }
  if (turn?.error) {
    return 'error';
  }
  return 'success';
}

function shouldShowTurnTerminal(turn: RuntimeTurnDTO) {
  if (!isFinalTurnStatus(turn.status)) {
    return false;
  }
  const diagnostics = turn.diagnostics;
  const missingFinal = diagnostics?.missingFinalAssistant || diagnostics?.hasFinalAssistant === false;
  return turn.status !== 'completed' || Boolean(missingFinal || turn.error);
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
  toolCallOrderByID: Map<string, number>;
}

function buildTurnContext(messages: RuntimeMessageDTO[], turns: RuntimeTurnDTO[]): TimelineTurnContext {
  const turnByID = new Map(turns.map((turn) => [turn.id, turn]));
  const turnIDByMessageID = new Map<string, string>();
  const toolCallOrderByID = new Map<string, number>();
  const messageIndex = new Map(messages.map((message, index) => [message.id, index]));
  let toolCallOrder = 0;
  for (const message of messages) {
    for (const part of message.parts ?? []) {
      if (part.type === 'tool_call' && part.toolCallId && !toolCallOrderByID.has(part.toolCallId)) {
        toolCallOrderByID.set(part.toolCallId, toolCallOrder);
        toolCallOrder += 1;
      }
    }
  }
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
  return { turnByID, turnIDByMessageID, toolCallOrderByID };
}

function timelineMessageOrder(item: ConversationTimelineItemViewModel, messageOrder: Map<string, number>) {
  if (!item.messageId) {
    return Number.MAX_SAFE_INTEGER;
  }
  return messageOrder.get(item.messageId) ?? Number.MAX_SAFE_INTEGER;
}

function timelineMessageSequenceOrder(item: ConversationTimelineItemViewModel, messageOrder: Map<string, number>) {
  const order = timelineMessageOrder(item, messageOrder);
  if (order === Number.MAX_SAFE_INTEGER) {
    return order;
  }
  if (item.kind === 'message' && item.role === 'user') {
    return order * 10;
  }
  if (item.kind === 'thinking') {
    return order * 10 + 2;
  }
  if (item.kind === 'tool_call') {
    return order * 10 + 3;
  }
  if (item.kind === 'permission') {
    return order * 10 + 4;
  }
  if (item.kind === 'message' && item.role === 'assistant') {
    return order * 10 + 8;
  }
  return order * 10 + 9;
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
  if (item.kind === 'message' && item.role === 'assistant') {
    return 1;
  }
  if (item.kind === 'thinking') {
    return 2;
  }
  if (item.kind === 'tool_call') {
    return 3;
  }
  if (item.kind === 'permission') {
    return 4;
  }
  if (item.kind === 'progress') {
    return 5;
  }
  if (item.kind === 'turn_terminal') {
    return 6;
  }
  if (item.kind === 'diagnostic') {
    return 7;
  }
  return 8;
}

function timelineTurnOrder(item: ConversationTimelineItemViewModel, messageOrder: Map<string, number>, context: TimelineTurnContext) {
  if (item.kind === 'message' && item.role === 'user') {
    return 0 + timelineMessageOrder(item, messageOrder);
  }
  if (item.kind === 'thinking') {
    return 50_000 + timelineMessageOrder(item, messageOrder);
  }
  if (item.kind === 'tool_call') {
    return 100_000 + Math.min(timelineToolCallOrder(item, context) * 10, timelineMessageOrder(item, messageOrder));
  }
  if (item.kind === 'permission') {
    return 100_000 + Math.min(timelineToolCallOrder(item, context) * 10, timelineMessageOrder(item, messageOrder)) + 1;
  }
  if (item.kind === 'message' && item.role === 'assistant') {
    return 800_000 + timelineMessageOrder(item, messageOrder);
  }
  if (item.kind === 'progress') {
    return 900_000;
  }
  if (item.kind === 'turn_terminal') {
    return 905_000;
  }
  if (item.kind === 'diagnostic') {
    return 910_000;
  }
  return 920_000;
}

function timelineToolCallOrder(item: ConversationTimelineItemViewModel, context: TimelineTurnContext) {
  if (!item.toolCallId) {
    return 0;
  }
  return context.toolCallOrderByID.get(item.toolCallId) ?? 0;
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

function mapReactCallchain(callchain?: RuntimeReactCallchainDTO): ReactCallchainViewModel | undefined {
  if (!callchain?.sessionId) {
    return undefined;
  }
  const nodes = (Array.isArray(callchain.nodes) ? callchain.nodes : [])
    .filter((node) => node.id && node.kind && typeof node.sequence === 'number')
    .map((node) => ({
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
    }))
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
  const compactBoundaries = (Array.isArray(assembly.compact) ? assembly.compact : [])
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
  const warnings = [
    ...contextSources
      .filter((source) => source.state === 'failed' || source.state === 'skipped' || Boolean(source.error))
      .map((source) => `${source.name}: ${source.error || source.reason || source.state}`),
    ...(messages.rawPromptStored ? ['Runtime reported raw prompt storage in prompt assembly metadata.'] : []),
    ...(skills.rawContentStored ? ['Runtime reported raw skill content storage in prompt assembly metadata.'] : []),
    ...(mcp.rawContentStored ? ['Runtime reported raw MCP instruction storage in prompt assembly metadata.'] : []),
  ];

  return {
    sessionId: assembly.sessionId,
    turnId: assembly.turnId,
    step: assembly.step,
    provider: assembly.provider,
    model: assembly.model,
    createdAt: assembly.createdAt,
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
      instructionHash: mcp.instructionHash,
      tokenEstimate: mcp.tokenEstimate,
      rawContentStored: Boolean(mcp.rawContentStored),
    },
    contextSources,
    compactBoundaries,
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

function runtimeMessageHasToolCalls(message: RuntimeMessageDTO) {
  return Array.isArray(message.parts) && message.parts.some((part) => part.type === 'tool_call' && part.toolCallId);
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
    models: provider.models,
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
    models: Array.isArray(response.models) ? response.models : [],
    error: response.error,
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
  const [status, sessionsResponse, modelsResponse, providerCatalog, configuredProvidersResponse, activeTurnsResponse, skillsResponse, pluginsResponse, mcpServersResponse] = await Promise.all([
    optionalRuntimeRequest(() => bridge.Status()),
    optionalRuntimeRequest(() => bridge.Sessions()),
    fullHydration ? bridge.Models().catch(() => undefined) : Promise.resolve(undefined),
    fullHydration ? bridge.ProviderCatalog?.().catch(() => undefined) : Promise.resolve(undefined),
    fullHydration ? bridge.ConfiguredProviders?.().catch(() => undefined) : Promise.resolve(undefined),
    optionalRuntimeRequest(() => bridge.Turns?.('active') ?? Promise.resolve(undefined)),
    fullHydration ? optionalRuntimeRequest(() => bridge.Skills?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration ? optionalRuntimeRequest(() => bridge.Plugins?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
    fullHydration ? optionalRuntimeRequest(() => bridge.MCPServers?.() ?? Promise.resolve(undefined)) : Promise.resolve(undefined),
  ]);
  const activeSessionID = status?.sessionId || sessionsResponse?.sessions?.find((session) => session.active)?.id;
  const narrowActivity = activeSessionID && refreshActivity ? await hydrateNarrowActivityFromHint(bridge, activeSessionID) : undefined;
  const activity = narrowActivity ?? (activeSessionID && refreshActivity ? await optionalRuntimeRequest(() => bridge.SessionActivity?.(activeSessionID) ?? Promise.resolve(undefined)) : undefined);
  const runProjection = activeSessionID && bridge.RunProjection && refreshRuns
    ? await optionalRuntimeRequest(() => bridge.RunProjection?.({ sessionId: activeSessionID, limit: 24 }) ?? Promise.resolve(undefined))
    : undefined;
  const schedulerTaskCandidates = actionTargetsInclude(refreshTargets, 'run_scheduler_plan') ? await hydrateRunSchedulerTaskCandidates(bridge, runProjection) : [];
  const agentTasks = activeSessionID ? await hydrateAgentTasks(bridge, activeSessionID) : undefined;
  const messagesResponse = activity
    ? { messages: Array.isArray(activity.messages) ? activity.messages : [] }
    : activeSessionID && fullHydration
      ? await optionalRuntimeRequest(() => bridge.SessionMessages?.(activeSessionID) ?? Promise.resolve(undefined))
    : fullHydration
      ? await optionalRuntimeRequest(() => bridge.Messages?.() ?? Promise.resolve(undefined))
    : undefined;
  const modelOptionList = modelsResponse ? modelOptions(modelsResponse) : current.composer.modelOptions;
  const selectedModel = modelsResponse ? modelOptionList.find((model) => model.selected) : current.composer.selectedModel;
  const currentProject = mapProjectFromStatus(status, current.currentProject);
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
  const reactCallchainDTO = activeSessionID && refreshActivity
    ? await hydrateReactCallchain(bridge, activeSessionID, activeTurnId)
    : undefined;
  const promptAssembliesDTO = activeSessionID && refreshActivity
    ? await hydratePromptAssemblies(bridge, activeSessionID, activeTurnId)
    : undefined;
  const policy = activity?.policy ?? (refreshPolicy ? (await optionalRuntimeRequest(() => bridge.GetPolicy?.() ?? Promise.resolve(undefined)))?.policy : undefined);
  const permissions = (Array.isArray(activity?.permissions) ? activity.permissions : []).map(mapPermission);
  const baseTimeline = activity
    ? narrowActivity
      ? mergeActivityTimeline(current.timeline, activity, reactCallchainDTO)
      : mapActivityTimeline(activity, reactCallchainDTO)
    : current.timeline;
  const timeline = attachAgentTasksToTimeline(baseTimeline, agentTasks);
  const conversation = activity && narrowActivity
    ? mergeConversationMessages(current.conversation, messagesResponse)
    : messagesResponse
      ? mapConversation(messagesResponse)
      : current.conversation;
  const pendingPermissions = activity && narrowActivity
    ? mergePendingPermissions(current.pendingPermissions, permissions)
    : activity
      ? permissions.filter((permission) => permission.status === 'pending')
      : current.pendingPermissions;
  const skills = mapSkills(skillsResponse) ?? current.settings.skills;
  const plugins = mapPlugins(pluginsResponse) ?? current.settings.plugins;
  const mcpServers = mapMCPServers(mcpServersResponse) ?? current.settings.mcpServers;
  const providers = mapProviderCatalogItems(providerCatalog) ?? current.settings.providers;
  const nextPermissionMode = policy ? permissionMode(policy) : (current.composer.permissionMode ?? permissionMode());
  const nextSettingsPermissionMode = policy ? permissionMode(policy) : (current.settings.permissionMode ?? permissionMode());

  return {
    ...current,
    currentProject,
    projects: currentProject.path ? [currentProject] : [],
    sessions: mapSessions(sessionsResponse, status?.sessionId, activeTurns, currentProject.id),
    conversation,
    timeline,
    turnDiagnostics: activity ? selectTurnDiagnostics(activity, sessionActiveTurn?.id) : current.turnDiagnostics,
    interruptedTurn: activity ? selectInterruptedTurn(activity, sessionActiveTurn?.id) : current.interruptedTurn,
    runProjection: mapRunProjection(runProjection, schedulerTaskCandidates) ?? (current.runProjection?.primarySessionId === activeSessionID ? current.runProjection : undefined),
    agentTasks: agentTasks ?? (activeSessionID ? current.agentTasks?.filter((task) => task.parentSessionId === activeSessionID || task.childSessionId === activeSessionID) : []),
    reactCallchain: mapReactCallchain(reactCallchainDTO) ?? (current.reactCallchain?.sessionId === activeSessionID ? current.reactCallchain : undefined),
    contextDiagnostics: mapContextDiagnostics(promptAssembliesDTO) ?? (current.contextDiagnostics?.sessionId === activeSessionID ? current.contextDiagnostics : undefined),
    pendingPermissions,
    composer: {
      ...current.composer,
      permissionLabel: nextPermissionMode.label,
      permissionMode: nextPermissionMode,
      permissionOptions: permissionModeOptions,
      modelLabel: modelLabel(status, modelsResponse),
      capabilityLabel: capabilityLabel(skills, mcpServers),
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
  const fromBridge = await readReactCallchain(bridge, sessionID, turnID);
  if (fromBridge) {
    return fromBridge;
  }
  if (bridge === runtimeHTTPBridge) {
    return undefined;
  }
  const httpBridge = await loadRuntimeHTTPBridge();
  if (!httpBridge) {
    return undefined;
  }
  return readReactCallchain(httpBridge, sessionID, turnID);
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
  const fromBridge = await readPromptAssemblies(bridge, sessionID, turnID);
  if (fromBridge) {
    return fromBridge;
  }
  if (bridge === runtimeHTTPBridge) {
    return undefined;
  }
  const httpBridge = await loadRuntimeHTTPBridge();
  if (!httpBridge) {
    return undefined;
  }
  return readPromptAssemblies(httpBridge, sessionID, turnID);
}

async function readPromptAssemblies(bridge: RuntimeBridgeModule, sessionID: string, turnID?: string) {
  return optionalRuntimeRequest(() => {
    if (turnID && bridge.TurnPromptAssemblies) {
      return bridge.TurnPromptAssemblies(turnID);
    }
    return bridge.SessionPromptAssemblies?.(sessionID, 6) ?? Promise.resolve(undefined);
  });
}

const runtimeHTTPURL = import.meta.env.DEV ? '/runtime-api' : import.meta.env.VITE_AGENT_BUILDER_RUNTIME_URL || 'http://127.0.0.1:5183';
const runtimeHTTPToken = import.meta.env.VITE_AGENT_BUILDER_RUNTIME_TOKEN || 'agent-builder-dev';
const runtimeOptionalRequestTimeoutMS = 3000;
const runtimeMutationTimeoutMS = 15000;
const terminalStreamPendingLimit = 256;

interface TerminalStreamConnection {
  closed: boolean;
  pending: unknown[];
  socket?: WebSocket;
}

const terminalStreams = new Map<string, TerminalStreamConnection>();

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
    return runtimeJSONP<T>(path, init);
  }
}

function runtimeJSONP<T>(path: string, init?: RuntimeHTTPInit): Promise<T> {
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
    const params = new URLSearchParams({
      path,
      token: runtimeHTTPToken,
      callback,
    });
    if (init?.method) {
      params.set('method', init.method);
    }
    if (init?.body) {
      params.set('body', init.body);
    }
    script.src = `${runtimeHTTPURL}/v1/dev/jsonp?${params.toString()}`;
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

function subscribeHTTPRuntimeTerminalStream(terminalID: string, onEvent: (event: TerminalEventViewModel) => void) {
  if (typeof window === 'undefined' || typeof window.WebSocket === 'undefined') {
    throw new Error('terminal WebSocket transport is unavailable');
  }
  const url = runtimeTerminalStreamURL(terminalID);
  if (!url) {
    throw new Error('terminal WebSocket URL is unavailable');
  }

  let lastSequence = 0;
  const connection: TerminalStreamConnection = {
    closed: false,
    pending: [],
  };
  terminalStreams.set(terminalID, connection);

  const flushPending = () => {
    const socket = connection.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return;
    }
    while (connection.pending.length > 0) {
      socket.send(JSON.stringify(connection.pending.shift()));
    }
  };

  const connect = () => {
    if (connection.closed) {
      return;
    }
    const socket = new window.WebSocket(lastSequence > 0 ? `${url}&after=${encodeURIComponent(String(lastSequence))}` : url);
    connection.socket = socket;
    socket.onopen = flushPending;
    socket.onmessage = (message) => {
      const events = parseTerminalStreamMessage(String(message.data));
      let maxSequence = lastSequence;
      for (const event of events) {
        if (event.sequence > maxSequence) {
          maxSequence = event.sequence;
        }
        onEvent({
          ...event,
          acknowledge: () => acknowledgeTerminalWebSocket(terminalID, event.sequence),
        });
      }
      if (maxSequence > lastSequence) {
        lastSequence = maxSequence;
      }
    };
    socket.onclose = () => {
      if (connection.socket === socket) {
        connection.socket = undefined;
      }
      if (!connection.closed) {
        window.setTimeout(connect, 500);
      }
    };
    socket.onerror = () => {
      socket?.close();
    };
  };

  connect();

  return () => {
    connection.closed = true;
    connection.pending = [];
    if (terminalStreams.get(terminalID) === connection) {
      terminalStreams.delete(terminalID);
    }
    connection.socket?.close();
  };
}

function acknowledgeTerminalWebSocket(terminalID: string, sequence: number) {
  if (!sequence || sequence <= 0) {
    return false;
  }
  return sendTerminalWebSocketMessage(terminalID, { type: 'ack', sequence });
}

function parseTerminalStreamMessage(data: string): TerminalEventViewModel[] {
  try {
    const message = JSON.parse(data) as RuntimeTerminalStreamMessageDTO | RuntimeTerminalEventDTO;
    if ('events' in message && Array.isArray(message.events)) {
      return [mapTerminalEventBatch(message.events)];
    }
    return [mapTerminalEvent(message as RuntimeTerminalEventDTO)];
  } catch {
    return [];
  }
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

function runtimeEventsPath(after?: number) {
  if (!after || after <= 0) {
    return '/v1/events';
  }
  return `/v1/events?after=${encodeURIComponent(String(after))}`;
}

function runtimeTerminalStreamURL(terminalID: string) {
  if (typeof window === 'undefined') {
    return '';
  }
  const streamPath = `/v1/terminals/${encodeURIComponent(terminalID)}/stream`;
  const tokenQuery = `token=${encodeURIComponent(runtimeHTTPToken)}`;
  if (runtimeHTTPURL.startsWith('/')) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}${runtimeHTTPURL}${streamPath}?${tokenQuery}`;
  }
  const url = new URL(`${runtimeHTTPURL}${streamPath}`);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('token', runtimeHTTPToken);
  return url.toString();
}

function sendTerminalWebSocketMessage(terminalID: string, message: unknown) {
  const connection = terminalStreams.get(terminalID);
  if (!connection || connection.closed) {
    return false;
  }
  const socket = connection.socket;
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(message));
    return true;
  }
  if (connection.pending.length >= terminalStreamPendingLimit) {
    return false;
  }
  connection.pending.push(message);
  return true;
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

const runtimeHTTPBridge: RuntimeBridgeModule = {
  Status: () => runtimeFetch<RuntimeStatusDTO>('/v1/runtime/status'),
  OpenProject: (req) =>
    runtimeFetch<RuntimeOpenProjectResponseDTO>('/v1/projects/open', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  CreateProject: (req) =>
    runtimeFetch<RuntimeOpenProjectResponseDTO>('/v1/projects', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  RenameProject: (req) =>
    runtimeFetch<RuntimeOpenProjectResponseDTO>(`/v1/projects/${encodeURIComponent(req.projectId)}/rename`, {
      method: 'POST',
      body: JSON.stringify({ name: req.name }),
    }),
  OpenProjectInExplorer: (req) =>
    runtimeFetch<RuntimeOpenProjectResponseDTO>(`/v1/projects/${encodeURIComponent(req.projectId)}/open-explorer`, {
      method: 'POST',
    }),
  RemoveProject: (req) =>
    runtimeFetch<RuntimeOpenProjectResponseDTO>(`/v1/projects/${encodeURIComponent(req.projectId)}`, {
      method: 'DELETE',
    }),
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
  DiscoverModelConfig: (req) =>
    runtimeFetch<RuntimeModelDiscoveryResponseDTO>('/v1/config/model/discover', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  VerifyModelConfig: (req) =>
    runtimeFetch<RuntimeModelVerifyResponseDTO>('/v1/config/model/verify', {
      method: 'POST',
      body: JSON.stringify(req),
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
    return runtimeFetch<RuntimeStatusDTO>('/v1/runtime/new-chat', {
      method: 'POST',
      body: JSON.stringify({ title }),
    });
  },
  async CreateSession(req) {
    return runtimeFetch<RuntimeSessionResponseDTO>('/v1/sessions', {
      method: 'POST',
      body: JSON.stringify(req ?? {}),
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
  SessionReactCallchain: (sessionID, limit) =>
    runtimeFetch<RuntimeReactCallchainDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/react-callchain?limit=${encodeURIComponent(String(limit))}`),
  SessionPromptAssemblies: (sessionID, limit) =>
    runtimeFetch<RuntimePromptAssembliesResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/prompt-assemblies?limit=${encodeURIComponent(String(limit))}`),
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
  RunSummaries: () => runtimeFetch<RuntimeRunSummariesResponseDTO>('/v1/run-summaries'),
  RunSummary: (runID) => runtimeFetch<RuntimeRunSummaryResponseDTO>(`/v1/run-summaries/${encodeURIComponent(runID)}`),
  RunCheckpointMarkers: (runID) =>
    runtimeFetch<RuntimeRunCheckpointMarkersResponseDTO>(`/v1/runs/${encodeURIComponent(runID)}/checkpoint-markers`),
  RunCheckpointMarker: (runID, checkpointID) =>
    runtimeFetch<RuntimeRunCheckpointMarkerResponseDTO>(
      `/v1/runs/${encodeURIComponent(runID)}/checkpoint-markers/${encodeURIComponent(checkpointID)}`,
    ),
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
  SessionAgentTasks: (sessionID) => runtimeFetch<RuntimeAgentTasksResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/agent-tasks`),
  AgentTask: (taskID) => runtimeFetch<RuntimeAgentTaskResponseDTO>(`/v1/agent-tasks/${encodeURIComponent(taskID)}`),
  AgentTaskFollowUp: (taskID, req) =>
    runtimeFetch<RuntimeAgentTaskMessageResponseDTO>(`/v1/agent-tasks/${encodeURIComponent(taskID)}/messages`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  CancelAgentTask: (taskID) =>
    runtimeFetch<RuntimeAgentTaskResponseDTO>(`/v1/agent-tasks/${encodeURIComponent(taskID)}/cancel`, {
      method: 'POST',
    }),
  AgentTaskOutput: (taskID) => runtimeFetch<RuntimeAgentTaskOutputResponseDTO>(`/v1/agent-tasks/${encodeURIComponent(taskID)}/output`),
  TurnActivity: (turnID) => runtimeFetch<RuntimeTurnActivityDTO>(`/v1/turns/${encodeURIComponent(turnID)}/activity`),
  ReactCallchain: (turnID) => runtimeFetch<RuntimeReactCallchainDTO>(`/v1/turns/${encodeURIComponent(turnID)}/react-callchain`),
  TurnPromptAssemblies: (turnID) => runtimeFetch<RuntimePromptAssembliesResponseDTO>(`/v1/turns/${encodeURIComponent(turnID)}/prompt-assemblies`),
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
  SubmitUserInput: (req) =>
    runtimeFetch<RuntimeChatResponseDTO>('/v1/user-inputs', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  UserInput: (inputID) => runtimeFetch<RuntimeNormalizedInputDTO>(`/v1/user-inputs/${encodeURIComponent(inputID)}`),
  CreateTerminal: (req) =>
    runtimeFetch<RuntimeTerminalResponseDTO>('/v1/terminals', {
      method: 'POST',
      body: JSON.stringify(req ?? {}),
    }),
  SessionTerminals: (sessionID) => runtimeFetch<RuntimeSessionTerminalsResponseDTO>(`/v1/sessions/${encodeURIComponent(sessionID)}/terminals`),
  DeleteTerminal: (terminalID) =>
    runtimeFetch<RuntimeTerminalResponseDTO>(`/v1/terminals/${encodeURIComponent(terminalID)}`, {
      method: 'DELETE',
    }),
};

async function loadRuntimeHTTPBridge() {
  try {
    await runtimeHTTPBridge.Status();
    return runtimeHTTPBridge;
  } catch (error) {
    console.warn('[runtime] HTTP bridge unavailable', error);
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

function resetConversationRuntimeState(current: WorkbenchViewModel): WorkbenchViewModel {
  return {
    ...current,
    conversation: [],
    timeline: [],
    turnDiagnostics: undefined,
    interruptedTurn: undefined,
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
      try {
        await bridge.OpenProject(request);
        return hydrateWorkbench(nextBase, bridge);
      } catch (error) {
        console.warn('[runtime] wails open project failed, trying HTTP bridge', error);
      }
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.OpenProject) {
      await httpBridge.OpenProject(request);
      return hydrateWorkbench(nextBase, httpBridge);
    }
    return staticWorkbenchAdapter.openProject(current, request);
  },
  async createProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.CreateProject) {
      try {
        await bridge.CreateProject(request);
        return hydrateWorkbench(nextBase, bridge);
      } catch (error) {
        console.warn('[runtime] wails create project failed, trying HTTP bridge', error);
      }
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.CreateProject) {
      await httpBridge.CreateProject(request);
      return hydrateWorkbench(nextBase, httpBridge);
    }
    return staticWorkbenchAdapter.createProject(current, request);
  },
  async renameProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.RenameProject) {
      try {
        await bridge.RenameProject(request);
        return hydrateWorkbench(nextBase, bridge);
      } catch (error) {
        console.warn('[runtime] wails rename project failed, trying HTTP bridge', error);
      }
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.RenameProject) {
      await httpBridge.RenameProject(request);
      return hydrateWorkbench(nextBase, httpBridge);
    }
    return staticWorkbenchAdapter.renameProject(current, request);
  },
  async openProjectInExplorer(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.OpenProjectInExplorer) {
      try {
        await bridge.OpenProjectInExplorer(request);
        return;
      } catch (error) {
        console.warn('[runtime] wails open project in explorer failed, trying HTTP bridge', error);
      }
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.OpenProjectInExplorer) {
      await httpBridge.OpenProjectInExplorer(request);
      return;
    }
    return staticWorkbenchAdapter.openProjectInExplorer(request);
  },
  async removeProject(current, request) {
    const nextBase = resetConversationRuntimeState({ ...current, mode: 'project' as const });
    const bridge = await loadRuntimeBridge();
    if (bridge?.RemoveProject) {
      try {
        await bridge.RemoveProject(request);
        return hydrateWorkbench(nextBase, bridge);
      } catch (error) {
        console.warn('[runtime] wails remove project failed, trying HTTP bridge', error);
      }
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.RemoveProject) {
      await httpBridge.RemoveProject(request);
      return hydrateWorkbench(nextBase, httpBridge);
    }
    return staticWorkbenchAdapter.removeProject(current, request);
  },
  async selectProjectDirectory() {
    const bridge = await loadRuntimeBridge();
    if (bridge?.SelectProjectDirectory) {
      return bridge.SelectProjectDirectory();
    }
    return staticWorkbenchAdapter.selectProjectDirectory();
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
  async createSession(current, target) {
    forceDraftChatSubmit = false;
    return withBridge(
      async (bridge) => {
        await bridge.NewChat('');
        const draft = target ?? current.newConversationDraft ?? defaultDraftTarget(current);
        const hydrated = await hydrateWorkbench(
          resetConversationRuntimeState({ ...current, mode: 'new-chat', newConversationDraft: draft }),
          bridge,
        );
        return { ...hydrated, mode: 'new-chat', newConversationDraft: draft };
      },
      () => staticWorkbenchAdapter.createSession(current),
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
  async decidePermission(current, permissionID, action) {
    return withBridge(
      async (bridge) => {
        if (!bridge.DecidePermission) {
          return staticWorkbenchAdapter.decidePermission(current, permissionID, action);
        }
        const response = await bridge.DecidePermission({ permissionId: permissionID, action });
        return hydrateWorkbenchForAction(current, bridge, response);
      },
      () => staticWorkbenchAdapter.decidePermission(current, permissionID, action),
    );
  },
  async sendPrompt(current, prompt) {
    return wailsWorkbenchAdapter.submitUserInput?.(current, promptToUserInput(prompt)) ?? staticWorkbenchAdapter.sendPrompt(current, prompt);
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
          return {
            ...optimistic,
            mode: 'new-chat',
            newConversationDraft: undefined,
            sessions: sessionsAfterDraftSubmit(current, responseSessionID, prompt, draftTarget, response.turnId),
            conversation:
              normalizedOnly
                ? [...current.conversation.filter((message) => message.status !== 'loading'), ...normalizedConversation]
                : optimistic.conversation,
            timeline: normalizedOnly ? [...current.timeline.filter((item) => item.status !== 'loading'), ...normalizedTimeline] : optimistic.timeline,
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
            interruptedTurn: undefined,
          },
          bridge,
          response,
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
        const response = await bridge.ResumeRunCheckpoint(runID, checkpointID);
        return hydrateWorkbenchForAction(current, bridge, response);
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
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.SessionTerminals) {
      return mapSessionTerminals(await httpBridge.SessionTerminals(sessionID));
    }
    return staticWorkbenchAdapter.listSessionTerminals(sessionID);
  },
  async createTerminal(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.CreateTerminal) {
      return mapTerminal(await bridge.CreateTerminal(request));
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.CreateTerminal) {
      return mapTerminal(await httpBridge.CreateTerminal(request));
    }
    return staticWorkbenchAdapter.createTerminal(request);
  },
  async writeTerminalInput(terminalID, data) {
    if (sendTerminalWebSocketMessage(terminalID, { type: 'input', data })) {
      return {
        id: terminalID,
        sessionId: '',
        cwd: '',
        shell: '',
        status: 'running',
      };
    }
    throw new Error('terminal stream is not connected');
  },
  async resizeTerminal(terminalID, columns, rows) {
    if (sendTerminalWebSocketMessage(terminalID, { type: 'resize', columns, rows })) {
      return {
        id: terminalID,
        sessionId: '',
        cwd: '',
        shell: '',
        columns,
        rows,
        status: 'running',
      };
    }
    throw new Error('terminal stream is not connected');
  },
  async subscribeTerminalEvents(terminalID, onEvent) {
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.CreateTerminal) {
      return subscribeHTTPRuntimeTerminalStream(terminalID, onEvent);
    }
    throw new Error('terminal stream is not connected');
  },
  async deleteTerminal(terminalID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DeleteTerminal) {
      await bridge.DeleteTerminal(terminalID);
      return;
    }
    const httpBridge = await loadRuntimeHTTPBridge();
    if (httpBridge?.DeleteTerminal) {
      await httpBridge.DeleteTerminal(terminalID);
      return;
    }
    return staticWorkbenchAdapter.deleteTerminal(terminalID);
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
    if (runtimeHTTPBridge.DiscoverModelConfig) {
      return mapDraftModelDiscovery(await runtimeHTTPBridge.DiscoverModelConfig(toRuntimeModelConfigRequest(request)));
    }
    return staticWorkbenchAdapter.discoverProviderDraftModels(request);
  },
  async discoverConfiguredProviderModels(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.DiscoverConfiguredProviderModels) {
      return bridge.DiscoverConfiguredProviderModels(providerID);
    }
    return runtimeHTTPBridge.DiscoverConfiguredProviderModels?.(providerID) ?? staticWorkbenchAdapter.discoverConfiguredProviderModels(providerID);
  },
  async testProviderDraft(request) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.VerifyModelConfig) {
      return mapDraftProviderTest(await bridge.VerifyModelConfig(toRuntimeModelConfigRequest(request)));
    }
    if (runtimeHTTPBridge.VerifyModelConfig) {
      return mapDraftProviderTest(await runtimeHTTPBridge.VerifyModelConfig(toRuntimeModelConfigRequest(request)));
    }
    return staticWorkbenchAdapter.testProviderDraft(request);
  },
  async testConfiguredProvider(providerID) {
    const bridge = await loadRuntimeBridge();
    if (bridge?.TestConfiguredProvider) {
      return bridge.TestConfiguredProvider(providerID);
    }
    return runtimeHTTPBridge.TestConfiguredProvider?.(providerID) ?? staticWorkbenchAdapter.testConfiguredProvider(providerID);
  },
  async measureProviderDraftLatency(request) {
    const startedAt = performance.now();
    const bridge = await loadRuntimeBridge();
    if (bridge?.VerifyModelConfig) {
      return mapDraftProviderTest(await bridge.VerifyModelConfig(toRuntimeModelConfigRequest(request)), Math.round(performance.now() - startedAt));
    }
    if (runtimeHTTPBridge.VerifyModelConfig) {
      return mapDraftProviderTest(await runtimeHTTPBridge.VerifyModelConfig(toRuntimeModelConfigRequest(request)), Math.round(performance.now() - startedAt));
    }
    return staticWorkbenchAdapter.measureProviderDraftLatency(request);
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
