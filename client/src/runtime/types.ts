export type RuntimeStatus = {
  ready: boolean
  workspaceId: string
  sessionId: string
  workingDir: string
  model: string
  provider: string
  busy: boolean
  usage: RuntimeUsage
  events: RuntimeEventStats
  requests: RuntimeRequests
}

export type RuntimeModel = {
  id: string
  name: string
  provider: string
  selected: boolean
}

export type RuntimeChatRequest = {
  prompt: string
  sessionId?: string
}

export type RuntimeChatResponse = {
  requestId: string
  turnId?: string
  status: RuntimeStatus
}

export type RuntimeTurn = {
  id: string
  sessionId: string
  status: string
  userMessageId?: string
  latestAssistantMessageId?: string
  startedAt: number
  finishedAt?: number
  durationMs?: number
  provider?: string
  model?: string
  promptPreview?: string
  usageBefore?: RuntimeUsage
  usageAfter?: RuntimeUsage
  usageDelta?: RuntimeUsage
  latestMessageId?: string
  latestAssistant?: RuntimeMessage
  error?: string
}

export type RuntimePermissionRequest = {
  id: string
  sessionId: string
  turnId?: string
  toolCallId: string
  toolName: string
  description?: string
  action: string
  params?: unknown
  path?: string
  target?: string
  risk?: string
  policyMode?: string
  policyReason?: string
  policyProfile?: string
  policyHeadless?: boolean
  policyHeadlessReason?: string
  policyRuleId?: string
  policyRuleSource?: string
  policyScopeKind?: string
  policyScopeValue?: string
  policyTargetSummary?: string
  decision?: string
  reason?: string
  status?: string
  createdAt: number
  decidedAt?: number
}

export type RuntimeToolCall = {
  id: string
  sessionId: string
  turnId: string
  messageId?: string
  name: string
  source: 'builtin' | 'mcp' | 'shell' | 'unknown' | string
  capabilityId?: string
  jobId?: string
  command?: string
  risk?: string
  policyReason?: string
  policyMode?: string
  policyProfile?: string
  policyHeadless?: boolean
  policyHeadlessReason?: string
  policyRuleId?: string
  policyRuleSource?: string
  policyScopeKind?: string
  policyScopeValue?: string
  policyTargetSummary?: string
  shellRisk?: string
  shellReason?: string
  exitCode?: number
  jobStatus?: string
  jobStartedAt?: number
  jobFinishedAt?: number
  status:
    | 'pending'
    | 'waiting_permission'
    | 'running'
    | 'completed'
    | 'failed'
    | 'cancelled'
    | 'denied'
    | string
  inputSummary?: string
  outputSummary?: string
  modelContent?: string
  structuredOutput?: string
  stdout?: string
  stderr?: string
  outputRefs?: string[]
  artifactRefs?: string[]
  diffRefs?: string[]
  isError?: boolean
  compacted?: boolean
  compactRef?: string
  compactBoundaryId?: string
  compactOriginalEstimatedTokens?: number
  compactedAt?: number
  startedAt: number
  finishedAt?: number
  error?: string
}

export type RuntimeHook = {
  id: string
  name: string
  source: string
  event: string
  matcher?: string
  enabled: boolean
  status: string
  diagnostics?: string
  reason?: string
  timeout?: number
}

export type RuntimeHookExecution = {
  id: string
  hookId: string
  hookName?: string
  hookSource?: string
  event: string
  status: string
  sessionId?: string
  turnId?: string
  toolCallId?: string
  taskId?: string
  capabilityId?: string
  mcpServer?: string
  skill?: string
  contextRef?: string
  policyMode?: string
  policyProfile?: string
  policyRule?: string
  policyDecision?: string
  policyReason?: string
  headless?: boolean
  headlessReason?: string
  sandboxDecisionId?: string
  sandboxStatus?: string
  scopeKind?: string
  scopeValue?: string
  reason?: string
  error?: string
  inputSummary?: string
  outputSummary?: string
  contextSummary?: string
  inputRewritten?: boolean
  contextInjected?: boolean
  redacted: boolean
  startedAt: number
  completedAt?: number
  durationMs?: number
}

