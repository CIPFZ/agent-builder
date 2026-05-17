import type { AgentRuntime, RuntimeChatRequest } from './types'
import { loadWailsRuntimeBridge } from './wailsBinding'

export const wailsRuntime: AgentRuntime = {
  async chat(request: RuntimeChatRequest) {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.Chat(request)
  },

  async getModelConfig() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.GetModelConfig()
    return response.config
  },

  async listModels() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Models()
    return response.models
  },

  async newChat(title: string) {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.NewChat(title)
  },

  async saveModelConfig(config) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.SaveModelConfig(config)
    return response.config
  },

  async status() {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.Status()
  },
}
