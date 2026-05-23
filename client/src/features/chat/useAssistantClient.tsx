import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import AntApp from 'antd/es/app'
import type { MenuProps } from 'antd'
import type { TextAreaRef } from 'antd/es/input/TextArea'
import {
  cancelRuntimeTurn,
  cancelRuntimeTurnById,
  deleteRuntimeSession,
  decideRuntimePermission,
  loadModelConfig,
  requestConfiguredModels,
  requestRuntimeAudit,
  requestRuntimeCapabilities,
  requestRuntimeEvents,
  requestRuntimeEventsEndpoint,
  requestRuntimeMcpPrompts,
  requestRuntimeMcpResources,
  requestRuntimeMcpServers,
  requestRuntimeMcpTools,
  requestRuntimeMessages,
  requestRuntimePermissions,
  requestRuntimeSessionAudit,
  requestRuntimeSessionMessages,
  requestRuntimeSessions,
  requestRuntimeSkills,
  requestRuntimeStatus,
  renameRuntimeSession,
  saveModelConfig,
  selectRuntimeSession,
  sendRuntimePrompt,
  startRuntimeChat,
} from '../../runtime/api'
import type { ModelConfig } from '../../runtime/api'
import { isDefaultSessionTitle } from './chatUtils'
import type { RuntimeFeatureView } from '../capabilities/RuntimeFeatureWorkspace'
import { useRuntimeEventSubscription } from '../../runtime/useRuntimeEventSubscription'
import type {
  RuntimeAuditEvent,
  RuntimeCapability,
  RuntimeEvent,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpServer,
  RuntimeMcpTool,
  RuntimeMessage,
  RuntimePermissionDecision,
  RuntimePermissionRequest,
  RuntimeSession,
  RuntimeSkill,
  RuntimeStatus,
} from '../../runtime'
const defaultConfig: ModelConfig = {
  protocol: 'openai',
  model: '',
  url: '',
  models: [],
}



const runtimeEventLimit = 200



