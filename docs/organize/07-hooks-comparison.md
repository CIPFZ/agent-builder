# Hook 机制梳理对比

本文基于 `docs/organize/` 现有项目梳理，继续聚焦 hook 机制：

- 后端：配置、事件、执行、权限/工具链路、持久化、恢复、审计。
- 前端：配置可见性、运行态展示、timeline/callchain/diagnostics、用户操作入口。

对比参考项目：

- `C:\Users\ytq\work\ai\cc-haha`
- `C:\Users\ytq\work\ai\DeepSeek-GUI`
- `C:\Users\ytq\work\ai\myclaw\claude-code`

## 结论摘要

- Agent Builder 当前 hook 后端基础已经存在，且比早期文档描述更宽。代码中已定义 `PreToolUse`、`PostToolUse`、`PostToolUseFailure`、`UserPromptSubmit`、`PreCompact`、`PostCompact`、`PostSampling`、`Stop`，runtime 也能列出配置、记录 hook execution、进入 audit/replay/recovery/callchain。
- 当前真正查到的主执行链路是工具前后 hook 和 `UserPromptSubmit`。`PreCompact`、`PostCompact`、`PostSampling`、`Stop` 目前更像已进入配置/契约枚举的事件名，未看到同等完整的运行触发链路。
- Agent Builder 的前端展示明显弱于后端能力。用户能在 blocked prompt、React callchain 的 hook count/node、turn diagnostics stop reason 里间接看到 hook，但没有独立 Hooks 设置页、hook execution 列表、运行进度、单次执行详情或配置浏览。
- `myclaw\claude-code` 与 `cc-haha` 的 hook 机制更完整，覆盖 command / prompt / agent / http / callback 多种 hook 类型，事件面包含 session、permission、compact、stop、subagent、file changed、worktree 等，并且有 hook 配置浏览、progress message、attachment message、Stop hook summary 等展示。
- `DeepSeek-GUI` 不是 Claude Code 风格的完整用户 hook 平台。它有轻量 `PreToolUse` / `PostToolUse` tool hook，支持 function hook 和 command hook，能 deny、改 arguments、改 output，但前端主要把结果当普通 tool result / runtime status 展示，没有独立 hook 产品面。
- Agent Builder 后续更适合先补“runtime-first hook visibility”，而不是直接照搬 Claude 系全部事件和 hook 类型。当前已有持久化事实源，前端应该消费 `RuntimeHookExecution`，不要从 tool 文本或前端临时状态推断。

## Agent Builder 当前 Hook 链路

### 后端与 runtime

关键文件：

- `internal/hooks/hooks.go`
- `internal/hooks/runner.go`
- `internal/hooks/input.go`
- `internal/agent/hooked_tool.go`
- `internal/runtime/runtime_hooks.go`
- `internal/runtime/runtime_input.go`
- `internal/runtime/runtime_react_callchain.go`
- `internal/runtime/runtime_turn_diagnostics.go`
- `internal/runtime/runtime_contract_types.go`
- `internal/runtime/runtime_http.go`
- `internal/runtimeapi/contract.go`
- `desktop/runtime_bridge.go`
- `internal/db/migrations/20260524100000_add_runtime_hook_executions.sql`
- `internal/config/config.go`
- `internal/config/load.go`

配置结构：

```go
type HookConfig struct {
	Matcher string `json:"matcher,omitempty"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}
