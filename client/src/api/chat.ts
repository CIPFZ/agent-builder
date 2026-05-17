import { getAgentRuntime } from '../runtime'

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

function lastUserPrompt(request: ChatRequest) {
  return [...request.messages].reverse().find((item) => item.role === 'user')?.content ?? ''
}

export async function requestChatCompletion(request: ChatRequest): Promise<ChatResponse> {
  const response = await getAgentRuntime().chat({ prompt: lastUserPrompt(request) })

  return {
    provider: response.provider,
    content: response.content,
    model: response.model,
  }
}

export async function requestConfiguredModels(): Promise<ModelsResponse> {
  const models = await getAgentRuntime().listModels()

  return { models: models.map((item) => item.id) }
}

export async function requestRuntimeStatus() {
  return getAgentRuntime().status()
}

export async function startRuntimeChat(title: string) {
  return getAgentRuntime().newChat(title)
}
