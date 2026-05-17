import type {
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeModel,
  RuntimeModelConfig,
  RuntimeStatus,
} from './types'

type WailsRuntimeBridge = {
  Chat: (request: RuntimeChatRequest) => Promise<RuntimeChatResponse>
  GetModelConfig: () => Promise<{ config: RuntimeModelConfig }>
  Models: () => Promise<{ models: RuntimeModel[] }>
  NewChat: (title: string) => Promise<RuntimeStatus>
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
