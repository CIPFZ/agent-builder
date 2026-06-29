import { AlertOutlined } from '@ant-design/icons';
import { Card, Space, Tag, Typography } from 'antd';
import type { RecoveredTurnViewModel, RecoveryActionViewModel } from '../../runtime/workbenchTypes.ts';
import { RecoveryActions } from './RecoveryActions.tsx';
import styles from './RecoveryCenter.module.css';

interface RecoveryTurnCardProps {
  turn: RecoveredTurnViewModel;
  actions: RecoveryActionViewModel[];
  onResumeTurn: (turnID: string) => void;
  onMarkDone: (turnID: string) => void;
  onDiscardTurn: (turnID: string) => void;
  onRetryError: (errorID: string) => void;
  onResumeCheckpoint: (runID: string, checkpointID: string) => void;
}

export function RecoveryTurnCard(props: RecoveryTurnCardProps) {
  const { turn, actions } = props;
  return (
    <Card className={styles.card} size="small">
      <Space align="start" className={styles.cardHeader}>
        <AlertOutlined className={styles.cardIcon} />
        <div className={styles.cardMain}>
          <Space wrap size={6}>
            <Typography.Text strong>{turn.interruptionKind}</Typography.Text>
            <Tag>{turn.status}</Tag>
            {turn.openToolCalls.length ? <Tag color="gold">{turn.openToolCalls.length} open tools</Tag> : null}
          </Space>
          <Typography.Paragraph className={styles.reason} ellipsis={{ rows: 2 }}>
            {turn.reason || turn.resumeHint || turn.promptPreview || turn.id}
          </Typography.Paragraph>
          {turn.openToolCalls.length ? (
            <div className={styles.metaLine}>
              {turn.openToolCalls.map((tool) => tool.display?.title || tool.name || tool.id).join(', ')}
            </div>
          ) : null}
          <RecoveryActions {...props} actions={actions} />
        </div>
      </Space>
    </Card>
  );
}
