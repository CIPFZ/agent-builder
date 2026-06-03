# Session Callchain 调用链路可视化设计

**日期：** 2026-05-26
**状态：** 设计完成

---

## 一、背景与动机

### 1.1 现状

Crush 的会话数据完整存储在 SQLite `messages` 表中，包含 user/assistant/tool 消息及其全部 ContentPart（reasoning、text、tool_call、tool_result 等）。ToolResultGuard 将超大的工具结果溢出到 `.crush/results/{session_id}/{tool_call_id}.txt`。

但**没有工具能把这些数据以调用链路的形式可视化出来**。现有 `crush session show` 是消息列表格式，不能展示模型与工具之间的前后交互逻辑。

### 1.2 目标

提供一个 CLI 命令，用于：

1. **人类分析** — 直观展示一个 session 中 LLM 与工具的调用链路树，方便调试和理解模型行为
2. **喂给 AI 分析** — 结构化 JSON 输出，AI 可精确解析后做深度分析（如"这个 session 中模型有没有正确使用工具结果"）

---

## 二、命令设计

### 2.1 命令形式

```
crush session callchain <session_id> [flags]
```

| flag | 短名 | 默认值 | 说明 |
|---|---|---|---|
| `--verbose` | `-v` | false | 展开完整内容（reasoning 全文、tool_call 完整入参、tool_result 完整输出） |
| `--json` | — | false | 输出 JSON 格式（默认文本树形） |
| `--max-depth` | `-n` | 0（不限制） | 限制嵌套 agent 工具调用的展开深度 |
| `--tool-only` | `-t` | false | 只显示 tool_call → tool_result 配对，跳过纯文本消息 |
| `--last` | `-l` | false | 直接查看最近一个 session，不需要指定 session_id |

### 2.2 Session 发现

复用现有 `crush session list` / `crush session ls` 列出所有 session（已实现）。

---

## 三、数据模型

### 3.1 CallNode 树结构

```go
type NodeType string

const (
    NodeUser      NodeType = "user"
    NodeAssistant NodeType = "assistant"
    NodeTool      NodeType = "tool"
)

type CallNode struct {
    Type      NodeType    `json:"type"`
    Role      string      `json:"role"`
    MessageID string      `json:"message_id"`
    Timestamp int64       `json:"timestamp"`
    Summary   string      `json:"summary"`
    Detail    *CallDetail `json:"detail,omitempty"`
    Meta      *CallMeta   `json:"meta,omitempty"`
    Children  []*CallNode `json:"children"`
}
```

### 3.2 详细内容（--verbose 填充）

```go
type CallDetail struct {
    Text      string          `json:"text,omitempty"`
    Reasoning *ReasoningInfo  `json:"reasoning,omitempty"`
    ToolCalls []ToolCallInfo  `json:"tool_calls,omitempty"`
    ToolResult *ToolResultInfo `json:"tool_result,omitempty"`
}

type ReasoningInfo struct {
    Text       string `json:"text"`
    DurationMs int64  `json:"duration_ms"`
}

type ToolCallInfo struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Input string `json:"input"`
}

type ToolResultInfo struct {
    ToolCallID   string `json:"tool_call_id"`
    Content      string `json:"content"`
    StoredPath   string `json:"stored_path,omitempty"`
    OriginalSize int64  `json:"original_size,omitempty"`
    TruncatedBy  string `json:"truncated_by,omitempty"`
    IsError      bool   `json:"is_error"`
}

type CallMeta struct {
    PromptTokens     int64 `json:"prompt_tokens,omitempty"`
    CompletionTokens int64 `json:"completion_tokens,omitempty"`
    DurationMs       int64 `json:"duration_ms,omitempty"`
}
```

### 3.3 树构建规则

1. 遍历消息列表（`created_at` 升序）
2. `user` 消息 → 创建 `NodeUser` 根节点
3. `assistant` 消息 → 创建 `NodeAssistant`，挂在当前 user 节点下
4. `tool` 消息 → 创建 `NodeTool`，通过 `ToolCallID` 匹配到前一个 assistant 中对应的 `ToolCall`，挂在同一个 assistant 下
5. 连续 assistant 消息（无中间 user）→ 依次挂在同一 user 下，表示多轮工具调用
6. Agent 工具（`agent`、`agentic_fetch`）→ 创建嵌套子节点，展开子会话消息

---

## 四、文本树形输出

### 4.1 默认模式

