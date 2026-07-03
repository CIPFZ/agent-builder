# 参考项目梳理:DeepSeek-GUI 的上下文管理 / 自动压缩 / token 用量展示

> 参考项目路径:`C:\Users\ytq\work\ai\DeepSeek-GUI`
> 梳理目的:为 agent-builder(Go + Wails3 + React)实现「自动上下文压缩、发送框 context 用量图标、压缩过程提示、按 provider/model 配置 context window 与阈值」提供可直接跳转的实现参考。

## 0. 项目简介

DeepSeek-GUI 是一个 **Electron + React + TypeScript** 的桌面 Agent 工作台("Kun 运行时的 Electron 前端"),架构分两层:

- **`kun/`**:独立的 TypeScript Agent 运行时(agent loop、工具执行、会话存储),以本地 HTTP/SSE 服务方式运行(`kun/src/server/`),由 Electron 主进程拉起子进程(`src/main/kun-process.ts`)。**上下文压缩、token 估算、模型 context profile 全部在这一层**,与 agent-builder 的 Go 后端(`internal/runtime`)角色对应。
- **`src/`**:Electron 主进程(`src/main`)+ React 渲染进程(`src/renderer`)。前端通过 `window.dsGui.runtimeRequest` 转发 HTTP 请求、通过 SSE 事件驱动 Zustand chat-store,与 agent-builder 的 Wails bridge + React 前端角色对应。

与本任务相关的核心模块全部集中在 `kun/src/loop/`(压缩)与 `src/renderer/src/`(展示)。

---

## 1. 模型 context window 的来源

**结论:硬编码内置表 + 本地 config.json 的 `models.profiles` 覆盖合并;没有从 API/模型列表自动获取。**

### 1.1 内置硬编码表

`kun/src/loop/model-context-profile.ts:70-90`:

```ts
export const DEFAULT_CONTEXT_THRESHOLDS: ModelContextThresholds = {
  softThreshold: 16_000,   // 未知模型的兜底软阈值
  hardThreshold: 24_000    // 未知模型的兜底硬阈值
}

const DEEPSEEK_V4_CONTEXT_WINDOW_TOKENS = 1_000_000
const DEEPSEEK_V4_SOFT_THRESHOLD_RATIO = 0.98   // 980k 开始压缩
const DEEPSEEK_V4_HARD_THRESHOLD_RATIO = 0.99   // 990k 强制压缩

export const MODEL_CONTEXT_PROFILES: readonly ModelContextProfile[] = [
  deepseekV4Profile('deepseek-v4-pro', ['deepseek-v4-pro']),
  deepseekV4Profile('deepseek-v4-flash', [
    'deepseek-v4-flash', 'deepseek-chat', 'deepseek-reasoner'  // 别名机制
  ])
]
```

每个 profile 的结构(同文件 :19-27):`canonicalModel` + `modelIds`(别名数组)+ `contextWindowTokens` + `softThreshold`/`hardThreshold` + 模态/工具能力(`inputModalities`、`supportsToolCalling`、`messageParts`)。模型匹配支持后缀匹配 `normalized.endsWith('/' + modelId)`(:98-100),可兼容 `provider/model` 形式的 id。

### 1.2 配置覆盖(两级)

- **kun 配置文件** `kun/config.example.json` 的 `models.profiles` 段:每个模型可配 `contextWindowTokens`、`contextCompaction.softThreshold/hardThreshold`(或 `softRatio/hardRatio` 按窗口比例算,见 `model-context-profile.ts:176-199` 的 `mergeModelContextProfile`/`thresholdFromWindow`)、`aliases`。顶层 `contextCompaction.defaultSoftThreshold/defaultHardThreshold` 是未知模型兜底。
- **GUI 主进程注入默认值**:`src/main/kun-process.ts:55-79` 定义 `DEFAULT_KUN_MODEL_PROFILES`(deepseek-v4-pro / v4-flash,1M 窗口、980k/990k 阈值),在拉起 kun 子进程时写入其配置——即 GUI 侧也维护了一份与内置表一致的硬编码默认。

