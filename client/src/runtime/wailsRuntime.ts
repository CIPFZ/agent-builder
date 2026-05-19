import type { AgentRuntime, RuntimeChatRequest } from './types'
import { loadWailsRuntimeBridge } from './wailsBinding'

export const wailsRuntime: AgentRuntime = {
  async auditSession(sessionId: string) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.AuditSession(sessionId)
    return response.events
  },

  async auditTurn(turnId: string) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.AuditTurn(turnId)
    return response.events
  },

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

  async deleteSession(sessionId) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.DeleteSession(sessionId)
    return response.sessions
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

  async listMcpTools(server) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.MCPTools(server)
    return response.tools
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

  async listSessionMessages(sessionId) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.SessionMessages(sessionId)
    return response.messages
  },

  async listPermissions() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Permissions()
    return response.permissions
  },

  async listSessions() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.Sessions()
    return response.sessions
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

  async refreshMcpServer(server) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.RefreshMCPServer(server)
    return response.servers
  },

  async refreshSkills() {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.RefreshSkills()
    return response.skills
  },

  async renameSession(sessionId, title) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.RenameSession({ sessionId, title })
    return response.sessions
  },

  async saveMcpServer(config) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.SaveMCPServer(config)
    return response.servers
  },

  async selectSession(sessionId) {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.SelectSession(sessionId)
  },

  async setMcpServerEnabled(server, enabled) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.SetMCPServerEnabled({ name: server, enabled })
    return response.servers
  },

  async setMcpToolEnabled(server, tool, enabled) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.SetMCPToolEnabled({ server, tool, enabled })
    return response.tools
  },

  async setSkillEnabled(name, enabled) {
    const bridge = await loadWailsRuntimeBridge()
    const response = await bridge.SetSkillEnabled({ name, enabled })
    return response.skills
  },

  async status() {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.Status()
  },

  async verifyModelConfig(config) {
    const bridge = await loadWailsRuntimeBridge()
    return bridge.VerifyModelConfig(config)
  },
}
