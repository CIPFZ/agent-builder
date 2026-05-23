import { getAgentRuntime } from './index'
import type { RuntimeMcpServerConfig, RuntimePermissionDecision, RuntimeSkillCreateRequest } from './types'

export type ModelConfig = {
  protocol: 'openai' | 'anthropic'
  model: string
  provider?: string
  url: string
  apiKey?: string
  proxy?: string
  models?: string[]
  hasApiKey?: boolean
  configPath?: string
}

export type ModelsResponse = {
  models: string[]
}

export async function sendRuntimePrompt(prompt: string, sessionId?: string) {
  const response = await getAgentRuntime().chat({ prompt, sessionId })
  return response
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
    models: config.models,
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
    models: saved.models,
    proxy: saved.proxy,
    hasApiKey: saved.hasApiKey,
    configPath: saved.configPath,
  }
}

export async function verifyModelConfig(config: ModelConfig) {
  return getAgentRuntime().verifyModelConfig(config)
}

export async function discoverModelConfig(config: ModelConfig) {
  return getAgentRuntime().discoverModelConfig(config)
}

export async function requestRuntimeStatus() {
  return getAgentRuntime().status()
}

export async function requestRuntimeRecoveryStatus() {
  return getAgentRuntime().getRecoveryStatus()
}

export async function requestRuntimeMessages() {
  return getAgentRuntime().listMessages()
}

export async function requestRuntimeSessions() {
  return getAgentRuntime().listSessions()
}

export async function requestRuntimeSessionMessages(sessionId: string) {
  return getAgentRuntime().listSessionMessages(sessionId)
}

export async function requestRuntimeEvents() {
  return getAgentRuntime().listEvents()
}

export async function requestRuntimeEventsEndpoint() {
  return getAgentRuntime().getEventsEndpoint()
}

export async function requestRuntimePermissions() {
  return getAgentRuntime().listPermissions()
}

export async function requestRuntimeSkills() {
  return getAgentRuntime().listSkills()
}

export async function requestRuntimeMcpServers() {
  return getAgentRuntime().listMcpServers()
}

export async function requestRuntimeMcpTools(server: string) {
  return getAgentRuntime().listMcpTools(server)
}

export async function requestRuntimeMcpResources(server: string) {
  return getAgentRuntime().listMcpResources(server)
}

export async function requestRuntimeMcpPrompts(server: string) {
  return getAgentRuntime().listMcpPrompts(server)
}

export async function requestRuntimeCapabilities() {
  return getAgentRuntime().listCapabilities()
}

export async function requestRuntimeAudit(turnId: string) {
  return getAgentRuntime().auditTurn(turnId)
}

export async function requestRuntimeTurns(status?: string) {
  return getAgentRuntime().listTurns(status)
}

export async function requestRuntimeSessionAudit(sessionId: string) {
  return getAgentRuntime().auditSession(sessionId)
}

export async function decideRuntimePermission(request: RuntimePermissionDecision) {
  return getAgentRuntime().decidePermission(request)
}

export async function cancelRuntimeTurn() {
  return getAgentRuntime().cancel()
}

export async function cancelRuntimeTurnById(turnId: string) {
  return getAgentRuntime().cancelTurn(turnId)
}

export async function refreshRuntimeSkills() {
  return getAgentRuntime().refreshSkills()
}

export async function createRuntimeSkill(request: RuntimeSkillCreateRequest) {
  return getAgentRuntime().createSkill(request)
}

export async function addRuntimeSkillPath(path: string) {
  return getAgentRuntime().addSkillPath(path)
}

export async function setRuntimeSkillEnabled(name: string, enabled: boolean) {
  return getAgentRuntime().setSkillEnabled(name, enabled)
}

export async function refreshRuntimeMcpServer(server: string) {
  return getAgentRuntime().refreshMcpServer(server)
}

export async function saveRuntimeMcpServer(config: RuntimeMcpServerConfig) {
  return getAgentRuntime().saveMcpServer(config)
}

export async function setRuntimeMcpServerEnabled(server: string, enabled: boolean) {
  return getAgentRuntime().setMcpServerEnabled(server, enabled)
}

export async function setRuntimeMcpToolEnabled(server: string, tool: string, enabled: boolean) {
  return getAgentRuntime().setMcpToolEnabled(server, tool, enabled)
}

export async function startRuntimeChat(title: string) {
  return getAgentRuntime().newChat(title)
}

export async function selectRuntimeSession(sessionId: string) {
  return getAgentRuntime().selectSession(sessionId)
}

export async function renameRuntimeSession(sessionId: string, title: string) {
  return getAgentRuntime().renameSession(sessionId, title)
}

export async function deleteRuntimeSession(sessionId: string) {
  return getAgentRuntime().deleteSession(sessionId)
}
