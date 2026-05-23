import type { RefObject } from 'react'
import Alert from 'antd/es/alert'
import Button from 'antd/es/button'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import type { MenuProps } from 'antd'
import { AppstoreOutlined, AuditOutlined, CodeOutlined, DownOutlined, EditOutlined, MenuOutlined, SettingOutlined, StopOutlined, ToolOutlined } from '@ant-design/icons'
import type { TextAreaRef } from 'antd/es/input/TextArea'
import type { ModelConfig } from '../../runtime/api'
import type { RuntimeAgentTask, RuntimeMessage, RuntimeSession, RuntimeStatus, RuntimeTodoSummary, RuntimeTurn } from '../../runtime'
import { Composer } from './Composer'
import { MessageItem } from './MessageItem'
import { UsageReadout } from './UsageReadout'

const { Text, Title } = Typography

const starterPrompts = [
  { label: 'Write', icon: <EditOutlined />, prompt: 'Draft a Kubernetes troubleshooting SOP template.' },
  { label: 'Learn', icon: <AppstoreOutlined />, prompt: 'Explain the relationship between agent runtime, tools, skills, and MCP.' },
  { label: 'Code', icon: <CodeOutlined />, prompt: 'Design a minimal HTTP + SSE API for a Go agent runtime.' },
  { label: 'Ops', icon: <ToolOutlined />, prompt: 'Simulate a troubleshooting chat for a service error-rate spike.' },
]

