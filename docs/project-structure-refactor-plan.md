# 项目目录整理与文件清理方案

本文定义 Agent Builder 从可行性验证 demo 走向稳定客户端项目的目录整理方案。当前阶段先输出方案，不移动代码。

## 背景判断

当前项目仍保留大量 Crush 原始结构，同时叠加了 Wails3 + React 客户端 PoC。它已经证明了方向可行，但目录、文件归属和架构边界还不稳定。

主要问题：

- `desktop/agent-builder/runtime_*` 承担了通用 runtime 职责，但位于 desktop 目录下。
- `desktop/agent-builder` 多套了一层目录，后续应改成单层 `desktop/`。
- `internal` 中混合了 Crush core、CLI/TUI、runtime、平台工具、客户端边界等不同职责。
- CLI/TUI 相关代码仍容易污染客户端主路径。
- React 客户端目前按技术层分组，后续应向 feature 分组演进。
- 根目录仍有较多 Crush 遗留配置，需要逐项确认保留、迁移或删除。

## 整理目标

最终项目应形成清晰边界：

```text
agent-builder/
  client/              # React 客户端
  desktop/             # Wails 桌面壳，单层目录
  internal/runtime/    # 客户端优先的 Go runtime
  internal/agent/      # agent loop、model、context
  internal/tools/      # tool scheduler、builtin tools、MCP tools
  internal/adapters/   # Wails/HTTP/CLI/TUI adapter
  internal/platform/   # OS、路径、进程、环境等底层能力
  docs/                # 架构和迁移文档
  scripts/             # 构建、测试、开发脚本
```

核心方向：

```text
React Client -> Runtime API + Event Stream -> internal/runtime -> internal/agent/tools
```

Wails 只作为桌面 adapter，不拥有业务 runtime。

## 目录归属原则

### Product Path

产品主路径只包含客户端产品必需代码：

- `client/`
- `desktop/`
- `internal/runtime/`
- `internal/agent/`
- `internal/tools/`
- `internal/config/`
- `internal/session/`
- `internal/message/`
- `internal/db/`
- `internal/permission/`
- `internal/skills/`
- `internal/hooks/`
- `internal/lsp/`

### Adapter Path

adapter 只做协议、平台或 UI 外壳适配：

- Wails bridge
- local HTTP API
- SSE transport
- legacy CLI
- legacy TUI

adapter 不应拥有 session、turn、tool、permission、audit 的业务状态。

### Legacy Path

CLI/TUI 相关能力应逐步归为 legacy 或 adapter：

- `internal/ui`
- `internal/cmd`
- `internal/commands`
- Bubble Tea / `tea.Msg`
- terminal prompt / keybinding / slash command UI

短期不强删，但客户端主路径不能继续依赖它们。

## 目标目录结构

### 顶层目录

目标：

```text
agent-builder/
  .agents/
  .github/
  client/
  desktop/
  docs/
  internal/
  scripts/
  go.mod
  go.sum
  README.md
  Taskfile.yaml
```

待确认或清理：

| 文件 | 建议 |
| --- | --- |
| `CLA.md` | 判断是否仍是产品文档；若是 Crush 遗留，迁入 `docs/archive/` 或删除。 |
| `crush.json` | 若仍用于 runtime 配置，保留并文档化；否则迁入示例配置。 |
| `.goreleaser.yml` | 若桌面客户端不用原 Crush release，重写或删除。 |
| `flake.nix` / `flake.lock` | 若团队不用 Nix，删除；否则保留并更新用途说明。 |
| `schema.json` | 若仍服务 Crush config，保留到 `internal/config` 或 `docs/reference/`；否则归档。 |
| `sqlc.yaml` | 若 DB 代码仍使用 sqlc，保留；否则删除。 |

### Desktop 目录

当前：

```text
desktop/
  agent-builder/
    main.go
    runtime_*.go
    go.mod
    go.sum
    README.md
    Taskfile.yml
```

目标：

```text
desktop/
  main.go
  bridge.go
  app.go
  wails.json or wails config
  README.md optional
```

要求：

- 不再保留 `desktop/agent-builder/` 二级目录。
- `desktop/` 只保留 Wails 桌面壳、窗口、菜单、打包、native bootstrap。
- `runtime_*` 中通用逻辑迁到 `internal/runtime/`。
- desktop 内的 `go.mod/go.sum` 需要评估是否保留。优先使用根 `go.mod`，避免多 Go module 造成依赖漂移。

迁移后调用关系：

```text
desktop/bridge.go -> internal/runtime.Service
```

### Internal 目录

当前 `internal` 包较多，来自 Crush 原始结构。不要一次性全部移动，先按职责分层。

目标方向：

```text
internal/
  runtime/
    service.go
    turns.go
    sessions.go
    messages.go
    events.go
    permissions.go
    audit.go
    capabilities.go
    skills.go
    mcp.go
    http.go
    sse.go

  agent/
    loop / coordinator / provider / context

  tools/
    scheduler/
    builtin/
    mcp/

  adapters/
    wails/
    http/
    cli/
    tui/

  platform/
    env/
    home/
    osprocess/
    fsext/
    filepathext/
    dns/

  config/
  db/
  message/
  session/
  permission/
  skills/
  hooks/
  lsp/
```

注意：Go 包迁移会影响 import，必须分阶段做。

### Client 目录

当前：

```text
client/src/
  api/
  components/
  hooks/
  runtime/
```

目标：

```text
client/src/
  app/
  runtime/
  features/
    chat/
    sessions/
    permissions/
    settings/
    capabilities/
    skills/
    mcp/
    audit/
  shared/
    components/
    hooks/
    utils/
```

原则：

