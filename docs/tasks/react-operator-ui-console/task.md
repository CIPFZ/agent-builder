# React Operator UI Console Task

Date: 2026-04-25

## 1. Task Purpose

This is the execution entrypoint for implementing the first React Operator UI for `myclaw`.

The purpose is to expose the capabilities already implemented in the Go runtime through `myclawd`, using React and Ant Design X.

This is not a TUI continuation task.

## 2. Required Reading

Before writing code, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/architecture/runtime-target-architecture.md`
3. `docs/architecture/frontend-backend-boundary.md`
4. `docs/tasks/react-operator-ui-console/task.md`
5. `docs/tasks/react-operator-ui-console/design.md`
6. `docs/tasks/react-operator-ui-console/source-alignment.md`
7. `docs/tasks/react-operator-ui-console/implementation-plan.md`
8. `docs/tasks/react-operator-ui-console/test-validation-plan.md`
9. `docs/tasks/react-operator-ui-console/review-checklist.md`
10. `docs/tasks/mcp-runtime-capability/task.md`
11. `docs/tasks/subagent-runtime-capability/task.md`
12. `docs/tasks/shell-runtime-capability/task.md`
13. `docs/tasks/ssh-runtime-capability/task.md`
14. `docs/tasks/tui-client-architecture/task.md`

Also review the current `myclawd` protocol and gateway implementation:

- `internal/protocol/ws/message.go`
- `internal/gateway/server.go`

## 3. Required External Reference

Use Ant Design X as the UI component direction:

- `https://x.ant.design/index-cn`
- `https://x.ant.design/components/overview-cn`
- `https://x.ant.design/components/bubble-cn`
- `https://x.ant.design/components/sender-cn`
- `https://x.ant.design/components/conversations-cn`
- `https://x.ant.design/components/prompts-cn`

Use it as a React AI UI toolkit, not as a backend protocol.

## 4. Objective

Implement a React web console that lets the operator see and use the current `myclaw` capabilities:

- chat
- tool execution
- file operations
- shell execution
- SSH execution
- MCP
- skills
- subagents/tasks
- approvals
- runtime/session status

## 5. In Scope

- React app scaffold
- Ant Design X conversation UI
- `myclawd` websocket client
- typed protocol state
- transcript rendering
- tool lifecycle rendering
- approval center
- MCP inventory page
- skills visibility page
- subagent/task page
- runtime/session status panel
- protocol gap ledger

## 6. Out Of Scope

- direct runtime calls from React
- replacing `myclawd`
- copying Go TUI implementation details
- full Claude Code visual parity
- Docker/database as first-class React pages before backend runtime contracts exist
- fake backend state presented as complete functionality

## 7. Current Capability Baseline

Current implemented or partially implemented capabilities to expose:

- Query/runtime loop
- file tools: read, write, edit, multiedit, glob, grep, ls
- shell/system execution
- SSH execution
- MCP discovery, status, reconnect, authenticate
- MCP-derived skills
- bundled/plugin/dynamic skills
- Skill tool invocation and skill attachments
- subagent spawn/list/status/stop/steer/resume
- orchestration status/history/plan APIs
- approval list/approve/reject/clear
- session status/model/permission APIs
- tool progress and structured result events

## 8. Required Implementation Order

Implement in this order:

1. frontend scaffold
2. `myclawd` websocket client
3. protocol reducer/store
4. conversation page
5. tool lifecycle cards
6. approval center
7. subagent/task page
8. MCP page
9. skills page
10. runtime/session panel
11. protocol gap ledger
12. manual validation guide

## 9. Validation Requirements

At minimum, validate:

- UI connects to `myclawd`
- prompt sends through `send_message`
- assistant stream renders
- tool calls render
- approval flow works
- MCP status renders
- subagent list/status renders
- runtime/session status renders
- missing skills/tools/file APIs are documented as protocol gaps if not fully available

## 10. Completion Output Requirements

Claude Code must report:

- implemented pages/components
- `myclawd` methods/events consumed
- unsupported capabilities and exact protocol gaps
- tests run
- manual validation steps for the user

## 11. Start Prompt For Claude Code

Use this prompt to start implementation:

```text
You are implementing the first React Operator UI for myclaw.

This is not a TUI continuation task. The goal is to expose the current Go runtime capabilities through myclawd using React + Ant Design X.

Before coding, read:
1. docs/execution/implementation-rules.md
2. docs/architecture/runtime-target-architecture.md
3. docs/architecture/frontend-backend-boundary.md
4. docs/tasks/react-operator-ui-console/task.md
5. docs/tasks/react-operator-ui-console/design.md
6. docs/tasks/react-operator-ui-console/source-alignment.md
7. docs/tasks/react-operator-ui-console/implementation-plan.md
8. docs/tasks/react-operator-ui-console/test-validation-plan.md
9. docs/tasks/react-operator-ui-console/review-checklist.md
10. internal/protocol/ws/message.go
11. internal/gateway/server.go

Also use Ant Design X docs as the UI reference:
- https://x.ant.design/index-cn
- https://x.ant.design/components/bubble-cn
- https://x.ant.design/components/sender-cn
- https://x.ant.design/components/conversations-cn
- https://x.ant.design/components/prompts-cn

After reading, output a short execution summary covering:
- UI objective
- pages/components to implement
- myclawd methods/events to consume
- current runtime capabilities to expose
- known protocol gaps
- validation plan

Then implement within scope.

Rules:
- React must talk to myclawd only.
- Do not call Go runtime internals from React.
- Do not fabricate runtime truth in the UI.
- Show current capabilities: chat, tools, files, shell, SSH, MCP, skills, subagents/tasks, approvals, runtime/session status.
- Docker/database should remain generic shell/MCP/tool execution views unless backend first-class contracts are added.
```