export type RuntimeHookExecutionListRequest = {
  sessionId?: string
  turnId?: string
  toolCallId?: string
  taskId?: string
  event?: string
  status?: string
}

export type RuntimeRef = {
  id: string
  uri: string
  sessionId: string
  turnId?: string
  toolCallId?: string
  taskId?: string
  kind: string
  mediaType?: string
  contentType?: string
  sizeBytes: number
  estimatedTokens: number
  preview?: string
  summary?: string
  storageKind: string
  storagePath?: string
  redactionStatus: string
  createdAt: number
  canReadContent: boolean
}

export type RuntimeRefsResponse = {
  refs: RuntimeRef[]
}

export type RuntimeRefResponse = {
  ref: RuntimeRef
}

export type RuntimeRefContentResponse = {
  ref: RuntimeRef
  content?: string
  redacted?: boolean
  truncated?: boolean
}

export type RuntimeAgentTask = {
  id: string
  parentTurnId?: string
  parentSessionId: string
  parentToolCallId?: string
  childSessionId?: string
  title: string
  kind: 'subagent' | 'agentic_fetch' | 'background' | string
  role?: string
  name?: string
  promptSummary?: string
  model?: string
  provider?: string
  allowedTools?: string[]
  capabilityScope?: string[]
  cwd?: string
  worktree?: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'interrupted' | string
  progress: number
  resultSummary?: string
  artifactRefs?: string[]
  startedAt: number
  updatedAt: number
  finishedAt?: number
  error?: string
  cancellationDetail?: string
  result?: RuntimeAgentTaskResult
}

export type RuntimeWorktree = {
  id: string
  sessionId: string
  turnId?: string
  taskId?: string
  baseRepoPath: string
  worktreePath: string
  branch: string
  ref?: string
  status: string
  preservePolicy: string
  cleanupPolicy: string
  createdAt: number
  enteredAt?: number
  exitedAt?: number
  cleanedAt?: number
  updatedAt: number
  error?: string
  owner?: string
  metadata?: Record<string, string>
}

export type RuntimeWorktreeCreateRequest = {
  sessionId?: string
  turnId?: string
  taskId?: string
  baseRepoPath?: string
  branch?: string
  ref?: string
  name?: string
  preservePolicy?: string
  cleanupPolicy?: string
}

export type RuntimeWorktreeActionRequest = {
  sessionId?: string
  turnId?: string
  taskId?: string
  preservePolicy?: string
}

export type RuntimeEffectiveScope = {
  sessionId?: string
  turnId?: string
  taskId?: string
  baseCwd?: string
  effectiveCwd?: string
  worktreeId?: string
  worktreePath?: string
  worktree?: RuntimeWorktree
  sandbox?: string
  remote?: string
}

export type RuntimeAgentRoleDefinition = {
  id: string
  name: string
  title?: string
  description?: string
  promptSummary?: string
  allowedTools?: string[]
  capabilityScope?: string[]
  model?: string
  provider?: string
  cwd?: string
  worktree?: string
  risk?: string
  policyMetadata?: Record<string, string>
  source?: string
  createdAt?: number
  updatedAt?: number
}

export type RuntimeAgentTaskMessage = {
  id: string
  taskId: string
  parentTaskId?: string
  parentTurnId?: string
  parentSessionId?: string
  childSessionId?: string
  direction: 'parent_to_child' | 'child_to_parent' | string
  kind: 'instruction' | 'control' | 'progress' | 'result' | 'artifact' | string
  status: string
  contentSummary?: string
  payload?: Record<string, unknown>
  relatedToolCallId?: string
  relatedMessageId?: string
  artifactRefs?: string[]
  createdAt: number
  deliveredAt?: number
}

export type RuntimeAgentTaskResult = {
  taskId: string
  status: string
  summary?: string
  errorDetail?: string
  cancellationDetail?: string
  artifactRefs?: string[]
  relatedMessageRefs?: string[]
  relatedToolCallRefs?: string[]
  compactBoundaryRefs?: string[]
  createdAt: number
  updatedAt: number
}

