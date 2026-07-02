# 对话输入/输出 UI 重构方案(两阶段展示 + 流式 + 聚合)

本文承接 `11-conversation-output-rendering-refactor-plan.md`,在其后端投影已落地 60~70% 的基础上,针对用户实际反馈("长任务大量不聚合的工具卡片、UI 丑、无流式")给出完整实施方案,并把主交互理念收敛为"两阶段 turn"。

## Context(为什么做)

### 当前展示问题

- **工具卡片墙**:runtime 分组策略只允许"已完成的 read/search"聚合(`runtimeToolGroupable == runtimeToolQuiet == completed && kind∈{file_read,file_search}`),shell/edit/write/failed 工具永不聚合;每个工具都是一张 antd Collapse 卡片,视觉很重。
- **无流式**:前端 1.2s 轮询全量快照 + 全量 `hydrateWorkbench`,内容以"最终状态"弹出,没有打字机文本、没有实时工具状态。
- **前后端重复推断**:Timeline.tsx 有第二套 `buildTurnBlocks`/`isFinalAssistantMessage`/`compactProcessItems`/`groupAdjacentToolCalls`,与 runtime 投影重复且不一致;`toRuntimeToolCall` 与 `applyConversationToolPolicy` 也重复计算 kind/quiet/groupable/defaultExpanded。

### 参考项目共识(cc-haha / claude-code / DeepSeek-GUI)

三个项目独立收敛到同一套模式:
- 结果合并进调用行(tool_result 不作为独立行)
- 连续静默工具折叠为**一行分类计数摘要**("已读取 12 个文件 · 运行 2 条命令")
- 成功默认折叠、错误自动展开
- 50ms delta 缓冲的流式渲染
- 子 agent 递归复用同一渲染器 + "+N more" 截断

### 用户决策(binding)

- **核心交互理念**:一个 turn 分两阶段——(1) **探索阶段**(思考/工具/子agent),完成后默认折叠为一行摘要;(2) **最终答复**,突出展示。过程可展开细看。
- **流式全做**:增量事件 + assistant 文本 delta 打字机 + 实时工具状态。
- **Composer 本轮只优化样式**,不加 slash 命令/附件/排队。
- **继续以 antd / antd X 组件为主**,不自造组件体系(评估 antd X `ThoughtChain` 做探索链;`CodeHighlighter` 已在依赖里)。
- **架构不变量**:Go runtime 是唯一事实源;React 只消费 adapter view model;契约需同步 Wails bridge + HTTP + runtimeapi + 前端 adapter + 测试。

## 已验证的关键事实

- **文本 delta 已到达 runtime 进程**:`internal/agent/agent.go:395/419` `OnReasoningDelta`/`OnTextDelta` 每 token 调 `a.messages.Update()` → `internal/message/message.go` pubsub 全量消息 → `internal/runtime/runtime_events.go` `recordRuntimeEvent`。**无需改 agent loop,tap 点在 `recordRuntimeEvent`**。
- **推送通道已有模板**:Wails 端 `desktop/runtime_bridge.go:521` `StartTerminalEventStream`(`app.Event.Emit` + batcher);HTTP 端 `/v1/events` SSE(runtime_http.go handleEvents)。前端目前两者都不用,强制 1.2s 轮询 + 全量 hydrateWorkbench。
- **SessionOutputEvents 后端已实现**(`runtime_output.go:31`)但前端从未调用;前端 `applyOutputEvent` reducer 已实现但是**死代码**。
- **写放大隐患**:每 delta 已经写一次 SQLite `messages UPDATE` + 一条 `message.updated` runtime event(ring buffer 上限 200,长 turn 必然溢出触发 `snapshot_required`)→ 流式 delta 必须是 **ephemeral 事件(不持久化、不进 ring buffer)**,且要给持久化的 `message.updated` 做 250ms 合并。
- **工具策略在两处重复且不一致**:`toRuntimeToolCall`(`runtime_tool_calls.go:178`)与 `applyConversationToolPolicy`(`runtime_output.go:477`),两处的 `runtimeToolDisplayKind` / `runtimeConversationToolKind` 对 MCP、todo 等分类不一致。
- **Sequence 不稳定**:`buildConversationItems` 全排序后 `Sequence=index+1`,中途插入 item 会移动后续所有 sequence → `snapshot+events` 回放不变量目前不成立,必须先修。
- **契约缺口**:`/v1/sessions/{id}/output` 与 `/output/events` 不在 `runtimeapi.Endpoints` 中。
- **antd X 2.7 已带**:`thought-chain`(status/blink/collapsible)、`bubble`(可关闭其内置 typing)、`code-highlighter`(基于 react-syntax-highlighter/Prism,已在 node_modules)、`sender`。无需新依赖。
- **乐观消息对账目前靠 content 字符串相等**(`outputSelectors.ts:125,157`);`RuntimeMessage` 已有 `ClientRequestID`,但 `RuntimeConversationItem` 没有,需要透出。

