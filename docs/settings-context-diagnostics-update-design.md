# 设置：上下文统计、诊断与版本更新设计

本文定义设置页后续三个正式产品能力的实现边界。设计遵循现有的
`React -> Wails -> RuntimeService -> SQLite/系统能力` 链路；React 只保存筛选、
展开和加载状态，不作为统计、诊断或更新状态的权威来源。

## 导航与信息架构

设置导航保留“上下文”和“诊断”。终端不再是设置项；工作区终端仍是会话能力，
由 Runtime 根据系统环境选择可用 profile。版本更新放在“通用”的“关于与更新”区，
不新增一级导航。

```text
通用
  外观
  默认打开目标
  关于与更新
上下文
  使用概览
  Token 活动
  上下文治理
诊断
  健康概览
  检查结果
  支持信息
```

## 上下文

### 页面布局

页面顶部使用五个等宽统计单元：累计 Token、峰值 Token、最长任务、当前连续天数、
最长连续天数。窄窗口下变为两列，最后一项占满剩余宽度。

页面只提供两个产品视图：“每日”和“累计”。“每日”默认显示最近一年的热力图；
“累计”显示全生命周期汇总和输入/输出/缓存 Token 构成。近 7 天、近 30 天、自然月等
数字都是同一份每日事实的统计结果，不作为独立视图或独立存储。

热力图每个格子有键盘焦点和 tooltip，内容包含日期、输入/输出/缓存 Token、会话数和 Turn 数；
颜色只使用 Ant Design 主题 token 派生的五级色阶。无数据与零值必须可区分。

设置导航将该页面命名为“Token 用量”，页面只展示统计结果，不再重复显示“上下文”标题。
自动压缩默认启用；自动压缩阈值、Microcompact、保留条数和摘要模型等治理参数不作为普通用户
设置项展示，继续由运行时配置和经过验证的默认策略管理。

### 统计口径

- 累计 Token：已落库的真实 Provider usage 事实总和；不把当前窗口估算值计入累计值。
- 峰值 Token：单个模型调用的 `input + output + cache read + cache write` 最大值。
- 最长任务：`runtime_turns.finished_at - started_at` 的最大值，运行中 Turn 不参与。
- 活跃日：至少存在一个已落库 usage delta 或完成 Turn 的本地自然日。
- 连续天数：按用户当前时区计算；当前连续天数允许今天尚未活跃但昨天活跃。
- 删除 Session 不删除已经发生的匿名 usage 事实，否则累计消费会倒退；只解除 Session/Turn
  的可导航关联。提供单独的“清除使用统计”动作处理用户的数据删除意图。

### 采集与存储

Token 统计使用 SQLite，不使用 JSON/JSONL 文件：文件无法同时提供事务、幂等约束、
范围查询和可靠的 Session 删除语义，也会在多实例写入时引入额外锁协议。

现有数据有三种不同语义，不能混用：

- `messages.usage_json` 是 assistant step 的真实 Provider usage，主要作为上下文计量锚点；
- `sessions.prompt_tokens/completion_tokens` 是当前 Session 的便捷计数，不是历史 usage ledger；
- `runtime_turns.usage_delta_json` 当前由 Turn 前后 Session 值相减得到，不应作为统计权威来源。

首版不新增“每个模型请求一行”的表，也不增加新的采集写入。Agent 已经在每个真实
Provider step 完成时把 usage 写入原本就必须保存的 assistant message；统计直接读取
`messages.usage_json`，按 `messages.created_at` 聚合每日数据并计算累计值。fallback estimate
不会写入 message usage，因此天然不会混入正式统计。

```sql
SELECT
    created_at,
    json_extract(usage_json, '$.input') AS input_tokens,
    json_extract(usage_json, '$.output') AS output_tokens,
    json_extract(usage_json, '$.cacheRead') AS cache_read_tokens,
    json_extract(usage_json, '$.cacheCreation') AS cache_creation_tokens
FROM messages
WHERE role = 'assistant'
  AND usage_json IS NOT NULL
  AND created_at >= ?
  AND created_at < ?;
```

总 Token 始终计算为 `input + output + cache read + cache creation`；reasoning token 只作为
输出的细分展示，不能再次加到总数中。不要依赖各 Provider 口径并不完全一致的 `total_tokens`。

这个口径表示“已持久化的对话模型 usage”。未形成 assistant message 的标题生成等内部调用
首版不纳入统计，并在页面说明口径；它们通常占比很小。若未来必须统计全部内部调用，再为
这些少量调用增加独立汇总入口，不因此为所有普通模型请求复制一份记录。

时间统一保存 UTC Unix milliseconds。Runtime 根据请求中的 IANA timezone 在 Go 中转换为
本地自然日再聚合，确保夏令时正确；不要用前端时钟决定归属日。

如果真实数据证明直接查询变慢，再增加只有每日一行的可重建投影 `runtime_token_usage_daily`。
投影可在 Turn 完成后更新一次，或在首次打开统计页时从 messages 重建；周、月、累计仍由
每日行求和。它是缓存而不是第二个权威来源。

### 聚合触发与并发保证

统计对象改为“带真实 usage 的 assistant message”，与 Turn 最终状态无关。一旦
`message.completed` 已经持久化，Token 就已经消耗；成功、失败、取消和中断都不撤销。

