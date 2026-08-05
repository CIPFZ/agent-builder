# 内存与对话 UI 性能优化实施方案

## 1. 文档状态

- 状态：待实施
- 调查日期：2026-08-02
- 适用范围：Wails 桌面产品路径、Go Runtime、Canonical Conversation V2、React 对话时间线与工具详情
- 性能目标：运行和静默状态下，Agent Builder 整个进程树的 Private Memory 必须低于 1 GB

本文汇总当前内存问题的实测结果、根因、UI 信息架构调整、分阶段实施顺序与验收标准。实施时不得新增 HTTP、SSE、轮询或浏览器侧第二套会话权威状态；前端与 Runtime 继续只通过 Wails bindings 和 Wails events 通信。

## 2. 产品目标

### 2.1 内存目标

| 组件 | 静默目标 | 活跃目标 |
|---|---:|---:|
| Go Runtime | 150–220 MB | 250–350 MB |
| WebView2 renderer | 250–350 MB | 400–550 MB |
| WebView2 browser/GPU/utility | 150–200 MB | 180–250 MB |
| 进程树总计 | 550–750 MB | 750–950 MB |

任何常规对话、长会话浏览、流式输出或后台任务都不得突破 1 GB 硬预算。瞬时峰值不能依赖用户手动重启才能恢复。

### 2.2 加载与释放目标

- 只加载用户当前正在使用的 Session、Turn 和详情。
- 后台 Session 保留在 Go/SQLite，不在浏览器中持有完整投影。
- 折叠必须意味着详情 DOM 未挂载，而不只是通过 CSS 隐藏。
- 大型工具输入、输出、文件正文和 Diff 默认只传摘要与引用。
- 切换 Session、关闭详情或进入静默状态后，失效的 UI 缓存和订阅应自动释放。
- Runtime 中的队列、事件 ring、catch-up outbox、终端回放和执行 working set 必须有数量与字节双重上限。
- 多项目累计 1K–10K 个持久化 Session 时，静默内存、订阅、goroutine 和 DOM 不随 Session 总数线性增长。

### 2.3 用户体验目标

主时间线首先回答：Agent 做了什么、结果如何、是否需要用户处理。文件正文、原始 JSON、完整 stdout 和大型 Diff 只在用户主动检查时加载，不得淹没最终回答。

## 3. 当前实测基线

2026-08-02 对正在运行的桌面实例进行进程树采样：

| 组件 | Private Memory |
|---|---:|
| WebView2 renderer | 4,334.7 MB |
| WebView2 GPU | 161.9 MB |
| Go Runtime | 145.4 MB |
| 其他 WebView2 进程 | 71.5 MB |
| 总计 | 4,713.6 MB |

采样时当前页面只有 2 个 Turn 和少量可见消息，说明 4 GB renderer 占用不是当前可见 DOM 的合理工作集，而是重复事件、历史对象、隐藏 DOM 和高峰后未回收的 renderer 内存共同造成。

静默观察 10 秒后，renderer 保持约 4.4 GB，事件数量与数据库大小不再增长，但内存没有回落。当前产品缺少有效的闲置释放边界。

### 3.1 当前会话数据放大

| 数据 | 数量 | 内容大小 |
|---|---:|---:|
| 当前 `conversation_entities_v2` | 449 | 0.375 MB |
| `conversation_entity_events_v2` | 122,746 | 158.49 MB |
| 原始 `runtime_events` | 3,959 | 1.84 MB |

约 0.4 MB 当前物化状态产生了 158 MB canonical 派生事件，内容放大约 422 倍。642 个 raw event batch 各自包含超过 100 个 entity upsert；普通 `tool.call.output` 可重发约 260 个历史实体。

### 3.2 已通过的正向基准

- React canonical conversation 合成基准中，10,000 Turn 的 heap 增长约 58 MB。
- Go 侧 30-Turn canonical window 传输测试通过。
- 当前 LRU Session cache、30-Turn 初始窗口和 Turn 级虚拟化方向正确。

因此不应重写 canonical store 或 Timeline 基础架构。应优先关闭绕过有界路径的事件、catch-up、水化和详情挂载旁路。

## 4. 已确认根因

### 4.1 P0：Canonical diff 永久性全量重发

相关代码：

- `internal/runtime/runtime_conversation_projector_v2.go`
- `canonicalDiffEntityEvents`
- `canonicalStateRowAtSequence`

当前 deferred batch 的行为：

1. diff 使用包含 `revision`、`activitySequence` 和 `updatedAt` 的完整 entity JSON 比较。
2. 发生差异后，`canonicalStateRowAtSequence` 把差异行的 revision 改成当前 raw sequence。
3. 下一轮 snapshot 又生成实体自身的语义 revision。
4. 上轮整体抬高的 revision 与当前实体 revision 不同，历史实体再次被判定为变化。
5. 每次 deferred/reconcile 都可能重发整个 Session 的实体。

