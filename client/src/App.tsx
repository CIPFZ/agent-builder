import { useEffect, useMemo, useRef, useState } from 'react'
import AntApp from 'antd/es/app'
import Alert from 'antd/es/alert'
import Button from 'antd/es/button'
import Collapse from 'antd/es/collapse'
import ConfigProvider from 'antd/es/config-provider'
import Drawer from 'antd/es/drawer'
import Dropdown from 'antd/es/dropdown'
import Flex from 'antd/es/flex'
import Form from 'antd/es/form'
import Input from 'antd/es/input'
import Modal from 'antd/es/modal'
import Select from 'antd/es/select'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import theme from 'antd/es/theme'
import type { MenuProps } from 'antd'
import {
  ApiOutlined,
  AppstoreOutlined,
  ArrowDownOutlined,
  CodeOutlined,
  CopyOutlined,
  DownOutlined,
  EditOutlined,
  FolderOutlined,
  MenuOutlined,
  MessageOutlined,
  PlusOutlined,
  ProjectOutlined,
  ReloadOutlined,
  SearchOutlined,
  SendOutlined,
  SettingOutlined,
  ShareAltOutlined,
  BulbOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  StopOutlined,
  ToolOutlined,
  UserOutlined,
} from '@ant-design/icons'
import TextArea from 'antd/es/input/TextArea'
import {
  cancelRuntimeTurn,
  decideRuntimePermission,
  loadModelConfig,
  requestConfiguredModels,
  requestRuntimeEvents,
  requestRuntimeEventsEndpoint,
  requestRuntimeMessages,
  requestRuntimePermissions,
  requestRuntimeStatus,
  saveModelConfig,
  sendRuntimePrompt,
  startRuntimeChat,
} from './api/chat'
import type { ModelConfig } from './api/chat'
import { subscribeRuntimeEvents } from './runtime/events'
import type {
  RuntimeEvent,
  RuntimeMessage,
  RuntimeMessagePart,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeStatus,
} from './runtime'
import './App.css'

const { Text, Title, Paragraph } = Typography

const defaultConfig: ModelConfig = {
  protocol: 'openai',
  model: '',
  url: '',
}

const starterPrompts = [
  { label: 'Write', icon: <EditOutlined />, prompt: 'Draft a Kubernetes troubleshooting SOP template.' },
  { label: 'Learn', icon: <AppstoreOutlined />, prompt: 'Explain the relationship between agent runtime, tools, skills, and MCP.' },
  { label: 'Code', icon: <CodeOutlined />, prompt: 'Design a minimal HTTP + SSE API for a Go agent runtime.' },
  { label: 'Ops', icon: <ToolOutlined />, prompt: 'Simulate a troubleshooting chat for a service error-rate spike.' },
]

const emptyUsage = {
  promptTokens: 0,
  completionTokens: 0,
  totalTokens: 0,
  cost: 0,
}

const runtimeEventLimit = 200

