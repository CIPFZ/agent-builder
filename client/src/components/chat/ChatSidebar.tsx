import Button from 'antd/es/button'
import Dropdown from 'antd/es/dropdown'
import Flex from 'antd/es/flex'
import Space from 'antd/es/space'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import {
  ArrowDownOutlined,
  CodeOutlined,
  DeleteOutlined,
  DownOutlined,
  EditOutlined,
  FolderOutlined,
  MenuOutlined,
  MessageOutlined,
  PlusOutlined,
  ProjectOutlined,
  SearchOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import type { RuntimeSession } from '../../runtime'

const { Text } = Typography

type ChatSidebarProps = {
  sessions: RuntimeSession[]
  onDeleteSession: (session: RuntimeSession) => void
  onOpenOperations: () => void
  onOpenSettings: () => void
  onRenameSession: (session: RuntimeSession) => void
  onSelectSession: (sessionId: string) => void
  onStartNewChat: () => void
}

export function ChatSidebar({
  sessions,
  onDeleteSession,
  onOpenOperations,
  onOpenSettings,
  onRenameSession,
  onSelectSession,
  onStartNewChat,
}: ChatSidebarProps) {
  return (
    <aside className="sidebar">
      <div className="sidebar-top">
        <Flex justify="space-between" align="center">
          <Space size={14}>
            <Button type="text" icon={<MenuOutlined />} />
            <Button type="text" icon={<SearchOutlined />} />
          </Space>
          <Tooltip title="New chat">
            <Button type="text" icon={<PlusOutlined />} onClick={onStartNewChat} />
          </Tooltip>
        </Flex>

        <div className="mode-switch">
          <button className="mode-tab active" type="button">
            <MessageOutlined />
            Chat
          </button>
          <button className="mode-tab" type="button" onClick={onOpenOperations}>
            <CodeOutlined />
            Ops
          </button>
        </div>

        <nav className="nav-list">
          <button className="nav-item active" type="button" onClick={onStartNewChat}>
            <PlusOutlined />
            New chat
          </button>
          <button className="nav-item" type="button">
            <FolderOutlined />
            Projects
          </button>
          <button className="nav-item" type="button" onClick={onOpenOperations}>
            <ProjectOutlined />
            Operations
          </button>
          <button className="nav-item" type="button" onClick={onOpenSettings}>
            <SettingOutlined />
            Model settings
          </button>
        </nav>
      </div>

      <div className="recents">
        <Text className="section-label">Recents</Text>
        {sessions.length === 0 ? (
          <Text className="empty-recents" type="secondary">
            No saved sessions
          </Text>
        ) : (
          sessions.map((session) => (
            <div className={session.active ? 'recent-row active' : 'recent-row'} key={session.id}>
              <button className="recent-item" type="button" onClick={() => onSelectSession(session.id)}>
                <span className="recent-title">{session.title || 'New chat'}</span>
                <span className="recent-meta">{session.messageCount} messages</span>
              </button>
              <Dropdown
                menu={{
                  items: [
                    {
                      key: 'rename',
                      icon: <EditOutlined />,
                      label: 'Rename',
                      onClick: () => onRenameSession(session),
                    },
                    {
                      key: 'delete',
                      danger: true,
                      icon: <DeleteOutlined />,
                      label: 'Delete',
                      onClick: () => onDeleteSession(session),
                    },
                  ],
                }}
                trigger={['click']}
              >
                <Button type="text" size="small" icon={<DownOutlined />} onClick={(event) => event.stopPropagation()} />
              </Dropdown>
            </div>
          ))
        )}
      </div>

      <div className="sidebar-footer">
        <Space>
          <span className="avatar-dot">A</span>
          <span>Agent Builder</span>
          <Text type="secondary">Local</Text>
        </Space>
        <Tooltip title="Runtime logs">
          <Button type="text" size="small" icon={<ArrowDownOutlined />} onClick={onOpenOperations} />
        </Tooltip>
      </div>
    </aside>
  )
}
