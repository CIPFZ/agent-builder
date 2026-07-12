import type { ReactNode } from 'react';
import { ConversationActions, type ConversationAction } from './ConversationActions.tsx';
import styles from './ConversationDock.module.css';

export function ConversationDock({
  actions,
  children,
}: {
  actions?: Array<ConversationAction | false | null | undefined>;
  children: ReactNode;
}) {
  const activeActions = (actions ?? []).filter((action): action is ConversationAction => Boolean(action));
  const floatingActions = activeActions.filter((action) => action.key === 'jump-to-bottom');
  const layoutActions = activeActions.filter((action) => action.key !== 'jump-to-bottom');

  return (
    <div className={styles.dock} data-testid="conversation-dock">
      {layoutActions.length > 0 ? <ConversationActions actions={layoutActions} /> : null}
      {floatingActions.length > 0 ? (
        <div className={styles.floatingActions} data-testid="conversation-floating-actions">
          {floatingActions.map((action) => <span className={styles.action} key={action.key}>{action.node}</span>)}
        </div>
      ) : null}
      <div className={styles.content}>{children}</div>
    </div>
  );
}
