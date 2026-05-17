import { getAgentRuntime } from '../runtime'
import type { RuntimeMessage } from '../runtime'

export type ModelConfig = {
  protocol: 'openai' | 'anthropic'
  model: string
  provider?: string
  url: string
  apiKey?: string
  proxy?: string
  hasApiKey?: boolean
  configPath?: string
}

export type ChatResponse = {
  message: RuntimeMessage
}

export type ModelsResponse = {
  models: string[]
}

export async function sendRuntimePrompt(prompt: string): Promise<ChatResponse> {
  const response = await getAgentRuntime().chat({ prompt })

  return {
    message: response.message,
  }
}

export async function requestConfiguredModels(): Promise<ModelsResponse> {
  const models = await getAgentRuntime().listModels()

  return { models: models.map((item) => item.id) }
}

export async function loadModelConfig(): Promise<ModelConfig> {
  const config = await getAgentRuntime().getModelConfig()
  return {
    protocol: config.protocol,
    url: config.url,
    apiKey: config.apiKey,
    model: config.model,
    proxy: config.proxy,
    hasApiKey: config.hasApiKey,
    configPath: config.configPath,
  }
}

export async function saveModelConfig(config: ModelConfig): Promise<ModelConfig> {
  const saved = await getAgentRuntime().saveModelConfig(config)
  return {
    protocol: saved.protocol,
    url: saved.url,
    apiKey: saved.apiKey,
    model: saved.model,
    proxy: saved.proxy,
    hasApiKey: saved.hasApiKey,
    configPath: saved.configPath,
  }
}

export async function requestRuntimeStatus() {
  return getAgentRuntime().status()
}

export async function requestRuntimeMessages() {
  return getAgentRuntime().listMessages()
}

export async function startRuntimeChat(title: string) {
  return getAgentRuntime().newChat(title)
}
