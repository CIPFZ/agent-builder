import type {
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeCapability,
  RuntimeCompactBoundary,
  RuntimeContextSource,
  RuntimeReadFileState,
  RuntimeAgentTask,
  RuntimeAgentTaskMessageCreateRequest,
  RuntimeAgentRoleDefinition,
  RuntimeAgentTaskMessage,
  RuntimeAgentTaskResult,
  RuntimeAuditEvent,
  RuntimeMcpServerConfig,
  RuntimeMcpRequest,
  RuntimeMcpRequestDecision,
  RuntimeMcpRequestListRequest,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpTool,
  RuntimeMcpServer,
  RuntimeMessage,
  RuntimeModel,
  RuntimeModelConfig,
  RuntimeModelDiscoveryResponse,
  RuntimeModelVerifyResponse,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimePolicy,
  RuntimePolicyMode,
  RuntimeRef,
  RuntimeRefContentResponse,
  RuntimeReplayExportRequest,
  RuntimeReplayExportResponse,
  RuntimeRecoveryStatus,
  RuntimeSession,
  RuntimeSkillCreateRequest,
  RuntimeSkill,
  RuntimeStatus,
  RuntimeTodoSummary,
  RuntimeToolCall,
  RuntimeToolSearchRequest,
  RuntimeToolSearchResponse,
  RuntimeTurn,
  RuntimeAPIEndpoint,
  RuntimeEventsResponse,
  RuntimeEffectiveScope,
  RuntimeHook,
  RuntimeHookExecution,
  RuntimeHookExecutionListRequest,
  RuntimeWorktree,
  RuntimeWorktreeActionRequest,
  RuntimeWorktreeCreateRequest,
} from './types'