合并逻辑 `modelContextProfilesFromConfig`(`model-context-profile.ts:131-150`):以内置表为底,配置逐模型覆盖;若配置只给 `contextWindowTokens` 没给阈值,按 `ratio ?? fallbackRatio`(继承内置比例,默认 0.98/0.99)自动折算(:218-226)。非法配置(hard < soft、既无窗口也无阈值)直接 throw(:194-199)。

### 1.3 向前端暴露

context window 通过 capabilities 清单暴露给 GUI:`modelCapabilitiesForModel`(`model-context-profile.ts:116-129`)→ 前端契约 `src/renderer/src/agent/kun-contract.ts:165-172` 的 `model.contextWindowTokens?: number`。**但前端目前只在设置页展示,没有用它做用量百分比仪表。**

---

## 2. token 用量统计

**结论:双通道 —— 本地"每 4 字符 ≈ 1 token"粗估 + API 响应 usage 的 `prompt_tokens` 实测,压缩判定取两者的 max;累计消费统计(成本/缓存)完全来自 API usage 字段的服务端聚合。**

### 2.1 本地估算器

`kun/src/loop/context-estimator.ts`(全文 48 行,极简):

```ts
export class ContextEstimator {
  constructor(charsPerToken = 4) { ... }
  estimateItem(item: TurnItem): number {
    const text = this.collectText(item)   // 按 item kind 抽文本:消息正文、工具名+JSON(arguments)、tool 输出 JSON、compaction summary…
    return Math.max(1, Math.ceil(text.length / this.charsPerToken))
  }
  estimateItems(items) { return items.reduce((sum, i) => sum + this.estimateItem(i), 0) }
}
```

注释明确说明设计意图:"目的是在合理阈值触发压缩,而不是精确复刻 provider 的 tokenizer"。

### 2.2 API usage 回灌(prompt pressure 机制)

`kun/src/loop/agent-loop.ts`:

- 流式响应每收到 `usage` chunk 就记录本 thread 的**最大** `promptTokens`(:739, :1492-1497 `recordPromptPressure`,只增不减);
- 下一次进入 `compactIfNeeded` 时 `consumePromptPressure`(:1277, :1611-1617)取出并清除;
- 判定用 `Math.max(本地估算, 实测 prompt_tokens)`(`context-compactor.ts:75-77`):

```ts
const estimatedTokens = this.estimate(compactableItems)
const tokens = Math.max(estimatedTokens, promptTokens ?? 0)
if (tokens < thresholds.softThreshold) return null
```

这样本地估算偏低时会被真实值纠正,估算偏高时提前压缩也无害——**这是"不装 tokenizer 也能可靠触发压缩"的关键设计**。

### 2.3 累计用量聚合(供 UI 展示)

usage 事件(`kun/src/contracts/events.ts:200` 附近 `UsageEvent`)持久化后,由服务端 `/v1/usage?group_by=thread|day|model` 聚合,字段含 `input_tokens / output_tokens / reasoning_tokens / cached_tokens / cache_miss_tokens / cache_hit_rate / cost_usd / cost_cny / cache_savings_usd / token_economy_savings_tokens / turns`。前端读取逻辑在 `src/renderer/src/hooks/use-thread-usage.ts:111-185`(`loadThreadUsage`,并另拉 `/v1/threads/:id` 从每个 turn 的 `usage.prompt_cache_hit_tokens/prompt_cache_miss_tokens` 汇总缓存命中,:81-109)。

---

## 3. 自动压缩 / 上下文裁剪

核心类 `ContextCompactor`,`kun/src/loop/context-compactor.ts`。

### 3.1 触发条件:三档阈值

`planCompaction`(:71-93):

| 档位 | 条件 | keepRecent(保留最近原文条数) |
|---|---|---|
| `normal` | tokens ≥ softThreshold | 4 |
| `aggressive` | tokens ≥ soft + (hard−soft)×0.6(:192-195) | 2 |
| `force` | tokens ≥ hardThreshold | 1 |

返回的 `reason` 会写进压缩摘要,注明数据来源是 `usage prompt_tokens` 还是 `estimated prompt tokens`(:86-92)。触发点在每个 turn 开始构造请求前:`agent-loop.ts:1271-1342` `compactIfNeeded`。