```

配置入口：

- `config.Config.Hooks map[string][]HookConfig`
- global / workspace config merge 后由 `ValidateHooks()` 校验。
- matcher 是正则，执行时按 event + tool name 匹配。

当前代码定义的事件：

- `PreToolUse`
- `PostToolUse`
- `PostToolUseFailure`
- `UserPromptSubmit`
- `PreCompact`
- `PostCompact`
- `PostSampling`
- `Stop`

当前已确认执行链路：

- `PreToolUse`：`internal/agent/hooked_tool.go` 在工具执行前运行。
- `PostToolUse`：工具成功后运行。
- `PostToolUseFailure`：工具返回 error 或 `resp.IsError` 时运行。
- `UserPromptSubmit`：`internal/runtime/runtime_turns.go` / `runtime_input.go` 在 turn 创建前对 normalized input 执行，可 block、rewrite prompt 或 prevent continuation。

当前未确认完整触发链路：

- `PreCompact`
- `PostCompact`
- `PostSampling`
- `Stop`

这些事件已经进入常量、runtime supported event 和配置可见性，但本轮未看到像 tool hook / prompt hook 一样的完整执行入口。后续补齐时应先按代码事实复查，不应只看事件名判断能力完整。

### 执行语义

`internal/hooks.Runner` 的执行模型：

- 按 event 找 hook。
- 按 matcher 匹配 tool name；无 matcher 则匹配全部。
- 按 command 字符串去重。
- 多个 hook 并行执行。
- 聚合结果按配置顺序处理，保证确定性。
- 通过 Agent Builder embedded POSIX shell 执行。
- timeout 默认走 `HookConfig.TimeoutDuration()`；超时或非阻断错误通常 fail-open。

hook stdin payload：

- common：`event`、`session_id`、`cwd`
- tool：`tool_name`、`tool_input`、`tool_output`、`tool_error`
- prompt：`prompt`

hook env：

- `AGENT_BUILDER=1`
- `AGENT=agent-builder`
- `AI_AGENT=agent-builder`
- `AGENT_BUILDER_EVENT`
- `AGENT_BUILDER_TOOL_NAME`
- `AGENT_BUILDER_SESSION_ID`
- `AGENT_BUILDER_CWD`
- `AGENT_BUILDER_PROJECT_DIR`
- `AGENT_BUILDER_TOOL_INPUT_COMMAND`
- `AGENT_BUILDER_TOOL_INPUT_FILE_PATH`
- `AGENT_BUILDER_PROMPT`

输出语义：

- exit `0`：解析 stdout JSON。
- exit `2`：deny/block 当前工具或 prompt。
- exit `49`：halt 整个 turn。
- 其他非零：非阻断错误，记录日志后继续。

stdout JSON 支持：

- `decision`: `allow` / `deny` / none
- `halt`
- `reason`
- `context`
- `updated_input`
- `updated_prompt`
- `prevent_continuation`
- Claude Code 兼容 `hookSpecificOutput.permissionDecision` / `updatedInput`

聚合规则：

- deny 优先于 allow。
- halt 是 sticky。
- reason/context 按配置顺序拼接。
- `updated_input` 是 shallow merge，不是 deep merge。
- `updated_prompt` 使用配置顺序里的最后一个非空值。
- `prevent_continuation` 任一 hook 设置即生效。

### 工具链路行为

`hooked_tool.go` 当前行为：

- top-level agent tools 会被 hook 包装。
- sub-agent 内部工具默认不触发 hook；外层 `agent` 工具调用本身仍可被 hook。
- `PreToolUse` deny 会返回 tool error；halt 会设置 `StopTurn`。
- `PreToolUse` rewrite 会替换 tool input，并记录 `hook.input.rewritten`。
- hook context 会追加到 tool response，让模型可见。
- `PostToolUse` / `PostToolUseFailure` 可追加 context；如果 deny/halt，会把 tool result 标为 error 并停止 turn。
- tool response metadata 会合并 hook metadata，供 UI/diagnostics 使用。

需要注意：

- hook allow 与 permission policy 的优先级需要继续核对。当前文档描述曾说 allow 可跳过 permission prompt；代码侧还存在 permission/scheduler/sandbox 等其他边界，补功能时不能让 hook allow 绕过 deterministic deny、headless fail-closed、sandbox 或 scope。
- 多 hook 并行执行但按配置顺序聚合；如果 hook 有外部副作用，实际副作用顺序并不等同于配置顺序。
- `updated_input` 是浅合并，复杂嵌套参数改写能力有限。

### 持久化、事件、审计和恢复

runtime 持久化表：

- `runtime_hook_executions`

主要字段：

- identity：`id`、`hook_id`、`hook_name`、`hook_source`
- scope：`session_id`、`turn_id`、`tool_call_id`、`task_id`
- capability：`capability_id`、`mcp_server`、`skill`
- policy/sandbox：`policy_mode`、`policy_profile`、`policy_rule`、`policy_decision`、`sandbox_decision_id`、`sandbox_status`
- result：`status`、`reason`、`error`、`input_summary`、`output_summary`、`context_summary`
- flags：`input_rewritten`、`context_injected`、`redacted`
- timing：`started_at`、`completed_at`、`duration_ms`

runtime event 类型：

- `hook.discovered`
- `hook.configured`
- `hook.execution.started`
- `hook.execution.completed`
- `hook.execution.skipped`
- `hook.execution.blocked`
- `hook.execution.failed`
- `hook.context.injected`
- `hook.input.rewritten`

runtime API：

- `Hooks(ctx)`：列出配置 hook。
- `HookExecutions(ctx, req)`：按 session / turn / tool call / task / event / status 查询执行记录。
- `HookExecution(ctx, executionID)`：查单条执行记录。
- HTTP：`GET /v1/hooks`
- HTTP：`GET /v1/hook-executions`
- HTTP：`GET /v1/hook-executions/{id}`
- Wails bridge 已有同名转发。

恢复和导出：

- runtime recovery status 包含 `hook_executions`。
- replay export summary 包含 `hooks`。
- audit turn summary 包含 `hooks`。
- runtime 启动恢复会把 running hook 标记为 failed/interrupted。

### 前端现状

关键文件：

- `client/src/runtime/workbenchTypes.ts`
- `client/src/runtime/wailsWorkbenchAdapter.ts`
- `client/src/features/diagnostics/ReactCallchainInspector.tsx`
- `client/src/features/diagnostics/TurnDiagnosticsPanel.tsx`
- `client/src/features/settings/SettingsPanel.tsx`
- `client/src/features/timeline/Timeline.tsx`

已实现展示：

- `RuntimeNormalizedInput.hookOutcome` 会被 adapter 映射为一条 assistant error message，例如 prompt 被 `UserPromptSubmit` hook blocked。
- `ReactCallchainViewModel` 有 `hookCount`、`usesHooks`、`hookExecutionId`。
- `ReactCallchainInspector` 显示 hook 数量，并把 hook node 当通用 callchain node 展示。
- `TurnDiagnosticsPanel` 能通过 stop reason 间接显示 hook halt。
- timeline 可通过普通 tool result / diagnostic 间接看到 hook block 后果。

当前明显缺口：

- 没有 `HookViewModel` / `HookExecutionViewModel`。
- adapter 没有 hydrate hooks / hook executions 到主 view model。
- Settings 没有 Hooks 配置浏览或状态页。
- Diagnostics 没有 Hook executions 面板。
- Timeline 没有一等 hook row/card，无法清楚展示 PreToolUse、PostToolUse、input rewrite、context injected、blocked、failed。
- 没有 hook progress 展示；用户只能看到执行后的聚合结果。
- `HookExecution(ctx, id)` 虽有后端 API，但前端没有详情入口。
- `docs/hooks/README.md` 仍写“当前只支持 PreToolUse”，与当前代码事实不一致。

## 参考项目对比

### `myclaw\claude-code`

关键文件：

- `src/entrypoints/sdk/coreTypes.ts`
- `src/entrypoints/sdk/coreSchemas.ts`
- `src/types/hooks.ts`
- `src/schemas/hooks.ts`
- `src/utils/hooks.ts`
- `src/utils/hooks/*`
- `src/services/tools/toolHooks.ts`
- `src/query/stopHooks.ts`
- `src/components/hooks/HooksConfigMenu.tsx`
- `src/components/hooks/SelectEventMode.tsx`
- `src/components/hooks/SelectMatcherMode.tsx`
- `src/components/hooks/SelectHookMode.tsx`
- `src/components/hooks/ViewHookMode.tsx`
- `src/components/messages/HookProgressMessage.tsx`
- `src/utils/messages.ts`
- `src/commands/hooks/hooks.tsx`
- `docs/25-hooks-and-runtime-extensibility.md`

事件面：

- `PreToolUse`
- `PostToolUse`
- `PostToolUseFailure`
- `Notification`
- `UserPromptSubmit`
- `SessionStart`
- `SessionEnd`
- `Stop`
- `StopFailure`
- `SubagentStart`
- `SubagentStop`
- `PreCompact`
- `PostCompact`
- `PermissionRequest`
- `PermissionDenied`
- `Setup`
- `TeammateIdle`
- `TaskCreated`
- `TaskCompleted`
- `Elicitation`
- `ElicitationResult`
- `ConfigChange`
- `WorktreeCreate`
- `WorktreeRemove`
- `InstructionsLoaded`
- `CwdChanged`
- `FileChanged`

hook 类型：

- command hook
- prompt hook
- agent hook
- http hook
- callback / function hook
- plugin hook
- session hook

可借鉴点：

- Hook event metadata 很完整，能解释每个 event 的触发时机、输入字段、exit code 语义和 matcher 字段。
- 配置浏览器按 event -> matcher -> hook drill down，且是 read-only，避免复制 settings 编辑器复杂度。
- Hook progress 是消息流的一部分，可显示 running / response / error。
- Hook 输出会转成 attachment：blocking error、additional context、cancelled、non-blocking error、stopped continuation、success。
- Stop hook 有 summary message，能汇总 hook count、command、duration、error、是否阻止继续。
- PermissionRequest hook 能在 headless/SDK 场景替代用户交互。
- session hook 是内存态，适合临时规则，不必都落配置。

不宜照搬点：

- 它是 CLI/TUI/SDK 主路径，UI 组件和 transcript 结构不能直接搬进 Agent Builder React desktop。
- 事件面非常宽，直接全量扩入会显著扩大 runtime、permission、worktree、MCP、compact、recovery 和 UI contract。
- 它的部分 hook 类型依赖 TS runtime 内部 AppState / callback 能力，Agent Builder 应继续保持 Go runtime 为事实来源。

### `cc-haha`

关键文件：

- `src/types/hooks.ts`
- `src/schemas/hooks.ts`
- `src/utils/hooks/*`
- `src/services/tools/toolHooks.ts`
- `src/query/stopHooks.ts`
- `src/components/hooks/HooksConfigMenu.tsx`
- `src/components/messages/HookProgressMessage.tsx`
- `desktop/src/pages/Settings.tsx`
- `desktop/src/pages/DiagnosticsSettings.tsx`
- `desktop/src/components/chat/ToolCallBlock.tsx`
- `desktop/src/components/trace/detail/ToolDetail.tsx`

整体判断：

- `cc-haha` 的 hook 核心与 `myclaw\claude-code` 基本同源，但额外存在 desktop UI、trace/detail、settings pages 等桌面化线索。
- CLI/TUI 侧 hook 配置浏览仍然成熟；desktop 侧更多是工具 trace 和设置页承载，未看到像 Agent Builder runtime hook execution store 这样的 Go 持久化边界。

可借鉴点：

- Hook 配置浏览做成 read-only 比直接做完整编辑器更稳。
- Hook progress 和 hook attachment 不必挤进权限弹窗，可作为消息/trace 的一部分。
- Tool detail / trace detail 是桌面客户端承载 hook execution 明细的自然位置。
- plugin hook、builtin hook、managed policy disabled hooks 的状态表达值得借鉴。

不宜照搬点：

- 不应复制其具体 UI 文案、品牌、样式或 TUI 交互。
- 它从 TS AppState 和消息流组织 hook 展示；Agent Builder 应从 `RuntimeHookExecution` hydrate。

### `DeepSeek-GUI`

关键文件：

- `kun/src/adapters/tool/tool-hooks.ts`
- `kun/src/adapters/tool/local-tool-host.ts`
- `kun/src/loop/agent-loop.ts`
- `kun/src/contracts/events.ts`
- `src/renderer/src/agent/kun-mapper.ts`
- `src/renderer/src/agent/types.ts`
- `src/renderer/src/components/chat/message-timeline-tools.ts`
- `src/renderer/src/components/chat/message-timeline-process.tsx`

当前能力：

- 只有轻量 tool hook：`PreToolUse` / `PostToolUse`。
- hook 可按 `toolNames` 匹配。
- 支持 function hook 和 command hook。
- `PreToolUse` 可 deny 或替换 arguments。
- `PostToolUse` 可替换 output / isError。
- command hook stdin 是 invocation JSON，非零退出在 PreToolUse 下会 deny，PostToolUse 下会标记 error。
- timeout 默认 5 秒。
- hook 结果作为普通 tool result/error 持久化到 turn item。

前端展示：

- 没有独立 hook UI。
- hook deny / failed 会表现为 tool error。
- renderer 侧通过 tool event、runtime status、process timeline 展示运行过程。
- 更强的可见 runtime status 是 `tool_storm_suppressed`、approval、user_input、compaction，而不是 hook 专属事件。

可借鉴点：

- 轻量 hook API 很克制，适合内建 runtime hygiene。
- hook 与 approval、read-before-edit、tool storm breaker、rate limit normalize 串在工具 host 中，边界清楚。
- hook 失败作为结构化 tool result code，例如 `hook_failed` / `hook_denied`，便于前端当普通 tool error 处理。

不宜照搬点：

- 它不是完整用户扩展体系，缺少配置管理、事件广度、审计和 hook execution detail。
- 它没有 Agent Builder 需要的 runtime-first durable hook execution 查询面。

## 对比矩阵

| 维度 | Agent Builder | myclaw/claude-code | cc-haha | DeepSeek-GUI |
| --- | --- | --- | --- | --- |
| 主要定位 | Go runtime-owned hook execution | CLI/TUI/SDK runtime extension | Claude 系 hook + 桌面 trace/settings 线索 | Tool host 内建轻量 hook |
| Hook 类型 | command shell hook | command / prompt / agent / http / callback / plugin / session | 同 Claude 系，另有桌面化承载 | function / command |
| 当前确认执行事件 | PreToolUse、PostToolUse、PostToolUseFailure、UserPromptSubmit | 事件面很宽，覆盖 tool、prompt、session、stop、compact、permission、subagent 等 | 基本同 Claude 系 | PreToolUse、PostToolUse |
| 配置结构 | `hooks[event][]HookConfig`，matcher + command + timeout | `hooks[event][]matcher{hooks[]}`，支持多 hook 类型 | 同 Claude 系 | `ResolvedToolHook[]` 注入 ToolHost |
| 匹配方式 | regex 匹配 tool name | event metadata 定义 matcher 字段，可匹配 tool/source/error/trigger 等 | 同 Claude 系 | toolNames 精确匹配 |
| 执行并发 | matching hooks 并行，结果按配置顺序聚合 | 多 hook 执行并进入消息/progress 系统 | 同 Claude 系 | 当前顺序执行 matching hooks |
| 权限关系 | hook 可 deny/allow/rewrite；与 policy/sandbox 关系需继续核对 | hook allow 不应绕过 deny/ask 等规则，PermissionRequest hook 可替代交互 | 同 Claude 系 | hook 在 approval 前运行，可 deny 或改 args |
| 持久化 | SQLite `runtime_hook_executions` | transcript/message/state 为主 | TS state/message + desktop trace | turn item/tool result |
| Runtime event | `hook.*` 事件完整 | hook progress/response event | hook progress/response event | 无 hook 专属前端事件 |
| Audit/replay/recovery | 已接入 | 主要 transcript/SDK/message 语义 | 类似 Claude 系 | 普通 thread/turn item 恢复 |
| 前端配置浏览 | 缺失 | `HooksConfigMenu` | `HooksConfigMenu`，desktop settings 线索 | 缺失 |
| 前端运行展示 | blocked prompt、callchain hook count/node、stop reason | progress message、attachment、Stop summary、transcript | 同 Claude 系 + desktop trace/detail 线索 | 普通 tool error/status |
| 单次 hook 详情 | 后端 API 有，前端缺失 | transcript/attachment/verbose | trace/detail 可借鉴 | 无专门详情 |
| 产品化成熟度 | 后端强，前端弱 | 成熟但偏 CLI/TUI | 成熟且有桌面方向线索 | 克制、轻量、非完整平台 |

## Agent Builder 当前缺口

### 后端缺口

- `docs/hooks/README.md` 与代码事实不一致，仍说当前只支持 `PreToolUse`。
- 事件名已支持 `PreCompact`、`PostCompact`、`PostSampling`、`Stop`，但未确认完整触发链路。
- hook execution 是 aggregate 记录，不是每个具体 hook command 的一条完整 execution；`AggregateResult.Hooks` 只进入 metadata，持久化明细粒度有限。
- hook allow、permission policy、headless、sandbox、scope 的优先级需要明确测试覆盖。
- hook timeout / fail-open / non-blocking error 对 UI 的可见性不足。
- `updated_input` 浅合并能力有限，需要文档明确，避免用户误以为是 deep merge。
- subagent 内部工具不触发 hook 是当前设计，需要在 UI 和文档中说明。

### 前端缺口

- 缺少 Hooks 配置浏览：用户无法看到当前启用了哪些 hook、来自哪个 event、matcher 是什么、是否被禁用。
- 缺少 Hook executions 面板：用户无法按 session / turn / tool call / event / status 查看历史执行。
- callchain 只显示 hook count 和通用 node，没有展示 event、decision、reason、input rewritten、context injected、duration。
- timeline 没有 hook row/card；hook block、rewrite、context 注入只能从 tool error 或诊断里间接推断。
- Settings 中没有 hook 状态，也没有提示 hook 文档与配置文件位置。
- 没有 hook progress；长 hook 运行时用户不知道卡在 hook、工具还是模型。
- 没有从 hook execution 详情跳转到关联 tool call / permission / audit / replay 的入口。

## 推荐补齐顺序

### 第一阶段：补前端可见性，不改执行语义

目标是把已有 runtime 事实展示出来：

- 新增 `HookViewModel` / `HookExecutionViewModel`。
- adapter 增加 `Hooks`、`HookExecutions`、`HookExecution` DTO mapper。
- Workbench view model 增加当前 session/turn hook execution summary。
- Settings 增加只读 Hooks 页：event、matcher、command preview、timeout、status、diagnostics。
- Diagnostics 增加 Hook executions panel：按 event/status 筛选，展示 reason/error/duration/input rewritten/context injected。
- React callchain hook node 展示 event、decision/status、duration、reason，不只显示通用 node。

### 第二阶段：补 timeline 和 tool detail 展示

目标是让普通用户能理解 hook 为什么影响了这次 turn：

- timeline 中为 blocked/failed/context injected/input rewritten hook 增加轻量 row。
- tool call card 内展示 hook badge：pre/post、blocked、rewritten、context。
- hook row 可打开详情，详情从 `HookExecution(ctx, id)` hydrate。
- prompt 被 `UserPromptSubmit` hook block 时，保留当前错误消息，同时提供“查看 hook”详情入口。

### 第三阶段：补契约和测试

目标是防止 hook 绕过安全边界或 UI 漂移：

- 增加 runtimeapi contract 对 `/v1/hooks`、`/v1/hook-executions`、`/v1/hook-executions/{id}` 的覆盖。
- 增加 scenario tests：hook allow 不能绕过 deterministic deny、headless fail-closed、sandbox、scope。
- 增加 prompt submit hook block / rewrite / prevent continuation 的 frontend smoke。
- 增加 post tool hook halt / context injected 的 callchain 和 diagnostics 测试。
- 更新 `docs/hooks/README.md`，把已实现、已枚举但未触发、未来事件分开。

### 第四阶段：再考虑扩展事件和 hook 类型

建议先小步扩展：

- 完整落地 `Stop` hook：turn 结束前运行，记录 durable execution，前端给 summary。
- 完整落地 `PreCompact` / `PostCompact`：接入 compact boundary、replay、recovery。
- 考虑 `PermissionRequest` hook：用于 headless/automation 场景替代人工权限决策，但必须经过 policy/sandbox/scope 兜底。

暂缓项：

- prompt hook / agent hook / http hook 作为持久配置类型。
- session-only function hook。
- file watcher hook。
- worktree create/remove hook。
- teammate/task hook。

这些能力会扩大 runtime 执行面和安全面，应单独设计，不应因为参考项目存在就一次性照搬。

## 后续修改注意事项

- Hook 执行事实应继续归 Go runtime 所有，React 只消费 adapter view model。
- 新增前端能力必须走 `WorkbenchAdapter`，不要直接调用 Wails binding 或裸 `fetch`。
- Hook 展示应优先使用现有 Ant Design、theme tokens 和 CSS Modules。
- 不要把参考项目的 TUI transcript 结构直接搬到 React；应映射为桌面诊断、timeline 和详情面板。
- 不要复制参考项目品牌、文案或视觉，只吸收事件组织、配置浏览和运行态可见性设计。
- 任何 hook allow / auto-approve 相关改动都必须先明确权限、sandbox、scope 的优先级。
