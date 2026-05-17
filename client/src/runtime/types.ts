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

export type AgentRuntime = {
  chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  listModels: () => Promise<RuntimeModel[]>
  newChat: (title: string) => Promise<RuntimeStatus>
  status: () => Promise<RuntimeStatus>
}
