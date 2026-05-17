export type ModelConfig = {
  provider: 'deepseek' | 'openai-compatible' | 'mock'
  model: string
  apiBase: string
  temperature?: number
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
  provider: 'deepseek' | 'fallback'
  content: string
}

export async function requestChatCompletion(request: ChatRequest): Promise<ChatResponse> {
  const apiBase = request.config.apiBase.replace(/\/$/, '')
  const response = await fetch(`${apiBase}/api/chat`, {
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