### 3.2 压缩流程(`compact`,:101-167)

1. 支持 `frozenMessageCount` 冻结开头 N 条不参与压缩(:117-121,用于子代理/复盘场景);
2. `trimTrailingToolCalls`(:182-190)剔除末尾悬空的 tool_call,避免压缩后出现"有 call 无 result"的坏历史;
3. 历史切成 `head`(被折叠)+ `tail`(最近 keepRecent 条保留原文);
4. head 生成**一条 `compaction` kind 的 item** 替换,新历史 = `[...frozen, summaryItem, ...tail]`;
5. 对 head 内容算短哈希 `sourceDigest` + `digestMarker`(`kun/src/loop/compaction-marker.ts`),写入摘要尾部——用于幂等/溯源,防止同一段历史被重复折叠;
6. 压缩后清空文件读取追踪 `toolHost.clearReadTracker`(agent-loop.ts:1320),并把 summaryItem 落盘 + 广播 `compaction_completed` 事件(:1321-1339)。

### 3.3 摘要内容:启发式(默认)与模型摘要(可选)

**启发式摘要** `buildCompactionSummary`(context-compactor.ts:209-260),纯本地拼装,零成本零延迟:

- Reason / Mode / Budget 头部;
- **Pinned constraints**(来自 immutable prefix 的用户/项目约束)逐条列出——"即使原文被删,约束也存活"(:229-236);
- **Skill pins**:从历史里正则抽取 `Active Skill:|Skill Pin:|Pinned Skill:` 行保留(:262-275);
- 每条 item 一行摘要(`summarizeItem` :282-307,截断到 360 字符;reasoning 直接丢弃);
- 超过 20 行时保留前 4 + 后 14,中间折叠为 "N middle item(s) omitted"(:309-318);
- 总字符预算 `budgetTokens*4`,clamp 在 1200~12000 字符(:277-280)。

**模型摘要**(config `contextCompaction.summaryMode: 'model'`),`agent-loop.ts:1292-1313` + `summarizeCompactionWithModel`(:1344-1447)。prompt 摘录(`buildModelCompactionPrompt` :1844-1867):

```
Summarize the following Kun conversation history for a context fold.
Preserve user goals, requirements, decisions, files touched, tool outcomes,
errors, constraints, active/pinned skills, and unresolved next steps.
Do not invent facts. Do not include generic advice. Prefer concise bullets grouped by topic.

Existing heuristic summary to cross-check:
<启发式摘要>

History excerpt to fold:
<[user]/[assistant]/[tool_call:x]/[tool_result:x] 逐行转写,单条截断 800~2000 字符,总量按 summaryInputMaxBytes(默认 96KB)截断>
```

调用参数:`temperature: 0`、`reasoningEffort: 'off'`、`maxTokens` 默认 1200、超时默认 15s(config.example.json `contextCompaction` 段)。

**失败处理(值得照抄)**:模型摘要超时/报错/返回空 → 一律回退启发式摘要,并 record 一条 `kind:'error', code:'compaction_summary_fallback'` 事件(:1362-1372, 1406-1442);前端把该错误码映射为温和的状态条文案而非红色报错(`kun-mapper.ts:819-823`,zh 文案:"模型压缩摘要暂不可用;Kun 已改用启发式摘要",`locales/zh/common.json:816`)。**压缩本身永不因摘要失败而失败。**

### 3.4 手动压缩命令

- 前端斜杠命令 `/compact [reason]`:`src/renderer/src/components/chat/floating-composer-commands.ts:4,31`;`FloatingComposer.tsx:906-908, 1069-1075` 解析后调 store;
- store action `compactActiveThread`(`src/renderer/src/store/chat-store-maintenance-actions.ts:242-270`):校验非 busy、runtime ready 后调 provider `compactThread`;
- HTTP 端点:`kun/src/server/routes/turns.ts:79-96` `compactTurn` → `kun/src/services/turn-service.ts:136-192` `compact()`(手动路径不走阈值判定,直接 compact 并发 `compaction_completed`)。

### 3.5 发送时轻量裁剪(与压缩互补的第二层)