这同时造成：

- SQLite 派生事件表快速增长；
- Wails 持续传输重复 JSON；
- React 重复解析、复制和投影同一批对象；
- V8/Blink 产生极高分配压力，renderer 私有内存不回落。

### 4.2 P0：Catch-up 无字节上限

`SubscribeSessionConversationEventsV2` 首次 catch-up 没有设置 `LimitRawEvents`。底层 `listRange` 会一次读取旧 cursor 到当前 checkpoint 的所有 entity events。

旧 Session cache、短暂断流或恢复都可能让 WebView 一次收到数十到上百 MB JSON。JSON 转成 JS 字符串和对象后通常需要数倍内存。

### 4.3 P0：辅助水化仍读取完整 Session

Canonical Timeline 已使用 30-Turn window，但 `hydrateWorkbench` 在 full hydration 和 busy refresh 中仍可能调用：

- 完整 `SessionActivity`；
- Session 级 Hook execution 列表；
- 全部 Agent Tasks，并逐个加载详情和输出；
- React callchain、Prompt Assemblies 和其他辅助投影。

Go 侧 `SessionActivity` 使用 `limit=0`。`hydrateActivityForSelection` 即使最终只返回 window，也会先读取完整 Session Messages 再过滤。

忙碌状态下的周期性 refresh 会重复触发这些工作。前端的 `Promise.race(timeout)` 只让调用方超时，不会取消底层 Wails 请求，可能留下重叠的后台大请求。

### 4.4 P0：视觉折叠仍挂载完整 DOM

相关代码：

- `client/src/features/timeline/TraceRow.tsx`
- `client/src/features/tools/ToolCallCard.tsx`
- Ant Design `Collapse` 使用点

当前问题：

- `TraceRow` 关闭时仍渲染 `children`，只通过样式隐藏。
- `QuietToolRow` 关闭时仍创建 `ToolDetails`。
- 一个折叠工具组会挂载所有工具行；每行又挂载隐藏的 `<pre>`、Tag、按钮和输出预览。
- Collapse 首次展开后默认可能保留内容，未统一使用 `destroyOnHidden`。

这与“折叠完成的过程详情不挂载”的架构目标不一致。

### 4.5 P0：Canonical ToolCall 携带完整工具输入

`RuntimeCanonicalToolCall.InputJSON` 当前直接使用完整 `call.InputSummary`。`validJSONObjectOrEmpty` 只验证 JSON 合法性，没有大小限制。

对于 Write/Edit/MultiEdit、Shell 或自定义 MCP 工具，输入可能包含：

- 完整新文件正文；
- old/new content；
- 大型 patch；
- 长命令或脚本；
- 大型结构化参数。

即使 UI 不显示，这些字符串也已经进入 canonical store 和 React view model。

### 4.6 P1：派生事件和缓存缺少生命周期上限

- `conversation_entity_events_v2` 和 projector batch 作为 catch-up outbox 永久增长。
- 浏览器缓存主要按 Session 数量限制，缺少总字节预算。
- 加载过的全文、Tool Object、Diff、Agent Task 输出没有统一全局 LRU。
- xterm.js runtime 与终端订阅主要按关闭 Tab 释放，隐藏或切换 Session 后仍可能保留 renderer working set。
- Runtime 支持任意 Session 执行，但缺少明确的重型 working set 并发准入预算。

### 4.7 P0：1K+ Session 当前仍走全量列表路径

真实使用场景中，多个项目累计 1,000 甚至 10,000 个 Session 是持久化容量问题，不应该等价为 1,000 个常驻 Runtime。当前实现尚未做到这一点：

- `internal/db` 的 `ListSessions` 没有 `LIMIT` 或 cursor，会读取全部未删除的顶级 Session。
- `runtimeService.Sessions()` 将全量记录转换为 DTO；`SidebarProjection()` 又调用 `Sessions()`，完整 Workbench hydration 同时还会单独调用 `bridge.Sessions()`，存在重复查询与重复跨 Wails 传输。
- `Sidebar.tsx` 对每个项目执行一次 `viewModel.sessions.filter(...)`，复杂度约为 `O(projects × sessions)`。
- 展开的项目直接 `map` 全部项目 Session，独立 Session 分组也直接挂载全部行；没有分页、虚拟列表或 DOM window。
- 现有 Session 索引覆盖了项目、scope、pinned 和更新时间，但列表契约没有稳定 cursor；分页索引还应加入唯一 `id` 作为同时间戳下的 tie-breaker。
- 已完成 Turn 的执行态索引已有 `releaseFinishedTurnMemory` 清理，这是可保留的正确基础；但项目 capability load、终端、前端最近会话对象和各类详情缓存仍没有统一的全局资源管理器。

因此，如果只修复单会话 Timeline，1K+ Session 仍会在每次刷新时制造全表读取、Go slice、JSON 编码、JS 对象、React diff 和 DOM 的叠加成本。

