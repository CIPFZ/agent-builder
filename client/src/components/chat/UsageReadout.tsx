import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import type { RuntimeStatus } from '../../runtime'
import { emptyUsage } from './chatUtils'

const { Text } = Typography

export function UsageReadout({ status }: { status: RuntimeStatus | null }) {
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