## 设计核心:两阶段 turn

```
┌ 用户消息气泡
├ ExplorationTrace(可折叠;有最终答复后默认折叠)
│   header: 状态动词 + "已读取 12 个文件 · 运行 2 条命令 · 1 个子任务" + 耗时
│   body:   thinking → 工具行/分组 → 权限 → hook → 子agent → 通知
└ 最终答复(document 风格 markdown,流式)
```

### 后端新增每 turn 一个 `exploration_summary` item

- ID 稳定为 `exploration-<turnID>`,原地更新。
- 结构:

```go
type RuntimeExplorationSummary struct {
    Status        string                    // exploring | done | failed | interrupted
    ToolCounts    []RuntimeExplorationCount // 按 display kind 分类计数(含 failed 数)
    ToolTotal     int
    FailedCount   int
    SubagentCount int
    ThinkingMS    int64
    ElapsedMS     int64
}
type RuntimeExplorationCount struct {
    Kind   string  // file_read / file_search / shell / file_edit / file_write / agent_task / generic
    Count  int
    Failed int
}
```

- 挂在 `RuntimeConversationItem.Exploration` 可选字段。
- 时态动词("正在读取…" / "已读取 12 个文件")由前端从 `Status`+counts 渲染。
- 前端计数**只增不减**(ratchet),live 提示 700ms 最短显示。

### 前端简化

- 删除 `buildTurnBlocks` 的二次推断;turn 分块变成按 `turnId` 的**纯视觉分组**;阶段判断只用 `item.phase` + `exploration_summary`。
- 删除前端的 kind/quiet/defaultExpanded 重推导,只消费 runtime 提供的 `display` 字段。

## PR 序列

### PR 1 — 后端:统一工具策略 + 放宽分组 + exploration summary

#### 新文件 `internal/runtime/runtime_tool_policy.go`

从 `runtime_tool_calls.go` 和 `runtime_output.go` 合并迁移:

- `runtimeToolKind(name, source, capabilityID, hints)` 单一 kind 推导:
  1. **内置工具静态注册表**(与 `capabilityIDForToolName` 复用同一命名):bash/job_output/job_kill→shell、view/read→file_read、write→file_write、edit/multiedit/apply_patch→file_edit、glob/grep/ls→file_search、todos→todo、agent/task_create/agentic_fetch→agent_task。
  2. **按 source**:mcp/plugin/custom → generic(除非 metadata 覆盖)。
  3. **字符串匹配只作兜底**。
  - 删除 `runtimeConversationToolKind`,合并 `runtimeToolDisplayKind`。
- `applyRuntimeToolPolicy(call, ctx)` 纯函数,消灭两处重复:
  - `toRuntimeToolCall` 用空 ctx 调用;
  - `applyConversationToolPolicy` 从投影上下文构建 ctx 再调用。
  - quiet/groupable/defaultExpanded/groupKey/status 归一化只在这里。

#### 分组策略

- `Groupable = 成功终态 && kind != agent_task`(shell/edit/write 现在可分组;todo 不进 timeline 分组,由 TodoTaskBar 承载)。
- `Quiet` 保持 `completed && kind∈{file_read,file_search}`,**只控制"单行轻量展示"**,不再控制可分组性。
- **failed/denied/cancelled/interrupted 永不分组**,独立 item + `DefaultExpanded=true`(加测试锁定)。
- `GroupKey` 去掉 messageID;`appendConversationToolItems` 改为 **turn 内相邻 run 累积**,遇到不可分组调用/可见 intermediate assistant 文本/权限请求/agent_task 才断组。**允许混合 kind 分组**(cc-haha / DeepSeek 都这样做)。
- `tool_group` 增加 `Display.Counts`(分类计数)与多类目组合标题("已读取 12 个文件 · 运行 2 条命令");组内任一成员 running/failed 时组 `DefaultExpanded=true`。

