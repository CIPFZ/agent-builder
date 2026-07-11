# 数据与能力

## SQLite 持久化

`internal/db` 包含连接实现、schema、SQL 查询和 sqlc 生成代码。`internal/db/migrations` 按时间顺序演进数据库，当前覆盖：

- sessions、messages、todos 和 read files；
- runtime turns、runs、transitions、events 和 audit；
- tool calls、permissions、sandbox decisions 和 hook executions；
- agent tasks、task messages/results 和 worktrees；
- provider settings、selected model 和 model metadata；
- prompt assemblies、context governance、message usage 和 project memory；
- projects 与 session management。

修改持久化模型时应新增迁移，更新 SQL/sqlc 代码，并验证旧数据库升级；不要直接重写历史迁移来适配新代码。

## 配置

`internal/config` 负责配置加载、作用域解析、provider、MCP、LSP、上下文治理和原子写入。配置可能来自全局和项目范围，调用方应使用解析后的配置而不是自行拼接路径或覆盖优先级。

Provider 配置与所选模型由 runtime 暴露给客户端，并包含验证、模型目录、endpoint、代理、密钥引用和 context window 等信息。敏感值不应通过普通日志或前端诊断完整回显。

## Skills

`internal/skills` 负责 Skills 的发现、目录、诊断、启停和附加路径；`internal/skills/builtin` 保存内置技能。Skill 是 Prompt 和工作流程能力的一部分，其加载状态会进入 runtime capability 投影。

## MCP

MCP 支持服务器配置、启停、刷新、重试、工具开关、resources、prompts 和交互请求决策。MCP 工具进入统一工具发现、权限和结果记录链路，不是独立的旁路执行器。

## Hooks

`internal/hooks` 负责 Hook 输入、匹配与执行。Runtime 记录 hook execution，并可把 Hook 产生的上下文、输入改写、阻断或失败投影到事件和审计中。

Hook 属于运行时边界：前端可以配置和展示 Hook，但不能在浏览器中代替 Go runtime 执行安全相关 Hook。

## LSP 与文件状态

`internal/lsp` 管理语言服务器连接、handler 和 workspace edit。`internal/filetracker`、`internal/fsext` 和 runtime read-file 记录共同支持文件读取状态、陈旧检测和上下文治理。

## 项目记忆

`internal/memory` 提供项目级 Markdown 记忆的路径、扫描、索引、检索和存储。Runtime API 支持列表、创建、更新、禁用、删除、刷新索引和诊断。记忆注入应遵守项目范围、启用状态和上下文预算。

## 终端、子任务与 Worktree

- Runtime 维护终端所有权和 session 关联，前端使用 xterm.js 展示。
- Agent Tasks 支持角色、有效 scope、消息、结果、输出、follow-up 和取消。
- Worktree 能力记录创建、进入、退出、清理与策略结果。

这些能力都应通过 RuntimeService 进行生命周期和审计管理，避免由 UI 直接管理外部进程或 Git 工作区事实。
