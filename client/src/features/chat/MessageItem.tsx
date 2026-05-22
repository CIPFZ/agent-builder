import Button from 'antd/es/button'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import { BulbOutlined, CheckCircleOutlined, CloseCircleOutlined, CopyOutlined, ReloadOutlined, ToolOutlined, UserOutlined } from '@ant-design/icons'
import type { RuntimeMessage, RuntimeMessagePart } from '../../runtime'
import { hasAssistantText, messageReasoningParts, messageToolParts } from './chatUtils'

const { Text } = Typography

export function MessageItem({ chatMessage, onCopy }: { chatMessage: RuntimeMessage; onCopy: (content: string) => void }) {
  const reasoningParts = messageReasoningParts(chatMessage)
  const toolParts = messageToolParts(chatMessage)
  const showText = chatMessage.role === 'user' || hasAssistantText(chatMessage)
  const isToolOnly = !showText && toolParts.length > 0

  return (
    <article className={`message-row ${isToolOnly ? 'tool' : chatMessage.role}`}>
      {chatMessage.role === 'user' ? (
        <div className="user-avatar">
          <UserOutlined />
        </div>
      ) : (
        <div className={isToolOnly ? 'tool-mark' : 'assistant-mark'}>{isToolOnly ? <ToolOutlined /> : '*'}</div>
      )}
      <div className="message-body">
        {reasoningParts.length > 0 ? <ReasoningPanel parts={reasoningParts} /> : null}
        {toolParts.length > 0 ? <ToolActivity parts={toolParts} /> : null}
        {showText ? <div className="message-bubble">{chatMessage.content}</div> : null}
        {hasAssistantText(chatMessage) ? (
          <Space className="message-actions" size={8}>
            <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => onCopy(chatMessage.content)} />
            <Button type="text" size="small" icon={<ReloadOutlined />} />
            {chatMessage.provider ? <Tag>{chatMessage.provider}</Tag> : null}
            {chatMessage.model ? <Tag>{chatMessage.model}</Tag> : null}
          </Space>
        ) : null}
      </div>
    </article>
  )
}

function ReasoningPanel({ parts }: { parts: RuntimeMessagePart[] }) {
  return (
    <div className="reasoning-panel">
      <Space size={8}>
        <BulbOutlined />
        <Text type="secondary">Thinking</Text>
      </Space>
      {parts.map((part, index) => (
        <pre className="part-preview" key={`${part.startedAt ?? index}-${index}`}>
          {part.thinking}
        </pre>
      ))}
    </div>
  )
}

function ToolActivity({ parts }: { parts: RuntimeMessagePart[] }) {
  return (
    <div className="tool-activity">
      {parts.map((part, index) => (
        <ToolActivityItem key={`${part.toolCallId ?? part.name ?? index}-${part.type}-${index}`} part={part} />
      ))}
    </div>
  )
}

function ToolActivityItem({ part }: { part: RuntimeMessagePart }) {
  const isResult = part.type === 'tool_result'
  const hasPreview = Boolean((isResult ? part.content || part.data || part.metadata : part.input)?.trim())
  const preview = isResult ? part.content || part.data || part.metadata : part.input

  return (
    <div className={part.isError ? 'tool-step error' : 'tool-step'}>
      <div className="tool-step-header">
        <Space size={8}>
          {part.isError ? <CloseCircleOutlined /> : isResult ? <CheckCircleOutlined /> : <ToolOutlined />}
          <Text strong>{part.name || 'tool'}</Text>
          <Tag>{isResult ? (part.isError ? 'failed' : 'result') : part.finished ? 'called' : 'running'}</Tag>
        </Space>
        {part.toolCallId ? <Text type="secondary">{part.toolCallId}</Text> : null}
      </div>
      {hasPreview ? <pre className="part-preview">{preview}</pre> : null}
    </div>
  )
}
