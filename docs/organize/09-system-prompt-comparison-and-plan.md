# System Prompt 机制梳理对比与 Claude 式拆分目标

本文聚焦 Agent Builder 的 system prompt 机制，并与以下参考项目对比：

- `C:\Users\ytq\work\ai\cc-haha`
- `C:\Users\ytq\work\ai\DeepSeek-GUI`
- `C:\Users\ytq\work\ai\myclaw\claude-code`

结论倾向：Agent Builder 后续不应只做“更稳定的字符串拼接”，而应直接按 Claude Code 的 section 化模型确立目标，把 system prompt 拆成有优先级、有缓存边界、有来源追踪、有运行可观测性的 prompt assembly graph。

## 结论摘要

- Agent Builder 当前 system prompt 已经具备基础能力：内嵌 coder prompt、context files、AGENTS/CLAUDE 兼容加载、skills XML、MCP instructions、provider system prompt prefix、prompt assembly 持久化摘要。
- 当前主要问题不是“缺 prompt”，而是拆分边界不够清晰。基础规则、环境信息、上下文文件、skills catalog、MCP instructions、provider prefix、compact/context projection 等被分散在不同路径，且最终大多收敛为一个较粗的 system prompt summary。
- Claude Code / cc-haha 的优势是 section graph：system prompt 是 `string[]` sections，静态段、动态段、缓存边界、CLAUDE.md user context、system context、MCP instructions delta、agent/custom append prompt 都有明确位置。
- DeepSeek-GUI / Kun 的优势是 cache-first invariant：`KUN_SYSTEM_PROMPT` 是不可变 prefix，skills/memory/todo/plan 等动态事实作为后续 system messages 或 context instructions 注入，不污染稳定 prefix。
- Agent Builder 应采用 Claude Code 的拆分模型作为产品目标，同时吸收 Kun 的稳定前缀约束：稳定基础 prompt 不随日期、git、MCP 连接、skills 变化、context files 改动而漂移。

## Agent Builder 当前机制

### 主执行链路

关键文件：

- `internal/agent/coordinator.go`
- `internal/agent/agent.go`
- `internal/agent/prompts.go`
- `internal/agent/prompt/prompt.go`
- `internal/agent/templates/coder.md.tpl`
- `internal/skills/skills.go`
- `internal/runtime/runtime_context.go`
- `internal/runtime/runtime_prompt_assembly.go`
- `internal/runtime/runtime_prompt_assembly_store.go`
- `internal/runtime/runtime_contract_types.go`

当前主链路：

```text
coordinator.buildAgent / buildSystemPrompt
  -> coderPrompt(prompt.WithWorkingDir, prompt.WithSkills)
  -> prompt.Build
  -> coder.md.tpl
  -> sessionAgent.Run
  -> append connected MCP instructions
  -> fantasy.WithSystemPrompt(systemPrompt)
  -> optional provider SystemPromptPrefix as extra system message
  -> model input projection
  -> recordPromptAssembly
  -> runtime prompt assembly store/API/UI diagnostics
```

### 当前注入内容

基础 coder prompt：

- 身份与 critical rules。
- 工作流、编辑、测试、工具使用、git/PR 等行为规则。
- env 信息：working directory、git repo、platform、date、git status snapshot。
- LSP presence。
- available skills XML。
- skills usage mandatory activation flow。
- memory/context files。

context source：

- 配置默认路径包括 `.github/copilot-instructions.md`、`.cursorrules`、`.cursor/rules/`、`.agents/rules/`、`CLAUDE.md`、`CLAUDE.local.md`、`GEMINI.md`、`agent-builder.md`、`AGENTS.md` 等。
- 还会向上发现 `AGENTS.md`、`CLAUDE.md`、`.claude/CLAUDE.md`、`.claude/rules/*.md`、`AGENTS.local.md`、`CLAUDE.local.md`。
- 支持 `@path` include、循环检测、深度限制、UTF-8/大小限制、frontmatter path rule。

skills：

- `skills.ToPromptXML` 只注入 `name`、`description`、`location`、`type`。
- 完整 `SKILL.md` body 不直接进入 system prompt，由模型通过 View 读取。
- `deny_all` policy 下不注入 skills。

MCP instructions：