export type RuntimeTodo = {
  content: string
  status: 'pending' | 'in_progress' | 'completed' | string
  activeForm?: string
}

export type RuntimeTodoSummary = {
  sessionId: string
  turnId?: string
  todos: RuntimeTodo[]
  pending: number
  inProgress: number
  completed: number
  total: number
  updatedAt?: number
}

export type RuntimePermissionRisk = 'read' | 'write' | 'execute' | 'network' | 'secret' | 'destructive' | string

export type RuntimePermissionDecision = {
  permissionId: string
  action: 'allow' | 'allow_session' | 'deny'
}

export type RuntimeMcpRequestKind = 'auth' | 'elicitation' | string
export type RuntimeMcpRequestStatus =
  | 'none'
  | 'not_required'
  | 'required'
  | 'pending'
  | 'approved'
  | 'completed'
  | 'denied'
  | 'failed'
  | 'expired'
  | 'cancelled'
  | string

export type RuntimeMcpRequest = {
  id: string
  kind: RuntimeMcpRequestKind
  server: string
  capabilityId?: string
  sessionId?: string
  turnId?: string
  status: RuntimeMcpRequestStatus
  prompt?: string
  description?: string
  responseSummary?: string
  policyMode?: string
  policyProfile?: string
  policyDecision?: string
  policyReason?: string
  policyRisk?: string
  policyRuleId?: string
  policyRuleSource?: string
  policyScopeKind?: string
  policyScopeValue?: string
  policyTargetSummary?: string
  policyHeadless?: boolean
  policyHeadlessReason?: string
  createdAt: number
  updatedAt: number
  expiresAt?: number
  completedAt?: number
  error?: string
  redacted: boolean
}

export type RuntimeMcpRequestListRequest = {
  kind?: RuntimeMcpRequestKind
  status?: RuntimeMcpRequestStatus
  server?: string
}

export type RuntimeMcpRequestDecision = {
  requestId: string
  action: 'approve' | 'complete' | 'answer' | 'submit' | 'deny' | 'cancel' | 'fail' | string
  responseSummary?: string
  error?: string
}

export type RuntimePolicyMode = 'ask' | 'auto_read' | 'plan' | 'deny_all'
export type RuntimePolicyDecision = 'allow' | 'ask' | 'deny'

export type RuntimePolicy = {
  mode: RuntimePolicyMode
  modes: RuntimePolicyMode[]
  profile?: string
  rules?: RuntimePolicyRule[]
  diagnostics?: RuntimePolicyDiagnostic[]
  description?: string
  updatedAt?: number
}

export type RuntimePolicyRule = {
  id: string
  decision: RuntimePolicyDecision
  source?: string
  reason?: string
  tool?: string
  capabilityId?: string
  builtinTool?: string
  mcpServer?: string
  mcpTool?: string
  mcpResource?: string
  mcpPrompt?: string
  skill?: string
  subagent?: string
  taskScope?: string
  cwdPrefix?: string
  pathPrefix?: string
  shellPrefix?: string
  shellRegex?: string
  policyMode?: RuntimePolicyMode
  policyProfile?: string
  scopeKind?: string
  scopeValue?: string
  precedence?: number
}

export type RuntimePolicyDiagnostic = {
  ruleId?: string
  level: 'error' | 'warning' | 'info' | string
  reason: string
}

export type RuntimePolicyEvaluation = {
  decision: RuntimePolicyDecision
  risk: RuntimePermissionRisk
  reason: string
  mode: RuntimePolicyMode
}

export type RuntimeEvent = {
  id: string
  sequence: number
  type: string
  created_at: string
  session_id?: string
  turn_id?: string
  message_id?: string
  tool_call_id?: string
  payload?: Record<string, unknown>
}

export type RuntimeEventsEndpoint = {
  url: string
  token?: string
}

export type RuntimeEventsResponse = {
  events: RuntimeEvent[]
  snapshot_required?: boolean
  first_sequence?: number
  last_sequence?: number
}

