import type { AgentRuntime } from './types'
import { wailsRuntime } from './wailsRuntime'

export type {
  AgentRuntime,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeMessage,
  RuntimeMessagePart,
  RuntimeModel,
  RuntimeEvent,
  RuntimeEventsEndpoint,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeRequests,
  RuntimeEventStats,
  RuntimeStatus,
  RuntimeUsage,
} from './types'

export function getAgentRuntime(): AgentRuntime {
  return wailsRuntime
}
