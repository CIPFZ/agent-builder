# 对话 UI 四个体验问题梳理与修复方案

本文针对 plan-12(两阶段 + 流式)落地后用户实测反馈的四个问题,给出根因定位(带代码证据)和修复方案。

四个问题:

1. 第一条消息发送后,流式过程中响应内容跳到用户消息上方,生成结束后又恢复正常。
2. 处理过程信息(探索 trace / 工具行)视觉未打磨。
3. 切换会话时先闪现上一个会话内容,再切到目标会话,不丝滑。
4. 发送后滚动条不自动跟随到底部;流式期间希望"钉在底部",但用户主动滚动时又不能被强制拉回。

四个问题不是孤立的:1 和 3 的根因都在 **outputStore 生命周期与刷新竞态**,4 是滚动跟随逻辑本身的设计缺陷,2 是 plan-12 PR4 的视觉收尾未完成。

---

## 问题 1:流式过程中 timeline 顺序错乱

### 现象还原

新对话(draft)发出第一条消息 → 流式响应期间,响应/工具内容出现在用户消息上方(或用户消息短暂消失)→ 生成结束(下一次全量快照落地)后顺序恢复。

### 根因(按确定性排序)

**根因 A(确定,主因):draft 发送时 outputStore 会话错位,流式事件被应用到一个"凭空新建的空 store"上。**

- `WorkbenchShell.tsx:504`:`sendPrompt` 里 `baseOutputStore = currentViewModel.outputStore ?? createOutputStore(activeSession?.id ?? '')`。
  - 从 draft 发送第一条消息时没有 active session:要么 `outputStore` 还是**上一个会话残留的 store**(`createSession`/`updateNewConversationDraft` 清了 `timeline` 但**没清 `outputStore`**),要么新建一个 `sessionId: ''` 的空 store。乐观用户消息被塞进了错误的 store。
- `WorkbenchShell.tsx:192-195`:流订阅的 `onEvents` 中,`current.outputStore.sessionId !== activeSessionID` 时直接 `createOutputStore(activeSessionID)` **新建空 store,只用增量事件拼状态**:
  - `optimisticByClientRequestId` 被丢弃 → 乐观用户消息消失;
  - `user_message` item 是否在事件流里取决于订阅起点(`after` cursor 传的是旧 store 的 cursor),经常拿不到 → store 里只有助手响应 item;
  - timeline 被 `selectConversationTimeline(空store+增量)` 整体替换 → 只剩响应内容/顺序错乱。
- 直到 800ms 后 busy 轮询(`WorkbenchShell.tsx:230-271`)做全量 `adapter.refresh` 拿到完整快照才恢复 → 与"生成结束后正常"完全吻合。已有会话内发消息时 `outputStore.sessionId` 匹配,所以问题集中出现在"对话的第一条消息"。

**根因 B(确定):乐观消息没有 sequence,排序时按 0 处理,永远排在整个 timeline 最顶端。**

- `outputSelectors.ts:71-90`:optimistic item 不带 `sequence`,`compareNumbers` 把 undefined 当 0;runtime item 的 sequence 是 `turnStartMs*1e6` 量级。在已有历史的会话里,新发的乐观消息会先排到**全会话最顶部**,等 runtime `user_message` item 到达对账替换后才跳回底部。

**根因 C(确定):全量刷新与流式增量互相覆盖,没有 epoch/新鲜度保护。**

- 三条路径都会无条件 `setViewModel(nextViewModel)` 整体替换 store:busy 轮询(`WorkbenchShell.tsx:245-260`)、runtime 事件触发的 `refreshFromRuntimeEvent`(`WorkbenchShell.tsx:108-132`)、`sendPrompt`/`selectSession` 的 resolve。
- refresh 是异步的:服务端快照在 T1 生成,T2 才落地;(T1,T2) 之间流式已应用的 item 被回滚,而流 cursor 已前移、这些事件不会重发 → 状态倒退,直到下一次全量刷新。流式期间每 3s 一次,视觉上就是"跳动/错乱→恢复"反复。

