# 上下文压缩第一阶段实施方案

## 1. 目标与范围

本方案实现 Agent Builder 第一阶段上下文压缩能力：

1. Tool Result Budget；
2. Provider-neutral Time-based Microcompact；
3. Auto Compact；
4. Session Memory 生成与 Session Memory Compact；
5. LLM Full Compact；
6. Provider Context Too Long 的 Reactive Recovery。

本阶段不实现 Snip Compact 和 Context Collapse，也不增加 Anthropic
`cache_reference/cache_edits`。所有压缩均作用于发送给 Provider 的模型投影，
SQLite 中的 canonical Message、ToolCall 和 ToolResult 保持完整、可审计、可恢复。

Claude Code 只作为机制参考。Agent Builder 的实现必须遵守以下边界：

- Runtime 是压缩决策和持久化的权威来源；
- `internal/contextmgr` 提供 Provider-neutral 的预算、投影算法和存储能力；
- `internal/agent` 只负责 Agent 循环、模型辅助调用和调用 Runtime 投影接口；
- Provider Adapter 只负责把规范化消息转换成各 Provider 的合法请求；
- React 不保存压缩状态，也不参与阈值判断；
- 大型原始内容进入 Runtime Object Store，不能只生成一个无法解析的伪 URI。

## 2. 当前实现基线

当前已有能力：

- `internal/agent/tool_result_guard.go` 已支持单结果截断和 Runtime Object 持久化；
- `internal/contextmgr/budget.go` 另有一套投影阶段 Tool Result Budget，但产品主链路未传入配置；
- `internal/contextmgr/microcompact.go` 已支持 Token-pressure Microcompact 和 replacement 持久化；
- `internal/runtime/runtime_context_usage.go` 已实现真实 usage 锚点、估算补充和
  `20k/13k/20k/3k` 阈值公式；
- `internal/runtime/runtime_prompt_assembly.go` 已实现手动、自动和 Reactive Full Compact；
- `internal/agent/compact_summarizer.go` 已实现模型摘要、图片清理、20k 输出上限和
  Prompt Too Long 重试；
- `runtime_context_*` 表已经记录 Projection、Boundary、Replacement、Warning 和
  Reactive Attempt；
- `sessions.summary_message_id` 已作为当前 Full Compact 的历史切片锚点。

第一阶段启动时的基线问题（已处理）：

- Tool Result Budget 有两套不一致的控制面；
- 当前 Microcompact 不是 Time-based，且 replacement 的原始引用不一定是可读取 Object；
- `contextmgr` 中的 Auto/Full/Snip 与 Runtime 产品实现重复，但主链路没有使用；
- Full Compact 模型失败后会静默使用启发式摘要；
- `summaryModel` 已配置但 Runtime 没有传入 `UseSmallModel`；
- 没有 Session Memory 持久化、提取器和压缩策略；
- `session.SummaryMessageID` 只能表达“摘要后全部丢弃”，不能表达
  “摘要 + 原样保留的近期消息”；
- Reactive Attempt 1 的行为是 Token-pressure Microcompact，不是本阶段确定的
  Time-based 策略复用；
- Context Governance 的桌面保存路径仍使用 JSON ConfigStore，与项目当前 SQLite
  权威存储约束不一致。

实施状态（2026-08-02）：上述条目是方案编写时的旧实现基线，不是当前待办。
WP0–WP8 已完成单一 Runtime 控制面、Time-based Microcompact、Runtime Object Ref、
严格 Full Compact 失败语义、`summaryModel` 路由、Session Memory、preserved tail Boundary、
Reactive Recovery 和 SQLite-only Context Governance。对应实施修订记录在 11.1 和 11.2；
这两节描述的是已同步进代码的决策，不是未完成问题。

### 2.1 Claude Code 参考点与本项目落点

审查输入来自 `cc-compact.txt` 和 `learn-harness/src`。下表只映射机制，不把 Claude Code
的 Provider、远程实验开关或文件存储实现直接搬入 Agent Builder：

| 能力 | Claude Code 参考代码 | 可复用机制 | Agent Builder 的差异化落点 |
|---|---|---|---|
| Tool Result Budget | `constants/toolLimits.ts`、`utils/toolResultStorage.ts` | 单结果 50k 字符、同一 API message/round 聚合 200k、冻结替换决策 | Runtime Object Store 保存原文；Agent Guard 只生成预览；不写 Session 私有工具结果文件 |
| Time-based Microcompact | `services/compact/timeBasedMCConfig.ts`、`microCompact.ts` | 主线程、API 调用前、空闲 60 分钟、保留最近 5 个 eligible 结果 | 本地配置而非 GrowthBook；Provider-neutral replacement；不实现 `cache_reference/cache_edits` |
| Auto Compact | `services/compact/autoCompact.ts` | 输出预留、阈值、递归保护、单 Turn 状态、三次失败熔断 | 使用 Runtime 的统一 Budget Report；小窗口采用安全缩放；状态持久化/可诊断 |
| Session Memory 生成 | `services/SessionMemory/sessionMemory.ts`、`sessionMemoryUtils.ts` | 首次 10k、增长 5k、3 个 ToolCall 或自然边界、后台单飞 | append/revision SQLite Store；不使用模块级状态和 `summary.md` 作为权威来源 |
| Session Memory Compact | `services/compact/sessionMemoryCompact.ts` | 等待提取 15 秒、10k/5 条文本/40k tail、配对安全 | 持久化 Boundary projector 重建 `memory summary + tail + new messages` |
| LLM Full Compact | `services/compact/compact.ts`、`autoCompact.ts` | Microcompact 后摘要、Prompt Too Long 有界重试、reinjection、cleanup | 摘要失败就是失败；不接受启发式摘要提交；Runtime 事务提交 Summary 和 Boundary |
| Reactive Recovery | `services/compact/reactiveCompact.ts`、`query.ts` | 仅 Context Too Long 触发、先局部缩减再 Full Compact、有界重试 | Attempt 1 复用 Provider-neutral reducer；Attempt 2 先 Session Memory 再 Full Compact |

