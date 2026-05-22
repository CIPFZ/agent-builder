import type { RefObject } from 'react'
import Button from 'antd/es/button'
import Dropdown from 'antd/es/dropdown'
import Flex from 'antd/es/flex'
import Space from 'antd/es/space'
import Tooltip from 'antd/es/tooltip'
import type { MenuProps } from 'antd'
import { ApiOutlined, DownOutlined, PlusOutlined, SendOutlined, ToolOutlined } from '@ant-design/icons'
import TextArea from 'antd/es/input/TextArea'
import type { TextAreaRef } from 'antd/es/input/TextArea'
import type { ModelConfig } from '../../runtime/api'
import { modelLabel } from './chatUtils'

export type ComposerProps = {
  config: ModelConfig
  input: string
  inputRef: RefObject<TextAreaRef | null>
  isDisabled: boolean
  isSending: boolean
  modelItems: MenuProps['items']
  onChange: (value: string) => void
  onOpenSettings: () => void
  onSend: () => void
}

export function Composer({ config, input, inputRef, isDisabled, isSending, modelItems, onChange, onOpenSettings, onSend }: ComposerProps) {
  return (
    <div className="composer" onClick={() => inputRef.current?.focus()}>
      <TextArea
        aria-label="Message composer"
        autoSize={{ minRows: 2, maxRows: 7 }}
        className="composer-input"
        ref={inputRef}
        placeholder="How can I help you today?"
        disabled={isDisabled || isSending}
        value={input}
        onChange={(event) => onChange(event.target.value)}
        onPressEnter={(event) => {
          if (!event.shiftKey) {
            event.preventDefault()
            onSend()
          }
        }}
      />
      <Flex justify="space-between" align="center" className="composer-toolbar">
        <Space>
          <Tooltip title="Clear input">
            <Button type="text" icon={<PlusOutlined />} onClick={() => onChange('')} />
          </Tooltip>
          <Tooltip title="Model settings">
            <Button type="text" icon={<ToolOutlined />} onClick={onOpenSettings} />
          </Tooltip>
        </Space>
        <Space>
          <Dropdown menu={{ items: modelItems }} trigger={['click']}>
            <Button type="text">
              {modelLabel(config)} <DownOutlined />
            </Button>
          </Dropdown>
          <Tooltip title="Open model settings">
            <Button type="text" icon={<ApiOutlined />} onClick={onOpenSettings} />
          </Tooltip>
          <Button type="primary" shape="circle" icon={<SendOutlined />} loading={isSending} onClick={onSend} />
        </Space>
      </Flex>
    </div>
  )
}
