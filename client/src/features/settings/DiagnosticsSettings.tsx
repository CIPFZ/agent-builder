import { useCallback, useEffect, useMemo, useState } from 'react';
import { CopyOutlined, ExperimentOutlined, ReloadOutlined, RightOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Descriptions, Drawer, Empty, Flex, Modal, Select, Skeleton, Space, Statistic, Tag, Timeline, Typography, message } from 'antd';
import type {
  DiagnosticIncidentViewModel,
  DiagnosticIncidentsResponseViewModel,
  DiagnosticSupportInformationViewModel,
  SessionViewModel,
  TargetedDiagnosticViewModel,
} from '../../runtime/workbenchTypes.ts';
import styles from './DiagnosticsSettings.module.css';

const { Paragraph, Text, Title } = Typography;

const kindOptions = [
  { value: '', label: '全部类型' },
  { value: 'provider_failure', label: '模型请求失败' },
  { value: 'turn_interrupted', label: '对话中断' },
  { value: 'tool_failure', label: '工具执行失败' },
  { value: 'persistence_failure', label: '数据持久化失败' },
];

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'unresolved', label: '未解决' },
  { value: 'recovered', label: '已恢复' },
];

const kindLabels: Record<string, string> = Object.fromEntries(kindOptions.map((item) => [item.value, item.label]));
const checkLabels: Record<string, string> = {
  provider_connection: '测试 Provider 连接',
  path_access: '验证引用路径',
  sqlite_quick_check: '检查数据库状态',
};

interface DiagnosticsSettingsProps {
  sessions: SessionViewModel[];
  onLoad: (request: { sessionId?: string; kind?: string; status?: string; limit?: number }) => Promise<DiagnosticIncidentsResponseViewModel>;
  onRunCheck: (incidentID: string, checkID: string) => Promise<TargetedDiagnosticViewModel>;
  onSupportInformation: (incidentID: string) => Promise<DiagnosticSupportInformationViewModel>;
  onAction: (incident: DiagnosticIncidentViewModel, actionKind: string) => Promise<void>;
  onOpenProviderSettings: () => void;
}

