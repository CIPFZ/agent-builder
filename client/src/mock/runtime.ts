import { troubleshootingEvents } from './events'
import { initialRuntimeState } from './fixtures'
import type { AgentItem, ApprovalDecision, RunEvent, RuntimeState } from '../types/runtime'

function updateAgent(agents: AgentItem[], nextAgent: AgentItem) {
  return agents.map((agent) => (agent.name === nextAgent.name ? nextAgent : agent))
}

export function reduceRunEvent(state: RuntimeState, event: RunEvent): RuntimeState {
  const eventLog = [
    ...state.eventLog,
    {
      id: `${state.eventLog.length + 1}`,
      timestamp: new Date().toISOString(),
      event,
    },
  ]

  switch (event.type) {
    case 'run_started':
      return {
        ...state,
        eventLog,
        run: event.run,
        messages: [...state.messages, event.message],
      }
    case 'agent_updated':
      return {
        ...state,
        eventLog,
        agents: updateAgent(state.agents, event.agent),
      }
    case 'message_added':
      return {
        ...state,
        eventLog,
        messages: [...state.messages, event.message],
      }
    case 'thought_updated':
      return {
        ...state,
        eventLog,
        thoughts: state.thoughts.map((thought) =>
          thought.key === event.thought.key ? event.thought : thought,
        ),
      }
    case 'timeline_added':
      return {
        ...state,
        eventLog,
        run: { ...state.run, status: 'running', progress: event.progress },
        timeline: [...state.timeline, event.entry],
      }
    case 'evidence_added':
      return {
        ...state,
        eventLog,
        run: { ...state.run, status: 'running', progress: event.progress },
        evidence: [...state.evidence, event.evidence],
      }
    case 'approval_requested':
      return {
        ...state,
        eventLog,
        run: { ...state.run, status: 'waiting_approval', progress: event.progress },
        approval: event.approval,
        messages: [...state.messages, event.message],
      }
    case 'report_generated':
      return {
        ...state,
        eventLog,
        run: { ...state.run, status: 'completed', progress: event.progress },
        recommendation: event.recommendation,
        messages: [...state.messages, event.message],
      }
    case 'approval_resolved':
      return {
        ...state,
        eventLog,
        approval: undefined,
        run: {
          ...state.run,
          status: event.decision === 'approved' ? 'completed' : 'completed',
          progress: event.progress,
        },
        timeline: [...state.timeline, event.entry],
        messages: [...state.messages, event.message],
      }
    default:
      return state
  }
}

export function replayEvents(
  onEvent: (event: RunEvent) => void,
  options?: { intervalMs?: number; signal?: AbortSignal },
) {
  const intervalMs = options?.intervalMs ?? 650
  let index = 0

  const tick = () => {
    if (options?.signal?.aborted || index >= troubleshootingEvents.length) {
      return
    }

    onEvent(troubleshootingEvents[index])
    index += 1
    window.setTimeout(tick, intervalMs)
  }

  tick()
}

export function createInitialRuntimeState(): RuntimeState {
  return structuredClone(initialRuntimeState)
}

export function createApprovalResolvedEvent(decision: ApprovalDecision): RunEvent {
  const approved = decision === 'approved'
  return {
    type: 'approval_resolved',
    approvalId: 'apr-plan-1',
    decision,
    progress: 100,
    message: {
      id: `msg-approval-${decision}`,
      role: 'user',
      content: approved ? '同意生成低风险修复计划。' : '暂不生成修复计划，保持只读排障结论。',
    },
    entry: {
      id: `tl-approval-${decision}`,
      title: approved ? '用户已批准生成修复计划' : '用户拒绝高风险后续动作',
      description: approved
        ? 'runtime 将只生成计划和报告，不执行真实修复命令。'
        : 'run 已保留排障证据和建议，未进入修复流程。',
      kind: approved ? 'success' : 'warning',
    },
  }
}