不改持久化历史、只在构造模型请求时生效的两套机制:

- **request-history-hygiene**(`kun/src/loop/request-history-hygiene.ts`):限制每条 tool_result 的行数/字节/token(默认 320 行 / 32KB / 8000 token)、tool 参数字符串(8KB / 2000 token)、数组元素(80 个),base64 一律替换为占位符;截断时优先保留含 error/warning 等"信号行"(SIGNAL_LINE_RE :24-25)。
- **token economy 模式**(`kun/src/loop/token-economy.ts`,默认关闭):压缩工具描述/schema 的 prose(去客套、去冠词,但用 `PROTECTED_PATTERNS` :61-70 保护代码块/URL/路径/标识符不被改写)、按工具类型压缩 tool 输出(bash 保留 head 24 + tail 96 + 信号行,:302-323)、给模型注入"简洁回复"指令(:27-32),并统计节省的 token/成本回显到 UI。

---

## 4. 前端 / UI 展示

### 4.1 发送框的用量指示器

位置:composer **底部 footer 左侧**(agent-builder 计划放右下角,形态可直接参考)。`src/renderer/src/components/chat/FloatingComposer.tsx:1742-1819`:

- 形态:圆角小胶囊(`rounded-lg border bg-ds-card/72 px-2.5 text-[12.5px]`),`BarChart3` 图标 + 用 `·` 分隔的紧凑指标:`tokens 总量 · 成本 · [省下的成本(绿色)] · 缓存命中% · turns`;
- 数字用 `formatCompactNumber`(1.2k / 3.4M,`use-thread-usage.ts:40-44`)+ `tabular-nums`;成本按语言自动切 `$ / ￥`(:60-67);
- 整个胶囊的 `title` tooltip 给完整明细(:1749-1765);
- 刷新时机:`useThreadUsageState(threadId, enabled, refreshKey)`,refreshKey 由 `updatedAt : busy/idle : usageRefreshKey` 拼成(:516-519),即**每个 turn 结束后重新拉取**,不做流式实时更新;
- 加载/空态显示 "加载中…/暂无用量"(:1811-1817)。

**注意:展示的是本会话累计消费(cost 视角),不是"当前上下文占窗口百分比"(context 视角)。该项目没有 context 用量百分比仪表、没有颜色分级(仅节省金额用 emerald 绿),也没有 system prompt/tools/messages 的当前上下文分布明细——agent-builder 需要的"占比环形图"该项目无此能力,需自行设计。** 最接近的是使用统计页 `InitialSessionUsageHeatmap.tsx`(按天热力图 + 按模型堆叠条,颜色区分 cachedInput/uncachedInput/output,:40-45),那是历史消费分布,不是当前 context 分布。

### 4.2 对话流中的压缩提示

压缩以 **`compaction` 类型的 block** 出现在 turn 的"过程区"(与 tool/reasoning 同级的折叠时间线),不是独立气泡:

- 事件链:SSE `compaction_started/completed` → `kun-mapper.ts:926-930` 映射为 `status: 'running' | 'success'` 的 `CompactionEventPayload` → chat-store `onCompaction` 追加/更新 block(`chat-store-side-actions.ts:177-193`);历史回放时由 `compaction` item 映射(`kun-mapper.ts:709-718`);
- 文案(`message-timeline-process.tsx:933-942` + `locales/zh/common.json:1200-1204`):running → "正在压缩上下文"(shiny 流光动画,`processBlockIsAutoOpenPending` :222-229 让 running 状态自动展开);success → "已压缩上下文 / 已自动压缩上下文";error → "上下文压缩失败"(红色);
- 行首图标 `Minimize2`(:376-378);行可点击展开 detail——detail 内容是 **pinned constraints 列表**(`kun-mapper.ts:715` `detail: item.pinnedConstraints.join('\n')`),即告诉用户"压缩后哪些约束仍然生效";
- 契约里预留了 `compactionCompletedWithCounts`("已压缩上下文({{before}} → {{after}} 条消息)"),但见 5.2 的坑。

### 4.3 设置页的 context 配置 UI

`src/renderer/src/components/settings-section-agents.tsx`(Agents 设置 → "高级"折叠区):

