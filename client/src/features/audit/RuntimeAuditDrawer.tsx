import Button from 'antd/es/button'
import Drawer from 'antd/es/drawer'
import Empty from 'antd/es/empty'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import { ReloadOutlined } from '@ant-design/icons'
import type { RuntimeAuditEvent, RuntimeSkillTurnItem, RuntimeTurnSkillSummary } from '../../runtime'

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
            <AuditHighlights event={event} />
            <TaskSummary payload={event.payload} />
            <SkillSummary payload={event.payload} />
            <pre className="audit-payload">{JSON.stringify(event.payload, null, 2)}</pre>
          </article>
        ))}
      </div>
    </Drawer>
  )
}

function TaskSummary({ payload }: { payload: Record<string, unknown> }) {
  const task = payload.agent_task
  if (!task || typeof task !== 'object') return null
  const value = task as Record<string, unknown>
  return (
    <Space size={6} wrap>
      <Tag color="purple">{stringValue(value.kind) || 'task'}</Tag>
      {stringValue(value.status) ? <Tag>{stringValue(value.status)}</Tag> : null}
      {stringValue(value.childSessionId) ? <Text type="secondary">child {shortID(stringValue(value.childSessionId))}</Text> : null}
      {stringValue(value.resultSummary) ? <Text type="secondary">{stringValue(value.resultSummary)}</Text> : null}
      {stringValue(value.error) ? <Text type="danger">{stringValue(value.error)}</Text> : null}
    </Space>
  )
}

function SkillSummary({ payload }: { payload: Record<string, unknown> }) {
  const summary = parseSkillSummary(payload.skill_summary)
  if (!summary) return null
  return (
    <div className="audit-skill-summary">
      <Space size={6} wrap>
        <Tag>skills {summary.available_count}</Tag>
        {summary.activated?.length ? <Tag color="green">activated {summary.activated.length}</Tag> : null}
        {summary.excluded?.length ? <Tag color="default">excluded {summary.excluded.length}</Tag> : null}
        {summary.failed?.length ? <Tag color="red">failed {summary.failed.length}</Tag> : null}
        {summary.policy_mode ? <Tag>{summary.policy_mode}</Tag> : null}
      </Space>
      {summary.activated?.length ? <Text type="secondary">Activated: {summary.activated.map((skill: RuntimeSkillTurnItem) => skill.name).join(', ')}</Text> : null}
      {summary.excluded?.length ? (
        <Text type="secondary">Excluded: {summary.excluded.map((skill: RuntimeSkillTurnItem) => `${skill.name}${skill.reason ? ` (${skill.reason})` : ''}`).join(', ')}</Text>
      ) : null}
      {summary.failed?.length ? (
        <Text type="danger">Failed: {summary.failed.map((skill: RuntimeSkillTurnItem) => `${skill.name || skill.path}${skill.error ? ` (${skill.error})` : ''}`).join(', ')}</Text>
      ) : null}
    </div>
  )
}

function parseSkillSummary(value: unknown): RuntimeTurnSkillSummary | null {
  if (!value || typeof value !== 'object') return null
  const summary = value as RuntimeTurnSkillSummary
  if (typeof summary.available_count !== 'number') return null
  return summary
}

function AuditHighlights({ event }: { event: RuntimeAuditEvent }) {
  const toolCalls = Array.isArray(event.payload.tool_calls) ? event.payload.tool_calls : []
  const firstTool = toolCalls[0] as Record<string, unknown> | undefined
  const tags = [
    stringValue(firstTool?.name),
    stringValue(firstTool?.job_id) ? `job ${stringValue(firstTool?.job_id)}` : '',
    stringValue(firstTool?.status),
    stringValue(firstTool?.risk),
    stringValue(event.payload.permission_risk),
    stringValue(event.payload.permission_policy),
  ].filter(Boolean)
  if (tags.length === 0) return null
  return (
    <Space size={6} wrap>
      {tags.map((tag) => (
        <Tag key={tag}>{tag}</Tag>
      ))}
    </Space>
  )
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function shortID(value: string) {
  return value.length > 10 ? `${value.slice(0, 10)}...` : value
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
