# myclaw

`myclaw` 是一个用 Go 实现的 `agent builder` 项目。

它的目标不是再做一个通用聊天 agent，而是提供两层能力：

1. 一个稳定、可验证、可扩展的 Claude Code 风格 `agent runtime`
2. 一个面向具体任务的 `agent builder`，用来快速构建任务专用 agent

当前阶段，底层 `agent runtime` 已经完成第一轮核心能力复刻和稳定性修复；后续工作的重点会逐步转向 `agent spec`、任务专用 agent 的生成、调试和运行。

## 当前目标

`myclaw` 的真实目标是帮助用户回答下面这个问题：

**“我有一个具体任务，应该怎样快速构建一个真正可用的专用 agent？”**

例如：
- 服务器排障 agent
- 代码审查 agent
- 需求分析 agent
- 日报生成 agent
- 发布巡检 agent

这类 agent 的难点，不只是“接一个 LLM”，而是：
- `system prompt` 怎么定义
- 需要哪些 `tools`
- 哪些操作必须审批
- 需要记住哪些上下文
- 任务流程怎么设计
- 怎样验证它真的能工作

`myclaw` 之后要做的，就是把这些东西结构化，变成一套可以生成、运行、测试的 agent 构建体系。

## 当前状态

当前仓库已经不是骨架，而是一个可运行的最小 `agent runtime`：

- 真实 LLM 接入
- Claude Code 风格 `QueryEngine`
- tool loop
- permission / approval
- compaction / memory
- subagent / `agent.task`
- TUI 验证入口

这一轮手工与自动回归已经验证通过的核心链路：

- 基础对话
- tool 调用
- approval 申请 / 批准 / 继续执行
- `agent.task`
- compact 生命周期触发与展示

所以当前仓库的定位可以明确成：

**runtime 内核已基本验通，接下来开始进入 agent builder 本体设计。**

## 项目分层

建议从两层来理解这个项目：

### 1. Agent Runtime

这是所有 agent 最终运行时依赖的底层内核，主要负责：

- 会话主循环
- prompt/context 组装
- tools 调用与回流
- approval / permissions
- compaction / memory
- subagent
- TUI / runtime 调试体验

### 2. Agent Builder

这是项目真正的产品层，后续要重点完成：

- `agent spec`
- 任务到 agent 的结构化定义
- prompt / tools / skills / scripts 推荐
- 任务专用 agent 的生成、调试与运行
- 针对 agent 的测试与验收

## 建议目录

```text
myclaw/
├── cmd/
│   ├── myclaw/                # CLI / TUI 入口
│   └── myclawd/               # daemon / gateway 入口
├── configs/                   # 本地配置与示例配置
├── docs/                      # 计划、结构、设计文档
├── internal/
│   ├── agent/                 # subagent 生命周期
│   ├── app/                   # CLI / daemon 装配
│   ├── approval/              # 审批记录与状态流转
│   ├── compaction/            # 上下文压缩
│   ├── config/                # 配置加载
│   ├── diagnostics/           # 结构化诊断日志
│   ├── gateway/               # WebSocket / HTTP 控制面
│   ├── llm/                   # LLM 适配层
│   ├── memory/                # session memory
│   ├── orchestration/         # 编排与控制面
│   ├── permissions/           # 权限与 approval 策略
│   ├── prompt/                # prompt/context 组装
│   ├── queryengine/           # Claude Code 风格主循环核心
│   ├── runtime/               # runtime 装配
│   ├── sandbox/               # 执行路由 / 沙箱
│   ├── session/               # session / transcript
│   ├── store/                 # 持久化抽象
│   ├── tools/                 # tool registry / agent.task / tool.search
│   ├── tui/                   # TUI 验证壳
│   └── workspace/             # 工作区上下文加载
├── scripts/
└── testdata/
```

## 当前可运行入口

```powershell
go run ./cmd/myclaw version
go run ./cmd/myclaw tui
go run ./cmd/myclawd
```

`myclaw tui` 当前是最小验证入口，用来确认 runtime 主链是否正常工作。

`myclawd` 当前提供最小 daemon / gateway 能力，包括：

- `/`
- `/healthz`
- `/statusz`
- `/ws`

## 配置

默认读取：

`configs/myclaw.json`

示例配置：

`configs/myclaw.example.json`

最小示例：

```json
{
  "llm": {
    "provider": "openai-compatible",
    "base_url": "https://api.longcat.chat/openai/v1/chat/completions",
    "api_key": "${MYCLAW_LLM_API_KEY}",
    "model": "LongCat-Flash-Chat"
  },
  "permissions": {
    "mode": "workspace-write",
    "subagent_mode": "ask",
    "plan_mode": false,
    "auto_mode": false,
    "workspace_roots": [
      "."
    ]
  },
  "compact": {
    "verification_mode": false
  }
}
```

说明：

- `myclaw` 和 `myclawd` 都会读取 `configs/myclaw.json`
- `${ENV_VAR}` 会自动展开
- 环境变量可以覆盖配置文件
- `compact.verification_mode` 用于低阈值 compact 验证

## 真实 LLM 接入

如果没有设置 `MYCLAW_LLM_API_KEY`，系统会回退到内置 mock client。

使用真实模型时可直接这样启动：

```powershell
$env:MYCLAW_LLM_API_KEY="your_key_here"
go run ./cmd/myclaw tui
```

## 当前里程碑

当前已经完成的里程碑：

- Claude Code 风格 runtime 核心链路初步成型
- runtime 核心能力完成一轮稳定性修复
- TUI 可用于 runtime 手工验收
- compact / approval / tool / subagent 主链已验证

接下来的下一阶段目标：

**开始定义 `agent spec`，正式进入 agent builder。**

## 下一步

下一步工作会从“继续扩 runtime”切换到“定义任务专用 agent 的构建模型”，重点包括：

- 一个任务专用 agent 应该有哪些字段
- `agent spec` 的结构
- 如何从任务描述生成 spec
- 如何根据 spec 装配 runtime
- 如何验证一个 agent 是否真的可用

---

如果你现在要继续往前推进，最自然的下一步就是：

**先拿一个真实案例，定义第一版 `agent spec`。**