export type RuntimeReplayExportRequest = {
  sessionId?: string
  turnId?: string
  after?: number
}

export type RuntimeReplayToolSearch = {
  query?: string
  selected?: string[]
  omittedCount?: number
  budgetImpact?: RuntimeToolSchemaBudgetImpact
  guardrail?: string
}

export type RuntimeReplayToolDiscovery = {
  selected?: string[]
  omitted?: string[]
  denied?: string[]
}

export type RuntimeReplayLifecycle = {
  started?: string[]
  allowed?: string[]
  denied?: string[]
  loaded?: string[]
  failed?: string[]
  disabled?: string[]
  updated?: string[]
}

export type RuntimeReplayPolicyDecision = {
  toolCallId?: string
  toolName?: string
  decision?: string
  risk?: string
  reason?: string
  mode?: string
  profile?: string
  matchedRuleId?: string
  matchedRuleSource?: string
  scopeKind?: string
  scopeValue?: string
  shellRisk?: string
  shellReason?: string
  headless?: boolean
  headlessReason?: string
}

export type RuntimeReplayPermission = {
  permissionId?: string
  toolCallId?: string
  toolName?: string
  action?: string
  decision?: string
  status?: string
  risk?: string
  reason?: string
}

export type RuntimeReplayMcpRequest = {
  requestId?: string
  kind?: string
  server?: string
  capabilityId?: string
  sessionId?: string
  turnId?: string
  status?: string
  decision?: string
  error?: string
  policyDecision?: string
  policyMode?: string
  policyProfile?: string
  policyReason?: string
  redacted: boolean
}

export type RuntimeReplayRecovery = {
  snapshotRequired?: boolean
  pendingPermissions?: number
  pendingMcpRequests?: number
  activeTurns?: number
  interruptedTurns?: number
  lastEventSequence?: number
}

export type RuntimeReplayExportSummary = {
  compactBoundaries?: RuntimeCompactBoundary[]
  budget?: RuntimeBudgetReport
  worktrees?: RuntimeWorktree[]
  toolSearches?: RuntimeReplayToolSearch[]
  toolDiscovery?: RuntimeReplayToolDiscovery
  capabilities?: RuntimeReplayLifecycle
  skills?: RuntimeReplayLifecycle
  mcp?: RuntimeReplayLifecycle
  mcpRequests?: RuntimeReplayMcpRequest[]
  agentTaskMessages?: RuntimeAgentTaskMessage[]
  agentTaskResults?: RuntimeAgentTaskResult[]
  agentTaskArtifacts?: string[]
  outputRefs?: RuntimeRef[]
  artifactRefs?: RuntimeRef[]
  compactOutputRefs?: RuntimeRef[]
  taskArtifactRefs?: RuntimeRef[]
  policyDecisions?: RuntimeReplayPolicyDecision[]
  hooks?: RuntimeHookExecution[]
  permissionEvents?: RuntimeReplayPermission[]
  readFiles?: RuntimeReadFileState[]
  toolCalls?: RuntimeToolCall[]
  recovery?: RuntimeReplayRecovery
  eventCounts?: Record<string, number>
  auditCounts?: Record<string, number>
  redacted: boolean
}

export type RuntimeReplayExportResponse = {
  sessionId?: string
  turnId?: string
  generatedAt: string
  source: string
  snapshotRequired?: boolean
  firstSequence?: number
  lastSequence?: number
  events: RuntimeEvent[]
  audit: RuntimeAuditEvent[]
  summary: RuntimeReplayExportSummary
}

export type RuntimeRecoveryStatus = {
  runtime_started_at: string
  last_event_sequence: number
  active_turns: RuntimeTurn[]
  interrupted_turns: RuntimeTurn[]
  interrupted_tasks?: RuntimeAgentTask[]
  compact_boundaries?: RuntimeCompactBoundary[]
  worktrees?: RuntimeWorktree[]
  hook_executions?: RuntimeHookExecution[]
  pending_permissions: RuntimePermissionRequest[]
  pending_mcp_requests?: RuntimeMcpRequest[]
  snapshot_required?: boolean
}