不维护每 Turn 内存 accumulator，也不给 Turn/Message 增加逐行 accounted marker。复用已有
持久化 `runtime_events.sequence` 作为顺序日志，并在 `runtime_settings` 保存一个全局
`token_statistics_cursor`。worker 每次读取 cursor 之后的事件，处理带 usage 的 assistant
`message.completed`，然后在同一事务更新 daily bucket 和 cursor。

当前 `message.completed` event payload 只有 role/summary，实施时需要增加结构化 usage，且保证
同一 message ID 只产生一次 durable completed event。payload 只包含计数，不包含消息正文、
工具参数或 secret。

```text
assistant message + usage 已落库
  -> append unique message.completed(sequence=N, messageId, usage)
  -> 等待下一个统计周期
  -> scan runtime_events where sequence > token_statistics_cursor
  -> daily += batch usage
  -> token_statistics_cursor = batch last sequence
```

message.completed event 与 cursor 都已持久化，因此不需要 channel 或事件唤醒。进程崩溃、
统计周期被跳过或大量并发消息完成都不会丢任务；下次定时任务从 cursor 继续。Turn 状态和
Turn 完成时间不参与统计 eligibility。

推荐调度：Runtime 启动后延迟 60–120 秒首次执行，此后每 10–15 分钟运行一次并加入少量
jitter，避免多个应用实例同时抢占数据库。统计页只读取已有 daily 缓存；缓存较旧时显示
“最后更新于”，手动刷新也只是请求下一次低优先级执行，不阻塞页面或对话。

每日表使用日期唯一键：

```sql
CREATE TABLE runtime_token_usage_daily (
    day TEXT PRIMARY KEY,
    timezone TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    turn_count INTEGER NOT NULL DEFAULT 0,
    model_call_count INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);
```

### 数值类型

Token、调用数和累计计数在 SQLite 与 Go 中统一使用有符号 64 位整数：SQLite `INTEGER`、
Go `int64`，禁止使用平台相关的 Go `int`。`int64` 上限约为 `9.22e18`；即使每天一万亿
Token，也需要约 2.5 万年才达到上限，足以覆盖桌面产品生命周期，同时保留 SQLite 原生
SUM、比较和索引能力。

不要把大整数直接作为 JSON number 传输。JavaScript `number` 只能精确表示到
`9,007,199,254,740,991`，而 Runtime event 的通用 `map[string]any` JSON 解码也可能经过
`float64`。因此：

- `message.completed` usage payload 使用十进制字符串，例如 `"input": "123456789"`；
- worker 使用 `strconv.ParseInt` 解析为 `int64`，不经 float；
- Wails statistics DTO 的 Token 字段使用 decimal string；
- React 需要运算时使用 `BigInt`，不要先转换为 `number`；
- 热力图等级、百分比和格式化短值可以由 Runtime 预计算，避免图表库接触超大整数。

所有加法使用溢出检查，不能让 Go 或 SQL 静默回绕。检测到接近 `math.MaxInt64` 时停止该桶
更新并产生诊断错误。Cost 不使用浮点累计；若后续展示费用，应单独使用固定精度的整数
最小单位或十进制字符串，不能与 Token 计数混为一列。

worker 每批读取一段顺序事件，先在内存中按事件发生日合并，再用一个短事务：

1. 执行 `daily = daily + batch delta`；
2. 更新 `token_statistics_cursor` 到本批最后 sequence；
3. commit。

累加和 cursor 必须属于同一事务：提交成功则两者同时成功，回滚则两者都未发生，因此重复扫描
不会重复记账。跨越午夜不需要 Turn 规则，每条 message 按 completed event 的本地日期归桶。

当前数据库池限制为单连接，worker 必须低优先级批处理。建议读取 128–512 个 event，按日期
合并后每天只执行一次 upsert；连接繁忙时退避，批次锁持有目标小于 10ms。正常路径只顺序读取
cursor 后的新事件，不扫描历史 message。全量重算仅用于首次迁移、显式修复和一致性检查。

每次周期设置总工作预算，例如最多 4 个批次或 40ms；仍有积压时保留 cursor，等下一周期继续。
扫描必须读取连续 sequence 区间，并把 cursor 推进到本批最后一个事件，即使该批没有 Token
事件，也不能反复扫描大量无关 Runtime events。

worker 资源预算：空闲时只有一个 timer，无常驻队列；执行时启动一个受控 goroutine，常驻
状态目标小于 32 KiB；512 个小型 usage event 的瞬时内存目标小于 512 KiB；单批 CPU 目标小于 2ms，
一次批次只有一个 SQLite commit。

这与 RRD 的共同点是固定 daily bucket 和滚动 365 天；区别是 SQLite 使用显式日期主键和
`DELETE WHERE day < cutoff`，不使用 `day % 365` 覆盖槽位。停机期间没有采样任务也没有关系，
恢复后从持久化 cursor 继续读取事件。

支持 100 个并发会话是 Runtime 全局写路径的问题，不是统计 worker 的问题。现有每个
`OnTextDelta` 同步 `messages.Update` 会造成主要写放大；应引入单写者优先级批处理：终态、
权限和恢复写为高优先级同步任务，流式 message checkpoint 按 message ID 在 100–250ms 内
保留最新值并合并到一个事务，Token 统计为最低优先级批次。这样 100 个生产者不会产生
100 条数据库连接或每 token 一次 commit，而是由一个稳定写者串行批量提交。

### 容量、保留与 Runtime 开销

首版复用已有 message.completed event，只为 event payload 增加少量 usage 计数；每日投影一年
最多约 365 行，通常远小于 1 MiB。统计不再为每个 Turn 或 Message 新增 marker 行。

