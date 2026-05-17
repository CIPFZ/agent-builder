import type { RunEvent } from '../types/runtime'

export const troubleshootingEvents: RunEvent[] = [
  {
    type: 'run_started',
    run: {
      id: 'RUN-1027',
      title: 'SSH 排障：订单服务错误率升高',
      target: 'prod-bj-01 / order-api',
      status: 'running',
      progress: 8,
    },
    message: {
      id: 'msg-agent-1',
      role: 'assistant',
      content:
        '我会以只读排障模式连接 prod-bj-01/order-api，先收集服务状态和最近日志，再通过 MCP 检索相关 runbook。',
    },
  },
  {
    type: 'agent_updated',
    agent: { name: 'Coordinator', status: 'planning' },
  },
  {
    type: 'thought_updated',
    thought: {
      key: 'context',
      title: '识别故障上下文',
      description: '目标服务为 prod-bj-01/order-api，用户描述为错误率升高和部分请求超时。',
      status: 'success',
    },
  },
  {
    type: 'agent_updated',
    agent: { name: 'SSH Collector', status: 'running' },
  },
  {
    type: 'timeline_added',
    progress: 20,
    entry: {
      id: 'tl-ssh',
      title: '连接 SSH 目标',
      description: '已连接 jumpbox 到 prod-bj-01/order-api，使用只读排障 profile。',
      kind: 'success',
    },
  },
  {
    type: 'evidence_added',
    progress: 32,
    evidence: {
      key: 'ev-service',
      source: 'SSH',
      command: 'systemctl status order-api',
      signal: '服务在线，但最近 10 分钟发生 14 次重启',
      status: 'warning',
    },
  },
  {
    type: 'evidence_added',
    progress: 46,
    evidence: {
      key: 'ev-journal',
      source: 'SSH',
      command: 'journalctl -u order-api --since "15 min ago"',
      signal: '出现数据库连接池耗尽：pool timeout after 30s',
      status: 'critical',
    },
  },
  {
    type: 'evidence_added',
    progress: 56,
    evidence: {
      key: 'ev-conn',
      source: 'SSH',
      command: 'ss -antp | grep 5432',
      signal: '到 PostgreSQL 的 ESTABLISHED 连接数接近配置上限',
      status: 'warning',
    },
  },
  {
    type: 'thought_updated',
    thought: {
      key: 'sop',
      title: '执行 SOP 只读检查',
      description: '已收集服务状态、最近日志、端口连接和资源水位。',
      status: 'success',
    },
  },
  {
    type: 'timeline_added',
    progress: 62,
    entry: {
      id: 'tl-sop',
      title: '执行 SOP 步骤 1-3',
      description: '收集 systemctl、journalctl、ss、df、top 摘要，输出已写入 evidence。',
      kind: 'success',
    },
  },
  {
    type: 'agent_updated',
    agent: { name: 'Runbook Searcher', status: 'running' },
  },
  {
    type: 'timeline_added',
    progress: 70,
    entry: {
      id: 'tl-mcp',
      title: '调用 MCP 知识搜索',
      description: '检索 order-api pool timeout、PostgreSQL 连接池耗尽、重启风暴。',
      kind: 'running',
    },
  },
  {
    type: 'evidence_added',
    progress: 78,
    evidence: {
      key: 'ev-runbook',
      source: 'MCP',
      command: 'search_runbook("order-api pool timeout")',
      signal: '命中 SOP：订单服务连接池耗尽排障步骤',
      status: 'normal',
    },
  },
  {
    type: 'thought_updated',
    thought: {
      key: 'knowledge',
      title: '检索知识库与 MCP 资源',
      description: '匹配到连接池耗尽相关 runbook，当前证据与已知故障模式一致。',
      status: 'success',
    },
  },
  {
    type: 'approval_requested',
    progress: 86,
    approval: {
      id: 'apr-plan-1',
      title: '需要确认：是否生成修复执行计划',
      description: '临时扩容连接池和滚动重启可能影响在线请求。当前只请求生成计划，不会执行修复。',
      actions: ['生成低风险修复计划', '导出排障报告', '保持只读模式'],
    },
    message: {
      id: 'msg-approval-1',
      role: 'approval',
      content: '发现潜在高风险修复动作。当前只展示建议，不会自动执行。',
    },
  },
  {
    type: 'timeline_added',
    progress: 90,
    entry: {
      id: 'tl-approval',
      title: '等待高风险动作确认',
      description: '建议临时扩容连接池和滚动重启，需要用户确认后才会执行。',
      kind: 'warning',
    },
  },
  {
    type: 'report_generated',
    progress: 100,
    recommendation: {
      title: '初步判断：数据库连接池耗尽导致请求超时',
      description: 'order-api 最近有重启记录，日志中出现 pool timeout，数据库连接数接近上限。',
      nextSteps: [
        '确认数据库端最大连接数和应用连接池配置是否匹配。',
        '观察数据库 CPU、连接数和慢查询指标。',
        '如流量仍在上升，在审批后生成临时扩容连接池和滚动重启计划。',
      ],
    },
    message: {
      id: 'msg-report-1',
      role: 'assistant',
      content:
        '初步判断是数据库连接池耗尽导致请求超时。建议先确认数据库最大连接数和应用连接池配置，再决定是否生成修复执行计划。',
    },
  },
  {
    type: 'thought_updated',
    thought: {
      key: 'report',
      title: '生成建议和风险提示',
      description: '已生成排障建议、后续步骤和审批提示。',
      status: 'success',
    },
  },
  {
    type: 'agent_updated',
    agent: { name: 'Coordinator', status: 'completed' },
  },
  {
    type: 'agent_updated',
    agent: { name: 'SSH Collector', status: 'completed' },
  },
  {
    type: 'agent_updated',
    agent: { name: 'Runbook Searcher', status: 'completed' },
  },
]