**根因 D(后端,加剧因素):流式中期 item 的 turnID 映射缺口导致 sequence 跨数量级跳变。**

- `runtime_output.go:519`:`buildConversationItems` 里 user/assistant item 的 `turnID` 只取 `messageTurnIDs[msg.ID]`,**没有** `runtimeOutputNearestTurnID` 兜底(assistant step 构建时有,line 191-193)。
- 映射不到时 `turnStart[""]=0` → sequence 从 `1.7e18` 量级掉到 `rank*100+intra` 量级,该 item 直接排到全 timeline 最前面;下一个 tick 映射补上后 sequence 又跳回大数 → 前端 merge(`{...old, ...new}`)后顺序翻转。
- turn 映射依赖 `turn.UserMessageID / LatestAssistantMessageID / LatestMessageID / tool call`(`runtime_output.go:1499-1529`),流式早期(assistant 消息刚创建、还没有 tool call、turn 字段未更新)存在真实缺口窗口。

**根因 E(后端,隐患):sequence 超出 JS 安全整数,JSON 传输有精度损失。**

- `sequence = turnStartMs*1_000_000 + rank*100 + intra ≈ 1.75e18`,远超 `Number.MAX_SAFE_INTEGER(9e15)`,float64 在该量级的步长是 256。同 rank 内 `intra`(<100)差值全部塌缩为相等,靠 `id.localeCompare` 决定顺序 → 同一批工具 item 的顺序可能与生成顺序不一致。

### 修复方案

前端(P0,一个 PR 内完成):

1. **store 生命周期收敛**:`createSession` / `updateNewConversationDraft` / `selectSession` 的乐观 viewModel 中显式清掉或替换 `outputStore`(`createOutputStore(目标sessionId或'')`),乐观 submit 永远进"当前目标会话"的 store。
2. **onEvents 不再用空 store 拼增量**:`sessionId` 不匹配时,保留 `optimisticByClientRequestId` 迁移到新 store,并立即触发一次 `requestFullRefresh()` 拿快照;快照落地前的增量照常应用(item 是全量 payload,merge 安全)。
3. **乐观消息排底部**:optimistic timeline item 赋 `sequence: Number.MAX_SAFE_INTEGER`(或 `max(现有 sequence)+1`),`user_message` 对账逻辑不变。
4. **刷新新鲜度保护**:
   - 所有异步 refresh 落地前校验 `sessionMutationSeqRef` + 目标 `activeSessionID` 是否仍一致(目前只有 selectSession 自己的 promise 有校验);
   - `hydrateOutputStore` 支持"不回退"合并:snapshot 的 cursor 落后于当前 store 的 `lastSequence` 时,以 merge 方式并入而不是整体替换(item 级 `updatedAt`/cursor 比较)。

后端(P1):

5. `buildConversationItems` 里 user/assistant message item 的 `turnID` 加 `runtimeOutputNearestTurnID` 兜底,保证流式中期不会出现 `turnStart=0` 的小 sequence。
6. sequence 基数改为**秒**:`turnStartSec*1e6 + rank*100 + intra ≈ 1.75e15 < 2^53`,JSON 无损;或 DTO 把 sequence 序列化成 string。前端 `applyOutputEvent` 的 cursor 推导已用事件响应 cursor,不受影响。
7. 为 6/5 补投影测试:同 turn 内 user < exploration < thinking < tool < final 的 sequence 全序,以及"turn 映射缺失 tick → 补全 tick"两次投影的 item sequence 单调不回跳。

### 验证

- `client/scripts/conversation-streaming-smoke.mjs` 增加场景:draft 首条消息(store sessionId 错位)→ 增量流入 → 断言 user_message 始终在响应 item 之前、乐观消息不置顶不丢失。
- Go 侧:`runtime_output_test.go` 增加流式中期(turn 字段未齐)投影顺序测试。

---

## 问题 3:切换会话先闪旧内容(和问题 1 同根)

### 根因

