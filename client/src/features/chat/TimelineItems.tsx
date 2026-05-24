import Button from 'antd/es/button'
import Collapse from 'antd/es/collapse'
import Progress from 'antd/es/progress'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import {
  AuditOutlined,
  CloseCircleOutlined,
  FileSearchOutlined,
  LoadingOutlined,
  StopOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import type {
  RuntimeAgentTask,
  RuntimeAuditEvent,
  RuntimePermissionRequest,
  RuntimeToolCall,
  RuntimeTurn,
} from '../../runtime'

const { Text } = Typography

export function TurnTimelineItem({ turn, onOpenAudit }: { turn: RuntimeTurn; onOpenAudit: (turnId?: string) => void }) {
  return (
    <div className="timeline-runtime-item">
      <Space size={8} wrap>
        <Tag color={statusColor(turn.status)}>{label(turn.status)}</Tag>
        <Text strong>Turn</Text>
        {turn.model ? <Tag>{turn.model}</Tag> : null}
        {turn.provider ? <Tag>{turn.provider}</Tag> : null}
        {turn.durationMs ? <Text type="secondary">{formatDuration(turn.durationMs)}</Text> : null}
      </Space>
      <Space size={6}>
        {turn.promptPreview ? <Text type="secondary">{turn.promptPreview}</Text> : null}
        <Tooltip title="Open audit">
          <Button type="text" size="small" icon={<AuditOutlined />} onClick={() => onOpenAudit(turn.id)} />
        </Tooltip>
      </Space>
      {turn.error ? <Text type="danger">{turn.error}</Text> : null}
    </div>
  )
}

export function ToolCallCard({ toolCall, onOpenAudit }: { toolCall: RuntimeToolCall; onOpenAudit: (turnId?: string) => void }) {
  const duration = durationBetween(toolCall.startedAt, toolCall.finishedAt)
  const detailItems = [
    summaryBlock('Input', firstText(toolCall.inputSummary, toolCall.command)),
    summaryBlock('Output', firstText(toolCall.outputSummary, toolCall.modelContent, toolCall.structuredOutput)),
    summaryBlock('stdout', toolCall.stdout),
    summaryBlock('stderr', toolCall.stderr),
    summaryBlock('Error', toolCall.error),
  ].filter(Boolean)

  return (
    <div className={`tool-card ${toolCall.isError || toolCall.status === 'failed' || toolCall.status === 'denied' ? 'error' : ''}`}>
      <div className="tool-card-header">
        <Space size={8} wrap>
          {toolStatusIcon(toolCall.status)}
          <Text strong>{toolCall.name || 'tool'}</Text>
          <Tag color={statusColor(toolCall.status)}>{label(toolCall.status)}</Tag>
          {toolCall.source ? <Tag>{toolCall.source}</Tag> : null}
          {toolCall.risk ? <Tag>{toolCall.risk}</Tag> : null}
          {toolCall.capabilityId ? <Tag>{toolCall.capabilityId}</Tag> : null}
        </Space>
        <Space size={6}>
          {duration ? <Text type="secondary">{formatDuration(duration)}</Text> : null}
          <Tooltip title="Open audit">
            <Button type="text" size="small" icon={<AuditOutlined />} onClick={() => onOpenAudit(toolCall.turnId)} />
          </Tooltip>
        </Space>
      </div>
      {toolCall.policyReason ? <Text type="secondary">{toolCall.policyReason}</Text> : null}
      {toolCall.jobId || toolCall.jobStatus || typeof toolCall.exitCode === 'number' ? (
        <Space size={6} wrap>
          {toolCall.jobId ? <Tag>job {shortID(toolCall.jobId)}</Tag> : null}
          {toolCall.jobStatus ? <Tag>{toolCall.jobStatus}</Tag> : null}
          {typeof toolCall.exitCode === 'number' ? <Tag>exit {toolCall.exitCode}</Tag> : null}
        </Space>
      ) : null}
      {detailItems.length > 0 ? (
        <Collapse
          ghost
          size="small"
          className="tool-card-collapse"
          items={[
            {
              key: 'details',
              label: 'Details',
              children: <div className="tool-card-details">{detailItems}</div>,
            },
          ]}
        />
      ) : null}
    </div>
  )
}

export function PermissionTimelineItem({
  permission,
  onDecide,
}: {
  permission: RuntimePermissionRequest
  onDecide: (permissionId: string, action: 'allow' | 'allow_session' | 'deny') => Promise<void>
}) {
  const pending = !permission.status || permission.status === 'pending'
  return (
    <div className="permission-card">
      <div className="permission-card-header">
        <Space size={8} wrap>
          <Tag color={permissionStatusColor(permission.status)}>{label(permission.status || 'pending')}</Tag>
          <Text strong>{permission.toolName || 'Tool permission'}</Text>
          {permission.action ? <Tag>{permission.action}</Tag> : null}
          {permission.risk ? <Tag>{permission.risk}</Tag> : null}
          {permission.policyMode ? <Tag>{permission.policyMode}</Tag> : null}
        </Space>
        {pending ? (
          <Space size={4}>
            <Button size="small" danger onClick={() => onDecide(permission.id, 'deny')}>
              Deny
            </Button>
            <Button size="small" onClick={() => onDecide(permission.id, 'allow_session')}>
              Session
            </Button>
            <Button size="small" type="primary" onClick={() => onDecide(permission.id, 'allow')}>
              Allow
            </Button>
          </Space>
        ) : null}
      </div>
      <Space size={6} wrap>
        {permission.target || permission.path ? <Text type="secondary">{permission.target || permission.path}</Text> : null}
        {permission.decision ? <Tag>{permission.decision}</Tag> : null}
        {permission.decidedAt ? <Text type="secondary">{formatTime(permission.decidedAt)}</Text> : null}
      </Space>
      {permission.reason || permission.policyReason ? <Text type="secondary">{permission.reason || permission.policyReason}</Text> : null}
      {permission.description ? <Text>{permission.description}</Text> : null}
    </div>
  )
}

export function AgentTaskTimelineItem({
  task,
  onCancel,
}: {
  task: RuntimeAgentTask
  onCancel: (taskId: string) => void
}) {
  const cancellable = task.status === 'running' || task.status === 'queued'
  return (
    <div className="task-card">
      <div className="task-card-header">
        <Space size={8} wrap>
          <Tag color={statusColor(task.status)}>{label(task.status)}</Tag>
          <Text strong>{task.title || task.name || task.kind}</Text>
          {task.kind ? <Tag>{task.kind}</Tag> : null}
          {task.childSessionId ? <Tag>child {shortID(task.childSessionId)}</Tag> : null}
          {task.parentToolCallId ? <Tag>tool {shortID(task.parentToolCallId)}</Tag> : null}
        </Space>
        {cancellable ? (
          <Tooltip title="Cancel task">
            <Button size="small" danger type="text" icon={<StopOutlined />} onClick={() => onCancel(task.id)} />
          </Tooltip>
        ) : null}
      </div>
      <Progress percent={boundedPercent(task.progress)} size="small" showInfo={false} />
      <Space size={6} wrap>
        {task.role ? <Tag>{task.role}</Tag> : null}
        {task.model ? <Tag>{task.model}</Tag> : null}
        {task.allowedTools?.length ? <Tag>{task.allowedTools.length} tools</Tag> : null}
        {task.capabilityScope?.length ? <Tag>{task.capabilityScope.join(', ')}</Tag> : null}
        {task.cwd ? <Text type="secondary">{task.cwd}</Text> : null}
      </Space>
      {task.result?.summary || task.resultSummary ? <Text type="secondary">{task.result?.summary || task.resultSummary}</Text> : null}
      {task.result?.artifactRefs?.length ? <Text type="secondary">{task.result.artifactRefs.length} artifact refs</Text> : null}
      {task.error ? <Text type="danger">{task.error}</Text> : null}
    </div>
  )
}

export function AuditTimelineItem({ event, onOpenAudit }: { event: RuntimeAuditEvent; onOpenAudit: (turnId?: string) => void }) {
  return (
    <div className="audit-timeline-item">
      <Space size={8} wrap>
        <FileSearchOutlined />
        <Tag>{event.type}</Tag>
        {event.turn_id ? <Text type="secondary">turn {shortID(event.turn_id)}</Text> : null}
        <Text type="secondary">{formatAuditTime(event.created_at)}</Text>
      </Space>
      <AuditSummary event={event} />
      <Button type="link" size="small" icon={<AuditOutlined />} onClick={() => onOpenAudit(event.turn_id)}>
        Audit
      </Button>
    </div>
  )
}

function AuditSummary({ event }: { event: RuntimeAuditEvent }) {
  const skillSummary = objectValue(event.payload.skill_summary)
  const contextSummary = objectValue(event.payload.context_summary)
  const task = objectValue(event.payload.agent_task)
  const tags = [
    skillSummary ? `skills ${numberValue(skillSummary.available_count)}` : '',
    contextSummary ? `context ${numberValue(contextSummary.available_count)}` : '',
    stringValue(task?.status),
    stringValue(event.payload.permission_decision),
    stringValue(event.payload.permission_risk),
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

function summaryBlock(title: string, value?: string) {
  const text = value?.trim()
  if (!text) return null
  return (
    <section className="tool-card-summary" key={title}>
      <Text type="secondary">{title}</Text>
      <pre className="part-preview">{truncate(text, 1600)}</pre>
    </section>
  )
}

function toolStatusIcon(status: string) {
  if (status === 'running' || status === 'pending' || status === 'waiting_permission') return <LoadingOutlined />
  if (status === 'failed' || status === 'denied' || status === 'cancelled') return <CloseCircleOutlined />
  return <ToolOutlined />
}

function statusColor(status?: string) {
  if (status === 'completed' || status === 'allowed' || status === 'allowed_for_session') return 'success'
  if (status === 'failed' || status === 'denied' || status === 'interrupted' || status === 'expired') return 'error'
  if (status === 'cancelled') return 'default'
  if (status === 'waiting_permission') return 'warning'
  return 'processing'
}

function permissionStatusColor(status?: string) {
  if (!status || status === 'pending') return 'warning'
  return statusColor(status)
}

function label(value?: string) {
  return (value || 'unknown').replaceAll('_', ' ')
}

function firstText(...values: Array<string | undefined>) {
  return values.find((value) => value?.trim())
}

function durationBetween(startedAt?: number, finishedAt?: number) {
  if (!startedAt || !finishedAt || finishedAt < startedAt) return 0
  return finishedAt - startedAt
}

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)} s`
}

function formatTime(value: number) {
  return new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatAuditTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function shortID(value: string) {
  return value.length > 10 ? `${value.slice(0, 10)}...` : value
}

function boundedPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, value))
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function numberValue(value: unknown) {
  return typeof value === 'number' ? value : 0
}

function truncate(value: string, max: number) {
  return value.length > max ? `${value.slice(0, max)}...` : value
}
