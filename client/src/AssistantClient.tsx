import AntApp from 'antd/es/app'
import {
  addRuntimeSkillPath,
  createRuntimeSkill,
  discoverModelConfig,
  refreshRuntimeMcpServer,
  refreshRuntimeSkills,
  saveModelConfig,
  saveRuntimeMcpServer,
  setRuntimeMcpServerEnabled,
  setRuntimeMcpToolEnabled,
  setRuntimeSkillEnabled,
  verifyModelConfig,
} from './api/chat'
import { ChatSidebar } from './components/chat/ChatSidebar'
import { ChatWorkspace } from './components/chat/ChatWorkspace'
import { PermissionReviewModal } from './components/permissions/PermissionReviewModal'
import { OperationsPreview } from './components/runtime/OperationsPreview'
import { ModelSettingsDrawer } from './components/settings/ModelSettingsDrawer'
import { useAssistantClient } from './hooks/useAssistantClient'

export function AssistantClient() {
  const { message } = AntApp.useApp()
  const client = useAssistantClient()

  return (
    <div className="desktop-shell">
      <ChatSidebar
        sessions={client.sessions}
        onDeleteSession={client.deleteSession}
        onOpenOperations={client.openOperations}
        onOpenSettings={() => client.setSettingsOpen(true)}
        onRenameSession={client.renameSession}
        onSelectSession={client.selectSession}
        onStartNewChat={client.startNewChat}
      />

      <ChatWorkspace
        activeChatTitle={client.activeChatTitle}
        activeSession={client.activeSession}
        composerInputRef={client.composerInputRef}
        config={client.config}
        configLoaded={client.configLoaded}
        hasMessages={client.hasMessages}
        input={client.input}
        isModelConfigured={client.isModelConfigured}
        isSending={client.isSending}
        lastError={client.lastError}
        messages={client.messages}
        modelItems={client.modelItems}
        modelSwitching={client.modelSwitching}
        runtimeStatus={client.runtimeStatus}
        viewportRef={client.viewportRef}
        onCancelTurn={client.cancelTurn}
        onCopyMessage={client.copyMessage}
        onOpenOperations={client.openOperations}
        onOpenSettings={() => client.setSettingsOpen(true)}
        onSendMessage={client.sendMessage}
        onSetInput={client.setInput}
      />

      <ModelSettingsDrawer
        config={client.config}
        open={client.settingsOpen}
        saving={client.settingsSaving}
        onClose={() => client.setSettingsOpen(false)}
        verifying={client.settingsVerifying}
        discovering={client.settingsDiscovering}
        onSave={async (nextConfig) => {
          client.setSettingsSaving(true)
          try {
            const saved = await saveModelConfig(nextConfig)
            client.setConfig((current) => ({ ...current, ...saved }))
            client.setModels(saved.models?.length ? saved.models : saved.model ? [saved.model] : [])
            client.setLastError('')
            message.success('Model settings saved')
            client.setSettingsOpen(false)
          } catch (error) {
            const reason = error instanceof Error ? error.message : String(error)
            message.error(reason)
          } finally {
            client.setSettingsSaving(false)
          }
        }}
        onVerify={async (nextConfig) => {
          client.setSettingsVerifying(true)
          try {
            const result = await verifyModelConfig(nextConfig)
            if (result.ok) {
              const nextModels = result.models?.length ? result.models : result.model ? [result.model] : []
              if (nextModels.length > 0) {
                client.setModels(nextModels)
                client.setConfig((current) => ({ ...current, models: nextModels, model: result.model || nextModels[0] }))
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
            client.setSettingsVerifying(false)
          }
        }}
        onDiscover={async (nextConfig) => {
          client.setSettingsDiscovering(true)
          try {
            const result = await discoverModelConfig(nextConfig)
            if (result.error) {
              message.error(result.error)
              return result
            }
            const nextModels = result.models ?? []
            client.setModels(nextModels)
            client.setConfig((current) => ({ ...current, models: nextModels, model: result.model || current.model }))
            message.success(`Loaded ${nextModels.length} models`)
            return result
          } catch (error) {
            const reason = error instanceof Error ? error.message : String(error)
            message.error(reason)
            throw error
          } finally {
            client.setSettingsDiscovering(false)
          }
        }}
      />
      <PermissionReviewModal permissions={client.permissions} onDecide={client.decidePermission} />
      <OperationsPreview
        capabilities={client.capabilities}
        auditEvents={client.auditEvents}
        events={client.events}
        mcpServers={client.mcpServers}
        mcpResourcesByServer={client.mcpResourcesByServer}
        mcpPromptsByServer={client.mcpPromptsByServer}
        mcpToolsByServer={client.mcpToolsByServer}
        open={client.operationsOpen}
        skills={client.skills}
        onEditMcpServer={async (config) => {
          const nextServers = await saveRuntimeMcpServer(config)
          client.setMcpServers(nextServers)
          await client.refreshRuntimeInventory()
          message.success('MCP server saved')
        }}
        onRefreshAudit={() => client.refreshAudit()}
        onRefreshMcpServer={async (server) => {
          const nextServers = await refreshRuntimeMcpServer(server)
          client.setMcpServers(nextServers)
          await client.refreshMcpTools(server).catch(() => undefined)
          await client.refreshRuntimeInventory()
          message.success('MCP server refreshed')
        }}
        onRefreshSkills={async () => {
          const nextSkills = await refreshRuntimeSkills()
          client.setSkills(nextSkills)
          await client.refreshRuntimeInventory()
          message.success('Skills refreshed')
        }}
        onCreateSkill={async (request) => {
          const nextSkills = await createRuntimeSkill(request)
          client.setSkills(nextSkills)
          await client.refreshRuntimeInventory()
          message.success('Skill created')
        }}
        onAddSkillPath={async (path) => {
          const nextSkills = await addRuntimeSkillPath(path)
          client.setSkills(nextSkills)
          await client.refreshRuntimeInventory()
          message.success('Skill path added')
        }}
        onToggleMcpServer={async (server, enabled) => {
          const nextServers = await setRuntimeMcpServerEnabled(server, enabled)
          client.setMcpServers(nextServers)
          await client.refreshRuntimeInventory()
          message.success(enabled ? 'MCP server enabled' : 'MCP server disabled')
        }}
        onToggleMcpTool={async (server, tool, enabled) => {
          const nextTools = await setRuntimeMcpToolEnabled(server, tool, enabled)
          client.setMcpToolsByServer((current) => ({ ...current, [server]: nextTools }))
          await client.refreshRuntimeInventory()
          message.success(enabled ? 'MCP tool allowed' : 'MCP tool denied')
        }}
        onToggleSkill={async (name, enabled) => {
          const nextSkills = await setRuntimeSkillEnabled(name, enabled)
          client.setSkills(nextSkills)
          await client.refreshRuntimeInventory()
          message.success(enabled ? 'Skill enabled' : 'Skill disabled')
        }}
        onViewMcpTools={(server) => client.refreshMcpTools(server)}
        onClose={() => client.setOperationsOpen(false)}
      />
    </div>
  )
}
