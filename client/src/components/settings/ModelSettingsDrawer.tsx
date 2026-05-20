import { useState } from 'react'
import Button from 'antd/es/button'
import Collapse from 'antd/es/collapse'
import Drawer from 'antd/es/drawer'
import Form from 'antd/es/form'
import Input from 'antd/es/input'
import Select from 'antd/es/select'
import Space from 'antd/es/space'
import Typography from 'antd/es/typography'
import type { ModelConfig } from '../../api/chat'
import type { RuntimeModelVerifyResponse } from '../../runtime'

const { Paragraph } = Typography

type ModelSettingsDrawerProps = {
  config: ModelConfig
  open: boolean
  saving: boolean
  verifying: boolean
  onClose: () => void
  onSave: (config: ModelConfig) => Promise<void>
  onVerify: (config: ModelConfig) => Promise<RuntimeModelVerifyResponse>
}

export function ModelSettingsDrawer({ config, open, saving, verifying, onClose, onSave, onVerify }: ModelSettingsDrawerProps) {
  const [form] = Form.useForm<ModelConfig>()
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const modelOptions = ((availableModels.length ? availableModels : undefined) ??
    (config.models?.length ? config.models : undefined) ??
    (config.model ? [config.model] : [])).map((model) => ({
    value: model,
    label: model,
  }))

  return (
    <Drawer
      title="Model settings"
      placement="right"
      size={420}
      open={open}
      onClose={onClose}
      afterOpenChange={(visible) => {
        if (visible) {
          form.setFieldsValue(config)
          setAvailableModels(config.models?.length ? config.models : config.model ? [config.model] : [])
        }
      }}
      extra={
        <Space>
          <Button
            loading={verifying}
            onClick={() => {
              form.validateFields().then(async (values) => {
                const result = await onVerify(values)
                const nextModels = result.models?.length ? result.models : result.model ? [result.model] : []
                if (result.ok && nextModels.length > 0) {
                  setAvailableModels(nextModels)
                  form.setFieldsValue({ model: result.model || nextModels[0] })
                }
              })
            }}
          >
            Verify
          </Button>
          <Button
            type="primary"
            loading={saving}
            onClick={() => {
              form.validateFields().then((values) => onSave(values))
            }}
          >
            Save
          </Button>
        </Space>
      }
    >
      <Paragraph type="secondary">Saved to the desktop config directory beside the application.</Paragraph>
      <Form form={form} layout="vertical" initialValues={config}>
        <Form.Item label="Protocol" name="protocol" rules={[{ required: true }]}>
          <Select
            options={[
              { value: 'openai', label: 'OpenAI compatible' },
              { value: 'anthropic', label: 'Anthropic compatible' },
            ]}
          />
        </Form.Item>
        <Form.Item label="URL" name="url" rules={[{ required: true }]}>
          <Input placeholder="https://api.example.com" />
        </Form.Item>
        <Form.Item label="API key" name="apiKey" rules={config.hasApiKey ? [] : [{ required: true }]}>
          <Input.Password placeholder={config.hasApiKey ? 'Saved. Leave empty to keep current key.' : 'sk-...'} />
        </Form.Item>
        <Form.Item label="Model" name="model">
          <Select showSearch placeholder="Model list is fetched after verification or save" options={modelOptions} />
        </Form.Item>
        <Collapse
          ghost
          items={[
            {
              key: 'advanced',
              label: 'Advanced',
              children: (
                <Form.Item label="Proxy" name="proxy">
                  <Input placeholder="http://127.0.0.1:7890" />
                </Form.Item>
              ),
            },
          ]}
        />
      </Form>
    </Drawer>
  )
}