- `sessionAgent.Run` 每次从 connected MCP server 的 initialize result 取 `Instructions`。
- 追加为 `<mcp-instructions>...</mcp-instructions>`。

provider prefix：

- provider config 的 `system_prompt_prefix` 不拼进主 system prompt 字符串。
- 在 prepared messages 前 prepend 一条 `system` message。

prompt assembly：

- 当前记录 system prompt hash、length、token estimate、prompt prefix hash/token、skills summary、MCP summary、tool summary、message summary、context sources、compact、budget。
- raw prompt 不存储，默认 redacted。

## 参考项目对比

### myclaw/claude-code

关键文件：

- `src/constants/prompts.ts`
- `src/constants/systemPromptSections.ts`
- `src/utils/systemPrompt.ts`
- `src/context.ts`
- `src/utils/claudemd.ts`
- `src/utils/api.ts`
- `src/services/api/claude.ts`

机制特点：

- `getSystemPrompt()` 返回 `string[]`，不是单个拼接字符串。
- 静态内容和动态内容之间有 `SYSTEM_PROMPT_DYNAMIC_BOUNDARY`。
- `systemPromptSection(name, compute)` 会缓存 section，直到 `/clear` 或 `/compact`。
- `DANGEROUS_uncachedSystemPromptSection(name, compute, reason)` 显式标注会破坏 prompt cache 的动态 section。
- `buildEffectiveSystemPrompt()` 负责处理 override、coordinator mode、agent prompt、custom prompt、append prompt 的优先级。
- `splitSysPromptPrefix()` 在 API 层把 attribution、CLI prefix、static block、dynamic block 拆成不同 cache scope。
- CLAUDE.md 不只是 system prompt 字符串的一部分，而是通过 `getUserContext()` 作为 meta user reminder prepend 到消息里。
- git status、cache breaker 等通过 `getSystemContext()` 管理。
- MCP instructions 可作为 system section，也可通过 delta attachment 避免 late MCP connect 破坏缓存。

可借鉴点：

- section 是一等结构，而不是诊断后再反推。
- 每个 section 有名字、缓存策略、来源和清理时机。
- override / append / agent prompt / coordinator prompt 优先级清晰。
- CLAUDE.md、system context、MCP instructions 不混在同一层。

### cc-haha

关键文件与 Claude Code 基本同源：

- `src/constants/prompts.ts`
- `src/constants/systemPromptSections.ts`
- `src/utils/systemPrompt.ts`
- `src/context.ts`
- `src/services/api/claude.ts`

机制特点：

- 底层 prompt section 模型与 Claude Code 基本一致。
- 额外有 desktop、trace、settings、IM/adapters 等承载线索。

可借鉴点：

- 桌面化展示可以借鉴 trace/detail/settings 入口。
- prompt assembly 的前端可见性可不照搬 TUI transcript，而应映射成 Agent Builder runtime diagnostics。

### DeepSeek-GUI / Kun

关键文件：

- `kun/src/prompt/kun-system-prompt.ts`
- `kun/src/cache/immutable-prefix.ts`
- `kun/src/loop/agent-loop.ts`
- `kun/src/ports/model-client.ts`
- `kun/src/adapters/model/deepseek-compat-model-client.ts`
- `kun/src/skills/skill-runtime.ts`

机制特点：

- `KUN_SYSTEM_PROMPT` 是稳定前缀，明确要求 runtime/user-specific facts 不要进入 prefix。
- `ImmutablePrefix` 包含 `systemPrompt`、tools、pinned constraints、few shots、fingerprint、revision。
- `verifyImmutablePrefix()` 可检测 drift。
- plan mode 作为 `modeInstruction` 第二条 system message。
- skills/memory/todo/tool catalog drift 等作为 `contextInstructions` 注入。
- model adapter 按顺序输出：
  1. stable `systemPrompt`
  2. optional `modeInstruction`
  3. per-turn `contextInstructions`
  4. prefix few shots + history

可借鉴点：

- 稳定前缀是 invariant，不是优化建议。
- 动态事实必须后置，且有独立 token/fingerprint 可观测性。
- skills 是按 turn 激活和预算注入，不是把所有技能都长期塞进主 prompt。

## 目标模型

