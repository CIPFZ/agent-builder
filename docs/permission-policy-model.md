# Permission 与 Policy 模型

Status: partially implemented design baseline. The deterministic policy
foundation now exists in `internal/permission/policy.go` and
`internal/runtime/runtime_policy.go`, with `ask`, `auto_read`, `plan`, and
`deny_all` modes. This document remains the policy model reference, but older
phrasing that treats policy as entirely missing is historical.

Current remaining gaps are tracked in:

- `docs/claude-code-runtime-parity-audit.md`
- `docs/claude-code-alignment-next-roadmap.md`

本文定义客户端化后的权限与策略模型。目标是把终端 prompt 风格的审批升级为 runtime 一等对象。

## 当前基础

`internal/permission` and `internal/runtime` now have:

- `PermissionRequest`
- session persistent grant
- allow once / allow session / deny
- mode-aware `PermissionPolicy`
- `ask`, `auto_read`, `plan`, and `deny_all`
- risk classification for read/write/execute/network/secret/destructive
- policy reason, decision, risk, and policy mode recorded on permission/tool
  lifecycle
- hook pre-approval
- pubsub notification
- `/v1/policy`

仍缺少或只部分实现的客户端产品结构：

- scoped policy rules for tool/MCP/skill/subagent/cwd/shell
- richer shell command parsing and safety
- headless profiles
- rule source precedence and diagnostics
- policy regression scenario harness
- target summary
- stronger secret/path/resource redaction coverage

Model-assisted permission remains future-only and advisory. A model may explain
risk or summarize intent, but Go runtime policy must make the final
allow/ask/deny decision.

## 目标模型

```text
ToolCall
  -> PermissionPolicy.Evaluate
    -> allow
    -> ask
    -> deny
```

Policy 负责判断；UI 负责展示；Tool Scheduler 负责等待和继续执行。

## PermissionRequest 字段

推荐字段：

```text
id
session_id
turn_id
tool_call_id
tool_name
action
risk: read | write | execute | network | secret | destructive
target
target_path
params_summary
policy_reason
options
status
created_at
expires_at
decided_at
decision
```

`params_summary` 必须是 redacted summary，不应直接把完整 env、headers、token、API key 暴露给 UI。

## Decision 类型

客户端至少支持：

```text
allow_once
allow_session
deny
cancel_turn
```

当前 TypeScript 中是 `allow | allow_session | deny`，建议后续改成更明确的 `allow_once`。

## Policy Mode

建议支持：

| 模式 | 行为 |
| --- | --- |
| `ask` | 风险操作询问用户。默认模式。 |
| `auto_read` | 读操作自动允许，写/执行询问。 |
| `plan` | 不执行写入和 shell，只产出计划。 |
| `deny_all` | 禁止所有工具执行，用于安全诊断。 |
| `trusted_workspace` | Future scoped profile; not implemented as a baseline mode. |

不要把这些模式做成前端判断。前端只展示当前模式并调用 runtime 修改配置。

## 风险分类

初始风险：

- `read`: 读文件、列目录、搜索。
- `write`: 写文件、修改配置、生成文件。
- `execute`: shell、脚本、二进制执行。
- `network`: 外部请求、远程 MCP。
- `secret`: 访问密钥、token、credential。
- `destructive`: 删除、覆盖、reset、kill、清空目录。

一个 tool call 可以有多个 risk tag。

## 审批 UI 要求

Permission modal/drawer 应展示：

- 工具名
- 操作类型
- 风险等级
- 目标路径或服务
- 输入摘要
- policy reason
- allow once / allow session / deny / cancel

不要展示完整 secret。高风险操作应有明确视觉层级，但不应让 UI 自行判断是否危险。

## Audit 要求

每次 permission lifecycle 都要写 audit：

```text
permission.requested
permission.allowed_once
permission.allowed_session
permission.denied
permission.expired
permission.cancelled
```

audit payload：

- permission id
- turn id
- tool call id
- tool name
- action
- risk
- target summary
- decision
- timestamp

不记录 secret。

## API

```text
GET  /v1/permissions
GET  /v1/permissions/{permission_id}
POST /v1/permissions/{permission_id}/decision
GET  /v1/policy
PUT  /v1/policy
```

Phase 2 可以先保留已有 permissions API，后续再增加 policy API。

## 迁移步骤

Historical checklist status:

1, 2, 4, and 5 are implemented as foundations. Step 3 remains a naming/API
cleanup candidate, and Step 6 needs richer diagnostics/rule editing after
scoped policy exists.

1. 在 runtime permission shape 中增加 `turn_id`、`risk`、`status`。
2. 把 permission decision 写入 audit store。
3. 把 `allow` 命名收敛为 `allow_once`。
4. 在 Tool Scheduler 中统一调用 PermissionPolicy。
5. 增加 policy mode 配置。
6. 客户端权限 UI 从 pending list 升级为 permission lifecycle view。
