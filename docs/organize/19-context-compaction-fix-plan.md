# 上下文压缩重构 修复轮实施方案(fix round)

针对 18 号方案首轮实施(commits context-1..5)的 review 结论,修复全部 P0/P1 问题并补齐未实现项。Review 证据见两份审查报告结论,汇总于本文各 WP 的"问题清单"。

执行协议:六个工作包(WP1~WP6)按三个波次执行,**波次内文件所有权互斥**;每个 WP 由指定模型的子代理完成,**不 commit**,由主控(Fable)逐波验证后统一提交。

| 波次 | WP | 内容 | 模型 | 文件所有权 |
|---|---|---|---|---|
| 1 | WP1 | 正确性核心:切分/投影/计量/并发 | Fable(最高) | runtime_prompt_assembly.go、runtime_context_usage.go、runtime_memory.go、agent.go(usage 写入)、message 模型、contextmgr 小改 |
| 2 | WP2 | 模型摘要 summarizer + 重注入 + hooks | Opus | 新 agent summarizer 文件、coordinator、runManualFullCompact 执行段、filetracker |
| 2 | WP3 | contextGovernance 配置/API/设置 UI | Sonnet | internal/config、新 governance 文件、SettingsPanel、adapter 设置段 |
| 2 | WP5 | 压缩 item 载荷 + 前端展示补全 + 事件时机 | Sonnet | runtime_output.go、runtime_output_stream.go、CompactDivider、ContextUsageIndicator、outputTypes |
| 3 | WP4 | reactive 413 重试 + 熔断器 | Opus | agent.go(错误路径)、coordinator、runtime 熔断状态 |
| 3 | WP6 | 死代码删除 + diagnostics 对齐 + 遗留测试修复 | Sonnet | runtime_compact*、recovery、ContextDiagnosticsPanel、fsext/skills 测试 |
| 4 | — | 全量验证 + code review + 提交 | Fable(主控) | — |

通用约束(所有 WP):遵守 18 号方案 D1~D6;契约五端同步;不留兼容;DB 改动走 migration + schema.sql + sqlc;UI 用 antd + token + CSS Modules;**不执行 git commit**;完成后输出改动文件清单与测试结果。

---

## WP1 正确性核心(Fable)

### 问题清单
- B1 压缩切分盲切尾部 6 条,无 tool_use/tool_result 配对校验(runtime_prompt_assembly.go:557-560)
- B2 自动压缩只对当前 step 生效,同 turn 后续 step 回退全量历史
- B3 project memory system 消息被压缩切掉或错位到摘要后
- 秒/毫秒时间戳混淆:boundary 毫秒 vs messages 秒,同秒消息永远判为 boundary 之前(runtime_context_usage.go:229-241)
- 估算 usage 冒充真实锚点 + 字节/rune 估算单位不一致(agent.go:501-508、usage_fallback.go:182-187)
- B5 ManualCompact 无并发防护;摘要/session/boundary 三次独立写无事务
- B6 二次压缩 preTokens/messageRefs 未排除上一 boundary 之前
- B7 固定开销超阈值时每 step 反复压缩无护栏
- B9 auto 触发 boundaryID 仍含 "manual"
- localEstimate 双重计数(ToolOutputs/InputBudget 与消息重复,runtime_context_usage.go:98)
- B4 启发式摘要 "Files" 节收集的是工具名而非文件路径
- Breakdown 缺 system_prompt 分类
- currentRuntimeModelLimits 只查全局 provider(runtime_budget.go:99-124)、applyModelConfig 未传 Catalog(runtime_model_config.go:331-334)
- "仅覆盖最大输出"把解析窗口锁死成 user_override(runtime_provider_settings.go:605-609 + adapter/SettingsPanel 回写)

