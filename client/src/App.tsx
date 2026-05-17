import { useEffect, useMemo, useRef, useState } from 'react'
import AntApp from 'antd/es/app'
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
  ToolOutlined,
  UserOutlined,
} from '@ant-design/icons'
import TextArea from 'antd/es/input/TextArea'
import { requestChatCompletion, requestConfiguredModels, startRuntimeChat } from './api/chat'
import type { ChatRequest, ModelConfig } from './api/chat'
import './App.css'

const { Text, Title, Paragraph } = Typography

type ChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  createdAt: string
  provider?: string
}

const defaultConfig: ModelConfig = {
  protocol: 'openai',
  model: 'deepseek-v4-flash',
  url: 'https://api.deepseek.com',
}

const starterPrompts = [
  { label: 'Write', icon: <EditOutlined />, prompt: '帮我写一个 Kubernetes 故障排查 SOP 模板。' },
  { label: 'Learn', icon: <AppstoreOutlined />, prompt: '解释一下 agent runtime、tools、skills、MCP 的关系。' },
  { label: 'Code', icon: <CodeOutlined />, prompt: '帮我设计一个 Go runtime 的最小 HTTP + SSE API。' },
  { label: 'Ops', icon: <ToolOutlined />, prompt: '模拟一次订单服务错误率升高的排障对话。' },
]

const recentItems = ['Greeting', 'Agent runtime 方案', 'SSH 排障助手 MVP']

function nowIso() {
  return new Date().toISOString()
}

function greeting() {
  const hour = new Date().getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

function makeId(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function modelLabel(config: ModelConfig) {
  return config.model || 'Select model'
}

function buildChatRequest(messages: ChatMessage[], config: ModelConfig): ChatRequest {
  const payloadMessages = messages.flatMap((message) =>
    message.role === 'user' || message.role === 'assistant'
      ? [
          {
            role: message.role,
            content: message.content,
          },
        ]
      : [],
  )

  return {
    config,
    messages: payloadMessages,
  }
}

function AppContent() {
  const { message } = AntApp.useApp()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [config, setConfig] = useState<ModelConfig>(defaultConfig)
  const [models, setModels] = useState<string[]>([defaultConfig.model])
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [operationsOpen, setOperationsOpen] = useState(false)
  const [isSending, setIsSending] = useState(false)
  const [activeChatTitle, setActiveChatTitle] = useState('New chat')
  const viewportRef = useRef<HTMLDivElement | null>(null)

  const hasMessages = messages.length > 0

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
        setModels([defaultConfig.model])
      })
  }, [])

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
    setMessages([])
    setInput('')
    setActiveChatTitle('New chat')
    startRuntimeChat('New chat').catch((error) => {
      const reason = error instanceof Error ? error.message : String(error)
      message.error(reason)
    })
  }

  const sendMessage = async (text = input) => {
    const content = text.trim()
    if (!content || isSending) return

    const userMessage: ChatMessage = {
      id: makeId('user'),
      role: 'user',
      content,
      createdAt: nowIso(),
    }
    const nextMessages = [...messages, userMessage]
    setMessages(nextMessages)
    setInput('')
    setIsSending(true)
    if (!hasMessages) {
      setActiveChatTitle(content.length > 24 ? `${content.slice(0, 24)}...` : content)
    }
    scrollToBottom()

    try {
      const response = await requestChatCompletion(buildChatRequest(nextMessages, config))

      setMessages((current) => [
        ...current,
        {
          id: makeId('assistant'),
          role: 'assistant',
          content: response.content,
          createdAt: nowIso(),
          provider: response.provider,
        },
      ])
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      message.error(reason)
      setMessages((current) => [
        ...current,
        {
          id: makeId('assistant'),
          role: 'assistant',
          content: `请求失败：${reason}`,
          createdAt: nowIso(),
          provider: 'error',
        },
      ])
    } finally {
      setIsSending(false)
      scrollToBottom()
    }
  }

  const copyMessage = async (content: string) => {
    await navigator.clipboard.writeText(content)
    message.success('Copied')
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
          <Tooltip title="Export later">
            <Button type="text" size="small" icon={<ArrowDownOutlined />} />
          </Tooltip>
        </div>
      </aside>

      <main className="chat-main">
        <header className="chat-header">
          <Space>
            <Text strong>{activeChatTitle}</Text>
            <DownOutlined className="muted-icon" />
          </Space>
          <Space size={4}>
            <Tooltip title="Share later">
              <Button type="text" icon={<ShareAltOutlined />} />
            </Tooltip>
            <Tooltip title="Model settings">
              <Button type="text" icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)} />
            </Tooltip>
          </Space>
        </header>

        <div className="chat-viewport" ref={viewportRef}>
          {!hasMessages ? (
            <section className="welcome-pane">
              <Tag className="plan-pill">Local prototype</Tag>
              <Title className="welcome-title">
                <span className="brand-flower">*</span>
                {greeting()}, Agent Builder
              </Title>
              <Composer
                config={config}
                input={input}
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
            </section>
          ) : (
            <section className="thread">
              {messages.map((chatMessage) => (
                <article className={`message-row ${chatMessage.role}`} key={chatMessage.id}>
                  {chatMessage.role === 'assistant' ? (
                    <div className="assistant-mark">*</div>
                  ) : (
                    <div className="user-avatar">
                      <UserOutlined />
                    </div>
                  )}
                  <div className="message-body">
                    <div className="message-bubble">{chatMessage.content}</div>
                    {chatMessage.role === 'assistant' ? (
                      <Space className="message-actions" size={8}>
                        <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => copyMessage(chatMessage.content)} />
                        <Button type="text" size="small" icon={<ReloadOutlined />} />
                        {chatMessage.provider ? <Tag>{chatMessage.provider}</Tag> : null}
                      </Space>
                    ) : null}
                  </div>
                </article>
              ))}
            </section>
          )}
        </div>

        {hasMessages ? (
          <div className="composer-dock">
            <Composer
              config={config}
              input={input}
              isSending={isSending}
              modelItems={modelItems}
              onChange={setInput}
              onOpenSettings={() => setSettingsOpen(true)}
              onSend={() => sendMessage()}
            />
            <Text className="disclaimer">Agent Builder can make mistakes. Check important operations before execution.</Text>
          </div>
        ) : null}
      </main>

      <ModelSettingsDrawer config={config} open={settingsOpen} onClose={() => setSettingsOpen(false)} onSave={setConfig} />
      <OperationsPreview open={operationsOpen} onClose={() => setOperationsOpen(false)} />
    </div>
  )
}

