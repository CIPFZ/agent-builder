import { useState } from 'react'
import { ReloadOutlined } from '@ant-design/icons'
import Button from 'antd/es/button'
import Collapse from 'antd/es/collapse'
import Drawer from 'antd/es/drawer'
import Form from 'antd/es/form'
import Input from 'antd/es/input'
import Select from 'antd/es/select'
import Space from 'antd/es/space'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import type { ModelConfig } from '../../api/chat'
import type { RuntimeModelDiscoveryResponse, RuntimeModelVerifyResponse } from '../../runtime'

const { Paragraph } = Typography

type ModelSettingsDrawerProps = {
  config: ModelConfig
  discovering: boolean
  open: boolean
  saving: boolean
  verifying: boolean
  onClose: () => void
  onDiscover: (config: ModelConfig) => Promise<RuntimeModelDiscoveryResponse>
  onSave: (config: ModelConfig) => Promise<void>
  onVerify: (config: ModelConfig) => Promise<RuntimeModelVerifyResponse>
}

export function ModelSettingsDrawer({
  config,
  discovering,
  open,
  saving,
  verifying,
  onClose,
  onDiscover,
  onSave,
  onVerify,
}: ModelSettingsDrawerProps) {
  const [form] = Form.useForm<ModelConfig>()
  const [availableModels, setAvailableModels] = useState<string[]>([])
  const [selectedModel, setSelectedModel] = useState(config.model)
  const modelOptions = availableModels.map((model) => ({
    value: model,
    label: model,
  }))

  const submitValues = (values: ModelConfig): ModelConfig => ({
    ...values,
    models: availableModels,
  })

  const refreshModels = () => {
    form.validateFields(['protocol', 'url', 'apiKey']).then(async () => {
      const values = form.getFieldsValue()
      const result = await onDiscover(values)
      const nextModels = result.models ?? []
      if (!result.error) {
        const nextModel = result.model || (nextModels.includes(values.model) ? values.model : undefined)
        setAvailableModels(nextModels)
        form.setFieldsValue({ model: nextModel })
        setSelectedModel(nextModel ?? '')
      }
    })
  }

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
          setSelectedModel(config.model)
        }
      }}
      extra={
        <Space>
          <Button
            loading={verifying}
            disabled={!selectedModel}
            onClick={() => {
              form.validateFields().then(async (values) => {
                const result = await onVerify(values)
                const nextModels = result.models?.length ? result.models : result.model ? [result.model] : []
                if (result.ok && nextModels.length > 0) {
                  setAvailableModels(nextModels)
                  form.setFieldsValue({ model: result.model })
                  setSelectedModel(result.model)
                }
              })
            }}
          >
            Verify
          </Button>
          <Button
            type="primary"
            loading={saving}
            disabled={!selectedModel}
            onClick={() => {
              form.validateFields().then((values) => onSave(submitValues(values)))
            }}
          >
            Save
          </Button>
        </Space>
      }
    >
      <Paragraph type="secondary">Saved to the desktop config directory beside the application.</Paragraph>
      <Form
        form={form}
        layout="vertical"
        initialValues={config}
        onValuesChange={(changed) => {
          if ('protocol' in changed || 'url' in changed || 'apiKey' in changed) {
            setAvailableModels([])
            setSelectedModel('')
            form.setFieldsValue({ model: '' })
          }
        }}
      >
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
        <Form.Item
          label={
            <Space size={8}>
              <span>Model</span>
              <Tooltip title="Refresh models">
                <Button
                  aria-label="Refresh models"
                  icon={<ReloadOutlined />}
                  loading={discovering}
                  onClick={(event) => {
                    event.preventDefault()
                    refreshModels()
                  }}
                  size="small"
                  type="text"
                />
              </Tooltip>
            </Space>
          }
          name="model"
          rules={[{ required: true, message: 'Refresh models and select one.' }]}
        >
          <Select
            showSearch
            placeholder="Refresh models first"
            options={modelOptions}
            notFoundContent="Refresh models after entering protocol, URL, and API key."
            onChange={(value) => setSelectedModel(value)}
          />
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
