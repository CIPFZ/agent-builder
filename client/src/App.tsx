import { useEffect, useMemo, useRef, useState } from 'react'
import type { RefObject } from 'react'
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
  DeleteOutlined,
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
import type { TextAreaRef } from 'antd/es/input/TextArea'
import {
  addRuntimeSkillPath,
  cancelRuntimeTurn,
  createRuntimeSkill,
  deleteRuntimeSession,
  decideRuntimePermission,
  loadModelConfig,
  requestConfiguredModels,
  requestRuntimeEvents,
  requestRuntimeEventsEndpoint,
  requestRuntimeCapabilities,
  requestRuntimeAudit,
  requestRuntimeMessages,
  requestRuntimeMcpServers,
  requestRuntimeMcpPrompts,
  requestRuntimeMcpResources,
  requestRuntimeMcpTools,
  requestRuntimePermissions,
  requestRuntimeSessionAudit,
  requestRuntimeSessionMessages,
  requestRuntimeSessions,
  requestRuntimeSkills,
  requestRuntimeStatus,
  refreshRuntimeMcpServer,
  refreshRuntimeSkills,
  renameRuntimeSession,
  saveModelConfig,
  saveRuntimeMcpServer,
  selectRuntimeSession,
  sendRuntimePrompt,
  setRuntimeMcpServerEnabled,
  setRuntimeMcpToolEnabled,
  setRuntimeSkillEnabled,
  startRuntimeChat,
  verifyModelConfig,
} from './api/chat'
import type { ModelConfig } from './api/chat'
import { subscribeRuntimeEvents } from './runtime/events'
import type {
  RuntimeEvent,
  RuntimeCapability,
  RuntimeAuditEvent,
  RuntimeMessage,
  RuntimeMessagePart,
  RuntimeMcpServer,
  RuntimeMcpServerConfig,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpTool,
  RuntimeModelVerifyResponse,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeSession,
  RuntimeSkill,
  RuntimeSkillCreateRequest,
  RuntimeStatus,
} from './runtime'
import './App.css'

const { Text, Title, Paragraph } = Typography

