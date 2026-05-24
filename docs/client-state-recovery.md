# 客户端状态恢复机制

Status: partially implemented design baseline. Runtime event sequence/cursor,
recovery status, turn/tool/permission persistence, startup recovery, and client
event subscription foundations now exist. This document still defines the
recovery model, but the open issues below are narrower than the original list.

Current remaining gaps are tracked in:

- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`

客户端化产品必须能从 runtime 恢复状态。React 内存状态不能成为核心业务状态来源。

## 需要解决的问题

当前客户端已经在启动时拉取 status、sessions、messages、permissions、
events、skills、MCP、capabilities，并通过 SSE 订阅事件。已实现基础：

- event sequence/cursor,
- `/v1/recovery/status`,
- turn/tool/permission/audit persistence foundations,
- interrupted turn/task marking,
- pending permission recovery,
- snapshot-required recovery behavior.

仍有缺口：

- long-term persisted event replay/export,
- compact-aware recovery and post-compact reinjection,
- richer task/tool/audit diagnostics after refresh,
- reducing any remaining polling assumptions to short fallback only.

## 恢复原则

1. API 是事实来源。
2. Event stream 是增量通知。
3. 客户端启动必须能无事件恢复完整当前状态。
4. 未完成 turn 必须有明确状态。
5. pending permission 必须可重新展示。
6. 断线重连后不能丢关键事件。

## 启动恢复流程

```text
load model config
  -> status
  -> sessions
  -> active session messages
  -> active/running turns
  -> pending permissions
  -> capabilities
  -> skills/MCP
  -> recent audit/events
  -> subscribe events with cursor
```

UI 应避免从空状态直接渲染为“没有会话”。应该区分：

- loading
- model not configured
- runtime unavailable
- no sessions
- active session loaded
- recovery in progress

## Event Cursor

建议事件增加 sequence：

```text
event_id
sequence
created_at
type
```

客户端订阅：

```text
GET /v1/events?after={sequence}
Accept: text/event-stream
```

如果 after 太旧，runtime 返回 snapshot required，客户端重新拉全量状态。

## Active Turn 恢复

runtime 应提供：

```text
GET /v1/turns?status=active
GET /v1/sessions/{session_id}/turns?status=active
```

客户端恢复后：

- 有 running turn：显示 active status 和 cancel。
- 有 waiting_permission turn：展示 permission modal/drawer。
- 有 interrupted turn：展示中断状态和 audit。
- 没有 active turn：正常显示历史消息。

## Pending Permission 恢复

`GET /v1/permissions` 必须返回所有 pending permission，而不依赖 SSE。

permission 应包含：

- session id
- turn id
- tool call id
- created at
- risk
- status

如果 permission 对应的 turn 已取消或中断，runtime 应将其标记为 cancelled/expired，而不是让 UI 永远卡住。

## Runtime 重启恢复

runtime 启动时扫描持久化状态：

```text
runtime_turns where status in queued/running/waiting_permission/cancelling
```

处理规则：

- 可恢复的后台任务继续。
- 不可恢复的 model/tool call 标记为 interrupted。
- pending permission 如果 tool call 已不存在，标记 expired。
- 写入 `turn.interrupted` 和 `audit.recorded`。

## 客户端重连策略

SSE 断线后：

1. 延迟重连。
2. 使用 last sequence 继续订阅。
3. 如果失败，拉 `GET /v1/events?after=...`。
4. 如果 runtime 要求 snapshot，执行全量刷新。

不要只靠 700ms message polling。轮询可以作为兜底，但不应是主机制。

## API 补充

```text
GET /v1/events?after={sequence}
GET /v1/turns?status=active
GET /v1/sessions/{session_id}/turns
GET /v1/recovery/status
```

`/v1/recovery/status` 可返回：

```text
runtime_started_at
last_event_sequence
interrupted_turns
active_turns
pending_permissions
snapshot_required
```

## 迁移步骤

Historical checklist status:

1-6 are implemented as foundations. Step 7 remains a cleanup goal: polling may
exist only as a fallback and should not be the source of truth.

1. 给 runtime event 增加 sequence。
2. 事件内存 ring buffer 保留 sequence 范围。
3. active turn 从内存 map 迁移到 turn store。
4. pending permission 增加状态并持久化。
5. 客户端启动时增加 active turn 恢复。
6. SSE hook 支持 reconnect + cursor。
7. 去掉发送消息流程里的长期轮询依赖，只保留短周期兜底。
