# 开发指南

## 环境

- Go 版本以 `go.mod` 为准（当前为 1.26.3）。
- 前端使用 npm，依赖和脚本见 `client/package.json`。
- 桌面构建需要 Wails 3 工具链及对应平台 WebView/打包依赖。
- 根 Taskfile 设置了 `CGO_ENABLED=0` 和 `GOEXPERIMENT=greenteagc`；单独执行命令时注意与任务环境的差异。

## 常用命令

```powershell
# Go 全量构建与测试
go build ./...
go test ./...

# 前端
cd client
npm install
npm run dev
npm run build
npm run lint

# 桌面端
cd desktop
task sync:frontend
task dev
task build

# 仓库 lint
task lint
```

根 `task test` 会启用 race detector；日常快速验证可以先执行 `go test ./...`，涉及并发、事件流、调度或终端的改动应再执行 race 测试。

## 推荐验证范围

| 改动 | 最低验证 |
|---|---|
| Runtime/Go 领域逻辑 | 对应包测试 + `go test ./...` |
| Runtime API/DTO | runtime、runtimeapi、desktop bridge 测试 |
| React/TypeScript | `npm run build` + 相关 smoke 脚本 |
| 桌面桥接/资源 | `task sync:frontend` + desktop build/smoke |
| DB/schema | 迁移测试、sqlc 生成、全量 Go 测试 |
| 调度/权限/恢复 | 对应 harness/smoke + race 测试 |

`client/package.json` 和 `desktop/scripts` 中保留了多组针对输出、调度、上下文、项目侧栏和 packaged WebView 的 smoke 测试；选择与改动相关的脚本运行，不依赖旧阶段文档解释其行为。

## 开发约束

- 修改前先定位权威状态所在的 Go 包和 RuntimeService 接口。
- 新前端能力先设计 DTO/view model 边界，再接入组件。
- 新 Wails 方法应有对应 runtime 方法；不得为 browser/dev 增加重复 HTTP 契约。
- 新工具必须接入 scheduler、policy/permission、Hook、审计和结构化输出链路。
- 数据结构变化通过新 migration 演进，并保持旧数据可升级。
- 不在主产品路径引入新的 CLI/TUI 依赖。
- 避免无关包拆分；当前 runtime 已按功能文件拆分，优先在现有边界内演进。

## 调试入口

- 根入口设置 `AGENT_BUILDER_PROFILE` 后在 `localhost:6060` 提供 pprof。
- Runtime 状态通过 Wails bridge 测试和 RuntimeService 测试检查。
- 审计、Run transition、Prompt assembly、Hook execution、sandbox decision 和 recovery 记录用于定位跨层问题。
- 桌面 WebView 问题应分别验证共享 React 构建、资源同步、Wails bridge 和 packaged 环境。
