# Implementation Roadmap

本文记录从当前 Crush 代码库演进到 agentic operations client 的阶段性
里程碑。路线原则是：先建立 baseline，再做 UI 原型和 runtime 边界，
最后逐步补齐 operation、policy、scheduler、subagent、sandbox 和插件体系。

## 总体目标

最终目标是构建一个面向企业产品操作场景的 agentic operations client：

- 用户通过自然语言下发目标。
- Crush/Go runtime 调度 agent、工具和插件完成流程。
- 客户端展示任务状态、工具调用、风险、审批、日志、artifact 和报告。
- 支持长任务、失败恢复、审计、权限控制、插件扩展和后续 agent teams。

## 路线总览

```text
Phase 0: Crush Baseline
Phase 1: UI Prototype
Phase 2: Runtime API Boundary
Phase 3: Operation / Run Model
Phase 4: Tool Scheduler and Policy
Phase 5: Subagent Tasks
Phase 6: Wails Desktop Client
Phase 7: Sandbox and Workspace Isolation
Phase 8: Plugin / Capability Package
Phase 9: Enterprise Hardening
```

## Phase 0: Crush Baseline

### 目标

保持 Crush 原状，先确认当前项目可以在本地稳定构建、测试和运行。
这一阶段不做架构改造。

### 主要任务

- 记录本地环境：Go 版本、系统、shell、关键环境变量。
- 跑通 `go build .`。
- 跑通 `go run . --help`。
- 尝试运行 `go run .` 或非交互 `go run . run "hello"`。
- 视情况运行 `go test ./...` 或先跑关键包测试。
- 记录失败项、跳过项和当前限制。
- 形成 baseline 文档。

### 交付物

- `docs/archive/dev-baseline.md`。
- 当前构建、运行、测试结果记录。
- 已知问题列表。

### 验收标准

- 知道当前代码在本机是否能构建。
- 知道当前测试是否通过，失败原因是否记录。
- 知道当前运行 Crush 需要哪些配置和 provider 条件。
- 后续改动有可对照的起点。

### 暂不做

- 不改 runtime 架构。
- 不改工具系统。
- 不做 UI。
- 不做 operation/run。

## Phase 1: UI Prototype

### 目标

先验证 agentic operations client 的信息架构和交互体验，不依赖真实 Crush
runtime 改造。Phase 1 结束时需要形成一个可供本地验收的客户端版本。

### 主要任务

- 创建 React + TypeScript + Vite 前端原型。
- 引入 Ant Design 和 Ant Design X。
- 先建立简单聊天首屏，降低首次验收复杂度。
- 增加 LLM 连接配置：协议、URL、API key、高级代理。
- 通过本地 proxy 支持 OpenAI-compatible 和 Anthropic-compatible 协议。
- 将 Operations/SSH/SOP 能力保留为二级入口，而不是首屏信息墙。
- 增加 DeepSeek 报告生成验收通道。
- 形成可打包的验收版本。

### 交付物

- 前端原型工程。
- Claude Desktop 风格聊天首页。
- LLM 连接配置抽屉。
- 本地 DeepSeek/OpenAI-compatible chat proxy。
- UI 交互说明文档。
- Phase 1 acceptance build。

### 验收标准

- 能直接进入聊天，而不是先理解复杂运维看板。
- 能配置 LLM 连接，并看到当前选中的模型。
- 能通过真实 DeepSeek API 完成基础对话。
- `npm run build` 和 `npm run lint` 无错误、无告警。
- 能验证 Claude Desktop 风格的基础交互是否满足第一阶段体验。
- UI 与真实 Crush runtime 解耦。

### 暂不做

- 不直接调用真实集群。
- 不让真实模型驱动工具执行。
- 不接真实 Crush 工具。
- 不执行真实高风险动作。

### Phase 1 子阶段

```text
Phase 1.1: Mock Event Stream
Phase 1.2: Approval Interaction + Event Log
Phase 1.3: SOP Fixture Selector
Phase 1.4: SSH Target Config Mock
Phase 1.5: DeepSeek Report Generation
Phase 1.6: Acceptance Build
```

Phase 1.5 的 DeepSeek 接入只用于报告生成和建议生成。SOP、SSH 和 MCP 可以
继续使用 mock 数据。API key 不写入前端源码，验收阶段通过本地环境配置。

Phase 1.6 的验收构建包含桌面包基础：

- 使用 `desktop` 作为 Wails v3 thin shell。
- Wails 桌面壳嵌入共享的 `client/dist`，不复制业务 UI。
- 构建时由 `desktop/scripts/sync-client-dist.mjs`
  先执行 `client` 生产构建，再同步静态资源。