Agent Builder 目标应定义为：Claude 式 section graph + Kun 式 stable prefix invariant。

### 核心原则

1. system prompt 不再被视为单个字符串，而是一组有类型、有顺序、有来源、有缓存策略的 sections。
2. stable base section 必须字节稳定，不能包含日期、git status、workspace path、MCP connected state、skills list、context files 等易变内容。
3. 动态内容仍可作为 system role 注入，但必须位于 stable boundary 之后，并在 assembly 中独立记录。
4. AGENTS/CLAUDE/context files 应从“模板内 memory block”升级为 `user_context` 或 `project_instructions` sections，独立 hash/token/provenance。
5. skills 应拆成两层：
   - skill catalog section：低成本能力索引，类似当前 `available_skills`。
   - skill body section：只有触发或显式加载后才注入或通过 View 工具进入上下文。
6. MCP instructions 应拆成 stable/discovered/delta 三类，不再直接混入主 system prompt summary。
7. provider/system prompt prefix、agent override、append prompt、mode prompt 必须有明确优先级，不靠调用点隐式 prepend。
8. prompt assembly 是事实源：前端、audit、replay、budget、context diagnostics 都从 section snapshot 读取，不从 raw prompt 或 tool text 反推。

### Section 类型

建议定义：

```text
stable_base
runtime_contract
tool_behavior
response_style
safety_policy
provider_prefix
agent_override
append_system_prompt
mode_instruction
session_guidance
environment_info
git_status
project_instructions
user_memory
local_memory
context_file
skill_catalog
skill_body
mcp_instructions
mcp_instructions_delta
compact_reinjection
read_file_state
budget_instruction
hook_context
```

### Section 属性

每个 section 至少应有：

```go
type PromptSection struct {
    ID            string
    Name          string
    Kind          string
    Role          string // system/user/developer-like/meta
    Order         int
    CachePolicy   string // stable/session_cached/turn_dynamic/uncached
    Source        string
    SourceRefs    []string
    Scope         string // runtime/provider/session/project/user/local/mcp/skill
    Content       string // only inside assembly build, not necessarily persisted
    Hash          string
    Length        int
    TokenEstimate int
    Redacted      bool
    RawStored     bool
    Diagnostics   string
}
```

持久化 DTO 不应默认保存 `Content`，只保存 hash/length/token/source/provenance。

### 优先级模型

建议按 Claude Code 的 `buildEffectiveSystemPrompt()` 明确优先级：

1. explicit override system prompt：替换默认 prompt。
2. coordinator / special runtime mode prompt：替换或包裹默认 prompt，按模式定义。
3. main agent role prompt：内置 agent 可 append，外部 agent 默认 replace，具体策略配置化。
4. provider prefix：作为独立 section，不隐式插入 messages。
5. stable default sections。
6. dynamic runtime sections。
7. append system prompt：最后追加，但仍作为独立 section 记录。

### 消息输出模型

第一版不必引入 OpenAI developer role，可先统一映射到 provider 能接受的 role：

```text
system messages:
  stable_base joined block
  provider_prefix
  mode_instruction
  dynamic_system_sections joined or separate blocks

meta user message:
  project/user/local instructions if provider/cache strategy要求从 system 移出

normal messages:
  compacted/projected session history
```

如果 provider 支持 system block cache metadata，则 API adapter 层再拆 cache scope；Go runtime 不应丢失 section 边界。

## 推荐实施路线

### Phase 0：事实冻结与测试基线

目标：

- 冻结当前 prompt assembly 事实，防止重构期间不可见漂移。

工作：

- 增加当前 `coder.md.tpl` build snapshot 测试，记录关键 section presence。
- 增加 MCP instructions、provider prefix、skills XML、AGENTS.md/CLAUDE.md 加载顺序测试。
- 明确 prompt assembly 当前字段和不存 raw content 的安全约束。

验收：

- 能用测试证明当前行为未被误删。
- 能列出一个 turn 中每类 prompt 来源是否进入模型。

### Phase 1：引入 PromptSection 内部模型

目标：

- 先不改变模型实际输入，只改变内部装配结构。

工作：

