# 数据存储规范

本文定义桌面产品路径的持久化边界。实现、数据库和测试与本文冲突时，
应在同一变更中统一修正，不得增加第二套兼容存储。

## 权威来源

- SQLite 保存结构化配置、实体关系、运行记录、索引和审计数据。
- 文件系统保存天然属于文件的内容、大对象、扩展包和临时数据。
- 每项数据只能有一个权威来源；不得同时写入 JSON 和 SQLite。
- 缓存必须可删除、可重建；日志不得参与业务恢复。
- 桌面产品不使用 `internal/config` 的 JSON 写入作为持久化来源。
  Runtime 从 SQLite 组装只读的运行配置；JSON ConfigStore 仅属于遗留适配路径。

## 作用域

持久化数据使用四级归属：Application、Project、Session 和 Turn/Run。
逻辑作用域必须与物理作用域一致。

- Application：Provider、全局 Skill/MCP、默认 Policy、终端偏好和维护状态。
- Project：项目设置、Memory、项目 Skill/MCP 和项目 Objects。
- Session：对话、任务、文件历史、环境元数据、下载和临时数据。
- Turn/Run：ToolCall、Permission、Sandbox、Audit 和 Object 引用。

项目身份只使用数据库 `projects.id`。项目名称和工作路径都是可变属性，
不得用于内部目录身份。用户工作目录与 Agent Builder 的项目数据目录是两个边界。

## 目录契约

开发阶段 Runtime Root 可以位于 `desktop/bin`。其内部数据遵循：

```text
config/
  bootstrap.json                 # 可选；仅数据库打开前必需的启动参数
data/
  agent-builder.db               # 结构化数据的唯一权威来源
  projects/
    <project-id>/
      metadata.json              # 可选的人类可读镜像，不是权威来源
      memory/
      skills/
      mcp/
      objects/                   # 内容和大对象存储
      sessions/
        <session-id>/
          environment/
          downloads/
          tmp/
      worktrees/
  global/
    skills/
    plugins/
  cache/
    providers/
    models/
    updates/
  backups/
    migrations/
    imports/
    recovery/
logs/
  agent-builder.log
```

不得再创建共享的 `data/memory` 或旧的 `data/runtime_refs`。原 Runtime Ref
概念直接重命名为 Object；不保留旧名称、旧表或旧路径的兼容层。

## 配置归属

- `model.json`：删除；Provider、模型元数据和模型选择由 SQLite 保存。
- `terminal.json`：删除；Profile 动态发现，SQLite 只保存选中的 Profile ID。
- `policy.json`：删除；默认 Policy、项目覆盖和会话状态由结构化表保存。
- `skills.json`：删除；注册和启停状态在 SQLite，Skill 包在全局或项目目录。
- `mcp.json`：删除；Server 配置在 SQLite，文件资源在全局或项目目录。
- `agent-builder-audit.jsonl`：删除；审计只写 SQLite，JSONL 由导出功能按需生成。

敏感凭据不得写入普通 JSON、日志、诊断或可直接导出的配置字段。Provider
记录保存 secret reference；具体 secret storage 由独立能力提供。

## Objects

Object Store 位于 `data/projects/<project-id>/objects`，统一保存大型工具输出、
Artifact、Diff、Compact 原始内容、Shell Job 输出、Agent Task 产物、文件历史快照
和下载内容。数据库 Object 记录必须显式保存 `project_id`，并可关联 Session、Turn、
ToolCall 和 Task。

Object 写入使用临时文件、内容校验和原子重命名。数据库写入失败后必须清理新建的
无引用文件；维护任务负责发现孤儿文件、缺失内容和 hash 不一致。

ToolResultGuard 只负责生成有界的模型可见预览。完整的大型工具输出由 Runtime
Scheduler Recorder 写入项目 Object Store，预览通过 `runtime://objects/<id>` 引用；
Agent 层不得在用户工作目录创建 `.agent-builder/results` 或执行独立 TTL 清理。

## 生命周期

外置内容必须声明保留等级：`permanent`、`project`、`session`、`recovery`、
`cache` 或 `temporary`。项目和 Session 的硬删除应清理其独占数据；软删除不得提前
删除恢复所需内容。

缓存、日志、备份、临时文件和 Objects 分别统计容量并具有独立清理策略。完整性检查
覆盖数据库悬空引用、孤儿文件、Memory 索引、Session 残留目录和 Worktree 状态。

## 数据演进决策

当前处于开发阶段，本轮重构不迁移、不读取、不兼容旧 JSON、旧数据库表、
`data/memory` 或 `data/runtime_refs`。数据库 schema 和测试 fixture 直接切换到新模型；
旧开发数据由开发者删除后重新创建。
