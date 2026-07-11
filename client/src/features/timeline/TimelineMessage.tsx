import { useEffect, useState } from 'react';
import { CheckOutlined, CopyOutlined } from '@ant-design/icons';
import { Button, Tag, Tooltip, message } from 'antd';
import Bubble from '@ant-design/x/es/bubble';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { MarkdownMessage } from '../markdown/MarkdownMessage.tsx';
import styles from './Timeline.module.css';

export function TimelineMessage({ item, messageApi }: { item: ConversationTimelineItemViewModel; messageApi: ReturnType<typeof message.useMessage>[0] }) {
  const streaming = Boolean(item.streaming);
  const displayContent = streaming ? completePartialMarkdown(item.content ?? '') : item.content;
  return (
    <Bubble
      className={[item.role === 'user' ? styles.userBubble : styles.assistantBubble, streaming ? styles.streamingBubble : undefined].filter(Boolean).join(' ')}
      typing={false}
      content={
        <span data-testid="timeline-message" data-streaming={streaming ? 'true' : undefined}>
          <MarkdownMessage content={displayContent} role={item.role} />
          {streaming ? <span className={styles.streamingCursor} aria-hidden="true">▌</span> : null}
        </span>
      }
      placement={item.role === 'user' ? 'end' : 'start'}
      variant={item.role === 'user' ? 'filled' : 'borderless'}
      footer={
        (item.role === 'user' || item.role === 'assistant') && isCompleteMessage(item)
          ? <MessageFooter align={item.role === 'user' ? 'end' : 'start'} content={item.content ?? ''} createdAt={item.createdAt} messageApi={messageApi} />
          : item.status === 'error' ? <Tag color="error">失败</Tag> : undefined
      }
    />
  );
}

function completePartialMarkdown(content: string) {
  const fenceMatches = content.match(/```/g);
  return fenceMatches && fenceMatches.length % 2 === 1 ? `${content}\n\`\`\`` : content;
}

function isCompleteMessage(item: ConversationTimelineItemViewModel) {
  return !item.streaming && (item.status === 'success' || item.status === 'error' || item.status === 'completed');
}

function MessageFooter({ align, content, createdAt, messageApi }: { align: 'start' | 'end'; content: string; createdAt?: number; messageApi: ReturnType<typeof message.useMessage>[0] }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(false), 1200);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const copyMessage = async () => {
    try {
      await copyText(content);
      setCopied(true);
      void messageApi.success('已复制');
    } catch {
      void messageApi.error('复制失败');
    }
  };

  return (
    <div className={`${styles.messageFooter} ${align === 'end' ? styles.userMessageFooter : styles.assistantMessageFooter}`}>
      <span className={styles.messageTime}>{formatMessageTime(createdAt)}</span>
      <Tooltip title={copied ? '已复制' : '复制'}>
        <Button aria-label={copied ? '已复制' : '复制消息'} className={styles.copyButton} icon={copied ? <CheckOutlined /> : <CopyOutlined />} size="small" type="text" onClick={copyMessage} />
      </Tooltip>
    </div>
  );
}

async function copyText(text: string) {
  if (typeof document !== 'undefined' && copyTextWithSelection(text)) return;
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await withTimeout(navigator.clipboard.writeText(text), 300);
      return;
    } catch {
      // Embedded webviews may expose Clipboard API but reject writes.
    }
  }
  if (typeof document === 'undefined' || !copyTextWithSelection(text)) throw new Error('clipboard is unavailable');
}

function copyTextWithSelection(text: string) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-1000px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);
  try {
    return document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number) {
  return new Promise<T>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('clipboard write timed out')), timeoutMs);
    promise.then(
      (value) => { window.clearTimeout(timer); resolve(value); },
      (error: unknown) => { window.clearTimeout(timer); reject(error); },
    );
  });
}

function formatMessageTime(createdAt?: number) {
  if (!createdAt) return '';
  const milliseconds = createdAt < 1_000_000_000_000 ? createdAt * 1000 : createdAt;
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' }).format(new Date(milliseconds));
}