## 5. 总体优化架构

```text
SQLite canonical state / Object Store
  -> 有界 window snapshot 或有界 entity batch
  -> React normalized active-session store
  -> Turn selector
  -> 高信号过程摘要
  -> 单行工具摘要
  -> 用户点击后按需请求 Tool Detail / Object chunk
```

Session 容量采用四层模型，Session 数量与活跃资源数量彻底解耦：

| 层级 | 含义 | 允许常驻的资源 |
|---|---|---|
| Cold / 持久化 | 绝大多数历史 Session | 仅 SQLite 行和索引；无 goroutine、timer、subscriber、prompt、terminal、React conversation store |
| Warm / 摘要 | 最近、置顶、运行中或搜索命中的少量 Session | 有界 SessionSummary cache，不含消息、工具正文和 canonical entities |
| Active / 后台活跃 | 100+ 个正在运行、调度、等待权限或重试的 Session | Runtime 中仅保留可恢复的轻量状态机；浏览器只接收状态摘要 |
| Hot / 聚焦展示 | 用户当前聚焦的 1 个 Session | 稳态 1 个 active stream 和 1 个 canonical window store；切换期间短暂允许新旧两个 store，完成后立即释放旧 store |
| Resident execution / 驻留执行 | 当前步骤已通过某类资源准入的 Turn | Model、Tool、Shell、Browser 等分别限流并设置字节预算，不使用单一“执行数”上限 |

“支持 1K+ Session”的含义是可以低成本持久化、检索和打开任意 Session，并允许其中 100+ 个处于逻辑活跃状态，而不是预加载 1K 份对话状态或让 100 个任务同时占用重型资源。

必须始终满足：

- Canonical store 保存当前可恢复实体，不保存 UI 展开状态。
- Runtime events 和 entity outbox 只承担恢复与失效通知，不作为无限历史副本。
- Timeline 不接收完整原始工具输入和输出。
- 完整内容在 Runtime Object Store 中，通过明确的按需读取接口访问。

## 6. 分阶段实施方案

### 6.1 P0-A：修复 canonical 事件放大

#### 实施要求

1. Canonical semantic diff 排除：
   - `revision`
   - `activitySequence`
   - `updatedAt`
2. 只有语义内容真正改变的实体才能生成 upsert。
3. 只有发生变化的实体推进到当前 raw sequence revision。
4. 普通 raw event 使用直接实体映射，不允许通过全量 snapshot diff 更新无关实体。
5. reconcile/deletion 等确需全局 diff 的事件也必须基于语义内容比较。

#### 回归要求

- 单个 `message.updated` 只更新目标 Message 及必要的 AssistantStep。
- 单个 `tool.call.output` 只更新目标 ToolCall/ToolResult。
- 100 个实体的 Session 中，更新一个实体不能产生 100 个 upsert。
- 同一语义状态重复投影产生零 entity events。
- 1,000 个 raw events 的派生事件数量保持 O(1,000)，禁止 O(raw events × historical entities)。

### 6.2 P0-B：为 catch-up 和 Wails batch 增加硬上限

建议默认值：

| 项目 | 上限 |
|---|---:|
| 单次 raw sequence catch-up | 100–256 |
| 单个编码 batch | 1–2 MB |
| 单 batch entity 数量 | 256 |
| durable subscriber queue | 数量和字节双限制 |

超限行为：

1. 不分割同一 raw sequence 的原子 entity batch。
2. 若单个 raw sequence 本身超过 transport 上限，返回 `snapshotRequired`。
3. cursor 落后超过 retention floor 时返回 `snapshotRequired: retention`。
4. React 重新请求最新 30-Turn window，不尝试接收完整历史补丁。

任何 Wails event 都不得携带几十 MB payload。

### 6.3 P0-C：移除全量 Workbench 水化旁路

#### Conversation 主路径

- Timeline、Todo、Tool、Permission、AgentTask 生命周期只消费 canonical store。
- 普通 runtime event 只能触发明确 opt-in 的非 conversation refresh target。
- busy 状态不再每 3 秒执行完整 workbench hydration。
- Session 切换使用 snapshot readiness，不等待诊断/设置/任务详情。

#### 辅助能力

- Activity 使用 `TurnActivity` 或明确 limit 的 window。
- Hook 列表只返回当前 Turn 或有界高信号摘要。
- Agent Task 列表只返回摘要；消息、结果和输出在打开任务详情时加载。
- Prompt Assembly、React callchain、诊断只在对应面板打开或故障触发时读取。
- Settings、Plugins、MCP、Provider Catalog 只在启动摘要或对应页面打开时加载。

#### 取消与 singleflight

- Session switch、generation 变化、超时和组件卸载必须取消底层 Wails 调用。
- 同一 Session/资源类型最多一个 in-flight request。
- `Promise.race(timeout)` 不能作为取消实现。
- 迟到响应不得进入当前 Session view model。

