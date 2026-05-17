export type RunStatus = 'idle' | 'running' | 'waiting_approval' | 'completed'
export type ApprovalDecision = 'approved' | 'denied'

export type EvidenceStatus = 'normal' | 'warning' | 'critical'

export type AgentStatus = 'idle' | 'planning' | 'running' | 'waiting' | 'completed'

export type ConversationRole = 'user' | 'assistant' | 'tool' | 'approval'

export type ThoughtStatus = 'success' | 'loading' | 'error' | 'abort'

export type TimelineKind = 'success' | 'running' | 'warning' | 'error'

export type RunItem = {
  id: string
  title: string
  target: string
  status: RunStatus
  progress: number
}

export type AgentItem = {
  name: string
  status: AgentStatus
}

export type CapabilityItem = {
  name: string
  meta: string
  type: 'ssh' | 'skill' | 'mcp' | 'audit'
}

export type ConversationMessage = {
  id: string
  role: ConversationRole
  content: string
}

export type ThoughtStep = {
  key: string
  title: string
  description: string
  status: ThoughtStatus
}

export type TimelineEntry = {
  id: string
  title: string
  description: string
  kind: TimelineKind
}

export type EvidenceItem = {
  key: string
  source: 'SSH' | 'MCP'
  command: string
  signal: string
  status: EvidenceStatus
}

export type ApprovalRequest = {
  id: string
  title: string
  description: string
  actions: string[]
}

export type RuntimeEventRecord = {
  id: string
  timestamp: string
  event: RunEvent
}

export type Recommendation = {
  title: string
  description: string
  nextSteps: string[]
}

export type RuntimeState = {
  run: RunItem
  agents: AgentItem[]
  capabilities: CapabilityItem[]
  messages: ConversationMessage[]
  thoughts: ThoughtStep[]
  timeline: TimelineEntry[]
  evidence: EvidenceItem[]
  approval?: ApprovalRequest
  recommendation?: Recommendation
  eventLog: RuntimeEventRecord[]
}

export type SopStep = {
  id: string
  title: string
  command: string
  expectedSignal: string
  risk: 'read' | 'write' | 'destructive'
}

export type SopFixture = {
  id: string
  name: string
  description: string
  targetService: string
  riskLevel: 'low' | 'medium' | 'high'
  requiredCapabilities: string[]
  steps: SopStep[]
}

export type SshTarget = {
  id: string
  name: string
  host: string
  user: string
  port: number
  profile: string
  environment: string
}

export type RunEvent =
  | {
      type: 'run_started'
      run: RunItem
      message: ConversationMessage
    }
  | {
      type: 'agent_updated'
      agent: AgentItem
    }
  | {
      type: 'message_added'
      message: ConversationMessage
    }
  | {
      type: 'thought_updated'
      thought: ThoughtStep
    }
  | {
      type: 'timeline_added'
      entry: TimelineEntry
      progress: number
    }
  | {
      type: 'evidence_added'
      evidence: EvidenceItem
      progress: number
    }
  | {
      type: 'approval_requested'
      approval: ApprovalRequest
      message: ConversationMessage
      progress: number
    }
  | {
      type: 'report_generated'
      recommendation: Recommendation
      message: ConversationMessage
      progress: number
    }
  | {
      type: 'approval_resolved'
      approvalId: string
      decision: ApprovalDecision
      message: ConversationMessage
      entry: TimelineEntry
      progress: number
    }
