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
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  ToolOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { Bubble, Sender, ThoughtChain } from '@ant-design/x'
import './App.css'

const { Header, Sider, Content } = Layout
const { Text, Title, Paragraph } = Typography

type RunStatus = 'running' | 'waiting' | 'completed'

type RunItem = {
  id: string
  title: string
  target: string
  status: RunStatus
  progress: number
}

type EvidenceItem = {
  key: string
  source: string
  command: string
  signal: string
  status: 'normal' | 'warning' | 'critical'
}

const runs: RunItem[] = [
  {
    id: 'RUN-1027',
    title: 'SSH 排障：订单服务错误率升高',
    target: 'prod-bj-01 / order-api',
    status: 'running',
    progress: 68,
  },
  {
    id: 'RUN-1026',
    title: '节点磁盘水位巡检',
    target: 'prod-sh-02 / node-7',
    status: 'completed',
    progress: 100,
  },
  {
    id: 'RUN-1025',
    title: '支付网关连接超时排查',
    target: 'prod-bj-01 / payment-gw',
    status: 'waiting',
    progress: 44,
  },
]

const evidence: EvidenceItem[] = [
  {
    key: '1',
    source: 'SSH',
    command: 'systemctl status order-api',
    signal: '服务在线，但最近 10 分钟发生 14 次重启',
    status: 'warning',
  },
  {
    key: '2',
    source: 'SSH',
    command: 'journalctl -u order-api --since "15 min ago"',
    signal: '出现数据库连接池耗尽：pool timeout after 30s',
    status: 'critical',
  },
  {
    key: '3',
    source: 'SSH',
    command: 'ss -antp | grep 5432',
    signal: '到 PostgreSQL 的 ESTABLISHED 连接数接近配置上限',
    status: 'warning',
  },
  {
    key: '4',
    source: 'MCP',
    command: 'search_runbook("order-api pool timeout")',
    signal: '命中 SOP：订单服务连接池耗尽排障步骤',
    status: 'normal',
  },
]

