import { troubleshootingEvents } from './events'
import { initialRuntimeState } from './fixtures'
import type { AgentItem, RunEvent, RuntimeState } from '../types/runtime'

function updateAgent(agents: AgentItem[], nextAgent: AgentItem) {
  return agents.map((agent) => (agent.name === nextAgent.name ? nextAgent : agent))
}

export function reduceRunEvent(state: RuntimeState, event: RunEvent): RuntimeState {
  switch (event.type) {
    case 'run_started':
      return {
        ...state,
        run: event.run,
        messages: [...state.messages, event.message],
      }
    case 'agent_updated':
      return {
        ...state,
        agents: updateAgent(state.agents, event.agent),
      }
    case 'message_added':
      return {
        ...state,
        messages: [...state.messages, event.message],
      }
    case 'thought_updated':
      return {
        ...state,
        thoughts: state.thoughts.map((thought) =>
          thought.key === event.thought.key ? event.thought : thought,
        ),
      }
    case 'timeline_added':
      return {
        ...state,
        run: { ...state.run, status: 'running', progress: event.progress },
        timeline: [...state.timeline, event.entry],
      }
    case 'evidence_added':
      return {
        ...state,
        run: { ...state.run, status: 'running', progress: event.progress },
        evidence: [...state.evidence, event.evidence],
      }
    case 'approval_requested':
      return {
        ...state,
        run: { ...state.run, status: 'waiting_approval', progress: event.progress },
        approval: event.approval,
        messages: [...state.messages, event.message],
      }
    case 'report_generated':
      return {
        ...state,
        run: { ...state.run, status: 'completed', progress: event.progress },
        recommendation: event.recommendation,
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
