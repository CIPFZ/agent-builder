import type { RuntimeState } from '../types/runtime'

export const initialRuntimeState: RuntimeState = {
  run: {
    id: 'RUN-1027',
    title: 'SSH 排障：订单服务错误率升高',
    target: 'prod-bj-01 / order-api',
    status: 'idle',
    progress: 0,
  },
  agents: [
    { name: 'Coordinator', status: 'idle' },
    { name: 'SSH Collector', status: 'idle' },
    { name: 'Runbook Searcher', status: 'idle' },
  ],
  capabilities: [
    { type: 'ssh', name: 'SSH Connector', meta: '只读 profile / prod-bj-01' },
    { type: 'skill', name: 'Troubleshooting SOP', meta: 'order-api-pool-timeout.md' },
    { type: 'mcp', name: 'Knowledge MCP', meta: 'runbook + incident archive' },
    { type: 'audit', name: 'Audit Stream', meta: 'run events enabled' },
  ],
  messages: [
    {
      id: 'msg-user-1',
      role: 'user',
      content: '订单服务错误率升高，帮我按 SOP 看一下是不是数据库连接池问题。',
    },
  ],
  thoughts: [
    {
      key: 'context',
      title: '识别故障上下文',
      description: '等待运行开始。',
      status: 'loading',
    },
    {
      key: 'sop',
      title: '执行 SOP 只读检查',
      description: '等待 SSH 连接。',
      status: 'loading',
    },
    {
      key: 'knowledge',
      title: '检索知识库与 MCP 资源',
      description: '等待证据收集完成。',
      status: 'loading',
    },
    {
      key: 'report',
      title: '生成建议和风险提示',
      description: '等待分析结论。',
      status: 'loading',
    },
  ],
  timeline: [],
  evidence: [],
}

export const historicRuns = [
  {
    id: 'RUN-1026',
    title: '节点磁盘水位巡检',
    target: 'prod-sh-02 / node-7',
    status: 'completed' as const,
    progress: 100,
  },
  {
    id: 'RUN-1025',
    title: '支付网关连接超时排查',
    target: 'prod-bj-01 / payment-gw',
    status: 'waiting_approval' as const,
    progress: 44,
  },
]