### 6.4 P0-D：工具 UI 改为真实按需挂载

#### DOM 规则

- `TraceRow`：`open === false` 时不渲染 `children`。
- `QuietToolRow`：关闭时不创建 `ToolDetails`。
- Ant Design Collapse 统一设置 `destroyOnHidden`。
- Process、Tool Group、Tool Item 三层折叠均需验证详情 DOM 数量为零。
- 完整内容 Drawer 关闭时清空正文 state。
- Session 切换时关闭 Drawer 并清除详情缓存。

不要把 `display:none`、`max-height: 0`、透明度或离屏定位视为内存卸载。

#### 组件结构

建议将详情从每个 Tool Row 中移出，改为 Workspace 级唯一实例：

```text
Timeline Tool Row --select toolCallId--> ToolDetailDrawer
ToolDetailDrawer --Wails request--> Runtime Tool Detail / Object chunks
```

浏览器只保存当前选择的 `toolCallId` 和当前 Drawer 的有界内容。

### 6.5 P0-E：Canonical ToolCall 摘要化

将 `InputJSON` 从主 snapshot/event DTO 中移除或降级为严格有界预览。建议契约：

```go
type RuntimeCanonicalToolCall struct {
    ID, SessionID, TurnID string
    Name, Kind, Status     string
    CommandPreview         string // bounded
    InputPreview           string // <= 1 KiB
    InputByteLength        int64
    InputTruncated         bool
    InputRef               string
    Targets                []string
    WorkingDir             string
    ChangeStats            RuntimeToolChangeStats
    ResultIDs              []string
}
```

完整 input、stdout、stderr、structured output、文件正文和 patch 保存到 Object Store。主 canonical transport 只返回摘要、长度、统计和引用。

建议不同类型的 preview：

| 工具类型 | Canonical 默认内容 |
|---|---|
| Read | 路径、行范围、读取行数/字节数，不传正文 |
| Write | 路径、新增/覆盖、行数、字节数、hash，不传新文件正文 |
| Edit | 路径、hunk 数、`+N/-N`、diff ref |
| Search | query/pattern、匹配数、前几个位置 |
| Shell | 单行 command preview、cwd、exit code、输出长度、output ref |
| MCP/Generic | server/tool、目标、状态、有界字段摘要、raw input ref |

ToolResult preview 也按工具类型处理：成功 Read/Write 默认不传正文；Shell 失败优先传末尾错误；Search 只传少量结果；Edit 传有界 Diff 摘要。

### 6.6 P1-A：用户视角的工具信息层级

#### 第一层：主时间线

只展示高信号汇总：

```text
✓ 检查并修改代码 · 38 秒
  读取 12 个文件 · 修改 3 个文件（+84 −21）· 运行 4 条命令
```

运行中只突出当前动作和累计进度：

```text
处理中
  正在编辑 internal/runtime/runtime_service.go
  已完成 18 项操作
```

完成后，成功且低风险的 Read/Search/Write/Edit 合并为统计。以下情况直接可见：

- 等待权限；
- 工具失败或非零退出；
- 高风险写入/删除；
- 产生需要用户确认的 Diff/Artifact；
- Runtime 判断为 attention-required 的结果。

#### 第二层：工具组

展开工具组只显示单行摘要：

```text
✓ runtime_service.go        读取 120–260 行
✓ runtime_sessions.go       读取 538–645 行
✓ ToolCallCard.tsx          读取 200–372 行
                         显示其余 9 项
```

每行只包含类型、目标、状态、耗时和结构化统计。超过 20–30 项时分页或虚拟化，不一次挂载全部行。

#### 第三层：统一 Tool Detail Drawer

点击单项后打开唯一详情抽屉：

| 工具 | 默认详情 |
|---|---|
| Read | 路径、行范围、文件大小；按需加载约 20 行预览 |
| Write | 路径、行数、字节数；展示变更摘要而不是整份新文件 |
| Edit | `+N/-N`、hunk 数；有界 unified diff |
| Search | 条件、匹配数、前 5–10 个位置 |
| Shell | command、cwd、exit、耗时；成功输出默认折叠，失败显示尾部错误 |
| Network/MCP | Server、操作、状态、耗时、返回对象数量 |

写文件示例：

```text
写入 docs/report.md
新增文件 · 246 行 · 8.4 KB

+ # Memory Optimization Report
+ ...

查看完整 Diff | 打开文件 | 复制路径
```

#### 第四层：原始输入与完整输出

- 放在 Drawer 次级菜单中。
- 用户点击前，完整内容不得进入浏览器。
- 通过 Object Ref 分块读取，每次建议 32–64 KiB。
- 大文件使用虚拟文本/Diff Viewer。
- “复制完整内容”优先由 Runtime 读取引用并写入剪贴板，避免完整字符串进入 DOM。