type ComposerProps = {
  config: ModelConfig
  input: string
  isSending: boolean
  modelItems: MenuProps['items']
  onChange: (value: string) => void
  onOpenSettings: () => void
  onSend: () => void
}

function Composer({ config, input, isSending, modelItems, onChange, onOpenSettings, onSend }: ComposerProps) {
  return (
    <div className="composer">
      <TextArea
        autoSize={{ minRows: 2, maxRows: 7 }}
        className="composer-input"
        placeholder="How can I help you today?"
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
          <Tooltip title="Attach later">
            <Button type="text" icon={<PlusOutlined />} />
          </Tooltip>
          <Tooltip title="Tools later">
            <Button type="text" icon={<ToolOutlined />} />
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

type ModelSettingsDrawerProps = {
  config: ModelConfig
  open: boolean
  onClose: () => void
  onSave: (config: ModelConfig) => void
}

function ModelSettingsDrawer({ config, open, onClose, onSave }: ModelSettingsDrawerProps) {
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
          onClick={() => {
            const values = form.getFieldsValue()
            onSave(values)
            onClose()
          }}
        >
          Save
        </Button>
      }
    >
      <Paragraph type="secondary">
        Configure the LLM connection used by the local proxy. Keep only the connection fields here; choose the model from the composer.
      </Paragraph>
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
          <Input placeholder="https://api.deepseek.com" />
        </Form.Item>
        <Form.Item label="API key" name="apiKey">
          <Input.Password placeholder="Read from local config if empty" />
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

function OperationsPreview({ open, onClose }: { open: boolean; onClose: () => void }) {
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
          fontFamily:
            'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
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