### 修复设计
1. **边界锚定投影(修 B1/B2/B3)**:full compact 完成时在 runtime 内存登记 per-(session,turn) 状态 `{summaryFantasyMessage, tailStartIndex}`(tailStartIndex=快照长度−配对安全 tail 长度)。`buildModelInputProjection` **每个 step** 检查该状态:存在则投影 = `summaryMsg + snapshot.Messages[tailStartIndex:]`(fantasy 消息在 turn 内只追加,索引稳定;长度异常则跳过并告警)。turn 结束清除状态;下一 turn 由 getSessionMessages 的 SummaryMessageID 切片接管。project memory 注入移到投影**之后**,以 system 消息前置在最终序列头部,且永不参与折叠统计。
2. **配对安全 tail 选择**:从尾部取 ≥6 条;若起点消息含 tool_result 而对应 tool_use 不在保留区,起点向前扩到包含该 assistant 消息;剔除末尾悬空 tool_use(assistant 带 call 无后续 result);system/synthetic 消息不作为切点。独立纯函数 + 表驱动单测(孤儿 result、跨消息配对、连续工具轮、全折叠退化)。
3. **ID 序位替代时间戳**:computeContextUsage 弃用时间戳比较;加载有序消息列表,以最新 completed full boundary 的 SummaryMessageID 定位索引,post-boundary=该索引起的切片;锚点=切片内最后一条带**真实** usage 的 assistant;找不到 summary ID(旧会话)则全列表。删除秒/毫秒归一化代码。
4. **真实 usage 门槛**:agent.go OnStepFinish 仅当 usage 非估算(fallbackStepUsage 未介入)才写 message.Usage;估算路径 message.Usage 留空(锚点自然跳过,Estimated 语义恢复真实)。usage_fallback 的估算统一改为 rune 口径 `(RuneCount+3)/4`。
5. **事务与并发**:摘要消息 + boundary completed 在单个 SQLite 事务;session.SummaryMessageID 更新紧随其后(失败则回滚事务并 fail boundary)。ManualCompact 入口检查会话 busy(runtime 已有 turn 活跃状态),busy 时返回明确错误「会话正在运行,请等待本轮完成后再压缩」。auto 路径在 PrepareStep 内天然串行。
6. **统计与护栏**:preTokens/messageRefs 只统计"上一 boundary 摘要索引之后、本次 tailStart 之前"的区间;boundaryID 改 `ctxbound_full_<trigger>_...`;每 turn 至多一次 auto compact(内存 flag),压缩后 used 仍 ≥ autoCompactAt 时记 `auto_compact_ineffective` warning 并跳过本 turn 后续尝试。
7. **计量修正**:localEstimate = Total − Messages − ToolOutputs − InputBudget + postBoundaryEstimate(消灭双重计数);Breakdown 增加 system_prompt 类(取最新 prompt assembly System.TokenEstimate),memory 并入 project memory 估算;messages 类 = used − 各已知类,clamp ≥0。
8. **模型限额修正**:currentRuntimeModelLimits 改按"当前会话实际选中模型"解析(接 selected_models 的 scope 链),查不到再退全局 provider;applyModelConfig 传入 catwalk Catalog 与 metadata cache。
9. **覆盖语义修正**:models_json 存储用户**显式输入值**,不回填解析值;source=user_override 仅当对应字段用户真的填过;enrich 返回 resolved 值放独立字段(ResolvedContextWindow/ResolvedMaxOutputTokens),adapter/SettingsPanel 同步改字段映射,保存时只写用户显式值。
10. B4:启发式摘要 Files 节改从 read_files 表取最近文件路径(≤10 条)。

### 验收
新增/修订单测:配对选择表驱动、多 step 投影持久(模拟 3 step,断言 step2/3 仍是压缩后序列)、同秒边界锚点(ID 序位)、估算 usage 不作锚点、preTokens 区间、每 turn 一次 auto、busy 拒绝 manual、覆盖语义(只填 maxOutput 不产生窗口 override)。`go test ./internal/...` 全绿(fsext/skills 两个既有失败除外)。

---

## WP2 模型摘要与压缩重建补全(Opus)

### 问题清单
- 模型 summarizer 全套缺失(现全走本地启发式)
- 重注入缺失(read_files/todos/skills/运行中任务)
- PreCompact/PostCompact hooks 未触发
- filetracker 压缩后未置 stale
- EventContextReinjected 孤儿事件(本 WP 让它有真实 emitter)

### 设计
1. 新 `internal/agent/compact_summarizer.go`:`GenerateCompactSummary(ctx, req{Messages, Instructions, UseSmallModel}) (string, error)`。fantasy agent 无工具、thinking off、maxOutput=min(20000,模型上限)、60s 超时;prompt = NO-TOOLS 前导 + `<analysis>` 草稿指令 + 9 节结构(全文按 16 号文档 §3.4,重点 All User Messages、Next Step verbatim)+ 可选 instructions;发送前图片/文档替换占位;输出剥离 `<analysis>`;摘要请求自身超长(contextmgr.IsContextLengthError)→按 assistant 起始的 API round 分组从最旧丢,重试 ≤3。Coordinator 暴露该能力,runtime 经与 modelInputBuilder 同款注册模式接线(自查现有 schedulerRecorder 注入路径选最干净方案)。
2. runManualFullCompact:优先模型摘要(WP3 的 summaryModel 设置,默认 session 模型;WP3 未合入前用默认),失败/超时回退启发式并在 boundary 记 `summary_mode=heuristic_fallback`;摘要期间每 30s 发 ephemeral 进度事件 `compact.progress`(注册进 EphemeralEventTypes,不落库)。
3. 重注入:摘要消息追加 parts——read_files 最近 ≤5 个(单 ≤5k tok/总 50k,磁盘重读,hash 变化标 stale,排除 CLAUDE.md/plan)、未完成 todos、已加载 skills 头部(单 5k/总 25k)、运行中 agent task 一览;boundary.ReinjectedRefs 记录,发 EventContextReinjected。
4. hooks:压缩前跑 PreCompact(stdout JSON 的 additionalInstructions 并入摘要指令),完成后跑 PostCompact;走 internal/hooks 现有 runner(参考工具 hook 触发点)。
5. 压缩成功后 filetracker 该会话已读状态置 stale。

