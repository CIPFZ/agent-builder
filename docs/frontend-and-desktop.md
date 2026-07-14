# 前端与桌面端

## 前端结构

前端入口为 `client/src/main.tsx`，应用主体位于 `client/src/app/App.tsx`。主要目录：

```text
client/src/app/            应用入口、错误边界和 WorkbenchShell
client/src/features/       按产品功能拆分的 UI
client/src/runtime/        Runtime DTO、适配器、输出 store 与刷新逻辑
client/src/lib/            通用前端能力
```

`features/` 当前包含会话侧栏、工作区、输入区、时间线、工具、权限、Hooks、设置、恢复、诊断、Todos、插件和 Agent Tasks 等界面。

## 状态边界

前端状态分为两类：

- Runtime 投影：会话、消息、Run、工具、权限、输出、能力等，可通过 RuntimeService 重新读取。
- 纯 UI 状态：面板展开、选择、临时输入、布局等，只影响展示。

Runtime 投影不能只存在浏览器内存中。提交成功、事件到达、窗口恢复或重连后，前端应以 runtime 快照重新校准。

## 传输适配

`client/src/runtime/wailsWorkbenchAdapter.ts` 是唯一产品适配器。请求/响应通过 Wails bindings，持续输出通过 Wails events。`staticWorkbenchAdapter.tsx` 仅用于纯 UI 数据构造，不得作为 Runtime 传输降级。

代码必须考虑以下环境差异：

- Wails WebView 可调用生成的/动态注入的 binding 和 Wails runtime。
- Vite 浏览器开发不能假定 Wails binding 存在。
- 应用内浏览器环境不保证任意网络 API 都可用。
- 传输检测和降级集中在适配器，不散落到 feature 组件。

## Canonical conversation state

Conversation state uses only the canonical V2 path. The adapter requests a
canonical full/window snapshot, records its decimal cursor in the normalized
React store, and subscribes to atomic canonical entity batches after that
cursor. The reducer applies revisioned upserts and tombstones idempotently;
duplicates and older revisions cannot regress state. A cursor gap, stream
overflow, reconnect, session switch, or explicit `snapshotRequired` starts a
new snapshot recovery cycle.

The normalized store feeds a pure Turn selector. Tool presentation grouping is
performed once after semantic selection and is never written back into the
store. Timeline rows, detail drawers, permission actions, AgentTask surfaces,
and the Todo capsule all consume the same canonical entities. Workbench
refreshes may update settings and diagnostics, but must not write conversation
entities or replace the active canonical store.

## UI 约束

- 优先复用 Ant Design/Ant Design X 组件和 theme token。
- 新功能样式使用 feature 或组件级 CSS Modules。
- `styles.css` 只保留真正的全局基础规则。
- Runtime DTO 先映射为 UI view model，避免组件直接依赖庞大的后端结构。

## 主题与配色

应用主题由两个正交设置组成：`themeId` 选择配色方案，`colorMode`
选择 `system`、`light` 或 `dark`。用户选择存入 `application_settings`；
内置配色方案保存在 `client/src/theme/themes/` 并通过 Theme Registry 按 ID
解析。业务组件不得根据主题 ID 编写条件分支或主题选择器。

每个主题独立提供浅色和深色语义 Token。统一的 Theme Provider 将同一份
Token 同时映射为 `--app-*` CSS Variables 和 Ant Design `ThemeConfig`。
Feature CSS Modules 只消费语义变量；新增主题不应要求修改业务组件。

新增内置配色方案时，应建立独立主题目录并实现 `AppTheme` 契约，然后在
Registry 中注册。无法解析的主题 ID 必须回退到 `builtin.default`。

运行 `cd client && npm run lint:theme-colors` 可检查业务代码是否重新引入
固定的 hex、rgb 或 rgba 颜色。固定颜色只允许出现在主题定义和终端 ANSI
调色板中；其他视觉表面必须使用 `--app-*` 语义变量。终端背景、前景、
光标和选区来自当前主题，ANSI 色则分别维护经过可读性校验的浅色和深色表。

## Wails 桌面壳

`desktop/main.go` 注册 `RuntimeBridge`，嵌入前端产物并创建最小尺寸受控的主窗口。`desktop/scripts/sync-client-dist.mjs` 负责把共享客户端构建到桌面资源目录。

桥接方法应保持薄：参数校验和传输转换可以放在桥接层，业务状态转换、策略和持久化必须留在 RuntimeService。