export type RuntimeBudgetBucket = {
  count: number
  estimatedTokens: number
}

export type RuntimeBudgetReport = {
  sessionId?: string
  turnId?: string
  model?: string
  contextWindow?: number
  inputBudget: RuntimeBudgetBucket
  messages: RuntimeBudgetBucket
  contextSources: RuntimeBudgetBucket
  toolSchemas: RuntimeBudgetBucket
  skills: RuntimeBudgetBucket
  mcp: RuntimeBudgetBucket
  toolOutputs: RuntimeBudgetBucket
  selectedToolSchemas: RuntimeBudgetBucket
  omittedToolSchemas: RuntimeBudgetBucket
  totalEstimatedTokens: number
  updatedAt: number
}

export type RuntimeCompactToolCallRef = {
  toolCallId: string
  name?: string
  ref?: string
  estimatedTokens?: number
  replacement?: string
  preserved?: boolean
  reason?: string
}

export type RuntimeReinjectedRef = {
  id: string
  kind: string
  name?: string
  path?: string
  uri?: string
  ref?: string
  status: 'recorded' | 'completed' | 'skipped' | 'failed' | string
  reason?: string
  error?: string
  contentSummary?: string
  tokenEstimate?: number
}

export type RuntimeCompactBoundary = {
  id: string
  sessionId: string
  turnId?: string
  kind: 'boundary' | 'micro' | 'full' | 'session_memory' | 'auto' | string
  trigger: string
  status: 'recorded' | 'completed' | 'skipped' | 'failed' | string
  budgetBefore?: RuntimeBudgetReport
  budgetAfter?: RuntimeBudgetReport
  summaryRef?: string
  messageRefs?: string[]
  toolCallRefs?: RuntimeCompactToolCallRef[]
  reinjectedRefs?: RuntimeReinjectedRef[]
  error?: string
  createdAt: number
  completedAt?: number
}

export type RuntimeCompactBoundariesResponse = {
  boundaries: RuntimeCompactBoundary[]
}

export type RuntimeAPIEndpoint = {
  url: string
  token: string
}

export type RuntimeSkill = {
  name: string
  description?: string
  builtin: boolean
  enabled: boolean
  path?: string
  skill_file_path?: string
  state: string
  diagnostics?: string
  error?: string
  reason?: string
  allowed_tools?: string[]
  activation?: RuntimeSkillActivationMetadata
  activation_metadata?: RuntimeSkillActivationMetadata
  metadata?: Record<string, string>
  capability_id?: string
  policy_mode?: string
  policy_risk?: string
  policy_reason?: string
}

export type RuntimeSkillActivationMetadata = {
  available: boolean
  included: boolean
  reason?: string
}

export type RuntimeSkillCreateRequest = {
  name: string
  description: string
  instructions: string
  directory?: string
}

export type RuntimeMcpServer = {
  name: string
  type: string
  url?: string
  command?: string
  args?: string[]
  disabled: boolean
  state: string
  counts: {
    tools: number
    prompts: number
    resources: number
  }
  diagnostics?: string
  reason?: string
  error?: string
  env?: Record<string, string>
  headers?: Record<string, string>
  enabled_tools?: string[]
  disabled_tools?: string[]
}

export type RuntimeMcpServerConfig = {
  name: string
  type: string
  url?: string
  command?: string
  args?: string[]
  disabled?: boolean
  env?: Record<string, string>
  headers?: Record<string, string>
  enabled_tools?: string[]
  disabled_tools?: string[]
}

export type RuntimeMcpTool = {
  server: string
  name: string
  description?: string
  enabled: boolean
  input_schema?: unknown
}

export type RuntimeMcpResource = {
  server: string
  uri: string
  name?: string
  description?: string
  mime_type?: string
}

export type RuntimeMcpPrompt = {
  server: string
  name: string
  description?: string
}

