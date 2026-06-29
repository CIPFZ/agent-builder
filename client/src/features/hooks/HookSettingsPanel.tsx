import { BranchesOutlined, CheckCircleOutlined, StopOutlined, WarningOutlined } from '@ant-design/icons';
import { Alert, Card, Descriptions, Empty, Flex, Table, Tag, Tooltip, Typography } from 'antd';
import { useMemo, useState } from 'react';
import type { ColumnsType } from 'antd/es/table';
import type { HookExecutionSummaryViewModel, HookViewModel } from '../../runtime/workbenchTypes.ts';
import { executionStatusColor } from './hookExecutionUtils.ts';
import styles from './HookSettingsPanel.module.css';

const { Text, Title } = Typography;

export function HookSettingsPanel({
  hooks,
  hookExecutions,
}: {
  hooks?: HookViewModel[];
  hookExecutions?: HookExecutionSummaryViewModel;
}) {
  const configuredHooks = hooks ?? [];
  const [selectedHookID, setSelectedHookID] = useState(configuredHooks[0]?.id ?? '');
  const selectedHook = configuredHooks.find((hook) => hook.id === selectedHookID) ?? configuredHooks[0];
  const recentExecutions = useMemo(() => {
    if (!selectedHook) {
      return [];
    }
    return (hookExecutions?.items ?? []).filter((execution) => execution.hookId === selectedHook.id).slice(0, 5);
  }, [hookExecutions?.items, selectedHook]);
  const events = [...new Set(configuredHooks.map((hook) => hook.event).filter(Boolean))];
  const activeCount = configuredHooks.filter((hook) => hook.status === 'active' || hook.enabled).length;
  const invalidCount = configuredHooks.filter((hook) => hook.status === 'invalid' || !hook.enabled).length;

  const columns: ColumnsType<HookViewModel> = [
    {
      title: 'Event',
      dataIndex: 'event',
      key: 'event',
      width: 150,
      render: (event: string) => <Tag color="blue">{event}</Tag>,
    },
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, hook) => <Text strong>{name || hook.event}</Text>,
    },
    {
      title: 'Source',
      dataIndex: 'source',
      key: 'source',
      width: 110,
      render: (source: string) => <Tag>{source || 'unknown'}</Tag>,
    },
    {
      title: 'Matcher',
      dataIndex: 'matcher',
      key: 'matcher',
      render: (matcher?: string) => <Text type="secondary">{matcher || 'all inputs/tools'}</Text>,
    },
    {
      title: 'Command',
      dataIndex: 'commandPreview',
      key: 'commandPreview',
      render: (command: string) => (
        <Tooltip title={command}>
          <code className={styles.command}>{command || 'not provided'}</code>
        </Tooltip>
      ),
    },
    {
      title: 'Timeout',
      dataIndex: 'timeoutMs',
      key: 'timeoutMs',
      width: 100,
      render: (timeoutMs?: number) => <Text type="secondary">{formatTimeout(timeoutMs)}</Text>,
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (_status: string, hook) => <HookStatusTag hook={hook} />,
    },
  ];

  return (
    <>
      <div className={styles.header}>
        <Title level={2}>Hooks</Title>
        <Text type="secondary">Read-only runtime hook configuration and recent execution diagnostics.</Text>
        <div className={styles.summary}>
          <Tag icon={<BranchesOutlined />}>{configuredHooks.length} configured</Tag>
          <Tag color="green" icon={<CheckCircleOutlined />}>{activeCount} active</Tag>
          <Tag color={invalidCount ? 'red' : 'default'} icon={<WarningOutlined />}>{invalidCount} invalid</Tag>
          <Tag>{events.length ? events.join(', ') : 'no events'}</Tag>
        </div>
      </div>

      {!hooks ? (
        <Alert type="warning" showIcon message="Hook API unavailable" description="The runtime did not return hook configuration data." />
      ) : configuredHooks.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No hooks configured" />
      ) : (
        <div className={styles.layout}>
          <Card className={styles.tableWrap} styles={{ body: { padding: 0 } }}>
            <Table
              columns={columns}
              dataSource={configuredHooks}
              pagination={false}
              rowKey="id"
              size="small"
              rowClassName={(hook) => (hook.id === selectedHook?.id ? 'ant-table-row-selected' : '')}
              onRow={(hook) => ({
                onClick: () => setSelectedHookID(hook.id),
              })}
            />
          </Card>

          <Card className={styles.details} styles={{ body: { padding: 18 } }}>
            {selectedHook ? (
              <>
                <Flex align="center" justify="space-between" gap={12}>
                  <Text strong>{selectedHook.name || selectedHook.event}</Text>
                  <HookStatusTag hook={selectedHook} />
                </Flex>
                <Descriptions column={1} size="small" className={styles.recent}>
                  <Descriptions.Item label="Event">{selectedHook.event}</Descriptions.Item>
                  <Descriptions.Item label="Source">{selectedHook.source || 'unknown'}</Descriptions.Item>
                  <Descriptions.Item label="Matcher">{selectedHook.matcher || 'all inputs/tools'}</Descriptions.Item>
                  <Descriptions.Item label="Timeout">{formatTimeout(selectedHook.timeoutMs)}</Descriptions.Item>
                  <Descriptions.Item label="Command">
                    <code className={styles.command}>{selectedHook.commandPreview || 'not provided'}</code>
                  </Descriptions.Item>
                </Descriptions>
                {selectedHook.diagnostics || selectedHook.reason ? (
                  <Alert
                    className={styles.diagnostics}
                    type={selectedHook.enabled ? 'info' : 'warning'}
                    showIcon
                    message={selectedHook.diagnostics || selectedHook.reason}
                  />
                ) : null}
                <div className={styles.recent}>
                  <Text strong>Recent executions</Text>
                  {!hookExecutions ? (
                    <Text type="secondary">Current session execution API is unavailable.</Text>
                  ) : recentExecutions.length === 0 ? (
                    <Text type="secondary">No execution for the current session.</Text>
                  ) : (
                    recentExecutions.map((execution) => (
                      <div key={execution.id} className={styles.recentRow}>
                        <Tag color={executionStatusColor(execution.status)}>{execution.status}</Tag>
                        {execution.durationMs ? <Tag>{formatDuration(execution.durationMs)}</Tag> : null}
                        {execution.reason ? <Text type="secondary">{execution.reason}</Text> : null}
                      </div>
                    ))
                  )}
                </div>
              </>
            ) : (
              <Empty className={styles.detailsEmpty} image={Empty.PRESENTED_IMAGE_SIMPLE} description="Select a hook" />
            )}
          </Card>
        </div>
      )}
    </>
  );
}

function HookStatusTag({ hook }: { hook: HookViewModel }) {
  if (!hook.enabled || hook.status === 'invalid') {
    return <Tag color="red" icon={<StopOutlined />}>{hook.status || 'disabled'}</Tag>;
  }
  return <Tag color="green" icon={<CheckCircleOutlined />}>{hook.status || 'active'}</Tag>;
}

function formatTimeout(timeoutMs?: number) {
  if (!timeoutMs) {
    return 'default';
  }
  return timeoutMs >= 1000 ? `${Math.round(timeoutMs / 1000)}s` : `${timeoutMs}ms`;
}

function formatDuration(durationMs?: number) {
  if (!durationMs) {
    return '0ms';
  }
  if (durationMs < 1000) {
    return `${durationMs}ms`;
  }
  return `${Math.round(durationMs / 100) / 10}s`;
}
