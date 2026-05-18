import type {
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeCapability,
  RuntimeAuditEvent,
  RuntimeEvent,
  RuntimeMcpServer,
  RuntimeMessage,
  RuntimeModel,
  RuntimeModelConfig,
  RuntimeModelVerifyResponse,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeSkill,
  RuntimeStatus,
} from './types'

type WailsRuntimeBridge = {
  AuditTurn: (turnId: string) => Promise<{ events: RuntimeAuditEvent[] }>
  Cancel: () => Promise<RuntimeStatus>
  Capabilities: () => Promise<{ capabilities: RuntimeCapability[] }>
  Chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  DecidePermission: (request: RuntimePermissionDecision) => Promise<RuntimeStatus>
  Events: () => Promise<{ events: RuntimeEvent[] }>
  EventsEndpoint: () => Promise<{ url: string }>
  GetModelConfig: () => Promise<{ config: RuntimeModelConfig }>
  MCPServers: () => Promise<{ servers: RuntimeMcpServer[] }>
  Messages: () => Promise<{ messages: RuntimeMessage[] }>
  Models: () => Promise<{ models: RuntimeModel[] }>
  NewChat: (title: string) => Promise<RuntimeStatus>
  Permissions: () => Promise<{ permissions: RuntimePermissionRequest[] }>
  SaveModelConfig: (request: RuntimeModelConfig) => Promise<{ config: RuntimeModelConfig }>
  Skills: () => Promise<{ skills: RuntimeSkill[] }>
  Status: () => Promise<RuntimeStatus>
  VerifyModelConfig: (request: RuntimeModelConfig) => Promise<RuntimeModelVerifyResponse>
}

let bridgePromise: Promise<WailsRuntimeBridge> | undefined

export async function loadWailsRuntimeBridge(): Promise<WailsRuntimeBridge> {
  const importBinding = new Function('path', 'return import(path)') as (
    path: string,
  ) => Promise<{ RuntimeBridge?: WailsRuntimeBridge }>

  bridgePromise ??= importBinding('/bindings/github.com/charmbracelet/crush/desktop/agent-builder/index.js').then((module) => {
    const runtimeBridge = (module as { RuntimeBridge?: WailsRuntimeBridge }).RuntimeBridge
    if (!runtimeBridge) {
      throw new Error('Wails RuntimeBridge binding is not available.')
    }
    return runtimeBridge
  })

  return bridgePromise
}
