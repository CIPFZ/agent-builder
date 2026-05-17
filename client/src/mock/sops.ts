import type { SopFixture, SshTarget } from '../types/runtime'

export const sopFixtures: SopFixture[] = [
  {
    id: 'order-api-pool-timeout',
    name: '订单服务连接池耗尽',
    description: '排查 order-api 请求超时、错误率升高、数据库连接池耗尽等问题。',
    targetService: 'order-api',
    riskLevel: 'medium',
    requiredCapabilities: ['SSH Connector', 'Troubleshooting SOP', 'Knowledge MCP'],
    steps: [
      {
        id: 'service-status',
        title: '检查服务状态',
        command: 'systemctl status order-api',
        expectedSignal: '服务状态、重启次数、最近失败原因',
        risk: 'read',
      },
      {
        id: 'recent-logs',
        title: '查看最近日志',
        command: 'journalctl -u order-api --since "15 min ago"',
        expectedSignal: 'pool timeout、连接池耗尽、数据库访问错误',
        risk: 'read',
      },
      {
        id: 'db-connections',
        title: '检查数据库连接',
        command: 'ss -antp | grep 5432',
        expectedSignal: '到 PostgreSQL 的连接数和状态',
        risk: 'read',
      },
    ],
  },
  {
    id: 'node-disk-pressure',
    name: '节点磁盘水位异常',
    description: '排查节点磁盘使用率过高、日志堆积、临时目录膨胀等问题。',
    targetService: 'node-7',
    riskLevel: 'low',
    requiredCapabilities: ['SSH Connector', 'Troubleshooting SOP'],
    steps: [
      {
        id: 'disk-usage',
        title: '查看磁盘水位',
        command: 'df -h',
        expectedSignal: '根分区、数据分区、日志分区使用率',
        risk: 'read',
      },
      {
        id: 'large-dirs',
        title: '定位大目录',
        command: 'du -xh /var/log | sort -h | tail -20',
        expectedSignal: '日志目录增长点',
        risk: 'read',
      },
      {
        id: 'log-rotate',
        title: '检查日志轮转',
        command: 'systemctl status logrotate.timer',
        expectedSignal: 'logrotate 定时器和最近执行状态',
        risk: 'read',
      },
    ],
  },
  {
    id: 'service-restart-loop',
    name: '服务重启风暴',
    description: '排查 systemd 服务反复重启、健康检查失败和依赖服务异常。',
    targetService: 'payment-gw',
    riskLevel: 'medium',
    requiredCapabilities: ['SSH Connector', 'Troubleshooting SOP', 'Knowledge MCP'],
    steps: [
      {
        id: 'status',
        title: '检查服务状态',
        command: 'systemctl status payment-gw',
        expectedSignal: '重启次数、退出码、最近错误',
        risk: 'read',
      },
      {
        id: 'journal',
        title: '查看服务日志',
        command: 'journalctl -u payment-gw --since "30 min ago"',
        expectedSignal: 'panic、配置错误、依赖不可用',
        risk: 'read',
      },
      {
        id: 'deps',
        title: '检查依赖端口',
        command: 'ss -antp | grep -E "6379|5432|9092"',
        expectedSignal: 'Redis、PostgreSQL、Kafka 连接状态',
        risk: 'read',
      },
    ],
  },
]

export const sshTargets: SshTarget[] = [
  {
    id: 'prod-bj-order',
    name: 'prod-bj-01 / order-api',
    host: '10.24.8.17',
    user: 'ops_readonly',
    port: 22,
    profile: 'readonly-prod',
    environment: 'production',
  },
  {
    id: 'prod-sh-node7',
    name: 'prod-sh-02 / node-7',
    host: '10.42.3.7',
    user: 'ops_readonly',
    port: 22,
    profile: 'readonly-prod',
    environment: 'production',
  },
  {
    id: 'staging-payment',
    name: 'staging-bj-01 / payment-gw',
    host: '10.18.5.29',
    user: 'ops_readonly',
    port: 22,
    profile: 'readonly-staging',
    environment: 'staging',
  },
]