统计默认读取最近 365 个本地自然日；“累计”读取独立的 lifetime 投影，不会因 daily bucket
轮转或 Session 删除而倒退。投影只保存匿名计数、峰值和聚合时间，不保存消息正文、工具参数
或 Provider secret。首次接管从 `messages.usage_json` 重建投影，并把 cursor 原子推进到当时
最大的 runtime event sequence；之后只顺序消费新的 `message.completed` 事件。

统计 worker 只顺序消费 persisted runtime event，不新增 Wails 事件或常驻 Turn accumulator。
统计页查询固定为最多 365 个 daily bucket，不加载历史 message。

### Runtime 契约

```go
type RuntimeContextStatisticsRequest struct {
    From       string // YYYY-MM-DD，按 Timezone 解释
    To         string
    View       string // daily | cumulative
    Timezone   string // IANA timezone
}

type RuntimeContextStatistics struct {
    TotalTokens       int64
    PeakTokens        int64
    PeakAt            string
    LongestTurnMillis int64
    LongestTurnID     string
    CurrentStreakDays int
    LongestStreakDays int
    ActiveDays        int
    Points            []RuntimeContextStatisticsPoint
}
```

新增 `RuntimeService.ContextStatistics`，由 `desktop/RuntimeBridge` 原样转发；前端适配器把
DTO 映射成 `ContextStatisticsViewModel`。范围或粒度变化时发一次请求，不使用轮询。

## 诊断

### 产品定位

设置中的“诊断”是面向用户的故障事件中心，不是默认运行几十项全量系统健康检查。主要场景是：
一次对话、工具或恢复操作失败后，用户希望知道发生了什么、可能原因、能否重试以及应该修改
哪个设置。

页面默认只读取已有持久化证据，不主动测试 Provider、不执行 SQLite integrity check、不遍历
文件系统，也不检查版本更新。完整 `/doctor` 保留为高级手动入口；它与页面可复用检查器，
但不是页面的默认数据源。

上下文诊断从 Runtime `ContextCompactionStatus` 和 Prompt Assembly 读取，按“当前预算、最近压缩、
Session Memory、投影优化、恢复尝试”展示。主页面不暴露 50k/200k、tail 等内部阈值；
`microcompactIdleMinutes` 的 resolved 值与来源只在诊断中展示。复制诊断摘要不得包含摘要正文、
原始 ToolResult 或敏感 reinjection 内容。

首版遵循“少而精”：覆盖面不是目标，定位价值才是。一个诊断项只有同时满足以下条件才进入
默认页面：

- 有明确的失败事实和关联 ID，能够回到具体 Session、Turn 或 ToolCall。
- 能展示至少一条可验证证据，而不是仅根据错误文本猜测原因。
- 能给出用户可执行的下一步，例如重试、修改 Provider 设置或查看工具输出。
- 能区分异常失败与用户取消、权限拒绝等预期行为。

不满足这些条件的信号继续留在日志或高级 `/doctor` 中，不能为了数量展示模糊告警、推测性结论
或长期为绿色的检查项。

### 现有可观测基础

当前数据库已经保存了大部分对话失败信息，首版不需要新建完整的 error/incident 事实表：

| 现有记录 | 可用于诊断的内容 |
|---|---|
| `runtime_turns` | Turn 状态、错误、Provider、模型、停止原因、开始/结束时间 |
| `runtime_tool_calls`、Message ToolResult | 工具状态、错误、stdout/stderr、退出码、结果是否送回模型 |
| `runtime_hook_executions`、`runtime_mcp_requests` | Hook 阻断/失败、MCP 请求与服务状态 |
| Agent task/result/message 表 | 子任务状态、结果和父会话关联 |
| context projection/reactive/warning 表 | 上下文投影、压缩、重注入和告警证据 |
| `runtime_permission_requests`、`runtime_sandbox_decisions` | 权限与 sandbox 决策，帮助区分拒绝和执行故障 |
| runs/transitions/checkpoints、`runtime_recovery_links` | 运行状态迁移、恢复来源和重试链路 |
| `runtime_audit_events` | 操作与策略审计证据 |
| 有序 `runtime_events` | 按 sequence 还原一次失败前后的 Runtime 事件 |

Runtime 还已有 `RuntimeTurnDiagnostics` 和 `RuntimeRecoverableError`，可以直接复用其停止原因、
产物、工具投递和 Provider 错误分类。设置页需要解决的是把这些分散证据组织成用户可理解的
故障事件，而不是重复保存一份所有错误。

### 首版故障范围

首版只整合最常见、证据最完整且用户能够采取行动的故障，不新建平行错误日志：

| 用户故障 | 主要证据 | 定向检查/动作 |
|---|---|---|
| 模型请求失败 | failed Turn、`RuntimeRecoverableError`、retry event | 重试、压缩上下文、打开 Provider 设置、测试连接 |
| 对话中断/无最终回复 | `RuntimeTurnDiagnostics`、runtime events、message/tool delivery | 恢复、标记完成、查看最后活动 |
| 工具执行失败 | `runtime_tool_calls`、ToolResult、sandbox/policy | 查看输出、重新运行、打开产物；普通非零退出不一定是产品故障 |
| 数据持久化失败 | 明确的 DB open/query/write 错误、缺失的终态写入 | 展示数据目录、重试写入；仅在有直接 DB 错误时运行 quick check |

