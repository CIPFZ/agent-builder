# Agentic Operations Client 设想

本文记录一个基于 Crush 演进出的产品设想：面向企业产品操作场景，
构建一个以 agent runtime 为核心的对话式客户端。它不是普通聊天工具，
也不是简单的 Claude Code 桌面版，而是一个能通过 agent 调度、插件工具、
权限控制和任务闭环来完成真实业务流程的操作客户端。

## 背景判断

当前大量 AI coding assistant 和 agent 工具都采用 CLI 形态，例如
Claude Code、Codex CLI、OpenCode、Crush 等。CLI 之所以常见，是因为
agent runtime 天然接近终端环境：它需要读写文件、运行命令、调用工具、
感知工作目录、处理 git、展示 diff，并在执行前后做权限确认。

桌面端或可视化客户端相对较少，不只是因为 UI 难，而是因为真正复杂的是
runtime 与系统边界：

- 工具调用与插件管理。
- 多 agent 调度。
- 动态创建 agent。
- 权限与审批。
- 沙箱与执行隔离。
- 长任务恢复。
- 审计与可观测性。
- 凭证与敏感数据管理。

开源桌面项目较少，往往是因为桌面壳只是外层体验，核心仍然需要一个稳定
的本地或远程 agent runtime。没有 runtime 能力，客户端只能是聊天 UI；
有 runtime 能力，客户端才可能成为真实任务的操作台。

## 需求场景

目标场景不是泛用聊天，而是适配公司特定产品，让用户通过自然语言完成
过去需要大量页面点击、命令操作和人工串联的流程。

一个典型例子是集群升级：

1. 用户输入：“帮我把某个集群升级到目标版本。”
2. 系统识别目标集群、当前版本、目标版本和升级约束。
3. agent 组织任务并调用插件连接集群。
4. 调用升级前检查插件，检查版本兼容性、资源状态、告警、备份、节点健康等。
5. 如果发现问题，调用日志收集、场景排障、知识库检索或自动修复插件。
6. 如果问题可修复，则继续闭环处理。
7. 如果存在高风险操作，则向用户说明风险并请求确认。
8. 执行升级、持续观察状态、处理异常。
9. 必要时进入回滚或人工介入流程。
10. 输出最终报告、操作记录、风险项、处理过程和审计链路。

对用户来说，核心体验是只表达目标。系统负责组织流程、调用工具、处理异常、
请求确认并给出报告。

## 产品定位

更准确的产品定位是：

> 面向企业产品操作的 agentic workflow client。用户通过自然语言下发目标，
> 系统通过 agent runtime 调度专业 agent 和插件，完成检查、执行、排障、
> 回滚、报告和审计闭环。

它和通用 coding assistant 的区别在于：

- 面向公司特定产品和运维、交付、排障、变更等真实业务流程。
- 强依赖产品知识、runbook、版本矩阵、错误码、历史案例和组织权限。
- 强调任务闭环，而不仅是回答问题或生成代码。
- 强调可控执行、审批、审计和回滚。
- 客户端承载任务状态、证据展示、权限确认和人工干预。

## Agent First 的含义

Agent first 不等于所有事情都让模型自由发挥。更合理的结构是：

- Agent 负责理解目标、规划步骤、解释结果和调度工具。
- 插件负责执行确定性动作。
- workflow 或 runbook 负责关键流程骨架。
- policy 负责权限、审批、风险分级和执行边界。
- client 负责展示、确认、审计和用户干预。

也就是说，agent first 的核心是以 agent runtime 为中心组织产品能力，
而不是把传统页面简单包上一层聊天框。

## 为什么单 Agent 不够

单 agent 可以完成简单任务，但复杂业务流程通常会遇到限制：

- 上下文过长，难以同时处理规划、执行、排障和汇总。
- 不同子任务需要不同工具、权限和知识边界。
- 长任务需要并行检查、持续观察和失败恢复。
- 高风险操作需要审批和审计，不能完全由一个 agent 自由执行。
- 企业流程往往跨系统、跨团队、跨角色。

因此，行业趋势正在从“单个会聊天和调用工具的 agent”走向
“可调度、可治理、可观测的 agentic workflow”。多 agent 或 agent teams
是其中一种重要形态，但不是目的本身。真正的目标是稳定解决真实问题。

## Runtime 是核心难点

这类系统的护城河主要在 agent runtime，而不是聊天窗口。

### 工具调用系统

工具不是简单函数调用。runtime 需要处理：

- 工具 schema 与输入验证。
- 工具输出压缩与结构化回传。
- 工具失败后的恢复策略。
- 工具是否幂等、是否可取消。
- 工具执行超时、重试与资源限制。
- 工具结果、证据和 artifact 管理。
- 工具权限声明与风险分级。

### 多 Agent 调度

多 agent 难点不是创建多个模型会话，而是调度：

- 谁来拆解任务。
- 哪些任务串行，哪些任务并行。
- 子 agent 获得多少上下文。
- 子 agent 能访问哪些工具和资源。
- 子 agent 失败后如何重试、降级或中止。
- 多 agent 的结果如何汇总和校验。
- 成本、token、时间和并发如何控制。

