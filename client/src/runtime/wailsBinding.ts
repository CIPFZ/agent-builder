import type {
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeEvent,
  RuntimeMessage,
  RuntimeModel,
  RuntimeModelConfig,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeStatus,
} from './types'

type WailsRuntimeBridge = {
  Cancel: () => Promise<RuntimeStatus>
  Chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  DecidePermission: (request: RuntimePermissionDecision) => Promise<RuntimeStatus>
  Events: () => Promise<{ events: RuntimeEvent[] }>
  GetModelConfig: () => Promise<{ config: RuntimeModelConfig }>
  Messages: () => Promise<{ messages: RuntimeMessage[] }>
  Models: () => Promise<{ models: RuntimeModel[] }>
  NewChat: (title: string) => Promise<RuntimeStatus>
  Permissions: () => Promise<{ permissions: RuntimePermissionRequest[] }>
  SaveModelConfig: (request: RuntimeModelConfig) => Promise<{ config: RuntimeModelConfig }>
  Status: () => Promise<RuntimeStatus>
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