const defaultConfig: ModelConfig = {
  protocol: 'openai',
  model: '',
  url: '',
  models: [],
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
  const [sessions, setSessions] = useState<RuntimeSession[]>([])
  const [skills, setSkills] = useState<RuntimeSkill[]>([])
  const [mcpServers, setMcpServers] = useState<RuntimeMcpServer[]>([])
  const [mcpToolsByServer, setMcpToolsByServer] = useState<Record<string, RuntimeMcpTool[]>>({})
  const [mcpResourcesByServer, setMcpResourcesByServer] = useState<Record<string, RuntimeMcpResource[]>>({})
  const [mcpPromptsByServer, setMcpPromptsByServer] = useState<Record<string, RuntimeMcpPrompt[]>>({})
  const [capabilities, setCapabilities] = useState<RuntimeCapability[]>([])
  const [input, setInput] = useState('')
  const [config, setConfig] = useState<ModelConfig>(defaultConfig)
  const [models, setModels] = useState<string[]>([defaultConfig.model])
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)
  const [settingsVerifying, setSettingsVerifying] = useState(false)
  const [modelSwitching, setModelSwitching] = useState(false)
  const [operationsOpen, setOperationsOpen] = useState(false)
  const [isSending, setIsSending] = useState(false)
  const [configLoaded, setConfigLoaded] = useState(false)
  const [lastError, setLastError] = useState('')
  const [activeChatTitle, setActiveChatTitle] = useState('New chat')
  const [runtimeStatus, setRuntimeStatus] = useState<RuntimeStatus | null>(null)
  const [auditEvents, setAuditEvents] = useState<RuntimeAuditEvent[]>([])
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const composerInputRef = useRef<TextAreaRef | null>(null)
  const activeSessionIdRef = useRef<string>('')

  const hasMessages = messages.length > 0
  const isModelConfigured = Boolean(config.url && config.model && (config.hasApiKey || config.apiKey))
  const activeSession = useMemo(
    () => sessions.find((session) => session.active) ?? sessions.find((session) => session.id === runtimeStatus?.sessionId),
    [runtimeStatus?.sessionId, sessions],
  )

  useEffect(() => {
    activeSessionIdRef.current = runtimeStatus?.sessionId ?? ''
  }, [runtimeStatus?.sessionId])

  const refreshMessages = async () => {
    const activeID = activeSessionIdRef.current
    const runtimeMessages = activeID
      ? await requestRuntimeSessionMessages(activeID)
      : await requestRuntimeMessages()
    setMessages(runtimeMessages)
  }

  const loadMessagesForSession = async (sessionId?: string) => {
    const runtimeMessages = sessionId
      ? await requestRuntimeSessionMessages(sessionId)
      : await requestRuntimeMessages()
    setMessages(runtimeMessages)
    return runtimeMessages
  }

  const refreshSessions = async () => {
    const nextSessions = await requestRuntimeSessions()
    setSessions(nextSessions)
    const current = nextSessions.find((session) => session.active)
    if (current) {
      setActiveChatTitle(current.title)
    }
    return nextSessions
  }

  const refreshStatus = async () => {
    const nextStatus = await requestRuntimeStatus()
    activeSessionIdRef.current = nextStatus.sessionId
    setRuntimeStatus(nextStatus)
    return nextStatus
  }

  const refreshPermissions = async () => {
    const nextPermissions = await requestRuntimePermissions()
    setPermissions(nextPermissions)
    return nextPermissions
  }

  const refreshRuntimeInventory = async () => {
    const [nextSkills, nextMcpServers, nextCapabilities] = await Promise.all([
      requestRuntimeSkills(),
      requestRuntimeMcpServers(),
      requestRuntimeCapabilities(),
    ])
    setSkills(nextSkills)
    setMcpServers(nextMcpServers)
    setCapabilities(nextCapabilities)
  }

  const refreshMcpTools = async (server: string) => {
    const [tools, resources, prompts] = await Promise.all([
      requestRuntimeMcpTools(server),
      requestRuntimeMcpResources(server),
      requestRuntimeMcpPrompts(server),
    ])
    setMcpToolsByServer((current) => ({ ...current, [server]: tools }))
    setMcpResourcesByServer((current) => ({ ...current, [server]: resources }))
    setMcpPromptsByServer((current) => ({ ...current, [server]: prompts }))
    return tools
  }

  const refreshAudit = async (turnId?: string) => {
    const activeID = activeSessionIdRef.current
    const id = turnId || runtimeStatus?.requests?.activeRequestId || [...events].reverse().find((event) => event.turn_id)?.turn_id
    const nextAudit = id
      ? await requestRuntimeAudit(id)
      : activeID
        ? await requestRuntimeSessionAudit(activeID)
        : []
    setAuditEvents(nextAudit)
  }

  const openOperations = () => {
    setOperationsOpen(true)
    refreshAudit().catch(() => undefined)
    refreshRuntimeInventory().catch(() => undefined)
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
    Promise.all([
      requestRuntimeStatus(),
      requestRuntimeSessions(),
      requestRuntimePermissions(),
      requestRuntimeEvents(),
      requestRuntimeSkills(),
      requestRuntimeMcpServers(),
      requestRuntimeCapabilities(),
    ])
      .then(async ([nextStatus, runtimeSessions, runtimePermissions, runtimeEvents, runtimeSkills, runtimeMcpServers, runtimeCapabilities]) => {
        if (cancelled) return
        const runtimeMessages = nextStatus.sessionId
          ? await requestRuntimeSessionMessages(nextStatus.sessionId)
          : await requestRuntimeMessages()
        if (cancelled) return
        activeSessionIdRef.current = nextStatus.sessionId
        setRuntimeStatus(nextStatus)
        setSessions(runtimeSessions)
        setActiveChatTitle(runtimeSessions.find((session) => session.active)?.title ?? 'New chat')
        setMessages(runtimeMessages)
        setPermissions(runtimePermissions)
        setEvents(runtimeEvents)
        setSkills(runtimeSkills)
        setMcpServers(runtimeMcpServers)
        setCapabilities(runtimeCapabilities)
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
            if (event.type === 'message.created' || event.type === 'message.updated' || event.type === 'message.completed') {
              refreshMessages().catch(() => undefined)
              refreshStatus().catch(() => undefined)
              refreshSessions().catch(() => undefined)
            }
            if (event.type === 'session.created' || event.type === 'session.updated' || event.type === 'session.deleted') {
              refreshSessions().catch(() => undefined)
            }
            if (event.type === 'permission.requested') {
              refreshPermissions().catch(() => undefined)
              refreshStatus().catch(() => undefined)
            }
            if (event.type.startsWith('skill.') || event.type.startsWith('mcp.')) {
              refreshRuntimeInventory().catch(() => undefined)
            }
            if (event.type === 'audit.recorded') {
              refreshAudit(event.turn_id).catch(() => undefined)
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

  const switchModel = async (modelName: string) => {
    if (modelName === config.model || modelSwitching) return
    setModelSwitching(true)
    try {
      const saved = await saveModelConfig({ ...config, model: modelName })
      setConfig((current) => ({ ...current, ...saved }))
      setModels(saved.models?.length ? saved.models : saved.model ? [saved.model] : [])
      setMessages([])
      setPermissions([])
      setAuditEvents([])
      const nextStatus = await refreshStatus().catch(() => undefined)
      await refreshSessions().catch(() => undefined)
      if (nextStatus?.sessionId) {
        setMessages(await requestRuntimeSessionMessages(nextStatus.sessionId).catch(() => []))
      }
      message.success(`Model switched to ${saved.model}`)
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
    } finally {
      setModelSwitching(false)
    }
  }

  const modelItems = useMemo<MenuProps['items']>(
    () =>
      models.map((modelName) => ({
        key: modelName,
        label: modelName,
        onClick: () => {
          void switchModel(modelName)
        },
      })),
    [config, modelSwitching, models],
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
    startRuntimeChat('New chat')
      .then((nextStatus) => {
        activeSessionIdRef.current = nextStatus.sessionId
        setRuntimeStatus(nextStatus)
        setMessages([])
        setPermissions([])
        setAuditEvents([])
        setActiveChatTitle('New chat')
        refreshSessions().catch(() => undefined)
        refreshRuntimeInventory().catch(() => undefined)
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
    if (activeSession?.title === 'New chat' || !activeSession?.title) {
      const previewTitle = content.length > 28 ? `${content.slice(0, 28)}...` : content
      setActiveChatTitle(previewTitle)
      if (runtimeStatus?.sessionId) {
        renameRuntimeSession(runtimeStatus.sessionId, previewTitle).catch(() => undefined)
      }
    }
    try {
      const targetSessionId = runtimeStatus?.sessionId
      if (!targetSessionId) {
        throw new Error('No active runtime session. Create or select a session before sending a message.')
      }
      const previousAssistantId = [...messages].reverse().find((chatMessage) => chatMessage.role === 'assistant')?.id
      await sendRuntimePrompt(content, targetSessionId)
      let runtimeMessages = await loadMessagesForSession(targetSessionId)
      setMessages(runtimeMessages)
      await refreshPermissions().catch(() => undefined)
      let nextStatus = await refreshStatus().catch(() => runtimeStatus)
      const started = Date.now()
      while (Date.now() - started < 30 * 60 * 1000) {
        await new Promise((resolve) => window.setTimeout(resolve, 700))
        runtimeMessages = await requestRuntimeSessionMessages(targetSessionId).catch(() => runtimeMessages)
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
      runtimeMessages = await requestRuntimeSessionMessages(targetSessionId).catch(() => runtimeMessages)
      setMessages(runtimeMessages)
      await refreshPermissions().catch(() => undefined)
      await refreshStatus().catch(() => undefined)
      await refreshSessions().catch(() => undefined)
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

  const selectSession = async (sessionId: string) => {
    if (sessionId === runtimeStatus?.sessionId) return
    try {
      const nextStatus = await selectRuntimeSession(sessionId)
      activeSessionIdRef.current = nextStatus.sessionId
      setRuntimeStatus(nextStatus)
      const [runtimeMessages, runtimePermissions, nextAudit] = await Promise.all([
        requestRuntimeSessionMessages(sessionId),
        requestRuntimePermissions(),
        requestRuntimeSessionAudit(sessionId).catch(() => []),
      ])
      setMessages(runtimeMessages)
      setPermissions(runtimePermissions)
      setAuditEvents(nextAudit)
      await refreshSessions().catch(() => undefined)
      scrollToBottom()
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
    }
  }

  const renameSession = async (session: RuntimeSession) => {
    const title = window.prompt('Rename session', session.title)?.trim()
    if (!title || title === session.title) return
    try {
      const nextSessions = await renameRuntimeSession(session.id, title)
      setSessions(nextSessions)
      if (session.active || session.id === runtimeStatus?.sessionId) {
        setActiveChatTitle(title)
      }
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
    }
  }

  const deleteSession = async (session: RuntimeSession) => {
    if (!window.confirm(`Delete session "${session.title}"?`)) return
    try {
      const nextSessions = await deleteRuntimeSession(session.id)
      setSessions(nextSessions)
      const nextActive = nextSessions.find((item) => item.active) ?? nextSessions[0]
      if (nextActive) {
        const nextStatus = await selectRuntimeSession(nextActive.id)
        activeSessionIdRef.current = nextStatus.sessionId
        setRuntimeStatus(nextStatus)
        setActiveChatTitle(nextActive.title)
        setMessages(await requestRuntimeSessionMessages(nextActive.id))
        setAuditEvents(await requestRuntimeSessionAudit(nextActive.id).catch(() => []))
      } else {
        activeSessionIdRef.current = ''
        setMessages([])
        setAuditEvents([])
        setActiveChatTitle('New chat')
      }
    } catch (error) {
      const reason = error instanceof Error ? error.message : String(error)
      setLastError(reason)
      message.error(reason)
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
            <button className="mode-tab" type="button" onClick={openOperations}>
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
            <button className="nav-item" type="button" onClick={openOperations}>
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
            {sessions.length === 0 ? (
              <Text className="empty-recents" type="secondary">No saved sessions</Text>
            ) : (
              sessions.map((session) => (
                <div className={session.active ? 'recent-row active' : 'recent-row'} key={session.id}>
                  <button className="recent-item" type="button" onClick={() => selectSession(session.id)}>
                    <span className="recent-title">{session.title || 'New chat'}</span>
                    <span className="recent-meta">{session.messageCount} messages</span>
                  </button>
                  <Dropdown
                    menu={{
                      items: [
                        {
                          key: 'rename',
                          icon: <EditOutlined />,
                          label: 'Rename',
                          onClick: () => renameSession(session),
                        },
                        {
                          key: 'delete',
                          danger: true,
                          icon: <DeleteOutlined />,
                          label: 'Delete',
                          onClick: () => deleteSession(session),
                        },
                      ],
                    }}
                    trigger={['click']}
                  >
                    <Button type="text" size="small" icon={<DownOutlined />} onClick={(event) => event.stopPropagation()} />
                  </Dropdown>
                </div>
              ))
            )}
          </div>

        <div className="sidebar-footer">
          <Space>
            <span className="avatar-dot">A</span>
            <span>Agent Builder</span>
            <Text type="secondary">Local</Text>
          </Space>
          <Tooltip title="Runtime logs">
            <Button type="text" size="small" icon={<ArrowDownOutlined />} onClick={openOperations} />
          </Tooltip>
        </div>
      </aside>

      <main className="chat-main">
        <header className="chat-header">
          <Space>
            <Text strong>{activeSession?.title ?? activeChatTitle}</Text>
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
              <Button type="text" icon={<ShareAltOutlined />} onClick={openOperations} />
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
                isSending={isSending || modelSwitching}
                modelItems={modelItems}
                onChange={setInput}
                inputRef={composerInputRef}
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
              isSending={isSending || modelSwitching}
              modelItems={modelItems}
              onChange={setInput}
              inputRef={composerInputRef}
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
        verifying={settingsVerifying}
        onSave={async (nextConfig) => {
          setSettingsSaving(true)
          try {
            const saved = await saveModelConfig(nextConfig)
            setConfig((current) => ({ ...current, ...saved }))
            setModels(saved.models?.length ? saved.models : saved.model ? [saved.model] : [])
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
        onVerify={async (nextConfig) => {
          setSettingsVerifying(true)
          try {
            const result = await verifyModelConfig(nextConfig)
            if (result.ok) {
              const nextModels = result.models?.length ? result.models : result.model ? [result.model] : []
              if (nextModels.length > 0) {
                setModels(nextModels)
                setConfig((current) => ({ ...current, models: nextModels, model: result.model || nextModels[0] }))
              }
              message.success(`Verified ${result.model || nextModels[0]}`)
            } else {
              message.error(result.error || 'Model verification failed')
            }
            return result
          } catch (error) {
            const reason = error instanceof Error ? error.message : String(error)
            message.error(reason)
            throw error
          } finally {
            setSettingsVerifying(false)
          }
        }}
      />
      <PermissionReviewModal permissions={permissions} onDecide={decidePermission} />
      <OperationsPreview
        capabilities={capabilities}
        auditEvents={auditEvents}
        events={events}
        mcpServers={mcpServers}
        mcpResourcesByServer={mcpResourcesByServer}
        mcpPromptsByServer={mcpPromptsByServer}
        mcpToolsByServer={mcpToolsByServer}
        open={operationsOpen}
        skills={skills}
        onEditMcpServer={async (config) => {
          const nextServers = await saveRuntimeMcpServer(config)
          setMcpServers(nextServers)
          await refreshRuntimeInventory()
          message.success('MCP server saved')
        }}
        onRefreshAudit={() => refreshAudit()}
        onRefreshMcpServer={async (server) => {
          const nextServers = await refreshRuntimeMcpServer(server)
          setMcpServers(nextServers)
          await refreshMcpTools(server).catch(() => undefined)
          await refreshRuntimeInventory()
          message.success('MCP server refreshed')
        }}
        onRefreshSkills={async () => {
          const nextSkills = await refreshRuntimeSkills()
          setSkills(nextSkills)
          await refreshRuntimeInventory()
          message.success('Skills refreshed')
        }}
        onCreateSkill={async (request) => {
          const nextSkills = await createRuntimeSkill(request)
          setSkills(nextSkills)
          await refreshRuntimeInventory()
          message.success('Skill created')
        }}
        onAddSkillPath={async (path) => {
          const nextSkills = await addRuntimeSkillPath(path)
          setSkills(nextSkills)
          await refreshRuntimeInventory()
          message.success('Skill path added')
        }}
        onToggleMcpServer={async (server, enabled) => {
          const nextServers = await setRuntimeMcpServerEnabled(server, enabled)
          setMcpServers(nextServers)
          await refreshRuntimeInventory()
          message.success(enabled ? 'MCP server enabled' : 'MCP server disabled')
        }}
        onToggleMcpTool={async (server, tool, enabled) => {
          const nextTools = await setRuntimeMcpToolEnabled(server, tool, enabled)
          setMcpToolsByServer((current) => ({ ...current, [server]: nextTools }))
          await refreshRuntimeInventory()
          message.success(enabled ? 'MCP tool allowed' : 'MCP tool denied')
        }}
        onToggleSkill={async (name, enabled) => {
          const nextSkills = await setRuntimeSkillEnabled(name, enabled)
          setSkills(nextSkills)
          await refreshRuntimeInventory()
          message.success(enabled ? 'Skill enabled' : 'Skill disabled')
        }}
        onViewMcpTools={(server) => refreshMcpTools(server)}
        onClose={() => setOperationsOpen(false)}
      />
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
  inputRef: RefObject<TextAreaRef | null>
  isDisabled: boolean
  isSending: boolean
  modelItems: MenuProps['items']
  onChange: (value: string) => void
  onOpenSettings: () => void
  onSend: () => void
}

function Composer({ config, input, inputRef, isDisabled, isSending, modelItems, onChange, onOpenSettings, onSend }: ComposerProps) {
  return (
    <div className="composer" onClick={() => inputRef.current?.focus()}>
      <TextArea
        aria-label="Message composer"
        autoSize={{ minRows: 2, maxRows: 7 }}
        className="composer-input"
        ref={inputRef}
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
  verifying: boolean
  onClose: () => void
  onSave: (config: ModelConfig) => Promise<void>
  onVerify: (config: ModelConfig) => Promise<RuntimeModelVerifyResponse>
}

function ModelSettingsDrawer({ config, open, saving, verifying, onClose, onSave, onVerify }: ModelSettingsDrawerProps) {
  const [form] = Form.useForm<ModelConfig>()
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const modelOptions = ((availableModels.length ? availableModels : undefined) ??
    (config.models?.length ? config.models : undefined) ??
    (config.model ? [config.model] : [])).map((model) => ({
    value: model,
    label: model,
  }))

  return (
    <Drawer
      title="Model settings"
      placement="right"
      size={420}
      open={open}
      onClose={onClose}
      afterOpenChange={(visible) => {
        if (visible) {
          form.setFieldsValue(config)
          setAvailableModels(config.models?.length ? config.models : config.model ? [config.model] : [])
        }
      }}
      extra={
        <Space>
          <Button
            loading={verifying}
            onClick={() => {
              form.validateFields().then(async (values) => {
                const result = await onVerify(values)
                const nextModels = result.models?.length ? result.models : result.model ? [result.model] : []
                if (result.ok && nextModels.length > 0) {
                  setAvailableModels(nextModels)
                  form.setFieldsValue({ model: result.model || nextModels[0] })
                }
              })
            }}
          >
            Verify
          </Button>
          <Button
            type="primary"
            loading={saving}
            onClick={() => {
              form.validateFields().then((values) => onSave(values))
            }}
          >
            Save
          </Button>
        </Space>
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
        <Form.Item label="Model" name="model">
          <Select
            showSearch
            placeholder="Model list is fetched after verification or save"
            options={modelOptions}
          />
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

function OperationsPreview({
  auditEvents,
  capabilities,
  events,
  mcpServers,
  mcpResourcesByServer,
  mcpPromptsByServer,
  mcpToolsByServer,
  open,
  skills,
  onEditMcpServer,
  onRefreshAudit,
  onRefreshMcpServer,
  onRefreshSkills,
  onCreateSkill,
  onAddSkillPath,
  onToggleMcpServer,
  onToggleMcpTool,
  onToggleSkill,
  onViewMcpTools,
  onClose,
}: {
  auditEvents: RuntimeAuditEvent[]
  capabilities: RuntimeCapability[]
  events: RuntimeEvent[]
  mcpServers: RuntimeMcpServer[]
  mcpResourcesByServer: Record<string, RuntimeMcpResource[]>
  mcpPromptsByServer: Record<string, RuntimeMcpPrompt[]>
  mcpToolsByServer: Record<string, RuntimeMcpTool[]>
  open: boolean
  skills: RuntimeSkill[]
  onEditMcpServer: (config: RuntimeMcpServerConfig) => Promise<void>
  onRefreshAudit: () => void
  onRefreshMcpServer: (server: string) => Promise<void>
  onRefreshSkills: () => Promise<void>
  onCreateSkill: (request: RuntimeSkillCreateRequest) => Promise<void>
  onAddSkillPath: (path: string) => Promise<void>
  onToggleMcpServer: (server: string, enabled: boolean) => Promise<void>
  onToggleMcpTool: (server: string, tool: string, enabled: boolean) => Promise<void>
  onToggleSkill: (name: string, enabled: boolean) => Promise<void>
  onViewMcpTools: (server: string) => Promise<RuntimeMcpTool[]>
  onClose: () => void
}) {
  const enabledSkills = skills.filter((skill) => skill.enabled).length
  const connectedMcp = mcpServers.filter((server) => server.state === 'connected').length
  const enabledCapabilities = capabilities.filter((capability) => capability.enabled).length

  return (
    <Modal title="Runtime details" open={open} onCancel={onClose} footer={<Button onClick={onClose}>Close</Button>} width={820}>
      <div className="runtime-summary-grid">
        <div className="runtime-summary-item">
          <Text type="secondary">Capabilities</Text>
          <Title level={4}>{enabledCapabilities}</Title>
        </div>
        <div className="runtime-summary-item">
          <Text type="secondary">Skills</Text>
          <Title level={4}>
            {enabledSkills}/{skills.length}
          </Title>
        </div>
        <div className="runtime-summary-item">
          <Text type="secondary">MCP</Text>
          <Title level={4}>
            {connectedMcp}/{mcpServers.length}
          </Title>
        </div>
      </div>
      <Collapse
        className="runtime-collapse"
        ghost
        items={[
          {
            key: 'audit',
            label: 'Audit',
            children: <RuntimeAuditList events={auditEvents} onRefresh={onRefreshAudit} />,
          },
          {
            key: 'skills',
            label: 'Skills',
            children: (
              <RuntimeSkillList
                skills={skills}
                onRefresh={onRefreshSkills}
                onCreate={onCreateSkill}
                onAddPath={onAddSkillPath}
                onToggle={onToggleSkill}
              />
            ),
          },
          {
            key: 'mcp',
            label: 'MCP servers',
            children: (
              <RuntimeMcpManager
                servers={mcpServers}
                resourcesByServer={mcpResourcesByServer}
                promptsByServer={mcpPromptsByServer}
                toolsByServer={mcpToolsByServer}
                onEdit={onEditMcpServer}
                onRefresh={onRefreshMcpServer}
                onToggle={onToggleMcpServer}
                onToggleTool={onToggleMcpTool}
                onViewTools={onViewMcpTools}
              />
            ),
          },
          {
            key: 'capabilities',
            label: 'Capabilities',
            children: <RuntimeCapabilityList capabilities={capabilities} />,
          },
        ]}
      />
      <div className="event-log">
        {events.slice(-8).map((event) => (
          <div className="event-log-row" key={event.id || `${event.created_at}-${event.type}`}>
            <Tag>{event.type}</Tag>
            {typeof event.payload?.role === 'string' ? <Text type="secondary">{event.payload.role}</Text> : null}
            {typeof event.payload?.summary === 'string' ? <Text>{event.payload.summary}</Text> : null}
          </div>
        ))}
      </div>
    </Modal>
  )
}

function RuntimeAuditList({ events, onRefresh }: { events: RuntimeAuditEvent[]; onRefresh: () => void }) {
  return (
    <div className="runtime-list">
      <Button size="small" onClick={onRefresh}>
        Refresh audit
      </Button>
      {events.length === 0 ? <Text type="secondary">No audit events for the active turn.</Text> : null}
      {events.slice(-10).map((event) => (
        <div className="runtime-list-row" key={event.id}>
          <Space size={8}>
            <Tag>{event.type}</Tag>
            <Text strong>{event.turn_id}</Text>
          </Space>
          <pre className="part-preview">{JSON.stringify(event.payload, null, 2)}</pre>
        </div>
      ))}
    </div>
  )
}

function RuntimeSkillList({
  skills,
  onRefresh,
  onCreate,
  onAddPath,
  onToggle,
}: {
  skills: RuntimeSkill[]
  onRefresh: () => Promise<void>
  onCreate: (request: RuntimeSkillCreateRequest) => Promise<void>
  onAddPath: (path: string) => Promise<void>
  onToggle: (name: string, enabled: boolean) => Promise<void>
}) {
  const [createOpen, setCreateOpen] = useState(false)
  const [pathOpen, setPathOpen] = useState(false)
  const [skillForm] = Form.useForm<RuntimeSkillCreateRequest>()
  const [pathForm] = Form.useForm<{ path: string }>()

  return (
    <div className="runtime-list">
      <Space wrap>
        <Button size="small" icon={<ReloadOutlined />} onClick={() => onRefresh()}>
          Refresh skills
        </Button>
        <Button size="small" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          Create skill
        </Button>
        <Button size="small" onClick={() => setPathOpen(true)}>
          Add path
        </Button>
      </Space>
      {skills.length === 0 ? <Text type="secondary">No skills discovered.</Text> : null}
      {skills.slice(0, 12).map((skill) => (
        <div className="runtime-list-row" key={`${skill.name}-${skill.path ?? ''}`}>
          <Space size={8}>
            <Tag color={skill.enabled ? 'green' : 'default'}>{skill.enabled ? 'enabled' : 'disabled'}</Tag>
            <Text strong>{skill.name}</Text>
            {skill.builtin ? <Tag>builtin</Tag> : null}
            <Button size="small" onClick={() => onToggle(skill.name, !skill.enabled)}>
              {skill.enabled ? 'Disable' : 'Enable'}
            </Button>
          </Space>
          {skill.error ? <Text type="danger">{skill.error}</Text> : <Text type="secondary">{skill.description}</Text>}
        </div>
      ))}
      <Modal
        title="Create skill"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => {
          skillForm
            .validateFields()
            .then((values) => onCreate(values))
            .then(() => {
              skillForm.resetFields()
              setCreateOpen(false)
            })
            .catch(() => undefined)
        }}
      >
        <Form form={skillForm} layout="vertical" initialValues={{ directory: '.agents/skills' }}>
          <Form.Item label="Name" name="name" rules={[{ required: true }]}>
            <Input placeholder="my-skill" />
          </Form.Item>
          <Form.Item label="Directory" name="directory">
            <Input placeholder=".agents/skills" />
          </Form.Item>
          <Form.Item label="Description" name="description" rules={[{ required: true }]}>
            <Input placeholder="Use when..." />
          </Form.Item>
          <Form.Item label="Instructions" name="instructions" rules={[{ required: true }]}>
            <TextArea rows={6} placeholder="# My Skill&#10;&#10;Steps..." />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title="Add skill path"
        open={pathOpen}
        onCancel={() => setPathOpen(false)}
        onOk={() => {
          pathForm
            .validateFields()
            .then(({ path }) => onAddPath(path))
            .then(() => {
              pathForm.resetFields()
              setPathOpen(false)
            })
            .catch(() => undefined)
        }}
      >
        <Form form={pathForm} layout="vertical">
          <Form.Item label="Path" name="path" rules={[{ required: true }]}>
            <Input placeholder=".agents/skills" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

export function RuntimeMcpList({ servers }: { servers: RuntimeMcpServer[] }) {
  if (servers.length === 0) return <Text type="secondary">No MCP servers configured.</Text>
  return (
    <div className="runtime-list">
      {servers.map((server) => (
        <div className="runtime-list-row" key={server.name}>
          <Space size={8}>
            <Tag color={server.state === 'connected' ? 'green' : server.state === 'error' ? 'red' : 'default'}>{server.state}</Tag>
            <Text strong>{server.name}</Text>
            <Tag>{server.type}</Tag>
          </Space>
          <Text type="secondary">
            tools {server.counts.tools} · prompts {server.counts.prompts} · resources {server.counts.resources}
          </Text>
          {server.error ? <Text type="danger">{server.error}</Text> : null}
        </div>
      ))}
    </div>
  )
}

function RuntimeMcpManager({
  servers,
  resourcesByServer,
  promptsByServer,
  toolsByServer,
  onEdit,
  onRefresh,
  onToggle,
  onToggleTool,
  onViewTools,
}: {
  servers: RuntimeMcpServer[]
  resourcesByServer: Record<string, RuntimeMcpResource[]>
  promptsByServer: Record<string, RuntimeMcpPrompt[]>
  toolsByServer: Record<string, RuntimeMcpTool[]>
  onEdit: (config: RuntimeMcpServerConfig) => Promise<void>
  onRefresh: (server: string) => Promise<void>
  onToggle: (server: string, enabled: boolean) => Promise<void>
  onToggleTool: (server: string, tool: string, enabled: boolean) => Promise<void>
  onViewTools: (server: string) => Promise<RuntimeMcpTool[]>
}) {
  const [editing, setEditing] = useState<RuntimeMcpServer | null>(null)
  const [form] = Form.useForm<RuntimeMcpServerConfig & { argsText?: string; envText?: string; headersText?: string }>()

  const openEditor = (server?: RuntimeMcpServer) => {
    const next = server ?? ({ name: '', type: 'http', disabled: false, state: 'disabled', counts: { tools: 0, prompts: 0, resources: 0 } } as RuntimeMcpServer)
    setEditing(next)
    form.setFieldsValue({
      name: next.name,
      type: next.type,
      url: next.url ?? '',
      command: next.command ?? '',
      disabled: next.disabled,
      argsText: (next.args ?? []).join('\n'),
      envText: mapToLines(next.env),
      headersText: mapToLines(next.headers),
      enabled_tools: next.enabled_tools ?? [],
      disabled_tools: next.disabled_tools ?? [],
    })
  }

  return (
    <div className="runtime-list">
      <Button size="small" icon={<PlusOutlined />} onClick={() => openEditor()}>
        Add MCP server
      </Button>
      {servers.length === 0 ? <Text type="secondary">No MCP servers configured.</Text> : null}
      {servers.map((server) => (
        <div className="runtime-list-row" key={server.name}>
          <Space size={8}>
            <Tag color={server.state === 'connected' ? 'green' : server.state === 'error' ? 'red' : 'default'}>{server.state}</Tag>
            <Text strong>{server.name}</Text>
            <Tag>{server.type}</Tag>
            <Button size="small" onClick={() => onToggle(server.name, server.disabled)}>
              {server.disabled ? 'Enable' : 'Disable'}
            </Button>
            <Button size="small" icon={<ReloadOutlined />} onClick={() => onRefresh(server.name)} />
            <Button size="small" onClick={() => openEditor(server)}>
              Edit
            </Button>
            <Button size="small" onClick={() => onViewTools(server.name)}>
              Tools
            </Button>
          </Space>
          <Text type="secondary">
            tools {server.counts.tools} / prompts {server.counts.prompts} / resources {server.counts.resources}
          </Text>
          {server.error ? <Text type="danger">{server.error}</Text> : null}
          {(toolsByServer[server.name] ?? []).map((tool) => (
            <div className="runtime-list-row compact" key={`${server.name}-${tool.name}`}>
              <Space size={8}>
                <Tag color={tool.enabled ? 'green' : 'default'}>{tool.enabled ? 'allowed' : 'denied'}</Tag>
                <Text>{tool.name}</Text>
                <Button size="small" onClick={() => onToggleTool(server.name, tool.name, !tool.enabled)}>
                  {tool.enabled ? 'Deny' : 'Allow'}
                </Button>
              </Space>
              {tool.description ? <Text type="secondary">{tool.description}</Text> : null}
            </div>
          ))}
          {(resourcesByServer[server.name] ?? []).map((resource) => (
            <div className="runtime-list-row compact" key={`${server.name}-${resource.uri}`}>
              <Space size={8}>
                <Tag>resource</Tag>
                <Text>{resource.name || resource.uri}</Text>
              </Space>
              {resource.description ? <Text type="secondary">{resource.description}</Text> : <Text type="secondary">{resource.uri}</Text>}
            </div>
          ))}
          {(promptsByServer[server.name] ?? []).map((prompt) => (
            <div className="runtime-list-row compact" key={`${server.name}-${prompt.name}`}>
              <Space size={8}>
                <Tag>prompt</Tag>
                <Text>{prompt.name}</Text>
              </Space>
              {prompt.description ? <Text type="secondary">{prompt.description}</Text> : null}
            </div>
          ))}
        </div>
      ))}
      <Modal
        title="MCP server"
        open={editing !== null}
        onCancel={() => setEditing(null)}
        onOk={() => {
          form.validateFields().then(async (values) => {
            await onEdit({
              name: values.name,
              type: values.type,
              url: values.url,
              command: values.command,
              disabled: values.disabled,
              args: linesToList(values.argsText),
              env: linesToMap(values.envText),
              headers: linesToMap(values.headersText),
              enabled_tools: values.enabled_tools,
              disabled_tools: values.disabled_tools,
            })
            setEditing(null)
          })
        }}
      >
        <Form form={form} layout="vertical">
          <Form.Item label="Name" name="name" rules={[{ required: true }]}>
            <Input disabled={Boolean(editing?.name)} placeholder="docs" />
          </Form.Item>
          <Form.Item label="Type" name="type" rules={[{ required: true }]}>
            <Select options={['http', 'sse', 'stdio'].map((value) => ({ label: value, value }))} />
          </Form.Item>
          <Form.Item label="URL" name="url">
            <Input placeholder="https://example.com/mcp" />
          </Form.Item>
          <Form.Item label="Command" name="command">
            <Input placeholder="npx" />
          </Form.Item>
          <Form.Item label="Args" name="argsText">
            <TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder={'--yes\n@modelcontextprotocol/server'} />
          </Form.Item>
          <Form.Item label="Env" name="envText">
            <TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="API_TOKEN=$MCP_TOKEN" />
          </Form.Item>
          <Form.Item label="Headers" name="headersText">
            <TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="Authorization=Bearer $MCP_TOKEN" />
          </Form.Item>
          <Form.Item label="Enabled tools" name="enabled_tools">
            <Select mode="tags" tokenSeparators={[',']} />
          </Form.Item>
          <Form.Item label="Disabled tools" name="disabled_tools">
            <Select mode="tags" tokenSeparators={[',']} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}

function mapToLines(values?: Record<string, string>) {
  if (!values) return ''
  return Object.entries(values)
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

function linesToList(value?: string) {
  return (value ?? '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
}

function linesToMap(value?: string) {
  const result: Record<string, string> = {}
  for (const line of linesToList(value)) {
    const index = line.indexOf('=')
    if (index <= 0) continue
    result[line.slice(0, index).trim()] = line.slice(index + 1)
  }
  return Object.keys(result).length > 0 ? result : undefined
}

function RuntimeCapabilityList({ capabilities }: { capabilities: RuntimeCapability[] }) {
  if (capabilities.length === 0) return <Text type="secondary">No capabilities available.</Text>
  return (
    <div className="runtime-list">
      {capabilities.slice(0, 18).map((capability) => (
        <div className="runtime-list-row compact" key={capability.id}>
          <Space size={8}>
            <Tag>{capability.kind}</Tag>
            <Text strong>{capability.name}</Text>
            <Tag color={capability.enabled ? 'green' : 'default'}>{capability.enabled ? 'on' : 'off'}</Tag>
            <Tag>{capability.risk}</Tag>
          </Space>
          {capability.source ? <Text type="secondary">{capability.source}</Text> : null}
        </div>
      ))}
    </div>
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
