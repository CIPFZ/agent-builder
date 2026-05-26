import type {
  AgentRuntime,
  RuntimeAuditEvent,
  RuntimeAgentRoleDefinition,
  RuntimeAgentTaskMessage,
  RuntimeAgentTaskResult,
  RuntimeAgentTask,
  RuntimeCapability,
  RuntimeContextSource,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeCompactBoundary,
  RuntimeEventsResponse,
  RuntimeEventsEndpoint,
  RuntimeHook,
  RuntimeHookExecution,
  RuntimeHookExecutionListRequest,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpServer,
  RuntimeMcpServerConfig,
  RuntimeMcpTool,
  RuntimeMessage,
  RuntimeModel,
  RuntimeModelConfig,
  RuntimeModelVerifyResponse,
  RuntimeMcpRequest,
  RuntimeMcpRequestDecision,
  RuntimeMcpRequestListRequest,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimePolicy,
  RuntimePolicyMode,
  RuntimeRef,
  RuntimeRefContentResponse,
  RuntimeReadFileState,
  RuntimeReplayExportRequest,
  RuntimeReplayExportResponse,
  RuntimeRecoveryStatus,
  RuntimeSession,
  RuntimeSkill,
  RuntimeSkillCreateRequest,
  RuntimeStatus,
  RuntimeTodoSummary,
  RuntimeToolCall,
  RuntimeToolSearchRequest,
  RuntimeToolSearchResponse,
  RuntimeTurn,
  RuntimeEffectiveScope,
  RuntimeWorktree,
  RuntimeWorktreeActionRequest,
  RuntimeWorktreeCreateRequest,
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
    exportReplay(request: RuntimeReplayExportRequest) {
      const params = new URLSearchParams()
      if (request.sessionId) params.set('session_id', request.sessionId)
      if (request.turnId) params.set('turn_id', request.turnId)
      if (request.after && request.after > 0) params.set('after', String(request.after))
      const query = params.toString()
      return get<RuntimeReplayExportResponse>(`/v1/replay/export${query ? `?${query}` : ''}`)
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
    async getPolicy() {
      const response = await get<{ policy: RuntimePolicy }>('/v1/policy')
      return response.policy
    },
    getRecoveryStatus() {
      return get<RuntimeRecoveryStatus>('/v1/recovery/status')
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
    async listTurnCompactBoundaries(turnId: string) {
      const response = await get<{ boundaries: RuntimeCompactBoundary[] }>(`/v1/turns/${encodePath(turnId)}/compact`)
      return response.boundaries
    },
    async listSessionCompactBoundaries(sessionId: string) {
      const response = await get<{ boundaries: RuntimeCompactBoundary[] }>(`/v1/sessions/${encodePath(sessionId)}/compact`)
      return response.boundaries
    },
    async listWorktrees() {
      const response = await get<{ worktrees: RuntimeWorktree[] }>('/v1/worktrees')
      return response.worktrees
    },
    async getWorktree(worktreeId: string) {
      const response = await get<{ worktree: RuntimeWorktree }>(`/v1/worktrees/${encodePath(worktreeId)}`)
      return response.worktree
    },
    async createWorktree(request: RuntimeWorktreeCreateRequest) {
      const response = await post<{ worktree: RuntimeWorktree }>('/v1/worktrees', request)
      return response.worktree
    },
    async enterWorktree(worktreeId: string, request: RuntimeWorktreeActionRequest = {}) {
      const response = await post<{ worktree: RuntimeWorktree }>(`/v1/worktrees/${encodePath(worktreeId)}/enter`, request)
      return response.worktree
    },
    async exitWorktree(worktreeId: string, request: RuntimeWorktreeActionRequest = {}) {
      const response = await post<{ worktree: RuntimeWorktree }>(`/v1/worktrees/${encodePath(worktreeId)}/exit`, request)
      return response.worktree
    },
    async cleanupWorktree(worktreeId: string, request: RuntimeWorktreeActionRequest = {}) {
      const response = await post<{ worktree: RuntimeWorktree }>(`/v1/worktrees/${encodePath(worktreeId)}/cleanup`, request)
      return response.worktree
    },
    async getToolCall(toolCallId: string) {
      const response = await get<{ toolCall: RuntimeToolCall }>(`/v1/tool-calls/${encodePath(toolCallId)}`)
      return response.toolCall
    },
    async getRef(refId: string) {
      const response = await get<{ ref: RuntimeRef }>(`/v1/refs/${encodePath(refId)}`)
      return response.ref
    },
    readRefContent(refId: string) {
      return get<RuntimeRefContentResponse>(`/v1/refs/${encodePath(refId)}/content`)
    },
    async getAgentTask(taskId: string) {
      const response = await get<{ task: RuntimeAgentTask }>(`/v1/tasks/${encodePath(taskId)}`)
      return response.task
    },
    async getAgentTaskEffectiveScope(taskId: string) {
      const response = await get<{ scope: RuntimeEffectiveScope }>(`/v1/tasks/${encodePath(taskId)}/effective-scope`)
      return response.scope
    },
    async cancelAgentTask(taskId: string) {
      const response = await post<{ task: RuntimeAgentTask }>(`/v1/tasks/${encodePath(taskId)}/cancel`)
      return response.task
    },
    async listAgentRoles() {
      const response = await get<{ roles: RuntimeAgentRoleDefinition[] }>('/v1/agent-roles')
      return response.roles
    },
    async getAgentRole(roleId: string) {
      const response = await get<{ role: RuntimeAgentRoleDefinition }>(`/v1/agent-roles/${encodePath(roleId)}`)
      return response.role
    },
    async listAgentTaskMessages(taskId: string) {
      const response = await get<{ messages: RuntimeAgentTaskMessage[] }>(`/v1/tasks/${encodePath(taskId)}/messages`)
      return response.messages
    },
    async getAgentTaskResult(taskId: string) {
      const response = await get<{ result: RuntimeAgentTaskResult }>(`/v1/tasks/${encodePath(taskId)}/result`)
      return response.result
    },
    async getSessionTodos(sessionId: string) {
      const response = await get<{ summary: RuntimeTodoSummary }>(`/v1/sessions/${encodePath(sessionId)}/todos`)
      return response.summary
    },
    async getTurnTodos(turnId: string) {
      const response = await get<{ summary: RuntimeTodoSummary }>(`/v1/turns/${encodePath(turnId)}/todos`)
      return response.summary
    },
    async listCapabilities() {
      const response = await get<{ capabilities: RuntimeCapability[] }>('/v1/capabilities')
      return response.capabilities
    },
    searchTools(request: RuntimeToolSearchRequest) {
      return post<RuntimeToolSearchResponse>('/v1/tools/search', request)
    },
    async refreshCapability(capabilityId: string) {
      const response = await post<{ capability: RuntimeCapability }>(`/v1/capabilities/${encodePath(capabilityId)}/refresh`)
      return response.capability
    },
    async listContextSources() {
      const response = await get<{ sources: RuntimeContextSource[] }>('/v1/context/sources')
      return response.sources
    },
    async listReadFiles(sessionId?: string) {
      const query = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : ''
      const response = await get<{ files: RuntimeReadFileState[] }>(`/v1/read-files${query}`)
      return response.files
    },
    listEvents(after?: number) {
      const query = after && after > 0 ? `?after=${encodeURIComponent(String(after))}` : ''
      return get<RuntimeEventsResponse>(`/v1/events${query}`)
    },
    async listMcpServers() {
      const response = await get<{ servers: RuntimeMcpServer[] }>('/v1/mcp/servers')
      return response.servers
    },
    async listMcpRequests(request: RuntimeMcpRequestListRequest = {}) {
      const params = new URLSearchParams()
      if (request.kind) params.set('kind', request.kind)
      if (request.status) params.set('status', request.status)
      if (request.server) params.set('server', request.server)
      const query = params.toString()
      const response = await get<{ requests: RuntimeMcpRequest[] }>(`/v1/mcp/requests${query ? `?${query}` : ''}`)
      return response.requests
    },
    async getMcpRequest(requestId: string) {
      const response = await get<{ request: RuntimeMcpRequest }>(`/v1/mcp/requests/${encodePath(requestId)}`)
      return response.request
    },
    async decideMcpRequest(request: RuntimeMcpRequestDecision) {
      const response = await post<{ request: RuntimeMcpRequest }>(`/v1/mcp/requests/${encodePath(request.requestId)}/decision`, request)
      return response.request
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
    async listHooks() {
      const response = await get<{ hooks: RuntimeHook[] }>('/v1/hooks')
      return response.hooks
    },
    async listHookExecutions(request: RuntimeHookExecutionListRequest = {}) {
      const params = new URLSearchParams()
      if (request.sessionId) params.set('session_id', request.sessionId)
      if (request.turnId) params.set('turn_id', request.turnId)
      if (request.toolCallId) params.set('tool_call_id', request.toolCallId)
      if (request.taskId) params.set('task_id', request.taskId)
      if (request.event) params.set('event', request.event)
      if (request.status) params.set('status', request.status)
      const query = params.toString()
      const response = await get<{ executions: RuntimeHookExecution[] }>(`/v1/hook-executions${query ? `?${query}` : ''}`)
      return response.executions
    },
    async getHookExecution(executionId: string) {
      const response = await get<{ execution: RuntimeHookExecution }>(`/v1/hook-executions/${encodePath(executionId)}`)
      return response.execution
    },
    async listRefs(request = {}) {
      const params = new URLSearchParams()
      const req = request as {
        sessionId?: string
        turnId?: string
        toolCallId?: string
        taskId?: string
        kind?: string
      }
      if (req.sessionId) params.set('session_id', req.sessionId)
      if (req.turnId) params.set('turn_id', req.turnId)
      if (req.toolCallId) params.set('tool_call_id', req.toolCallId)
      if (req.taskId) params.set('task_id', req.taskId)
      if (req.kind) params.set('kind', req.kind)
      const query = params.toString()
      const response = await get<{ refs: RuntimeRef[] }>(`/v1/refs${query ? `?${query}` : ''}`)
      return response.refs
    },
    async listTurnAgentTasks(turnId: string) {
      const response = await get<{ tasks: RuntimeAgentTask[] }>(`/v1/turns/${encodePath(turnId)}/tasks`)
      return response.tasks
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
    async retryMcpServer(server: string) {
      const response = await post<{ servers: RuntimeMcpServer[] }>(`/v1/mcp/servers/${encodePath(server)}/retry`)
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
    async updatePolicy(mode: RuntimePolicyMode, rules, profile) {
      const response = await put<{ policy: RuntimePolicy }>('/v1/policy', { mode, rules, profile })
      return response.policy
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
