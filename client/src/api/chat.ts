export type ModelConfig = {
  protocol: 'openai' | 'anthropic'
  model: string
  provider?: string
  url: string
  apiKey?: string
  proxy?: string
}

export type ChatMessagePayload = {
  role: 'user' | 'assistant'
  content: string
}

export type ChatRequest = {
  config: ModelConfig
  messages: ChatMessagePayload[]
}

export type ChatResponse = {
  provider: string
  content: string
  model?: string
}

export type ModelsResponse = {
  models: string[]
}

type RuntimeModel = {
  id: string
  name: string
  provider: string
  selected: boolean
}

type RuntimeChatResponse = {
  provider: string
  content: string
  model: string
}

type RuntimeStatus = {
  ready: boolean
  workspaceId: string
  sessionId: string
  workingDir: string
  model: string
  provider: string
  busy: boolean
}

type RuntimeBridgeApi = {
  Chat: (request: { prompt: string }) => Promise<RuntimeChatResponse>
  Models: () => Promise<{ models: RuntimeModel[] }>
  NewChat: (title: string) => Promise<RuntimeStatus>
  Status: () => Promise<RuntimeStatus>
}

async function runtimeBridge(): Promise<RuntimeBridgeApi> {
  const bindingPath = '/bindings/github.com/charmbracelet/crush/desktop/agent-builder/index.js'
  const module = (await import(/* @vite-ignore */ bindingPath)) as { RuntimeBridge: RuntimeBridgeApi }

  return module.RuntimeBridge
}

function lastUserPrompt(request: ChatRequest) {
  return [...request.messages].reverse().find((item) => item.role === 'user')?.content ?? ''
}

export async function requestChatCompletion(request: ChatRequest): Promise<ChatResponse> {
  const bridge = await runtimeBridge()
  const response = await bridge.Chat({ prompt: lastUserPrompt(request) })

  return {
    provider: response.provider,
    content: response.content,
    model: response.model,
  }
}

export async function requestConfiguredModels(): Promise<ModelsResponse> {
  const bridge = await runtimeBridge()
  const response = await bridge.Models()
  const models = response.models.map((item: RuntimeModel) => item.id)

  return { models }
}

export async function requestRuntimeStatus() {
  const bridge = await runtimeBridge()
  return bridge.Status()
}

export async function startRuntimeChat(title: string) {
  const bridge = await runtimeBridge()
  return bridge.NewChat(title)
}
