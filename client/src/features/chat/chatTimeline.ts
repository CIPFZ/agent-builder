import type {
  RuntimeAgentTask,
  RuntimeAuditEvent,
  RuntimeMessage,
  RuntimePermissionRequest,
  RuntimeToolCall,
  RuntimeTurn,
} from '../../runtime'

export type ChatTimelineItem =
  | { id: string; kind: 'turn'; at: number; turn: RuntimeTurn }
  | { id: string; kind: 'message'; at: number; message: RuntimeMessage }
  | { id: string; kind: 'tool'; at: number; toolCall: RuntimeToolCall }
  | { id: string; kind: 'permission'; at: number; permission: RuntimePermissionRequest }
  | { id: string; kind: 'task'; at: number; task: RuntimeAgentTask }
  | { id: string; kind: 'audit'; at: number; event: RuntimeAuditEvent }

type BuildChatTimelineInput = {
  auditEvents: RuntimeAuditEvent[]
  messages: RuntimeMessage[]
  permissions: RuntimePermissionRequest[]
  tasks: RuntimeAgentTask[]
  toolCalls: RuntimeToolCall[]
  turns: RuntimeTurn[]
}

const finalTurnStatuses = new Set(['completed', 'failed', 'cancelled', 'interrupted'])
const noisyAuditTypes = new Set(['turn_summary'])

export function buildChatTimelineItems({
  auditEvents,
  messages,
  permissions,
  tasks,
  toolCalls,
  turns,
}: BuildChatTimelineInput): ChatTimelineItem[] {
  const items: ChatTimelineItem[] = []
  const seen = new Set<string>()

  for (const turn of turns) {
    if (!turn.id || finalTurnStatuses.has(turn.status)) continue
    push(items, seen, { id: `turn:${turn.id}`, kind: 'turn', at: firstPositive(turn.startedAt, turn.finishedAt), turn })
  }

  for (const message of messages) {
    push(items, seen, { id: `message:${message.id}`, kind: 'message', at: firstPositive(message.createdAt, message.updatedAt), message })
  }

  for (const toolCall of toolCalls) {
    push(items, seen, {
      id: `tool:${toolCall.id}`,
      kind: 'tool',
      at: firstPositive(toolCall.startedAt, toolCall.finishedAt),
      toolCall,
    })
  }

  for (const permission of permissions) {
    push(items, seen, {
      id: `permission:${permission.id}`,
      kind: 'permission',
      at: firstPositive(permission.createdAt, permission.decidedAt),
      permission,
    })
  }

  for (const task of tasks) {
    push(items, seen, { id: `task:${task.id}`, kind: 'task', at: firstPositive(task.startedAt, task.updatedAt, task.finishedAt), task })
  }

  for (const event of auditEvents) {
    if (noisyAuditTypes.has(event.type)) continue
    if (!isSummaryAuditEvent(event)) continue
    push(items, seen, { id: `audit:${event.id}`, kind: 'audit', at: Date.parse(event.created_at) || 0, event })
  }

  return items.sort((a, b) => {
    const delta = a.at - b.at
    if (delta !== 0) return delta
    return kindRank(a.kind) - kindRank(b.kind)
  })
}

function push(items: ChatTimelineItem[], seen: Set<string>, item: ChatTimelineItem) {
  if (seen.has(item.id)) return
  seen.add(item.id)
  items.push(item)
}

function firstPositive(...values: Array<number | undefined>) {
  return values.find((value) => typeof value === 'number' && value > 0) ?? 0
}

function kindRank(kind: ChatTimelineItem['kind']) {
  switch (kind) {
    case 'turn':
      return 0
    case 'message':
      return 1
    case 'permission':
      return 2
    case 'tool':
      return 3
    case 'task':
      return 4
    case 'audit':
      return 5
  }
}

function isSummaryAuditEvent(event: RuntimeAuditEvent) {
  return Boolean(
    event.payload?.skill_summary ||
      event.payload?.context_summary ||
      event.payload?.agent_task ||
      event.payload?.permission_decision ||
      event.payload?.permission_risk,
  )
}
