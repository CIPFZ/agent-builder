# 项目目录规划

这份规划的目标不是“像标准模板一样好看”，而是让你的学习路径和代码结构同步增长。

## 顶层目录职责

### `cmd/`

- `cmd/myclaw`：面向用户的 CLI 程序入口。
- `cmd/myclawd`：守护进程或服务端入口，适合承载 WebSocket gateway。

这样做的好处是，CLI 和 daemon 后面即使依赖同一套 `internal/app` 装配逻辑，也不会互相耦死。

### `internal/app`

- 应用初始化
- 依赖组装
- 配置注入
- 生命周期管理

你可以把它理解为“把零件装成系统”的地方，不承担具体业务细节。

### `internal/gateway`

- WebSocket 服务启动
- 客户端连接管理
- 会话绑定
- 消息收发与广播

这是阶段一最先长肉的目录。

### `internal/protocol/ws`

- WebSocket 收发消息结构
- 编码/解码
- 事件类型常量

把协议结构从 `gateway` 拆出来，后面测试会轻松很多。

### `internal/llm`

- 大模型客户端封装
- block streaming
- tool streaming
- token / message 适配

建议这里先做 provider-neutral 抽象，哪怕第一版只接一个模型。

### `internal/prompt`

- system prompt 模板
- 角色提示词拼接
- workspace 配置注入

后面你研究 `AGENTS.md`、`SOUL.md`、`TOOLS.md` 的心得，也可以沉淀成这里的模板策略。

### `internal/workspace`

- 读取 `~/.openclaw/workspace`
- 合并本地配置
- 处理缺省值和覆盖规则

这个目录的存在感第 3 天会非常强，后面也会持续参与 prompt 构建。

### `internal/session`

- Session ID 分配
- `main` / 非主会话区分
- 生命周期管理
- 路由到工具、模型、沙盒

会话是整个系统的中轴，建议不要把它混进 `gateway`。

### `internal/store`

- 上下文持久化
- 历史消息保存
- 会话元数据存储

第一版可以从内存实现开始，后面再补文件或 sqlite。

### `internal/tools`

- 工具注册表
- 调用分发
- allowlist / denylist

子目录建议这样拆：

- `internal/tools/system`：宿主机工具，比如 `system.run`
- `internal/tools/sessions`：跨会话工具，比如 `sessions_list`

### `internal/node`

- `node.list`
- `node.describe`
- `node.invoke`

把 Node 协议单列出来，会让“工具协议”与“工具实现”边界更清楚。

### `internal/sandbox`

- 沙盒路由决策
- 主 / 非主会话执行策略
- 权限判定

### `internal/sandbox/docker`

- Docker SDK 封装
- 容器创建/销毁
- 命令执行
- 文件挂载与资源限制

阶段三可以主要围绕这两个目录推进。

### `internal/agent`

- agent loop
- 内部任务转发
- 多代理协同

### `internal/runtime`

- 把 gateway、session、llm、tools、sandbox 串起来
- 组织一次完整请求的运行流程

如果 `agent` 更像“大脑”，那 `runtime` 更像“神经系统”。

### `internal/cli`

- `/status`
- `/new`
- `/reset`
- 其他命令解析与路由

### `internal/event`

- 内部事件模型
- 状态通知
- 会话消息投递

等你做到 `REPLY_SKIP`、`ANNOUNCE_SKIP` 这类机制时，这里会很有用。

### `internal/model`

- Session
- Message
- ToolCall
- SandboxPolicy

核心领域模型统一放这里，避免类型散落。

### `internal/config`

- 配置结构体
- 环境变量映射
- 本地配置文件解析

### `pkg/types`

- 只放确实需要跨层复用且不依赖业务细节的公共类型

如果暂时没有公共能力，宁可空着，也不要把 `internal/` 的东西提前抽出来。

### `docs/`

- `docs/plan`：阶段计划、模块分工
- `docs/notes`：对标 openclaw 的源码阅读笔记

推荐你每天新增一篇，比如：

- `docs/notes/day-01-gateway.md`
- `docs/notes/day-02-agent-loop.md`

## 推荐的实现顺序

1. 先做 `cmd/myclawd` + `internal/app` + `internal/gateway`
2. 再补 `internal/protocol/ws` 与 `internal/model`
3. 接着做 `internal/llm`、`internal/prompt`、`internal/workspace`
4. 然后建立 `internal/session`、`internal/store`
5. 最后扩展 `tools`、`sandbox`、`agent`、`cli`

## 当前这版规划的特点

- 适合从空仓库起步
- 能支持你按阶段逐步实现
- 后期加 Docker 和多代理时不用大搬家
- 对 Go 项目来说足够清晰，但不会一开始过度抽象