### 6.7 P1-B：浏览器缓存和详情释放

| 缓存 | 建议上限 |
|---|---:|
| canonical active/recent Session stores | 2 个 Session 且总计 <= 64 MB |
| 全文/Tool Object/Diff 详情 | 总计 16–32 MB |
| 单 Drawer 初次加载 | 64 KiB |
| 单 Timeline 文本预览 | 2 KiB / 20 行 |

释放规则：

- 切换 Session：清除旧 Session live delta、全文、诊断和详情。
- 关闭 Drawer：立即删除 full content state。
- 详情空闲 60 秒：从 LRU 淘汰。
- 大 Session 离开后：只允许保留最新 30 Turn window，否则直接驱逐。
- 缓存按实际 UTF-8 字节估算，不只按对象数量限制。

### 6.8 P1-C：Canonical outbox 保留策略

`conversation_entities_v2` 是当前物化状态；`conversation_entity_events_v2` 与 `conversation_projector_batches_v2` 是可重建的 catch-up outbox，不应永久保留。

建议每个 Session 同时应用：

- 最近 1,000–2,000 raw sequences；
- 最大 16–32 MB；
- 可选时间上限 10–30 分钟；
- 不早于活跃 subscriber 的安全 cursor。

落后于 retention floor 的客户端必须重新 snapshot。清理 outbox 后保留最新 materialized entities 和 checkpoint。

现有异常数据需要一次维护迁移：

1. 验证 `conversation_entities_v2` 与 Runtime canonical snapshot 一致。
2. 清理旧 entity events 和 projector batches。
3. 把 checkpoint 对齐当前 raw sequence。
4. 记录 retention floor。
5. SQLite 文件缩容使用显式闲置维护，不在普通启动路径直接执行 `VACUUM`。

### 6.9 P1-D：终端、MCP、LSP 与执行 working set

#### 终端

- 终端进程可以继续由 Go Runtime 管理。
- 只有当前可见 Tab 创建 xterm renderer 和订阅。
- 隐藏/切换 Session 时销毁 xterm DOM、addon 和浏览器订阅。
- 重新打开时读取 Runtime 有界 replay；更早输出通过 Object Ref 查看。

#### MCP/LSP/Provider

- 按项目和活跃 Turn 延迟创建。
- 无活跃消费者后使用空闲 TTL 关闭连接和 worker。
- Skills/MCP catalog 只加载摘要；资源、prompt 和 tool schema 在实际启用或设置页打开时加载。

#### 执行并发

- 支持 100+ Session 同时处于逻辑活跃状态，包括调度中、等待 Model、执行 Tool、等待权限、退避重试和可恢复挂起。
- 不使用单一的“最多 2 个 Executing Session”限制；Model 网络请求、重型 Tool、Shell 子进程、Browser、Terminal 和项目能力分别进行资源准入。
- 每个任务只在执行当前步骤时加载必要 Prompt、Tool 和 Provider 状态；步骤结果持久化后立即释放临时 working set。
- 等待权限、等待重试、等待某类资源和可恢复暂停时，只保留 Session/Turn ID、状态、阶段、revision 和调度元数据，不保留完整 Prompt、消息历史或 Tool output。
- 超出某类资源预算时进入对应的 durable runnable queue，不把等待任务上下文常驻内存，也不能把“正在调度”错误展示为“正在执行工具”。

### 6.10 P1-E：多项目与 1K+ Session 容量治理

#### 列表查询与 Wails 契约

新增轻量 `SessionSummaryPage` 契约，只包含：`id/title/projectId/scope/status/pinned/updatedAt/active/busy` 以及必要的短摘要字段，不携带 Todos、消息、工具、上下文或诊断对象。

- 首屏只读取 30–50 条；每页最多 50–100 条，并增加编码后 128–256 KiB 的响应上限。
- 使用 keyset cursor，不使用随数据量增大而退化的 offset。cursor 至少包含 `pinned + updatedAt + id`。
- 分别支持 `projectId/scope/query/status/pinned` 过滤；搜索由 SQLite 完成，不先把全部 Session 拉入浏览器过滤。
- 为实际查询增加匹配的复合索引，例如 `(project_id, deleted_at, pinned DESC, updated_at DESC, id DESC)` 和 standalone 对应索引。
- `SidebarProjection` 只组合项目摘要、首屏 Session page、运行中 Session summary 和当前选择 ID；不得内部再调用全量 `Sessions()`。
- Workbench hydration 不再并发请求 `SidebarProjection` 和 `Sessions()`；同一 generation 同一资源只允许一个 in-flight 请求。
- Session 新建、改名、置顶、状态变化和删除通过 `{id, revision, changedFields}` 增量更新可见 page，不触发“刷新全部 Session”。page 失效时只重取当前 page。

#### 侧边栏信息架构与 DOM