#### exploration_summary

- `buildConversationItems` 末尾为每个有探索项的 turn 产出该 item,rank ~500(user_message 之后、thinking 之前)。
- Status: `exploring` while `!isRuntimeTurnTerminal`,else `done | failed | interrupted`。
- `ThinkingMS` 从 thinking item 时间戳;`ElapsedMS` 从 turn started → finished/now。
- `itemsForRuntimeEvent` 增加规则:任何带 `ToolCallID`/message/turn 的事件也匹配该 turn 的 `exploration_summary`,使计数可流式更新。

#### Sequence 稳定化(PR 2 回放不变量的前提)

- `Sequence = turnStartMs*1_000_000 + rank*100 + intraRankCounter`,item 一旦生成 sequence 不再变;快照与增量事件排序一致。

#### 测试(`runtime_output_test.go` + 新 `runtime_tool_policy_test.go`)

- 两调用点 kind 一致性
- 非零 exit shell = failed 且独立
- 跨 assistant 消息相邻分组
- intermediate 文本断组
- 失败工具独立 + 展开
- 分组计数
- exploration_summary 生命周期(exploring→done、计数、子任务数)
- 插入 item 时 sequence 稳定

**此 PR 可独立发布**:旧 UI 对未知 kind 返回 null,更丰富的 tool_group 走现有渲染路径,不影响功能。

### PR 2 — 后端:流式 surface + 推送通道 + 契约

#### Ephemeral delta 事件

`internal/runtimeapi/contract.go` 新增:
```go
EventOutputTextDelta = "output.text.delta"  // ephemeral: never persisted, never counted in cursors
```

`recordRuntimeEvent` 中对 assistant 消息 pubsub 维护 `streamCursors map[messageID]{lastTextLen, lastReasoningLen}`(message.completed / turn 结束时清除),计算后缀 delta 并发布:

```go
RuntimeOutputEvent{
  Kind: "output.text.delta",
  EntityID: messageID,
  Operation: "delta",
  TextDelta: &RuntimeOutputTextDelta{
    MessageID, TurnID,
    PartType: "text" | "reasoning",
    Delta: string,
    ContentLen: int,  // 应用后总长度(幂等键)
  },
}
```

**幂等/顺序规则写入契约**:delta 是单调前缀扩展,消费方 `ContentLen > knownLen` 才应用;任何全量 `message` / `item` payload 覆盖累积文本。

**回放不变量只对持久化事件定义**:`snapshot(c0) + persistedEvents(c0..cN) == snapshot(cN)`。deltas 是 advisory。

#### Per-session 订阅

新 `internal/runtime/runtime_output_stream.go`:

```go
func (r *runtimeService) SubscribeSessionOutputEvents(
    ctx context.Context, sessionID string, after string,
) (<-chan RuntimeOutputEvent, func())
```

实现要点:
- deltas 即刻转发(同 message 合并);
- 持久化事件置 dirty 标志,50ms debounce 后内部调用现有 `SessionOutputEvents(sessionID, lastCursor)` 转发 typed events(**复用投影 diff**,不再造第二套增量引擎;仅活跃 session,代价远低于现在 1.2s 全量 hydrateWorkbench);
- 有界 channel(256),溢出丢弃并发一条 `snapshot_required`。

#### 通道

- **Wails**:`desktop/runtime_bridge.go` 新增 `StartSessionOutputStream(ctx, req) (resp, error)` + `StopSessionOutputStream`,克隆终端流模式(521-642 行),batcher **50ms**(vs 终端 8ms)并合并同 message 连续 delta,`app.Event.Emit("agent-builder:output-stream", batch)`;新 session 开流自动停旧流。
- **HTTP dev**:`GET /v1/sessions/{id}/output/stream`(SSE),handler 仿 `handleEvents`。
- **轮询兜底**:`SessionOutputEvents` 不变,由 PR 3 真正用起来。