type WailsRuntimeBridge = {
  AddSkillPath: (request: { path: string }) => Promise<{ skills: RuntimeSkill[] }>
  APIEndpoint: () => Promise<RuntimeAPIEndpoint>
  AuditSession: (sessionId: string) => Promise<{ events: RuntimeAuditEvent[] }>
  AuditTurn: (turnId: string) => Promise<{ events: RuntimeAuditEvent[] }>
  ReplayExport: (request: RuntimeReplayExportRequest) => Promise<RuntimeReplayExportResponse>
  Cancel: () => Promise<RuntimeStatus>
  CancelTurn: (turnId: string) => Promise<RuntimeStatus>
  Capabilities: () => Promise<{ capabilities: RuntimeCapability[] }>
  ContextSources: () => Promise<{ sources: RuntimeContextSource[] }>
  ReadFiles: (sessionId: string) => Promise<{ files: RuntimeReadFileState[] }>
  RefreshCapability: (capabilityId: string) => Promise<{ capability: RuntimeCapability }>
  SearchTools: (request: RuntimeToolSearchRequest) => Promise<RuntimeToolSearchResponse>
  Chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  CreateSkill: (request: RuntimeSkillCreateRequest) => Promise<{ skills: RuntimeSkill[] }>
  DecidePermission: (request: RuntimePermissionDecision) => Promise<RuntimeStatus>
  DiscoverModelConfig: (request: RuntimeModelConfig) => Promise<RuntimeModelDiscoveryResponse>
  Events: () => Promise<RuntimeEventsResponse>
  EventsEndpoint: () => Promise<{ url: string }>
  GetModelConfig: () => Promise<{ config: RuntimeModelConfig }>
  GetPolicy: () => Promise<{ policy: RuntimePolicy }>
  Hooks: () => Promise<{ hooks: RuntimeHook[] }>
  HookExecutions: (request: RuntimeHookExecutionListRequest) => Promise<{ executions: RuntimeHookExecution[] }>
  HookExecution: (executionId: string) => Promise<{ execution: RuntimeHookExecution }>
  RecoveryStatus: () => Promise<RuntimeRecoveryStatus>
  MCPServers: () => Promise<{ servers: RuntimeMcpServer[] }>
  MCPRequests: (request: RuntimeMcpRequestListRequest) => Promise<{ requests: RuntimeMcpRequest[] }>
  MCPRequest: (requestId: string) => Promise<{ request: RuntimeMcpRequest }>
  DecideMCPRequest: (request: RuntimeMcpRequestDecision) => Promise<{ request: RuntimeMcpRequest }>
  RetryMCPServer: (server: string) => Promise<{ servers: RuntimeMcpServer[] }>
  MCPResources: (server: string) => Promise<{ resources: RuntimeMcpResource[] }>
  MCPPrompts: (server: string) => Promise<{ prompts: RuntimeMcpPrompt[] }>
  MCPTools: (server: string) => Promise<{ tools: RuntimeMcpTool[] }>
  Messages: () => Promise<{ messages: RuntimeMessage[] }>
  Models: () => Promise<{ models: RuntimeModel[] }>
  NewChat: (title: string) => Promise<RuntimeStatus>
  DeleteSession: (sessionId: string) => Promise<{ sessions: RuntimeSession[] }>
  Permissions: () => Promise<{ permissions: RuntimePermissionRequest[] }>
  RefreshMCPServer: (server: string) => Promise<{ servers: RuntimeMcpServer[] }>
  RefreshSkills: () => Promise<{ skills: RuntimeSkill[] }>
  SaveModelConfig: (request: RuntimeModelConfig) => Promise<{ config: RuntimeModelConfig }>
  UpdatePolicy: (request: { mode: RuntimePolicyMode; rules?: RuntimePolicy['rules']; profile?: string }) => Promise<{ policy: RuntimePolicy }>
  SaveMCPServer: (request: RuntimeMcpServerConfig) => Promise<{ servers: RuntimeMcpServer[] }>
  RenameSession: (request: { sessionId: string; title: string }) => Promise<{ sessions: RuntimeSession[] }>
  SelectSession: (sessionId: string) => Promise<RuntimeStatus>
  SessionMessages: (sessionId: string) => Promise<{ messages: RuntimeMessage[] }>
  SessionCompactBoundaries: (sessionId: string) => Promise<{ boundaries: RuntimeCompactBoundary[] }>
  Worktrees: () => Promise<{ worktrees: RuntimeWorktree[] }>
  Worktree: (worktreeId: string) => Promise<{ worktree: RuntimeWorktree }>
  CreateWorktree: (request: RuntimeWorktreeCreateRequest) => Promise<{ worktree: RuntimeWorktree }>
  EnterWorktree: (worktreeId: string, request: RuntimeWorktreeActionRequest) => Promise<{ worktree: RuntimeWorktree }>
  ExitWorktree: (worktreeId: string, request: RuntimeWorktreeActionRequest) => Promise<{ worktree: RuntimeWorktree }>
  CleanupWorktree: (worktreeId: string, request: RuntimeWorktreeActionRequest) => Promise<{ worktree: RuntimeWorktree }>
  SessionTodos: (sessionId: string) => Promise<{ summary: RuntimeTodoSummary }>
  Sessions: () => Promise<{ sessions: RuntimeSession[] }>
  SetMCPServerEnabled: (request: { name: string; enabled: boolean }) => Promise<{ servers: RuntimeMcpServer[] }>
  SetMCPToolEnabled: (request: { server: string; tool: string; enabled: boolean }) => Promise<{ tools: RuntimeMcpTool[] }>
  SetSkillEnabled: (request: { name: string; enabled: boolean }) => Promise<{ skills: RuntimeSkill[] }>
  Skills: () => Promise<{ skills: RuntimeSkill[] }>
  Status: () => Promise<RuntimeStatus>
  ToolCall: (toolCallId: string) => Promise<{ toolCall: RuntimeToolCall }>
  Ref: (refId: string) => Promise<{ ref: RuntimeRef }>
  Refs: (request: { sessionId?: string; turnId?: string; toolCallId?: string; taskId?: string; kind?: string }) => Promise<{ refs: RuntimeRef[] }>
  ReadRefContent: (refId: string) => Promise<RuntimeRefContentResponse>
  AgentTask: (taskId: string) => Promise<{ task: RuntimeAgentTask }>
  TaskEffectiveScope: (taskId: string) => Promise<{ scope: RuntimeEffectiveScope }>
  CancelAgentTask: (taskId: string) => Promise<{ task: RuntimeAgentTask }>
  AgentRoles: () => Promise<{ roles: RuntimeAgentRoleDefinition[] }>
  AgentRole: (roleId: string) => Promise<{ role: RuntimeAgentRoleDefinition }>
  AgentTaskMessages: (taskId: string) => Promise<{ messages: RuntimeAgentTaskMessage[] }>
  AgentTaskFollowUp: (taskId: string, request: RuntimeAgentTaskMessageCreateRequest) => Promise<{ message: RuntimeAgentTaskMessage }>
  AgentTaskResult: (taskId: string) => Promise<{ result: RuntimeAgentTaskResult }>
  Turn: (turnId: string) => Promise<{ turn: RuntimeTurn }>
  TurnCompactBoundaries: (turnId: string) => Promise<{ boundaries: RuntimeCompactBoundary[] }>
  TurnTodos: (turnId: string) => Promise<{ summary: RuntimeTodoSummary }>
  Turns: (status: string) => Promise<{ turns: RuntimeTurn[] }>
  TurnToolCalls: (turnId: string) => Promise<{ toolCalls: RuntimeToolCall[] }>
  TurnAgentTasks: (turnId: string) => Promise<{ tasks: RuntimeAgentTask[] }>
  VerifyModelConfig: (request: RuntimeModelConfig) => Promise<RuntimeModelVerifyResponse>
}