1. **无 epoch 保护的刷新竞态**(问题 1 根因 C 的另一表现):切换瞬间,之前已在途的 busy 轮询 / `refreshFromRuntimeEvent` 带着**旧会话的快照**晚于乐观清屏落地 → 旧内容闪回,随后 `selectSession` 的 promise 再切到新会话。
2. **hydrate 太重太慢**:`wailsWorkbenchAdapter.ts` 的 hydrate 是十几个顺序 `await`(hookExecutions → SessionOutput → narrowActivity → runProjection → agentTasks → todos → reactCallchain → promptAssemblies...),切换耗时几百 ms,放大了竞态窗口,也让 loading 期变长。
3. **无 per-session 缓存**:每次切换都从"空白 + 正在加载"开始,回切刚看过的会话也要全量等待。

### 修复方案

1. **epoch guard**(与问题 1 修复 4 同一实现):任何 `setViewModel(await ...)` 前校验发起时的 mutation seq 与 activeSessionID。
2. **切换路径瘦身**:`selectSession` 只等 `SessionOutput` 快照 + sessions 列表就渲染对话主体;hooks/runProjection/todos/callchain/promptAssemblies 等诊断类数据后台异步补齐(落地时同样过 epoch guard)。SessionOutput 之外的调用从"顺序 await"改成 `Promise.all` + 延迟批。
3. **per-session store 缓存**:shell 维护 `Map<sessionId, OutputStore>`(带容量上限,如 5 个):
   - 切走时把当前 store 存入缓存;
   - 切入时若有缓存,**立即渲染缓存内容**(不闪空白),同时后台拉最新快照原地更新;
   - 无缓存时渲染骨架屏(保持现有 "正在加载对话..." 但只占对话区,不整屏空白)。
4. 切换动效(可选):对话区 120ms 淡入,掩盖快照落地瞬间的重排。

---

## 问题 4:滚动跟随

### 根因

`Workspace.tsx` 现有逻辑的三个缺陷:

1. **跟随只由"最后一个 item 的 ID 变化"触发**(`Workspace.tsx:376-394`,依赖 `timelineLastID`/`conversationLastID`):流式文本增长、trace 内工具行增加、卡片展开都发生在**已有 item 内部**,最后 item ID 不变 → 内容长高但不滚动 → "跳在某处"。
2. **程序化滚动会自己解除钉底**(`Workspace.tsx:153-162`):`scrollTo({behavior:'smooth'})` 动画过程中持续触发 `onScroll` → `updateJumpToBottomVisibility` 在距底 >120px 时把 `scrollPinnedRef` 置 false → 钉底被自己的平滑动画杀掉;且 smooth 动画目标是发起时的 `scrollHeight`,流式期间高度持续增长,永远追不上底。
3. **无法区分用户滚动与程序滚动**:所以也做不到"用户上滚就让出控制、滚回底部就恢复跟随"。

### 修复方案(重写为标准 sticky-bottom 交互)

状态模型:`pinned: boolean`,只有**用户手势**能改变它。

1. **解除钉底**:仅在用户主动向上滚动时解除 —— 监听 `wheel`(deltaY < 0)、`touchmove`、滚动条拖拽(`pointerdown` 在滚动条区域)、键盘 `PageUp/Home/↑`;不要用 `onScroll` 距离推断。
2. **恢复钉底**:用户滚动至距底 ≤ 48px 时恢复(此判断只在上述用户手势的 scroll 中做);点击"跳到底部"按钮恢复。
3. **跟随执行**:用 `ResizeObserver` 观察 timeline 内容包裹层(覆盖流式文本、卡片展开、图片加载等一切高度变化),`pinned` 时每次尺寸变化 `container.scrollTop = container.scrollHeight`,用 **instant** 而非 smooth(流式高频增长下 smooth 动画互相打架,这正是现在"追不上底"的原因;偶发大跳可保留 smooth,持续流式用 instant)。
4. **程序滚动免疫**:程序化 scrollTo 前置 `programmaticScrollRef = true`,scroll 事件里检测到该标志直接跳过 pinned 判定,滚动停止(scrollend 或 100ms debounce)后复位。
5. **发送即钉底**:`onPromptSubmit` 时强制 `pinned = true` 并立即滚底 —— 用户发消息 = 明确表达"我要看最新内容"。
6. "跳到底部"按钮逻辑保留;顺带修复其 aria-label 乱码(`Workspace.tsx:617` 的 `璺冲埌搴曢儴` 是 GBK mojibake,应为"跳到底部")。

