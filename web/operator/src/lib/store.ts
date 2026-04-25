import {
  ApprovalItem,
  initialOperatorState,
  McpServer,
  OperatorState,
  OrchestrationRun,
  SessionStatus,
  SubagentRun,
  ToolRun,
  TranscriptMessage,
  WsEnvelope,
} from "./protocol";

type ToolStatus = ToolRun["status"];

export type OperatorAction =
  | { type: "connection/connecting"; endpoint: string }
  | { type: "connection/connected"; endpoint: string }
  | { type: "connection/error"; error: string }
  | { type: "connection/disconnected" }
  | { type: "ws/event"; message: WsEnvelope }
  | { type: "session/status"; payload: SessionStatus }
  | { type: "mcp/status"; payload: { inventory?: Record<string, unknown>; servers?: McpServer[] } }
  | { type: "approvals/list"; payload: ApprovalItem[] }
  | { type: "subagents/list"; payload: SubagentRun[] }
  | { type: "orchestration/status"; payload: OrchestrationRun[] };

function upsertTranscript(messages: TranscriptMessage[], next: TranscriptMessage): TranscriptMessage[] {
  const index = messages.findIndex((item) => item.id === next.id);
  if (index === -1) {
    return [...messages, next];
  }
  const copy = messages.slice();
  copy[index] = next;
  return copy;
}

function toToolKey(payload: Record<string, unknown>): string | undefined {
  const toolUseId = payload.tool_use_id;
  if (typeof toolUseId === "string" && toolUseId) {
    return toolUseId;
  }
  const providerId = payload.provider_message_id;
  if (typeof providerId === "string" && providerId) {
    return providerId;
  }
  return undefined;
}

function asArrayOfStrings(input: unknown): string[] {
  if (!Array.isArray(input)) {
    return [];
  }
  return input.filter((item): item is string => typeof item === "string");
}