明确不照搬：

- Claude Code 的 Cached Microcompact 是 Anthropic cache editing 协议能力；
- Claude Code 的 Session Memory 文件与进程级锚点不满足本项目跨重启事务恢复要求；
- Claude Code 的工具名静态集合在本项目改为 Tool capability/category；
- Claude Code 的 GrowthBook/环境变量实验开关在本项目改为 SQLite 配置和 Runtime resolved policy；
- Snip Compact 和 Context Collapse 已完成评估，但不进入本阶段控制链路。

## 3. 目标控制链路

每次主线程 Provider 调用前只经过一个 Runtime 控制器：

```text
Canonical session history
  -> restore latest completed compact-boundary projection
  -> history hygiene / tool pairing validation
  -> apply frozen Tool Result Budget decisions
  -> apply Time-based Microcompact when idle threshold is met
  -> calculate one shared context budget
  -> if below auto threshold: call Provider
  -> if above auto threshold:
       try Session Memory Compact
       -> if unavailable/invalid/ineffective: LLM Full Compact
  -> persist Projection + Boundary/Replacement/Warning evidence
  -> call Provider
```

Provider 返回 Context Too Long 后：

```text
Attempt 1: force the same local tool-result reducer with reactive policy
Attempt 2: try Session Memory Compact, then LLM Full Compact fallback
Attempt 3: only when an eligible ToolResult still provides safe projection reduction,
           run the shared reducer once more; never regenerate an equivalent LLM summary
Circuit breaker: three consecutive auto/reactive full-compact failures per session
```

Time-based 和 Reactive 使用同一个本地 Microcompact 投影器，但触发原因和策略参数不同。
不为 Reactive 再创建第二套消息替换实现。

实施修订（2026-07-30）：当前 Agent retry 回调发生在每次 Provider context-length 错误之后；
Attempt 2 若已成功提交 Full/Session Memory Boundary，再次生成等价 Full Summary 不会提供有界收益，
并会额外消耗摘要模型。因此 Attempt 3 按 WP7“仅有安全缩减空间才重试”的约束收口为最终一次
Provider-neutral reducer；无 eligible ToolResult 时直接结束恢复。此修订替代上方基线中原先的
“再次 LLM Full Compact”描述，不改变最多三次 Provider 重试和三次 Full Compact 失败熔断语义。

## 4. 核心设计决定

### 4.1 Canonical history 与 Provider projection 分离

任何压缩均不得删除或覆盖 canonical message。压缩结果通过以下数据表达：

- Compact Boundary；
- Summary Message；
- preserved message references；
- Content Replacement；
- Reinjection references；
- Projection 及其选中消息。

UI 继续显示完整会话；模型只接收最新有效投影。

### 4.2 用持久化 Boundary 投影替代单一 SummaryMessageID 切片

Session Memory 只总结到 `last_summarized_message_id`，其后的近期消息必须原样保留。
因此不能继续依赖：

```go
messages = messages[summaryMessageIndex:]
```

目标 Boundary 需要同时记录：

```text
summary_message_id
summarized_message_refs
preserved_message_refs
boundary_cutoff_message_id
reinjected_refs
```

恢复后的模型历史为：

```text
leading system context
+ summary message
+ preserved canonical messages in recorded order
+ canonical messages created after boundary cutoff
```

需要在 Agent 和 Runtime 之间增加 `SessionHistoryProjector`（名称可在实现时调整）接口。
Agent 获取 canonical session messages 后，先让 Runtime 根据最新 completed boundary
构建历史投影，再转换为 Provider messages。`sessions.summary_message_id` 在迁移期间保留，
但不再作为唯一恢复算法。

### 4.3 单一预算来源

所有阈值判断统一使用 Runtime 的 `ContextBudgetResolver`：

- 最后一条有效 Assistant Provider usage 作为真实锚点；
- 锚点之后使用本地估算；
- System、Tools、Skills、MCP、Memory、附件和输出预留纳入同一报告；
- Provider 没有 usage 时全部使用估算并标记 `estimated=true`；
- 同一请求中的 Auto Compact、警告、Session Memory 有效性检查使用同一份报告。

禁止在 `contextmgr`、Runtime 和前端分别维护不同阈值公式。

### 4.4 Compact 成功语义

只有以下条件全部满足才能提交 completed boundary：

