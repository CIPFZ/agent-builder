import type { AgentRuntime } from './types'
import { wailsRuntime } from './wailsRuntime'

export type {
  AgentRuntime,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeCapability,
  RuntimeMessage,
  RuntimeMessagePart,
  RuntimeModel,
  RuntimeMcpServer,
  RuntimeEvent,
  RuntimeEventsEndpoint,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeRequests,
  RuntimeEventStats,
  RuntimeSkill,
  RuntimeStatus,
  RuntimeUsage,
} from './types'

export function getAgentRuntime(): AgentRuntime {
  return wailsRuntime
}
