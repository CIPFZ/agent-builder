import type { AgentRuntime } from './types'
import { createHTTPRuntime } from './httpRuntime'
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
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpTool,
  RuntimeModel,
  RuntimeModelDiscoveryResponse,
  RuntimeModelVerifyResponse,
  RuntimeMcpServer,
  RuntimeEvent,
  RuntimeEventsEndpoint,
  RuntimeTurn,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimePolicy,
  RuntimePolicyMode,
  RuntimeRecoveryStatus,
  RuntimeSession,
  RuntimeSkillCreateRequest,
  RuntimeTodo,
  RuntimeTodoSummary,
  RuntimeRequests,
  RuntimeEventStats,
  RuntimeSkill,
  RuntimeStatus,
  RuntimeUsage,
} from './types'

export function getAgentRuntime(): AgentRuntime {
  const baseUrl = import.meta.env.VITE_RUNTIME_API_URL?.trim()
  const token = import.meta.env.VITE_RUNTIME_API_TOKEN?.trim()
  if (baseUrl && token) {
    return createHTTPRuntime({ baseUrl, token })
  }
  return wailsRuntime
}