- 摘要非空且通过格式/长度校验；
- Summary Message 已创建；
- preserved refs 和 reinjection refs 已确定；
- 压缩后的预算低于安全阈值；
- Summary、Boundary 和必要锚点在一个数据库事务中提交。

模型摘要失败、空摘要、超时或验证失败都属于 Compact 失败。启发式摘要不得更新
Session anchor，也不得伪装成成功的 compact。

## 5. 配置契约

沿用现有 `contextGovernance` 配置契约，增加 Time-based 和 Session Memory 字段。
建议第一版逻辑配置形状：

```json
{
  "contextGovernance": {
    "autoCompactEnabled": true,
    "autoCompactPercent": null,
    "microcompactEnabled": true,
    "microcompactIdleMinutes": 60,
    "microcompactKeepRecent": 5,
    "sessionMemoryEnabled": true,
    "summaryModel": "session"
  },
  "options": {
    "tool_result_guard": {
      "enabled": true,
      "max_result_chars": 50000,
      "turn_budget": 200000,
      "exempt_tools": ["view"]
    }
  }
}
```

决定：

- `microcompactIdleMinutes` 默认 60，最小 1；
- `microcompactKeepRecent` 默认 5，最小 1；
- `sessionMemoryEnabled` 默认 true；
- `summaryModel=session|small` 必须真正控制摘要模型；
- Tool Result 默认单结果阈值先与 Claude Code 对齐为 50,000 字符；
- 聚合预算保持 200,000 字符；
- 原有配置缺省时自动获得新默认值，不要求用户迁移文件；
- Provider/model override 第一阶段只继续覆盖 Auto Compact 百分比，避免配置矩阵失控。

配置来源：

- `internal/config` 继续提供配置类型、默认值和校验；文件读写只服务 CLI/遗留适配入口；
- 桌面产品只从 SQLite `application_settings` 读取和保存 Context Governance，不能同时读取
  JSON ConfigStore 后再用 SQLite 覆盖；
- 桌面 Runtime 的解析顺序为内置默认值 -> SQLite 设置；
- 现有 `SaveContextGovernanceSettings` 的 ConfigStore 桌面写入必须在 WP0 迁移到 SQLite；
- 本阶段不在普通设置 UI 暴露内部阈值，但诊断 DTO 返回最终 resolved 值。

## 6. 持久化设计

### 6.1 扩展 Compact Boundary

在 `runtime_context_boundaries` 增加：

```text
preserved_message_refs_json TEXT
boundary_cutoff_message_id TEXT
summary_mode TEXT
```

`message_refs_json` 明确改名义为 summarized message refs；为避免破坏旧数据库，
第一阶段保留列名，只在 Go DTO 和文档中澄清含义。

Boundary Kind：

- `full`：LLM Full Compact；
- `session_memory`：Session Memory Compact；
- `micro`：Microcompact evidence。

最新历史边界查询必须同时识别 `full` 和 `session_memory`。

### 6.2 Session Memory Revision

新增 `runtime_session_memory_revisions`：

```text
id                         TEXT PRIMARY KEY
session_id                 TEXT NOT NULL
turn_id                    TEXT
revision                   INTEGER NOT NULL
status                     TEXT NOT NULL  -- started/completed/failed
base_revision              INTEGER
content                    TEXT
content_hash               TEXT
last_summarized_message_id TEXT
source_message_count       INTEGER NOT NULL DEFAULT 0
source_token_estimate      INTEGER NOT NULL DEFAULT 0
source_tool_call_count     INTEGER NOT NULL DEFAULT 0
provider                   TEXT
model                      TEXT
created_at                 INTEGER NOT NULL
completed_at               INTEGER
error                      TEXT
```

索引/约束：

- unique `(session_id, revision)`；
- index `(session_id, status, revision)`；
- 读取时只选择最新 `completed` revision；
- started/failed revision 永远不能覆盖最后一个 completed revision。

Session Memory 正文预计小于 20k Token，第一阶段直接存 SQLite，便于事务读取和恢复。
如果后续真实数据证明正文显著膨胀，再迁移到 Object Store，不预先增加双存储。

### 6.3 Replacement 唯一性

Content Replacement 的稳定键调整为：

```text
(session_id, tool_call_id, kind)
```

其中 `kind` 至少区分：

- `tool_result_budget`；
- `microcompact`。

避免当前仅按 ToolCall ID 查询导致不同机制复用错误 replacement。

所有 `original_ref` 必须是实际存在的 `runtime://objects/...`，提交 replacement 前验证
Object 已成功写入。

## 7. 分阶段工作包

### WP0：冻结契约并统一控制面

目标：先消除“代码存在但产品未使用”的歧义。

实施：

- 在 `runtime_prompt_assembly.go` 建立唯一的压缩编排入口；
- `contextmgr` 只保留纯投影算法、预算类型和 Store；
- Runtime 负责模型调用、Session mutation、事务、事件和熔断；
- 将桌面 `ContextGovernanceSettings/SaveContextGovernanceSettings` 从 JSON ConfigStore
  迁移到 SQLite `application_settings`，保持逻辑 DTO，不建立双写或双读兼容层；