侧边栏优先展示“运行中”“置顶”“最近”三个有界分组，项目展开后只显示该项目首屏，并提供“查看更多”或项目内搜索。这样用户寻找会话的主要路径是最近访问、置顶和搜索，而不是滚动 1,000 行。

- 项目数和 Session 数较大时使用虚拟列表，只挂载可见行和 overscan，总 Session row DOM `< 150`。
- Go 返回已分组或可按 `projectId` O(1) 索引的 DTO；React 不再为每个项目扫描整个 Session 数组。
- 折叠项目后卸载其 Session rows，并释放项目 page 对象；只保留 cursor、总数和展开偏好。
- Session 总数、项目计数使用单独的聚合查询，不通过 `array.length` 暗示数据已全量加载。
- 搜索输入 debounce 并可取消；只保留最新 generation 结果，迟到响应不得写回当前列表。

#### Runtime 资源准入与回收

- Cold Session 不允许拥有专属 goroutine、ticker、event subscriber、provider client、MCP/LSP worker、terminal buffer 或 prompt/history working set。
- 浏览器使用一个 Workspace 级状态流接收所有后台活跃 Session 的有界摘要；详细 canonical stream 只订阅当前聚焦 Session。切换过的 100 个 Session 不能留下 100 个详细订阅。
- 后台活跃摘要只包含 `sessionId/projectId/status/phase/progressLabel/updatedAt/unread/revision` 等短字段，单条目标 `<= 2 KiB`，不得包含消息、Prompt、Tool input/output、Diff、终端输出或 Agent Task 明细。
- 浏览器稳态只保留当前聚焦 Session 的 canonical window store，并受 `<= 48–64 MB` 字节预算约束。切换期间可以短暂同时存在新旧 store，但新 Session 首屏提交后必须立即释放旧 store。
- Runtime 可以维护 100+ 个轻量可恢复任务状态机，但只有当前步骤通过对应资源准入的任务才能创建 Prompt、Provider 请求、Tool buffer、子进程或项目能力 working set。
- 终端属于显式活跃资源，不随 Session 切换强制杀死正在执行的命令，但必须有全局数量、输出 buffer 字节和空闲 TTL 上限；已退出或长期空闲终端主动关闭并从 ownership map 移除。
- MCP、LSP、Skills 和项目 capability 按项目延迟加载，以 `{projectId, capabilityRevision}` 共享；无活跃 Turn、可见面板或终端消费者后按 TTL 驱逐。`capabilityLoads` 等记录型 map 也必须有数量/TTL 上限或持久化后删除。
- 建立统一 Resource Governor，记录 logical active tasks、active streams、in-flight model requests、各类 Tool、子进程、terminals、provider clients、project capabilities 和各缓存的 count/bytes/lastUsed；超限时让任务进入对应等待队列或驱逐 LRU，不依赖 GC 猜测。
- 项目切换只改变选择和加载目标项目摘要，不遍历或 hydrate 该项目全部 Session，也不预热所有项目能力。

聚焦 Session 切换必须遵循以下生命周期：

```text
选择 Session B
  -> 取消 Session A 的详细请求与 canonical 订阅
  -> 卸载 A 的 Timeline、Drawer、Diff、Markdown AST、xterm UI
  -> 仅在 Workspace 状态索引中保留 A 的短状态
  -> 获取 B 的最近 conversation window
  -> 提交 B 的首屏并建立唯一详细流
  -> 释放 A 的 canonical store
  -> B 的 Tool/Diff/Terminal 详情继续按需加载
```

建议第一版硬预算如下，后续根据基准数据调整，但不能取消边界：

| 资源 | 默认硬上限 | 超限行为 |
|---|---:|---|
| logical active Session 状态机 | 100–500 | 仅保留 durable 状态与轻量调度元数据 |
| Workspace 后台状态流 | 1 | 合并、去重并按 revision 更新摘要 |
| active canonical stream | 1 | 取消旧订阅后再切换 |
| canonical Session stores | 稳态 1 个、切换瞬时 2 个，合计 <= 64 MB | 新首屏提交后立即释放旧 store |
| 后台 ActiveSessionStatus index | 单条 <= 2 KiB、合计 <= 1 MB | 只保留活跃/attention 状态并压缩低频字段 |
| sidebar SessionSummary cache | <= 3 pages 且 <= 2 MB | 驱逐最久未访问 page |
| in-flight Model 请求 | 初始 16–32 | 进入 model runnable queue，按 Provider 限额与压测调整 |
| 重型 Tool working sets | 2–4 | 进入 tool runnable queue |
| Shell/编译子进程 | 4–8 | 进入 process runnable queue |
| Browser/Computer worker | 1–2 | 进入 browser runnable queue |
| 可见 xterm renderer/subscriber | 1 | 销毁旧 renderer 与 UI 订阅 |
| Runtime terminal output buffers | 全局 <= 32 MB | 截断 replay、落 Object Store 或关闭空闲 terminal |
| warm project capability sets | <= 2 个项目且 <= 32 MB | TTL/LRU 关闭 worker 并驱逐 schema |
| Tool/Diff/full-content detail cache | <= 16–32 MB | 关闭 Drawer 或 LRU 驱逐 |

