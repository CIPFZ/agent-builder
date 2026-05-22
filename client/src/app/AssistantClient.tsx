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
} from '../runtime/api'
import { ChatSidebar } from '../features/chat/ChatSidebar'
import { ChatWorkspace } from '../features/chat/ChatWorkspace'
import { PermissionReviewModal } from '../features/permissions/PermissionReviewModal'
import { RuntimeAuditDrawer } from '../features/audit/RuntimeAuditDrawer'
import { RuntimeFeatureWorkspace } from '../features/capabilities/RuntimeFeatureWorkspace'
import { ModelSettingsDrawer } from '../features/settings/ModelSettingsDrawer'
import { useAssistantClient } from '../features/chat/useAssistantClient'

export function AssistantClient() {
  const { message } = AntApp.useApp()
  const client = useAssistantClient()

  return (
    <div className={client.sidebarCollapsed ? 'desktop-shell sidebar-hidden' : 'desktop-shell'}>
      <ChatSidebar
        activeView={client.activeView}
        collapsed={client.sidebarCollapsed}
        sessions={client.sessions}
        onDeleteSession={client.deleteSession}
        onOpenSettings={() => client.setSettingsOpen(true)}
        onOpenView={client.openRuntimeView}
        onRenameSession={client.renameSession}
        onSearch={() => client.composerInputRef.current?.focus()}
        onSelectSession={client.selectSession}
        onStartNewChat={() => {
          client.setActiveView('chat')
          client.startNewChat()
        }}
        onToggleCollapsed={() => client.setSidebarCollapsed((current) => !current)}
      />

      {client.activeView === 'chat' ? (
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
          sidebarCollapsed={client.sidebarCollapsed}
          viewportRef={client.viewportRef}
          onCancelTurn={client.cancelTurn}
          onCopyMessage={client.copyMessage}
          onOpenAudit={client.openAudit}
          onOpenSettings={() => client.setSettingsOpen(true)}
          onSendMessage={client.sendMessage}
          onSetInput={client.setInput}
          onToggleSidebar={() => client.setSidebarCollapsed(false)}
        />
      ) : (
        <RuntimeFeatureWorkspace
          capabilities={client.capabilities}
          mcpServers={client.mcpServers}
          mcpResourcesByServer={client.mcpResourcesByServer}
          mcpPromptsByServer={client.mcpPromptsByServer}
          mcpToolsByServer={client.mcpToolsByServer}
          skills={client.skills}
          sidebarCollapsed={client.sidebarCollapsed}
          view={client.activeView}
          onEditMcpServer={async (config) => {
            const nextServers = await saveRuntimeMcpServer(config)
            client.setMcpServers(nextServers)
            await client.refreshRuntimeInventory()
            message.success('MCP server saved')
          }}
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
          onToggleSidebar={() => client.setSidebarCollapsed(false)}
          onViewMcpTools={(server) => client.refreshMcpTools(server)}
        />
      )}

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
      <RuntimeAuditDrawer
        events={client.auditEvents}
        open={client.auditOpen}
        onClose={() => client.setAuditOpen(false)}
        onRefresh={() => client.refreshAudit()}
      />
    </div>
  )
}