- 同时冻结 `ContextCompactionStatus`、扩展 Boundary、Session Memory 状态和 compact event
  payload 的 Runtime/Wails DTO，避免后端完成后再由前端反推契约；
- 移除或停止导出未接入的 `contextmgr` Auto/Full/Snip 操作；
- `ManualSnip` 不再作为可调用产品能力；历史 Snip 表先保留，不在本阶段做破坏性迁移；
- 增加 `SessionHistoryProjector`，为可恢复 Boundary 投影建立接口；
- 为当前 Full Compact 补写 preserved refs/cutoff，使重启前后投影一致。

验收：

- 代码中只有一个 Auto/Full Compact 编排入口；
- 关闭所有压缩配置时 Provider 输入与当前行为一致；
- Full Compact 后重启，Provider 输入与重启前一致；
- UI canonical timeline 不丢失或重复消息。

### WP1：统一 Tool Result Budget

目标：形成一个策略、两个执行点。

执行点 A——工具结果进入 Runtime：

- Runtime Scheduler/Recorder 负责把完整的大型结果写入项目 Object Store；
- Agent `ToolResultGuard` 只基于已经持久化的真实 Object Ref 生成有界模型预览；
- canonical ToolResult 保存真实 Object Ref、有界预览、原始大小和截断原因；
- 持久化失败时使用明确的 fallback preview，并产生 warning，不能伪造 Object URI。

执行点 B——Provider 投影：

- 对同一个 API round 中的 ToolResult 计算 200k 聚合预算；
- 使用冻结 replacement 决策，保证重试和 Prompt Cache 前缀稳定；
- 已经由执行点 A 缩减的结果不重复落盘；
- 预算超过时按确定性顺序选择需要替换的结果；
- Read/View 豁免由工具能力声明提供，不在算法中写死字符串分支。

需要重点修正：

- `ToolResultGuard.ResetTurn` 的语义改为 API round，而不是含混的 Turn；
- 消除 turn-budget 路径的重复计数；
- 共享 preview builder、UTF-8 安全切片和 token estimator；
- 接通 `contextmgr.applyToolResultBudget` 或用统一实现替换它，不能继续双轨。

验收：

- 50k 单结果、200k 聚合边界测试；
- Unicode、错误尾部、JSON 尾部和媒体结果测试；
- Object 写入失败测试；
- 重试、重启、Resume 后 replacement 稳定；
- canonical Object 可通过 Ref 读取完整内容；
- 不产生第二份同内容 Object。

### WP2：Time-based Microcompact

目标：只实现 Provider-neutral 的本地消息投影清理。

触发：

- 仅 main-turn Provider 调用；
- `now - last_completed_main_assistant_at >= idle interval`；
- 默认 60 分钟；
- helper、summary、compact 和 subagent 调用不触发；
- 每个 idle period 只触发一次，新的 Assistant completion 重新计时。

行为：

- 默认保留最近 5 个 eligible ToolResult；
- 仅清理有可靠恢复来源的 ToolResult；
- 清理前确保原始结果已有 Runtime Object；
- Provider 投影替换为固定短文本和真实 Object Ref；
- canonical message 不变；
- Boundary 记录触发时间、替换 ToolCall、节省 Token 和 resolved 配置；
- Full/Session Memory Compact 后清理失效的 Microcompact tracking state。

Eligible tool 使用工具 capability/category 决定，初始覆盖文件读取、搜索、Shell、Web、
Edit/Write 类工具；错误、权限拒绝和仍在运行的工具结果不清理。

Reactive Recovery 可以调用同一 reducer，并传入 `trigger=reactive`、`keepRecent=1`；
这不是第二种 Cached Microcompact。

验收：

- 59 分钟不触发、60 分钟触发；
- KeepRecent、工具 allowlist、主/子线程隔离；
- canonical message 完全不变；
- tool-use/result 配对不被破坏；
- 重启后投影重放稳定；
- OpenAI、Anthropic、Gemini 风格 Adapter 合法性测试。

### WP3：统一 Auto Compact 与预算判断

目标：Auto Compact 只负责决策，不包含第二套摘要算法。

实施：

- 提取 `ContextBudgetResolver`；
- 保留 `outputReserve=min(maxOutput, 20k)`；
- 保留安全公式 `effectiveWindow - min(13k, contextWindow/10)`；
- warning、blocking、auto threshold 共用一个报告；
- 显式百分比只能提前触发，不能突破安全上限；
- 每个 Turn 最多主动 Auto Compact 一次；
- helper/summary/compact 调用使用 recursion guard；
- Auto Compact 依次尝试 Session Memory -> LLM Full；
- 压缩后重新计算实际投影预算，仍不安全则视为 ineffective，不提交错误锚点；
- 三次连续 auto/reactive Full Compact 失败打开 Session 熔断；
- 手动 Compact 重置熔断。

验收：

- 有/无真实 usage 的阈值测试；
- 不同 Context Window/MaxOutput 模型测试；
- 小窗口、未知模型 metadata 和百分比覆盖测试；
- helper recursion、每 Turn 一次、熔断和手动重置测试；
- 前后预算来自真实 Provider projection，而不是 canonical 全历史。

### WP4：Session Memory Store 与后台提取器

目标：先持续生成可靠 Memory，再让 Compact 消费它。