### 验收
summarizer 单测(mock provider:正常/超时回退/PTL 丢轮/analysis 剥离);重注入预算单测;hooks 触发单测(fake hook 脚本);heuristic 回退端到端。

---

## WP3 contextGovernance 配置与设置 UI(Sonnet)

### 问题清单
- 配置存储/API/UI 全缺,自动压缩无法关闭、无阈值覆盖
- staticWorkbenchAdapter 假 governance 数据仍被渲染(staticWorkbenchAdapter.tsx:52-60、SettingsPanel.tsx:1769-1773)
- 警告条不按 autoCompactEnabled 分支文案

### 设计
1. config(internal/config)增加 `ContextGovernance{AutoCompactEnabled *bool, AutoCompactPercent *float64, MicrocompactEnabled *bool, MicrocompactKeepRecent int, SummaryModel string("session"|"small"), ProviderOverrides map[providerID]{AutoCompactPercent *float64, Models map[modelID]{AutoCompactPercent *float64}}}`,global JSON 持久化。
2. runtime 访问器 `contextGovernanceFor(provider, model)` 带默认值(enabled=true、pct=nil、micro=true、keep=5、summary=session);buildModelInputProjection 的 auto 检查、microcompact 配置、contextThresholds 的 percent 覆盖(autoCompactAt=min(推导, window×pct))全部改走它;`RuntimeContextUsage` 增加 `AutoCompactEnabled bool` 字段。
3. API:GET/PUT `/v1/settings/context-governance` + bridge + Endpoints + adapter + 测试(五端)。
4. 设置 UI:SettingsPanel「上下文」分区(真数据):开关+当前模型触发点说明;高级折叠:百分比覆盖(空=自动,0.05~0.95 校验)、microcompact 开关/保留条数、摘要模型;当前模型 profile 只读卡片(窗口/触发点/警告点/来源)。删除假 ContextGovernanceSettingsViewModel 数据与旧渲染路径。
5. 警告条:autoCompactEnabled=true → 灰字「距自动压缩还剩 x%」;false → warning/error 色「上下文即将用尽(剩 x%),请使用 /compact 手动压缩」。

### 验收
设置读写端到端(保存后 GET 一致、runtime 判定生效);关自动压缩后超阈值不压缩且警告条变色;percent=0.05 触发点变化断言;前端 build。

---

## WP5 压缩 item 载荷与前端展示补全(Sonnet)

### 问题清单
- B10 摘要全文前端不可达(item 只有 ref 串);Display 无 preTokens/postTokens/summarizedCount/summaryMessageID
- B8 自动压缩失败也产出红色 divider(方案要求 auto 静默)
- B11 Timeline 死代码(isContextGovernanceItem 的 compact 分支不可达)
- B12 /compact 无 turn 时未捕获异常(wailsWorkbenchAdapter.ts:1954、Workspace.tsx:453)
- compactCount refreshNonce 强制刷新未实现;Popover 无 top-5 合并;圆环中心硬编码 #fff(暗色主题白盘)
- context.usage.updated 缺"assistant step 完成"发布时机
- contract_test 无 started↔completed/failed 成对断言

