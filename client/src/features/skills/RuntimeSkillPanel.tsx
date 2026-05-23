import { useState } from 'react'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import Button from 'antd/es/button'
import Form from 'antd/es/form'
import Input from 'antd/es/input'
import Modal from 'antd/es/modal'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import TextArea from 'antd/es/input/TextArea'
import Typography from 'antd/es/typography'
import type { RuntimeSkill, RuntimeSkillCreateRequest } from '../../runtime'

const { Text } = Typography

function skillStateColor(state: string, enabled: boolean) {
  if (!enabled || state === 'disabled') return 'default'
  if (state === 'loaded') return 'green'
  if (state === 'loading') return 'processing'
  if (state === 'failed' || state === 'unavailable') return 'red'
  return 'gold'
}

export function RuntimeSkillPanel({
  skills,
  onRefresh,
  onCreate,
  onAddPath,
  onToggle,
}: {
  skills: RuntimeSkill[]
  onRefresh: () => Promise<void>
  onCreate: (request: RuntimeSkillCreateRequest) => Promise<void>
  onAddPath: (path: string) => Promise<void>
  onToggle: (name: string, enabled: boolean) => Promise<void>
}) {
  const [createOpen, setCreateOpen] = useState(false)
  const [pathOpen, setPathOpen] = useState(false)
  const [skillForm] = Form.useForm<RuntimeSkillCreateRequest>()
  const [pathForm] = Form.useForm<{ path: string }>()

  return (
    <div className="runtime-list">
      <Space wrap>
        <Button size="small" icon={<ReloadOutlined />} onClick={() => onRefresh()}>
          Refresh skills
        </Button>
        <Button size="small" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          Create skill
        </Button>
        <Button size="small" onClick={() => setPathOpen(true)}>
          Add path
        </Button>
      </Space>
      {skills.length === 0 ? <Text type="secondary">No skills discovered.</Text> : null}
      {skills.slice(0, 12).map((skill) => (
        <div className="runtime-list-row" key={`${skill.name}-${skill.path ?? ''}`}>
          <Space size={8}>
            <Tag color={skill.enabled ? 'green' : 'default'}>{skill.enabled ? 'enabled' : 'disabled'}</Tag>
            <Tag color={skillStateColor(skill.state, skill.enabled)}>{skill.state}</Tag>
            <Text strong>{skill.name}</Text>
            {skill.builtin ? <Tag>builtin</Tag> : null}
            {skill.allowed_tools?.length ? <Tag>tools {skill.allowed_tools.length}</Tag> : null}
            <Button size="small" onClick={() => onToggle(skill.name, !skill.enabled)}>
              {skill.enabled ? 'Disable' : 'Enable'}
            </Button>
          </Space>
          {skill.error || skill.diagnostics ? <Text type="danger">{skill.error || skill.diagnostics}</Text> : <Text type="secondary">{skill.description}</Text>}
          {skill.activation?.reason || skill.activation_metadata?.reason || skill.reason ? (
            <Text type="secondary">{skill.activation?.reason || skill.activation_metadata?.reason || skill.reason}</Text>
          ) : null}
          {skill.policy_reason ? <Text type="secondary">{skill.policy_reason}</Text> : null}
        </div>
      ))}
      <Modal
        title="Create skill"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => {
          skillForm
            .validateFields()
            .then((values) => onCreate(values))
            .then(() => {
              skillForm.resetFields()
              setCreateOpen(false)
            })
            .catch(() => undefined)
        }}
      >
        <Form form={skillForm} layout="vertical" initialValues={{ directory: '.agents/skills' }}>
          <Form.Item label="Name" name="name" rules={[{ required: true }]}>
            <Input placeholder="my-skill" />
          </Form.Item>
          <Form.Item label="Directory" name="directory">
            <Input placeholder=".agents/skills" />
          </Form.Item>
          <Form.Item label="Description" name="description" rules={[{ required: true }]}>
            <Input placeholder="Use when..." />
          </Form.Item>
          <Form.Item label="Instructions" name="instructions" rules={[{ required: true }]}>
            <TextArea rows={6} placeholder="# My Skill&#10;&#10;Steps..." />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title="Add skill path"
        open={pathOpen}
        onCancel={() => setPathOpen(false)}
        onOk={() => {
          pathForm
            .validateFields()
            .then(({ path }) => onAddPath(path))
            .then(() => {
              pathForm.resetFields()
              setPathOpen(false)
            })
            .catch(() => undefined)
        }}
      >
        <Form form={pathForm} layout="vertical">
          <Form.Item label="Path" name="path" rules={[{ required: true }]}>
            <Input placeholder=".agents/skills" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