#### 加固(同 PR)

持久化的 `message.updated` 事件在 `recordRuntimeEvent` 做 **250ms/条 合并**(`message.created` / `message.completed` 始终持久化),消除每 token 的 SQLite event 写入与 ring buffer 溢出。

#### 契约同步

- `runtimeapi.Endpoints` 补 `/output`、`/output/events`、`/output/stream`;
- `EventTypes` 加 `EventOutputTextDelta` 并加 `EphemeralEventTypes` 集合;
- `contract_test.go` 断言 ephemeral 类型不落库(unit test on `appendRuntimeEventLocked` guard)。

#### 测试

- **回放不变量**:脚本化 turn 驱动 `recordRuntimeEvent`,逐 item 断言 `snapshot(c0) + events == snapshot(cN)`(依赖 PR 1 的稳定 sequence)。
- delta 单调性、乱序拒绝、`ContentLen` 幂等键。
- 溢出 → `snapshot_required`。
- `message.updated` 250ms 合并测试。
- `desktop/runtime_bridge_test.go` Start/Stop(扩展 `recordingRuntimeService` 加新方法),batch emit,event name。
- `runtime_http_test.go` SSE 握手 + 首批。

### PR 3 — 前端数据层:消费流

#### 新 `client/src/runtime/outputStream.ts`

```ts
export function subscribeSessionOutput(opts: {
  bridge: RuntimeBridgeModule;
  sessionId: string;
  after?: string;
  onBatch(events: RuntimeOutputEvent[]): void;
  onSnapshotRequired(): void;
}): () => void
```

通道选择:
1. **Wails**:`bridge.StartSessionOutputStream` + `wailsRuntime.Events.On('agent-builder:output-stream')`
2. **SSE**:`EventSource` on loopback URL
3. **Polling fallback**:`bridge.SessionOutputEvents(sessionId, cursor)` 1.2s

通道与 React state 间加 **50ms 合并缓冲**(同 message delta 合并,每次 flush 一次 setState)。

#### Reducer/类型

`outputTypes.ts` / `outputReducer.ts`:

- `RuntimeOutputEvent.textDelta?: { messageId; turnId; partType: 'text'|'reasoning'; delta; contentLen }`
- `OutputStore.streamingByMessageId: Record<string, { text; thinking; textLen; thinkingLen }>`
- `applyOutputEvent` 处理 `operation: 'delta'`:`contentLen > known` 才应用;收到该 message 全量 payload 或状态离开 streaming 时清除该 entry。
- 后端在 `user_message` item 上带出 `clientRequestId`(来自 `msg.ClientRequestID`),替换 `outputSelectors.ts:125,157` 的 content 相等对账。
- `exploration_summary` + `Display.counts` 类型,selector 映射为 `ExplorationSummaryViewModel`。

#### Selector overlay

`selectRuntimeConversationTimeline` 把 `streamingByMessageId` 叠加到匹配 `messageId` 的 `assistant_message`/`assistant_thinking` item 上,标记 `item.streaming = true`。

#### 接线

`wailsWorkbenchAdapter.ts` + `WorkbenchShell.tsx`:
- adapter 新增 `subscribeSessionOutput(sessionId, onStoreChange)`;
- shell 用流批次更新 `outputStore` 并重算 timeline,**不再走 `hydrateWorkbench`**;
- `hydrateWorkbench` 保留用于:session 切换、`snapshot_required`、非对话状态(hooks/tasks/todos/callchain/prompt-assemblies);
- busy 循环全量刷新降到 3s;
- `runtimeEventRefresh` 在流可用时不再把 `message.*` / `tool.call.*` 当刷新触发(status/usage 保留)。

#### 删除死代码(plan-11 PR6 范围)

- `mapActivityTimeline` / `mergeActivityTimeline` / `compareActivityTimelineItems` 等(adapter ~2515 行起)
- selector 无 version 的 fallback 分支
- `staticWorkbenchAdapter.tsx` fixtures 改为 `version: 1` + `items`

#### 注意

PR 1 改了 sequence 语义后,`applyOutputEvent` 从 `sequence/100` 推 cursor 的逻辑要改为使用 events 响应的 `cursor` 字段。

