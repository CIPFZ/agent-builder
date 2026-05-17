import { useMemo, useRef, useState } from 'react'
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
  Layout,
  Progress,
  Row,
  Segmented,
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
  UserOutlined,
} from '@ant-design/icons'
import { Bubble, Sender, ThoughtChain } from '@ant-design/x'
import { historicRuns } from './mock/fixtures'
import { createInitialRuntimeState, reduceRunEvent, replayEvents } from './mock/runtime'
import type {
  CapabilityItem,
  ConversationMessage,
  EvidenceItem,
  RunStatus,
  RuntimeState,
  TimelineEntry,
} from './types/runtime'
import './App.css'

const { Header, Sider, Content } = Layout
const { Text, Title, Paragraph } = Typography

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

function messageAvatar(role: ConversationMessage['role']) {
  if (role === 'user') return <UserOutlined />
  if (role === 'tool') return <ToolOutlined />
  if (role === 'approval') return <SafetyCertificateOutlined />
  return <DeploymentUnitOutlined />
}

function messagePlacement(role: ConversationMessage['role']) {
  return role === 'user' ? 'end' : 'start'
}

function renderMessage(message: ConversationMessage, state: RuntimeState) {
  if (message.role === 'assistant' && message.id === 'msg-agent-1') {
    return (
      <Space orientation="vertical" size={10}>
        <Text>{message.content}</Text>
        <ThoughtChain items={state.thoughts} />
      </Space>
    )
  }

  if (message.role === 'approval') {
    return (
      <Alert
        type="warning"
        showIcon
        title="发现潜在高风险修复动作"
        description={message.content}
      />
    )
  }

  return message.content
}

function App() {
  const [runtimeState, setRuntimeState] = useState(createInitialRuntimeState)
  const [isReplaying, setIsReplaying] = useState(false)
  const abortControllerRef = useRef<AbortController | null>(null)

  const runs = useMemo(() => [runtimeState.run, ...historicRuns], [runtimeState.run])

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
    setRuntimeState(createInitialRuntimeState())
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
    setRuntimeState(createInitialRuntimeState())
    setIsReplaying(false)
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
                {runs.map((run) => (
                  <div key={run.id} className={run.id === runtimeState.run.id ? 'run-item active' : 'run-item'}>
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
                <div className="conversation-scroll">
                  {runtimeState.messages.map((message) => (
                    <Bubble
                      key={message.id}
                      placement={messagePlacement(message.role)}
                      avatar={messageAvatar(message.role)}
                      content={renderMessage(message, runtimeState)}
                    />
                  ))}
                </div>
                <Sender
                  className="sender"
                  placeholder="Describe the issue, ask for another SOP step, or approve a proposed action..."
                  disabled
                  value=""
                />
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
                  <Descriptions.Item label="Target">{runtimeState.run.target}</Descriptions.Item>
                  <Descriptions.Item label="SSH profile">readonly-prod</Descriptions.Item>
                  <Descriptions.Item label="SOP">order-api-pool-timeout.md</Descriptions.Item>
                  <Descriptions.Item label="Policy mode">default</Descriptions.Item>
                </Descriptions>
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
                            {runtimeState.approval.actions.map((action) => (
                              <Button key={action}>{action}</Button>
                            ))}
                          </Space>
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
            </aside>
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  )
}

export default App
