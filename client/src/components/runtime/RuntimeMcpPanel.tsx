import { useState } from 'react'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import Button from 'antd/es/button'
import Form from 'antd/es/form'
import Input from 'antd/es/input'
import Modal from 'antd/es/modal'
import Select from 'antd/es/select'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import TextArea from 'antd/es/input/TextArea'
import Typography from 'antd/es/typography'
import type { RuntimeMcpPrompt, RuntimeMcpResource, RuntimeMcpServer, RuntimeMcpServerConfig, RuntimeMcpTool } from '../../runtime'
import { linesToList, linesToMap, mapToLines } from './runtimePanelUtils'

const { Text } = Typography

export function RuntimeMcpPanel({
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

