# 功能查漏补缺清单

本文件只记录现状和可疑缺口，不提供重构路线图。

## 已实现能力

### Runtime/API

- `RuntimeService` 已统一 Wails 和 HTTP/dev runtime API 的后端边界。
- loopback HTTP server 支持 Bearer token、SSE、terminal WebSocket、dev JSONP/module fallback。
- runtime events 支持 cursor replay 和 SSE 订阅。
- `SessionActivity` / window / turn activity 已作为 UI timeline 的主要 hydration 来源。

### 项目、会话、消息

- 项目 open/create/rename/remove/open explorer。
- session CRUD、select、new chat、rename/delete、project/standalone scope。
- message CRUD、content parts、tool call/result/reasoning/image/binary/finish。
- session activity 汇总 messages、turns、tool calls、permissions、diagnostics 等 runtime 证据。

### 模型与 Provider

- provider catalog。
- configured provider CRUD。
- provider model discovery/test/latency。
- model config get/save/discover/verify。
- selected model get/save。

### Turn 与输入

- chat 提交。
- structured user input 提交与归一化。
- Slash/meta/shell/voice/multimodal 输入字段已有 DTO 支持。
- turn 查询、列表、取消、interrupted done。

### Tool、Permission、Policy

- tool call start/output/completed/failed/cancelled 记录。
- tool wrapper 支持 scheduler recorder 和 hooks wrapper。
- pending permission 查询和决策。
- runtime policy get/update。
- shell risk、tool risk、MCP/skill scope、headless policy、scoped rules 有测试线索。
- sandbox decision store 和查询。

### Audit、Recovery、Replay

- turn/session audit 查询。
- replay export。
- 启动恢复未完成 turn/task/tool/hook/worktree/permission/MCP request 的线索已实现。
- interrupted recovery DTO 已进入前端诊断面板。

### Run / Scheduler

- runtime runs、run summaries、checkpoint markers。
- checkpoint acknowledge/discard/resume。
- run projection。
- transition history。
- scheduler plan。
- execute run task。

### Skills / Plugins / MCP / Capabilities

- skills list/refresh/create/add path/toggle。
- builtin/user/project skills discovery、frontmatter validate、同名覆盖、disabled filter、prompt XML 注入、loaded tracker。
- plugins list。
- MCP server save/toggle/refresh/retry。
- MCP tools/resources/prompts list。
- MCP requests list/detail/decision。
- capability list/refresh。
- tool search。

### Agent Tasks / Worktrees / Terminal

- agent task list/detail/cancel。
- task messages、follow-up、result、output。
- default agent roles。
- worktree create/enter/exit/cleanup/effective scope。
- session terminal create/list/input/resize/delete/event subscribe。

### Frontend UI

- Workbench shell。
- Sidebar/project/session/new chat。
- Composer。
- Timeline message/thinking/tool/permission/progress/diagnostic/agent task/terminal marker。
- Settings provider/model/policy/skills/MCP/common settings。
- Plugin center。
- Diagnostics panels：turn diagnostics、callchain、context diagnostics、run projection、agent tasks。
- Runtime adapter fallback：Wails、HTTP、dev module、JSONP、XHR/polling。
- xterm terminal 面板，支持 create/list/stream/input/resize/delete。
- Run projection preview、checkpoint resume、scheduler candidates、execute task 入口。
- Plugin center 支持 plugins/skills 浏览、搜索、筛选、详情和 skill toggle。

## 部分实现或可疑缺口

### 契约与路由漂移

- `internal/runtimeapi.Endpoints` 与 `internal/runtime/runtime_http.go` 的实际路由不完全一致。HTTP 已有更多 routes，例如 projects、providers、user-inputs、terminals、run-summaries 等。
- `runtime_runs.go` 发布 `run.checkpoint.resumed`，但 `runtimeapi.EventTypes` / `Validate()` 可能未包含该事件类型。
- Wails bridge、HTTP switch、frontend adapter DTO、runtimeapi contract 四处都维护 runtime API 形状，存在漂移风险。
- `desktop/runtime_bridge.go` 通过大量手写转发保持 Wails bridge，虽然测试较多，但新增 `RuntimeService` 方法时仍容易漏转发。

### 文档/路径不一致

- `internal/workbench/server/apitypes` 路径不存在，实际是 `internal/workbench` 和 `internal/apitypes`。
- 现有 `docs/client-first-runtime-refactor.md` 有中文编码异常，不能直接作为中文事实来源。

### 持久化访问层不对称

