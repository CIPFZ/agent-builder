import { SafetyCertificateOutlined } from '@ant-design/icons';
import { Empty, Space, Typography } from 'antd';
import type { RecoveryStatusViewModel } from '../../runtime/workbenchTypes.ts';
import { RecoveryErrorCard } from './RecoveryErrorCard.tsx';
import { RecoveryTurnCard } from './RecoveryTurnCard.tsx';
import styles from './RecoveryCenter.module.css';

interface RecoveryCenterProps {
  recovery?: RecoveryStatusViewModel;
  onResumeTurn: (turnID: string) => void;
  onMarkDone: (turnID: string) => void;
  onDiscardTurn: (turnID: string) => void;
  onRetryError: (errorID: string) => void;
  onResumeCheckpoint: (runID: string, checkpointID: string) => void;
}

export function RecoveryCenter(props: RecoveryCenterProps) {
  const recovery = props.recovery;
  const turns = recovery?.interruptedTurns ?? [];
  const errors = recovery?.recoverableErrors ?? [];
  const actions = recovery?.actions ?? [];
  const hasItems = turns.length > 0 || errors.length > 0;
  return (
    <section className={styles.center} data-testid="recovery-center">
      <div className={styles.titleRow}>
        <SafetyCertificateOutlined />
        <Typography.Text strong>Recovery</Typography.Text>
      </div>
      {!hasItems ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No recoverable runtime state" />
      ) : (
        <Space className={styles.stack} direction="vertical" size={8}>
          {turns.map((turn) => (
            <RecoveryTurnCard
              key={turn.id}
              {...props}
              turn={turn}
              actions={actions.filter((action) => action.turnId === turn.id)}
            />
          ))}
          {errors.map((error) => (
            <RecoveryErrorCard
              key={error.id}
              {...props}
              error={error}
              actions={actions.filter((action) => action.id === `retry:${error.id}` || action.turnId === error.turnId)}
            />
          ))}
        </Space>
      )}
    </section>
  );
}
