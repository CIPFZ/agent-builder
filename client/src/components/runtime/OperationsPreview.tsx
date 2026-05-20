import { useState } from 'react'
import Button from 'antd/es/button'
import Collapse from 'antd/es/collapse'
import Form from 'antd/es/form'
import Input from 'antd/es/input'
import Modal from 'antd/es/modal'
import Select from 'antd/es/select'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import TextArea from 'antd/es/input/TextArea'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
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
      {events.length === 0 ? <Text type="secondary">No audit events for the active session or turn.</Text> : null}
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