Hook、MCP、Skill、文件和 Object 问题首版不单独形成诊断分类。只有它们已经造成上述 Turn 或
工具故障时，才作为关联证据展示。后续新增分类前必须先证明它能够稳定归因并提供有效动作。

权限拒绝、用户主动取消、命令返回业务性非零状态等不能自动标为应用故障；必须结合 Turn 的
stop reason 和用户动作区分“预期停止”与“异常失败”。

数据库完全无法打开时设置页本身可能不可用，因此这类启动故障需要独立的 bootstrap/error
surface 展示内存中的最小诊断和日志位置；不能假设所有诊断都能先写回同一个故障数据库。

诊断不得包含 Provider secret、完整请求/回复正文、完整环境变量或未经确认的敏感路径。
“复制支持信息”只导出 allowlist 字段，并在复制前显示预览。

### Incident 投影、关联与去重

`RuntimeDiagnosticIncident` 是对现有事实的只读投影，不是新的权威错误账本。投影优先使用
数据库中已经存在的外键和稳定 ID 关联证据：`session_id`、`turn_id`、`run_id`、
`tool_call_id`、task ID、event sequence 和 recovery link。只有旧数据缺少直接关联时，才使用
受限时间窗口加 Provider/error code 等字段做弱关联；不能只按错误文本合并。

稳定 Incident ID 按最接近用户操作的故障根生成：

- 对话/Provider 故障：`turn:<turn_id>`。
- 独立工具故障：`tool:<tool_call_id>`；已属于失败 Turn 时只作为该 Turn 的证据。
- Hook、MCP 等首版没有独立 Incident ID；能关联 Turn 或 ToolCall 时只并入其证据。
- 同一根 ID 下的 Turn、event、audit、recovery classification 是一张卡片的多条证据，不重复展示。
- 恢复或重试成功后保留原 Incident，并把它标为已恢复；新 Turn 仍保留自己的运行事实和 recovery link。

典型证据链如下：

```text
Provider 429
  -> runtime_turns.status=failed + error/provider/model
  -> runtime_events: turn.failed
  -> RuntimeRecoverableError 分类为 rate_limit
  -> runtime_audit_events / runtime_recovery_links
  -> Incident turn:<turn_id>：解释限流、展示重试动作和最终恢复结果
```

投影时要过滤预期行为：用户主动取消、权限拒绝、业务命令的预期非零退出、已禁用能力，以及
已经自动重试成功且未影响用户结果的瞬时失败，默认不进入故障列表。它们仍可作为详情证据或在
高级筛选中查看。

### 现有缺口

当前问题主要是可观测证据分散，而不是完全没有失败记录：

- 错误分类尚未完全结构化，部分路径只有自由文本，首版需要集中映射稳定 `kind/error_code`。
- 一些 best-effort 失败只写入 `slog`，不会成为可查询的 Runtime 事实。
- 个别持久化错误被忽略，例如终态 Turn 的 `Upsert` 结果没有处理；这会造成日志显示失败但数据库缺证据。
- 数据库打开/迁移失败时无法把错误写回同一个数据库，需要 bootstrap 诊断面承接。
- 现有表没有统一的“已查看/忽略/已处理”用户状态，也没有统一 Incident 查询模型。

实现时先修复关键写入错误的显式记录，并将适合用户诊断的 log-only 失败转换为结构化事件或
现有表字段；不要把所有 `slog` 都复制进数据库。只有确实需要跨启动保存用户处理状态时，才增加
很小的 `runtime_diagnostic_acknowledgements` 表，建议只保存 `incident_id`、状态、更新时间和
可选备注，不复制 title、error、证据正文等可由现有事实重建的内容。

### UI

顶部显示最近 7 天失败数量、最近一次故障和仍可恢复的任务数，不展示一排长期为绿色的健康项。
主体按时间倒序列出故障事件，支持按 Session、类型和“可恢复/已处理”筛选。

每项显示用户可读标题、发生时间、关联会话/Turn、简短原因和建议动作。详情抽屉再展示错误码、
Provider/模型、最后活动、相关 Tool/Hook/MCP、脱敏技术详情与证据来源。操作优先复用现有
`RuntimeRecoveryAction`：重试、压缩、恢复、标记完成、打开设置和复制支持信息。

只有用户点击某个故障的“进一步检查”时才运行定向检查。检查有独立超时，失败只更新该故障
详情，不能启动与当前证据无关的扫描。

首版“进一步检查”只保留 Provider 连接测试、错误中明确路径的存在性/访问验证、直接 DB 错误
触发的 SQLite quick check。每个 Incident 最多推荐一个主检查和两个主动作；没有明确诊断假设时
不运行检查。检查结果必须回答一个具体问题，例如“当前凭据能否连接该 Provider”，不能只返回
泛化的 pass/warning。

```go
type RuntimeDiagnosticIncident struct {
    ID, Kind, Severity string
    SessionID, TurnID, RunID, ToolCallID string
    Title, Summary, ErrorCode string
    Recoverable, Resolved bool
    CreatedAt, LastObservedAt string
    Evidence []RuntimeDiagnosticEvidence
    Actions []RuntimeRecoveryAction
}

type RuntimeTargetedDiagnostic struct {
    IncidentID, CheckID, Title string
    Status           string // pass | info | warning | error | unknown
    Summary, Detail  string
    DurationMillis   int64
}
```

