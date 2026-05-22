import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import type { RuntimeCapability } from '../../runtime'

const { Text } = Typography

export function RuntimeCapabilityPanel({ capabilities }: { capabilities: RuntimeCapability[] }) {
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

