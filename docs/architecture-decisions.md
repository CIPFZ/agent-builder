# Architecture Decisions

本文记录当前已经明确的关键架构与产品决策，作为后续实现时的基线。

Status update, 2026-05-24:

- ADR-004 through ADR-008 remain useful architecture decisions.
- ADR-001 through ADR-003 are historical startup decisions. The current product
  path is no longer an SSH troubleshooting mock MVP or pre-refactor baseline
  exercise.
- Current runtime priorities are maintained in
  `docs/claude-code-alignment-next-roadmap.md`.

## Historical ADR-001: MVP 选择 SSH 排障助手

### 决策

第一版 MVP 不做集群升级。集群升级只是长期目标场景示例，第一版收敛为
“SSH 排障助手”：

1. 用户描述问题或选择一个排障 SOP/skill。
2. 客户端通过受控 SSH 工具连接远程机器或集群节点。
3. agent 按 skill 中定义的 SOP 步骤执行只读或低风险命令。
4. 工具收集系统信息、日志片段、服务状态、资源指标等证据。
5. agent 通过 MCP 或知识搜索工具检索相关文档、经验和故障模式。
6. agent 汇总证据，给出排障判断、可能原因和下一步建议。
7. 高风险修复动作不自动执行，只展示建议或请求确认。

### 原因

这个 MVP 比“集群升级”更适合作为第一阶段：

- 流程更短，容易闭环。
- 主要是读信息、查知识和给建议，风险更低。
- 可以验证 SSH tool、skill/SOP、MCP 搜索、证据汇总、报告生成这些核心能力。
- 不需要一开始实现复杂升级、回滚、审批和长事务。
- 非常贴近企业产品运维和售后场景。

### 第一版范围

第一版只要求：

- 能配置一个 SSH 目标。
- 能选择或触发一个排障 skill。
- 能按 SOP 顺序执行命令。
- 能收集命令输出。
- 能通过 MCP 或知识搜索工具查找相关资料。
- 能给出排障建议和报告。
- 能展示工具调用和证据链。

### 暂不做

- 不做自动修复。
- 不做集群升级。
- 不做回滚。
- 不做复杂审批流。
- 不做多 agent teams。
- 不做生产级 sandbox。

## Historical ADR-002: 前端目录放在本仓库

### 决策

前端原型放在本仓库中，建议目录名为：

```text
client/
```

如果后续需要区分多个客户端，可继续扩展为：

```text
client/web/
client/desktop/
client/shared/
```

第一阶段先使用单一 `client/` 目录即可。

### 原因

- 当前目标是和 Agent Builder runtime 一起演进，不需要拆仓库。
- 方便共享 docs、mock event schema 和后续本地 API。
- 方便 Wails desktop shell 后续复用同一套 React UI。
- 目录名 `client` 比 `frontend` 更贴近最终多客户端形态。

## Historical ADR-003: 先做本机 Agent Builder Baseline

### 决策

进入实现前，先在本机实际构建和测试当前 Agent Builder：

- `go build .`
- `go run . --help`
- `go test ./...` 或先运行关键包测试。
- 如条件允许，尝试非交互运行。

结果记录到：

```text
docs/archive/dev-baseline.md
```

### 原因

后续改造前必须知道原始项目在当前机器上的状态。否则后续出现失败时，
无法判断是环境问题、原项目问题，还是我们改造引入的问题。

## ADR-004: Runtime API 协议选择

### 备选方案

#### HTTP + SSE

请求使用 HTTP/REST 或轻量 JSON API，事件流使用 SSE。

优点：

- 实现简单。
- 浏览器和桌面端都原生支持 EventSource。
- 很适合单向 runtime event stream。
- 方便调试和 curl。
- 适合第一阶段 session/run/tool/permission 事件。

缺点：

- SSE 是单向流，客户端到 runtime 的控制消息仍需要 HTTP 请求。
- 对双向实时协作、远程控制、多客户端复杂同步不如 WebSocket。
- JSON-RPC 语义需要自己补。

#### JSON-RPC + WebSocket

请求与事件都通过 WebSocket 上的 JSON-RPC 或类似协议承载。

优点：

- 双向通信自然。
- 更适合远程 runtime、多客户端协作、实时控制。
- 方法调用、错误码、请求 id、响应 id 更规范。
- Codex app-server 的方向可作为参考。

缺点：

- 第一阶段实现和调试成本更高。
- 连接管理、重连、心跳、并发请求和错误处理更复杂。
- 对简单本地客户端可能过重。

### 决策

第一阶段采用：

```text
HTTP JSON API + SSE event stream
```

后续在 runtime API 稳定后，再评估是否升级或补充：

```text
JSON-RPC over WebSocket
```

### 原因

当前最重要的是快速打通本地客户端和 Agent Builder runtime。HTTP + SSE 足够表达：

- 创建 session/run。
- 发送用户输入。
- 获取历史消息。
- 接收 assistant/tool/permission/run events。
- 提交 approval decision。
- 取消任务。

等到出现远程 runtime、多客户端协作、复杂双向控制或 SDK 需求时，再引入
JSON-RPC/WebSocket 更合理。

## ADR-005: Plugin 定义为 Tool Capability 组合

### 决策

当前对 plugin 的定义是：plugin 本质上是可被 agent 使用的 tool capability
组合。

plugin 可以由以下能力组成：

- skill + scripts。
- executable tool package。
- MCP server + scripts。
- MCP tools + skills。
- hooks + policy。
- agent definition + allowed tools。
- 其他能被 runtime 注册成 tool/capability 的组合。

