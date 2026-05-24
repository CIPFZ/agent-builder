import { ReloadOutlined } from '@ant-design/icons'
import Button from 'antd/es/button'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import type { RuntimeCapability } from '../../runtime'

const { Text } = Typography

function capabilityStateColor(state: string, enabled: boolean) {
  if (!enabled || state === 'disabled') return 'default'
  if (state === 'loaded') return 'green'
  if (state === 'loading') return 'processing'
  if (state === 'failed' || state === 'unavailable') return 'red'
  return 'gold'
}

export function RuntimeCapabilityPanel({
  capabilities,
  onRefresh,
}: {
  capabilities: RuntimeCapability[]
  onRefresh: (capabilityId: string) => Promise<void>
}) {
  if (capabilities.length === 0) return <Text type="secondary">No capabilities available.</Text>
  return (
    <div className="runtime-list">
      <div className="runtime-list-row compact">
        <Space size={8}>
          <Tag>context</Tag>
          <Text strong>Instruction sources</Text>
          <Tag color="gold">metadata</Tag>
        </Space>
        <Text type="secondary">Context source inventory is runtime-owned and visible in audit events.</Text>
      </div>
      {capabilities.slice(0, 18).map((capability) => (
        <div className="runtime-list-row compact" key={capability.id}>
          <Space size={8}>
            <Tag>{capability.kind}</Tag>
            <Text strong>{capability.name}</Text>
            <Tag color={capability.enabled ? 'green' : 'default'}>{capability.enabled ? 'on' : 'off'}</Tag>
            <Tag color={capabilityStateColor(capability.state, capability.enabled)}>{capability.state}</Tag>
            <Tag>{capability.risk}</Tag>
            <Button size="small" icon={<ReloadOutlined />} disabled={!capability.enabled || capability.state === 'loading'} onClick={() => onRefresh(capability.id)} />
          </Space>
          {capability.source ? <Text type="secondary">{capability.source}</Text> : null}
          {capability.schemaSummary || capability.schemaDigest ? (
            <Text type="secondary">
              {capability.schemaSummary}
              {capability.schemaDigest ? ` (${capability.schemaDigest})` : ''}
            </Text>
          ) : null}
          {capability.error || capability.diagnostics || capability.reason ? (
            <Text type={capability.error ? 'danger' : 'secondary'}>{capability.error || capability.diagnostics || capability.reason}</Text>
          ) : null}
        </div>
      ))}
    </div>
  )
}
