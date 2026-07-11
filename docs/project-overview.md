# 项目概览

## 定位

Agent Builder 是一个以桌面客户端为主的 AI 编程 Agent。项目使用 Go 承载运行时和持久化，Wails 3 提供桌面容器，React 提供产品界面。

模块路径为 `github.com/CIPFZ/agent-builder`。当前主产品链路是：

```text
React Client -> Wails/HTTP Adapter -> Go Runtime -> Agent/Tools/Providers -> SQLite
```

根目录的 CLI/HTTP 入口仍然存在，但属于兼容和服务入口；桌面产品不应依赖 CLI/TUI 状态。

## 技术栈

| 层 | 主要技术 |
|---|---|
| 桌面容器 | Wails 3 |
| 前端 | React 19、TypeScript、Vite |
| UI | Ant Design 6、Ant Design X 2 |
| 终端 | xterm.js |
| 后端 | Go 1.26 |
| 持久化 | SQLite、SQL migrations、sqlc 生成代码 |
| Agent/模型 | Fantasy、OpenAI 兼容 provider、可配置 provider |
| 扩展 | Skills、MCP、Hooks、LSP |

## 核心原则

- Go runtime 是会话、运行、工具调用、权限、审计和恢复状态的权威来源。
- React 负责展示、输入和交互投影，只维护可重建的 UI 状态。
- Wails 是进程内传输适配器，不承载业务规则。
- Vite/browser 开发与 Wails WebView 必须共享同一套运行时契约。
- 工具调用、权限决策、事件和输出必须保持结构化并可恢复。
- 新 UI 使用 Ant Design token 和局部 CSS Modules，避免继续扩大全局样式。

## 顶层目录

```text
client/              React 客户端
desktop/             Wails 3 桌面壳与 RuntimeBridge
internal/runtime/    面向客户端的运行时服务与投影
internal/agent/      Agent 循环、Prompt、Provider、上下文处理
internal/tools/      工具调度与执行基础设施
internal/permission/ 权限与策略原语
internal/db/         SQLite、查询代码和迁移
internal/config/     配置加载、解析和校验
internal/skills/     Skills 发现、目录和内置技能
internal/hooks/      Hook 匹配与执行
internal/lsp/        LSP 客户端和管理器
docs/                当前架构与开发文档
```

更细的包级说明见 [目录索引](repository-map.md)。
