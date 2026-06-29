import { WarningOutlined } from '@ant-design/icons';
import { Card, Space, Tag, Typography } from 'antd';
import type { RecoverableErrorViewModel, RecoveryActionViewModel } from '../../runtime/workbenchTypes.ts';
import { RecoveryActions } from './RecoveryActions.tsx';
import styles from './RecoveryCenter.module.css';

interface RecoveryErrorCardProps {
  error: RecoverableErrorViewModel;
  actions: RecoveryActionViewModel[];
  onResumeTurn: (turnID: string) => void;
  onMarkDone: (turnID: string) => void;
  onDiscardTurn: (turnID: string) => void;
  onRetryError: (errorID: string) => void;
  onResumeCheckpoint: (runID: string, checkpointID: string) => void;
}

export function RecoveryErrorCard(props: RecoveryErrorCardProps) {
  const { error, actions } = props;
  return (
    <Card className={styles.card} size="small">
      <Space align="start" className={styles.cardHeader}>
        <WarningOutlined className={styles.cardIcon} />
        <div className={styles.cardMain}>
          <Space wrap size={6}>
            <Typography.Text strong>{error.kind}</Typography.Text>
            {error.provider ? <Tag>{error.provider}</Tag> : null}
            {error.model ? <Tag>{error.model}</Tag> : null}
            {error.compactEligible ? <Tag color="blue">compact</Tag> : null}
            {error.retryEligible ? <Tag color="green">retry</Tag> : null}
          </Space>
          <Typography.Paragraph className={styles.reason} ellipsis={{ rows: 3 }}>
            {error.message || error.userAction || error.id}
          </Typography.Paragraph>
          <RecoveryActions {...props} actions={actions} />
        </div>
      </Space>
    </Card>
  );
}
