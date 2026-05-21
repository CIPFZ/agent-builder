# 客户端信息架构

本文定义 Agent Builder 客户端的信息架构。目标是保持 Codex 形态的 conversation-first，同时让 runtime 能力可发现、可配置、可审计。

## 产品定位

Agent Builder 是桌面 agent 客户端，不是 CLI/TUI，不是配置管理后台，也不是单纯聊天 demo。

第一屏应服务日常工作：

- 选择或创建 session。
- 输入任务。
- 查看 assistant 输出。
- 查看 tool/permission 状态。
- 必要时打开设置、能力、审计面板。

## 主布局

```text
Desktop Shell
  Left Sidebar
    Sessions
    Search
    Runtime navigation

  Center Workspace
    Chat timeline
    Active turn status
    Composer

  Drawers/Modals
    Model settings
    Permission review
    Tool detail
    Audit
    MCP/Skill detail
```

## 一级区域

### Chat

默认视图。

包含：

- session title
- model switcher
- runtime status
- message timeline
- tool cards
- active turn progress
- cancel
- composer

Chat 不应该被配置项挤占。配置和高级诊断应在 drawer 或独立 runtime view。

### Sessions

左侧 sidebar 中展示：

- 新建会话
- 会话列表
- active session
- rename/delete
- usage 摘要可后续加入

Session 是上下文边界，不是简单前端 tab。

### Capabilities

展示 runtime 可用能力：

- built-in tools
- MCP tools
- MCP prompts/resources
- skills
- future plugins

Capabilities 是只读总览为主，具体配置跳转到 Skills/MCP/Settings。

### Skills

展示：

- builtin skills
- local skills
- enabled/disabled
- invalid/error 状态
- refresh
- add path
- create skill

Skill instructions 由 runtime 注入上下文，前端不负责拼 prompt。

### MCP

展示：

- server list
- connection state
- tools/resources/prompts counts
- refresh
- enable/disable server
- enable/disable tool
- simple add/edit
- error detail

Secret 字段必须 redacted。

### Audit / Diagnostics

按 turn/session 查看：

- model/provider
- usage
- events
- tool calls
- permission decisions
- skills/MCP inventory
- errors
- duration

Audit 是 runtime 数据，不是前端 console。

### Settings

包含：

- model provider
- base URL
- API key
- model discovery
- proxy
- policy mode
- future workspace settings

Settings 不作为默认首页。模型未配置时可以自动打开或给出明确入口。

## 客户端状态分类

前端状态分三类：

| 类型 | 归属 | 示例 |
| --- | --- | --- |
| Runtime state | Go runtime | sessions、messages、turns、permissions、capabilities |
| UI state | React | drawer open、sidebar collapsed、selected panel |
| Draft state | React until submitted | composer text、未保存表单 |

不要把 runtime state 存到 localStorage 作为事实来源。

## 核心用户流程

### 首次配置

```text
Open app
  -> model not configured
  -> open settings drawer
  -> save provider/url/key/model
  -> verify
  -> enter Chat
```

### 日常对话

```text
Open app
  -> restore active session
  -> type message
  -> create turn
  -> observe streaming/tool events
  -> approve if needed
  -> inspect result/audit if needed
```

### 能力管理

```text
Open Capabilities
  -> see inventory
  -> open Skills or MCP
  -> refresh/edit/toggle
  -> runtime emits inventory events
  -> Chat uses updated capability set
```

### 审计诊断

```text
Open audit from turn/message
  -> read runtime audit
  -> inspect tool calls/permissions/usage
  -> copy or export later
```

## UI 不应承担的职责

React 不应：

- 构造 assistant/tool message。
- 判断工具是否危险。
- 拼接 skill prompt。
- 直接执行 MCP/tool。
- 保存 API key 到浏览器本地存储作为主配置。
- 从 message 文本反推核心状态。
- 自己维护 active turn 真相。

## 现有实现评价

当前 `client/src/AssistantClient.tsx` 和 `client/src/hooks/useAssistantClient.tsx` 已经接近目标信息架构：

- Chat 为默认主视图。
- Sidebar 有 session。
- Settings drawer 管理模型。
- Permission modal 已有。
- RuntimeFeatureWorkspace 管 capabilities/skills/MCP。
- Audit drawer 已有。

下一步重点不是大改页面，而是让页面消费更稳定的 runtime primitives：

- active turn API
- ToolCall API
- Permission lifecycle
- event cursor/recovery
- audit store

