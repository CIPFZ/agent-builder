# Client UI Plan

本文记录 agentic operations client 的客户端与 UI 技术选型。核心原则是：
Crush/Go runtime 是执行核心，客户端只是控制平面和体验层。UI 不应该把
runtime 绑死在某一个桌面框架上。

## 目标

目标客户端不是普通聊天窗口，而是企业产品操作场景的 agent control plane。
它需要让用户通过自然语言下发目标，同时能观察、确认和审计 agent runtime
正在执行的任务。

客户端需要支持：

- 对话式任务入口。
- agent / subagent 状态展示。
- 工具和插件调用过程展示。
- 任务时间线。
- 权限确认和高风险操作审批。
- 风险项、检查项和处理建议。
- 日志、artifact、diff、报告展示。
- 长任务进度、失败、重试、回滚和人工接管。
- 后续扩展为桌面端、Web 控制台或嵌入式管理界面。

## 初步技术选型

```text
Go / Crush Runtime
  -> Local API / Event Stream
  -> Wails Desktop Shell
  -> React + Ant Design + Ant Design X
```

### Runtime

Runtime 继续基于 Crush/Go 演进。它负责：

- session / operation / run。
- agent loop。
- tool scheduler。
- permission / policy。
- plugin / MCP / skills / hooks。
- sandbox / workspace isolation。
- event log / audit / artifact。

Runtime 应通过稳定 API 暴露能力，而不是只通过 TUI 或 Wails binding 暴露。

### Desktop Shell

桌面壳优先考虑 Wails v3。

Wails 的价值是：

- 与 Go runtime 适配自然。
- 可以使用 React/Vite 等前端栈。
- 提供窗口、菜单、托盘、通知、文件选择等桌面能力。
- 可以让 Go 和前端在同一个桌面应用中分发。

但 Wails 不应该成为 runtime 架构中心。它应该是 thin shell：

- 启动或嵌入本地 Crush runtime。
- 提供必要的系统集成能力。
- 承载 React UI。
- 通过统一 API/Event Stream 与 runtime 通信。

不建议让 React UI 大量直接依赖 Wails Go bindings。否则未来 Web 版、
远程控制台和测试 harness 都会难以复用。

### Frontend

前端建议使用：

- React。
- TypeScript。
- Vite。
- Ant Design。
- Ant Design X。
- TanStack Query。
- Zustand 或 Jotai。
- EventSource 或 WebSocket。

Ant Design 负责企业操作台能力：

- layout。
- table。
- form。
- drawer。
- modal。
- tabs。
- steps。
- timeline。
- descriptions。
- alert。
- tree。

Ant Design X 负责 AI/agent 交互能力：

- conversation。
- bubble。
- sender。
- thought chain。
- attachments。
- prompts。
- welcome。
- suggestion。
- markdown / code block。
- sources / citations。

后续可按需引入：

- Monaco Editor：查看和编辑配置、diff、脚本、YAML。
- xterm.js：展示命令执行或交互式终端。
- Mermaid：展示任务流程、依赖和状态机。
- AntV：展示资源拓扑、任务关系或指标趋势。

## 推荐架构

推荐保持三层边界：

```text
React Client
  - UI state
  - operation dashboard
  - chat and agent timeline
  - approval UX
  - reports and artifacts

Client Transport
  - HTTP / JSON-RPC for commands
  - SSE / WebSocket for runtime events
  - same contract for Wails desktop and future Web console

Go Runtime
  - Crush agent runtime
  - session / run / operation
  - tool scheduler
  - policy / permission
  - plugin / MCP / skill
  - audit / event log
```

这样做的好处：

- Wails 桌面端和 Web 控制台可以复用同一套 React 和 API。
- TUI、desktop、headless CLI、automation 都可以消费同一 runtime。
- 后续如果 Wails v3 API 或产品形态变化，runtime 和 UI 不会被重写。
- 企业部署时可以选择本地桌面、远程 runtime 或 Web 管理台。

## 不推荐的架构

不推荐：

```text
React UI -> Wails Go bindings -> 内部 runtime 函数
```

这种方式早期写起来快，但会导致：

- UI 和 runtime 强耦合。
- 不能自然支持 Web 控制台。
- 测试和自动化困难。
- 事件流、权限、长任务状态很难形成稳定协议。
- 后续多客户端和远程运行改造成本高。

## UI 信息架构

客户端首屏不应该只是一个聊天框。建议采用操作台布局：

```text
┌───────────────────────────────────────────────┐
│ Top Bar: workspace / model / connection / user │
├───────────────┬───────────────────────────────┤
│ Sessions/Runs │ Main Agent Conversation        │
│ Agents        │ + tool calls + thought chain   │
│ Plugins       │ + approval prompts             │
├───────────────┼───────────────────────────────┤
│ Run Timeline  │ Detail Panel                   │
│ Steps         │ logs / risks / artifacts       │
│ Status        │ diff / report / resource view  │
└───────────────┴───────────────────────────────┘
```

核心视图：

