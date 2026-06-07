import { useCallback, useEffect, useRef, useState } from 'react';
import { ArrowDownOutlined, CodeOutlined, ControlOutlined, CopyOutlined, DesktopOutlined, EditOutlined, MoreOutlined } from '@ant-design/icons';
import { Button, Dropdown, Input, Modal, Tooltip, message as antdMessage } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Composer } from '../composer/Composer.tsx';
import { TurnDiagnosticsPanel } from '../diagnostics/TurnDiagnosticsPanel.tsx';
import { Timeline } from '../timeline/Timeline.tsx';
import styles from './Workspace.module.css';

interface WorkspaceProps {
  sidebarCollapsed?: boolean;
  viewModel: WorkbenchViewModel;
  onModelSelect: (configuredProviderID: string, model: string) => Promise<void>;
  onPermissionDecide: (permissionID: string, action: 'allow' | 'allow_for_session' | 'deny') => Promise<void>;
  onPermissionModeSelect: (mode: string) => Promise<void>;
  onPromptCancel: () => Promise<void>;
  onSessionRename: (sessionID: string, title: string) => Promise<void>;
  onPromptSubmit: (prompt: string) => Promise<void>;
}

export function Workspace({
  sidebarCollapsed = false,
  viewModel,
  onModelSelect,
  onPermissionDecide,
  onPermissionModeSelect,
  onPromptCancel,
  onSessionRename,
  onPromptSubmit,
}: WorkspaceProps) {
  const [messageApi, messageContextHolder] = antdMessage.useMessage();
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameTitle, setRenameTitle] = useState('');
  const [renaming, setRenaming] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);
  const hasProjectContext = Boolean(viewModel.currentProject.id || viewModel.currentProject.name || viewModel.currentProject.path);
  const hasConversation = viewModel.conversation.length > 0;
  const activeSession = viewModel.sessions.find((session) => session.active);
  const sessionTitle = activeSession?.title || viewModel.currentProject.name || '新对话';
  const title =
    viewModel.mode === 'project' && hasProjectContext ? `我们应该在 ${viewModel.currentProject.name} 中构建什么？` : '我们该做什么？';
  const bubbleItems = viewModel.conversation.map((message) => ({
    key: message.id,
    role: message.role === 'user' ? 'user' : 'ai',
    content: message.content,
    status: message.status,
    footer: <MessageActions content={message.content} createdAt={message.createdAt} messageApi={messageApi} role={message.role} />,
  }));
  const updateJumpToBottomVisibility = useCallback(() => {
    const container = scrollContainerRef.current;
    if (!container) {
      setShowJumpToBottom(false);
      return;
    }
    const distanceToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
    setShowJumpToBottom(distanceToBottom > 180);
  }, []);
  const jumpToBottom = () => {
    scrollContainerRef.current?.scrollTo({
      top: scrollContainerRef.current.scrollHeight,
      behavior: 'smooth',
    });
  };
  useEffect(() => {
    const frame = window.requestAnimationFrame(updateJumpToBottomVisibility);
    return () => window.cancelAnimationFrame(frame);
  }, [updateJumpToBottomVisibility, viewModel.conversation.length, viewModel.timeline.length]);
  const openRenameDialog = () => {
    if (!activeSession) {
      void messageApi.warning('请先选择一个对话');
      return;
    }
    setRenameTitle(activeSession.title);
    setRenameOpen(true);
  };
  const submitRename = async () => {
    if (!activeSession) {
      setRenameOpen(false);
      return;
    }
    const nextTitle = renameTitle.trim();
    if (!nextTitle) {
      void messageApi.warning('请输入对话名称');
      return;
    }
    if (nextTitle === activeSession.title) {
      setRenameOpen(false);
      return;
    }
    setRenaming(true);
    try {
      await onSessionRename(activeSession.id, nextTitle);
      setRenameOpen(false);
      void messageApi.success('对话名称已更新');
    } catch {
      void messageApi.error('更新对话名称失败');
    } finally {
      setRenaming(false);
    }
  };

  return (
    <section
      className={styles.workspace}
      data-has-conversation={hasConversation}
      data-mode={viewModel.mode}
      data-sidebar-collapsed={sidebarCollapsed ? 'true' : 'false'}
    >
      {messageContextHolder}
      <header className={styles.sessionHeader}>
        <div className={styles.sessionTitleWrap}>
          <h2 className={styles.sessionTitle}>{sessionTitle}</h2>
          <Dropdown
            menu={{
              items: [{ key: 'rename', icon: <EditOutlined />, label: '重命名' }],
              onClick: ({ key }) => {
                if (key === 'rename') {
                  openRenameDialog();
                }
              },
            }}
            trigger={['click']}
          >
            <Button aria-label="更多对话操作" className={styles.headerIconButton} icon={<MoreOutlined />} type="text" />
          </Dropdown>
        </div>
        <div className={styles.headerActions} aria-label="工作区面板">
          <Tooltip title="打开代码面板">
            <Button aria-label="打开代码面板" className={styles.headerIconButton} icon={<CodeOutlined />} type="text" />
          </Tooltip>
          <Tooltip title="打开任务列表">
            <Button aria-label="打开任务列表" className={styles.headerIconButton} icon={<ControlOutlined />} type="text" />
          </Tooltip>
          <Tooltip title="打开右侧面板">
            <Button aria-label="打开右侧面板" className={styles.headerIconButton} icon={<DesktopOutlined />} type="text" />
          </Tooltip>
        </div>
      </header>
      <Modal
        cancelText="取消"
        confirmLoading={renaming}
        okText="保存"
        open={renameOpen}
        title="重命名对话"
        onCancel={() => {
          if (!renaming) {
            setRenameOpen(false);
          }
        }}
        onOk={submitRename}
      >
        <Input
          aria-label="对话名称"
          autoFocus
          maxLength={80}
          placeholder="请输入对话名称"
          showCount
          value={renameTitle}
          onChange={(event) => setRenameTitle(event.target.value)}
          onPressEnter={() => {
            void submitRename();
          }}
        />
      </Modal>
      <div
        ref={hasConversation ? scrollContainerRef : undefined}
        className={hasConversation ? styles.chatContent : styles.content}
        onScroll={hasConversation ? updateJumpToBottomVisibility : undefined}
      >
        {viewModel.timeline.length > 0 ? (
          <div className={styles.timelineLayout}>
            <div className={styles.timelineColumn}>
              <Timeline items={viewModel.timeline} onPermissionDecide={onPermissionDecide} />
            </div>
            <div className={styles.diagnosticsColumn}>
              <TurnDiagnosticsPanel diagnostics={viewModel.turnDiagnostics} />
            </div>
          </div>
        ) : hasConversation ? (
          <Bubble.List
            autoScroll
            className={styles.conversation}
            items={bubbleItems}
            role={{
              ai: {
                placement: 'start',
                variant: 'borderless',
                className: styles.assistantBubble,
              },
              user: {
                placement: 'end',
                variant: 'filled',
                className: styles.userBubble,
              },
            }}
          />
        ) : (
          <h1 className={styles.title}>{title}</h1>
        )}
        {hasConversation && showJumpToBottom && (
          <button
            aria-label="跳到底部"
            className={styles.jumpToBottomButton}
            type="button"
            onClick={jumpToBottom}
          >
            <ArrowDownOutlined />
          </button>
        )}
        <Composer
          composer={viewModel.composer}
          project={viewModel.currentProject}
          showProjectContext={viewModel.mode === 'project' && hasProjectContext && !hasConversation}
          onModelSelect={onModelSelect}
          onPermissionModeSelect={onPermissionModeSelect}
          onCancel={onPromptCancel}
          onSubmit={onPromptSubmit}
        />
      </div>
    </section>
  );
}