`RuntimeService.DiagnosticIncidents` 首版只从 failed/interrupted Turn、失败 ToolCall、明确的
持久化错误及其 events、audit、recovery links 投影最近故障；Hooks、MCP 和文件/Object 状态只作
关联证据。查询可以按时间窗口先取候选根记录，再批量加载关联证据，避免 UI 为每张卡片产生
N+1 查询。`RunTargetedDiagnostic` 只接受少量枚举 check ID；React 不能提交任意 shell 命令。
真正会改状态的修复继续使用独立 Runtime 方法与权限确认。

### 当前实现

设置页已实现四类 Incident 投影：`provider_failure`、`turn_interrupted`、`tool_failure` 和
`persistence_failure`。查询以 `runtime_turns` 和 `runtime_tool_calls` 为候选根，批量关联
`runtime_events`、`runtime_audit_events`、`runtime_recovery_links`、Hook、MCP、权限与 sandbox
事实；失败工具属于失败 Turn 时只作为证据，不生成重复卡片。用户取消、权限/策略拒绝以及已被
成功 Turn 正常处理的命令失败不会进入默认列表。

终态 Turn 写入失败不再静默忽略。数据库或 bootstrap 无法写入时，Runtime 使用有界的进程内
诊断缓冲提供最小故障面，不递归写回故障数据库。定向检查只接受 Incident ID 和枚举 Check ID，
Provider 检查使用关联 Provider、路径检查只使用证据中的路径、SQLite quick check 只接受直接
持久化 Incident。支持信息由 Runtime 按 allowlist 生成，并在 React 复制前展示预览。

React 页面通过 Wails binding 加载数据，提供最近七天数量、最近故障、可安全修复数量、Session/类型/
状态筛选、手动刷新、详情 Drawer、已确认原因、解决方案、精简故障链和支持信息预览。诊断页不直接
重试模型或工具任务，需要再次执行时引导用户返回关联会话。定向检查只在现有证据不足且 Incident
明确适用时显示具体名称；页面不轮询，也不会在默认加载时运行 Provider、文件系统或数据库检查。

## 版本更新

### 当前边界与首版决策

Windows 生产构建已经把 Go Runtime、Wails shell 和 React 静态资源嵌入
`AgentBuilder.exe`，因此首版应用更新只需要替换完整 exe，不需要单独更新前端文件。当前
`data/`、`config/`、`logs/` 位于 exe 同级目录，更新包不得包含、覆盖或移动这些用户数据。

二进制采用全量更新，数据库采用增量迁移：

- 每次下载完整、签名的 `AgentBuilder.exe`。暂不做二进制 delta；它会增加补丁生成、基线匹配、
  签名、失败恢复和测试组合，而当前收益不足。
- SQLite 只执行从当前 schema 到目标 schema 的有序增量 migration，不全量重建数据库。
- `projects/`、Objects、会话下载等数据文件默认保持原位；只有明确发生文件布局变化时才增加
  独立、可恢复的 data migration。
- 将来若 exe 体积或带宽成为实际问题，可在 manifest 增加 delta asset，但必须始终保留完整 exe
  作为回退路径。

当前阶段不决定最终使用压缩包、NSIS 或其他安装程序，也不决定更新资产最终托管在 GitHub、
对象存储或自建服务。先冻结统一的 Application Root 目录协议，所有分发方式安装或解压后都应
产生同一逻辑结构：

```text
<AgentBuilderRoot>/
  AgentBuilder.exe
  config/
  data/
    agent-builder.db
    projects/
    global/
    cache/updates/
    backups/migrations/
  logs/
```

Application Root 的实际位置由用户、安装程序或部署规范决定，Runtime 不能根据“压缩包版”或
“安装版”切换数据语义。更新程序只通过该 root 定位 current exe、数据、缓存和备份，安装器仅负责
创建相同目录、快捷方式和卸载信息。

为了支持不提权的应用内更新，该 root 必须允许当前用户创建临时文件、原子替换 exe、写数据库和
备份。如果某种系统安装机制把 exe 放入只读/受保护目录，它不能物理满足这一协议，必须由该安装
机制自身完成二进制更新，或选择另一个可写 root；客户端不能绕过系统权限。逻辑目录和数据契约
仍保持一致。

### 发布与信任模型

更新服务发布 HTTPS manifest，应用内置 Ed25519 公钥并校验 manifest 签名。资产还要校验大小、
SHA-256 和 Windows Authenticode signer；私钥只存在于发布 CI。manifest URL 由构建/发行配置
注入，不作为普通用户可任意修改的设置。其协议不能依赖 GitHub 专有响应，因此以后可以在不修改
客户端更新状态机的情况下把资产放到 GitHub Releases、对象存储或自建服务。旧的
`internal/update.Check` 可以作为版本比较逻辑参考，但当前 GitHub latest response 不能替代签名
manifest。

```json
{
  "schemaVersion": 1,
  "channel": "stable",
  "version": "0.2.0",
  "publishedAt": "2026-07-15T08:00:00Z",
  "minUpgradableVersion": "0.1.0",
  "databaseSchema": 3,
  "minDatabaseSchema": 2,
  "releaseNotesUrl": "https://example.invalid/releases/0.2.0",
  "assets": {
    "windows-amd64": {
      "kind": "windows-full-exe",
      "url": "https://example.invalid/agent-builder-0.2.0-windows-amd64.exe",
      "size": 123,
      "sha256": "..."
    }
  },
  "signature": "base64-ed25519-signature"
}
```