### 设计原则

- agent 最终调用的是 tool/capability。
- skill 负责告诉 agent 何时、如何使用能力。
- script 或 executable package 负责执行确定性动作。
- MCP 负责暴露标准化外部工具和资源。
- hooks/policy 负责约束和增强执行过程。

### 第一版范围

第一版 plugin 不做 marketplace。先支持：

- 本地 skill。
- 本地脚本工具。
- MCP server。
- 简单 manifest。
- 工具来源和权限声明。

## ADR-006: 权限模式参考 Claude Code

### 决策

权限需要支持不同策略模式，参考 Claude Code / Gemini CLI / Codex 的思路，
但第一版保持简单。

建议第一批模式：

- `plan`：只读规划模式，只允许读信息和生成计划。
- `default`：默认模式，高风险操作需要确认。
- `accept_edits`：允许低风险写入或修改，但高风险仍需确认。
- `bypass_permissions`：显式绕过权限确认，仅限受信环境和高级用户。
- `headless`：非交互模式，遇到 ask 默认 deny 或进入等待审批。

### 策略维度

后续 policy rule 至少需要表达：

- tool。
- agent。
- MCP server。
- command prefix / regex。
- args pattern。
- resource/path。
- risk level。
- interactive/headless。
- decision：allow / ask / deny。

### 原因

高风险操作不能只有一个全局开关。用户需要在不同场景下选择不同策略：

- 只想让 agent 分析和规划。
- 允许 agent 做低风险动作。
- 临时允许某类工具。
- 在自动化环境中禁止交互确认。
- 在受信环境中减少确认噪音。

## ADR-007: Web 控制台作为预留方向

### 决策

第一阶段不实现 Web 控制台，但架构必须预留。

### 后续有价值的使用场景

预留 Web 控制台是有价值的，原因包括：

- 团队共享任务状态：多人查看同一个 run、报告和证据链。
- 运维大屏或值班台：集中查看多个 agent run 和告警排障进度。
- 远程 runtime：runtime 部署在内网服务器，用户通过浏览器访问。
- 审批中心：负责人在 Web 上审批高风险操作。
- 插件管理：管理员统一启用/禁用插件、MCP server、policy。
- 审计查询：安全或运维团队检索历史操作和工具调用。
- 售后协作：支持工程师和客户侧人员共享排障报告。

### 架构要求

为了预留 Web，React UI 不能强依赖 Wails binding。必须通过统一 runtime API：

```text
HTTP JSON API + SSE/WebSocket event stream
```

Wails 只是桌面壳，不是唯一客户端协议。

## ADR-008: 数据模型命名

### 决策

数据模型命名参考 Agent Builder、Codex、Claude Code 和 Gemini CLI，但以 Agent Builder
现有概念为主，减少认知跳跃。

建议核心命名：

| 名称 | 含义 |
| --- | --- |
| `Session` | 一段对话上下文，沿用 Agent Builder 现有概念。 |
| `Turn` | 用户输入到 agent 完成一次响应的执行轮次。 |
| `Message` | 用户、assistant、tool result 等消息。 |
| `ToolCall` | 一次工具调用请求和结果。 |
| `PermissionRequest` | 工具执行前的确认/审批请求。 |
| `Run` | 面向业务目标的一次任务执行，例如一次 SSH 排障。 |
| `RunStep` | run 中的阶段或步骤，例如收集系统信息、查询日志、搜索知识库。 |
| `RunEvent` | run 的不可变事件流，用于审计和 UI 时间线。 |
| `Artifact` | 工具输出产生的文件、日志、报告、diff、截图等证据。 |
| `AgentTask` | 子 agent 或后台 agent 的持久任务。 |
| `Capability` | tool、skill、MCP、script 等可供 agent 使用的能力抽象。 |
| `Plugin` | capability 的组合包。 |
| `PolicyRule` | allow/ask/deny 等权限策略规则。 |

### 关系

```text
Run
  -> RunStep
  -> RunEvent
  -> Session
      -> Turn
      -> Message
      -> ToolCall
      -> PermissionRequest
  -> Artifact
  -> AgentTask
```

### 原因

- `Session`、`Message` 与 Agent Builder 现有模型一致。
- `Turn` 参考 Codex，更适合表达一次 agent loop。
- `Run` 作为业务任务主线，比直接使用 session 更适合企业操作场景。
- `AgentTask` 避免过早引入复杂 agent teams。
- `Capability` 可以统一 plugin、tool、skill、MCP、script 的抽象。

## 当前结论

Historical startup conclusion:

1. 做 Agent Builder baseline，形成 `docs/archive/dev-baseline.md`。
2. 创建 `client/`，做 SSH 排障助手的 mock UI。
3. 用 HTTP JSON API + SSE 作为第一版 runtime/client 协议。
4. 第一版 plugin/capability 以 skill + script + MCP 为主。
5. 权限先实现 plan/default/accept_edits/bypass_permissions/headless 的概念。
6. Web 控制台不做，但协议和 UI 架构要能复用。

Current conclusion:

1. Keep Go runtime as source of truth and React as presentation/client state.
2. Do not restore CLI/TUI as the product main path.
3. Keep provider/model/tool protocol ownership in `charm.land/fantasy`.
4. Implement compact lifecycle foundation next.
5. Then harden tool search/budget, scoped policy, AgentTask communication, and
   scenario/eval coverage.
