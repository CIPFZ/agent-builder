import { LoadingOutlined } from '@ant-design/icons';
import type { ConversationTimelineItemViewModel } from '../../runtime/workbenchTypes.ts';
import { MarkdownMessage } from '../markdown/MarkdownMessage.tsx';
import styles from './Timeline.module.css';

export function ProcessNarration({ item }: { item: ConversationTimelineItemViewModel }) {
  const content = item.content?.trim();
  if (content) {
    return <div className={styles.processNarration} data-testid="process-narration"><MarkdownMessage content={content} role="assistant" /></div>;
  }
  if (item.source === 'react_callchain' && item.title) {
    return <div className={styles.processNarrationMuted} data-testid="process-narration">{item.title}</div>;
  }
  if (item.status === 'running' || item.status === 'streaming') {
    return <div className={styles.processNarrationMuted} data-testid="process-narration"><LoadingOutlined spin /> 正在思考…</div>;
  }
  return null;
}
