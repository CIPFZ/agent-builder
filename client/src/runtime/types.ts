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

export type RuntimeMessage = {
  id: string
  sessionId: string
  role: 'user' | 'assistant'
  content: string
  provider?: string
  model?: string
  createdAt: number
  updatedAt: number
  finished: boolean
  finishReason?: string
  error?: string
}

export type RuntimeUsage = {
  promptTokens: number
  completionTokens: number
  totalTokens: number
  cost: number
}

export type RuntimeEventStats = {
  lastEventAt: number
  messageEvents: number
  sessionEvents: number
  otherEvents: number
  assistantEvents: number
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

export type AgentRuntime = {
  chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  getModelConfig: () => Promise<RuntimeModelConfig>
  listModels: () => Promise<RuntimeModel[]>
  listMessages: () => Promise<RuntimeMessage[]>
  newChat: (title: string) => Promise<RuntimeStatus>
  saveModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelConfig>
  status: () => Promise<RuntimeStatus>
}