- `internal/db` migration 包含大量 runtime 表，但 sqlc query 主要覆盖 session/message/files/read_files/stats。runtime 表访问层分散在 `internal/runtime/*_store.go`。
- 这不一定是 bug，但梳理时需要把“DB schema”和“runtime store”一起看，否则容易误判能力是否完整。

### 删除事件语义不一致

- `message.Service.DeleteSessionMessages` 逐条删除并发布 message 删除事件。
- `session.Delete` 直接执行 SQL 删除 session messages/files/session，不逐条发布 message 删除事件。
- 如果 UI 或 audit 依赖 message deletion event，这里可能有补缺点。

### 非持久状态

- `session.EstimatedUsage` 是内存 map，重启丢失。可能是有意设计，但如果 UI 希望恢复 usage 估算，需要明确。

### 文件读取跟踪

- `filetracker.RecordRead` / `RecordReadState` 写库失败只打日志，调用方无法感知。
- `filetracker` 使用进程当前工作目录计算相对路径，不是显式 workspace root，多 workspace 或 cwd 变化时可能有偏差。

### 项目索引并发与写入

- `projects.Register` 的 read-modify-write 不是单个临界区。
- `projects.Save` 使用普通 `os.WriteFile`，不是 atomic write。

### 前端能力暴露待核对

- capability inventory / tool search 后端能力完整度较高，但前端产品化入口不明确。
- run summaries、checkpoint markers、transition history 有低层 adapter/HTTP 线索，但未作为主要 UI state 消费。
- runtime terminal 与前端 terminal 面板都有生命周期能力，但重连、session ownership、WebSocket ack 体验仍需实测。
- agent task 后端能力较多，前端有 AgentTaskPanel 和 timeline row，但 role 管理、follow-up、output refs 的完整 UI 暴露需要继续核对。
- Settings nav 中有 agents、memory、computer-use、token-usage、diagnostics 等项，但 switch 主要实现 providers/skills/MCP/common/general，其他 key 走默认 General。
- 目标计划中的 refs/artifacts center、audit/replay、worktree/diff detail、usage/cost、memory/context editor、computer use、automations 等仍偏目标或部分入口。

### 前端开发环境约束

- Vite/in-app browser 可能没有 `fetch` 和 `XMLHttpRequest`。
- adapter 已提供 fallback，但新增功能如果直接调用 fetch/Wails binding，会绕过现有约束。

### Agent/tools 链路复杂度

- `internal/tools/scheduler` 当前不是完整执行 scheduler，没有明显队列、公平性、重试和持久化恢复执行器。
- 工具权限分散在各工具内部，同时还有 scheduler policy wrapper 与 hook approval，新增工具容易漏接权限分类、metadata 或 plan 模式白名单。
- `plan` 模式只允许已知 read-only 工具，新 read-only 工具若未登记会被误挡。
- hooks 并行执行，聚合结果按配置顺序保留；如果 hook 有外部副作用，执行顺序并不等同于配置顺序。
- hooks 的 `updated_input` 是浅合并，复杂嵌套参数改写能力有限。
- skills tracker 按 name 跟踪，不按 skill file path；同名多源诊断粒度有限。

### Legacy 兼容层

- 当前产品入口不直接启动 `internal/server.Server`，该层属于 legacy 兼容路径。
- `internal/server.Server.ListenAndServe` 创建 listener 后未明显赋给 `s.ln`，already-started 判断和 close listener 语义可疑。
- runtime 的 LSP/MCP state 查询有忽略 workspaceID 的路径，多 workspace 隔离边界可疑。
- legacy DELETE workspace 文档写 404，但实现直接调用 DeleteWorkspace，not found 反馈不明显。

## 测试缺口线索

- session 业务除 usage 外测试较少，和 runtime session activity 相比覆盖不均。
- message 删除事件语义未见明确测试。
- projects 并发 register 和 atomic write 未见测试。
- filetracker workspace root 语义未见测试。
- runtimeapi contract 对实际 HTTP route 的覆盖可能不完整。
- 前端普通 TS/React 单测线索少，更多依赖 smoke/harness。
- scheduler 持久化恢复未见直接覆盖。
- permission policy 并发更新未见直接覆盖。
- hooks 与真实 permission/UI 端到端链路未见直接覆盖。
- 新增工具是否自动纳入权限分类/plan 模式 read-only 白名单缺少明显防回归机制。

## 待继续确认

- 每个内置工具的权限参数、输出 metadata、artifact/ref 结构需要逐工具展开。
- 前端各 UI 面板是否完整覆盖后端能力，尤其 capability、MCP request、audit/replay、refs/artifacts、worktree/diff。
- packaged WebView2 的完整点击流和真实模型长任务仍需要按 smoke 脚本复验。
