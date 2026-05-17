import { lazy, Suspense, useMemo, useRef, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  ConfigProvider,
  Descriptions,
  Divider,
  Flex,
  Form,
  Input,
  Layout,
  Progress,
  Row,
  Segmented,
  Select,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  Timeline,
  Typography,
  theme,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  AlertOutlined,
  ApiOutlined,
  AuditOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  CloudServerOutlined,
  CodeOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  FileSearchOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import { historicRuns } from './mock/fixtures'
import { generateTroubleshootingReport } from './api/report'
import {
  createApprovalResolvedEvent,
  createInitialRuntimeState,
  reduceRunEvent,
  replayEvents,
} from './mock/runtime'
import { sopFixtures, sshTargets } from './mock/sops'
import type {
  ApprovalDecision,
  CapabilityItem,
  EvidenceItem,
  RunEvent,
  RunStatus,
  SopFixture,
  SshTarget,
  TimelineEntry,
} from './types/runtime'
import './App.css'

const { Header, Sider, Content } = Layout
const { Text, Title, Paragraph } = Typography
const AgentConversation = lazy(() =>
  import('./components/AgentConversation').then((module) => ({ default: module.AgentConversation })),
)

const evidenceColumns: ColumnsType<EvidenceItem> = [
  {
    title: '来源',
    dataIndex: 'source',
    width: 88,
    render: (source: EvidenceItem['source']) => (
      <Tag icon={source === 'SSH' ? <CodeOutlined /> : <SearchOutlined />}>{source}</Tag>
    ),
  },
  {
    title: '命令 / 查询',
    dataIndex: 'command',
    ellipsis: true,
    render: (command: string) => <Text code>{command}</Text>,
  },
  {
    title: '信号',
    dataIndex: 'signal',
  },
  {
    title: '状态',
    dataIndex: 'status',
    width: 92,
    render: (status: EvidenceItem['status']) => {
      const color = status === 'critical' ? 'red' : status === 'warning' ? 'gold' : 'green'
      const label = status === 'critical' ? '严重' : status === 'warning' ? '注意' : '正常'
      return <Tag color={color}>{label}</Tag>
    },
  },
]

function statusBadge(status: RunStatus) {
  if (status === 'completed') return <Badge status="success" text="Completed" />
  if (status === 'waiting_approval') return <Badge status="warning" text="Waiting" />
  if (status === 'idle') return <Badge status="default" text="Idle" />
  return <Badge status="processing" text="Running" />
}

function capabilityIcon(type: CapabilityItem['type']) {
  if (type === 'ssh') return <CloudServerOutlined />
  if (type === 'skill') return <FileSearchOutlined />
  if (type === 'mcp') return <ApiOutlined />
  return <AuditOutlined />
}

function timelineIcon(kind: TimelineEntry['kind']) {
  if (kind === 'success') return <CheckCircleOutlined />
  if (kind === 'warning') return <AlertOutlined />
  if (kind === 'error') return <AlertOutlined />
  return <SearchOutlined />
}

function timelineColor(kind: TimelineEntry['kind']) {
  if (kind === 'success') return 'green'
  if (kind === 'warning') return 'gold'
  if (kind === 'error') return 'red'
  return 'blue'
}

function eventSummary(event: RunEvent) {
  switch (event.type) {
    case 'run_started':
      return event.run.title
    case 'agent_updated':
      return `${event.agent.name}: ${event.agent.status}`
    case 'message_added':
      return event.message.content
    case 'thought_updated':
      return event.thought.title
    case 'timeline_added':
      return event.entry.title
    case 'evidence_added':
      return event.evidence.signal
    case 'approval_requested':
      return event.approval.title
    case 'report_generated':
      return event.recommendation.title
    case 'approval_resolved':
      return `${event.decision}: ${event.entry.title}`
  }
}

function App() {
  const [runtimeState, setRuntimeState] = useState(createInitialRuntimeState)
  const [selectedSopId, setSelectedSopId] = useState(sopFixtures[0].id)
  const [selectedTargetId, setSelectedTargetId] = useState(sshTargets[0].id)
  const [isReplaying, setIsReplaying] = useState(false)
  const [isGeneratingReport, setIsGeneratingReport] = useState(false)
  const [reportError, setReportError] = useState<string | null>(null)
  const [reportProvider, setReportProvider] = useState<string | null>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  const selectedSop = useMemo<SopFixture>(
    () => sopFixtures.find((sop) => sop.id === selectedSopId) ?? sopFixtures[0],
    [selectedSopId],
  )
  const selectedTarget = useMemo<SshTarget>(
    () => sshTargets.find((target) => target.id === selectedTargetId) ?? sshTargets[0],
    [selectedTargetId],
  )

  const configuredRun = useMemo(
    () => ({
      ...runtimeState.run,
      title: `SSH 排障：${selectedSop.name}`,
      target: selectedTarget.name,
    }),
    [runtimeState.run, selectedSop.name, selectedTarget.name],
  )

  const timelineItems = useMemo(
    () =>
      runtimeState.timeline.map((entry) => ({
        color: timelineColor(entry.kind),
        icon: timelineIcon(entry.kind),
        content: (
          <>
            <Text strong>{entry.title}</Text>
            <Paragraph className="timeline-copy">{entry.description}</Paragraph>
          </>
        ),
      })),
    [runtimeState.timeline],
  )

  const startReplay = () => {
    abortControllerRef.current?.abort()
    const controller = new AbortController()
    abortControllerRef.current = controller
    setRuntimeState((current) => ({
      ...createInitialRuntimeState(),
      run: {
        ...current.run,
        title: `SSH 排障：${selectedSop.name}`,
        target: selectedTarget.name,
        status: 'idle',
        progress: 0,
      },
      capabilities: current.capabilities.map((capability) => ({
        ...capability,
        meta:
          capability.type === 'skill'
            ? `${selectedSop.id}.md`
            : capability.type === 'ssh'
              ? `${selectedTarget.profile} / ${selectedTarget.host}`
              : capability.meta,
      })),
    }))
    setIsReplaying(true)
    replayEvents(
      (event) => {
        setRuntimeState((current) => reduceRunEvent(current, event))
      },
      { signal: controller.signal },
    )
    window.setTimeout(() => setIsReplaying(false), 14_000)
  }

  const resetReplay = () => {
    abortControllerRef.current?.abort()
    setRuntimeState({
      ...createInitialRuntimeState(),
      run: {
        ...createInitialRuntimeState().run,
        title: `SSH 排障：${selectedSop.name}`,
        target: selectedTarget.name,
      },
    })
    setIsReplaying(false)
  }

  const resolveApproval = (decision: ApprovalDecision) => {
    setRuntimeState((current) => reduceRunEvent(current, createApprovalResolvedEvent(decision)))
  }

  const generateReport = async () => {
    setIsGeneratingReport(true)
    setReportError(null)
    try {
      const report = await generateTroubleshootingReport({
        userGoal: '订单服务错误率升高，按 SOP 排查是否为数据库连接池问题。',
        sop: selectedSop,
        target: selectedTarget,
        evidence: runtimeState.evidence,
      })
      setReportProvider(report.provider)
      setRuntimeState((current) => ({
        ...current,
        recommendation: {
          title: report.title,
          description: report.summary || report.description,
          nextSteps: report.nextSteps,
        },
      }))
    } catch (error) {
      setReportError(error instanceof Error ? error.message : String(error))
    } finally {
      setIsGeneratingReport(false)
    }
  }

  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#2563eb',
          borderRadius: 6,
          fontFamily:
            'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
        },
      }}
    >
      <Layout className="app-shell">
        <Header className="topbar">
          <Flex align="center" justify="space-between" className="topbar-inner">
            <Space size={12}>
              <div className="brand-mark">
                <DeploymentUnitOutlined />
              </div>
              <div>
                <Title level={4} className="brand-title">
                  Agentic Operations
                </Title>
                <Text type="secondary">SSH troubleshooting event-stream prototype</Text>
              </div>
            </Space>
            <Space size={12} wrap>
              <Segmented options={['Plan', 'Default', 'Accept edits']} value="Default" />
              <Tag icon={<SafetyCertificateOutlined />} color="blue">
                mock event stream
              </Tag>
              <Button icon={<ReloadOutlined />} onClick={resetReplay}>
                Reset
              </Button>
              <Button type="primary" icon={<PlayCircleOutlined />} loading={isReplaying} onClick={startReplay}>
                Start replay
              </Button>
              <Button icon={<SettingOutlined />}>Settings</Button>
            </Space>
          </Flex>
        </Header>

        <Layout className="main-layout">
          <Sider width={284} className="left-rail">
            <section className="rail-section">
              <Flex justify="space-between" align="center">
                <Text strong>Runs</Text>
                <Button size="small" type="primary" icon={<PlayCircleOutlined />} onClick={startReplay}>
                  New
                </Button>
              </Flex>
              <div className="run-list">
                {[configuredRun, ...historicRuns].map((run) => (
                  <div key={run.id} className={run.id === configuredRun.id ? 'run-item active' : 'run-item'}>
                    <Space orientation="vertical" size={6} className="full-width">
                      <Flex justify="space-between" align="center">
                        <Text strong>{run.id}</Text>
                        {statusBadge(run.status)}
                      </Flex>
                      <Text>{run.title}</Text>
                      <Text type="secondary">{run.target}</Text>
                      <Progress percent={run.progress} size="small" showInfo={false} />
                    </Space>
                  </div>
                ))}
              </div>
            </section>

            <section className="rail-section">
              <Text strong>Troubleshooting SOP</Text>
              <Space orientation="vertical" size={10} className="selector-block">
                <Select
                  value={selectedSopId}
                  options={sopFixtures.map((sop) => ({ value: sop.id, label: sop.name }))}
                  onChange={setSelectedSopId}
                />
                <Text type="secondary">{selectedSop.description}</Text>
                <Tag color={selectedSop.riskLevel === 'high' ? 'red' : selectedSop.riskLevel === 'medium' ? 'gold' : 'green'}>
                  {selectedSop.riskLevel} risk
                </Tag>
              </Space>
            </section>

            <section className="rail-section">
              <Text strong>SSH Target</Text>
              <Space orientation="vertical" size={10} className="selector-block">
                <Select
                  value={selectedTargetId}
                  options={sshTargets.map((target) => ({ value: target.id, label: target.name }))}
                  onChange={setSelectedTargetId}
                />
                <Text type="secondary">
                  {selectedTarget.user}@{selectedTarget.host}:{selectedTarget.port}
                </Text>
                <Tag>{selectedTarget.profile}</Tag>
              </Space>
            </section>

            <section className="rail-section">
              <Text strong>Agents</Text>
              <div className="compact-list">
                {runtimeState.agents.map((agent) => (
                  <div key={agent.name} className="compact-list-item">
                    <Space>
                      <span className="list-icon">
                        {agent.name === 'Coordinator' ? <BranchesOutlined /> : agent.name === 'SSH Collector' ? <ThunderboltOutlined /> : <SearchOutlined />}
                      </span>
                      <span>{agent.name}</span>
                      <Tag>{agent.status}</Tag>
                    </Space>
                  </div>
                ))}
              </div>
            </section>

            <section className="rail-section">
              <Text strong>Capabilities</Text>
              <div className="compact-list">
                {runtimeState.capabilities.map((capability) => (
                  <div key={capability.name} className="compact-list-item">
                    <Space align="start">
                      <span className="list-icon">{capabilityIcon(capability.type)}</span>
                      <Space orientation="vertical" size={0}>
                        <Text>{capability.name}</Text>
                        <Text type="secondary">{capability.meta}</Text>
                      </Space>
                    </Space>
                  </div>
                ))}
              </div>
            </section>
          </Sider>

          <Content className="content-grid">
            <section className="conversation-panel">
              <Card className="panel-card" title="Conversation" extra={<Tag color="processing">{runtimeState.run.id}</Tag>}>
                <Suspense fallback={<Alert type="info" showIcon title="Loading conversation" />}>
                  <AgentConversation state={runtimeState} />
                </Suspense>
              </Card>

              <Card className="panel-card" title="Run Timeline">
                {timelineItems.length > 0 ? (
                  <Timeline items={timelineItems} />
                ) : (
                  <Alert
                    type="info"
                    showIcon
                    title="等待运行"
                    description="点击 Start replay，模拟从 runtime SSE 接收 RunEvent 并逐步更新 UI。"
                  />
                )}
              </Card>
            </section>

            <aside className="detail-panel">
              <Card className="panel-card" title="Run Summary">
                <Row gutter={[12, 12]}>
                  <Col span={8}>
                    <Statistic title="Progress" value={runtimeState.run.progress} suffix="%" />
                  </Col>
                  <Col span={8}>
                    <Statistic title="Signals" value={runtimeState.evidence.length} />
                  </Col>
                  <Col span={8}>
                    <Statistic title="Risk" value={runtimeState.approval ? 'Medium' : 'Low'} />
                  </Col>
                </Row>
                <Divider />
                <Descriptions column={1} size="small">
                  <Descriptions.Item label="Target">{selectedTarget.name}</Descriptions.Item>
                  <Descriptions.Item label="SSH profile">{selectedTarget.profile}</Descriptions.Item>
                  <Descriptions.Item label="SOP">{selectedSop.id}.md</Descriptions.Item>
                  <Descriptions.Item label="Policy mode">default</Descriptions.Item>
                </Descriptions>
              </Card>

              <Card className="panel-card" title="SOP and SSH Configuration">
                <Form layout="vertical" size="small">
                  <Form.Item label="SOP">
                    <Input value={selectedSop.name} readOnly />
                  </Form.Item>
                  <Form.Item label="Target host">
                    <Input value={`${selectedTarget.user}@${selectedTarget.host}:${selectedTarget.port}`} readOnly />
                  </Form.Item>
                  <Form.Item label="SOP steps">
                    <div className="sop-steps">
                      {selectedSop.steps.map((step, index) => (
                        <div className="sop-step" key={step.id}>
                          <Flex justify="space-between" align="center">
                            <Text strong>
                              {index + 1}. {step.title}
                            </Text>
                            <Tag>{step.risk}</Tag>
                          </Flex>
                          <Text code>{step.command}</Text>
                          <Text type="secondary">{step.expectedSignal}</Text>
                        </div>
                      ))}
                    </div>
                  </Form.Item>
                </Form>
              </Card>

              <Card className="panel-card" title="Evidence">
                <Table<EvidenceItem>
                  columns={evidenceColumns}
                  dataSource={runtimeState.evidence}
                  locale={{ emptyText: 'Start replay to collect SSH and MCP evidence.' }}
                  pagination={false}
                  size="small"
                  rowKey="key"
                />
              </Card>

              <Card className="panel-card" title="Recommendation">
                <Tabs
                  items={[
                    {
                      key: 'summary',
                      label: 'Summary',
                      children: runtimeState.recommendation ? (
                        <Space orientation="vertical" size={12}>
                          <Alert
                            type="error"
                            showIcon
                            title={runtimeState.recommendation.title}
                            description={runtimeState.recommendation.description}
                          />
                          <ul className="next-steps">
                            {runtimeState.recommendation.nextSteps.map((step) => (
                              <li key={step}>{step}</li>
                            ))}
                          </ul>
                          {reportProvider ? <Tag color="blue">generated by {reportProvider}</Tag> : null}
                        </Space>
                      ) : (
                        <Alert
                          type="info"
                          showIcon
                          title="尚未生成建议"
                          description="等待 mock runtime 生成报告事件。"
                        />
                      ),
                    },
                    {
                      key: 'deepseek',
                      label: 'DeepSeek',
                      children: (
                        <Space orientation="vertical" size={12} className="full-width">
                          <Alert
                            type="info"
                            showIcon
                            title="验收报告生成"
                            description="调用本地 DeepSeek proxy 生成报告。未配置 DEEPSEEK_API_KEY 时会返回 deterministic fallback。"
                          />
                          {reportError ? <Alert type="error" showIcon title="报告生成失败" description={reportError} /> : null}
                          <Button
                            type="primary"
                            loading={isGeneratingReport}
                            disabled={runtimeState.evidence.length === 0}
                            onClick={generateReport}
                          >
                            Generate report
                          </Button>
                          {runtimeState.evidence.length === 0 ? (
                            <Text type="secondary">请先点击 Start replay 收集 mock evidence。</Text>
                          ) : null}
                        </Space>
                      ),
                    },
                    {
                      key: 'approval',
                      label: 'Approval',
                      children: runtimeState.approval ? (
                        <Space orientation="vertical" size={12} className="full-width">
                          <Alert
                            type="warning"
                            showIcon
                            title={runtimeState.approval.title}
                            description={runtimeState.approval.description}
                          />
                          <Space wrap>
                            <Button type="primary" onClick={() => resolveApproval('approved')}>
                              Approve plan
                            </Button>
                            <Button onClick={() => resolveApproval('denied')}>Deny</Button>
                            <Button>Export report</Button>
                          </Space>
                          <div className="approval-actions">
                            {runtimeState.approval.actions.map((action) => (
                              <Tag key={action}>{action}</Tag>
                            ))}
                          </div>
                        </Space>
                      ) : (
                        <Alert
                          type="success"
                          showIcon
                          title="暂无待审批动作"
                          description="当前只运行只读 SSH/SOP/MCP 步骤。"
                        />
                      ),
                    },
                  ]}
                />
              </Card>

              <Card className="panel-card" title="Runtime Contract Preview">
                <Space orientation="vertical" size={8}>
                  <Tag icon={<DatabaseOutlined />}>RunEvent</Tag>
                  <Tag icon={<ToolOutlined />}>ToolCall</Tag>
                  <Tag icon={<SafetyCertificateOutlined />}>PermissionRequest</Tag>
                  <Tag icon={<AuditOutlined />}>Artifact</Tag>
                  <Text type="secondary">
                    This prototype replays typed mock events. Later phases can replace the mock replay with HTTP JSON API + SSE events from Crush.
                  </Text>
                </Space>
              </Card>

              <Card className="panel-card" title="Raw Event Log">
                <div className="event-log">
                  {runtimeState.eventLog.length > 0 ? (
                    runtimeState.eventLog.map((record) => (
                      <div className="event-log-row" key={record.id}>
                        <Flex justify="space-between" align="center" gap={8}>
                          <Tag color="blue">{record.event.type}</Tag>
                          <Text type="secondary">{new Date(record.timestamp).toLocaleTimeString()}</Text>
                        </Flex>
                        <Text className="event-log-summary">{eventSummary(record.event)}</Text>
                      </div>
                    ))
                  ) : (
                    <Text type="secondary">No events received yet.</Text>
                  )}
                </div>
              </Card>
            </aside>
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}

export default App