默认调度参考 Claude Code：

- 上下文达到 10k Token 后允许首次提取；
- 相比上次 completed revision 至少增长 5k Token；
- 增长阈值始终必须满足；
- 同时满足新增 3 个 ToolCall，或当前 Assistant Turn 没有 ToolCall、处于自然边界；
- 每个 Session 同时只允许一个 extraction；
- 只在 completed main Assistant step 后异步启动，不阻塞回答展示。

提取输入：

- 上一个 completed Session Memory；
- 上次 `last_summarized_message_id` 之后的 canonical messages；
- 必要的用户纠正、ToolResult Ref 和状态信息；
- 不携带工具 schema，不允许工具调用；
- 使用 `summaryModel` 选择 session model 或 small model。

输出采用稳定 Markdown 结构：

```text
Current objective
User requirements and corrections
Decisions and rationale
Current state
Files, symbols, commands and identifiers
Errors and resolutions
Pending work
Exact next steps
```

质量规则：

- 输出非空且包含必要 section；
- 目标控制在约 8k Token，硬上限 20k Token；
- 超限或格式无效时 revision 失败，保留上一 completed revision；
- 不使用启发式内容补全；
- 只有成功 revision 才移动 `last_summarized_message_id`；
- 锚点不能落在未完成 ToolCall 或 dangling tool-use 上；
- Runtime 重启后从 SQLite 恢复，不依赖进程级变量。

验收：

- 首次/增量阈值和自然边界测试；
- singleflight、取消、超时、崩溃残留 started revision 测试；
- 模型空输出、超长输出和格式错误测试；
- 成功更新原子移动锚点，失败不移动；
- Resume 后继续从正确锚点增量更新；
- `summaryModel=small` 确实调用 small model。

### WP5：Session Memory Compact

目标：Auto Compact 优先使用最新 completed Session Memory，避免重新总结全历史。

使用条件：

- `sessionMemoryEnabled=true`；
- 存在非空 completed revision；
- `last_summarized_message_id` 能在 canonical history 中定位；
- 等待在途 extraction 最多 15 秒；超时后使用最后 completed revision；
- 构建后的投影低于 Auto Compact 安全阈值。

保留 Tail：

- 从 Memory anchor 后开始；
- 反向扩展至至少 10k Token；
- 至少 5 条含文本消息；
- 目标上限 40k Token；
- 保持 ToolUse/ToolResult、Provider reasoning/signature 和消息 Role 不变量；
- 未完成当前 Turn 必须完整保留；
- 如果配对扩展导致超过上限，允许超过 tail 软上限，但最终总投影必须通过安全预算校验。

成功结果：

- 创建 synthetic user Session Memory summary message；
- 创建 `kind=session_memory` completed boundary；
- 持久化 summarized refs、preserved refs 和 cutoff；
- 重新注入有界的 Read Files、Todo、Skills 和运行中 Agent Tasks；
- 更新 Session 最新 boundary/兼容 SummaryMessageID；
- 重启后通过 Boundary projector 重建 `summary + tail + new messages`。

Memory 缺失、锚点失效、Memory 过大、投影仍超阈值或任何验证失败时返回
`not_applicable`，由同一个 Compact Coordinator 继续执行 LLM Full Compact。

验收：

- 正常 anchor、缺失 anchor、Resume、在途 extraction 超时；
- min tokens/min text/max tokens 和 pairing-safe tail；
- 压缩后仍超阈值时正确 fallback；
- Session Memory boundary 后继续多轮对话；
- 应用重启前后 Provider history 完全一致；
- UI canonical history 无重复、无删除。

### WP6：LLM Full Compact 可靠性收口

目标：Full Compact 成为明确失败、可恢复的最终兜底。

实施：

- 删除 `heuristic_fallback` 成功路径；
- `GenerateCompactSummary` 的空结果、超时和错误直接返回失败；
- 摘要请求 Prompt Too Long 时按完整 API round/Turn 删除最旧输入，最多重试 3 次；
- 摘要前移除图片、二进制和压缩后会重新注入的附件；
- `summaryModel` 正确映射到 `UseSmallModel`；
- 自定义手动 Compact 指令直接进入 Full Compact，不使用 Session Memory；
- reinjection 每类及总量都有预算，并计入压缩后预算；
- Summary、Boundary、preserved refs 和 Session anchor 原子提交；
- PostCompact cleanup 在 manual/auto/reactive 三条路径保持一致；
- 失败保留旧 boundary 和旧 Session Memory，不改变当前历史投影。

启发式摘要函数可以删除；如果诊断页面需要预览，只能改名为 diagnostic preview，
且不得进入 Provider history。

验收：

- 空摘要、模型错误、超时、三次 PTL 后失败；
- PTL 每次删除完整 round，不制造孤立 ToolResult；
- custom instructions、small/session model 路由；
- reinjection 预算和 stale file 状态；
- 事务失败不会出现只有 Summary 或只有 Boundary；
- Full Compact 后重启投影稳定。

### WP7：Reactive Recovery、可观测性与清理

实施：

