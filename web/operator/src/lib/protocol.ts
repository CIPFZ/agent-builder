export type WsMessageType = "req" | "res" | "event" | "control_request" | "control_response";

export interface WsErrorPayload {
  message: string;
}

export interface WsEnvelope<TPayload = Record<string, unknown>> {
  type: WsMessageType;
  id?: string;
  method?: string;
  event?: string;
  ok?: boolean;
  error?: WsErrorPayload;
  payload?: TPayload;
}

export interface TranscriptMessage {
  id: string;
  role: string;
  content: string;
  created_at?: string;
}

export interface ToolRun {
  tool_use_id: string;
  tool_name: string;
  status: "called" | "running" | "completed" | "failed" | "approval_required";
  run_id?: string;
  provider_message_id?: string;
  tool_input?: string;
  tool_input_object?: unknown;
  progress: Array<{ type?: string; message?: string; data?: unknown; at: string }>;
  result_message?: TranscriptMessage;
  structured_content?: unknown;
  meta?: unknown;
  error?: string;
}

export interface ApprovalItem {
  approval_id: string;
  run_id?: string;
  session_id?: string;
  tool_name?: string;
  tool_input?: string;
  tool_input_object?: unknown;
  reason?: string;
  status: string;
  decision_reason?: string;
  decision_reason_details?: unknown;
  accept_feedback?: string;
  content_blocks?: unknown;
}

export interface SubagentRun {
  run_id: string;
  label?: string;
  status: string;
  parent_session_id?: string;
  child_session_id?: string;
  child_session_key?: string;
  attempt?: number;
  last_action?: string;
  output?: string;
  output_file?: string;
  error?: string;
  created_at?: string;
  started_at?: string;
  updated_at?: string;
  completed_at?: string;
  control_messages?: string[];
}

export interface McpServer {
  name: string;
  transport_type?: string;
  endpoint?: string;
  enabled?: boolean;
  status?: string;
  tools?: string[];
  prompts?: string[];
  resources?: string[];
  skills?: string[];
  auth_url?: string;
  auth_message?: string;
  auth_scope?: string;
  error?: string;
}

export interface SessionStatus {
  session_id?: string;
  session_key?: string;
  agent_id?: string;
  is_main?: boolean;
  message_count?: number;
  permission_mode?: string;
  subagent_mode?: string;
  plan_mode?: boolean;
  auto_mode?: boolean;
  workspace_roots?: string[];
  main_loop_model?: string;
  session_main_loop_model_override?: string;
  resolved_main_loop_model?: string;
}

export interface OrchestrationRun {
  run_id: string;
  session_id?: string;
  status?: string;
  last_action?: string;
  message?: string;
  tool_name?: string;
  recommended_action?: string;
  decision_priority?: string;
}

export type ConnectionState = "idle" | "connecting" | "connected" | "disconnected" | "error";

export interface OperatorState {
  connection: {
    status: ConnectionState;
    endpoint: string;
    clientId?: string;
    error?: string;
  };
  session: SessionStatus;
  transcript: TranscriptMessage[];
  streaming: {
    content: string;
    runId?: string;
  };
  tools: Record<string, ToolRun>;
  approvals: Record<string, ApprovalItem>;
  subagents: Record<string, SubagentRun>;
  mcp: {
    inventory: Record<string, unknown>;
    servers: McpServer[];
  };
  skills: {
    derived: string[];
    invoked: string[];
  };
  orchestration: OrchestrationRun[];
  gaps: string[];
}

export const initialOperatorState: OperatorState = {
  connection: {
    status: "idle",
    endpoint: "",
  },
  session: {},
  transcript: [],
  streaming: {
    content: "",
  },
  tools: {},
  approvals: {},
  subagents: {},
  mcp: {
    inventory: {},
    servers: [],
  },
  skills: {
    derived: [],
    invoked: [],
  },
  orchestration: [],
  gaps: [
    "Missing skills_status API for a complete runtime skill catalog.",
    "Missing tools inventory API; first version derives visibility from tool events only.",
    "Missing file_tree/file_preview API; file panel is event-derived, not browse-first.",
    "Missing session history API for cold reload transcript restoration.",
  ],
};