function MessageActions({
  content,
  createdAt,
  messageApi,
  role,
}: {
  content: string;
  createdAt?: number;
  messageApi: ReturnType<typeof antdMessage.useMessage>[0];
  role: string;
}) {
  const copyMessage = async () => {
    try {
      await copyText(content);
      void messageApi.success('已复制');
    } catch {
      void messageApi.error('复制失败');
    }
  };
  const sentAt = role === 'user' ? formatMessageTime(createdAt) : '';

  return (
    <div className={role === 'user' ? styles.userMessageActions : styles.assistantMessageActions}>
      <Tooltip title="复制">
        <Button aria-label="复制" icon={<CopyOutlined />} size="small" type="text" onClick={copyMessage} />
      </Tooltip>
      {sentAt && <span className={styles.messageTime}>{sentAt}</span>}
    </div>
  );
}

async function copyText(text: string) {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  if (typeof document === 'undefined') {
    throw new Error('clipboard is unavailable');
  }
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-1000px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  try {
    const copied = document.execCommand('copy');
    if (!copied) {
      throw new Error('copy command failed');
    }
  } finally {
    document.body.removeChild(textarea);
  }
}

function formatMessageTime(createdAt?: number) {
  if (!createdAt) {
    return '';
  }
  const milliseconds = createdAt < 1_000_000_000_000 ? createdAt * 1000 : createdAt;
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(milliseconds));
}