- Provider 错误分类优先使用结构化 status/code，再使用字符串兼容匹配；
- Attempt 1 使用统一 Microcompact reducer 的 reactive policy；
- Attempt 2 使用统一 Compact Coordinator：Session Memory -> Full Compact；
- Attempt 3 只在仍有安全缩减空间时重试，避免重复生成等价摘要；
- Reactive Attempt、action、pre/post budget、will_retry、circuit_open 全部落库；
- Context/Compact 事件只由 Runtime 发出；
- Prompt Assembly 关联 Projection、Boundary、Replacement 和 Memory revision；
- Session 删除/清理补齐新表；
- 更新 Runtime DTO、Wails bindings 和诊断投影；
- 删除不再使用的重复代码、测试和注释；
- 更新 `docs/runtime.md`、`docs/data-storage.md` 和设置文档。

验收：

- 普通 Provider 错误绝不触发压缩；
- Attempt 1/2/3 行为顺序固定且有界；
- 熔断后不再消耗摘要模型调用；
- 手动 Compact 可以解除熔断；
- Session purge 不留下 Memory/Boundary/Replacement；
- 诊断可以回答“为什么压缩、压缩了什么、节省多少、使用了哪份 Memory”。

### WP8：前端 UI、Wails 契约与交互收口

目标：让用户看得懂正在发生什么，但不让 React 成为压缩状态或历史投影的第二权威来源。

实施：

- 增加 Runtime `ContextCompactionStatus` 查询，并把 active operation、latest completed、
  latest failed、circuit state 和 latest Session Memory revision 纳入会话快照；
- `compact.*` Wails 事件只作为失效通知和即时进度，切换 Session、丢失事件或重启后由
  状态查询恢复，前端不得仅依靠事件序列推断最终状态；
- 扩展 Boundary DTO/ViewModel：`kind/trigger/status`、pre/post tokens、saved tokens、
  summarized/preserved counts、Memory revision、failure、will retry 和 circuit open；
- Composer 上下文环读取压缩后的真实 Provider projection budget，增加 compacting、
  reactive recovery 和 circuit-open 状态；
- 自动 Full/Session Memory Compact 在当前 Assistant 过程状态中显示“正在整理上下文”；
  Reactive 显示“上下文超限，正在缩减并重试”，不中途弹出可恢复的 fallback 错误；
- 主时间线只展示 Manual/Auto Full Compact、Session Memory Compact 和最终 Reactive
  Recovery Notice；Tool Result Budget 与 Time-based Microcompact 默认静默，只在诊断中可查；
- synthetic summary message 属于 Provider projection artifact，不作为新的用户消息显示；
  canonical 原始消息继续正常显示，不能出现摘要与原消息重复的伪对话；
- Compact Notice 可展开查看类型、触发原因、前后 Token、摘要/保留消息数、耗时及结果，
  不默认展示摘要正文、原始工具输出或敏感引用内容；
- 保留 `/compact [instructions]` 和手动 Compact 操作，Session 内压缩 singleflight；运行中
  禁止重复触发。只有 Runtime 支持取消时才展示取消入口，否则复用 Turn 中断能力；
- 删除 Manual Snip 按钮、`onManualSnip` props、Workbench Adapter 方法和 Wails 暴露，
  历史 Snip 记录只在兼容诊断中只读展示；
- Context Diagnostics 改为中文产品文案，分为“当前预算、最近压缩、Session Memory、
  投影优化、恢复尝试”，支持复制不含正文的诊断摘要；
- 普通设置页不暴露 50k/200k、tail 或内部阈值。Time-based idle 仍由配置契约控制，
  诊断页展示最终 resolved 值与来源；后续若增加高级设置，仍通过 Wails 写 SQLite；
- 使用 Ant Design token 和 CSS Modules；不增加 HTTP、SSE、轮询或浏览器本地持久化。

交互状态：

```text
idle
  -> compact.started: compacting
  -> compact.progress: compacting（更新阶段/耗时，不轮询）
  -> compact.completed: refresh snapshot -> completed notice -> idle
  -> compact.failed + will_retry=true: recovering（不弹最终错误）
  -> compact.failed + will_retry=false: failed/circuit-open（给出手动 Compact 或切换模型入口）
```

验收：

- Session 切换、事件丢失、窗口刷新和应用重启后状态一致；
- Auto、Manual、Session Memory、Reactive 四种可见压缩文案和动作正确；
- Intermediate fallback 不产生重复 toast，最终失败只提示一次；
- Tool Result Budget/Microcompact 不污染主时间线，但诊断证据完整；
- `/compact`、按钮和 Runtime singleflight 不会产生重复请求；
- synthetic summary 不进入 canonical 聊天气泡，原始历史不消失；
- Snip 不再有可触发入口；
- 键盘、屏幕阅读器、窄窗口和深浅主题验证通过。

## 8. 前端 UI 信息架构

### 8.1 Composer 与运行状态

现有 `ContextUsageIndicator` 继续作为主入口，但数字必须来自 Runtime 对最终 Provider
projection 的预算报告，而不是 canonical timeline 的总大小。Popover 展示：

- 当前使用量 / Context Window；
- 距 Auto Compact 的 Token 和百分比；
- output reserve 与 safety buffer；
- 是否为估算值；
- 当前压缩阶段或最近一次压缩节省量；
- circuit open 时的原因和可执行恢复动作。