- **只读展示当前模型 profile**(:862-905):四张小卡片 —— 模型 / 上下文窗口 token / 开始压缩 token / 强制压缩 token,并标注来源("内置模型配置" vs "未匹配模型,使用下方兜底阈值",`locales/zh/settings.json:188-195`);已知模型的数值在前端**又硬编码了一份** `DEEPSEEK_V4_CONTEXT_PROFILE`(:69-73);
- **可编辑项**只有全局兜底:`defaultSoftThreshold` / `defaultHardThreshold` 数字输入(:946, :957)、摘要方式下拉(本地规则/模型生成,:974)、摘要超时/最大 token/输入字节(settings.json:196-207);
- **没有"逐模型编辑 context window"的表单 UI**——逐模型配置只能手改 kun 的 config.json(设置文案明确引导:"模型窗口和模型级压缩阈值来自本地 config.json 的 models.profiles")。设置持久化 schema 在 `src/shared/app-settings-kun.ts:152-160, 348-357`。

---

## 5. 借鉴与规避

### 5.1 值得借鉴的精华

1. **`max(本地粗估, API prompt_tokens)` 的双通道判定 + prompt pressure 回灌**。Go 侧无需引入 tokenizer,`len(text)/4` 起步,每次流式响应把 `usage.prompt_tokens` 记为该会话水位,下一 turn 取 max 判阈值。简单、自纠错、provider 无关。
2. **soft/aggressive/force 三档 + keepRecent 递减(4/2/1)**,以及阈值统一用 `窗口 × ratio` 推导(0.98/0.99 这类比例存 profile,换模型不用重配)。agent-builder 建议比例更保守(如 0.75/0.92),DeepSeek 敢用 0.98 是因为 1M 窗口。
3. **压缩摘要的"永不失败"降级链**:启发式摘要兜底 → 模型摘要可选增强(temperature 0、独立超时、限 maxTokens/输入字节)→ 失败回退并发一条低调的状态事件。启发式摘要里**强制保留 pinned constraints 和 skill pins** 的做法,直接对应 agent-builder 的 system prompt/skills 场景。
4. **压缩产物是历史中的一等 item + 独立 SSE 事件**(`compaction_completed` 带 summary/replacedTokens/sourceDigest/sourceItemIds),前端把它渲染成过程区的可展开行。事件模型与 agent-builder 的 runtime_events.go 体系可直接对齐(新增 compaction 事件类型即可)。
5. **`trimTrailingToolCalls` + 压缩后 `clearReadTracker`**:两个容易漏掉的正确性细节——不把悬空 tool_call 卷进压缩边界;压缩后清除"文件已读"缓存,强制模型重新 read。
6. **digestMarker 幂等标记**:摘要尾部附短哈希,避免重复折叠同一段历史,也便于校验/调试。
7. **发送时 hygiene(限制单条 tool result 行/字节/token,保留 error 信号行)与持久化压缩分层**:大部分"上下文爆炸"来自工具输出,先在请求构造时裁剪,能显著推迟真正的 compaction。

### 5.2 应规避的糟粕