export type RuntimeCapability = {
  id: string
  kind: string
  name: string
  source?: string
  enabled: boolean
  risk: string
  description?: string
  state: 'unavailable' | 'disabled' | 'unloaded' | 'loading' | 'loaded' | 'failed' | string
  diagnostics?: string
  error?: string
  reason?: string
  capabilityId?: string
  schemaDigest?: string
  schemaSummary?: string
  searchText?: string
}

export type RuntimeToolSearchRequest = {
  query: string
  maxResults?: number
  turnId?: string
  sessionId?: string
  source?: string
}

export type RuntimeToolSearchResult = {
  id: string
  kind: string
  name: string
  source?: string
  description?: string
  risk?: string
  capabilityId?: string
  schemaDigest?: string
  schemaSummary?: string
  state?: string
  score?: number
}

export type RuntimeToolSearchOmission = {
  id: string
  kind?: string
  name?: string
  source?: string
  reason: string
  risk?: string
  state?: string
}

export type RuntimeToolSchemaBudgetImpact = {
  selected: RuntimeBudgetBucket
  omitted: RuntimeBudgetBucket
}

export type RuntimeToolSearchResponse = {
  query: string
  results: RuntimeToolSearchResult[]
  omitted?: RuntimeToolSearchOmission[]
  total: number
  budgetImpact: RuntimeToolSchemaBudgetImpact
  guardrail?: string
  guardrailError?: string
}

export type RuntimeContextSource = {
  id: string
  kind: 'managed_instructions' | 'user_memory' | 'project_agents' | 'project_claude' | 'dot_claude' | 'claude_rule' | 'local_memory' | 'skill_context' | 'mcp_resource_context' | 'context_file' | 'read_file_state' | 'compact_reinjected_context' | string
  name: string
  path?: string
  uri?: string
  scope?: string
  enabled: boolean
  state: 'unavailable' | 'disabled' | 'unloaded' | 'loading' | 'loaded' | 'failed' | string
  reason?: string
  diagnostics?: string
  error?: string
  content_summary?: string
  token_estimate?: number
  loaded_at?: string
  provenance?: string
  parent_id?: string
  rule_globs?: string[]
  size_bytes?: number
  mtime_unix?: number
  content_hash?: string
}

export type RuntimeReadFileState = {
  sessionId: string
  turnId?: string
  toolCallId?: string
  path: string
  readAt: number
  sizeBytes?: number
  contentHash?: string
  mtimeUnix?: number
  offset?: number
  limit?: number
  partial?: boolean
  tokenEstimate?: number
  state: 'recorded' | 'stale' | 'missing' | string
  reason?: string
  diagnostics?: string
}

export type RuntimeMessage = {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  parts?: RuntimeMessagePart[]
  provider?: string
  model?: string
  createdAt: number
  updatedAt: number
  finished: boolean
  finishReason?: string
  error?: string
}

export type RuntimeSession = {
  id: string
  title: string
  messageCount: number
  promptTokens: number
  completionTokens: number
  cost: number
  createdAt: number
  updatedAt: number
  active: boolean
  usage: RuntimeUsage
}

export type RuntimeMessagePart = {
  type: 'text' | 'reasoning' | 'tool_call' | 'tool_result' | 'finish' | 'image_url' | 'binary'
  text?: string
  thinking?: string
  startedAt?: number
  finishedAt?: number
  toolCallId?: string
  name?: string
  input?: string
  finished?: boolean
  content?: string
  data?: string
  mimeType?: string
  metadata?: string
  isError?: boolean
  reason?: string
  message?: string
  details?: string
}

export type RuntimeUsage = {
  promptTokens: number
  completionTokens: number
  totalTokens: number
  cost: number
}

export type RuntimeRequests = {
  activeRequestId?: string
  activeStartedAt?: number
  activeDurationMs?: number
  running: number
}

export type RuntimeEventStats = {
  lastEventAt: number
  messageEvents: number
  sessionEvents: number
  otherEvents: number
  assistantEvents: number
  permissionEvents: number
}

export type RuntimeModelConfig = {
  protocol: 'openai' | 'anthropic'
  url: string
  apiKey?: string
  model: string
  proxy?: string
  models?: string[]
  hasApiKey?: boolean
  configPath?: string
}

