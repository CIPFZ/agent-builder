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

  it("tracks session list, active session, and restored messages", () => {
    const listed = operatorReducer(initialOperatorState, {
      type: "sessions/list",
      payload: [
        {
          session_id: "main-000001",
          session_key: "agent:main:main",
          title: "Main session",
          message_count: 0,
        },
      ],
    });
    expect(listed.sessions).toHaveLength(1);

    const created = operatorReducer(listed, {
      type: "session/created",
      payload: {
        session_id: "session-000002",
        session_key: "agent:main:session:000002",
        title: "New chat",
        message_count: 0,
      },
    });
    expect(created.activeSessionKey).toBe("agent:main:session:000002");
    expect(created.transcript).toEqual([]);

    const restored = operatorReducer(created, {
      type: "session/messages",
      sessionKey: "agent:main:session:000002",
      payload: [{ id: "msg-1", role: "user", content: "hello" }],
    });
    expect(restored.transcript).toEqual([{ id: "msg-1", role: "user", content: "hello" }]);
  });

  it("ignores session-scoped events for inactive sessions", () => {
    const active = operatorReducer(initialOperatorState, {
      type: "session/status",
      payload: {
        session_id: "session-1",
        session_key: "active-key",
      },
    });

    const ignored = operatorReducer(active, {
      type: "ws/event",
      message: {
        type: "event",
        event: "message.created",
        payload: {
          session_id: "session-2",
          session_key: "other-key",
          message: { id: "msg-2", role: "user", content: "other session" },
        },
      },
    });
    expect(ignored.transcript).toEqual([]);
  });
});