function greeting() {
  const hour = new Date().getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

type ChatWorkspaceProps = {
  activeChatTitle: string
  activeSession?: RuntimeSession
  activeTurns: RuntimeTurn[]
  agentTasks: RuntimeAgentTask[]
  composerInputRef: RefObject<TextAreaRef | null>
  config: ModelConfig
  configLoaded: boolean
  hasMessages: boolean
  input: string
  isModelConfigured: boolean
  isSending: boolean
  lastError: string
  messages: RuntimeMessage[]
  modelItems: MenuProps['items']
  modelSwitching: boolean
  runtimeStatus: RuntimeStatus | null
  sidebarCollapsed: boolean
  todoSummary: RuntimeTodoSummary | null
  viewportRef: RefObject<HTMLDivElement | null>
  onCancelAgentTask: (taskId: string) => void
  onCancelTurn: () => void
  onCopyMessage: (content: string) => void
  onOpenAudit: () => void
  onOpenSettings: () => void
  onSendMessage: (text?: string) => void
  onSetInput: (value: string) => void
  onToggleSidebar: () => void
}

export function ChatWorkspace({
  activeChatTitle,
  activeSession,
  activeTurns,
  agentTasks,
  composerInputRef,
  config,
  configLoaded,
  hasMessages,
  input,
  isModelConfigured,
  isSending,
  lastError,
  messages,
  modelItems,
  modelSwitching,
  runtimeStatus,
  sidebarCollapsed,
  todoSummary,
  viewportRef,
  onCancelAgentTask,
  onCancelTurn,
  onCopyMessage,
  onOpenAudit,
  onOpenSettings,
  onSendMessage,
  onSetInput,
  onToggleSidebar,
}: ChatWorkspaceProps) {
  return (
    <main className="chat-main">
      <header className="chat-header">
        <Space>
          {sidebarCollapsed ? (
            <Tooltip title="Show sidebar">
              <Button type="text" icon={<MenuOutlined />} onClick={onToggleSidebar} />
            </Tooltip>
          ) : null}
          <Text strong>{activeSession?.title ?? activeChatTitle}</Text>
          {todoSummary && todoSummary.total > 0 ? (
            <Tag>
              Plan {todoSummary.completed}/{todoSummary.total}
            </Tag>
          ) : null}
          <DownOutlined className="muted-icon" />
          <UsageReadout status={runtimeStatus} />
        </Space>
        <Space size={4}>
          {runtimeStatus?.busy || isSending || activeTurns.length > 0 ? (
            <Tooltip title="Cancel current run">
              <Button type="text" danger icon={<StopOutlined />} onClick={onCancelTurn} />
            </Tooltip>
          ) : null}
          <Tooltip title="Audit">
            <Button type="text" icon={<AuditOutlined />} onClick={onOpenAudit} />
          </Tooltip>
          <Tooltip title="Model settings">
            <Button type="text" icon={<SettingOutlined />} onClick={onOpenSettings} />
          </Tooltip>
        </Space>
      </header>

      <div className="chat-viewport" ref={viewportRef}>
        {!hasMessages ? (
          <section className="welcome-pane">
            <Tag className="plan-pill">Crush runtime</Tag>
            <Title className="welcome-title">
              <span className="brand-flower">*</span>
              {greeting()}, Agent Builder
            </Title>
            <Composer
              config={config}
              input={input}
              isDisabled={!configLoaded || !isModelConfigured}
              isSending={isSending || modelSwitching}
              modelItems={modelItems}
              onChange={onSetInput}
              inputRef={composerInputRef}
              onOpenSettings={onOpenSettings}
              onSend={() => onSendMessage()}
            />
            <div className="starter-row">
              {starterPrompts.map((prompt) => (
                <button className="starter-chip" key={prompt.label} type="button" onClick={() => onSendMessage(prompt.prompt)}>
                  {prompt.icon}
                  {prompt.label}
                </button>
              ))}
            </div>
            {!isModelConfigured ? (
              <Alert
                className="runtime-alert"
                type="warning"
                showIcon
                message="Model configuration required"
                description="Open model settings and save protocol, URL, API key, and model before chatting."
                action={
                  <Button size="small" type="primary" onClick={onOpenSettings}>
                    Configure
                  </Button>
                }
              />
            ) : null}
            {lastError && isModelConfigured ? <Alert className="runtime-alert" type="error" showIcon message={lastError} /> : null}
            {activeTurns.length > 0 ? <Alert className="runtime-alert" type="info" showIcon message={`Recovered ${activeTurns.length} active turn${activeTurns.length === 1 ? '' : 's'}.`} /> : null}
          </section>
        ) : (
          <section className="thread">
            {todoSummary && todoSummary.total > 0 ? (
              <div className="runtime-todo-panel">
                <Space size={6} wrap>
                  <Text strong>Plan</Text>
                  <Tag>{todoSummary.pending} pending</Tag>
                  <Tag color="processing">{todoSummary.inProgress} active</Tag>
                  <Tag color="success">{todoSummary.completed} done</Tag>
                </Space>
                <div className="runtime-todo-list">
                  {todoSummary.todos.slice(0, 6).map((todo) => (
                    <div className="runtime-todo-row" key={`${todo.status}:${todo.content}`}>
                      <Tag>{todo.status.replace('_', ' ')}</Tag>
                      <Text>{todo.activeForm || todo.content}</Text>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}
            {agentTasks.length > 0 ? <AgentTaskPanel tasks={agentTasks} onCancel={onCancelAgentTask} /> : null}
            {messages.map((chatMessage) => (
              <MessageItem chatMessage={chatMessage} key={chatMessage.id} onCopy={onCopyMessage} />
            ))}
          </section>
        )}
      </div>

      {hasMessages ? (
        <div className="composer-dock">
          <Composer
            config={config}
            input={input}
            isDisabled={!configLoaded || !isModelConfigured}
            isSending={isSending || modelSwitching}
            modelItems={modelItems}
            onChange={onSetInput}
            inputRef={composerInputRef}
            onOpenSettings={onOpenSettings}
            onSend={() => onSendMessage()}
          />
          {lastError ? <Alert className="dock-alert" type="error" showIcon message={lastError} /> : null}
          <Text className="disclaimer">Agent Builder can make mistakes. Check important operations before execution.</Text>
        </div>
      ) : null}
    </main>
  )
}

function AgentTaskPanel({ tasks, onCancel }: { tasks: RuntimeAgentTask[]; onCancel: (taskId: string) => void }) {
  return (
    <div className="runtime-task-panel">
      <Space size={6} wrap>
        <Text strong>Child Tasks</Text>
        <Tag>{tasks.length}</Tag>
      </Space>
      <div className="runtime-task-list">
        {tasks.map((task) => (
          <div className="runtime-task-row" key={task.id}>
            <Space size={6} wrap>
              <Tag color={taskStatusColor(task.status)}>{task.status}</Tag>
              <Text>{task.title || task.name || task.kind}</Text>
              {task.childSessionId ? <Text type="secondary">child {shortID(task.childSessionId)}</Text> : null}
            </Space>
            <Space size={6}>
              {task.resultSummary ? <Text type="secondary">{task.resultSummary}</Text> : null}
              {task.error ? <Text type="danger">{task.error}</Text> : null}
              {task.status === 'running' || task.status === 'queued' ? (
                <Tooltip title="Cancel task">
                  <Button size="small" danger type="text" icon={<StopOutlined />} onClick={() => onCancel(task.id)} />
                </Tooltip>
              ) : null}
            </Space>
          </div>
        ))}
      </div>
    </div>
  )
}

function taskStatusColor(status: string) {
  if (status === 'completed') return 'success'
  if (status === 'failed' || status === 'interrupted') return 'error'
  if (status === 'cancelled') return 'default'
  return 'processing'
}

function shortID(value: string) {
  return value.length > 10 ? `${value.slice(0, 10)}...` : value
}