export function operatorReducer(state: OperatorState = initialOperatorState, action: OperatorAction): OperatorState {
  switch (action.type) {
    case "connection/connecting":
      return {
        ...state,
        connection: { status: "connecting", endpoint: action.endpoint },
      };
    case "connection/connected":
      return {
        ...state,
        connection: { status: "connected", endpoint: action.endpoint },
      };
    case "connection/error":
      return {
        ...state,
        connection: { ...state.connection, status: "error", error: action.error },
      };
    case "connection/disconnected":
      return {
        ...state,
        connection: { ...state.connection, status: "disconnected" },
      };
    case "session/status":
      return {
        ...state,
        session: action.payload,
      };
    case "mcp/status": {
      const derived = new Set<string>();
      for (const server of action.payload.servers ?? []) {
        for (const skill of server.skills ?? []) {
          derived.add(skill);
        }
      }
      return {
        ...state,
        mcp: {
          inventory: action.payload.inventory ?? {},
          servers: action.payload.servers ?? [],
        },
        skills: {
          ...state.skills,
          derived: Array.from(derived).sort(),
        },
      };
    }
    case "approvals/list":
      return {
        ...state,
        approvals: Object.fromEntries(action.payload.map((item) => [item.approval_id, item])),
      };
    case "subagents/list":
      return {
        ...state,
        subagents: Object.fromEntries(action.payload.map((item) => [item.run_id, item])),
      };
    case "orchestration/status":
      return {
        ...state,
        orchestration: action.payload,
      };
    case "ws/event": {
      const payload = (action.message.payload ?? {}) as Record<string, unknown>;
      switch (action.message.event) {
        case "hello":
          return {
            ...state,
            connection: {
              ...state.connection,
              status: "connected",
              clientId: typeof payload.client_id === "string" ? payload.client_id : undefined,
            },
            session: {
              ...state.session,
              session_id: typeof payload.session_id === "string" ? payload.session_id : state.session.session_id,
              session_key: typeof payload.session_key === "string" ? payload.session_key : state.session.session_key,
              agent_id: typeof payload.agent_id === "string" ? payload.agent_id : state.session.agent_id,
            },
          };
        case "message.created": {
          const message = payload.message as TranscriptMessage | undefined;
          if (!message?.id) {
            return state;
          }
          return {
            ...state,
            transcript: upsertTranscript(state.transcript, message),
            streaming: message.role === "assistant" ? { content: "" } : state.streaming,
          };
        }
        case "assistant.delta":
          return {
            ...state,
            streaming: {
              content: state.streaming.content + (typeof payload.delta === "string" ? payload.delta : ""),
              runId: typeof payload.run_id === "string" ? payload.run_id : state.streaming.runId,
            },
          };
        case "tool.called": {
          const key = toToolKey(payload);
          if (!key) {
            return state;
          }
          const next: ToolRun = {
            tool_use_id: key,
            tool_name: typeof payload.tool_name === "string" ? payload.tool_name : "unknown",
            status: "called",
            run_id: typeof payload.run_id === "string" ? payload.run_id : undefined,
            provider_message_id: typeof payload.provider_message_id === "string" ? payload.provider_message_id : undefined,
            tool_input: typeof payload.tool_input === "string" ? payload.tool_input : undefined,
            tool_input_object: payload.tool_input_object,
            progress: [],
          };
          return {
            ...state,
            tools: { ...state.tools, [key]: next },
          };
        }
        case "tool.progress": {
          const key = toToolKey(payload);
          if (!key) {
            return state;
          }
          const existing = state.tools[key];
          if (!existing) {
            return state;
          }
          return {
            ...state,
            tools: {
              ...state.tools,
              [key]: {
                ...existing,
                status: "running" as ToolStatus,
                progress: [
                  ...existing.progress,
                  {
                    type: typeof payload.type === "string" ? payload.type : undefined,
                    message: typeof payload.message === "string" ? payload.message : undefined,
                    data: payload.data,
                    at: new Date().toISOString(),
                  },
                ],
              },
            },
          };
        }
        case "tool.result": {
          const key = toToolKey(payload);
          if (!key) {
            return state;
          }
          const existing = state.tools[key];
          if (!existing) {
            return state;
          }
          const message = payload.message as TranscriptMessage | undefined;
          const invoked = state.skills.invoked.slice();
          if (existing.tool_name === "Skill" && existing.tool_input) {
            invoked.push(existing.tool_input);
          }
          return {
            ...state,
            tools: {
              ...state.tools,
              [key]: {
                ...existing,
                status: "completed" as ToolStatus,
                result_message: message,
                structured_content: payload.structured_content,
                meta: payload.meta,
              },
            },
            skills: {
              ...state.skills,
              invoked,
            },
          };
        }
        case "run.error": {
          const runId = typeof payload.run_id === "string" ? payload.run_id : undefined;
          const error = typeof payload.message === "string" ? payload.message : "runtime error";
          const nextTools = { ...state.tools };
          for (const [key, tool] of Object.entries(nextTools)) {
            if (tool.run_id === runId) {
              nextTools[key] = { ...tool, status: "failed" as ToolStatus, error };
            }
          }
          return { ...state, tools: nextTools };
        }
        case "permission.required": {
          const approvalId = typeof payload.approval_id === "string" ? payload.approval_id : undefined;
          if (!approvalId) {
            return state;
          }
          const approval: ApprovalItem = {
            approval_id: approvalId,
            run_id: typeof payload.run_id === "string" ? payload.run_id : undefined,
            session_id: typeof payload.session_id === "string" ? payload.session_id : undefined,
            tool_name: typeof payload.tool_name === "string" ? payload.tool_name : undefined,
            tool_input: typeof payload.tool_input === "string" ? payload.tool_input : undefined,
            tool_input_object: payload.tool_input_object,
            reason: typeof payload.reason === "string" ? payload.reason : undefined,
            status: typeof payload.status === "string" ? payload.status : "pending",
            decision_reason: typeof payload.decision_reason === "string" ? payload.decision_reason : undefined,
            decision_reason_details: payload.decision_reason_details,
            accept_feedback: typeof payload.accept_feedback === "string" ? payload.accept_feedback : undefined,
            content_blocks: payload.content_blocks,
          };
          const key = toToolKey(payload);
          const tools = key && state.tools[key]
            ? {
                ...state.tools,
                [key]: { ...state.tools[key], status: "approval_required" as ToolStatus },
              }
            : state.tools;
          return {
            ...state,
            approvals: { ...state.approvals, [approvalId]: approval },
            tools,
          };
        }
        case "approval.updated": {
          const approvalId = typeof payload.approval_id === "string" ? payload.approval_id : undefined;
          if (!approvalId || !state.approvals[approvalId]) {
            return state;
          }
          return {
            ...state,
            approvals: {
              ...state.approvals,
              [approvalId]: {
                ...state.approvals[approvalId],
                status: typeof payload.status === "string" ? payload.status : state.approvals[approvalId].status,
              },
            },
          };
        }
        case "approval.cleared": {
          const cleared = Array.isArray(payload.cleared) ? payload.cleared.filter((value): value is string => typeof value === "string") : [];
          const next = { ...state.approvals };
          for (const id of cleared) {
            delete next[id];
          }
          return {
            ...state,
            approvals: next,
          };
        }
        case "subagent.updated":
        case "subagent.completed": {
          const runId = typeof payload.run_id === "string" ? payload.run_id : undefined;
          if (!runId) {
            return state;
          }
          return {
            ...state,
            subagents: {
              ...state.subagents,
              [runId]: {
                run_id: runId,
                label: typeof payload.label === "string" ? payload.label : undefined,
                status: typeof payload.status === "string" ? payload.status : "unknown",
                parent_session_id: typeof payload.parent_session_id === "string" ? payload.parent_session_id : undefined,
                child_session_id: typeof payload.child_session_id === "string" ? payload.child_session_id : undefined,
                child_session_key: typeof payload.child_session_key === "string" ? payload.child_session_key : undefined,
                attempt: typeof payload.attempt === "number" ? payload.attempt : undefined,
                last_action: typeof payload.last_action === "string" ? payload.last_action : undefined,
                output: typeof payload.output === "string" ? payload.output : undefined,
                output_file: typeof payload.output_file === "string" ? payload.output_file : undefined,
                error: typeof payload.error === "string" ? payload.error : undefined,
                created_at: typeof payload.created_at === "string" ? payload.created_at : undefined,
                started_at: typeof payload.started_at === "string" ? payload.started_at : undefined,
                updated_at: typeof payload.updated_at === "string" ? payload.updated_at : undefined,
                completed_at: typeof payload.completed_at === "string" ? payload.completed_at : undefined,
                control_messages: asArrayOfStrings(payload.control_messages),
              },
            },
          };
        }
        case "orchestration.updated": {
          const run = payload as unknown as OrchestrationRun;
          const next = state.orchestration.filter((item) => item.run_id !== run.run_id);
          return { ...state, orchestration: [...next, run] };
        }
        case "orchestration.plan_step.updated":
        case "queue.enqueued":
        default:
          return state;
      }
    }
    default:
      return state;
  }
}
