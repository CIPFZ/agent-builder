import type { AgentRuntime, RuntimeChatRequest } from './types'
import { loadWailsRuntimeBridge } from './wailsBinding'

export const wailsRuntime: AgentRuntime = {
  async cancel() {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.Cancel()
  },

  async chat(request: RuntimeChatRequest) {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.Chat(request)
  },

  async decidePermission(request) {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.DecidePermission(request)
  },

  async getModelConfig() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.GetModelConfig()
    return response.config
  },

  async getEventsEndpoint() {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.EventsEndpoint()
  },

  async listCapabilities() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Capabilities()
    return response.capabilities
  },

  async listEvents() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Events()
    return response.events
  },

  async listMcpServers() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.MCPServers()
    return response.servers
  },

  async listModels() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Models()
    return response.models
  },

  async listMessages() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Messages()
    return response.messages
  },

  async listPermissions() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Permissions()
    return response.permissions
  },

  async listSkills() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Skills()
    return response.skills
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