以上逻辑抽成 `useStickToBottom(containerRef)` hook,便于单测(纯逻辑部分:手势 → pinned 状态机)。

---

## 问题 2:过程信息视觉打磨(plan-12 PR4 收尾)

现状问题(`Timeline.tsx` / `ToolCallCard`):

- 三种容器混用:antd `Collapse`(trace、工具卡)、原生 `<details>`(`ToolRunSummary`、`AssistantProcessNote`、agent task summary)、裸 `div` 行(permission/hook/context),视觉不统一,原生 details 无主题 token。
- 每个工具仍偏"卡片",quiet 完成工具没有降为真正的单行轻量 row。
- trace 内缺少纵向 rail/连线,过程步骤之间没有视觉连续性。
- exploration header 已有分类计数,但没有 plan-12 说的 ratchet(计数只增不减)与 700ms 最短显示,流式时数字/动词闪跳。
- 失败态强调不足(仅 Tag),stderr/failureReason 不够醒目。
- 存在 mojibake 文案(Workspace 跳底按钮、若干历史文档同源问题),建议全局扫一遍 `[一-鿿]` 乱码模式。

方案(独立视觉 PR,不动数据层):

1. **统一行组件**:所有过程行(tool/permission/hook/context/agent_task)收敛到一个 `TraceRow` 基础组件(图标槽 + 标题 + meta + 可展开 body),展开一律用受控 Collapse ghost,删除原生 `<details>`。
2. **ExplorationTrace 视觉**:CSS module 纵向 rail(左侧 2px 线 + 节点圆点,running 节点用 pulse 动画),或按 plan-12 spike antd X `ThoughtChain`(pending/success/error 态映射现成);两者以行组件容器无关为前提,不合适可退。
3. **quiet 工具单行化**:quiet+completed 单工具渲染为无 chrome 的一行(kind 图标 + display.title + `primaryTarget` code span + 耗时),tool_group 用 runtime 标题/计数做 header,子项即单行 row。
4. **错误强调**:failed/denied/interrupted 行用 `colorErrorBg` 背景条 + 必显 failureReason/stderr 摘录,永不折叠(runtime 已给 defaultExpanded)。
5. **exploration header 微交互**:`useRatchet`(计数只增)+ `useMinDisplay(700ms)`(状态动词最短显示),转 done 自动折叠用受控 activeKey + 用户手动覆盖 latch(plan-12 原设计,未实现部分)。
6. **间距/字号 token 化**:trace 内行高、缩进、图标尺寸统一走 theme token,与最终答复 document 风格拉开层次(过程小一号、低对比,答复正常字号)。

---

## 实施顺序

| PR | 内容 | 解决 |
| --- | --- | --- |
| PR-A 状态层 | store 生命周期 + onEvents 空store修复 + 乐观 sequence + epoch guard + 后端 turnID 兜底 + sequence 秒基 | 问题 1、问题 3 的闪回 |
| PR-B 切换体验 | 切换路径瘦身 + per-session 缓存 + 骨架屏 | 问题 3 的丝滑度 |
| PR-C 滚动 | useStickToBottom 重写 | 问题 4 |
| PR-D 视觉 | TraceRow 统一 + rail + quiet 单行 + ratchet + mojibake 清理 | 问题 2 |

PR-A 是其余一切的地基(数据不稳,视觉打磨无意义);PR-B/C/D 互相独立可并行。

每 PR 验证:

```
go test ./internal/runtime ./internal/runtimeapi ./desktop
cd client && npm run build
node client/scripts/conversation-output-store-smoke.mjs
node client/scripts/conversation-streaming-smoke.mjs
```

手工场景:draft 首条消息流式全程顺序稳定;长任务中途上滚阅读不被拉回、滚回底部恢复跟随;快速连续切换 3 个会话无旧内容闪回;busy 期间切换会话不串台。