export function DiagnosticsSettings({ sessions, onLoad, onRunCheck, onSupportInformation, onAction, onOpenProviderSettings }: DiagnosticsSettingsProps) {
  const [sessionId, setSessionId] = useState('');
  const [kind, setKind] = useState('');
  const [status, setStatus] = useState('');
  const [data, setData] = useState<DiagnosticIncidentsResponseViewModel>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<DiagnosticIncidentViewModel>();
  const [runningCheck, setRunningCheck] = useState(false);
  const [checkResult, setCheckResult] = useState<TargetedDiagnosticViewModel>();
  const [support, setSupport] = useState<DiagnosticSupportInformationViewModel>();
  const [actionID, setActionID] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await onLoad({ sessionId: sessionId || undefined, kind: kind || undefined, status: status || undefined, limit: 100 });
      setData(response);
      setSelected((current) => current ? response.incidents.find((item) => item.id === current.id) ?? current : current);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : String(loadError));
    } finally {
      setLoading(false);
    }
  }, [kind, onLoad, sessionId, status]);

  useEffect(() => {
    const timeout = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timeout);
  }, [load]);

  const latest = data?.incidents[0];
  const sessionOptions = useMemo(() => [{ value: '', label: '全部会话' }, ...sessions.map((item) => ({ value: item.id, label: item.title || item.id }))], [sessions]);

  const runCheck = async () => {
    if (!selected?.recommendedCheckId) return;
    setRunningCheck(true);
    try {
      setCheckResult(await onRunCheck(selected.id, selected.recommendedCheckId));
    } catch (checkError) {
      message.error(checkError instanceof Error ? checkError.message : String(checkError));
    } finally {
      setRunningCheck(false);
    }
  };

  const prepareSupport = async (incidentID = selected?.id) => {
    if (!incidentID) return;
    try { setSupport(await onSupportInformation(incidentID)); }
    catch (supportError) { message.error(supportError instanceof Error ? supportError.message : String(supportError)); }
  };

  const copySupport = async () => {
    if (!support) return;
    await navigator.clipboard.writeText(support.text);
    message.success('支持信息已复制');
    setSupport(undefined);
  };

  const executeAction = async (incident: DiagnosticIncidentViewModel, actionKind: string, actionId: string) => {
    if (actionKind === 'open_provider_settings') { onOpenProviderSettings(); return; }
    if (actionKind === 'copy_support_information') { setSelected(incident); await prepareSupport(incident.id); return; }
    setActionID(actionId);
    try { await onAction(incident, actionKind); await load(); }
    catch (actionError) { message.error(actionError instanceof Error ? actionError.message : String(actionError)); }
    finally { setActionID(''); }
  };

  return (
    <section className={styles.page} aria-label="诊断故障事件中心">
      <Flex justify="space-between" align="flex-start" gap={16} wrap>
        <div><Title level={3}>诊断</Title><Text type="secondary">了解故障原因和解决方案；需要再次执行的任务请返回关联会话。</Text></div>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void load()}>刷新</Button>
      </Flex>

      <div className={styles.summaryGrid}>
        <Card><Statistic title="最近 7 天故障" value={data?.recentCount ?? 0} /></Card>
        <Card><Statistic title="可安全修复" value={data?.recoverableCount ?? 0} /></Card>
        <Card><Statistic title="最近一次故障" value={latest ? formatTime(latest.lastObservedAt) : '暂无'} valueStyle={{ fontSize: 16 }} /></Card>
      </div>

      <Flex className={styles.filters} gap={8} wrap>
        <Select aria-label="按会话筛选" value={sessionId} options={sessionOptions} onChange={setSessionId} className={styles.filter} showSearch optionFilterProp="label" />
        <Select aria-label="按故障类型筛选" value={kind} options={kindOptions} onChange={setKind} className={styles.filter} />
        <Select aria-label="按状态筛选" value={status} options={statusOptions} onChange={setStatus} className={styles.filter} />
      </Flex>

      {error ? <Alert type="error" showIcon message="无法加载诊断事件" description={error} action={<Button size="small" onClick={() => void load()}>重试</Button>} /> : null}
      {loading && !data ? <Card><Skeleton active /></Card> : null}
      {!loading && !error && (data?.incidents.length ?? 0) === 0 ? <Card><Empty description="当前筛选范围内没有需要处理的故障" /></Card> : null}

      <div className={styles.incidentList}>
        {data?.incidents.map((incident) => {
          const primary = incident.actions[0];
          return (
            <Card key={incident.id} className={styles.incidentCard}>
              <Flex justify="space-between" align="flex-start" gap={16}>
                <div className={styles.incidentBody}>
                  <Space wrap><Tag color={incident.resolved ? 'success' : 'error'}>{incident.resolved ? '已恢复' : '未解决'}</Tag><Tag>{kindLabels[incident.kind] ?? incident.kind}</Tag><Text type="secondary">{formatTime(incident.lastObservedAt)}</Text></Space>
                  <Title level={5}>{incident.title}</Title>
                  <Paragraph ellipsis={{ rows: 2 }}>{incident.cause}</Paragraph>
                  <Text type="secondary">{[incident.sessionId && `Session ${shortID(incident.sessionId)}`, incident.turnId && `Turn ${shortID(incident.turnId)}`].filter(Boolean).join(' · ')}</Text>
                </div>
                <Space direction="vertical" align="end">
                  {primary ? <Button type="primary" loading={actionID === primary.id} onClick={() => void executeAction(incident, primary.kind, primary.id)}>{primary.label}</Button> : null}
                  <Button type="text" icon={<RightOutlined />} iconPosition="end" onClick={() => { setSelected(incident); setCheckResult(undefined); }}>查看详情</Button>
                </Space>
              </Flex>
            </Card>
          );
        })}
      </div>

      <Drawer title={selected?.title} width={560} open={Boolean(selected)} onClose={() => setSelected(undefined)}>
        {selected ? <Space direction="vertical" size="large" className={styles.drawerContent}>
          <Alert type={selected.resolved ? 'success' : 'error'} showIcon message={selected.summary} />
          <div className={styles.guidanceGrid}>
            <section className={styles.guidanceItem}><Text type="secondary">已确认原因</Text><Paragraph>{selected.cause}</Paragraph></section>
            <section className={styles.guidanceItem}><Text type="secondary">建议解决方案</Text><Paragraph>{selected.resolution}</Paragraph></section>
          </div>
          <Descriptions size="small" column={1} bordered items={[
            { key: 'code', label: '错误码', children: selected.errorCode || '未结构化识别' },
            { key: 'provider', label: 'Provider / 模型', children: [selected.provider, selected.model].filter(Boolean).join(' / ') || '不适用' },
            { key: 'turn', label: '关联', children: [selected.sessionId && `Session ${selected.sessionId}`, selected.turnId && `Turn ${selected.turnId}`, selected.toolCallId && `ToolCall ${selected.toolCallId}`].filter(Boolean).join(' · ') },
            { key: 'last', label: '最后活动', children: formatTime(selected.lastObservedAt) },
          ]} />
          <div><Title level={5}>故障链</Title><Timeline items={selected.evidence.slice(0, 5).map((item) => ({ children: <div><Text strong>{item.label}</Text><br /><Text>{item.summary || item.kind}</Text><br /><Text type="secondary">{formatTime(item.timestamp)}</Text></div> }))} /></div>
          {checkResult ? <Alert type={checkResult.status === 'pass' ? 'success' : 'warning'} showIcon message={checkResult.summary} description={checkResult.detail} /> : null}
          <Flex gap={8} wrap>
            {selected.actions.slice(0, 2).map((action) => <Button key={action.id} loading={actionID === action.id} danger={action.destructive} onClick={() => void executeAction(selected, action.kind, action.id)}>{action.label}</Button>)}
            {selected.recommendedCheckId ? <Button icon={<ExperimentOutlined />} loading={runningCheck} onClick={() => void runCheck()}>{checkLabels[selected.recommendedCheckId] ?? '运行定向检查'}</Button> : null}
            <Button icon={<CopyOutlined />} onClick={() => void prepareSupport()}>复制支持信息</Button>
          </Flex>
        </Space> : null}
      </Drawer>

      <Modal title="支持信息预览" open={Boolean(support)} okText="复制" cancelText="取消" onOk={() => void copySupport()} onCancel={() => setSupport(undefined)}>
        <Paragraph type="secondary">仅包含 Runtime allowlist 字段，不包含 API Key、消息正文、完整工具参数、环境变量或完整输出。</Paragraph>
        <pre className={styles.supportPreview}>{support?.text}</pre>
      </Modal>
    </section>
  );
}

function formatTime(value?: string) { if (!value) return '未知'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString(); }
function shortID(value: string) { return value.length > 12 ? `${value.slice(0, 8)}…` : value; }
