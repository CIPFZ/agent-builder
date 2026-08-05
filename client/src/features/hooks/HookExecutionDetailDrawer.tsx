import { Alert, Descriptions, Drawer, Spin, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';
import type { HookExecutionViewModel } from '../../runtime/workbenchTypes.ts';
import { executionStatusColor } from './hookExecutionUtils.ts';
import styles from './HookExecutionsPanel.module.css';

const { Text } = Typography;

export function HookExecutionDetailDrawer({
  executionId,
  fallback,
  open,
  onClose,
  onLoad,
}: {
  executionId?: string;
  fallback?: HookExecutionViewModel;
  open: boolean;
  onClose: () => void;
  onLoad?: (executionId: string) => Promise<HookExecutionViewModel>;
}) {
  const [loadedExecution, setLoadedExecution] = useState<HookExecutionViewModel | undefined>();
  const [loadingExecutionId, setLoadingExecutionId] = useState<string>();
  const [loadError, setLoadError] = useState<{ executionId: string; message: string }>();
  const execution = loadedExecution?.id === executionId ? loadedExecution : fallback;
  const loading = Boolean(executionId && loadingExecutionId === executionId);
  const error = loadError && loadError.executionId === executionId ? loadError.message : '';
  const releaseDetail = () => {
    setLoadedExecution(undefined);
    setLoadingExecutionId(undefined);
    setLoadError(undefined);
  };

  useEffect(() => {
    if (!open || !executionId || !onLoad) {
      return undefined;
    }
    let cancelled = false;
    void Promise.resolve().then(() => {
      if (!cancelled) {
        setLoadingExecutionId(executionId);
        setLoadError(undefined);
      }
    });
    void onLoad(executionId)
      .then((detail) => {
        if (!cancelled) {
          setLoadedExecution(detail);
        }
      })
      .catch((loadError: unknown) => {
        if (!cancelled) {
          setLoadError({
            executionId,
            message: loadError instanceof Error ? loadError.message : 'Failed to load hook execution detail.',
          });
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingExecutionId((current) => current === executionId ? undefined : current);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [executionId, onLoad, open]);

  return (
    <Drawer destroyOnHidden open={open} title="Hook execution" width={520} afterOpenChange={(visible) => { if (!visible) releaseDetail(); }} onClose={onClose}>
      {loading && !execution ? <Spin /> : null}
      {error ? <Alert type="warning" showIcon message={error} /> : null}
      {execution ? <HookExecutionDetail execution={execution} /> : !loading ? <Text type="secondary">No execution selected.</Text> : null}
    </Drawer>
  );
}

export function HookExecutionDetail({ execution }: { execution: HookExecutionViewModel }) {
  return (
    <>
      <section className={styles.drawerSection}>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="ID"><span className={styles.mono}>{execution.id}</span></Descriptions.Item>
          <Descriptions.Item label="Hook ID"><span className={styles.mono}>{execution.hookId}</span></Descriptions.Item>
          <Descriptions.Item label="Name">{execution.hookName || 'unknown'}</Descriptions.Item>
          <Descriptions.Item label="Source">{execution.hookSource || 'unknown'}</Descriptions.Item>
          <Descriptions.Item label="Event"><Tag color="blue">{execution.event}</Tag></Descriptions.Item>
          <Descriptions.Item label="Status"><Tag color={executionStatusColor(execution.status)}>{execution.status}</Tag></Descriptions.Item>
        </Descriptions>
      </section>

      <section className={styles.drawerSection}>
        <Text strong>Scope</Text>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="Session"><span className={styles.mono}>{execution.sessionId || 'none'}</span></Descriptions.Item>
          <Descriptions.Item label="Turn"><span className={styles.mono}>{execution.turnId || 'none'}</span></Descriptions.Item>
          <Descriptions.Item label="Tool call"><span className={styles.mono}>{execution.toolCallId || 'none'}</span></Descriptions.Item>
          <Descriptions.Item label="Task"><span className={styles.mono}>{execution.taskId || 'none'}</span></Descriptions.Item>
        </Descriptions>
      </section>

      <section className={styles.drawerSection}>
        <Text strong>Timing</Text>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="Started">{formatTime(execution.startedAt)}</Descriptions.Item>
          <Descriptions.Item label="Completed">{formatTime(execution.completedAt)}</Descriptions.Item>
          <Descriptions.Item label="Duration">{formatDuration(execution.durationMs)}</Descriptions.Item>
        </Descriptions>
      </section>

      <section className={styles.drawerSection}>
        <Text strong>Result</Text>
        {execution.reason ? <div className={styles.summaryBlock}>{execution.reason}</div> : null}
        {execution.error ? <Alert type="error" showIcon message={execution.error} /> : null}
        {execution.redacted ? <Alert type="info" showIcon message="内容已脱敏" /> : null}
        <Summary title="Input summary" value={execution.inputSummary} redacted={execution.redacted} />
        <Summary title="Output summary" value={execution.outputSummary} redacted={execution.redacted} />
        <Summary title="Context summary" value={execution.contextSummary} redacted={execution.redacted} />
      </section>

      <section className={styles.drawerSection}>
        <Text strong>Policy and sandbox</Text>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="Policy mode">{execution.policyMode || 'none'}</Descriptions.Item>
          <Descriptions.Item label="Policy profile">{execution.policyProfile || 'none'}</Descriptions.Item>
          <Descriptions.Item label="Policy rule">{execution.policyRule || 'none'}</Descriptions.Item>
          <Descriptions.Item label="Policy decision">{execution.policyDecision || 'none'}</Descriptions.Item>
          <Descriptions.Item label="Sandbox status">{execution.sandboxStatus || 'none'}</Descriptions.Item>
        </Descriptions>
      </section>

      <section className={styles.drawerSection}>
        <Text strong>Flags</Text>
        <div className={styles.flags}>
          <Tag color={execution.inputRewritten ? 'purple' : 'default'}>input rewritten {String(execution.inputRewritten)}</Tag>
          <Tag color={execution.contextInjected ? 'blue' : 'default'}>context injected {String(execution.contextInjected)}</Tag>
          <Tag color={execution.redacted ? 'gold' : 'default'}>redacted {String(execution.redacted)}</Tag>
        </div>
      </section>
    </>
  );
}

function Summary({ title, value, redacted }: { title: string; value?: string; redacted?: boolean }) {
  if (redacted && !value) {
    return (
      <div className={styles.summaryBlock}>
        <Text strong>{title}</Text>
        <br />
        <Text type="secondary">内容已脱敏</Text>
      </div>
    );
  }
  if (!value) {
    return null;
  }
  return (
    <div className={styles.summaryBlock}>
      <Text strong>{title}</Text>
      <br />
      {value}
    </div>
  );
}

function formatTime(value?: number) {
  if (!value) {
    return 'none';
  }
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(normalizeTimestamp(value)));
}

function formatDuration(value?: number) {
  if (!value) {
    return '0ms';
  }
  if (value < 1000) {
    return `${value}ms`;
  }
  return `${Math.round(value / 100) / 10}s`;
}

function normalizeTimestamp(value: number) {
  return value < 1_000_000_000_000 ? value * 1000 : value;
}