应用必须拒绝版本倒退、错误通道、错误平台/架构、当前版本低于 `minUpgradableVersion` 的直接升级和未知
manifest schema。构建版本只允许一个权威源，并同时注入 `internal/version`、Wails 平台元数据和
manifest；不能继续分别手工维护版本。

### 触发更新与活跃会话

检查和下载不改变 Runtime 状态，可以在对话期间后台执行。首版支持两种检查触发：设置页手动
“检查更新”，以及启动后延迟检查；自动检查最多每天一次并带随机抖动，不能阻塞启动。发现更新
时只显示轻量提示，不在生成回复期间弹出抢焦点对话框。自动下载默认关闭。

安装必须由用户点击“重启并更新”显式触发，首版不做静默安装，也不把普通关闭窗口隐式解释为
同意更新。首版不提供“等待后自动安装”或“自动取消任务并更新”，只做简单的空闲检查：

1. 用户点击“重启并更新”时，Runtime 在同一个临界区内检查 active Turn、Agent task、仍在执行的
   ToolCall、待处理权限/MCP 请求和运行中的 terminal job。
2. 任一项仍在执行就返回 `update_busy` 和数量，保持更新为 ready，不改变 Runtime 状态。UI 提示
   “有正在执行的会话，请等待完成或手动停止后重试”，并允许跳转到对应会话。
3. 应用不替用户取消 Turn、工具或任务，也不在它们结束后自动触发安装。用户处理完成后再次点击
   “重启并更新”。
4. 只有检查为空闲时，Runtime 才原子进入 `preparing_update`，立即拒绝新的 Chat、恢复、Agent task
   和其他会产生运行状态的操作，避免检查通过后又创建新 Turn。
5. 随后刷新事件投影、统计批次和日志，关闭终端/后台进程，释放所有 SQLite 连接，最后才允许
   updater 替换 exe。

update gate 必须在 Runtime 中实现，而不是只在 React 禁用输入框。准备失败时解除 gate 并恢复正常
使用；检查/下载失败不能影响现有对话。当前 `RuntimeBridge`/`RuntimeService` 还没有桌面更新专用的
prepare/shutdown 生命周期，实现更新前必须补齐，不能依赖关闭窗口时的进程终止来释放数据库。

### Windows 替换与重启协议

Windows 运行中的 exe 不能替换自身。为了保持单一可执行 payload，不必长期安装第二个 helper：
下载完成的新 exe 本身以最小 updater 模式运行，该模式必须在 Wails 和数据库初始化之前处理。

```text
旧 AgentBuilder.exe
  -> 下载并验证 cache/updates/<version>/AgentBuilder.exe
  -> 启动新 exe --apply-update <token> --wait-pid <old-pid>
  -> 旧进程完成 prepare/shutdown、写 pending marker、退出
  -> updater 等待旧进程结束
  -> current exe -> AgentBuilder.exe.old
  -> updater 从自身已验证的文件内容复制出新的 current exe
  -> 启动 current exe --post-update <token>
  -> 新进程迁移数据库并写 healthy marker
  -> updater 退出，健康的新进程在后续清理候选缓存
  -> 失败则 updater 恢复数据库和旧 exe 后重启旧版本
```

`pending/healthy/failed` marker 放在 `data/cache/updates/<version>/`，使用临时文件加原子重命名，
至少记录 token、源/目标版本、目标路径、数据库 schema、阶段和时间。marker 不依赖待迁移数据库，
否则数据库打开失败时 updater 无法判断回滚。使用 application/update lock 防止多实例同时更新。

替换必须验证目标路径仍是最初检查过的 exe，拒绝符号链接/重解析点和路径漂移。保留的 `.old`
只在新版本连续健康启动后清理。若目录不可写、签名不匹配、hash 改变或无法取得锁，更新停留在
ready/failed，不退出当前应用。

### 数据库迁移与数据文件

正式更新前必须先替换当前数据库初始化策略。仓库虽然保留了带 `goose` 标记的 migration SQL，
但 `internal/db.Connect` 当前只验证 `schema_generation`；generation 不匹配时会备份 DB/WAL/SHM
并创建空库。这只适合开发期，发布后会表现为升级后会话消失，不能作为更新迁移机制。

#### 首版 baseline

项目尚未发布第一个稳定版本，因此不应把当前几十个开发期 migration 永久变成公共升级契约。
在 0.1.0 schema 冻结时执行一次 squash：

- 生成一个 `0001_v0_1_0_baseline.sql`，内容对应冻结后的完整 `schema.sql`。
- 现有 migration 移出生产嵌入目录，作为 pre-release 历史或测试参考；正式 runner 只看到 baseline
  和 0.1.0 之后的增量 migration。
- 新数据库执行 baseline，并写入版本 `1`；后续 migration 使用严格递增 `int64` 版本。
- 已有开发数据库不能重放 baseline。若它具有 `schema_generation=2`，runner 先执行结构探针、
  `quick_check` 和 baseline schema fingerprint；全部匹配后只登记为 baseline version `1`，不改业务表。
- 无法证明等价的旧数据库进入 bootstrap 恢复页，允许备份和导出；不得自动重建空库。

这样可以在首版发布前清理开发历史，同时确保从 0.1.0 开始的每个 migration 一经发布即不可修改。

#### 权威元数据

`schema_migrations` 是唯一 schema 版本权威，`runtime_settings.schema_generation` 在完成 legacy
baseline 接管后不再参与迁移判断：

