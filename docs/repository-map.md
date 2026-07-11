# 目录索引

## 产品入口

| 路径 | 职责 |
|---|---|
| `main.go` | CLI/服务入口，调用 `internal/cmd` |
| `desktop/main.go` | Wails 桌面入口 |
| `desktop/runtime_bridge.go` | Wails 到 RuntimeService 的桥接 |
| `client/src/main.tsx` | React 入口 |
| `client/src/app/App.tsx` | 客户端应用根组件 |

## Go 包

| 路径 | 职责 |
|---|---|
| `internal/runtime` | 客户端运行时服务、DTO、HTTP/SSE、投影和持久化协调 |
| `internal/runtimeapi` | HTTP endpoint 与事件契约 |
| `internal/app` | 应用依赖装配 |
| `internal/workbench` | Agent、会话、权限等工作台服务组合 |
| `internal/agent` | Agent 循环、Provider、Prompt、压缩、工具处理 |
| `internal/agent/tools` | 内置文件、Shell、网络、MCP、诊断等工具 |
| `internal/tools/scheduler` | ToolCall 状态机、存储接口与调度 |
| `internal/permission` | 权限请求与 policy 规则 |
| `internal/session` | 会话领域模型 |
| `internal/message` | 消息与 content block 模型 |
| `internal/db` | SQLite、schema、queries、migrations |
| `internal/config` | 配置加载、合并、解析和校验 |
| `internal/projects` | 项目模型与管理 |
| `internal/memory` | 项目记忆扫描、索引和检索 |
| `internal/skills` | Skill 目录、加载、跟踪和诊断 |
| `internal/hooks` | Hook 定义和执行 |
| `internal/lsp` | LSP client、manager 和 edit 工具 |
| `internal/workspace` | 工作区抽象 |
| `internal/filetracker` | 文件读取/变化跟踪 |
| `internal/pubsub`、`internal/event` | 进程内事件基础设施和事件模型 |
| `internal/client` | 连接运行时的 Go 客户端与平台 dialer |
| `internal/server` | 现有 HTTP 服务与 Swagger 接入 |
| `internal/cmd` | 命令行适配入口 |

其余 `internal/*` 多为路径、环境、日志、diff、OAuth、Shell、更新和字符串等支撑包。判断是否属于主桌面路径时，从 `desktop -> RuntimeBridge -> internal/runtime` 反向追踪依赖。

## React 目录

| 路径 | 职责 |
|---|---|
| `client/src/app/shell` | 主工作台布局 |
| `client/src/features/sidebar` | 项目与会话导航 |
| `client/src/features/composer` | 用户输入和提交 |
| `client/src/features/timeline` | 对话与活动时间线 |
| `client/src/features/tools` | 工具调用展示 |
| `client/src/features/permissions` | 权限请求交互 |
| `client/src/features/agentTasks` | 子任务展示与交互 |
| `client/src/features/plugins` | Skills/MCP 等能力管理 |
| `client/src/features/settings` | 模型、provider 和运行时设置 |
| `client/src/features/recovery` | 错误恢复与重试 |
| `client/src/features/diagnostics` | 运行时诊断 |
| `client/src/runtime` | DTO、adapter、事件刷新与输出 store |

## 契约与变更定位

- 查 HTTP endpoint 或事件名：`internal/runtimeapi/contract.go`
- 查 Wails 暴露能力：`desktop/runtime_bridge.go`
- 查前端可调用方法：`client/src/runtime/workbenchTypes.ts` 与 adapters
- 查 Runtime DTO：`internal/runtime/runtime_contract_types.go`
- 查数据库历史：`internal/db/migrations`
- 查构建入口：根 `Taskfile.yaml`、`desktop/Taskfile.yml`、`client/package.json`