const evidenceColumns: ColumnsType<EvidenceItem> = [
  {
    title: '来源',
    dataIndex: 'source',
    width: 88,
    render: (source: string) => <Tag icon={source === 'SSH' ? <CodeOutlined /> : <SearchOutlined />}>{source}</Tag>,
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

const thoughtItems = [
  {
    title: '识别故障上下文',
    description: '目标服务为 prod-bj-01/order-api，用户描述为错误率升高和部分请求超时。',
    status: 'success' as const,
  },
  {
    title: '执行 SOP 只读检查',
    description: '通过 SSH 收集服务状态、最近日志、端口连接和资源水位。',
    status: 'success' as const,
  },
  {
    title: '检索知识库与 MCP 资源',
    description: '匹配到连接池耗尽相关 runbook，正在比对当前证据。',
    status: 'loading' as const,
  },
  {
    title: '生成建议和风险提示',
    description: '准备输出低风险观察项和需要审批的修复动作。',
    status: 'loading' as const,
  },
]

const timelineItems = [
  {
    color: 'green',
    icon: <CheckCircleOutlined />,
    content: (
      <>
        <Text strong>连接 SSH 目标</Text>
        <Paragraph className="timeline-copy">已连接 jumpbox 到 prod-bj-01/order-api，使用只读排障 profile。</Paragraph>
      </>
    ),
  },
  {
    color: 'green',
    icon: <CheckCircleOutlined />,
    content: (
      <>
        <Text strong>执行 SOP 步骤 1-3</Text>
        <Paragraph className="timeline-copy">收集 systemctl、journalctl、ss、df、top 摘要，输出已写入 evidence。</Paragraph>
      </>
    ),
  },
  {
    color: 'blue',
    icon: <SearchOutlined />,
    content: (
      <>
        <Text strong>调用 MCP 知识搜索</Text>
        <Paragraph className="timeline-copy">检索 order-api pool timeout、PostgreSQL 连接池耗尽、重启风暴。</Paragraph>
      </>
    ),
  },
  {
    color: 'gold',
    icon: <AlertOutlined />,
    content: (
      <>
        <Text strong>等待高风险动作确认</Text>
        <Paragraph className="timeline-copy">建议临时扩容连接池和滚动重启，需要用户确认后才会执行。</Paragraph>
      </>
    ),
  },
]

const pluginItems = [
  { icon: <CloudServerOutlined />, name: 'SSH Connector', meta: '只读 profile / prod-bj-01' },
  { icon: <FileSearchOutlined />, name: 'Troubleshooting SOP', meta: 'order-api-pool-timeout.md' },
  { icon: <ApiOutlined />, name: 'Knowledge MCP', meta: 'runbook + incident archive' },
  { icon: <AuditOutlined />, name: 'Audit Stream', meta: 'run events enabled' },
]

function statusBadge(status: RunStatus) {
  if (status === 'completed') return <Badge status="success" text="Completed" />
  if (status === 'waiting') return <Badge status="warning" text="Waiting" />
  return <Badge status="processing" text="Running" />
}

function App() {
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
                <Text type="secondary">SSH troubleshooting prototype</Text>
              </div>
            </Space>
            <Space size={12}>
              <Segmented options={['Plan', 'Default', 'Accept edits']} value="Default" />
              <Tag icon={<SafetyCertificateOutlined />} color="blue">
                mock runtime
              </Tag>
              <Button icon={<SettingOutlined />}>Settings</Button>
            </Space>
          </Flex>
        </Header>

        <Layout className="main-layout">
          <Sider width={284} className="left-rail">
            <section className="rail-section">
              <Flex justify="space-between" align="center">
                <Text strong>Runs</Text>
                <Button size="small" type="primary" icon={<PlayCircleOutlined />}>
                  New
                </Button>
              </Flex>
              <div className="run-list">
                {runs.map((run) => (
                  <div key={run.id} className={run.id === 'RUN-1027' ? 'run-item active' : 'run-item'}>
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
                {[
                  { name: 'Coordinator', status: 'planning', icon: <BranchesOutlined /> },
                  { name: 'SSH Collector', status: 'running', icon: <ThunderboltOutlined /> },
                  { name: 'Runbook Searcher', status: 'running', icon: <SearchOutlined /> },
                ].map((item) => (
                  <div key={item.name} className="compact-list-item">
                    <Space>
                      <span className="list-icon">{item.icon}</span>
                      <span>{item.name}</span>
                      <Tag>{item.status}</Tag>
                    </Space>
                  </div>
                ))}
              </div>
            </section>

            <section className="rail-section">
              <Text strong>Capabilities</Text>
              <div className="compact-list">
                {pluginItems.map((item) => (
                  <div key={item.name} className="compact-list-item">
                    <Space align="start">
                      <span className="list-icon">{item.icon}</span>
                      <Space orientation="vertical" size={0}>
                        <Text>{item.name}</Text>
                        <Text type="secondary">{item.meta}</Text>
                      </Space>
                    </Space>
                  </div>
                ))}
              </div>
            </section>
          </Sider>

          <Content className="content-grid">
            <section className="conversation-panel">
              <Card className="panel-card" title="Conversation" extra={<Tag color="processing">RUN-1027</Tag>}>
                <div className="conversation-scroll">
                  <Bubble
                    placement="end"
                    avatar={<UserOutlined />}
                    content="订单服务错误率升高，帮我按 SOP 看一下是不是数据库连接池问题。"
                  />
                  <Bubble
                    avatar={<DeploymentUnitOutlined />}
                    content={
                      <Space orientation="vertical" size={10}>
                        <Text>我会以只读排障模式连接 prod-bj-01/order-api，先收集服务状态和最近日志，再通过 MCP 检索相关 runbook。</Text>
                        <ThoughtChain items={thoughtItems} />
                      </Space>
                    }
                  />
                  <Bubble
                    avatar={<ToolOutlined />}
                    content={
                      <Alert
                        type="warning"
                        showIcon
                        title="发现潜在高风险修复动作"
                        description="临时扩容连接池和滚动重启可能影响在线请求。当前只展示建议，不会自动执行。"
                      />
                    }
                  />
                </div>
                <Sender
                  className="sender"
                  placeholder="Describe the issue, ask for another SOP step, or approve a proposed action..."
                  disabled
                  value=""
                />
              </Card>

              <Card className="panel-card" title="Run Timeline">
                <Timeline items={timelineItems} />
              </Card>
            </section>

            <aside className="detail-panel">
              <Card className="panel-card" title="Run Summary">
                <Row gutter={[12, 12]}>
                  <Col span={8}>
                    <Statistic title="Progress" value={68} suffix="%" />
                  </Col>
                  <Col span={8}>
                    <Statistic title="Signals" value={4} />
                  </Col>
                  <Col span={8}>
                    <Statistic title="Risk" value="Medium" />
                  </Col>
                </Row>
                <Divider />
                <Descriptions column={1} size="small">
                  <Descriptions.Item label="Target">prod-bj-01 / order-api</Descriptions.Item>
                  <Descriptions.Item label="SSH profile">readonly-prod</Descriptions.Item>
                  <Descriptions.Item label="SOP">order-api-pool-timeout.md</Descriptions.Item>
                  <Descriptions.Item label="Policy mode">default</Descriptions.Item>
                </Descriptions>
              </Card>

              <Card className="panel-card" title="Evidence">
                <Table<EvidenceItem>
                  columns={evidenceColumns}
                  dataSource={evidence}
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
                      children: (
                        <Space orientation="vertical" size={12}>
                          <Alert
                            type="error"
                            showIcon
                            title="初步判断：数据库连接池耗尽导致请求超时"
                            description="order-api 最近有重启记录，日志中出现 pool timeout，数据库连接数接近上限。"
                          />
                          <Paragraph>
                            建议先确认数据库端最大连接数和应用连接池配置是否匹配。若业务流量仍在上升，可在审批后临时扩大连接池并滚动重启服务。
                          </Paragraph>
                        </Space>
                      ),
                    },
                    {
                      key: 'approval',
                      label: 'Approval',
                      children: (
                        <Space orientation="vertical" size={12} className="full-width">
                          <Alert
                            type="warning"
                            showIcon
                            title="需要确认：是否生成修复执行计划"
                            description="此原型不会执行真实修复，只展示后续权限确认和审计体验。"
                          />
                          <Space>
                            <Button type="primary">Approve plan</Button>
                            <Button>Deny</Button>
                            <Button>Export report</Button>
                          </Space>
                        </Space>
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
                    This prototype is driven by mock events. The UI is shaped to later consume HTTP JSON API + SSE events from Crush.
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