- 新建 `internal/agent/prompt/sections.go` 或同等文件。
- 把当前 coder prompt build 结果包装为 `stable_base` / `legacy_coder_prompt` section。
- 把 provider prefix、MCP instructions、skills XML、context files 先作为独立 section 记录，但最终仍按旧方式 join，确保行为兼容。
- `PromptAssemblySnapshot` 增加 `Sections` summary。

验收：

- 模型实际输入 hash 与旧逻辑一致或有受控差异。
- runtime prompt assembly API 能看到 sections summary。

### Phase 2：拆 stable base 与 dynamic sections

目标：

- 真正把稳定基础规则从日期/git/context/skills/MCP 中分离。

工作：

- 将 `coder.md.tpl` 拆成多个模板或 builder：
  - stable identity/rules/tool behavior/response style/safety。
  - env/date/git。
  - skills catalog。
  - memory/context files。
  - skills usage。
- stable base 不包含 `Date`、`WorkingDir`、`GitStatus`、`AvailSkillXML`、`ContextFiles`。
- env/date/git 改为 `environment_info` / `git_status` dynamic sections。
- context files 改为 `project_instructions` / `user_memory` / `local_memory` sections。

验收：

- stable base hash 在同一版本、不同工作区、不同日期下保持一致。
- dynamic sections 的 hash/token 单独变化。

### Phase 3：Claude 式 section cache 和 invalidation

目标：

- 引入类似 `systemPromptSection` 的缓存/清理机制。

工作：

- section cache policy：
  - `stable`: 版本内稳定。
  - `session_cached`: session 内缓存，clear/compact/config reload 后刷新。
  - `turn_dynamic`: 每 turn 计算。
  - `uncached`: 每次模型 step 计算，必须带 reason。
- skills list、MCP instructions、context files 进入各自 invalidation key。
- config reload、skills refresh、MCP reconnect、context file changed、compact 后有明确 cache invalidation。

验收：

- section cache change 能在 prompt assembly 中看到原因。
- 易变 section 不会改变 stable base hash。

### Phase 4：AGENTS/CLAUDE/context 分层对齐 Claude

目标：

- 把 instructions/memory 从“模板内 memory XML”升级为可审计 context sections。

工作：

- 建立加载顺序：
  1. managed instructions
  2. user memory
  3. project instructions root -> cwd
  4. `.claude/CLAUDE.md`
  5. `.claude/rules/*.md`
  6. local memory
  7. configured context paths
- 明确 `AGENTS.md` 与 `CLAUDE.md` 的优先级。建议 Agent Builder 自身优先 `AGENTS.md`，但保留 Claude 兼容。
- include、frontmatter、exclude、scope、diagnostics 全部进入 section provenance。
- 可选：将 project/user instructions 改为 meta user reminder，而不是 system section，以贴近 Claude Code。

验收：

- UI 能解释每条 instructions 从哪里来、为什么被加载或跳过。
- 大文件、重复、include cycle、outside scope 都有 section diagnostic。

### Phase 5：Skills 改为 catalog + activation

目标：

- 不再长期把所有 active skills 当同一块 prompt XML。

工作：

- 保留 `skill_catalog`：name、description、location、type。
- 增加 `skill_activation`：匹配当前用户任务后才加载 skill body 或增加强提醒。
- skill tracker 从 name 扩展到 source/path/hash。
- prompt assembly 记录 available、activated、loaded、omitted_by_budget、disabled_by_policy。

验收：

- 大量 skills 不会线性膨胀每 turn system prompt。
- 被激活 skill 的 body/summary 能被单独追踪。

### Phase 6：MCP instructions delta

目标：

- 避免 MCP server late connect/disconnect 破坏稳定 system prompt。

工作：

- MCP instructions 独立为 `mcp_instructions` section。
- 记录 instruction 内容 hash/token，而不是仅 server name hash。
- 支持 delta：新连接 server 的 instructions 作为后置 section 或 meta attachment 注入。
- prompt assembly 区分 server list hash 与 instruction content hash。

验收：

- MCP 连接变化只影响 MCP section，不影响 stable base。
- diagnostics 能显示哪个 server 提供了 instructions、何时变化。

### Phase 7：Provider adapter cache scope

目标：

- 把 section 边界传递到 provider adapter。

工作：

