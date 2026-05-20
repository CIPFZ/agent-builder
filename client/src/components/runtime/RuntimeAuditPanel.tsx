import Button from 'antd/es/button'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import type { RuntimeAuditEvent } from '../../runtime'

const { Text } = Typography

export function RuntimeAuditPanel({ events, onRefresh }: { events: RuntimeAuditEvent[]; onRefresh: () => void }) {
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