- Windows 可执行文件输出到
  `desktop/bin/AgentBuilder.exe`。
- 这一阶段仍然不连接真实 SSH/runtime，只用于本机第一阶段验收。

## Phase 2: Runtime API Boundary

### 目标

稳定 Crush 的 headless runtime 边界，让 TUI、未来桌面端、Web 控制台和
headless CLI 都能通过统一协议访问 runtime。

### 主要任务

- 梳理现有 `internal/workspace` 和 `internal/server`。
- 定义核心 API：
  - session。
  - turn。
  - message。
  - tool call。
  - permission request。
  - event stream。
- 明确 API transport：
  - HTTP/JSON-RPC 用于请求。
  - SSE 或 WebSocket 用于事件。
- 将现有 pub/sub 事件整理成 machine-readable event schema。
- 保持 TUI 继续可用。

### 交付物

- runtime API 设计文档。
- event schema 设计文档。
- 最小可用本地 API。
- 简单 API smoke test。

### 验收标准

- 客户端可以创建/读取 session。
- 客户端可以发送一轮用户输入。
- 客户端可以收到 assistant/tool/permission 事件。
- TUI 不被破坏。

### 暂不做

- 不设计完整企业权限。
- 不实现 operation/run。
- 不做桌面壳。

## Phase 3: Operation / Run Model

### 目标

引入企业操作任务的一等模型。Chat session 继续存在，但 operation/run 成为
任务闭环和审计的主线。

### 主要任务

- 设计 operation/run 数据模型。
- 新增 SQLite migrations。
- 建立 run event log。
- 将 session/message/tool call/permission 与 run 关联。
- 支持 run 状态：
  - pending。
  - planning。
  - running。
  - waiting_approval。
  - blocked。
  - failed。
  - completed。
  - cancelled。
  - rolled_back。
- 支持 artifact 和 report 的最小模型。

### 交付物

- operation/run 设计文档。
- 数据库 migration。
- run service。
- run event API。
- run 列表和详情 API。

### 验收标准

- 一个用户目标可以创建一个 run。
- run 可以记录步骤、工具调用、审批、artifact 和最终报告。
- run 可以恢复展示历史事件。
- UI prototype 可以从真实 API 读取 run timeline。

### 暂不做

- 不做复杂 workflow engine。
- 不做多租户 RBAC。
- 不做完整回滚系统。

## Phase 4: Tool Scheduler and Policy

### 目标

把工具执行从“直接调用工具”升级为受 scheduler 和 policy 管理的执行协议。

### 主要任务

- 设计 central tool scheduler。
- 统一工具生命周期：
  - requested。
  - validated。
  - policy_checked。
  - approval_requested。
  - running。
  - streaming。
  - completed。
  - failed。
  - cancelled。
- 接入 hooks。
- 接入 permission service。
- 增加 mode-aware policy：
  - plan/read-only。
  - default。
  - auto-edit。
  - bypass。
  - headless。
- 为工具增加 capability metadata：
  - read。
  - write。
  - destructive。
  - network。
  - secret。
  - idempotent。
  - retryable。
- 写入 audit/run event。

### 交付物

- tool scheduler 设计文档。
- policy 数据模型。
- scheduler service。
- tool lifecycle event。
- policy/permission API。

### 验收标准

- 所有工具调用通过 scheduler。
- policy 可以 allow/ask/deny。
- permission request 能进入 UI event stream。
- tool 执行过程可被审计。
- headless 模式下 ask 操作有明确行为。

### 暂不做

- 不做完整 RBAC。
- 不做外部审批系统。
- 不做复杂并发 DAG。

## Phase 5: Subagent Tasks

### 目标

将子 agent 从简单辅助工具演进为持久化、可观察、可取消的 task。

### 主要任务

- 扩展现有 `agent` tool。
- 定义 agent task 数据模型。
- 支持 parent/child session。
- 支持 role/name/model。
- 支持 allowed tools / allowed MCP。
- 支持 cwd/worktree/isolation 字段。
- 支持 background run。
- 支持 cancel、resume、summary。
- 防止无界递归和无限并发。

### 交付物

- subagent task 设计文档。
- agent task service。
- agent task API。
- agent task event。
- task 列表和详情。

### 验收标准

- 主 agent 可以创建子 agent task。
- 子 agent 有独立状态和结果。
- 用户可以看到、取消和查看子 agent。
- 子 agent 的工具范围可控。
- 子 agent 结果能汇总回主 run。