#### 测试

- 扩展 `client/scripts/conversation-output-store-smoke.mjs`:delta 幂等、乱序拒绝、完成时清除 overlay、clientRequestId 对账
- 新 `client/scripts/conversation-streaming-smoke.mjs`:脚本化事件序列(created → deltas → tool started → group update → final),断言中间态 timeline
- `npm run build`

### PR 4 — 前端渲染重写(antd 为基础)

#### Timeline.tsx → 薄分发

删除 `buildTurnBlocks` 的状态合并、`isFinalAssistantMessage`、`compactProcessItems`、`groupAdjacentToolCalls`、`mergeTurnStatus`。

新分组:按 `turnId` 分块;`user_message` → 头部;`phase === 'final'` → 尾部;`exploration_summary` → trace 头部数据;其余按序进 trace。

拆文件:
- `features/timeline/TurnBlock.tsx`
- `features/timeline/ExplorationTrace.tsx`
- 行组件保留

#### ExplorationTrace.tsx

- antd `Collapse`(ghost / small),label 从 `ExplorationSummaryViewModel` 渲染:
  - 时态动词(`useMinDisplay` 700ms)
  - ratchet 计数(`useRatchet`)
  - 子任务数
  - 耗时
- `defaultActiveKey`:`status === 'exploring' || failedCount > 0` 时展开;转 `done` 自动折叠(**受控 activeKey + 用户手动覆盖 latch**)。
- **Body spike @ant-design/x `ThoughtChain`**:子项映射 `ThoughtChainItemType`(pending / success / error / abort,running 时 `blink`,body `collapsible`)。
- **验收**:与现有 CSS Modules / theme token 兼容、无固定高度冲突;不合适则退回 CSS module 纵向 rail 包住现有行组件(**行组件保持容器无关**,方便替换)。

#### 工具行(ToolCallCard.tsx 改造)

- **quiet 完成的单工具** → 一行轻量 row(kind 图标 + runtime `display.title` + `primaryTarget` code span + 耗时),**无 Collapse chrome**。
- **非 quiet / 可展开** → Collapse ghost small,body 按 kind 分发到新 `features/tools/bodies/`:
  - `ShellBody`(命令行 + 终端风格 stdout/stderr 摘录 + exit code Tag)
  - `EditBody`(路径 + diffSummary/diffCount + diff refs)
  - `SearchBody`(pattern + 匹配列表)
  - `ReadBody`
  - `GenericBody`(输入/输出摘录)
  - 字段都已在 `RuntimeToolCallDisplay` 上,纯展示工作。
- **tool_group** → runtime 标题/计数做 header,子项为单行 row 可各自展开;组 `DefaultExpanded` 遵循 runtime。
- **错误**:failed/denied/interrupted 用 `colorError` token,必显 `failureReason`/stderr,永不默认折叠。
- **删除前端 `toolKind()` / `shouldOpenByDefault` 重推导**,只消费 `display.kind` / `defaultExpanded`。

#### 流式文本

