import type { AgentRuntime } from './types'
import { wailsRuntime } from './wailsRuntime'

export type {
  AgentRuntime,
  RuntimeAuditEvent,
  RuntimeChatRequest,
  RuntimeChatResponse,
  RuntimeCapability,
  RuntimeMessage,
  RuntimeMessagePart,
  RuntimeMcpServerConfig,
  RuntimeMcpTool,
  RuntimeModel,
  RuntimeModelVerifyResponse,
  RuntimeMcpServer,
  RuntimeEvent,
  RuntimeEventsEndpoint,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeSession,
  RuntimeRequests,
  RuntimeEventStats,
  RuntimeSkill,
  RuntimeStatus,
  RuntimeUsage,
} from './types'

export function getAgentRuntime(): AgentRuntime {
  return wailsRuntime
}
