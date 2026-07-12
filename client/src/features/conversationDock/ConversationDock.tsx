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

  return (
    <div className={styles.dock} data-testid="conversation-dock">
      {activeActions.length > 0 ? <ConversationActions actions={activeActions} /> : null}
      <div className={styles.content}>{children}</div>
    </div>
  );
}