```sql
CREATE TABLE schema_migrations (
    version       INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL,       -- baseline | migration
    checksum      TEXT NOT NULL,
    app_version   TEXT NOT NULL,
    applied_at    INTEGER NOT NULL,
    duration_ms   INTEGER NOT NULL
);
```

checksum 是 migration Up 内容规范化后的 SHA-256。每次启动都校验已应用版本在当前二进制中的
checksum；不匹配说明发布资产或数据库历史被修改，必须停止启动。表中只插入成功记录，不保存
`running/failed` 行；进行中的状态由数据库外的原子 marker 保存，避免事务回滚后丢失崩溃证据。

#### Runner 结构与接口

runner 由 `internal/db` 持有，复用 Goose 的 migration 文件格式和成熟 SQL 分段能力，但不允许
业务 Runtime 直接调用 Goose 全局 API。建议拆成 `migration_catalog.go`、`migration_runner.go`、
`migration_backup.go` 和平台 lock 文件，不新建第二套数据库连接管理。

```go
type MigrationPlan struct {
    CurrentVersion, TargetVersion int64
    LegacyBaseline                bool
    Pending                       []MigrationMetadata
    BackupRequired                bool
    EstimatedBackupBytes          int64
}

type MigrationResult struct {
    FromVersion, ToVersion int64
    BackupPath             string
    Applied                []MigrationMetadata
    DurationMillis         int64
}

func InspectMigrations(ctx context.Context, dataDir string) (MigrationPlan, error)
func ApplyMigrations(ctx context.Context, dataDir string, plan MigrationPlan,
    progress func(MigrationProgress)) (MigrationResult, error)
func RestoreMigrationBackup(ctx context.Context, backupPath, dataDir string) error
```

`MigrationPlan` 必须由 runner 根据实际数据库重新生成；调用方不能提交自定义目标版本、SQL 或
路径。`ApplyMigrations` 开始时再次验证数据库 identity、当前版本和 catalog checksum，防止检查
与执行之间状态变化。

#### 启动顺序与连接所有权

migration 必须发生在任何 Runtime worker 和业务数据库连接之前：

```text
desktop main
  -> resolve/ensure data layout
  -> acquire application + migration lock
  -> open bootstrap SQLite connection
  -> inspect / backup / migrate / validate
  -> close bootstrap connection
  -> create RuntimeBridge and Wails window
  -> db.Connect opens only target-schema business connection
```

需要把当前 `db.Connect -> ensureSchema -> backupAndRecreateDatabase` 拆开。新的 `db.Connect` 只做
连接和 target schema 校验；发现版本较旧、较新、dirty marker 或 checksum 不一致时返回明确的
typed error，绝不在普通业务调用中顺便迁移。这样不会出现两个并发请求同时触发 migration，也不会
让 migration 在 Runtime 已经启动后占用单连接。

跨进程使用 OS 文件锁，SQLite 内部再使用 `BEGIN IMMEDIATE` 防止其他写者。lock 文件本身不作为
“正在迁移”的事实；进程崩溃后 OS lock 会释放，runner 还必须读取外部 marker 决定恢复流程。

#### 单个 migration 规则

- 只允许 forward Up migration；生产回滚恢复备份，不执行 Down SQL。Down 可用于开发测试，但不
  能作为用户数据恢复方案。
- 默认一个 migration 一个事务，DDL、数据变换和 `schema_migrations` 插入必须形成同一成功边界。
- 生产 migration 禁止 `NO TRANSACTION`、`VACUUM`、网络、shell 和任意文件系统副作用。
- 不用 `IF NOT EXISTS` 掩盖未知 schema；预期对象不符合时应失败并进入恢复，而不是继续拼凑。
- migration 必须确定性、可在真实数据量上估算，并在发布后保持文件名和内容不可变。
- 简单加列/索引直接执行；表重写使用 new table -> copy/transform -> validate counts -> swap -> rebuild
  indexes/triggers 的 shadow-table 模式。
- 大数据变换若无法在可接受时间内完成，不应放在启动 migration；应先添加兼容 schema，启动后由
  可恢复 worker 分批 backfill，完成后的后续版本再删除旧字段。

Goose 默认的逐 migration 事务适合当前 SQLite 场景，但 Agent Builder wrapper 仍负责 checksum、
备份、外部 marker、进度和最终结构校验。如果选用的 Goose API 无法让版本/checksum 记录与 DDL
共享成功边界，应在 wrapper 中补齐 dirty 检测并在失败后强制恢复备份，不能静默接受半登记状态。

#### 备份、执行与崩溃恢复

每次存在 pending migration 时执行以下协议：

1. 获取 application/migration lock，确认 Runtime 尚未启动。
2. 写 `migration.json.tmp` 后原子改名为 `migration.json`，阶段为 `planning`。
3. 根据 DB/WAL 大小、pending migration 类型和 shadow-table 需求估算空间，确认可用空间足以
   容纳数据库备份、预计临时增长和安全余量；不足时在修改数据库前停止。
4. 执行 `wal_checkpoint(TRUNCATE)`，使用 SQLite backup API 创建一致备份到
   `data/backups/migrations/<from>-to-<to>-<timestamp>/agent-builder.db`。
5. 写入源数据库 hash、版本、目标版本、app version、backup path 和阶段 `backed_up`。Objects 等
   大文件不随普通 schema migration 备份。
