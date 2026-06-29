import { CheckOutlined, DeleteOutlined, PlayCircleOutlined, ReloadOutlined, StepForwardOutlined } from '@ant-design/icons';
import { Button, Space, Tooltip } from 'antd';
import type { RecoveryActionViewModel } from '../../runtime/workbenchTypes.ts';

interface RecoveryActionsProps {
  actions: RecoveryActionViewModel[];
  onResumeTurn: (turnID: string) => void;
  onMarkDone: (turnID: string) => void;
  onDiscardTurn: (turnID: string) => void;
  onRetryError: (errorID: string) => void;
  onResumeCheckpoint: (runID: string, checkpointID: string) => void;
}

export function RecoveryActions({
  actions,
  onResumeTurn,
  onMarkDone,
  onDiscardTurn,
  onRetryError,
  onResumeCheckpoint,
}: RecoveryActionsProps) {
  if (actions.length === 0) {
    return null;
  }
  return (
    <Space wrap size={6}>
      {actions.map((action) => {
        const danger = Boolean(action.destructive);
        const click = () => {
          if (action.kind === 'resume_interrupted_turn' && action.turnId) {
            onResumeTurn(action.turnId);
          } else if (action.kind === 'mark_interrupted_done' && action.turnId) {
            onMarkDone(action.turnId);
          } else if (action.kind === 'discard_interrupted_turn' && action.turnId) {
            onDiscardTurn(action.turnId);
          } else if (action.kind === 'retry_recoverable_error') {
            onRetryError(action.id.replace(/^retry:/, ''));
          } else if (action.kind === 'resume_checkpoint' && action.runId && action.checkpointId) {
            onResumeCheckpoint(action.runId, action.checkpointId);
          }
        };
        const icon =
          action.kind === 'resume_interrupted_turn' ? <PlayCircleOutlined /> :
          action.kind === 'mark_interrupted_done' ? <CheckOutlined /> :
          action.kind === 'discard_interrupted_turn' ? <DeleteOutlined /> :
          action.kind === 'retry_recoverable_error' ? <ReloadOutlined /> :
          <StepForwardOutlined />;
        return (
          <Tooltip key={action.id} title={action.evidence.join(', ')}>
            <Button danger={danger} icon={icon} size="small" type={action.startsWorker && !danger ? 'primary' : 'default'} onClick={click}>
              {action.label}
            </Button>
          </Tooltip>
        );
      })}
    </Space>
  );
}
