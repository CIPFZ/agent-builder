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
  isError?: boolean
  startedAt: number
  finishedAt?: number
  error?: string
}

export type RuntimePermissionDecision = {
  permissionId: string
  action: 'allow' | 'allow_session' | 'deny'
}

export type RuntimePolicyMode = 'ask' | 'auto_read' | 'plan' | 'deny_all' | string

export type RuntimePolicy = {
  mode: RuntimePolicyMode
  modes: RuntimePolicyMode[]
  description?: string
  updatedAt?: number
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

export type RuntimeRecoveryStatus = {
  runtime_started_at: string
  last_event_sequence: number
  active_turns: RuntimeTurn[]
  interrupted_turns: RuntimeTurn[]
  pending_permissions: RuntimePermissionRequest[]
  snapshot_required?: boolean
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
  error?: string
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
  type: string
  created_at: string
  payload: Record<string, unknown>
}

export type AgentRuntime = {
  auditSession: (sessionId: string) => Promise<RuntimeAuditEvent[]>
  auditTurn: (turnId: string) => Promise<RuntimeAuditEvent[]>
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
  getToolCall: (toolCallId: string) => Promise<RuntimeToolCall>
  getEventsEndpoint: () => Promise<RuntimeEventsEndpoint>
  discoverModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelDiscoveryResponse>
  addSkillPath: (path: string) => Promise<RuntimeSkill[]>
  listCapabilities: () => Promise<RuntimeCapability[]>
  refreshCapability: (capabilityId: string) => Promise<RuntimeCapability>
  listEvents: (after?: number) => Promise<RuntimeEventsResponse>
  listMcpServers: () => Promise<RuntimeMcpServer[]>
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
  createSkill: (request: RuntimeSkillCreateRequest) => Promise<RuntimeSkill[]>
  newChat: (title: string) => Promise<RuntimeStatus>
  refreshMcpServer: (server: string) => Promise<RuntimeMcpServer[]>
  refreshSkills: () => Promise<RuntimeSkill[]>
  saveModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelConfig>
  updatePolicy: (mode: RuntimePolicyMode) => Promise<RuntimePolicy>
  saveMcpServer: (config: RuntimeMcpServerConfig) => Promise<RuntimeMcpServer[]>
  renameSession: (sessionId: string, title: string) => Promise<RuntimeSession[]>
  selectSession: (sessionId: string) => Promise<RuntimeStatus>
  setMcpServerEnabled: (server: string, enabled: boolean) => Promise<RuntimeMcpServer[]>
  setMcpToolEnabled: (server: string, tool: string, enabled: boolean) => Promise<RuntimeMcpTool[]>
  setSkillEnabled: (name: string, enabled: boolean) => Promise<RuntimeSkill[]>
  status: () => Promise<RuntimeStatus>
  verifyModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelVerifyResponse>
}