#### 数据保留与维护

1K+ Session 可以永久保留摘要，但派生 outbox、诊断明细、终端 replay 和可重建缓存不能永久随 Session 数量增长。应按数据类型配置 TTL、每 Session 字节上限和全局字节上限。归档/删除使用软删除和后台小批次清理；SQLite checkpoint、`PRAGMA optimize` 和可选 `VACUUM` 只在真正空闲且满足文件膨胀阈值时执行，不能阻塞启动或活跃 Turn。

### 6.11 P2：WebView2 空闲内存守卫

结构性修复后，产品不应依赖强制 GC。但 WebView2/V8 可能不主动归还历史高峰的 committed pages，因此增加最终兜底：

1. 无活跃 Turn、Permission、Tool、Terminal interaction 和未保存草稿。
2. 窗口闲置 2–5 分钟。
3. renderer 持续超过约 750–800 MB。
4. 先驱逐 UI cache、详情、Markdown AST、Diff Viewer、xterm renderer。
5. 若仍不回落，保存纯 UI 状态后受控重载 WebView。
6. 重载后从 Runtime snapshot 恢复 active Session、scroll target 和必要 UI preference。

强制 `window.gc()` 只能用于测试，不作为产品内存治理机制。若当前 Wails/WebView2 版本支持 suspend 或低内存目标 API，可在实测有效后作为补充，但不能替代有界数据结构。

## 7. UI 与 DOM 性能预算

| 场景 | 硬目标 |
|---|---:|
| 完成状态单 Turn 默认 DOM | < 100 nodes |
| 活跃 Turn、100 个工具默认 DOM | < 500 nodes |
| 折叠工具组的详情 DOM | 0 nodes |
| 同时挂载完整 Tool Detail | 1 |
| 单 Timeline preview | <= 2 KiB / 20 行 |
| Tool group 首屏行数 | <= 20–30 |
| Drawer 初次内容 | <= 64 KiB |
| 全局详情 cache | <= 16–32 MB |
| 侧边栏初次 Session rows | <= 50 |
| 侧边栏已挂载 Session rows（含 overscan） | < 150 |
| 侧边栏首屏 Wails payload | <= 128–256 KiB |

可使用 `content-visibility` 和 CSS containment 减少布局/绘制，但它们只是补充；未挂载仍是首要规则。

## 8. 内存与性能验收矩阵

### 8.1 进程树

- 全新启动静默 10 分钟：总 Private Memory `< 700 MB`。
- 普通对话活跃：总 Private Memory `< 900 MB`。
- 打开 1,000-Turn Session：峰值 `< 900 MB`。
- 连续流式输出 60 分钟：预热后增长斜率 `< 1 MB/min`。
- 离开大 Session 后 60 秒：回到启动基线 `+150 MB` 以内。
- 任何标准验收场景不得超过 1 GB。

### 8.2 Canonical transport

- 单 durable Wails batch `< 2 MB`。
- 一次 entity update 不扫描或重发无关 Turn。
- stale cursor 超限后走 snapshot recovery。
- 30-Turn window 编码保持现有 `< 256 KiB` 门槛。
- 静默 Runtime event rate 为零。

### 8.3 UI

- Process/Tool Group/Tool Row 折叠后详情 DOM 为零。
- 展开 100 个工具的 Group 时只挂载可见 window。
- 收起 100 个读写工具后，一次 GC 后 heap 回到展开前 `+10 MB` 以内。
- Drawer 关闭后全文字符串不可从 React state、detached DOM 或 event listener 路径访问。
- 切换 Session 不保留旧 Session 的 Tool Drawer、Diff、Markdown AST 或 xterm renderer。

### 8.4 数据库

- entity outbox 大小随保留窗口稳定，不随 Session 生命周期无限增长。
- 1,000 raw events 的派生事件与存储保持线性。
- materialized canonical state 与窗口 snapshot 在 outbox 清理前后相同。

### 8.5 多项目与 1K+ Session