压缩期间不额外创建聊天气泡。Assistant 状态行显示阶段，Composer 是否禁用服从现有 Turn
状态；前端不能为了显示 loading 自行锁定或解锁 Runtime Turn。

### 8.2 Timeline 呈现策略

| 机制 | 主时间线 | 诊断 | 原因 |
|---|---|---|---|
| Tool Result Budget | 不显示 | Replacement/Object Ref 计数 | 高频保护机制，避免噪声 |
| Time-based Microcompact | 不显示 | 触发时间、keepRecent、节省 Token | API 前投影优化，不是对话事件 |
| Session Memory 生成 | 不显示 | revision、anchor、更新时间、状态 | 后台维护，不改变当前会话 |
| Session Memory Compact | 显示一条 Notice | 完整 Boundary | 改变 Provider 历史投影 |
| LLM Full Compact | 显示一条 Notice | 完整 Boundary | 高成本、影响后续模型上下文 |
| Reactive Recovery | 最终成功/失败显示 Notice | 每次 attempt | 与错误恢复直接相关 |

Notice 只显示操作事实，不把 summary 内容伪装成 Assistant 回复。点击后打开详情 Drawer 或
现有诊断侧栏，并按 Boundary ID 读取 Runtime DTO。

### 8.3 诊断与 Session Memory

诊断需要同时展示“当前发送给模型的投影”和“本地仍保存完整历史”，避免用户误以为消息已被
删除。Session Memory 只展示元数据和可选的安全预览：revision、最后锚点、覆盖消息数、Token
估算、生成模型、更新时间和失败原因。默认不把 Memory 全文或原始 ToolResult 发给前端。

### 8.4 设置边界

普通设置保持简单：Auto Compact 和内部预算采用已验证默认策略，不增加容易导致 Context Too
Long 的自由数值组合。用户要求的 `microcompactIdleMinutes` 可通过配置契约设置，UI 诊断展示
resolved value。若后续确需高级设置，必须进行范围校验、显示“恢复默认值”，并明确修改只影响
后续 Provider 调用，不修改本地历史。

## 9. Provider 适配要求

所有核心算法只接触规范化 Message/Part，不直接判断 Provider 名称。

Provider Adapter 必须分别验证：

- Role 顺序合法；
- ToolUse/ToolResult 成对；
- 不重复发送 ToolCall ID；
- Anthropic thinking signature 不被拆分；
- OpenAI Responses reasoning/function items 不产生孤立引用；
- Gemini function call/response 顺序合法；
- 不支持图片的模型在压缩辅助请求中收到文本 placeholder；
- Compact Summary 和 Session Memory helper call 不携带工具。

第一阶段不承诺跨 Provider Prompt Cache 保持命中。要求是 replacement 决策稳定、
请求确定性可重放；Provider-specific cache 优化以后作为独立能力评估。

## 10. 测试矩阵

### 单元测试

- Config 默认值、验证和桌面 SQLite 解析；
- Tool Result 单结果/聚合预算、UTF-8、Object Ref、冻结决策；
- Time-based 触发、KeepRecent、eligible tools、主/子线程隔离；
- Budget Resolver 的真实 usage/估算/模型窗口公式；
- Pair-safe tail 和完整 Turn 删除；
- Session Memory revision、锚点和状态机；
- Full Compact 重试、失败语义和 reinjection 预算；
- Reactive 状态机和熔断。

### Runtime 集成测试

- 新会话 -> Tool Loop -> Microcompact -> Auto Compact；
- Session Memory 更新 -> Memory Compact -> 继续下一 Turn；
- Memory 不可用 -> Full Compact；
- Provider 413/400 context-too-long -> Reactive Recovery；
- Compact 期间中断、数据库失败、Object Store 失败；
- Session A/B 隔离、Resume、Restart、Purge；
- Prompt Assembly 与上下文治理记录关联。

### Provider Contract 测试

- OpenAI-compatible；
- Anthropic；
- Gemini；
- 不支持 Tool Calling 的文本模型；
- 不返回 usage 的 Provider；
- Context Window metadata 缺失的 Provider。

### UI/Wails 验证

- `compact.started/progress/completed/failed` 状态机与事件丢失恢复；
- Session Memory、Full Compact、Manual、Auto 和 Reactive trigger 区分；
- pre/post tokens、saved tokens、summarized/preserved count；
- 主时间线静默/可见机制矩阵正确，Notice 展开详情正确；
- 重启后 Timeline、active operation 与 Runtime canonical store 一致；
- synthetic summary 隐藏，canonical message 不丢失、不重复；
- Manual Snip 入口和调用链已移除；
- 前端不自行修改 Message、推断压缩边界或持久化压缩状态；
- Wails binding、adapter mapper、static adapter 和组件测试同步更新。

## 11. 实施顺序与提交边界

建议按以下顺序提交，每个工作包独立通过测试后再进入下一包：

```text
WP0 control plane / boundary projection
 -> WP1 tool result budget
 -> WP2 time-based microcompact
 -> WP3 budget resolver / auto controller
 -> WP4 session memory revisions / extractor
 -> WP5 session memory compact
 -> WP6 full compact hardening
 -> WP7 reactive / diagnostics / cleanup
 -> WP8 frontend UI / Wails contract / interaction
```

