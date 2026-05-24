import type {
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeCapability,
  RuntimeCompactBoundary,
  RuntimeContextSource,
  RuntimeAgentTask,
  RuntimeAuditEvent,
  RuntimeMcpServerConfig,
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
} from './types'

type WailsRuntimeBridge = {
  AddSkillPath: (request: { path: string }) => Promise<{ skills: RuntimeSkill[] }>
  APIEndpoint: () => Promise<RuntimeAPIEndpoint>
  AuditSession: (sessionId: string) => Promise<{ events: RuntimeAuditEvent[] }>
  AuditTurn: (turnId: string) => Promise<{ events: RuntimeAuditEvent[] }>
  Cancel: () => Promise<RuntimeStatus>
  CancelTurn: (turnId: string) => Promise<RuntimeStatus>
  Capabilities: () => Promise<{ capabilities: RuntimeCapability[] }>
  ContextSources: () => Promise<{ sources: RuntimeContextSource[] }>
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
  RecoveryStatus: () => Promise<RuntimeRecoveryStatus>
  MCPServers: () => Promise<{ servers: RuntimeMcpServer[] }>
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
  SessionTodos: (sessionId: string) => Promise<{ summary: RuntimeTodoSummary }>
  Sessions: () => Promise<{ sessions: RuntimeSession[] }>
  SetMCPServerEnabled: (request: { name: string; enabled: boolean }) => Promise<{ servers: RuntimeMcpServer[] }>
  SetMCPToolEnabled: (request: { server: string; tool: string; enabled: boolean }) => Promise<{ tools: RuntimeMcpTool[] }>
  SetSkillEnabled: (request: { name: string; enabled: boolean }) => Promise<{ skills: RuntimeSkill[] }>
  Skills: () => Promise<{ skills: RuntimeSkill[] }>
  Status: () => Promise<RuntimeStatus>
  ToolCall: (toolCallId: string) => Promise<{ toolCall: RuntimeToolCall }>
  AgentTask: (taskId: string) => Promise<{ task: RuntimeAgentTask }>
  CancelAgentTask: (taskId: string) => Promise<{ task: RuntimeAgentTask }>
  Turn: (turnId: string) => Promise<{ turn: RuntimeTurn }>
  TurnCompactBoundaries: (turnId: string) => Promise<{ boundaries: RuntimeCompactBoundary[] }>
  TurnTodos: (turnId: string) => Promise<{ summary: RuntimeTodoSummary }>
  Turns: (status: string) => Promise<{ turns: RuntimeTurn[] }>
  TurnToolCalls: (turnId: string) => Promise<{ toolCalls: RuntimeToolCall[] }>
  TurnAgentTasks: (turnId: string) => Promise<{ tasks: RuntimeAgentTask[] }>
  VerifyModelConfig: (request: RuntimeModelConfig) => Promise<RuntimeModelVerifyResponse>
}

let bridgePromise: Promise<WailsRuntimeBridge> | undefined

export async function loadWailsRuntimeBridge(): Promise<WailsRuntimeBridge> {
  const importBinding = new Function('path', 'return import(path)') as (
    path: string,
  ) => Promise<{ RuntimeBridge?: WailsRuntimeBridge }>

  bridgePromise ??= importBinding('/bindings/github.com/charmbracelet/crush/desktop/index.js').then((module) => {
    const runtimeBridge = (module as { RuntimeBridge?: WailsRuntimeBridge }).RuntimeBridge
    if (!runtimeBridge) {
      throw new Error('Wails RuntimeBridge binding is not available.')
    }
    return runtimeBridge
  })

  return bridgePromise
}
