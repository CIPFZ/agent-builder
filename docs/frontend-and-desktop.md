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

## 输出刷新

`outputStore.ts`、`outputReducer.ts`、`outputStream.ts` 和 selectors 组合处理消息流、工具状态与动作后刷新。事件用于提示“哪些数据可能变化”，最终状态仍由 runtime 快照确认。

新增运行时动作时，应同时明确：

1. 调用哪个 adapter 方法；
2. 成功后需要刷新哪些投影；
3. 哪些事件可触发同一刷新；
4. 重复事件和乱序事件如何保持幂等。

## UI 约束

- 优先复用 Ant Design/Ant Design X 组件和 theme token。
- 新功能样式使用 feature 或组件级 CSS Modules。
- `styles.css` 只保留真正的全局基础规则。
- Runtime DTO 先映射为 UI view model，避免组件直接依赖庞大的后端结构。

## Wails 桌面壳

`desktop/main.go` 注册 `RuntimeBridge`，嵌入前端产物并创建最小尺寸受控的主窗口。`desktop/scripts/sync-client-dist.mjs` 负责把共享客户端构建到桌面资源目录。

桥接方法应保持薄：参数校验和传输转换可以放在桥接层，业务状态转换、策略和持久化必须留在 RuntimeService。
