import { Alert, Space, Typography } from 'antd'
import { DeploymentUnitOutlined, SafetyCertificateOutlined, ToolOutlined, UserOutlined } from '@ant-design/icons'
import Bubble from '@ant-design/x/es/bubble'
import Sender from '@ant-design/x/es/sender'
import ThoughtChain from '@ant-design/x/es/thought-chain'
import type { ConversationMessage, RuntimeState } from '../types/runtime'

const { Text } = Typography

function messageAvatar(role: ConversationMessage['role']) {
  if (role === 'user') return <UserOutlined />
  if (role === 'tool') return <ToolOutlined />
  if (role === 'approval') return <SafetyCertificateOutlined />
  return <DeploymentUnitOutlined />
}

function messagePlacement(role: ConversationMessage['role']) {
  return role === 'user' ? 'end' : 'start'
}

function renderMessage(message: ConversationMessage, state: RuntimeState) {
  if (message.role === 'assistant' && message.id === 'msg-agent-1') {
    return (
      <Space orientation="vertical" size={10}>
        <Text>{message.content}</Text>
        <ThoughtChain items={state.thoughts} />
      </Space>
    )
  }

  if (message.role === 'approval') {
    return (
      <Alert
        type="warning"
        showIcon
        title="发现潜在高风险修复动作"
        description={message.content}
      />
    )
  }

  return message.content
}

type AgentConversationProps = {
  state: RuntimeState
}

export function AgentConversation({ state }: AgentConversationProps) {
  return (
    <>
      <div className="conversation-scroll">
        {state.messages.map((message) => (
          <Bubble
            key={message.id}
            placement={messagePlacement(message.role)}
            avatar={messageAvatar(message.role)}
            content={renderMessage(message, state)}
          />
        ))}
      </div>
      <Sender
        className="sender"
        placeholder="Describe the issue, ask for another SOP step, or approve a proposed action..."
        disabled
        value=""
      />
    </>
  )
}
