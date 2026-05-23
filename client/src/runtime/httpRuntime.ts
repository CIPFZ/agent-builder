import type {
  AgentRuntime,
  RuntimeAuditEvent,
  RuntimeCapability,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeEventsResponse,
  RuntimeEventsEndpoint,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpServer,
  RuntimeMcpServerConfig,
  RuntimeMcpTool,
  RuntimeMessage,
  RuntimeModel,
  RuntimeModelConfig,
  RuntimeModelVerifyResponse,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeSession,
  RuntimeSkill,
  RuntimeSkillCreateRequest,
  RuntimeStatus,
  RuntimeToolCall,
  RuntimeTurn,
} from './types'

type RuntimeHTTPOptions = {
  baseUrl: string
  token: string
}

function trimBaseUrl(baseUrl: string) {
  return baseUrl.replace(/\/+$/, '')
}

function encodePath(value: string) {
  return encodeURIComponent(value)
}

async function runtimeFetch<T>(options: RuntimeHTTPOptions, path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${trimBaseUrl(options.baseUrl)}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${options.token}`,
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (!response.ok) {
    let message = `Runtime API request failed: ${response.status}`
    try {
      const body = (await response.json()) as { error?: string }
      if (body.error) {
        message = body.error
      }
    } catch {
      // Keep the status-based error.
    }
    throw new Error(message)
  }
  return (await response.json()) as T
}

function body(method: 'POST' | 'PUT', value: unknown): RequestInit {
  return {
    method,
    body: JSON.stringify(value),
  }
}

export function createHTTPRuntime(options: RuntimeHTTPOptions): AgentRuntime {
  const get = <T>(path: string) => runtimeFetch<T>(options, path)
  const post = <T>(path: string, value: unknown = {}) => runtimeFetch<T>(options, path, body('POST', value))
  const put = <T>(path: string, value: unknown = {}) => runtimeFetch<T>(options, path, body('PUT', value))
  const del = <T>(path: string) => runtimeFetch<T>(options, path, { method: 'DELETE' })

  return {
    async addSkillPath(path: string) {
      const response = await post<{ skills: RuntimeSkill[] }>('/v1/skills/paths', { path })
      return response.skills
    },
    async auditSession(sessionId: string) {
      const response = await get<{ events: RuntimeAuditEvent[] }>(`/v1/audit/sessions/${encodePath(sessionId)}`)
      return response.events
    },
    async auditTurn(turnId: string) {
      const response = await get<{ events: RuntimeAuditEvent[] }>(`/v1/audit/turns/${encodePath(turnId)}`)
      return response.events
    },
    async cancel() {
      const status = await get<RuntimeStatus>('/v1/runtime/status')
      const turnId = status.requests?.activeRequestId
      if (!turnId) {
        throw new Error('No active runtime turn to cancel.')
      }
      return this.cancelTurn(turnId)
    },
    cancelTurn(turnId: string) {
      return post<RuntimeStatus>(`/v1/turns/${encodePath(turnId)}/cancel`)
    },
    async chat(request: RuntimeChatRequest) {
      let sessionId = request.sessionId
      if (!sessionId) {
        const status = await get<RuntimeStatus>('/v1/runtime/status')
        sessionId = status.sessionId
      }
      if (!sessionId) {
        return post<RuntimeChatResponse>('/v1/turns', request)
      }
      return post<RuntimeChatResponse>(`/v1/sessions/${encodePath(sessionId)}/turns`, { ...request, sessionId })
    },
    async createSkill(request: RuntimeSkillCreateRequest) {
      const response = await post<{ skills: RuntimeSkill[] }>('/v1/skills', request)
      return response.skills
    },
    discoverModelConfig(config: RuntimeModelConfig) {
      return post('/v1/config/model/discover', config)
    },
    decidePermission(request: RuntimePermissionDecision) {
      return post<RuntimeStatus>(`/v1/permissions/${encodePath(request.permissionId)}/decision`, request)
    },
    async deleteSession(sessionId: string) {
      const response = await del<{ sessions: RuntimeSession[] }>(`/v1/sessions/${encodePath(sessionId)}`)
      return response.sessions
    },
    async getModelConfig() {
      const response = await get<{ config: RuntimeModelConfig }>('/v1/config/model')
      return response.config
    },
    getAPIEndpoint() {
      return Promise.resolve({ url: trimBaseUrl(options.baseUrl), token: options.token })
    },
    getEventsEndpoint(): Promise<RuntimeEventsEndpoint> {
      return Promise.resolve({ url: `${trimBaseUrl(options.baseUrl)}/v1/events`, token: options.token })
    },
    async getTurn(turnId: string) {
      const response = await get<{ turn: RuntimeTurn }>(`/v1/turns/${encodePath(turnId)}`)
      return response.turn
    },
    async getToolCall(toolCallId: string) {
      const response = await get<{ toolCall: RuntimeToolCall }>(`/v1/tool-calls/${encodePath(toolCallId)}`)
      return response.toolCall
    },
    async listCapabilities() {
      const response = await get<{ capabilities: RuntimeCapability[] }>('/v1/capabilities')
      return response.capabilities
    },
    listEvents(after?: number) {
      const query = after && after > 0 ? `?after=${encodeURIComponent(String(after))}` : ''
      return get<RuntimeEventsResponse>(`/v1/events${query}`)
    },
    async listMcpServers() {
      const response = await get<{ servers: RuntimeMcpServer[] }>('/v1/mcp/servers')
      return response.servers
    },
    async listMcpResources(server: string) {
      const response = await get<{ resources: RuntimeMcpResource[] }>(`/v1/mcp/servers/${encodePath(server)}/resources`)
      return response.resources
    },
    async listMcpPrompts(server: string) {
      const response = await get<{ prompts: RuntimeMcpPrompt[] }>(`/v1/mcp/servers/${encodePath(server)}/prompts`)
      return response.prompts
    },
    async listMcpTools(server: string) {
      const response = await get<{ tools: RuntimeMcpTool[] }>(`/v1/mcp/servers/${encodePath(server)}/tools`)
      return response.tools
    },
    async listModels() {
      const response = await get<{ models: RuntimeModel[] }>('/v1/config/models')
      return response.models
    },
    async listMessages() {
      const status = await get<RuntimeStatus>('/v1/runtime/status')
      if (!status.sessionId) {
        return []
      }
      return this.listSessionMessages(status.sessionId)
    },
    async listSessionMessages(sessionId: string) {
      const response = await get<{ messages: RuntimeMessage[] }>(`/v1/sessions/${encodePath(sessionId)}/messages`)
      return response.messages
    },
    async listPermissions() {
      const response = await get<{ permissions: RuntimePermissionRequest[] }>('/v1/permissions')
      return response.permissions
    },
    async listTurnToolCalls(turnId: string) {
      const response = await get<{ toolCalls: RuntimeToolCall[] }>(`/v1/turns/${encodePath(turnId)}/tool-calls`)
      return response.toolCalls
    },
    async listSessions() {
      const response = await get<{ sessions: RuntimeSession[] }>('/v1/sessions')
      return response.sessions
    },
    async listSkills() {
      const response = await get<{ skills: RuntimeSkill[] }>('/v1/skills')
      return response.skills
    },
    async listTurns(status?: string) {
      const query = status ? `?status=${encodeURIComponent(status)}` : ''
      const response = await get<{ turns: RuntimeTurn[] }>(`/v1/turns${query}`)
      return response.turns
    },
    newChat(title: string) {
      return post<RuntimeStatus>('/v1/sessions', { title })
    },
    async refreshMcpServer(server: string) {
      const response = await post<{ servers: RuntimeMcpServer[] }>(`/v1/mcp/servers/${encodePath(server)}/refresh`)
      return response.servers
    },
    async refreshSkills() {
      const response = await post<{ skills: RuntimeSkill[] }>('/v1/skills/refresh')
      return response.skills
    },
    async renameSession(sessionId: string, title: string) {
      const response = await put<{ sessions: RuntimeSession[] }>(`/v1/sessions/${encodePath(sessionId)}`, { title })
      return response.sessions
    },
    async saveModelConfig(config: RuntimeModelConfig) {
      const response = await put<{ config: RuntimeModelConfig }>('/v1/config/model', config)
      return response.config
    },
    async saveMcpServer(config: RuntimeMcpServerConfig) {
      const response = await put<{ servers: RuntimeMcpServer[] }>(`/v1/mcp/servers/${encodePath(config.name)}`, config)
      return response.servers
    },
    selectSession(sessionId: string) {
      return post<RuntimeStatus>(`/v1/sessions/${encodePath(sessionId)}/select`)
    },
    async setMcpServerEnabled(server: string, enabled: boolean) {
      const response = await post<{ servers: RuntimeMcpServer[] }>(`/v1/mcp/servers/${encodePath(server)}/enabled`, { enabled })
      return response.servers
    },
    async setMcpToolEnabled(server: string, tool: string, enabled: boolean) {
      const response = await post<{ tools: RuntimeMcpTool[] }>(`/v1/mcp/servers/${encodePath(server)}/tools/${encodePath(tool)}/enabled`, { enabled })
      return response.tools
    },
    async setSkillEnabled(name: string, enabled: boolean) {
      const response = await post<{ skills: RuntimeSkill[] }>(`/v1/skills/${encodePath(name)}/enabled`, { enabled })
      return response.skills
    },
    status() {
      return get<RuntimeStatus>('/v1/runtime/status')
    },
    verifyModelConfig(config: RuntimeModelConfig) {
      return post<RuntimeModelVerifyResponse>('/v1/config/model/verify', config)
    },
  }
}