export function useAssistantClient() {
  const { message } = AntApp.useApp()
  const { modal } = AntApp.useApp()
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
  const [settingsDiscovering, setSettingsDiscovering] = useState(false)
  const [settingsSaving, setSettingsSaving] = useState(false)
  const [settingsVerifying, setSettingsVerifying] = useState(false)
  const [modelSwitching, setModelSwitching] = useState(false)
  const [auditOpen, setAuditOpen] = useState(false)
  const [activeView, setActiveView] = useState<RuntimeFeatureView | 'chat'>('chat')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [isSending, setIsSending] = useState(false)
  const [configLoaded, setConfigLoaded] = useState(false)
  const [lastError, setLastError] = useState('')
  const [activeChatTitle, setActiveChatTitle] = useState('New chat')
  const [runtimeStatus, setRuntimeStatus] = useState<RuntimeStatus | null>(null)
  const [auditEvents, setAuditEvents] = useState<RuntimeAuditEvent[]>([])
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const composerInputRef = useRef<TextAreaRef | null>(null)
  const activeSessionIdRef = useRef<string>('')
  const lastEventSequenceRef = useRef(0)

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

  const refreshRuntimeSnapshot = useCallback(async () => {
    const [nextStatus, runtimeSessions, runtimePermissions, runtimeEvents, runtimeSkills, runtimeMcpServers, runtimeCapabilities] =
      await Promise.all([
        requestRuntimeStatus(),
        requestRuntimeSessions(),
        requestRuntimePermissions(),
        requestRuntimeEvents(),
        requestRuntimeSkills(),
        requestRuntimeMcpServers(),
        requestRuntimeCapabilities(),
      ])
    const runtimeMessages = nextStatus.sessionId
      ? await requestRuntimeSessionMessages(nextStatus.sessionId)
      : await requestRuntimeMessages()
    activeSessionIdRef.current = nextStatus.sessionId
    setRuntimeStatus(nextStatus)
    setSessions(runtimeSessions)
    setActiveChatTitle(runtimeSessions.find((session) => session.active)?.title ?? 'New chat')
    setMessages(runtimeMessages)
    setPermissions(runtimePermissions)
    setEvents(runtimeEvents.events)
    lastEventSequenceRef.current = runtimeEvents.last_sequence ?? runtimeEvents.events.at(-1)?.sequence ?? 0
    setSkills(runtimeSkills)
    setMcpServers(runtimeMcpServers)
    setCapabilities(runtimeCapabilities)
  }, [])

  const openAudit = () => {
    setAuditOpen(true)
    refreshAudit().catch(() => undefined)
  }

  const openRuntimeView = (view: RuntimeFeatureView) => {
    setActiveView(view)
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
        setEvents(runtimeEvents.events)
        lastEventSequenceRef.current = runtimeEvents.last_sequence ?? runtimeEvents.events.at(-1)?.sequence ?? 0
        setSkills(runtimeSkills)
        setMcpServers(runtimeMcpServers)
        setCapabilities(runtimeCapabilities)
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [isModelConfigured])

  const handleRuntimeEvent = (event: RuntimeEvent) => {
    lastEventSequenceRef.current = Math.max(lastEventSequenceRef.current, event.sequence || 0)
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
  }

  useRuntimeEventSubscription({
    enabled: isModelConfigured,
    lastSequence: lastEventSequenceRef.current,
    requestEndpoint: requestRuntimeEventsEndpoint,
    onEvent: handleRuntimeEvent,
    onSnapshotRequired: () => {
      refreshRuntimeSnapshot().catch(() => undefined)
    },
  })

  const switchModel = useCallback(async (modelName: string) => {
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
  }, [config, message, modelSwitching])

  const modelItems = useMemo<MenuProps['items']>(
    () =>
      models.map((modelName) => ({
        key: modelName,
        label: modelName,
        onClick: () => {
          void switchModel(modelName)
        },
      })),
    [models, switchModel],
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
    if (!runtimeStatus?.sessionId && messages.length === 0) {
      setPermissions([])
      setAuditEvents([])
      setActiveChatTitle('New chat')
      composerInputRef.current?.focus()
      return
    }
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
        composerInputRef.current?.focus()
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
    if (isDefaultSessionTitle(activeSession?.title)) {
      const previewTitle = content.length > 28 ? `${content.slice(0, 28)}...` : content
      setActiveChatTitle(previewTitle)
      if (runtimeStatus?.sessionId) {
        renameRuntimeSession(runtimeStatus.sessionId, previewTitle).catch(() => undefined)
      }
    }
    try {
      const targetSessionId = runtimeStatus?.sessionId
      const previousAssistantId = [...messages].reverse().find((chatMessage) => chatMessage.role === 'assistant')?.id
      const turn = await sendRuntimePrompt(content, targetSessionId)
      const sessionId = turn.status.sessionId || targetSessionId
      if (!sessionId) {
        throw new Error('Runtime did not create a session for this message.')
      }
      const activeTurnId = turn.turnId || turn.requestId
      setRuntimeStatus(turn.status)
      activeSessionIdRef.current = sessionId
      let runtimeMessages = await loadMessagesForSession(sessionId)
      setMessages(runtimeMessages)
      await refreshPermissions().catch(() => undefined)
      let nextStatus = await refreshStatus().catch(() => runtimeStatus)
      const started = Date.now()
      while (Date.now() - started < 30 * 60 * 1000) {
        await new Promise((resolve) => window.setTimeout(resolve, 700))
        runtimeMessages = await requestRuntimeSessionMessages(sessionId).catch(() => runtimeMessages)
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
      runtimeMessages = await requestRuntimeSessionMessages(sessionId).catch(() => runtimeMessages)
      setMessages(runtimeMessages)
      await refreshPermissions().catch(() => undefined)
      await refreshStatus().catch(() => undefined)
      if (activeTurnId) {
        await refreshAudit(activeTurnId).catch(() => undefined)
      }
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
    let nextTitle = session.title
    modal.confirm({
      title: 'Rename Session',
      content: (
        <input
          className="session-modal-input"
          autoFocus
          defaultValue={session.title}
          onChange={(event) => {
            nextTitle = event.target.value
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              const button = document.querySelector<HTMLElement>('.ant-modal-confirm-btns .ant-btn-primary')
              button?.click()
            }
          }}
        />
      ),
      okText: 'Save',
      async onOk() {
        const title = nextTitle.trim()
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
          throw error
        }
      },
    })
  }

  const deleteSession = async (session: RuntimeSession) => {
    modal.confirm({
      title: 'Delete Session',
      content: `Delete "${session.title || 'Untitled Session'}"? This only removes the local session history.`,
      okText: 'Delete',
      okButtonProps: { danger: true },
      async onOk() {
        try {
          const nextSessions = await deleteRuntimeSession(session.id)
          setSessions(nextSessions)
          if (session.active || session.id === runtimeStatus?.sessionId) {
            const nextStatus = await refreshStatus()
            activeSessionIdRef.current = nextStatus.sessionId
            setMessages([])
            setAuditEvents([])
            setPermissions([])
            setActiveChatTitle('New chat')
          }
        } catch (error) {
          const reason = error instanceof Error ? error.message : String(error)
          setLastError(reason)
          message.error(reason)
          throw error
        }
      },
    })
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
      const activeTurnId = runtimeStatus?.requests?.activeRequestId || [...events].reverse().find((event) => event.turn_id)?.turn_id
      const nextStatus = activeTurnId ? await cancelRuntimeTurnById(activeTurnId) : await cancelRuntimeTurn()
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

	return {
		activeChatTitle,
    activeView,
		activeSession,
    auditOpen,
		auditEvents,
		capabilities,
    cancelTurn,
    config,
    configLoaded,
    copyMessage,
    decidePermission,
    deleteSession,
    hasMessages,
    input,
    isModelConfigured,
    isSending,
    lastError,
    mcpPromptsByServer,
    mcpResourcesByServer,
    mcpServers,
    mcpToolsByServer,
    modelItems,
		modelSwitching,
		events,
		messages,
    openAudit,
    openRuntimeView,
    permissions,
    refreshAudit,
    refreshMcpTools,
    refreshRuntimeInventory,
    renameSession,
    runtimeStatus,
    selectSession,
    sendMessage,
    sessions,
		setConfig,
		setInput,
		setLastError,
		setMcpServers,
		setMcpToolsByServer,
		setModels,
    setActiveView,
    setAuditOpen,
    setSettingsDiscovering,
    setSettingsOpen,
    setSettingsSaving,
    setSettingsVerifying,
    setSkills,
    settingsDiscovering,
    settingsOpen,
    settingsSaving,
    settingsVerifying,
    setSidebarCollapsed,
    sidebarCollapsed,
    skills,
    startNewChat,
    viewportRef,
    composerInputRef,
  }
}