function greeting() {
  const hour = new Date().getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

function modelLabel(config: ModelConfig) {
  return config.model || 'Select model'
}

function hasAssistantText(chatMessage: RuntimeMessage) {
  return chatMessage.role === 'assistant' && chatMessage.content.trim() !== ''
}

function messageToolParts(chatMessage: RuntimeMessage) {
  return (chatMessage.parts ?? []).filter((part) => part.type === 'tool_call' || part.type === 'tool_result')
}

function messageReasoningParts(chatMessage: RuntimeMessage) {
  return (chatMessage.parts ?? []).filter((part) => part.type === 'reasoning' && part.thinking?.trim())
}

function AppContent() {
  const { message } = AntApp.useApp()
  const [messages, setMessages] = useState<RuntimeMessage[]>([])
  const [permissions, setPermissions] = useState<RuntimePermissionRequest[]>([])
  const [events, setEvents] = useState<RuntimeEvent[]>([])
  const [input, setInput] = useState('')
  const [config, setConfig] = useState<ModelConfig>(defaultConfig)
  const [models, setModels] = useState<string[]>([defaultConfig.model])
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)
  const [operationsOpen, setOperationsOpen] = useState(false)
  const [isSending, setIsSending] = useState(false)
  const [configLoaded, setConfigLoaded] = useState(false)
  const [lastError, setLastError] = useState('')
  const [activeChatTitle, setActiveChatTitle] = useState('New chat')
  const [runtimeStatus, setRuntimeStatus] = useState<RuntimeStatus | null>(null)
  const viewportRef = useRef<HTMLDivElement | null>(null)

  const hasMessages = messages.length > 0
  const isModelConfigured = Boolean(config.url && config.model && (config.hasApiKey || config.apiKey))
  const recentItems = useMemo(() => {
    const titles = messages
      .filter((chatMessage) => chatMessage.role === 'user' && chatMessage.content.trim())
      .map((chatMessage) => chatMessage.content.trim())
      .slice(-5)
      .reverse()
    return titles.length > 0 ? titles : [activeChatTitle]
  }, [activeChatTitle, messages])

  const refreshMessages = async () => {
    const runtimeMessages = await requestRuntimeMessages()
    setMessages(runtimeMessages)
  }

  const refreshStatus = async () => {
    const nextStatus = await requestRuntimeStatus()
    setRuntimeStatus(nextStatus)
    return nextStatus
  }

  const refreshPermissions = async () => {
    const nextPermissions = await requestRuntimePermissions()
    setPermissions(nextPermissions)
    return nextPermissions
  }

  useEffect(() => {
    loadModelConfig()
      .then((savedConfig) => {
        setConfig((current) => ({ ...current, ...savedConfig }))
        setLastError('')
      })
      .catch(() => {
        setConfig(defaultConfig)
        setLastError('Model is not configured. Open model settings and save protocol, URL, API key, and model before chatting.')
      })
      .finally(() => {
        setConfigLoaded(true)
      })
  }, [])

  useEffect(() => {
    requestConfiguredModels()
      .then((response) => {
        if (response.models.length === 0) return
        setModels(response.models)
        setConfig((current) => ({
          ...current,
          model: response.models.includes(current.model) ? current.model : response.models[0],
        }))
      })
      .catch(() => {
        setModels(config.model ? [config.model] : [])
      })
  }, [config.model])

  useEffect(() => {
    if (!isModelConfigured) return
    let cancelled = false
    Promise.all([requestRuntimeStatus(), requestRuntimeMessages(), requestRuntimePermissions(), requestRuntimeEvents()])
      .then(([nextStatus, runtimeMessages, runtimePermissions, runtimeEvents]) => {
        if (cancelled) return
        setRuntimeStatus(nextStatus)
        setMessages(runtimeMessages)
        setPermissions(runtimePermissions)
        setEvents(runtimeEvents)
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [isModelConfigured])

  useEffect(() => {
    if (!isModelConfigured) return
    let unsubscribe: (() => void) | undefined
    let cancelled = false

    requestRuntimeEventsEndpoint()
      .then(({ url }) => {
        if (cancelled || !url) return
        unsubscribe = subscribeRuntimeEvents(
          url,
          (event) => {
            setEvents((current) => [...current, event].slice(-runtimeEventLimit))
            if (event.type === 'message') {
              refreshMessages().catch(() => undefined)
              refreshStatus().catch(() => undefined)
            }
            if (event.type === 'permission_requested') {
              refreshPermissions().catch(() => undefined)
              refreshStatus().catch(() => undefined)
            }
          },
          () => undefined,
        )
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
      unsubscribe?.()
    }
  }, [isModelConfigured])

  const modelItems = useMemo<MenuProps['items']>(
    () =>
      models.map((modelName) => ({
        key: modelName,
        label: modelName,
        onClick: () => setConfig((current) => ({ ...current, model: modelName })),
      })),
    [models],
  )

  const scrollToBottom = () => {
    window.setTimeout(() => viewportRef.current?.scrollTo({ top: viewportRef.current.scrollHeight, behavior: 'smooth' }), 50)
  }

  const startNewChat = () => {
    if (!isModelConfigured) {
      setSettingsOpen(true)
      setLastError('Configure a model before creating a runtime session.')
      return
    }
    setInput('')
    setActiveChatTitle('New chat')
      startRuntimeChat('New chat')
      .then((nextStatus) => {
        setRuntimeStatus(nextStatus)
        setMessages([])
        setPermissions([])
        setEvents([])
      })
      .catch((error) => {
        const reason = error instanceof Error ? error.message : String(error)
        message.error(reason)
      })
  }

  const sendMessage = async (text = input) => {
    const content = text.trim()
    if (!content || isSending) return
    if (!isModelConfigured) {
      setSettingsOpen(true)
      setLastError('Configure a model before sending a message.')
      return
    }

    setInput('')
    setIsSending(true)
    setLastError('')
    if (!hasMessages) {
      setActiveChatTitle(content.length > 24 ? `${content.slice(0, 24)}...` : content)
    }
    try {
      const previousAssistantId = [...messages].reverse().find((chatMessage) => chatMessage.role === 'assistant')?.id
      await sendRuntimePrompt(content)
      let runtimeMessages = await requestRuntimeMessages()
      setMessages(runtimeMessages)
      await refreshPermissions().catch(() => undefined)
      let nextStatus = await refreshStatus().catch(() => runtimeStatus)
      const started = Date.now()
      while (Date.now() - started < 30 * 60 * 1000) {
        await new Promise((resolve) => window.setTimeout(resolve, 700))
        runtimeMessages = await requestRuntimeMessages().catch(() => runtimeMessages)
        setMessages(runtimeMessages)
        await refreshPermissions().catch(() => undefined)
        nextStatus = await refreshStatus().catch(() => nextStatus)
        scrollToBottom()
        const latestAssistant = [...runtimeMessages].reverse().find((chatMessage) => chatMessage.role === 'assistant')
        if (latestAssistant?.error) {
          setLastError(latestAssistant.error)
        }
        if (nextStatus && !nextStatus.busy && latestAssistant?.finished && latestAssistant.id !== previousAssistantId) break
      }
      runtimeMessages = await requestRuntimeMessages().catch(() => runtimeMessages)
      setMessages(runtimeMessages)
      await refreshPermissions().catch(() => undefined)
      await refreshStatus().catch(() => undefined)
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
      await refreshMessages().catch(() => undefined)
    } finally {
      setIsSending(false)
      scrollToBottom()
    }
  }

  const copyMessage = async (content: string) => {
    await navigator.clipboard.writeText(content)
    message.success('Copied')
  }

  const decidePermission = async (permissionId: string, action: RuntimePermissionDecision['action']) => {
    try {
      const nextStatus = await decideRuntimePermission({ permissionId, action })
      setRuntimeStatus(nextStatus)
      setPermissions((current) => current.filter((permission) => permission.id !== permissionId))
      message.success(action === 'deny' ? 'Permission denied' : 'Permission granted')
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
    }
  }

  const cancelTurn = async () => {
    try {
      const nextStatus = await cancelRuntimeTurn()
      setRuntimeStatus(nextStatus)
      setIsSending(false)
      await refreshMessages().catch(() => undefined)
      await refreshPermissions().catch(() => undefined)
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
    }
  }

  return (
    <div className="desktop-shell">
      <aside className="sidebar">
        <div className="sidebar-top">
          <Flex justify="space-between" align="center">
            <Space size={14}>
              <Button type="text" icon={<MenuOutlined />} />
              <Button type="text" icon={<SearchOutlined />} />
            </Space>
            <Tooltip title="New chat">
              <Button type="text" icon={<PlusOutlined />} onClick={startNewChat} />
            </Tooltip>
          </Flex>

          <div className="mode-switch">
            <button className="mode-tab active" type="button">
              <MessageOutlined />
              Chat
            </button>
            <button className="mode-tab" type="button" onClick={() => setOperationsOpen(true)}>
              <CodeOutlined />
              Ops
            </button>
          </div>

          <nav className="nav-list">
            <button className="nav-item active" type="button" onClick={startNewChat}>
              <PlusOutlined />
              New chat
            </button>
            <button className="nav-item" type="button">
              <FolderOutlined />
              Projects
            </button>
            <button className="nav-item" type="button" onClick={() => setOperationsOpen(true)}>
              <ProjectOutlined />
              Operations
            </button>
            <button className="nav-item" type="button" onClick={() => setSettingsOpen(true)}>
              <SettingOutlined />
              Model settings
            </button>
          </nav>
        </div>

          <div className="recents">
            <Text className="section-label">Recents</Text>
            {recentItems.map((item) => (
              <button className={item === activeChatTitle ? 'recent-item active' : 'recent-item'} key={item} type="button">
                {item}
              </button>
            ))}
          </div>

        <div className="sidebar-footer">
          <Space>
            <span className="avatar-dot">A</span>
            <span>Agent Builder</span>
            <Text type="secondary">Local</Text>
          </Space>
          <Tooltip title="Runtime logs">
            <Button type="text" size="small" icon={<ArrowDownOutlined />} onClick={() => setOperationsOpen(true)} />
          </Tooltip>
        </div>
      </aside>

      <main className="chat-main">
        <header className="chat-header">
          <Space>
            <Text strong>{activeChatTitle}</Text>
            <DownOutlined className="muted-icon" />
            <UsageReadout status={runtimeStatus} />
          </Space>
          <Space size={4}>
            {runtimeStatus?.busy || isSending ? (
              <Tooltip title="Cancel current run">
                <Button type="text" danger icon={<StopOutlined />} onClick={cancelTurn} />
              </Tooltip>
            ) : null}
            <Tooltip title="Runtime events">
              <Button type="text" icon={<ShareAltOutlined />} onClick={() => setOperationsOpen(true)} />
            </Tooltip>
            <Tooltip title="Model settings">
              <Button type="text" icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)} />
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
                isSending={isSending}
                modelItems={modelItems}
                onChange={setInput}
                onOpenSettings={() => setSettingsOpen(true)}
                onSend={() => sendMessage()}
              />
              <div className="starter-row">
                {starterPrompts.map((prompt) => (
                  <button className="starter-chip" key={prompt.label} type="button" onClick={() => sendMessage(prompt.prompt)}>
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
                    <Button size="small" type="primary" onClick={() => setSettingsOpen(true)}>
                      Configure
                    </Button>
                  }
                />
              ) : null}
              {lastError && isModelConfigured ? <Alert className="runtime-alert" type="error" showIcon message={lastError} /> : null}
            </section>
          ) : (
            <section className="thread">
              {messages.map((chatMessage) => (
                <MessageItem chatMessage={chatMessage} key={chatMessage.id} onCopy={copyMessage} />
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
              isSending={isSending}
              modelItems={modelItems}
              onChange={setInput}
              onOpenSettings={() => setSettingsOpen(true)}
              onSend={() => sendMessage()}
            />
            {lastError ? <Alert className="dock-alert" type="error" showIcon message={lastError} /> : null}
            <Text className="disclaimer">Agent Builder can make mistakes. Check important operations before execution.</Text>
          </div>
        ) : null}
      </main>

      <ModelSettingsDrawer
        config={config}
        open={settingsOpen}
        saving={settingsSaving}
        onClose={() => setSettingsOpen(false)}
        onSave={async (nextConfig) => {
          setSettingsSaving(true)
          try {
            const saved = await saveModelConfig(nextConfig)
            setConfig((current) => ({ ...current, ...saved }))
            setModels(saved.model ? [saved.model] : [])
            setLastError('')
            message.success('Model settings saved')
            setSettingsOpen(false)
          } catch (error) {
            const reason = error instanceof Error ? error.message : String(error)
            message.error(reason)
          } finally {
            setSettingsSaving(false)
          }
        }}
      />
      <PermissionReviewModal permissions={permissions} onDecide={decidePermission} />
      <OperationsPreview events={events} open={operationsOpen} onClose={() => setOperationsOpen(false)} />
    </div>
  )
}

function PermissionReviewModal({
  permissions,
  onDecide,
}: {
  permissions: RuntimePermissionRequest[]
  onDecide: (permissionId: string, action: RuntimePermissionDecision['action']) => Promise<void>
}) {
  const permission = permissions[0]

  return (
    <Modal
      title="Tool permission"
      open={Boolean(permission)}
      closable={false}
      maskClosable={false}
      footer={
        permission
          ? [
              <Button key="deny" danger onClick={() => onDecide(permission.id, 'deny')}>
                Deny
              </Button>,
              <Button key="allow-session" onClick={() => onDecide(permission.id, 'allow_session')}>
                Allow session
              </Button>,
              <Button key="allow" type="primary" onClick={() => onDecide(permission.id, 'allow')}>
                Allow once
              </Button>,
            ]
          : null
      }
      width={640}
    >
      {permission ? (
        <div className="permission-review">
          <Space wrap>
            <Tag>{permission.toolName}</Tag>
            <Tag>{permission.action}</Tag>
            {permission.path ? <Tag>{permission.path}</Tag> : null}
          </Space>
          {permission.description ? <Paragraph>{permission.description}</Paragraph> : null}
          {permission.params ? <pre className="part-preview">{JSON.stringify(permission.params, null, 2)}</pre> : null}
        </div>
      ) : null}
    </Modal>
  )
}

function MessageItem({ chatMessage, onCopy }: { chatMessage: RuntimeMessage; onCopy: (content: string) => void }) {
  const reasoningParts = messageReasoningParts(chatMessage)
  const toolParts = messageToolParts(chatMessage)
  const showText = chatMessage.role === 'user' || hasAssistantText(chatMessage)
  const isToolOnly = !showText && toolParts.length > 0

  return (
    <article className={`message-row ${isToolOnly ? 'tool' : chatMessage.role}`}>
      {chatMessage.role === 'user' ? (
        <div className="user-avatar">
          <UserOutlined />
        </div>
      ) : (
        <div className={isToolOnly ? 'tool-mark' : 'assistant-mark'}>{isToolOnly ? <ToolOutlined /> : '*'}</div>
      )}
      <div className="message-body">
        {reasoningParts.length > 0 ? <ReasoningPanel parts={reasoningParts} /> : null}
        {toolParts.length > 0 ? <ToolActivity parts={toolParts} /> : null}
        {showText ? <div className="message-bubble">{chatMessage.content}</div> : null}
        {hasAssistantText(chatMessage) ? (
          <Space className="message-actions" size={8}>
            <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => onCopy(chatMessage.content)} />
            <Button type="text" size="small" icon={<ReloadOutlined />} />
            {chatMessage.provider ? <Tag>{chatMessage.provider}</Tag> : null}
            {chatMessage.model ? <Tag>{chatMessage.model}</Tag> : null}
          </Space>
        ) : null}
      </div>
    </article>
  )
}

function ReasoningPanel({ parts }: { parts: RuntimeMessagePart[] }) {
  return (
    <div className="reasoning-panel">
      <Space size={8}>
        <BulbOutlined />
        <Text type="secondary">Thinking</Text>
      </Space>
      {parts.map((part, index) => (
        <pre className="part-preview" key={`${part.startedAt ?? index}-${index}`}>
          {part.thinking}
        </pre>
      ))}
    </div>
  )
}

function ToolActivity({ parts }: { parts: RuntimeMessagePart[] }) {
  return (
    <div className="tool-activity">
      {parts.map((part, index) => (
        <ToolActivityItem key={`${part.toolCallId ?? part.name ?? index}-${part.type}-${index}`} part={part} />
      ))}
    </div>
  )
}

function ToolActivityItem({ part }: { part: RuntimeMessagePart }) {
  const isResult = part.type === 'tool_result'
  const hasPreview = Boolean((isResult ? part.content || part.data || part.metadata : part.input)?.trim())
  const preview = isResult ? part.content || part.data || part.metadata : part.input

  return (
    <div className={part.isError ? 'tool-step error' : 'tool-step'}>
      <div className="tool-step-header">
        <Space size={8}>
          {part.isError ? <CloseCircleOutlined /> : isResult ? <CheckCircleOutlined /> : <ToolOutlined />}
          <Text strong>{part.name || 'tool'}</Text>
          <Tag>{isResult ? (part.isError ? 'failed' : 'result') : part.finished ? 'called' : 'running'}</Tag>
        </Space>
        {part.toolCallId ? <Text type="secondary">{part.toolCallId}</Text> : null}
      </div>
      {hasPreview ? <pre className="part-preview">{preview}</pre> : null}
    </div>
  )
}

type ComposerProps = {
  config: ModelConfig
  input: string
  isDisabled: boolean
  isSending: boolean
  modelItems: MenuProps['items']
  onChange: (value: string) => void
  onOpenSettings: () => void
  onSend: () => void
}

function Composer({ config, input, isDisabled, isSending, modelItems, onChange, onOpenSettings, onSend }: ComposerProps) {
  return (
    <div className="composer">
      <TextArea
        autoSize={{ minRows: 2, maxRows: 7 }}
        className="composer-input"
        placeholder="How can I help you today?"
        disabled={isDisabled || isSending}
        value={input}
        onChange={(event) => onChange(event.target.value)}
        onPressEnter={(event) => {
          if (!event.shiftKey) {
            event.preventDefault()
            onSend()
          }
        }}
      />
      <Flex justify="space-between" align="center" className="composer-toolbar">
        <Space>
          <Tooltip title="Clear input">
            <Button type="text" icon={<PlusOutlined />} onClick={() => onChange('')} />
          </Tooltip>
          <Tooltip title="Model settings">
            <Button type="text" icon={<ToolOutlined />} onClick={onOpenSettings} />
          </Tooltip>
        </Space>
        <Space>
          <Dropdown menu={{ items: modelItems }} trigger={['click']}>
            <Button type="text">
              {modelLabel(config)} <DownOutlined />
            </Button>
          </Dropdown>
          <Tooltip title="Open model settings">
            <Button type="text" icon={<ApiOutlined />} onClick={onOpenSettings} />
          </Tooltip>
          <Button type="primary" shape="circle" icon={<SendOutlined />} loading={isSending} onClick={onSend} />
        </Space>
      </Flex>
    </div>
  )
}

function UsageReadout({ status }: { status: RuntimeStatus | null }) {
  const usage = status?.usage ?? emptyUsage
  return (
    <Space className="usage-readout" size={10}>
      <Tag>{status?.busy ? 'Running' : 'Idle'}</Tag>
      <Text type="secondary">Tokens {usage.totalTokens}</Text>
      <Text type="secondary">In {usage.promptTokens}</Text>
      <Text type="secondary">Out {usage.completionTokens}</Text>
      <Text type="secondary">Events {status?.events?.messageEvents ?? 0}</Text>
      <Text type="secondary">Perms {status?.events?.permissionEvents ?? 0}</Text>
      <Text type="secondary">${usage.cost.toFixed(4)}</Text>
    </Space>
  )
}

type ModelSettingsDrawerProps = {
  config: ModelConfig
  open: boolean
  saving: boolean
  onClose: () => void
  onSave: (config: ModelConfig) => Promise<void>
}

function ModelSettingsDrawer({ config, open, saving, onClose, onSave }: ModelSettingsDrawerProps) {
  const [form] = Form.useForm<ModelConfig>()

  return (
    <Drawer
      title="Model settings"
      placement="right"
      size={420}
      open={open}
      onClose={onClose}
      afterOpenChange={(visible) => {
        if (visible) form.setFieldsValue(config)
      }}
      extra={
        <Button
          type="primary"
          loading={saving}
          onClick={() => {
            form.validateFields().then((values) => onSave(values))
          }}
        >
          Save
        </Button>
      }
    >
      <Paragraph type="secondary">Saved to the desktop config directory beside the application.</Paragraph>
      <Form form={form} layout="vertical" initialValues={config}>
        <Form.Item label="Protocol" name="protocol" rules={[{ required: true }]}>
          <Select
            options={[
              { value: 'openai', label: 'OpenAI compatible' },
              { value: 'anthropic', label: 'Anthropic compatible' },
            ]}
          />
        </Form.Item>
        <Form.Item label="URL" name="url" rules={[{ required: true }]}>
          <Input placeholder="https://api.example.com" />
        </Form.Item>
        <Form.Item label="API key" name="apiKey" rules={config.hasApiKey ? [] : [{ required: true }]}>
          <Input.Password placeholder={config.hasApiKey ? 'Saved. Leave empty to keep current key.' : 'sk-...'} />
        </Form.Item>
        <Form.Item label="Model" name="model" rules={[{ required: true }]}>
          <Input placeholder="model-name" />
        </Form.Item>
        <Collapse
          ghost
          items={[
            {
              key: 'advanced',
              label: 'Advanced',
              children: (
                <Form.Item label="Proxy" name="proxy">
                  <Input placeholder="http://127.0.0.1:7890" />
                </Form.Item>
              ),
            },
          ]}
        />
      </Form>
    </Drawer>
  )
}

function OperationsPreview({ events, open, onClose }: { events: RuntimeEvent[]; open: boolean; onClose: () => void }) {
  return (
    <Modal title="Operations workspace" open={open} onCancel={onClose} footer={<Button onClick={onClose}>Close</Button>} width={720}>
      <Paragraph>
        The SSH troubleshooting workspace is still available as the next layer, but it is intentionally no longer the first screen.
      </Paragraph>
      <Space wrap>
        <Tag>SSH target</Tag>
        <Tag>SOP skill</Tag>
        <Tag>MCP knowledge search</Tag>
        <Tag>Approval policy</Tag>
        <Tag>Runtime event log</Tag>
      </Space>
      <div className="event-log">
        {events.slice(-8).map((event) => (
          <div className="event-log-row" key={`${event.createdAt}-${event.type}-${event.messageId ?? event.sessionId ?? ''}`}>
            <Tag>{event.type}</Tag>
            {event.role ? <Text type="secondary">{event.role}</Text> : null}
            {event.summary ? <Text>{event.summary}</Text> : null}
          </div>
        ))}
      </div>
    </Modal>
  )
}

function App() {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#d97757',
          borderRadius: 8,
          fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
        },
      }}
    >
      <AntApp>
        <AppContent />
      </AntApp>
    </ConfigProvider>
  )
}

export default App