- Runs：任务列表和状态。
- Conversation：用户与主 agent 对话。
- Timeline：任务阶段、工具调用、审批和结果。
- Agent Panel：当前 agent 和 subagent 状态。
- Plugin Panel：已启用插件、MCP server、工具能力和权限。
- Approval Center：待确认/待审批操作。
- Artifact Viewer：日志、报告、diff、检查结果、图表。
- Settings：provider、model、workspace、policy、插件配置。

## Agent 交互组件

对话区需要展示的不只是消息：

- 用户目标。
- agent 计划。
- tool call started / completed。
- tool input summary。
- tool output summary。
- permission request。
- policy decision。
- subagent started / completed。
- risk detected。
- checkpoint。
- rollback recommendation。
- final report。

这些都应该来自 runtime event stream，而不是 UI 自己猜测。

## 任务与审批 UI

企业操作场景中，审批体验是核心：

- 高风险操作必须明确展示影响范围。
- 展示 dry-run 结果。
- 展示将调用的工具和目标资源。
- 展示回滚方案。
- 支持 allow once / deny / always allow / request approval。
- 支持非交互模式下自动 deny 或进入等待审批。
- 所有审批结果写入 audit log。

UI 上可以使用 Ant Design 的 Modal、Drawer、Descriptions、Alert、Steps
和 Timeline 组合实现。

## MVP UI

第一阶段先做 mock UI，不直接改 Crush runtime：

1. 创建 React + Vite 前端原型。
2. 使用 Ant Design 和 Ant Design X 搭建主界面。
3. 使用 mock event stream 模拟 agent 执行。
4. 实现一个“SSH 排障助手”演示流程：
   - 用户描述故障现象或选择排障 SOP。
   - agent 生成排障计划。
   - 通过 mock SSH tool 执行 SOP 中的只读命令。
   - 展示系统信息、服务状态、日志片段和指标证据。
   - 通过 mock MCP 搜索相关知识和故障模式。
   - 展示排障判断、可能原因和下一步建议。
   - 对潜在高风险修复动作只展示确认提示，不自动执行。
5. 与 Crush runtime 先保持解耦。

这个阶段的目标是验证信息架构和交互，而不是实现真实操作。

## 后续演进路线

### 阶段 0：Crush Baseline

- 保持 Crush 原状。
- 跑通 build / test / run。
- 记录当前行为和问题。
- 不做大规模 runtime 改造。

### 阶段 1：UI Prototype

- React + Ant Design + Ant Design X。
- mock runtime event stream。
- 做出 agent operations client 的主界面。
- 验证“SSH 排障”这类端到端任务展示是否合理。
- 支持 SOP fixture selector。
- 支持 SSH target config mock。
- 支持审批交互和 raw event log。
- 支持验收阶段的 DeepSeek 报告生成。

### 阶段 1 验收版

Phase 1 结束时需要形成一个可供本地验收的客户端版本：

- 客户端可以本地 build、preview，并通过 Wails v3 打成桌面可执行文件。
- 首屏必须是简单的聊天入口，而不是完整运维工作台。
- 可以配置 LLM 连接信息：协议、URL、API key 和高级代理。
- 可以通过真实 DeepSeek API 进行基础对话。
- Operations/SSH/SOP 能力作为二级入口保留，不在首屏展开。
- 不执行真实 SSH 高风险操作。

DeepSeek API 在 Phase 1 可用于基础聊天和报告总结，但不让模型自由决定工具
调用，也不让模型执行真实命令。后续接入 Crush runtime 后，模型和工具调用
都应通过 Go runtime 的 provider、tool scheduler 和 permission/policy 层。

Phase 1.6 先提供一个桌面验收壳：

- 桌面工程位于 `desktop/agent-builder`。
- Wails 只负责窗口、资源嵌入和本地打包。
- React UI 仍然以 `client` 为唯一来源。
- 构建时先运行 `client` 的生产构建，再将 `client/dist` 同步到
  `desktop/agent-builder/frontend/dist`。
- Windows 验收可执行文件输出到
  `desktop/agent-builder/bin/AgentBuilder.exe`。

### 阶段 2：Runtime API

- 在 Crush 中稳定本地 API。
- 定义 session / turn / tool / permission / event 协议。
- UI 从 mock event stream 切换到真实 API。

### 阶段 3：Wails Desktop

- 用 Wails v3 包装 React UI。
- Wails 负责桌面窗口和系统集成。
- Runtime 继续通过统一 API 通信。

### 阶段 4：Operation Runtime

- 新增 operation/run 模型。
- 新增 tool scheduler。
- 新增 policy/approval。
- 新增 audit/event/artifact。

### 阶段 5：Enterprise Capabilities

- 插件包。
- subagent teams。
- sandbox/worktree isolation。
- RBAC / approval workflow。
- Web 控制台和远程 runtime。

## 结论

React + Ant Design + Ant Design X 适合构建 agentic operations client 的
前端体验。Wails v3 适合作为 Go runtime 的桌面壳，但不应该成为核心架构
依赖。正确边界是：

```text
Crush/Go runtime 是核心
React UI 是客户端
Wails 是桌面壳
HTTP/JSON-RPC + SSE/WebSocket 是长期协议
```

先做 UI prototype 和 Crush baseline，再逐步连接真实 runtime，是当前风险
最低、演进空间最大的路线。