### 设计
1. `RuntimeConversationItem` 增加 `Compact *RuntimeCompactInfo{Trigger, Status, PreTokens, PostTokens, SummarizedCount, SummaryMessageID, SummaryText(≤4000 rune 截断), Error}`;runtime_output.go 从 boundary+摘要消息填充(摘要消息虽被会话列表过滤,投影层可直接按 ID 读);**status=failed 且 trigger=auto 的 boundary 不产 item**(circuit_open 例外,WP4 后生效);清理 B11 死分支。契约五端同步。
2. CompactDivider:completed 展开 = meta 行(触发方式/前后 token/覆盖消息数)+ SummaryText 全文(限高 240px 滚动);failed(manual)红边+原因;compacting 不变。
3. B12:/compact 无活跃 turn 时 adapter 返回友好错误,Workspace 捕获后 antd message 提示「当前没有可压缩的对话轮次」。
4. 指示器:compactCount 变化触发立即强制拉取 SessionContextUsage(绕过 350ms 合并,丢弃 in-flight);Popover 分类 top-5 + 「其他」合并;圆环中心色改 `var(--ant-color-bg-container)` 或等效 token。
5. 事件时机:assistant 消息带真实 usage 完成时(runtime_events.go 的 message.completed 路径)debounce 500ms 发 context.usage.updated。
6. 测试:runtime_output_test 增加 compact item 载荷断言、auto-fail 无 item 断言;runtime 集成测试断言每个 compact.started 终有 completed|failed(成对);前端 smoke 扩展 divider 载荷。

---

## WP4 reactive 重试与熔断(Opus)

### 问题清单
- reactive 413 完全未接线(contextmgr.ReactiveCompact/IsContextLengthError 生产零调用)
- 熔断器缺失;compact.failed 的 attempt/will_retry/circuit_open 是硬编码假值(runtime_prompt_assembly.go:225-227)

### 设计
1. agent.go 请求错误路径(Stream 返回 err 处)识别 contextmgr.IsContextLengthError → 返回类型化 `ErrContextTooLong`;coordinator/agent 捕获后通过注册回调请求 runtime 执行 reactive compact(记 ReactiveAttempt:attempt1=强化投影(microcompact 全量+snip 中段),attempt2+=强制 full compact,trigger=reactive),随后重试该次调用;每 turn ≤3 次,超次 turn 失败并给出引导错误。
2. 熔断:runtime per-session 内存 `consecutiveCompactFailures`,full compact 失败 +1(auto/reactive),≥3 → 停止自动与 reactive 压缩、发 compact.failed{circuit_open:true} + diagnostics warning(此失败 item 允许产出,WP5 约定);成功或手动 compact 重置。事件字段 attempt/will_retry/circuit_open 填真实值。
3. 与 WP1 的"每 turn 一次 auto"标志共存:reactive 不受该标志限制(它是错误驱动),但受熔断限制。

### 验收
mock provider 返回 prompt-too-long:attempt1 投影强化重试成功 / attempt1 失败→attempt2 full compact→成功 / 3 次失败→turn 失败;熔断计数、重置、事件真值断言。

---

## WP6 清理与遗留(Sonnet)

### 问题清单
- runtime_compact_boundaries 表/store/DTO 未删;GET /v1/sessions|turns/{id}/compact 读空表;recovery/recordPromptAssembly 仍引用旧 store
- ContextDiagnosticsPanel 与指示器数字两套口径
- EventContextReinjected 孤儿断言(WP2 接线后复查);contract_test 补"无孤儿事件"检查
- 两个既有测试失败:internal/fsext TestListDirectory(Windows 路径尾随空格/根目录计数)、TestRuntimeSkillsFromConfigIncludesDesktopManagedPath(SkillFilePath 正反斜杠前缀不匹配)
- docs:18 号 §6 状态如实修正,19 号(本文)登记完成状态

### 设计
1. 删除 runtime_compact.go/runtime_compact_payload.go/runtime_compact_store.go/runtime_compact_test.go;migration drop runtime_compact_boundaries + schema.sql;`TurnCompactBoundaries/SessionCompactBoundaries` 改读 contextmgr store 映射(RuntimeContextBoundary 已有),HTTP 路由保留但换数据源;recovery(runtime_recovery.go:115-131)与 recordPromptAssembly:316-318 改读 contextmgr;runtime_lifecycle.go:331 移除旧 store 初始化。
2. ContextDiagnosticsPanel:头部大数字与百分比改消费 SessionContextUsage(与指示器同源),boundary/replacement/reactive 明细保留现有来源。
3. contract_test:遍历 EventTypes,断言仓内(internal/,排除 *_test.go)每个类型存在字符串引用(emitter 或映射),防孤儿;若 EventContextReinjected 仍无 emitter(WP2 意外未接)则报告而非静默。
4. 修两个既有失败:fsext(检查 ls 对根目录/尾随反斜杠的处理与断言口径)、skills 测试(SkillFilePath 与 root 前缀比较统一 filepath.ToSlash)。修复以最小改动为准,行为语义不变。
5. 文档:18 号 §6 表格改为真实状态(标注哪些由本修复轮完成);本文件底部登记各 WP 完成情况。

