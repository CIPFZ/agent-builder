import { describe, expect, it } from "vitest";

import { initialOperatorState, WsEnvelope } from "./protocol";
import { operatorReducer } from "./store";

describe("operatorReducer", () => {
  it("accumulates assistant streaming and final message reconciliation", () => {
    const delta: WsEnvelope = {
      type: "event",
      event: "assistant.delta",
      payload: { run_id: "run-1", delta: "hello" },
    };
    const streamed = operatorReducer(initialOperatorState, { type: "ws/event", message: delta });
    expect(streamed.streaming.content).toBe("hello");

    const created: WsEnvelope = {
      type: "event",
      event: "message.created",
      payload: {
        message: { id: "msg-1", role: "assistant", content: "hello", created_at: "2026-04-25T12:00:00Z" },
      },
    };
    const finalState = operatorReducer(streamed, { type: "ws/event", message: created });
    expect(finalState.streaming.content).toBe("");
    expect(finalState.transcript).toHaveLength(1);
    expect(finalState.transcript[0].content).toBe("hello");
  });

  it("tracks tool lifecycle and approvals from runtime events", () => {
    const called = operatorReducer(initialOperatorState, {
      type: "ws/event",
      message: {
        type: "event",
        event: "tool.called",
        payload: {
          tool_use_id: "tool-1",
          tool_name: "Read",
          run_id: "run-2",
          tool_input: "{\"file\":\"README.md\"}",
        },
      },
    });
    const progressed = operatorReducer(called, {
      type: "ws/event",
      message: {
        type: "event",
        event: "tool.progress",
        payload: {
          tool_use_id: "tool-1",
          type: "stdout",
          message: "reading file",
        },
      },
    });
    expect(progressed.tools["tool-1"].status).toBe("running");
    expect(progressed.tools["tool-1"].progress).toHaveLength(1);

    const approvalRequired = operatorReducer(progressed, {
      type: "ws/event",
      message: {
        type: "event",
        event: "permission.required",
        payload: {
          approval_id: "approval-1",
          tool_use_id: "tool-1",
          run_id: "run-2",
          tool_name: "Read",
          status: "pending",
          reason: "needs confirmation",
        },
      },
    });
    expect(approvalRequired.approvals["approval-1"].reason).toBe("needs confirmation");
    expect(approvalRequired.tools["tool-1"].status).toBe("approval_required");
  });

  it("derives mcp skills from mcp status response", () => {
    const next = operatorReducer(initialOperatorState, {
      type: "mcp/status",
      payload: {
        inventory: { server_count: 1, skill_count: 2 },
        servers: [
          { name: "github", skills: ["gh-address-comments", "gh-fix-ci"] },
          { name: "browser", skills: ["gh-fix-ci"] },
        ],
      },
    });
    expect(next.skills.derived).toEqual(["gh-address-comments", "gh-fix-ci"]);
  });
});
