import Button from 'antd/es/button'
import Drawer from 'antd/es/drawer'
import Empty from 'antd/es/empty'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import { ReloadOutlined } from '@ant-design/icons'
import type { RuntimeAuditEvent } from '../../runtime'

const { Text } = Typography

export function RuntimeAuditDrawer({
  events,
  open,
  onClose,
  onRefresh,
}: {
  events: RuntimeAuditEvent[]
  open: boolean
  onClose: () => void
  onRefresh: () => void
}) {
  const orderedEvents = [...events].reverse()

  return (
    <Drawer
      title="Audit"
      placement="right"
      width={560}
      open={open}
      onClose={onClose}
      extra={
        <Button icon={<ReloadOutlined />} onClick={onRefresh}>
          Refresh
        </Button>
      }
    >
      <div className="audit-drawer">
        {orderedEvents.length === 0 ? <Empty description="No audit events for the active session or turn." /> : null}
        {orderedEvents.map((event) => (
          <article className="audit-card" key={event.id}>
            <div className="audit-card-header">
              <Space size={8} wrap>
                <Tag color="blue">{event.type}</Tag>
                {event.session_id ? <Text type="secondary">session {shortID(event.session_id)}</Text> : null}
                {event.turn_id ? <Text type="secondary">turn {shortID(event.turn_id)}</Text> : null}
              </Space>
              <Text type="secondary">{formatTime(event.created_at)}</Text>
            </div>
            <pre className="audit-payload">{JSON.stringify(event.payload, null, 2)}</pre>
          </article>
        ))}
      </div>
    </Drawer>
  )
}

function shortID(value: string) {
  return value.length > 10 ? `${value.slice(0, 10)}...` : value
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
