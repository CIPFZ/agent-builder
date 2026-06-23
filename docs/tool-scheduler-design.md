# Tool Scheduler 设计

Status: partially implemented design baseline. The foundation exists in
`internal/tools/scheduler`, `internal/agent/scheduler_tool.go`, and
`internal/runtime/runtime_scheduler_recorder.go`. This document still describes
the intended scheduler semantics, but it should not be read as entirely
unimplemented.

Current remaining gaps are tracked in:

- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`

Tool Scheduler 是客户端化 runtime 的关键模块。它把模型产生的工具调用变成可审批、可取消、可审计、可恢复、可展示的 runtime 对象。

## Historical Problem Statement

Earlier Agent Builder tool execution hid most lifecycle state in the agent loop and
tool implementation, while the desktop client observed message parts and
converted them into runtime events. The scheduler foundation now records
ToolCall lifecycle, policy decisions, events, and audit. The remaining problems
are narrower:

- tool search/discovery is missing,
- explicit deadlock/recursion/concurrency policy is incomplete,
- large output refs and artifact/diff refs are still partial,
- result normalization needs stronger model-visible vs UI-visible boundaries,
- scheduler scenario tests need to cover MCP, skills, shell, and subagents.

## 目标职责

Tool Scheduler 负责：

- 接收 tool call 请求。
- 生成 ToolCall runtime object。
- 校验 input schema。
- 计算风险等级。
- 调用 permission policy。
- 管理并发和队列。
- 执行 built-in / MCP / plugin tool。
- 规范化输出。
- 提取 diff/artifact。
- 发出 lifecycle events。
- 写入 audit。
- 响应 cancellation。

## 目标流程

```text
Agent loop emits tool call
  -> ToolScheduler.CreateCall()
  -> validate input
  -> policy.evaluate()
  -> if approval needed: create PermissionRequest and pause
  -> execute tool
  -> normalize result
  -> record ToolCall
  -> emit events
  -> return result to agent loop
```

## ToolCall 状态机

```text
pending
  -> waiting_permission
  -> running
  -> completed

pending
  -> running
  -> failed

waiting_permission
  -> denied
  -> failed

running
  -> cancelling
  -> cancelled
```

客户端只展示状态，不直接修改状态。状态变化必须来自 runtime。

## Scheduler 接口草案

```go
type Scheduler interface {
    Execute(ctx context.Context, req ToolCallRequest) (ToolCallResult, error)
    GetCall(ctx context.Context, id string) (ToolCall, error)
    ListCalls(ctx context.Context, turnID string) ([]ToolCall, error)
    CancelCall(ctx context.Context, id string) error
}
```

`ToolCallRequest`：

```text
id
turn_id
session_id
message_id
name
source
input
capability_id
```

`ToolCallResult`：

```text
tool_call_id
status
content
structured_output
stdout
stderr
diffs
artifacts
error
```

## 输出规范

工具输出至少分为：

- `content`: 给模型看的文本结果。
- `structured_output`: 给客户端和审计看的结构化结果。
- `stdout`: shell 或进程标准输出。
- `stderr`: shell 或进程标准错误。
- `diffs`: 文件修改 diff 引用。
- `artifacts`: 生成文件、图片、报告等引用。

不要把所有输出压成一段文本。客户端应能按工具类型渲染不同视图。

## Permission 关系

Tool Scheduler 不直接问 UI。它调用 runtime policy：

```text
decision = PermissionPolicy.Evaluate(tool_call)
```

结果：

- `allow`: 直接执行。
- `ask`: 创建 PermissionRequest 并等待 decision。
- `deny`: 直接失败。

Permission decision 后，Scheduler 继续或终止 tool call。

## Event 关系

Scheduler 发出：

```text
tool.call.started
permission.requested
permission.decided
tool.call.output
tool.call.completed
tool.call.failed
tool.call.cancelled
audit.recorded
```

event payload 可以携带摘要，但完整输入输出、diff、artifact 应通过 API 查询。

## 与 MCP 的关系

MCP tool 对 Scheduler 来说只是 source 不同：

```text
source = mcp
capability_id = mcp:{server}:{tool}
```

Scheduler 统一处理：

- MCP tool input schema
- permission
- timeout
- cancellation
- result normalization
- audit

## 与客户端的关系

客户端需要 API：

```text
GET /v1/turns/{turn_id}/tool-calls
GET /v1/tool-calls/{tool_call_id}
POST /v1/tool-calls/{tool_call_id}/cancel
```

初期也可以只在 `GET /v1/turns/{turn_id}` 中嵌入 tool call 摘要，但 detail view 需要独立 API。

## 迁移步骤

Historical checklist status:

1-5 are implemented as foundations. Step 6 is partially implemented in React
timeline/tool surfaces and still needs richer detail/replay/diagnostic views.

1. 定义 `ToolCall` runtime struct 和 contract。
2. 从 `runtime_events.go` 的 message part 反推逻辑中提取 tool call normalization。
3. 在 agent tool execution 前后插入 scheduler hooks。
4. 把 permission request 与 tool call id、turn id 绑定。
5. 将 audit tool calls 从 message 扫描迁移到 scheduler store。
6. 客户端 tool card 改读 ToolCall API。
