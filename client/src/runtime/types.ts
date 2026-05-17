export type RuntimeStatus = {
  ready: boolean
  workspaceId: string
  sessionId: string
  workingDir: string
  model: string
  provider: string
  busy: boolean
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
  provider: string
  content: string
  model: string
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
  newChat: (title: string) => Promise<RuntimeStatus>
  saveModelConfig: (config: RuntimeModelConfig) => Promise<RuntimeModelConfig>
  status: () => Promise<RuntimeStatus>
}