```
Session: 排查 OOM 问题
路径:    /Users/linj/projects/myapp
模型:    claude-sonnet-4-6    Token: 入 12,340 / 出 4,567    消息: 15 条
Duration: ~8.3s
────────────────────────────────────────────────────────────

▼ [1] user: "帮我排查一下服务器 OOM 的问题，Java 进程总是被..."
  ▼ [2] assistant · reasoning (2.3s)
    │ [text] "让我先看一下内存和进程情况。"
    │ [tool_call] bash: free -m
    ▼ [3] tool: bash (1.2 KB · 0.1s)
      │               total   used   free   shared   buff/cache   available
      │ Mem:          15892   3456   8901    234      3535          11234
      │ Swap:          2048      0   2048
  ▼ [4] assistant · reasoning (1.8s)
    │ [text] "内存正常，检查 OOM 日志。"
    │ [tool_call] bash: journalctl -u myapp --since "10 min ago" | grep -i oom
    │ [tool_call] view: /etc/sysctl.conf
    ▼ [5] tool: bash (45.2 KB · persisted → .crush/results/abc/def.txt · 0.3s)
      │ <persisted-output>
      │ This tool result was too large (45,678 characters, 44.6 KB).
      │ Full output saved to: .crush/results/abc/def.txt
      │ Preview (head 200 + tail 100 chars):
      │   May 26 14:22:15 myapp kernel: oom-killer invoked...
      │   ...
      │   May 26 14:22:16 myapp kernel: Out of memory: Killed process 28492 (java)
      │ ...
      │ </persisted-output>
    ▼ [6] tool: view (2.1 KB · 0.1s)
      │ # Kernel sysctl configuration
      │ vm.overcommit_memory = 0
      │ ...
  ▼ [7] assistant · reasoning (3.1s)
    │ [text] "找到了。OOM Killer 杀掉了 Java 进程（PID 28492），..."
```

### 4.2 --verbose 模式

所有折叠内容展开：reasoning 全文、tool_call 完整 JSON input、tool_result 完整输出。被持久化的文件标注路径但不全量打印（避免刷屏）。

### 4.3 格式规则

| 元素 | 默认 | --verbose |
|---|---|---|
| user 文本 | 截断 100 字符 | 全文 |
| reasoning | 只显示 `· reasoning (Xs)`，不展示内容 | 全文 |
| text 内容 | 全文（通常不长） | 全文 |
| tool_call input | 不显示，只显示工具名 | 完整 JSON input |
| tool_result | persisted-output 块 或截断 200 字符 | 全文 |
| nested agent | 一行 `[agent] coder → 12 条子消息` | 展开完整子树 |

### 4.4 颜色

| 角色 | 颜色 |
|---|---|
| user | 蓝色 |
| assistant | 绿色 |
| tool | 黄色 |
| reasoning | 灰色斜体 |
| 错误 | 红色 |
| persisted-output | 品红 |

---

## 五、JSON 输出

`--json` 标志切换为 JSON 输出。

### 5.1 顶层结构

```json
{
  "session_id": "abc123",
  "title": "排查 OOM 问题",
  "working_dir": "/Users/linj/projects/myapp",
  "model": "claude-sonnet-4-6",
  "total_tokens": {"prompt": 12340, "completion": 4567},
  "total_duration_ms": 8320,
  "message_count": 15,
  "nodes": [ ... ]
}
```

### 5.2 设计决策

- **扁平化** — 统一 `CallNode` 结构 + `type` 字段区分，AI 解析不需要分支判断
- **detail 仅在 `--verbose` 时填充** — 减小默认 JSON 体积
- **溢出文件处理** — `stored_path` 存在时，`content` 保留预览，不完全读盘
- **嵌套 agent** — `children` 递归展开，`--max-depth` 控制深度

---

## 六、组件结构

### 6.1 文件清单

```
internal/callchain/           # 新建包
├── tree.go                   # 消息列表 → 调用树构建
├── format_text.go            # 文本树形输出（默认）
└── format_json.go            # JSON 结构化输出

internal/cmd/session.go       # 修改：新增 callchain 子命令
```

### 6.2 依赖关系

```
cmd/session.go → callchain/tree.go → session.Service.ListMessages()
               → callchain/format_text.go
               → callchain/format_json.go
```

不依赖 UI 层（`internal/ui/`），纯数据 + 格式化。复用现有 `message.Message` / `message.ContentPart` 类型。

### 6.3 不变更

- SQLite messages 表
- `crush session list` 等现有命令
- TUI UI 层
- ToolResultGuard 持久化机制

---

## 七、测试策略

| 测试对象 | 覆盖点 |
|---|---|
| `tree.go` | 单轮 user→assistant→tool 构建、多轮连续调用、嵌套 agent 展开、错误 tool_result、空 session |
| `format_text.go` | 默认摘要截断、verbose 全展开、persisted-output 渲染、terminal 宽度自适应 |
| `format_json.go` | 序列化完整树、omitempty 行为、嵌套深度正确 |
| CLI 集成 | `--last` 自动选择最近 session、`--max-depth` 限制、无效 session_id 报错 |

---

## 八、后续迭代

- TUI 中使用同一棵 `CallNode` 树做可视化交互（可折叠树 + 键盘导航）
- 支持直接从 callchain JSON 输出重建为 AI prompt（一键喂给模型分析）
- 溢出文件内容按需加载（`--load-persisted` 标志从磁盘填充完整 tool_result）
