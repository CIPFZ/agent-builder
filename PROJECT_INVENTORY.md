# Agent Builder 项目全量梳理

> 基于 `C:/Users/ytq/work/ai/agent-builder/` 仓库的完整盘点。基准日期 2026‑07‑04。

---

## 1. 项目定位

**Agent Builder** 是一个基于 **Go + Wails 3 + React** 的桌面 Agent 客户端。
当前正在围绕 **client‑first 架构** 进行重构：以 Agent Builder 自己为运行时基座，
Claude Code 作为交互设计参考，但**禁止复制其品牌、文案或视觉资产**。

- **模块路径**：`github.com/CIPFZ/agent-builder`
- **仓库**：`C:/Users/ytq/work/ai/agent-builder/`
- **目录归属**：单仓多模块（desktop / client / internal/*），并非 Go workspace
- **运维脚本**：Taskfile（`task lint`、`go build ./...`、`go test ./...`、`cd client && npm run build`）

---

## 2. 顶层目录结构

```
agent-builder/
├── AGENTS.md                        # 开发指南（架构原则、命令、边界）
├── README.md                        # 项目说明
├── Taskfile.yaml / Taskfile.yml     # 任务编排（lint、build）
├── go.mod / go.sum                  # Go 模块
├── agent-builder.json               # Agent Builder 自身配置（providers/hooks/skills/…）
├── desktop/                         # Wails 桌面壳
├── client/                          # React 前端
├── internal/                        # 业务运行时（Go 服务端）
├── docs/                            # 设计 & 存档文档
├── scripts/                         # 仓库级脚本
├── dev-dist/  bin/                  # 构建产物目录
└── examples/  testdata/             # 配置样例 / 测试数据
```

---

## 3. 架构总览

```
+---------------------------------------------------------------+
|                          client/  (React)                     |
|  - Ant Design + Ant Design X 主 UI                            |
|  - Zustand/Redux 微状态 + 工作台（Workbench）状态              |
|  - src/runtime/* 适配器层（wails / static / http-dev）        |
+---------------▲-----------------------+-----------------------+
                | wails bindings (webview)                     |
                | 或 HTTP/fetch 回退（vite/dev）               |
+---------------+-----------------------▼-----------------------+
|                          desktop/  (Wails shell)             |
|  - main.go / wails.json / build/                              |
|  - runtime_bridge.go  把 internal/runtime 暴露给 webview      |
+---------------▲-----------------------------------------------+
                | 直接调用同进程 Go 结构体
+---------------+-----------------------------------------------+
|                       internal/runtime (Go)                   |
|  - RuntimeService（transport-neutral）：turn / event / sse    |
|  - AppContainer：装配 Config / DB / Agent / Tools / Perm       |
+---------------▲-----------------------------------------------+
                |
+---------------+-----------------------------------------------+
|                       internal/* (领域实现)                    |
| agent | tools | permission | session | message | db | config  |
| skills | hooks | lsp | oauth | events | workspace | projects  |
| audit | runtimeapi | client (transport) | app | cmd           |
+---------------------------------------------------------------+
```

**核心边界原则（来自 `AGENTS.md` / `docs/desktop-runtime-boundary.md`）**

1. Runtime 状态属于 Go，不属于浏览器。
2. Wails 是适配层，不是业务边界。
3. 前后端走 transport‑neutral 抽象；浏览器开发用 HTTP/dev transport，
   不假设 `fetch`、axios、Wails bindings 一定可用。
4. CLI/TUI 兼容视为历史遗留，不进主产品路径。
5. React 只把 Go DTO 映射为 UI view model，不持有运行时真理。

---

## 4. `internal/` 领域包全量清单

按依赖层级自下而上排列，便于阅读。

### 4.1 模型 & 持久化
| 包 | 职责 |
|---|---|
| `internal/session` | 会话实体（与 Claude Code 兼容的会话模型） |
| `internal/message` | 消息/内容块结构（tool_use、tool_result、thinking …） |
| `internal/event` | 事件模型 |
| `internal/db` | SQLite 存储 + migrations |
| `internal/audit` | 审计日志 |
| `internal/history` | 历史记录裁剪与卫生化 |

### 4.2 配置 / 上下文
| 包 | 职责 |
|---|---|
| `internal/config` | 配置加载 & 校验 |
| `internal/pubsub` | 内部事件总线 |
| `internal/commands` | slash‑commands |
| `internal/projects` | 项目元数据 |
| `internal/workspace` | 工作区路径解析 |
| `internal/workbench` | 工作台聚合 |

### 4.3 能力扩展
| 包 | 职责 |
|---|---|
| `internal/skills` | 自定义技能加载 |
| `internal/skills/builtin` | 内置 skills（`agent-builder-config`、`agent-builder-hooks`、`jq`、`skill-creator`）|
| `internal/hooks` | Claude Code 兼容 hook 执行链（`PreToolUse` / `PostToolUse` / `PostToolUseFailure` / `UserPromptSubmit`，事件定义 `PreCompact` / `PostCompact` / `PostSampling` / `Stop`） |
| `internal/lsp` | LSP 客户端与 util |
| `internal/oauth` | OAuth（`copilot`、`hyper` 子包） |
| `internal/memory` | 轻量级记忆管理 |

### 4.4 执行引擎
| 包 | 职责 |
|---|---|
| `internal/agent` | Agent 循环：provider 抽象、prompt 模板、compact、history hygiene、HookedTool |
| `internal/agent/templates` | Agent 内置 prompt 模板 |
| `internal/agent/tools` | Agent 调用工具（含 MCP） |
| `internal/tools` | 工具 SDK（`invowk` 之类） |
| `internal/tools/scheduler` | **工具调度器**（见 `docs/tool-scheduler-design.md`） |

### 4.5 权限 & 治理
| 包 | 职责 |
|---|---|
| `internal/permission` | 权限图元语 + policy 引擎（见 `docs/permission-policy-model.md`） |
| `internal/permission/diff` | 命令差异比对 |

### 4.6 运行时面
| 包 | 职责 |
|---|---|
| `internal/runtime` | **RuntimeService**、事件流、SSE、HTTP 适配 |
| `internal/runtimeapi` | Runtime 与前端之间的 DTO 契约（`contract.go`） |
| `internal/app` | AppContainer 装配 runtime 全部依赖 |
| `internal/cmd` | 命令行入口（CLI 兼容残留） |
| `internal/client` | 外部 SDK / transport 客户端（Windows / 其他平台 dialing 分支） |

---

## 5. 运行时核心：`internal/runtime`

> **规模**：`~73 个 .go` 文件，是项目体量最大的一块。

主要子组件（推断自职责）：

| 组件文件 | 职责 |
|---|---|
| `runtime_service.go` | RuntimeService 主接口，对接 frontend |
| `runtime_service_types.go` | 服务层 DTO |
| `runtime_http.go` | HTTP transport，给 vite 浏览器开发用 |
| `runtime_*.go` | 拆分的子模块（turn / session / event / audit / skill / hook / perm 适配） |
| `runtime_api_bridge.go` | 桥接到 Wails / 外部 |

**运行时模型**：见 `docs/turn-task-run-model.md`

```
User → Turn → Run → Task → ToolCall → ToolScheduler → Policy Gate → Hook Chain → Provider → Event → Client
```

- Turn：用户的最小交互单元。
- Run：同 Turn 内可多轮采样。
- Task：会话内的工作项。
- ToolScheduler：批准 / 并发 / 重试 / 取消。
- Hook Chain：用户脚本可逐事件注入。
- Event：广播到 SSE / Wails / 内部总线。

---

## 6. 前端 `client/`

- **包管理**：npm（`package.json`）；构建 **Vite**
- **UI 基础**：**Ant Design** + **Ant Design X**
- **样式**：CSS Modules + Ant Design tokens（`docs/frontend-runtime-ui-technical-plan.md`）
- **入口**：`src/main.tsx → App.tsx`

### 6.1 `client/src/runtime/`（适配器层）

| 文件 | 角色 |
|---|---|
| `workbenchTypes.ts` | 工作台 UI 类型契约（来自 RuntimeService） |
| `outputTypes.ts` | 输出模型 |
| `outputStore.ts / outputReducer.ts / outputStream.ts / outputSelectors.ts / actionRefreshSelector.ts / runtimeEventRefresh.ts` | 客户端状态机（streaming / diff / 事件合并） |
| `wailsWorkbenchAdapter.ts` | 在桌面内通过 `window.go.agentbuilder.*` 调用 |
| `staticWorkbenchAdapter.tsx` | 浏览器/Vite 开发回退，返回静态假数据用于 UI 开发 |

### 6.2 `client/src/app/shell/` & `features/`

前端功能模块（按目录组织）：

| 模块 | 负责 |
|---|---|
| `shell/` | 应用壳（侧栏 / 主面板 / 顶栏） |
| `workspace/` | 工作区视图 |
| `composer/` | 提示词编辑器 + 提交 |
| `timeline/` | 时序事件流 |
| `sidebar/` | 会话 / 项目列表 |
| `tools/` | 工具调用 UI |
| `permissions/` | 权限对话框 |
| `hooks/` | Hook 配置面板 |
| `settings/` | 设置页 |
| `recovery/` | 错误恢复 / 重试 |
| `diagnostics/` | 日志 / 诊断 |
| `todos/` | 待办列表 |
| `markdown/` | Markdown 渲染 |
| `plugins/` | 插件面板（MCP、skills） |
| `agentTasks/` | 子任务面板 |

### 6.3 `client/src/lib/`

- `i18n/`：多语言
- `styles/`：共享样式 token
- 其他辅助（fetcher、hook、util）

---

## 7. 桌面壳 `desktop/`

> Wails 3 入口：`desktop/main.go`

- `main.go`：构建 `App`，加载 `runtime_bridge.go`
- `runtime_bridge.go`：将 `internal/runtime.RuntimeService` 暴露给 webview
- `frontend/`：Wails 默认前端目录（实际产物来自 `client/`）
- `build/`：Windows / Darwin / Linux 打包资源（icon、manifest、Info.plist）
- `Taskfile.yml`：desktop 任务（`task dev`、`task build`）
- `README.md`：Wails 桌面壳说明

构建链路：

```
client (vite build) → frontend/dist/
        │
        ▼
desktop (wails build) → bin/ + 可执行文件
```

---

## 8. 文档体系 `docs/`

```
docs/
├── README.md
├── frontend-runtime-ui-technical-plan.md          # AntD + AntDX UI 总纲
├── frontend-runtime-integration-notes.md          # 浏览器 vs Wails 边界
├── desktop-runtime-boundary.md                    # 何时用 Wails / 何时用 HTTP
├── client-architecture-and-core-flow.md           # 前端流图
├── tool-scheduler-design.md                       # 工具调度设计
├── turn-task-run-model.md                         # 运行时模型
├── permission-policy-model.md                    # 权限模型
├── permission-policy-rules.md                     # 权限规则示例
├── runtime-service-design.md                      # RuntimeService 拆分
├── runtime-framework-design.md                    # Runtime 架构总纲
├── remote-host-integration-design.md              # 远端主机接入
├── …
├── hooks/                                         # 用户文档（Claude Code 风格）
│   └── README.md                                  # hook 全部事件 + 环境变量
├── organize/                                      # 面向工程师的体系梳理
│   ├── 01-project-overview.md
│   ├── 02-module-function-overview.md
│   ├── 03-runtime-flow.md
│   └── 04-module-deep-dive.md
└── archive/                                       # 历史文档归档
```

> 当前文档 **双轨制**：仓库根 `AGENTS.md` 是机器友好总纲；`docs/` 内有多份专门设计文档和面向用户/Agent 两种语气的版本（`hooks/README.md` 仿 Claude Code），分层清晰。

---

## 9. 配置与构建

| 项 | 内容 |
|---|---|
| Go 模块 | `github.com/CIPFZ/agent-builder` |
| Wails | Wails 3 |
| 前端 | Vite + React + TypeScript |
| UI 库 | Ant Design + Ant Design X |
| 状态 | Zustand/Redux（视 feature） + 适配器层 |
| 持久化 | SQLite（`internal/db`） |
| 任务 | Taskfile（`task lint`、各构建） |

---

## 10. 工作准则（AI Agent 开发时要遵守）

来自 `AGENTS.md` 与 `docs/`：

1. **聚焦当前重构边界** —— 不在迁移窗口期引入新模块拆分。
2. **保持测试可见** —— 移动包时保证旧测试仍然跑通。
3. **运行时 DTO ≠ UI 视图** —— React 持有 ViewModel，runtime 持有真理。
4. **样式用主题 token + CSS Modules**，别堆全局 CSS。
5. **传输层中立**：浏览器只通过 `staticWorkbenchAdapter` / HTTP 调；
   只有桌面 Wails 运行环境下才使用 `wailsWorkbenchAdapter`。
6. **品牌中性**：可借鉴 Claude Desktop 信息架构，但**禁止复制品牌、资产、具体视觉**。
7. **CLI/TUI 是历史包袱** —— 不进入主产品路径。

---

## 11. 风险点 / 观察（仅观察，不修复）

- `desktop/runtime/` 与 `internal/runtime/` 同名前缀，初次接触易混淆；需结合目录与 AGENTS.md 区分。
- `docs/` 内存在同一主题多份文档（如 `hooks/README.md` 既面向人也面向 Agent），
  后续若用 `skill` 化整理需要去重。
- `runtime_bridge.go` 与 `wailsWorkbenchAdapter.ts` 是桌面/前端唯一耦合点，
  任何重构都需要同步测试这两个文件。
- 测试覆盖主要在 `internal/agent/*_test.go`、`internal/runtime/*_test.go`，
  客户端尚未看到完整单测基础设施。

---

*结束。本梳理以可见文件 + AGENTS.md + 关键设计文档为依据，
未做编译或运行验证，仅作静态盘点。*