不得把 WP4 和 WP5 合并成一个大提交。Session Memory 生成必须先在不参与 Compact 的情况下
运行和验证，确认锚点、恢复和质量可靠后，才能允许它改变 Provider history。

WP8 是最终交互收口，不代表前端工作全部延后：WP0 先冻结 DTO；各 Runtime WP 同步更新 Wails
binding、adapter mapper 和契约 fixture；WP8 再完成组件呈现、交互状态和端到端验收。

### 11.1 实施修订（2026-07-30，验证阻塞修复）

全量 Runtime 测试证明，canonical projector 若在事件持久化副作用中调用
`ensureWorkspaceStarted`，会隐式启动 workbench 和 permission consumer，导致无界等待、事件序列冲突
以及测试/桌面配置污染。为保持 WP0 的 Runtime/SQLite 权威边界，实施约束修订为：

- 只有显式 `SessionConversationSnapshotV2` API 边界允许确保 workspace 已启动；
- 事件 projector 只能使用已经挂接的 Runtime stores，缺失 store 从同一个 `eventStore.db`
  构造，不得因投影副作用启动 workspace；
- workbench 尚未挂接时，projector 只投影 SQLite 中可恢复的 canonical entities；显式快照请求
  再读取 workbench messages；
- 自动事件序列每次分配前与 SQLite `MaxSequence` 对齐，查询不得持有 Runtime mutex，避免
  projector transaction 与 mutex 锁顺序反转；
- Context Governance 的桌面读写统一复用 `runtimeService.configDB`，生产仍只使用 SQLite，
  测试通过实例级 data directory 隔离，不再依赖进程级环境变量或 JSON fallback；
- Windows 日志 lint 使用 Go AST 检查器，保持原有规则强度并避免 `.sh` 执行依赖。

### 11.2 实施修订（2026-08-02，桌面手测回归修复）

桌面手测发现 canonical notice 状态和值域、Wails 实时流恢复仍需进一步收口。为满足 WP8
“当前 Session 无需切换即可收敛”的验收条件，实施约束补充为：

- Runtime 将所有 `*.started` canonical notice 规范化为 `running`；前端必须把 `running`
  作为压缩处理中主状态，同时仅兼容旧快照中的 `started`/`compacting`；
- general Runtime event stream 作为独立的 Wails invalidation channel：若它已经观察到某个
  Session 的 durable sequence，而 canonical conversation stream 在 500ms 宽限期内没有推进到
  对应 cursor，前端 coordinator 必须从 Runtime/SQLite canonical snapshot 恢复；React 不据此
  推断 Turn、Todo 或压缩终态；
- Runtime refresh 与 canonical stream 合并后必须同步更新 shell ref，避免后续异步动作以陈旧
  view model 为基线；
- Wails binding 动态加载超时只允许令当前调用失败，不得把 `null` 永久缓存到 WebView 生命周期，
  后续调用必须可以重试本地生成 binding；
- 回归测试必须覆盖 `compact.started -> compact.completed -> turn.completed` 的同一 Session
  实时序列、Notice 原位更新、terminal Todo 隐藏，以及 missed canonical wakeup 的 snapshot 恢复。

## 12. 全阶段完成标准

第一阶段只有在以下条件全部满足后才算完成：

- Tool Result 原文永远可恢复，Provider 输入永远有界；
- 空闲 60 分钟的 Time-based Microcompact 按配置稳定触发；
- 所有主动压缩使用同一个预算报告和 Auto Controller；
- Session Memory 跨重启持久化，失败不移动锚点；
- Auto Compact 优先 Session Memory，失败后才调用 Full Compact；
- Full Compact 没有启发式成功降级；
- Summary + preserved tail 跨 Turn、Resume 和 Restart 可重建；
- Context Too Long 恢复有界、可观测且受熔断保护；
- 前端可以恢复并准确展示压缩状态，静默机制不污染主时间线；
- synthetic summary 不形成伪对话，Manual Snip 不再可触发；
- OpenAI-compatible、Anthropic、Gemini 基本消息不变量通过；
- canonical conversation、ToolCall、ToolResult 和审计记录没有被压缩操作删除；
- `go test ./...`、`go build ./...`、`cd client && npm run build` 和 `task lint`
  全部通过。

## 13. 第一阶段之后的推进边界

本文档只定义第一阶段，没有已批准的“上下文压缩第二阶段”实施范围。
WP0–WP8 后的直接工作是第一阶段的桌面验收和发布稳定化：

- 完成 Auto/Manual/Session Memory/Reactive 的桌面手测矩阵；
- 验证当前 Session 实时收敛、事件丢失恢复、重启恢复和跨 Provider 消息不变量；
- 将手测发现的回归修复成自动化测试，通过第 12 节的全量门禁后再发布。

第二阶段必须另行立项和评审。可候选但尚未授权的方向包括
Provider-specific Prompt Cache 优化、有数据支持时的 Session Memory Object Store 迁移，
以及高级 Context Governance 设置。Snip Compact、Context Collapse 和 Anthropic Cached
Microcompact 不会因为进入后续阶段而自动纳入范围。
