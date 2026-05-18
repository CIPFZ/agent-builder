import type { AgentRuntime } from './types'
import { wailsRuntime } from './wailsRuntime'

export type {
  AgentRuntime,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeMessage,
  RuntimeModel,
  RuntimeEventStats,
  RuntimeStatus,
  RuntimeUsage,
} from './types'

export function getAgentRuntime(): AgentRuntime {
  return wailsRuntime
}
