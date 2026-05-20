import Button from 'antd/es/button'
import Collapse from 'antd/es/collapse'
import Modal from 'antd/es/modal'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import type {
  RuntimeAuditEvent,
  RuntimeCapability,
  RuntimeEvent,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpServer,
  RuntimeMcpServerConfig,
  RuntimeMcpTool,
  RuntimeSkill,
  RuntimeSkillCreateRequest,
} from '../../runtime'
import { RuntimeAuditPanel } from './RuntimeAuditPanel'
import { RuntimeCapabilityPanel } from './RuntimeCapabilityPanel'
import { RuntimeMcpPanel } from './RuntimeMcpPanel'
import { RuntimeSkillPanel } from './RuntimeSkillPanel'

const { Text, Title } = Typography

export type OperationsPreviewProps = {
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
}

export function OperationsPreview({
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
}: OperationsPreviewProps) {
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
            children: <RuntimeAuditPanel events={auditEvents} onRefresh={onRefreshAudit} />,
          },
          {
            key: 'skills',
            label: 'Skills',
            children: (
              <RuntimeSkillPanel
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
              <RuntimeMcpPanel
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
            children: <RuntimeCapabilityPanel capabilities={capabilities} />,
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


