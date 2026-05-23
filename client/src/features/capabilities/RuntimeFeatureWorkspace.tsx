import type { ReactNode } from 'react'
import { AppstoreOutlined, ApiOutlined, MenuOutlined, ToolOutlined } from '@ant-design/icons'
import Button from 'antd/es/button'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import type {
  RuntimeCapability,
  RuntimeMcpPrompt,
  RuntimeMcpResource,
  RuntimeMcpServer,
  RuntimeMcpServerConfig,
  RuntimeMcpTool,
  RuntimeSkill,
  RuntimeSkillCreateRequest,
} from '../../runtime'
import { RuntimeCapabilityPanel } from './RuntimeCapabilityPanel'
import { RuntimeMcpPanel } from '../mcp/RuntimeMcpPanel'
import { RuntimeSkillPanel } from '../skills/RuntimeSkillPanel'

const { Text, Title } = Typography

export type RuntimeFeatureView = 'skills' | 'plugins' | 'mcp'

type RuntimeFeatureWorkspaceProps = {
  capabilities: RuntimeCapability[]
  mcpServers: RuntimeMcpServer[]
  mcpResourcesByServer: Record<string, RuntimeMcpResource[]>
  mcpPromptsByServer: Record<string, RuntimeMcpPrompt[]>
  mcpToolsByServer: Record<string, RuntimeMcpTool[]>
  skills: RuntimeSkill[]
  sidebarCollapsed: boolean
  view: RuntimeFeatureView
  onAddSkillPath: (path: string) => Promise<void>
  onCreateSkill: (request: RuntimeSkillCreateRequest) => Promise<void>
  onEditMcpServer: (config: RuntimeMcpServerConfig) => Promise<void>
  onRefreshMcpServer: (server: string) => Promise<void>
  onRefreshCapability: (capabilityId: string) => Promise<void>
  onRefreshSkills: () => Promise<void>
  onToggleMcpServer: (server: string, enabled: boolean) => Promise<void>
  onToggleMcpTool: (server: string, tool: string, enabled: boolean) => Promise<void>
  onToggleSkill: (name: string, enabled: boolean) => Promise<void>
  onToggleSidebar: () => void
  onViewMcpTools: (server: string) => Promise<RuntimeMcpTool[]>
}

const viewMeta = {
  skills: {
    icon: <ToolOutlined />,
    title: 'Skills',
    description: 'Project and builtin skills available to the runtime.',
  },
  plugins: {
    icon: <AppstoreOutlined />,
    title: 'Plugins',
    description: 'Runtime capability inventory from builtin tools, skills, and MCP.',
  },
  mcp: {
    icon: <ApiOutlined />,
    title: 'MCP',
    description: 'Model Context Protocol servers, tools, prompts, and resources.',
  },
} satisfies Record<RuntimeFeatureView, { icon: ReactNode; title: string; description: string }>

export function RuntimeFeatureWorkspace({
  capabilities,
  mcpServers,
  mcpResourcesByServer,
  mcpPromptsByServer,
  mcpToolsByServer,
  skills,
  sidebarCollapsed,
  view,
  onAddSkillPath,
  onCreateSkill,
  onEditMcpServer,
  onRefreshCapability,
  onRefreshMcpServer,
  onRefreshSkills,
  onToggleMcpServer,
  onToggleMcpTool,
  onToggleSkill,
  onToggleSidebar,
  onViewMcpTools,
}: RuntimeFeatureWorkspaceProps) {
  const meta = viewMeta[view]

  return (
    <main className="feature-main">
      <header className="feature-header">
        {sidebarCollapsed ? (
          <Tooltip title="Show sidebar">
            <Button type="text" icon={<MenuOutlined />} onClick={onToggleSidebar} />
          </Tooltip>
        ) : null}
        <div className="feature-title-icon">{meta.icon}</div>
        <div>
          <Title level={3}>{meta.title}</Title>
          <Text type="secondary">{meta.description}</Text>
        </div>
      </header>
      <section className="feature-content">
        {view === 'skills' ? (
          <RuntimeSkillPanel skills={skills} onRefresh={onRefreshSkills} onCreate={onCreateSkill} onAddPath={onAddSkillPath} onToggle={onToggleSkill} />
        ) : null}
        {view === 'plugins' ? <RuntimeCapabilityPanel capabilities={capabilities} onRefresh={onRefreshCapability} /> : null}
        {view === 'mcp' ? (
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
        ) : null}
      </section>
    </main>
  )
}