如果没有稳定调度器，多 agent 很容易变成多个 LLM 互相传话，成本高、
延迟高、错误传播快，而且难以调试。

### 动态创建 Agent

动态创建 agent 很有价值，例如升级 agent 可以创建日志分析 agent、
排障 agent 或报告 agent。但 runtime 必须定义清楚：

- agent 的角色和任务边界。
- 可访问的工具集合。
- 可访问的文件、集群、资源和凭证范围。
- 生命周期和完成条件。
- 输出格式。
- 超时、失败和取消策略。
- 父子关系、权限继承和审计链路。

### 权限与审批

企业产品场景中，权限系统是生命线。它至少需要表达：

- 用户身份、组织、项目和集群权限。
- 工具级权限。
- 操作风险等级。
- 只读、dry-run、低风险变更、高风险变更等执行级别。
- 是否需要用户确认。
- 是否需要审批流。
- 是否允许自动执行。
- 是否允许读取敏感日志、密钥或凭证。

agent 越强，权限系统越重要。

### 沙箱与隔离

当 agent 可以运行命令、访问文件、连接集群或调用外部系统时，需要考虑：

- 文件系统隔离。
- 网络访问控制。
- 环境变量和 secret 注入。
- 插件进程隔离。
- 命令白名单或黑名单。
- 容器、虚拟机或 worktree 隔离。
- 超时和资源限制。
- 执行日志和回滚能力。

### 状态、恢复与审计

真实任务经常不是一次完成的。升级、巡检、迁移和排障可能持续很久，
中途还可能失败、断网、重启或需要用户介入。因此 runtime 需要：

- event log。
- task state machine。
- checkpoint。
- resumable session。
- tool call history。
- artifact 管理。
- long-running job 管理。
- 最终报告与审计记录。

## 客户端的价值

客户端不是简单 UI，而是 agent runtime 的控制平面。它应该让用户看到并控制：

- 当前任务执行到哪一步。
- 哪个 agent 正在工作。
- 调用了哪些插件。
- 每一步的输入、输出、证据和风险。
- 哪些操作需要确认或审批。
- 哪些问题已经自动修复。
- 哪些步骤失败、是否重试。
- 是否需要回滚或人工介入。
- 最终报告和审计链路。

CLI 适合工程师即时操作，但企业产品用户、运维、交付、售后和客户成功等
角色通常更需要可视化任务流、权限确认、证据展示和审计回放。

## 插件模型

插件不应该只被理解成简单工具。更完整的插件能力可以包括：

- tools：agent 可调用的动作，例如查询集群、执行检查、升级、回滚。
- resources：可注入上下文，例如产品文档、schema、版本矩阵。
- skills：包含指令、脚本、模板和工作流经验的能力包。
- hooks：在工具执行前后进行拦截、检查或增强。
- UI extensions：为客户端提供配置页、结果面板或可视化组件。
- background services：长期运行的 MCP server、索引器、watcher。

早期可以统一抽象为 capability，其中 tool 是第一等能力；后续再扩展
resource、skill、hook 和 UI extension。

## 建议演进路线

不要一开始就追求完整的 Claude Code 能力或复杂 agent teams。更现实的
路线是先跑通一个高价值闭环场景。

第一阶段：

1. 选择一个 MVP 场景，例如集群升级闭环。
2. 做稳定的 tool runtime。
3. 做权限确认和审批边界。
4. 做任务 event log 和状态恢复。
5. 用单 agent 串起完整 workflow。
6. 客户端展示任务状态、工具调用、风险项、确认点和最终报告。

第二阶段：

1. 引入专业 agent，例如巡检 agent、日志分析 agent、排障 agent、报告 agent。
2. 增加并行检查和子任务调度。
3. 为子 agent 设置工具范围、上下文范围和权限边界。
4. 增强失败恢复、回滚和人工接管。

第三阶段：

1. 支持动态创建 agent。
2. 支持插件市场或组织内插件分发。
3. 支持跨系统 agent 协作。
4. 支持更完整的审计、成本、质量和安全治理。

## 与 Crush 的关系

Crush 是一个合适的技术起点，因为它已经具备 Go 实现的 agent 基础设施：

- provider 抽象。
- session 管理。
- tools。
- MCP 集成。
- skills。
- hooks。
- SQLite 持久化。
- pub/sub。
- TUI。

可能的演进方向不是推倒重来，而是将 Crush 向 headless runtime 和多客户端
架构演进：

```text
Go Agent Runtime
  - session / message / event log
  - model provider abstraction
  - tool registry
  - permission system
  - scheduler / subagents
  - workspace / sandbox
  - MCP / skills / hooks

Local API Layer
  - HTTP / gRPC / WebSocket / SSE
  - desktop client connects here
  - CLI / TUI client can also connect here

Clients
  - CLI / TUI
  - desktop app
  - web console

Plugin System
  - built-in tools
  - MCP servers
  - local executable plugins
  - skills / prompts / resources
```