---

## 波次门禁(主控执行)

每波结束:`go test ./...`(仅允许 WP6 前的两个既有失败)+ `cd client && npm run build` + 两个 conversation smoke;波内代理只留工作区改动不提交,主控 review 后按波 commit(`context-fix-w1/w2/w3`);全部完成后主控跑 /code-review 级审查 + 手工链路清单(18 号 §7),修尾差,更新文档。

---

## 完成状态记录

以下由各 WP 代理在完工时登记,commit hash 由主控统一提交后回填;并行执行期间状态可能滞后于实际代码。

| WP | 状态 | 备注 |
|---|---|---|
| WP1 | 完成(由 WP1 代理登记,细节见其自身报告) | 边界锚定投影、配对安全 tail、ID 序位锚点、真实 usage 门槛、事务化 ManualCompact、preTokens 区间统计、模型限额修正、覆盖语义修正等在本波次落地。 |
| WP2 | 完成(由 WP2 代理登记,细节见其自身报告) | 模型 summarizer、重注入(read_files/todos/skills/运行中任务)、PreCompact/PostCompact hooks、filetracker stale 化;`EventContextReinjected` 在本波次获得首个真实 emitter(WP6 复查确认,见下)。 |
| WP3 | 完成(由 WP3 代理登记,细节见其自身报告) | `contextGovernance` 配置存储、`GET/PUT /v1/settings/context-governance`、设置 UI「上下文」分区、警告条按 autoCompactEnabled 分支文案。 |
| WP4 | 完成(主控终校 2026-07-04) | reactive 413 重试环(agent.go 包 Stream 的 for 循环:PTL 识别→删除空占位 assistant→ReactiveCompactor 回调→重启,≤3 次,超次返回引导错误);attempt1=microcompact 强化覆盖(KeepRecent=1、TokenDivisor=2,turn 级内存),attempt2+=full compact(trigger=reactive);session 级熔断器(连续 3 次失败开路,成功或手动 /compact 重置);compact.failed 的 attempt/will_retry/circuit_open 全部真实值;auto 分支熔断开路时跳过并发 circuit_open 事件。14 个新测试。 |
| 终审(w4) | 完成(主控 2026-07-04) | 跨 WP 缝合终审发现 3 高/2 中/1 低,已全部处置:H1 熔断开路的 auto 失败 boundary 以 `[circuit-open]` 标记并例外产出 divider(用户可见"自动压缩已停止",marker 对用户不可见,新增回归测试);H2 reactive attempt-1 的 microcompact 覆盖强制生效,不受 governance 关闭影响;H3 复核为**不成立**——事务内 `message.NewService(db.New(tx))` 是独立 broker 无订阅者,不存在"提交前幽灵持久化事件",摘要消息本就不发 message 事件(与其被会话列表过滤的语义一致);M1 auto 压缩改为 merge 而非覆盖 turn 状态,保留 reactive 覆盖字段;M2 `compact.progress` 在 output stream 增加直发分支(ephemeral 事件此前无任何前端可达路径);L1 消息删除的 pubsub 事件不再被误持久化为 message.created,改为仅唤醒 output stream 让前端丢弃幽灵条目。 |
| WP6 | 完成 | 详见本次报告:删除 `runtime_compact.go`/`runtime_compact_payload.go`/`runtime_compact_store.go`(功能迁移到 `runtime_compact_boundaries.go`,改读 contextmgr);`runtime_compact_test.go` 重写(保留仍覆盖真实行为的用例,删除仅测旧 store 的用例,recovery 用例改用 contextmgr 造数据);新增 drop-table migration + schema.sql 同步;`recovery`/`recordPromptAssembly`/`runtime_purge` 的旧 store 引用清理;`runtime_lifecycle.go`/`runtime_service.go`/`runtime_service_types.go` 移除旧 store 字段与初始化;`ContextDiagnosticsPanel` 头部数字改消费 `contextUsage`;`contract_test.go` 新增 `TestNoOrphanEventTypes`(AST 解析 contract.go 的事件常量 + 全仓源码扫描),移除 6 个确认孤儿的事件常量;修复 `internal/fsext` Windows 根路径分隔符 bug 与 skills 测试路径分隔符不一致问题。**偏离**:`RuntimeCompactBoundary`/`RuntimeReinjectedRef`/`RuntimeCompactToolCallRef` 三个 DTO 未删除,因为 `runtime_output.go`(WP5)与重注入路径(WP2)已将其复用为活跃类型,不再是死代码;仅其 SQL 存储层被删除。`go test ./...` 全绿,`cd client && npm run build` 通过。 |