- 1,000 个 Session 分布在多个项目时，启动和侧边栏首屏最多传输 50 个 SessionSummary，编码后 `<= 256 KiB`。
- 从 1,000 增长到 10,000 个 Cold Session 时，除 SQLite page cache/索引的少量变化外，静默 Go heap、renderer heap、goroutine、subscriber 和 DOM 数量不随 Session 总数线性增长。
- 本地 SQLite 暖缓存下，侧边栏首屏查询目标 `< 200 ms`，打开普通 Session 并得到可绘制 window 目标 `< 500 ms`。
- 100+ Session 同时逻辑活跃时，浏览器只有 1 个 Workspace 状态流、1 个聚焦 Session 详细流和稳态 1 个 canonical store；后台只持有 `<= 1 MB` 状态索引。
- 100+ 逻辑活跃任务按 Model、Tool、Shell、Browser 等资源分别准入；等待某类资源的任务不持有 Prompt、Provider payload、Tool output 或子进程 working set。
- 连续切换 100 个 Session 后：active canonical stream `= 1`，canonical store 稳态 `= 1`，不存在遗留 terminal UI subscriber、timer、Drawer content 或迟到请求写回。
- 创建 1,000 个 queued/runnable Turn 时，只有通过各类 Resource Governor 配额的步骤持有重型 working set；其余只占用 durable queue 行和轻量调度状态。
- 侧边栏任何滚动位置挂载的 Session rows `< 150`，折叠项目后该项目 row DOM 为零。
- 静默 10 分钟期间 Session/项目相关 event rate 为零；Resource Governor 指标稳定，无周期性全量 Session 查询。

## 9. 建议测试与工具

继续使用：

```text
cd client && npm run benchmark:conversation-performance
go test ./internal/runtime -run TestCanonicalConversationWindowTransportStaysBounded -count=1
scripts/conversation-memory-profile.ps1 -DurationSeconds 600 -IntervalSeconds 2 -MaxPrivateMB 900
```

新增测试：

- canonical semantic diff amplification regression；
- durable batch byte cap 和 stale cursor recovery；
- Workbench refresh 不调用 full SessionActivity；
- Wails request cancellation/singleflight；
- collapsed Tool DOM structural test；
- Tool Detail Drawer lazy-load 与 close-release test；
- canonical ToolCall 不包含完整 write/edit content；
- 100/1,000 tool rows virtualization benchmark；
- outbox retention 和 checkpoint recovery；
- 60 分钟流式输出 heap plateau 测试。
- 1K/10K Session seed 后的分页查询、Wails payload 和启动内存基准；
- keyset cursor 在相同 `updatedAt`、插入、置顶和删除并发下无重复、无漏项测试；
- 100 次 Session 切换后的 stream、subscriber、timer、cache 与 detached DOM 泄漏测试；
- 100+ logical active Session 的状态摘要内存、Workspace 状态流合并和聚焦 Session 唯一详细流测试；
- 1,000 queued/runnable Turn 的多维 Resource Governor、公平性、背压和重启恢复测试；
- Model 32 并发、重型 Tool 4 并发、Shell 8 并发等独立资源压力测试，并根据 `<1 GB` 结果收紧默认值；
- 多项目 capability lazy-load、共享、TTL 驱逐和项目切换不全量加载测试。

## 10. 推荐提交顺序

为了便于验证和回滚，按以下独立提交落地：

1. 修复 canonical semantic diff，并加入放大回归测试。
2. 增加 catch-up/batch 字节上限与 snapshot recovery。
3. 清除 full SessionActivity/Workbench hydration 旁路及请求重叠。
4. 折叠详情真实卸载，统一 `destroyOnHidden`。
5. Canonical ToolCall/ToolResult 摘要化与 Object Ref 接口。
6. 实现 Workspace 级唯一 Tool Detail Drawer。
7. 工具组汇总、虚拟列表和按工具类型的用户展示。
8. 浏览器字节 LRU、Session switch 和空闲释放。
9. outbox retention、现有异常派生数据维护迁移。
10. SessionSummary keyset 分页、SidebarProjection 去重和增量 summary 更新。
11. 侧边栏最近/置顶/运行中分组、项目分页搜索与虚拟列表。
12. Resource Governor、Cold/Warm/Hot/Executing 生命周期和 durable queue 准入。
13. 终端/MCP/LSP/capability lazy lifecycle、TTL 与全局预算。
14. WebView2 空闲内存守卫。
15. 完成 10 分钟、60 分钟、长 Session 和 1K/10K Session 桌面验收。

## 11. 完成标准

只有同时满足以下条件，才能认为本轮优化完成：

- 修复事件全量重发，数据库不再产生二次方放大。
- 所有 conversation transport 都有数量和字节边界。
- 产品主路径不再读取完整 SessionActivity。
- 大型 Tool input/output 不进入默认 canonical snapshot/event。
- 折叠过程和工具详情真实卸载 DOM。
- 用户可以从摘要逐层进入单项详情和完整原始内容。
- Session 切换和静默状态会自动释放失效缓存。
- 1K/10K 个持久化 Session 不会变成等量内存对象、DOM、goroutine、timer 或 subscriber；列表始终分页且按需加载。
- 支持 100+ Session 逻辑活跃，但 UI 稳态只完整加载 1 个聚焦 Session，其他 Session 只保留有界状态摘要。
- queued/runnable 状态与驻留执行资源分离，Model、Tool、Shell、Browser、Terminal 和项目能力均有独立准入及字节预算。
- 标准运行、长会话、流式输出和静默场景的整个进程树均稳定低于 1 GB。