export type RuntimeModelVerifyResponse = {
  ok: boolean
  protocol: RuntimeModelConfig['protocol']
  model: string
  models?: string[]
  error?: string
}

export type RuntimeModelDiscoveryResponse = {
  protocol: RuntimeModelConfig['protocol']
  model?: string
  models: string[]
  error?: string
}

export type RuntimeAuditEvent = {
  id: string
  session_id?: string
  turn_id?: string
  tool_call_id?: string
  permission_id?: string
  type: string
  created_at: string
  payload: Record<string, unknown>
}

export type RuntimeSkillTurnItem = {
  name: string
  capability_id?: string
  builtin: boolean
  path?: string
  skill_file_path?: string
  state?: string
  reason?: string
  error?: string
  allowed_tools?: string[]
}

export type RuntimeTurnSkillSummary = {
  available_count: number
  available?: RuntimeSkillTurnItem[]
  activated?: RuntimeSkillTurnItem[]
  excluded?: RuntimeSkillTurnItem[]
  failed?: RuntimeSkillTurnItem[]
  policy_mode?: string
  policy_risk?: string
  policy_reason?: string
  source_paths?: string[]
}

export type RuntimeTurnContextSummary = {
  available_count: number
  available?: RuntimeContextSource[]
  loaded?: RuntimeContextSource[]
  injected?: RuntimeContextSource[]
  skipped?: RuntimeContextSource[]
  failed?: RuntimeContextSource[]
  token_estimate?: number
}