6. 按版本执行 migration，每完成一个更新 marker 进度；SQL 失败立即停止。
7. 执行 `PRAGMA quick_check`、`PRAGMA foreign_key_check`、target schema fingerprint 和关键表探针。
8. 全部通过后把 marker 改为 `completed`，关闭 bootstrap connection，再允许 Runtime 启动。

如果进程在 `planning` 阶段崩溃且没有有效备份，可重新 inspect；在 `backed_up/applying/validating`
阶段崩溃时，下一次启动默认先恢复备份再重新规划，首版不尝试从未知的半迁移状态继续。空数据库
首次创建失败时可以删除未完成数据库并重试，因为其中还没有用户数据。

恢复时先把当前 DB/WAL/SHM 移入 `backups/migrations/failed-*` 作为调查证据，再从一致备份恢复；
恢复完成后重新执行 fingerprint 和 quick check。任何恢复失败都停留在 bootstrap 恢复页，提供日志、
备份位置和“打开数据目录”，不能继续启动一个可能损坏的 Runtime。

#### 版本兼容与验证

- 数据库版本低于二进制支持的最小版本：要求安装中间版本或使用离线迁移工具。
- 数据库版本高于当前二进制：拒绝打开，提示该数据已被新版本升级；不得自动执行 downgrade。
- 数据库版本等于目标但 checksum/fingerprint 不匹配：视为未知 schema，不修改数据。
- migration 成功不等于更新健康；仍需等待数据库重新打开、Runtime 基础查询和前端首次加载。

CI 必须维护各受支持 schema 的脱敏 fixture，并覆盖：空库创建、legacy baseline 接管、每个受支持
版本升级、真实行数据保留、WAL 中仍有提交、checksum 篡改、SQL 中途失败、进程在每个 marker
阶段被终止、磁盘不足、只读目录、数据库版本过新和两个实例竞争。另做 schema parity 测试，保证
“从 baseline 连续迁移到 target”的结构与目标 `schema.sql` fingerprint 一致。

新版本的健康标记不能在“进程创建成功”时写入。至少要等 migration 完成、数据库重新打开、schema
校验通过、Runtime 基础状态可读且主窗口完成首次加载。超过健康超时或进程异常退出即视为升级
失败。迁移备份保留最近 2–3 份并受独立容量策略管理，不与普通 cache 一起立即删除。

文件数据默认不迁移。如果未来调整 `projects/` 或 Object 布局，必须把文件迁移作为带 journal 的
独立步骤：先复制/生成新布局，校验引用和 hash，再原子切换数据库引用，最后延迟清理旧布局；
不能在成功前原地批量移动唯一副本。

### 状态机

```text
idle -> checking -> up_to_date
                 -> available -> downloading -> verifying -> ready
                 -> failed                               |
                                                        -> update_busy -> ready
                                                        -> preparing_update
                                                           -> applying
                                                              -> restarting
                                                                 -> migrating
                                                                    -> healthy
                                                                    -> rolling_back -> failed
```

Runtime 是检查、下载和 prepare 状态的权威；进程退出后由 marker 和 updater 状态机接管。下载使用
`.part` 文件，支持安全断点续传，完成验证后原子改名。启动时发现遗留 marker 必须先恢复/继续
更新，不能一边运行正常会话一边留下不确定的替换状态。

### Runtime 契约与设置

```go
type RuntimeUpdateSettings struct {
    Channel      string // stable | preview
    AutoDownload bool
}

type RuntimeUpdateState struct {
    Status, CurrentVersion, AvailableVersion string
    CurrentSchema, TargetSchema               int64
    ProgressPercent                          int
    ReleaseNotesURL, Error                   string
    ActiveTurns, ActiveTasks, PendingActions int
    CanInstall, RequiresInstaller             bool
    CheckedAt                                string
}
```

建议方法：`UpdateSettings`、`SaveUpdateSettings`、`CheckForUpdates`、`DownloadUpdate`、
`PrepareUpdateRestart`、`CancelPreparedUpdate`、`UpdateState`。
下载进度通过 Wails event `agent-builder:update-state` 发送；React 只渲染 Runtime 状态。

更新偏好和普通检查结果可以落 SQLite，但 pending apply 状态必须同时写独立 marker。通用页展示
当前版本、通道、自动下载、最后检查时间、下载大小和一个主动作按钮。ready 且 Runtime idle 时
按钮为“重启并更新”；存在活跃工作时点击后显示数量和“请先停止或等待完成”，不提供自动取消或
自动排队更新。

发布 CI 至少覆盖 exe/分发包构建与代码签名、manifest 生成与签名、从所有受支持 schema 的
升级测试、活跃会话拒绝更新、空闲检查与 update gate 竞争、迁移失败回滚、替换中断恢复、磁盘
不足、目录只读和多实例竞争。

## 实施顺序

1. 上下文统计：补 Runtime 聚合查询、DTO、热力图和现有治理区重排。
2. 诊断：先只实现模型失败、对话中断、工具失败和持久化失败，再补三个定向检查并让 `/doctor` 复用检查器。
3. 数据演进：先引入真正的 migration runner、schema 兼容契约、备份恢复和 bootstrap 错误页。
4. 更新检查：统一版本源，确定发布根地址、公钥和 CI，实现 manifest 校验、下载与通用页 UI。
5. Windows 更新：实现 Runtime 空闲检查/update gate、新 exe updater 模式、健康标记和联合回滚。
6. 发布扩展：按统一 Application Root 协议接入压缩包或安装器，并在有实际收益后评估 delta 更新。
