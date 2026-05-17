import type { AgentRuntime } from './types'
import { wailsRuntime } from './wailsRuntime'

export type {
  AgentRuntime,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeMessage,
  RuntimeModel,
  RuntimeStatus,
} from './types'

export function getAgentRuntime(): AgentRuntime {
  return wailsRuntime
}
