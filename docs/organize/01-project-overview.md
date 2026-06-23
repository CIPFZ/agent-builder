# 项目整体概述

## 项目定位

Agent Builder 是一个桌面优先的 agent runtime/client 项目，技术栈是 Go、Wails 3、React、Vite。当前 Go module path 是 `github.com/CIPFZ/agent-builder`，产品目标已经从 CLI/TUI agent 转向桌面客户端。
当前主路径：

```text
React Client
  -> Workbench Adapter
  -> Wails Bridge �?HTTP Dev Runtime API
  -> internal/runtime.RuntimeService
  -> internal/workbench.Service
  -> internal/agent / tools / permission / session / db
  -> runtime events / persisted state
  -> React hydration
```

核心原则�?Go runtime 作为事实来源，React 负责展示、输入、权限决策、设置和诊断，不把业务状态保存在浏览器内存里作为权威状态�?
## 主要入口

- `main.go`：根 CLI/runtime 入口，调�?`internal/cmd.Execute()`，带可�?pprof�?- `desktop/main.go`：Wails 3 桌面入口，创�?`Agent Builder` 窗口，注�?`NewRuntimeBridge()` 服务，并加载内嵌前端资产�?- `desktop/runtime_bridge.go`：Wails adapter，几乎所有方法都直接委托�?`internal/runtime.RuntimeService`�?- `client/src/app/App.tsx`：React app 入口，加�?`wailsWorkbenchAdapter.loadInitialViewModel()` 后渲�?`WorkbenchShell`�?- `internal/runtime/runtime_service.go`：创�?runtime 服务实例，初始化 runtime store、tool scheduler、event stream、HTTP API�?- `internal/runtime/runtime_service_types.go`：定�?transport-neutral �?`RuntimeService` 接口，是 Wails �?HTTP/dev adapter 的共同后端边界�?
## 主体架构分层

### 产品 UI �?
- `client/` �?React 产品 UI�?- 使用 Ant Design / Ant Design X 作为 UI 基础�?- `client/src/runtime/wailsWorkbenchAdapter.ts` 负责�?runtime DTO 映射�?UI view model�?- 前端支持 Wails binding、HTTP runtime API、dev module、JSONP、XHR 等多种传输路径，因为 Vite/in-app browser 不一定有 `fetch` �?Wails runtime�?- 当前实际状态管理是 `WorkbenchShell` 内部 `useState` �?adapter hydration；目标文档提�?TanStack Query �?Zustand，但当前 `client/package.json` 未引入这两个依赖�?
### 桌面适配�?
- `desktop/` �?Wails shell�?- `RuntimeBridge` 暴露 Wails service 方法�?- 桌面层不承载业务边界，而是通过 type alias 和转发方法把 runtime DTO 暴露�?Wails�?
### Runtime 服务�?
- `internal/runtime/` 是当�?client-first runtime 的核心�?- `RuntimeService` 覆盖项目、模型、provider、session、turn、run、tool call、permission、policy、event、audit、replay、skills、MCP、capabilities、terminal、worktree、agent task 等能力�?- Runtime 同时提供 Wails adapter �?loopback HTTP API�?
### Agent Builder 后端兼容�?
- `internal/workbench/` 仍承�?Agent Builder runtime 聚合能力�?- `internal/apitypes/` 是旧 API DTO/protocol 边界�?- runtime 目前仍依�?`workbench.Service`、`apitypes.Workspace`、`apitypes.Message`、`apitypes.PermissionRequest` 等类型，再映射成 `Runtime*` DTO�?- `internal/server/` �?`internal/client/` 仍提�?legacy `/v1` HTTP/SSE/API client-server 能力，但当前桌面主路径和 `serve-http` 命令都走 `internal/runtime`�?
### Agent 与工具层

- `internal/agent/` 提供 session-based agent loop、provider/model、prompt、tool execution、tool result guard、subagent/task 等能力�?- `internal/agent/tools/` 是内置工具集合，包括文件查看/写入/编辑、bash、grep/glob/ls、web fetch/search、MCP、todos、LSP references 等�?- `internal/tools/scheduler/` 更像工具调用生命周期 store/事件模型，而不是完整执行队列。真实执行发生在 `fantasy.AgentTool` wrapper 中，runtime recorder 负责落库和事件�?
### 状态与持久化层

- `internal/db/` �?SQLite 连接、migration �?sqlc typed query 层�?- `internal/session/`、`internal/message/` 是核心会话和消息业务模型�?- runtime 额外通过 migration �?store 文件持久�?turns、tool calls、events、audit、permissions、agent tasks、worktrees、MCP requests、runs、run transitions、prompt assemblies、user inputs 等�?
## 当前主能力边�?
从代码接口看，Agent Builder 已经不是单纯聊天客户端，而是结构�?agent runtime�?
- 项目与工作区管理
- provider/model 配置、发现、验证、selected model
- session/message/turn 生命周期
- 多模�?语音/Slash/meta/shell 输入归一�?- tool call 调度、记录、输出、失�?取消状�?- permission policy、pending permission、headless policy
- runtime event stream、SSE、cursor replay
- audit、replay export、recovery
- run projection、checkpoint、scheduler plan、execute task
- skills、plugins、MCP server/tool/resource/prompt/request
- capability inventory �?tool search
- agent task/subagent、task message/result/output
- git worktree lifecycle �?task effective scope
- session terminal �?terminal stream
- context sources、prompt assemblies、budget、compact boundaries、read-files tracking

## 需要注意的边界事实

- `internal/workbench/server/apitypes` 这个路径当前不存在。实际相关边界是 `internal/workbench` �?`internal/apitypes`�?- `internal/runtimeapi` 提供契约冻结，但它的 `Endpoints` 清单�?`runtime_http.go` 的实际路由存在不完全同步的迹象�?- `RuntimeService` 接口很宽，说明主边界已经集中，但也意味着前端、Wails、HTTP、测�?mock 都容易与它产生漂移�?- 前端不是直接使用 Wails binding，而是通过 `WorkbenchAdapter` �?transport fallback �?DTO/view model 映射�?- 代码中存在少量中�?mojibake，例�?Wails 选择项目目录标题和部分历史文�?文案，影响文档与 UI 文案质量�?