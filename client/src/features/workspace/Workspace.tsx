import { CopyOutlined } from '@ant-design/icons';
import { Button, Tooltip, message as antdMessage } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Composer } from '../composer/Composer.tsx';
import { Timeline } from '../timeline/Timeline.tsx';
import styles from './Workspace.module.css';

interface WorkspaceProps {
  viewModel: WorkbenchViewModel;
  onModelSelect: (configuredProviderID: string, model: string) => Promise<void>;
  onPermissionDecide: (permissionID: string, action: 'allow' | 'allow_for_session' | 'deny') => Promise<void>;
  onPermissionModeSelect: (mode: string) => Promise<void>;
  onPromptCancel: () => Promise<void>;
  onPromptSubmit: (prompt: string) => Promise<void>;
}

export function Workspace({
  viewModel,
  onModelSelect,
  onPermissionDecide,
  onPermissionModeSelect,
  onPromptCancel,
  onPromptSubmit,
}: WorkspaceProps) {
  const [messageApi, messageContextHolder] = antdMessage.useMessage();
  const hasProjectContext = Boolean(viewModel.currentProject.id || viewModel.currentProject.name || viewModel.currentProject.path);
  const hasConversation = viewModel.conversation.length > 0;
  const title =
    viewModel.mode === 'project' && hasProjectContext ? `我们应该在 ${viewModel.currentProject.name} 中构建什么？` : '我们该做什么？';
  const bubbleItems = viewModel.conversation.map((message) => ({
    key: message.id,
    role: message.role === 'user' ? 'user' : 'ai',
    content: message.content,
    status: message.status,
    footer: <MessageActions content={message.content} createdAt={message.createdAt} messageApi={messageApi} role={message.role} />,
  }));

  return (
    <section className={styles.workspace} data-has-conversation={hasConversation} data-mode={viewModel.mode}>
      {messageContextHolder}
      <div className={hasConversation ? styles.chatContent : styles.content}>
        {viewModel.timeline.length > 0 ? (
          <Timeline items={viewModel.timeline} onPermissionDecide={onPermissionDecide} />
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