- `TimelineMessage` 对 streaming item 渲染 `MarkdownMessage` + CSS 光标;
- 内容先过 `completePartialMarkdown()`(闭合未配对 ``` fence)再 `react-markdown`;
- 50ms flush 提供真实打字机(**不叠加 Bubble 的合成 typing 动画**——设 `typing={false}`,内容驱动);
- `ThinkingItem` 同 overlay 显示实时思考。

#### Markdown 升级(MarkdownMessage.tsx)

- 带语言的 fence → antd X `CodeHighlighter`(`prismLightMode`,头部语言 + 复制);
- 内联 code 不变;
- 最终答复 document 风格:`Bubble variant="borderless"`(已是)+ CSS module 类 widening max-width when 含块级内容(table/pre)。

#### 保持不动

- `TodoTaskBar`、`PermissionGate` dock、`PermissionTraceRow`(只做视觉 token 对齐)

### PR 5 — 打磨与收敛

- **Composer 样式**(features/composer/Composer.tsx + module.css):Sender 容器 token(边框/圆角/焦点环/阴影)、busy/streaming 状态的停止按钮突出、与 permission dock 视觉连续。**无行为变更**。
- **删除残余 SessionActivity 派生对话路径**(diagnostics-only 用途加注释保留)。
- **删除被流替代的 `runtimeEventRefresh` 条目**、legacy `assistantSteps` selector helpers(`pushAssistantStepItems` / `pushToolItems` / `completedToolGroupTitle`)。
- **可选**:`RuntimeOutputSnapshot` legacy 数组放到 `Detail: true` 请求参数后面(先测量 payload 大小再决定)。

## 关键文件

### 后端

- `internal/runtime/runtime_output.go`(投影主体,~1420 行)
- `internal/runtime/runtime_tool_calls.go`(display 派生,迁往新 runtime_tool_policy.go)
- `internal/runtime/runtime_events.go`(recordRuntimeEvent,delta tap 点 + message.updated 合并)
- 新:`internal/runtime/runtime_tool_policy.go`、`internal/runtime/runtime_output_stream.go`
- `internal/runtimeapi/contract.go`、`internal/runtime/runtime_http.go`、`desktop/runtime_bridge.go`

### 前端

- `client/src/runtime/outputTypes.ts` / `outputReducer.ts` / `outputSelectors.ts` / `wailsWorkbenchAdapter.ts`
- 新:`client/src/runtime/outputStream.ts`
- `client/src/features/timeline/Timeline.tsx`(拆出 TurnBlock.tsx、ExplorationTrace.tsx)
- `client/src/features/tools/ToolCallCard.tsx`(+ 新 bodies/ 目录)
- `client/src/features/markdown/MarkdownMessage.tsx`、`client/src/features/composer/Composer.tsx`

## 验证

每 PR:
```
go test ./internal/runtime ./internal/runtimeapi ./desktop
cd client && npm run build
node client/scripts/conversation-output-store-smoke.mjs
node client/scripts/conversation-streaming-smoke.mjs   # PR 3 起
```

合并前全量:
```
go test ./...
```

### 端到端手工场景(桌面 + Vite dev 两个通道都验)

1. **简单问答**:文本流式打字机,无探索 trace。
2. **长多工具任务**:探索 trace 实时更新计数("正在读取…")→ 最终答复出现后自动折叠为"已读取 N 个文件 · 运行 M 条命令 · 耗时 Xs",可展开看全过程。
3. **中途 shell 失败**:失败工具独立展开显示 stderr,trace 因 `failedCount > 0` 保持展开。
4. **权限审批**:流式过程中 permission dock 出现,timeline 有 trace row,决策后继续。
5. **子 agent 任务**:trace 内 agent_task 行可打开详情。
6. **长 turn(>200 事件)**:溢出触发 `snapshot_required` 后 UI 无缝重挂快照。
7. **session 切换/重启**:旧流停止、新流启动,历史 turn 全部默认折叠。

## 风险

- **活跃 session 每 50ms 一次投影重建**:有 debounce 且单 session,严格优于现在 1.2s 全量 hydrateWorkbench;profiling 不过关时 broadcaster 接口已为真增量投影器预留插槽。
- **ThoughtChain 样式适配**是 PR 4 内的受控 spike,行组件容器无关,可退回。
- **与进行中的存储重构的关系**:本方案只动投影输出侧与前端,projection 是纯函数,存储落地后输入重接一次即可;建议避免同时改 `runtime_output.go` 的输入 hydrate 部分。

## 与 plan-11 的关系

plan-11 定义了完整的 runtime-owned conversation projection 契约,已实现 PR1-3(投影骨架 + 工具策略 + 前端切主路径)与部分 PR6(标注 diagnostics-only)。**本方案是 plan-11 的 PR4/PR5 的具体化 + 补足流式与两阶段 UX**:
- plan-11 PR4(Timeline / ToolCallCard 重写为 display-only)= 本文 PR 4
- plan-11 PR5(Hook/AgentTask/Todo/Context 接入 projection)在本文中通过 exploration_summary 与 tool_group 的分类计数**更进一步**
- plan-11 PR6(删除旧 activity rendering)= 本文 PR 3 与 PR 5 分两步完成
- **新增**:两阶段 turn UX、流式 delta 通道、message.updated 合并、sequence 稳定化
