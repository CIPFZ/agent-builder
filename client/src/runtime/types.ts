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
}

export type RuntimeChatResponse = {
  requestId: string
  status: RuntimeStatus
}

export type RuntimePermissionRequest = {
  id: string
  sessionId: string
  toolCallId: string
  toolName: string
  description?: string
  action: string
  params?: unknown
  path?: string
  createdAt: number
}

export type RuntimePermissionDecision = {
  permissionId: string
  action: 'allow' | 'allow_session' | 'deny'
}

export type RuntimeEvent = {
  id: string
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

export type RuntimeMcpServer = {
  name: string
  type: string
  url?: string
  command?: string
  disabled: boolean
  state: string
  counts: {
    tools: number
    prompts: number
    resources: number
  }
  error?: string
}

export type RuntimeCapability = {
  id: string
  kind: string
  name: string
  source?: string
  enabled: boolean
  risk: string
  description?: string
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
  auditTurn: (turnId: string) => Promise<RuntimeAuditEvent[]>
  cancel: () => Promise<RuntimeStatus>
  chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  decidePermission: (request: RuntimePermissionDecision) => Promise<RuntimeStatus>
  getModelConfig: () => Promise<RuntimeModelConfig>
  getEventsEndpoint: () => Promise<RuntimeEventsEndpoint>
  listCapabilities: () => Promise<RuntimeCapability[]>
  listEvents: () => Promise<RuntimeEvent[]>
  listMcpServers: () => Promise<RuntimeMcpServer[]>
  listModels: () => Promise<RuntimeModel[]>
  listMessages: () => Promise<RuntimeMessage[]>
  listPermissions: () => Promise<RuntimePermissionRequest[]>
  listSkills: () => Promise<RuntimeSkill[]>
  newChat: (title: string) => Promise<RuntimeStatus>
  saveModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelConfig>
  status: () => Promise<RuntimeStatus>
  verifyModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelVerifyResponse>
}
