export type ModelConfig = {
  protocol: 'openai' | 'anthropic'
  model: string
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
  provider: 'deepseek'
  content: string
}

export type ModelsResponse = {
  models: string[]
}

export async function requestChatCompletion(request: ChatRequest): Promise<ChatResponse> {
  const response = await fetch('http://127.0.0.1:4177/api/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
  })

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new Error(payload.message || `Chat request failed with ${response.status}`)
  }

  return response.json()
}

export async function requestConfiguredModels(): Promise<ModelsResponse> {
  const response = await fetch('http://127.0.0.1:4177/api/models')

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new Error(payload.message || `Models request failed with ${response.status}`)
  }

  return response.json()
}