let bridgePromise: Promise<WailsRuntimeBridge> | undefined

function createMockRuntimeBridge(): WailsRuntimeBridge {
  const emptyStatus: RuntimeStatus = {
    ready: false,
    workspaceId: '',
    sessionId: '',
    workingDir: '',
    model: '',
    provider: '',
    busy: false,
    usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0, cost: 0 },
    events: {
      lastEventAt: 0,
      messageEvents: 0,
      sessionEvents: 0,
      otherEvents: 0,
      assistantEvents: 0,
      permissionEvents: 0,
    },
    requests: {
      running: 0,
    },
  }
  const bridge: any = {
    AddSkillPath: () => Promise.resolve({ skills: [] }),
    APIEndpoint: () => Promise.resolve({ url: '', token: '' }),
    AuditSession: () => Promise.resolve({ events: [] }),
    AuditTurn: () => Promise.resolve({ events: [] }),
    ReplayExport: async () => ({
      generatedAt: new Date().toISOString(),
      source: 'mock',
      events: [],
      audit: [],
      summary: { redacted: true },
    }),
    Cancel: () => Promise.resolve(emptyStatus),
    CancelTurn: () => Promise.resolve(emptyStatus),
    Capabilities: () => Promise.resolve({ capabilities: [] }),
    ContextSources: () => Promise.resolve({ sources: [] }),
    ReadFiles: () => Promise.resolve({ files: [] }),
    RefreshCapability: async (capabilityId: string) => ({ capability: { id: capabilityId, kind: 'mock', name: capabilityId, enabled: false, risk: 'unknown', state: 'unavailable' } }),
    SearchTools: async (request: RuntimeToolSearchRequest) => ({ query: request.query, results: [], total: 0, budgetImpact: { selected: { count: 0, estimatedTokens: 0 }, omitted: { count: 0, estimatedTokens: 0 } } }),
    Chat: async () => ({ requestId: '', status: emptyStatus }),
    CreateSkill: () => Promise.resolve({ skills: [] }),
    DecidePermission: () => Promise.resolve(emptyStatus),
    DiscoverModelConfig: async (request: RuntimeModelConfig) => ({ protocol: request.protocol, models: request.models ?? [], model: request.model, error: 'runtime unavailable' }),
    Events: () => Promise.resolve({ events: [], snapshot_required: false, first_sequence: 0, last_sequence: 0 }),
    EventsEndpoint: () => Promise.resolve({ url: '', token: '' }),
    GetModelConfig: async () => ({ config: { protocol: 'openai', url: '', model: '', models: [], hasApiKey: false, configPath: '' } }),
    GetPolicy: async () => ({ policy: { mode: 'ask', modes: ['ask', 'auto_read', 'plan', 'deny_all'] } }),
    Hooks: () => Promise.resolve({ hooks: [] }),
    HookExecutions: () => Promise.resolve({ executions: [] }),
    HookExecution: async (executionId: string) => ({ execution: { id: executionId, hookId: executionId, event: '', status: 'unavailable', redacted: true, startedAt: Date.now() } }),
    RecoveryStatus: async () => ({
      runtime_started_at: new Date().toISOString(),
      last_event_sequence: 0,
      active_turns: [],
      interrupted_turns: [],
      pending_permissions: [],
      pending_mcp_requests: [],
      snapshot_required: false,
    }),
    MCPServers: () => Promise.resolve({ servers: [] }),
    MCPRequests: () => Promise.resolve({ requests: [] }),
    MCPRequest: async (requestId: string) => ({ request: { id: requestId, kind: 'auth', server: '', status: 'none', createdAt: Date.now(), updatedAt: Date.now(), redacted: true } }),
    DecideMCPRequest: async (request: RuntimeMcpRequestDecision) => ({ request: { id: request.requestId, kind: 'auth', server: '', status: 'denied', createdAt: Date.now(), updatedAt: Date.now(), redacted: true, policyDecision: request.action } }),
    RetryMCPServer: () => Promise.resolve({ servers: [] }),
    MCPResources: () => Promise.resolve({ resources: [] }),
    MCPPrompts: () => Promise.resolve({ prompts: [] }),
    MCPTools: () => Promise.resolve({ tools: [] }),
    Messages: () => Promise.resolve({ messages: [] }),
    Models: () => Promise.resolve({ models: [] }),
    NewChat: async () => emptyStatus,
    DeleteSession: () => Promise.resolve({ sessions: [] }),
    Permissions: () => Promise.resolve({ permissions: [] }),
    RefreshMCPServer: () => Promise.resolve({ servers: [] }),
    RefreshSkills: () => Promise.resolve({ skills: [] }),
    SaveModelConfig: async (request: RuntimeModelConfig) => ({ config: request }),
    UpdatePolicy: async (request: { mode: RuntimePolicyMode; rules?: RuntimePolicy['rules']; profile?: string }) => ({ policy: { mode: request.mode, modes: ['ask', 'auto_read', 'plan', 'deny_all'], profile: request.profile } }),
    SaveMCPServer: () => Promise.resolve({ servers: [] }),
    RenameSession: () => Promise.resolve({ sessions: [] }),
    SelectSession: async () => emptyStatus,
    SessionMessages: () => Promise.resolve({ messages: [] }),
    SessionCompactBoundaries: () => Promise.resolve({ boundaries: [] }),
    Worktrees: () => Promise.resolve({ worktrees: [] }),
    Worktree: async (worktreeId: string) => ({ worktree: { id: worktreeId, sessionId: '', baseRepoPath: '', worktreePath: '', branch: '', status: 'unavailable', preservePolicy: '', cleanupPolicy: '', createdAt: Date.now(), updatedAt: Date.now() } }),
    CreateWorktree: async (request: RuntimeWorktreeCreateRequest) => ({
      worktree: {
        id: request.name || 'mock-worktree',
        sessionId: request.sessionId || '',
        baseRepoPath: request.baseRepoPath || '',
        worktreePath: '',
        branch: request.branch || '',
        status: 'unavailable',
        preservePolicy: request.preservePolicy || '',
        cleanupPolicy: request.cleanupPolicy || '',
        createdAt: Date.now(),
        updatedAt: Date.now(),
      },
    }),
    EnterWorktree: async (worktreeId: string) => ({ worktree: { id: worktreeId, sessionId: '', baseRepoPath: '', worktreePath: '', branch: '', status: 'unavailable', preservePolicy: '', cleanupPolicy: '', createdAt: Date.now(), updatedAt: Date.now() } }),
    ExitWorktree: async (worktreeId: string) => ({ worktree: { id: worktreeId, sessionId: '', baseRepoPath: '', worktreePath: '', branch: '', status: 'unavailable', preservePolicy: '', cleanupPolicy: '', createdAt: Date.now(), updatedAt: Date.now() } }),
    CleanupWorktree: async (worktreeId: string) => ({ worktree: { id: worktreeId, sessionId: '', baseRepoPath: '', worktreePath: '', branch: '', status: 'unavailable', preservePolicy: '', cleanupPolicy: '', createdAt: Date.now(), updatedAt: Date.now() } }),
    SessionTodos: async (sessionId: string) => ({ summary: { sessionId, todos: [], pending: 0, inProgress: 0, completed: 0, total: 0, updatedAt: Date.now() } }),
    Sessions: () => Promise.resolve({ sessions: [] }),
    SetMCPServerEnabled: () => Promise.resolve({ servers: [] }),
    SetMCPToolEnabled: () => Promise.resolve({ tools: [] }),
    SetSkillEnabled: () => Promise.resolve({ skills: [] }),
    Skills: () => Promise.resolve({ skills: [] }),
    Status: () => Promise.resolve(emptyStatus),
    ToolCall: async (toolCallId: string) => ({ toolCall: { id: toolCallId, sessionId: '', turnId: '', name: '', source: 'unknown', status: 'unavailable', startedAt: Date.now() } }),
    Ref: async (refId: string) => ({ ref: { id: refId, uri: refId, sessionId: '', kind: 'unknown', sizeBytes: 0, estimatedTokens: 0, storageKind: 'mock', redactionStatus: 'unavailable', createdAt: Date.now(), canReadContent: false } }),
    Refs: () => Promise.resolve({ refs: [] }),
    ReadRefContent: async (refId: string) => ({
      ref: { id: refId, uri: refId, sessionId: '', kind: 'unknown', sizeBytes: 0, estimatedTokens: 0, storageKind: 'mock', redactionStatus: 'unavailable', createdAt: Date.now(), canReadContent: false },
      content: '',
      redacted: true,
    }),
    AgentTask: async (taskId: string) => ({ task: { id: taskId, parentSessionId: '', title: '', kind: 'background', status: 'unavailable', progress: 0, startedAt: Date.now(), updatedAt: Date.now() } }),
    TaskEffectiveScope: async () => ({ scope: { baseCwd: '', effectiveCwd: '' } }),
    CancelAgentTask: async (taskId: string) => ({ task: { id: taskId, parentSessionId: '', title: '', kind: 'background', status: 'cancelled', progress: 0, startedAt: Date.now(), updatedAt: Date.now() } }),
    AgentRoles: () => Promise.resolve({ roles: [] }),
    AgentRole: async (roleId: string) => ({ role: { id: roleId, name: roleId } }),
    AgentTaskMessages: () => Promise.resolve({ messages: [] }),
    AgentTaskFollowUp: async (taskId: string) => ({ message: { id: taskId, taskId, direction: 'parent_to_child', kind: 'progress', status: 'unavailable', createdAt: Date.now(), redacted: true } }),
    AgentTaskResult: async (taskId: string) => ({ result: { taskId, status: 'unavailable', createdAt: Date.now(), updatedAt: Date.now() } }),
    Turn: async (turnId: string) => ({ turn: { id: turnId, sessionId: '', status: 'unavailable', startedAt: Date.now() } }),
    TurnCompactBoundaries: () => Promise.resolve({ boundaries: [] }),
    TurnTodos: async (turnId: string) => ({ summary: { sessionId: '', turnId, todos: [], pending: 0, inProgress: 0, completed: 0, total: 0, updatedAt: Date.now() } }),
    Turns: () => Promise.resolve({ turns: [] }),
    TurnToolCalls: () => Promise.resolve({ toolCalls: [] }),
    TurnAgentTasks: () => Promise.resolve({ tasks: [] }),
    VerifyModelConfig: async (request: RuntimeModelConfig) => ({ ok: false, protocol: request.protocol, model: request.model, models: request.models, error: 'runtime unavailable' }),
  }

  return bridge as WailsRuntimeBridge
}

export async function loadWailsRuntimeBridge(): Promise<WailsRuntimeBridge> {
  if (typeof window === 'undefined' || !('go' in window)) {
    bridgePromise ??= Promise.resolve(createMockRuntimeBridge())
    return bridgePromise
  }

  const importBinding = new Function('path', 'return import(path)') as (
    path: string,
  ) => Promise<{ RuntimeBridge?: WailsRuntimeBridge }>
  bridgePromise ??= importBinding('/bindings/github.com/charmbracelet/crush/desktop/index.js')
    .then((module) => {
      const runtimeBridge = (module as { RuntimeBridge?: WailsRuntimeBridge }).RuntimeBridge
      if (!runtimeBridge) {
        throw new Error('Wails RuntimeBridge binding is not available.')
      }
      return runtimeBridge
    })
    .catch(() => createMockRuntimeBridge())

  return bridgePromise
}
