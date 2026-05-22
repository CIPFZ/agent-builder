import Button from 'antd/es/button'
import Dropdown from 'antd/es/dropdown'
import Flex from 'antd/es/flex'
import Space from 'antd/es/space'
import Tooltip from 'antd/es/tooltip'
import Typography from 'antd/es/typography'
import {
  ApiOutlined,
  AppstoreOutlined,
  DeleteOutlined,
  DownOutlined,
  EditOutlined,
  MenuOutlined,
  MessageOutlined,
  PlusOutlined,
  SearchOutlined,
  SettingOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import type { RuntimeSession } from '../../runtime'
import type { RuntimeFeatureView } from '../capabilities/RuntimeFeatureWorkspace'

const { Text } = Typography

type ChatSidebarProps = {
  activeView: RuntimeFeatureView | 'chat'
  collapsed: boolean
  sessions: RuntimeSession[]
  onDeleteSession: (session: RuntimeSession) => void
  onOpenSettings: () => void
  onOpenView: (view: RuntimeFeatureView) => void
  onRenameSession: (session: RuntimeSession) => void
  onSearch: () => void
  onSelectSession: (sessionId: string) => void
  onStartNewChat: () => void
  onToggleCollapsed: () => void
}

export function ChatSidebar({
  activeView,
  collapsed,
  sessions,
  onDeleteSession,
  onOpenSettings,
  onOpenView,
  onRenameSession,
  onSearch,
  onSelectSession,
  onStartNewChat,
  onToggleCollapsed,
}: ChatSidebarProps) {
  if (collapsed) return null

  return (
    <aside className="sidebar">
      <div className="sidebar-top">
        <Flex justify="space-between" align="center">
          <Space size={10}>
            <Tooltip title="Hide sidebar">
              <Button type="text" icon={<MenuOutlined />} onClick={onToggleCollapsed} />
            </Tooltip>
            <Tooltip title="Search">
              <Button type="text" icon={<SearchOutlined />} onClick={onSearch} />
            </Tooltip>
          </Space>
          <Tooltip title="New chat">
            <Button type="text" icon={<PlusOutlined />} onClick={onStartNewChat} />
          </Tooltip>
        </Flex>

        <nav className="nav-list">
          <button className={activeView === 'chat' ? 'nav-item active' : 'nav-item'} type="button" onClick={onStartNewChat}>
            <MessageOutlined />
            <span>New chat</span>
          </button>
          <button className={activeView === 'skills' ? 'nav-item active' : 'nav-item'} type="button" onClick={() => onOpenView('skills')}>
            <ToolOutlined />
            <span>Skills</span>
          </button>
          <button className={activeView === 'plugins' ? 'nav-item active' : 'nav-item'} type="button" onClick={() => onOpenView('plugins')}>
            <AppstoreOutlined />
            <span>Plugins</span>
          </button>
          <button className={activeView === 'mcp' ? 'nav-item active' : 'nav-item'} type="button" onClick={() => onOpenView('mcp')}>
            <ApiOutlined />
            <span>MCP</span>
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
        <Tooltip title="Settings">
          <Button type="text" size="small" icon={<SettingOutlined />} onClick={onOpenSettings} />
        </Tooltip>
      </div>
    </aside>
  )
}