### 暂不做

- 不做复杂 agent teams。
- 不做跨机器 agent。
- 不做自动任务市场。

## Phase 6: Wails Desktop Client

### 目标

将 React UI 包装成桌面客户端，并与本地 Crush runtime 连接。

### 主要任务

- 创建 Wails v3 桌面应用。
- 集成 React UI。
- 桌面端启动或连接本地 runtime。
- 支持窗口、菜单、通知、文件选择。
- UI 通过统一 API/Event Stream 通信。
- 保持未来 Web 控制台可复用。

### 交付物

- Wails desktop shell。
- React UI 集成。
- 本地 runtime 连接逻辑。
- 基础桌面构建脚本。

### 验收标准

- 可以启动桌面客户端。
- 可以看到真实 runs/session。
- 可以通过桌面 UI 发起任务。
- 可以接收 runtime 事件。
- Wails 只作为 thin shell，不破坏 API 边界。

### 暂不做

- 不做复杂自动更新。
- 不做完整安装包分发。
- 不做远程多用户登录。

## Phase 7: Sandbox and Workspace Isolation

### 目标

为高风险工具和 agent task 增加隔离能力。

### 主要任务

- 支持 worktree isolation。
- 支持 tool-level env/path/network policy。
- 支持 command risk classification。
- 支持 scoped permission expansion。
- 评估容器或 OS sandbox。
- 为 secret 注入建立边界。

### 交付物

- sandbox/isolation 设计文档。
- worktree isolation。
- tool-level policy enforcement。
- sandbox event/audit。

### 验收标准

- 高风险文件修改可以在 worktree 中执行。
- 工具可以声明需要的 path/network/secret scope。
- 权限提升有明确审批和审计。
- agent task 的隔离方式可见。

### 暂不做

- 不追求 Codex/Gemini 级别完整跨平台 sandbox。
- 不做生产级容器编排。

## Phase 8: Plugin / Capability Package

### 目标

将 MCP、skills、hooks、policy、agent definitions 和工具能力打包为可管理
capability package。

### 主要任务

- 定义 plugin/capability manifest。
- 支持插件声明：
  - tools。
  - MCP servers。
  - skills。
  - hooks。
  - policies。
  - agents。
  - UI metadata。
- 支持启用/禁用。
- 支持来源和版本记录。
- 支持权限声明和审计。

### 交付物

- plugin/capability manifest 设计。
- plugin loader。
- plugin registry API。
- 插件列表 UI。

### 验收标准

- 一个业务插件可以携带 MCP、skill、policy 和 agent 定义。
- 用户可以看到插件提供了哪些能力。
- 插件能力可以被启用/禁用。
- 插件调用可以追溯来源。

### 暂不做

- 不做公开 marketplace。
- 不做自动更新。
- 不做复杂签名体系。

## Phase 9: Enterprise Hardening

### 目标

面向真实企业环境增强可靠性、安全性和治理能力。

### 主要任务

- RBAC / tenant / resource permission。
- 外部审批流。
- 审计导出。
- 远程 runtime。
- 多用户 Web 控制台。
- 插件签名和信任策略。
- 长任务恢复和重试策略。
- eval / regression suite。
- 性能和资源限制。

### 交付物

- 企业权限模型。
- 审计导出。
- 多用户 API。
- 部署文档。
- eval 和回归测试体系。

### 验收标准

- 可以在企业环境中限制用户、资源和操作。
- 高风险操作可走审批。
- 所有关键动作可审计。
- 长任务可恢复。
- 插件和工具来源可追踪。

## 近期执行建议

短期不要大改 Crush。建议顺序是：

1. 做 Phase 0，建立 baseline。
2. 并行做 Phase 1 的 UI mock prototype。
3. 在 baseline 稳定后进入 Phase 2，梳理 runtime API。
4. 只有当 API 和事件流清楚后，再做 operation/run 和 scheduler。

## 关键原则

- 先跑起来，再改造。
- 先稳定边界，再扩展能力。
- 先单 agent workflow，再 agent teams。
- 先本地 runtime，再桌面壳。
- 先 primitives，再 plugin package。
- 先审计和权限，再自动执行高风险操作。

## 风险控制

- 每个阶段都应有明确可运行产物。
- 不在同一阶段同时大改 runtime、UI、插件和 sandbox。
- 保持 TUI 可用，避免破坏原 Crush。
- 参考项目只借鉴设计，不直接照搬代码。
- 复杂企业能力分阶段引入，避免初期过度设计。
