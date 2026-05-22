import type { ModelConfig } from '../../runtime/api'
import type { RuntimeMessage, RuntimeMessagePart } from '../../runtime'

export const emptyUsage = {
  promptTokens: 0,
  completionTokens: 0,
  totalTokens: 0,
  cost: 0,
}

export function modelLabel(config: ModelConfig) {
  return config.model || 'Choose model'
}

export function isDefaultSessionTitle(title?: string) {
  const normalized = title?.trim().toLowerCase()
  return !normalized || normalized === 'new chat' || normalized === 'new session'
}

export function hasAssistantText(chatMessage: RuntimeMessage) {
  return chatMessage.role === 'assistant' && Boolean(chatMessage.content?.trim())
}

export function messageToolParts(chatMessage: RuntimeMessage) {
  return (chatMessage.parts ?? []).filter((part) => part.type === 'tool_call' || part.type === 'tool_result')
}

export function messageReasoningParts(chatMessage: RuntimeMessage) {
  return (chatMessage.parts ?? []).filter((part) => part.type === 'reasoning' && part.thinking?.trim())
}

export type RuntimeMessageDisplayPart = RuntimeMessagePart