## 核心结论

这个方向是成立的，但难点不在桌面 UI，而在 runtime。

需求上，企业产品中的升级、巡检、排障、交付和变更流程适合用对话式
agent 客户端承接。产品形态上，它应该是 agent runtime 驱动的操作客户端，
而不是传统页面加聊天框。技术上，真正需要优先攻克的是工具调用、多 agent
调度、动态 agent、权限、沙箱、长任务恢复、审计和上下文管理。

多 agent 不是目标本身。目标是稳定解决真实任务。早期应该先把单 agent、
工具、权限和 workflow 闭环做扎实，再逐步演进到 agent teams。

## 初步技术选型

当前先不从零实现完整链路，而是基于优秀开源项目快速落地，再逐步补齐
企业产品操作场景所需的能力。

### 主开发底座：Crush

Crush 作为主开发底座，原因是：

- Go 实现，适合做稳定、本地优先、低环境依赖的 runtime。
- 已经具备 tools、MCP、skills、hooks、session、SQLite、pub/sub、TUI。
- 当前代码已在本地，可直接改造和验证。
- 更符合后续演进为 headless runtime、多客户端和企业插件体系的目标。

Crush 当前更偏 terminal coding agent，还不是 enterprise operations runtime。
后续需要重点补齐 headless API、任务状态机、权限分级、审批、沙箱、多
agent 调度、动态 agent 和客户端控制平面。

### 一级参考蓝图：Claude Code

Claude Code 作为核心设计蓝图，重点参考：

- tool system。
- permission context。
- plan mode。
- AgentTool / subagent。
- background task。
- worktree / remote isolation。
- session recovery。
- hooks / skills / MCP。
- runtime lifecycle。

Claude Code 的价值主要在设计层面。它展示了成熟 agentic CLI 如何把工具、
权限、上下文、UI、任务和恢复语义组织成完整 runtime。后续应学习其架构
思想，而不是直接迁移源码。

### 二级参考：Codex

Codex 作为本地 agent 工程边界参考，重点关注：

- Rust 本地 agent runtime 的组织方式。
- sandbox 和执行安全边界。
- CLI 交互和 approval 体验。
- model/tool loop。
- OpenAI provider 接入。
- 本地执行约束和可恢复性。

Codex 适合作为工程质量、安全边界和本地执行模型的参考。

### 二级参考：Gemini CLI

Gemini CLI 作为通用 CLI agent 和工具生态参考，重点关注：

- MCP 集成。
- tool registry。
- 配置体验。
- 大上下文处理。
- Web fetch、搜索和外部信息工具设计。
- provider 生态和命令行体验。

Gemini CLI 适合参考通用工具生态和大上下文场景，但不作为主底座。

### 保留观察：OpenCode、Goose、Cline、OpenHands、Aider

除上述项目外，还应持续观察其他优秀实现：

- OpenCode：client/server、agent/subagent、权限和 desktop beta。
- Goose：desktop、CLI、API、多 provider 和 MCP 扩展生态。
- Cline：multi-agent teams、SDK、任务板、scheduled agents。
- OpenHands：企业化 Web/REST/云端形态、权限和多用户能力。
- Aider：repo map、git、diff 和代码修改体验。

这些项目不作为当前主底座，但可在具体能力设计时作为补充参考。

## 参考项目分析策略

对 Crush、Claude Code、Codex、Gemini CLI 做全量分析是有必要的，但不应
理解为逐文件阅读和复刻。更合理的方式是分层全量分析：先建立整体地图，
再围绕 runtime 核心能力深入。

### 为什么需要全量分析

需要全量分析的原因：

- 避免只看到 UI 或工具表面，漏掉真正的 runtime 边界。
- 找出各项目在工具、权限、任务、上下文、沙箱和恢复上的成熟做法。
- 判断哪些能力可以直接在 Crush 上补齐，哪些需要重构。
- 形成后续实现路线图，避免边做边推翻。
- 为企业产品操作场景建立可对照的架构依据。

### 为什么不能逐文件硬读

参考项目体量较大，逐文件硬读成本高、收益低。更有效的是按能力面建立
索引和判断：

- 启动与主循环。
- model/provider 抽象。
- message/session/event log。
- tool registry 和 tool execution protocol。
- permission / approval / policy。
- MCP / plugin / skill。
- subagent / background task / scheduler。
- sandbox / worktree / process isolation。
- context loading / compression / memory。
- CLI/TUI/API/desktop client 边界。
- observability / tracing / telemetry。
- tests / evals / harness。

### 建议产出

每个参考项目至少产出一份分析文档：

- 项目定位。
- 技术栈。
- 架构分层。
- 核心 runtime 流程。
- 工具系统。
- 权限与安全。
- 多 agent / task 能力。
- 插件与扩展。
- 可借鉴设计。
- 不适合作为底座的原因。
- 对 Crush 改造的启发。

最后再产出一份横向对比文档，用于决定 Crush 的改造优先级。
