import Button from 'antd/es/button'
import Modal from 'antd/es/modal'
import Space from 'antd/es/space'
import Tag from 'antd/es/tag'
import Typography from 'antd/es/typography'
import type { RuntimePermissionDecision, RuntimePermissionRequest } from '../../runtime'

const { Paragraph } = Typography

export function PermissionReviewModal({
  permissions,
  onDecide,
}: {
  permissions: RuntimePermissionRequest[]
  onDecide: (permissionId: string, action: RuntimePermissionDecision['action']) => Promise<void>
}) {
  const permission = permissions[0]

  return (
    <Modal
      title="Tool permission"
      open={Boolean(permission)}
      closable={false}
      maskClosable={false}
      footer={
        permission
          ? [
              <Button key="deny" danger onClick={() => onDecide(permission.id, 'deny')}>
                Deny
              </Button>,
              <Button key="allow-session" onClick={() => onDecide(permission.id, 'allow_session')}>
                Allow session
              </Button>,
              <Button key="allow" type="primary" onClick={() => onDecide(permission.id, 'allow')}>
                Allow once
              </Button>,
            ]
          : null
      }
      width={640}
    >
      {permission ? (
        <div className="permission-review">
          <Space wrap>
            <Tag>{permission.toolName}</Tag>
            <Tag>{permission.action}</Tag>
            {permission.path ? <Tag>{permission.path}</Tag> : null}
          </Space>
          {permission.description ? <Paragraph>{permission.description}</Paragraph> : null}
          {permission.params ? <pre className="part-preview">{JSON.stringify(permission.params, null, 2)}</pre> : null}
        </div>
      ) : null}
    </Modal>
  )
}