- `runtime/` 保留 TypeScript runtime contract 和 adapters。
- feature 目录承载具体产品功能。
- `shared/` 只放跨 feature 的 UI 和工具。
- 不在 React 中保存 runtime 事实状态作为权威来源。

## 分阶段实施计划

### 阶段 0：冻结现状

目标：避免清理过程中丢失 PoC 结论。

动作：

- 保留当前新增架构文档。
- 新增本目录整理方案。
- 记录当前可用验证命令。
- 不移动代码。

验收：

```powershell
git status --short
```

### 阶段 1：文件清点与归类

目标：明确哪些文件保留、迁移、归档、删除。

输出：

```text
docs/legacy-crush-inventory.md
```

清点范围：

- 根目录 Crush 遗留文件。
- `internal/ui` / `internal/cmd` / `internal/commands`。
- `desktop/agent-builder` 内重复 module 文件。
- 过时 docs。
- demo-only 或 mock-only 文件。

分类：

| 分类 | 含义 |
| --- | --- |
| keep | 当前产品主路径需要。 |
| migrate | 需要移动到新目录。 |
| legacy | 暂时保留，但不属于客户端主路径。 |
| archive | 只作历史参考，迁入 docs/archive。 |
| delete | 明确无用，可删除。 |

### 阶段 2：Desktop 单层化

目标：把 `desktop/agent-builder` 改成单层 `desktop/`。

动作：

- 移动 Wails 入口和桌面壳代码到 `desktop/`。
- 更新 Taskfile、构建脚本、Wails 配置和 README。
- 评估并移除 desktop 子 module，优先使用根 Go module。
- 保留最小 bridge，先不大规模改 runtime 逻辑。

目标结构：

```text
desktop/
  main.go
  runtime_bridge.go
  README.md
  Taskfile.yml optional
```

验收：

```powershell
go test ./...
cd client
npm run build
```

### 阶段 3：抽出 `internal/runtime`

目标：把通用 runtime 从 desktop 包迁出。

迁移对象：

- `runtime_service.go`
- `runtime_service_types.go`
- `runtime_contract_types.go`
- `runtime_internal_types.go`
- `runtime_lifecycle.go`
- `runtime_status.go`
- `runtime_turns.go`
- `runtime_sessions.go`
- `runtime_events.go`
- `runtime_permissions.go`
- `runtime_audit*.go`
- `runtime_capabilities.go`
- `runtime_skills.go`
- `runtime_mcp*.go`
- `runtime_model*.go`
- `runtime_http.go`
- `runtime_sse.go`

目标：

```text
internal/runtime/
  service.go
  contract_types.go
  lifecycle.go
  turns.go
  sessions.go
  events.go
  permissions.go
  audit.go
  capabilities.go
  skills.go
  mcp.go
  model.go
  http.go
  sse.go
```

desktop 中只保留：

```text
desktop/runtime_bridge.go
```

验收：

```powershell
go test ./internal/runtime ./desktop ./internal/runtimeapi
go test ./...
```

### 阶段 4：Runtime Event 去 TUI 化

目标：客户端主路径不再依赖 `tea.Msg`。

动作：

- 建立 runtime-native event bus。
- 将 `tea.Msg` 转换限制在 TUI adapter。
- `internal/runtime` 只发布 `runtimeapi.Event` 或后续 runtime event 类型。
- Wails/HTTP/SSE 只消费 runtime event。

验收：

- `internal/runtime` 不 import Bubble Tea。
- 客户端事件仍能收到 message/tool/permission/turn 事件。

### 阶段 5：Tool Scheduler 与 PermissionPolicy

目标：整理工具调用和权限审批主链路。

动作：

- 新增 `internal/tools/scheduler`。
- ToolCall 成为 runtime object。
- PermissionPolicy 从 permission service 中抽象出来。
- permission decision 写入 audit。

验收：

- tool call lifecycle 可通过 API 查询。
- permission request 可恢复。
- audit 能按 turn 聚合 tool + permission。

### 阶段 6：React Feature 化

目标：整理客户端目录和产品模块。

迁移：

```text
components/chat        -> features/chat
components/permissions -> features/permissions
components/settings    -> features/settings
components/runtime     -> features/capabilities 或 features/runtime
hooks/useAssistant...  -> features/chat 或 app
api/chat.ts            -> runtime/client 或 runtime/api
```

验收：

```powershell
cd client
npm run build
```

### 阶段 7：删除与归档

目标：清理确认无用的遗留文件。

只能删除满足以下条件的文件：

- 不在 product path。
- 不被测试和构建引用。
- 不承担历史参考价值。
- 已在 `legacy-crush-inventory.md` 标记为 delete。

建议先归档后删除，特别是 Crush 原始文档和配置。

## 禁止事项

整理过程中不要：

- 一次性大规模移动所有 `internal` 包。
- 在没有测试保护时删除 CLI/TUI。
- 让 React 直接承担 runtime 事实状态。
- 在 `desktop/` 中继续新增业务 runtime。
- 继续保留 `desktop/agent-builder` 二级目录。
- 把 Wails bridge 当成长期业务协议。

## 优先级

最高优先级：

1. `desktop/agent-builder` 单层化为 `desktop/`。
2. `desktop/runtime_*` 通用逻辑迁到 `internal/runtime`。
3. `tea.Msg` 从客户端 runtime 主路径剥离。
4. Tool Scheduler / PermissionPolicy 建立主链路。
5. React 按 feature 整理。

## 推荐下一步

下一步先输出文件清点文档：

```text
docs/legacy-crush-inventory.md
```

然后从低风险开始执行：

1. 清点根目录和 `desktop/agent-builder`。
2. 标记 keep/migrate/legacy/archive/delete。
3. 先做 `desktop/agent-builder -> desktop` 单层化。
4. 再迁移 runtime 到 `internal/runtime`。