- 对支持 cache metadata 的 provider，把 stable sections 标成 cacheable block。
- dynamic sections 单独作为 non-cache block。
- 对不支持多 system block 的 provider，按顺序 join，但保留 assembly section 信息。
- provider prefix 不再特殊 prepend，而是标准 section。

验收：

- prompt cache 命中率变化可以被 runtime/usage 解释。
- provider 差异不泄漏到业务层。

### Phase 8：前端 prompt assembly 可观测性

目标：

- 让用户能看到 prompt 由哪些 section 组成，而不是只看总 hash/token。

工作：

- `RuntimePromptAssembly` 增加 sections summary。
- Context diagnostics 增加 section tree。
- 展示 stable/dynamic/cache policy/source/length/token/hash/redacted。
- 支持按 turn/step 查看 prompt assembly diff。

验收：

- 用户能定位“这条指令来自哪个 AGENTS.md/CLAUDE.md/skill/MCP server”。
- 用户能定位“为什么 prompt cache 被破坏”。

## 目标数据流

```text
Runtime config / provider config / policy
  -> PromptSectionRegistry
  -> PromptSectionResolver
      -> stable sections
      -> session cached sections
      -> turn dynamic sections
      -> uncached sections with reason
  -> PromptAssembler
      -> provider role mapping
      -> cache boundary mapping
      -> model input projection
  -> PromptAssemblyRecorder
      -> section summaries
      -> hashes / token estimates
      -> source refs / diagnostics
  -> Runtime API / frontend diagnostics / audit / replay
```

## 关键设计决策

### 1. 以 Claude 模型为目标，不以 Kun 模型替代

Kun 的 stable prefix 很适合 cache-first，但 section 类型较少。Agent Builder 的扩展面更接近 Claude Code：skills、plugins、MCP、hooks、AGENTS/CLAUDE、subagent、provider prefix、mode prompt 都需要细粒度 section graph。

因此目标应是 Claude 式 section graph，Kun 的 fingerprint/invariant 作为 stable base 的质量门禁。

### 2. 保留 Agent Builder 对 AGENTS.md 的优先支持

Claude Code 以 `CLAUDE.md` 为核心。Agent Builder 应继续把 `AGENTS.md` 作为首要项目指令格式，同时兼容 `CLAUDE.md`。建议后续明确：

- 同目录同时存在 `AGENTS.md` 和 `CLAUDE.md` 时，`AGENTS.md` 优先。
- `CLAUDE.md` 作为兼容 project_claude section。
- local variants 后置，优先级更高，但标记 scope=local。

### 3. 不默认存 raw prompt

prompt assembly 必须继续默认 redacted。需要 debug dump 时应走显式开发者开关，并明确路径、权限和脱敏策略。

### 4. 重构顺序必须先内部结构、后行为切换

直接改最终模型输入风险过高。先用 section 模型复刻旧输入，再逐步拆 stable/dynamic，才能让测试定位差异。

## 验收标准

完成目标路线后，应满足：

- system prompt assembly 是 section graph，不是单字符串。
- stable base hash 在日期、cwd、git status、skills、MCP、context files 变化时保持不变。
- 每个 dynamic section 有独立 hash/token/source/cache policy。
- AGENTS/CLAUDE/context files 的加载顺序、include、scope、失败原因可审计。
- skills catalog 与 activated skill body 分离。
- MCP instructions content hash/token 被正确记录，late changes 不污染 stable base。
- provider prefix、agent override、append prompt、mode prompt 都有明确 section 和优先级。
- frontend 能查看 prompt assembly sections 和 diff。
- audit/replay/recovery 只依赖 section summaries，不需要 raw prompt。

## 推荐第一轮落地范围

如果控制第一轮 PR 范围，建议只做：

1. 新增 PromptSection 内部模型。
2. 在不改变模型输入的前提下，把现有 coder prompt、provider prefix、MCP instructions、skills XML、context files 映射成 section summaries。
3. 扩展 `PromptAssemblySnapshot` 和 `RuntimePromptAssembly`，持久化 section summaries。
4. 增加 focused tests，证明旧输入兼容、section summary 正确。
5. 修正 MCP instruction hash：区分 server list hash 和 instruction content hash。

第一轮完成后，再进入 stable/dynamic 真拆分。这样目标一次定清楚，但执行上保持可验证和可回滚。