1. **context window 数值在三处重复硬编码**(kun 内置表 `model-context-profile.ts:75-90`、GUI 主进程 `kun-process.ts:55-79`、设置页 `settings-section-agents.tsx:69-73` + 模型 id 白名单 :87)——改一个模型要动三处。agent-builder 应做成**单一来源**:Go 侧一张默认表 + 用户配置覆盖,前端一律通过 API/bridge 读取,不在 React 里复刻数值。
2. **没有从 provider API 自动获取 context window**。该项目只服务 DeepSeek 所以无所谓;agent-builder 是多 provider,建议"默认表 → 若 provider 的 /models 返回 context_length(如 OpenRouter/Ollama)则自动覆盖 → 用户手动配置最高优先"。
3. **`messagesBefore` 字段语义错位的沉睡 bug**:mapper 把 `replacedTokens`(token 数)塞进 `messagesBefore`(消息数)且从不设置 `messagesAfter`(`kun-mapper.ts:580,715,793`),导致 "{{before}} → {{after}} 条消息" 文案永远走不到——契约字段与展示文案没有端到端验证。定事件 schema 时把 tokens/条数分开命名并两端都测。
4. **`compaction_started` 事件后端从未发出**(契约 `events.ts:33` 和前端 running 映射都在,但 `agent-loop.ts`/`turn-service.ts` 只发 `completed`),"正在压缩上下文" 的 running 态实际展示不出来。agent-builder 要做"正在压缩"提示,必须真的在压缩前发 started 事件(尤其模型摘要模式下压缩可能耗时 10s+)。
5. **用量指示器只有"累计消费"视角,缺"当前上下文占用"视角**:capabilities 已经暴露了 `contextWindowTokens`,前端却没做 已用/窗口 百分比。agent-builder 的核心诉求恰是后者,不要照抄这里的 footer 而丢了百分比仪表。
6. **usage 拉取是 turn 级轮询拼 refreshKey**(`FloatingComposer.tsx:516-519`),而 usage 本来就有 SSE 事件,存在重复通道。agent-builder 已有事件流,直接在 usage/compaction 事件里携带上下文水位推给前端即可,不必再造 HTTP 轮询。
7. token economy 的英文 prose 压缩(去冠词/客套的正则,`token-economy.ts:52-60`)对中文内容基本无效且有语义风险,不建议引入;只取其"按工具类型裁剪输出 + 保留信号行"的部分。

---

## 6. 对 agent-builder 的借鉴要点(落地清单)

对照需求逐条给出方案:

1. **长对话自动压缩(Go 后端)**
   - 在 runtime_service 的 turn 启动路径加 `compactIfNeeded`:`tokens = max(estimate(history), lastPromptTokens[conversationID])`;estimate 用 `ceil(len/4)`(中文可用 `len(runes)/1.6` 微调,但保持"宁高勿低"即可);
   - 阈值来自 model profile:`soft = window × softRatio`(默认 0.75~0.8)、`hard = window × 0.92`,三档 keepRecent 4/2/1;
   - 压缩产物 = 一条 `compaction` 消息(summary + replacedTokens + sourceDigest),持久化进会话;摘要默认启发式(保留 system 约束、活跃 skill、文件清单、未完成步骤),模型摘要作为可选项并带超时回退;
   - 复用 3.2 的细节:trimTrailingToolCalls、压缩后清 read 缓存、digest 幂等;手动入口 `/compact` 命令 + bridge 方法。

2. **发送框右下角 context 用量图标(React)**
   - 数据:usage 事件里附带 `{promptTokens, contextWindow, breakdown:{systemPrompt, skills, tools, messages}}`——breakdown 由 Go 侧在构造请求时按段落用同一 estimator 计算(DeepSeek-GUI 无此能力,需自建;计算点就在拼 request 的地方,各段 `len/4` 即可);
   - 形态:参考 4.1 的胶囊(图标 + 紧凑数字 + tooltip),但主体显示 `已用/窗口 %`,颜色分级建议 <60% 灰、60-80% 黄、>80% 红(该项目无分级,自定);点击弹出分布明细(占比条形)。

3. **对话流压缩提示**
   - 事件:`compaction_started`(压缩前必发,见 5.2-4)/ `compaction_completed`(summary、replacedTokens、保留条数);
   - UI:过程区一行,running 态流光/spinner 自动展开,完成态 `Minimize2` 图标 + "已自动压缩上下文(省下 ~N tokens)",可展开看摘要与保留的约束;摘要降级时发低调状态条而非错误。

4. **设置:按 provider/model 配 context window 与阈值**
   - Go 侧单一默认表(model id + 别名 + window + ratios),支持 `endsWith("/id")` 别名匹配;用户配置按模型覆盖,只给 window 时按 ratio 自动折算,校验 hard ≥ soft;
   - 自动获取:provider 的模型列表 API 若带 context_length 字段则填充默认,手动配置优先;
   - 设置 UI:当前模型 profile 只读卡片(模型/窗口/软阈值/硬阈值 + 来源标注)照抄 4.3 的形态即可,但把"逐模型编辑"做进 UI(该项目只能手改 json,是短板),外加全局兜底阈值 + 摘要方式(本地/模型)两项。