export type AgentRuntime = {
  auditSession: (sessionId: string) => Promise<RuntimeAuditEvent[]>
  auditTurn: (turnId: string) => Promise<RuntimeAuditEvent[]>
  exportReplay: (request: RuntimeReplayExportRequest) => Promise<RuntimeReplayExportResponse>
  cancel: () => Promise<RuntimeStatus>
  cancelTurn: (turnId: string) => Promise<RuntimeStatus>
  chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  deleteSession: (sessionId: string) => Promise<RuntimeSession[]>
  decidePermission: (request: RuntimePermissionDecision) => Promise<RuntimeStatus>
  getPolicy: () => Promise<RuntimePolicy>
  getModelConfig: () => Promise<RuntimeModelConfig>
  getRecoveryStatus: () => Promise<RuntimeRecoveryStatus>
  getAPIEndpoint: () => Promise<RuntimeAPIEndpoint>
  getTurn: (turnId: string) => Promise<RuntimeTurn>
  listHooks: () => Promise<RuntimeHook[]>
  listHookExecutions: (request?: RuntimeHookExecutionListRequest) => Promise<RuntimeHookExecution[]>
  getHookExecution: (executionId: string) => Promise<RuntimeHookExecution>
  listTurnCompactBoundaries: (turnId: string) => Promise<RuntimeCompactBoundary[]>
  listSessionCompactBoundaries: (sessionId: string) => Promise<RuntimeCompactBoundary[]>
  listWorktrees: () => Promise<RuntimeWorktree[]>
  getWorktree: (worktreeId: string) => Promise<RuntimeWorktree>
  createWorktree: (request: RuntimeWorktreeCreateRequest) => Promise<RuntimeWorktree>
  enterWorktree: (worktreeId: string, request?: RuntimeWorktreeActionRequest) => Promise<RuntimeWorktree>
  exitWorktree: (worktreeId: string, request?: RuntimeWorktreeActionRequest) => Promise<RuntimeWorktree>
  cleanupWorktree: (worktreeId: string, request?: RuntimeWorktreeActionRequest) => Promise<RuntimeWorktree>
  getToolCall: (toolCallId: string) => Promise<RuntimeToolCall>
  getRef: (refId: string) => Promise<RuntimeRef>
  readRefContent: (refId: string) => Promise<RuntimeRefContentResponse>
  getAgentTask: (taskId: string) => Promise<RuntimeAgentTask>
  getAgentTaskEffectiveScope: (taskId: string) => Promise<RuntimeEffectiveScope>
  cancelAgentTask: (taskId: string) => Promise<RuntimeAgentTask>
  listAgentRoles: () => Promise<RuntimeAgentRoleDefinition[]>
  getAgentRole: (roleId: string) => Promise<RuntimeAgentRoleDefinition>
  listAgentTaskMessages: (taskId: string) => Promise<RuntimeAgentTaskMessage[]>
  getAgentTaskResult: (taskId: string) => Promise<RuntimeAgentTaskResult>
  getSessionTodos: (sessionId: string) => Promise<RuntimeTodoSummary>
  getTurnTodos: (turnId: string) => Promise<RuntimeTodoSummary>
  getEventsEndpoint: () => Promise<RuntimeEventsEndpoint>
  discoverModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelDiscoveryResponse>
  addSkillPath: (path: string) => Promise<RuntimeSkill[]>
  listCapabilities: () => Promise<RuntimeCapability[]>
  searchTools: (request: RuntimeToolSearchRequest) => Promise<RuntimeToolSearchResponse>
  refreshCapability: (capabilityId: string) => Promise<RuntimeCapability>
  listContextSources: () => Promise<RuntimeContextSource[]>
  listReadFiles: (sessionId?: string) => Promise<RuntimeReadFileState[]>
  listEvents: (after?: number) => Promise<RuntimeEventsResponse>
  listMcpServers: () => Promise<RuntimeMcpServer[]>
  listMcpRequests: (request?: RuntimeMcpRequestListRequest) => Promise<RuntimeMcpRequest[]>
  getMcpRequest: (requestId: string) => Promise<RuntimeMcpRequest>
  decideMcpRequest: (request: RuntimeMcpRequestDecision) => Promise<RuntimeMcpRequest>
  listMcpResources: (server: string) => Promise<RuntimeMcpResource[]>
  listMcpPrompts: (server: string) => Promise<RuntimeMcpPrompt[]>
  listMcpTools: (server: string) => Promise<RuntimeMcpTool[]>
  listModels: () => Promise<RuntimeModel[]>
  listMessages: () => Promise<RuntimeMessage[]>
  listSessionMessages: (sessionId: string) => Promise<RuntimeMessage[]>
  listPermissions: () => Promise<RuntimePermissionRequest[]>
  listSessions: () => Promise<RuntimeSession[]>
  listSkills: () => Promise<RuntimeSkill[]>
  listTurns: (status?: string) => Promise<RuntimeTurn[]>
  listTurnToolCalls: (turnId: string) => Promise<RuntimeToolCall[]>
  listRefs: (request?: {
    sessionId?: string
    turnId?: string
    toolCallId?: string
    taskId?: string
    kind?: string
  }) => Promise<RuntimeRef[]>
  listTurnAgentTasks: (turnId: string) => Promise<RuntimeAgentTask[]>
  createSkill: (request: RuntimeSkillCreateRequest) => Promise<RuntimeSkill[]>
  newChat: (title: string) => Promise<RuntimeStatus>
  refreshMcpServer: (server: string) => Promise<RuntimeMcpServer[]>
  retryMcpServer: (server: string) => Promise<RuntimeMcpServer[]>
  refreshSkills: () => Promise<RuntimeSkill[]>
  saveModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelConfig>
  updatePolicy: (mode: RuntimePolicyMode, rules?: RuntimePolicyRule[], profile?: string) => Promise<RuntimePolicy>
  saveMcpServer: (config: RuntimeMcpServerConfig) => Promise<RuntimeMcpServer[]>
  renameSession: (sessionId: string, title: string) => Promise<RuntimeSession[]>
  selectSession: (sessionId: string) => Promise<RuntimeStatus>
  setMcpServerEnabled: (server: string, enabled: boolean) => Promise<RuntimeMcpServer[]>
  setMcpToolEnabled: (server: string, tool: string, enabled: boolean) => Promise<RuntimeMcpTool[]>
  setSkillEnabled: (name: string, enabled: boolean) => Promise<RuntimeSkill[]>
  status: () => Promise<RuntimeStatus>
  verifyModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelVerifyResponse>
}
