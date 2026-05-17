import { createServer } from 'node:http'

const port = Number(process.env.DEEPSEEK_PROXY_PORT || 4177)
const apiKey = process.env.DEEPSEEK_API_KEY
const model = process.env.DEEPSEEK_MODEL || 'deepseek-chat'
const apiBase = process.env.DEEPSEEK_API_BASE || 'https://api.deepseek.com'

function json(res, statusCode, payload) {
  res.writeHead(statusCode, {
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Headers': 'content-type',
    'Access-Control-Allow-Methods': 'POST, OPTIONS',
    'Content-Type': 'application/json; charset=utf-8',
  })
  res.end(JSON.stringify(payload))
}

function fallbackReport(body) {
  const evidence = Array.isArray(body.evidence) ? body.evidence : []
  const critical = evidence.filter((item) => item.status === 'critical')
  const warnings = evidence.filter((item) => item.status === 'warning')

  return {
    provider: 'fallback',
    title: '初步判断：数据库连接池耗尽导致请求超时',
    summary:
      '当前为本地 fallback 报告。证据显示服务存在重启记录，日志中出现连接池超时，数据库连接数接近上限。',
    findings: [
      `严重信号 ${critical.length} 个，注意信号 ${warnings.length} 个。`,
      '建议优先确认应用连接池配置、数据库最大连接数和近期流量变化。',
      '高风险修复动作仍需人工确认，不应由模型自动执行。',
    ],
    nextSteps: [
      '导出当前排障报告并附带 SSH/MCP 证据。',
      '检查数据库端连接数、慢查询和 CPU 使用率。',
      '如业务流量仍在上升，审批后生成连接池扩容和滚动重启计划。',
    ],
  }
}

function fallbackChat(body) {
  const messages = Array.isArray(body.messages) ? body.messages : []
  const lastUserMessage = [...messages].reverse().find((message) => message.role === 'user')
  const content = lastUserMessage?.content || 'hello'

  return {
    provider: 'fallback',
    content: [
      `我已经收到你的消息：“${content}”。`,
      '',
      '当前本地 proxy 没有检测到 DEEPSEEK_API_KEY，所以返回 fallback 对话结果。',
      '你可以先用它验证桌面聊天体验、模型选择和输入框交互；配置 key 后会切换为真实 DeepSeek 响应。',
    ].join('\n'),
  }
}

async function readJson(req) {
  const chunks = []
  for await (const chunk of req) {
    chunks.push(chunk)
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}')
}

async function generateChatWithDeepSeek(body) {
  const requestedConfig = body?.config || {}
  const requestedModel = requestedConfig.model || model
  const requestedTemperature =
    typeof requestedConfig.temperature === 'number' ? requestedConfig.temperature : 0.2
  const messages = Array.isArray(body.messages) ? body.messages : []

  const response = await fetch(`${apiBase}/chat/completions`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: requestedModel,
      messages: [
        {
          role: 'system',
          content:
            'You are Agent Builder, a concise desktop assistant for agentic operations design, coding, and troubleshooting. Reply in Chinese unless the user asks otherwise.',
        },
        ...messages.map((message) => ({
          role: message.role,
          content: message.content,
        })),
      ],
      temperature: requestedTemperature,
    }),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`DeepSeek chat request failed: ${response.status} ${text}`)
  }

  const data = await response.json()
  const content = data?.choices?.[0]?.message?.content
  if (!content) {
    throw new Error('DeepSeek chat response did not include message content')
  }

  return {
    provider: 'deepseek',
    content,
  }
}

async function generateWithDeepSeek(body) {
  const prompt = [
    '你是企业运维排障助手。请基于用户问题、SOP、SSH/MCP 证据生成排障报告。',
    '要求输出 JSON，字段为 title、summary、findings、nextSteps。',
    '不要建议自动执行高风险修复命令；高风险动作必须请求人工确认。',
    '',
    JSON.stringify(body, null, 2),
  ].join('\n')

  const response = await fetch(`${apiBase}/chat/completions`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model,
      messages: [
        {
          role: 'system',
          content: 'You generate concise, structured troubleshooting reports in Chinese.',
        },
        {
          role: 'user',
          content: prompt,
        },
      ],
      response_format: { type: 'json_object' },
      temperature: 0.2,
    }),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`DeepSeek request failed: ${response.status} ${text}`)
  }

  const data = await response.json()
  const content = data?.choices?.[0]?.message?.content
  if (!content) {
    throw new Error('DeepSeek response did not include message content')
  }

  return {
    provider: 'deepseek',
    ...JSON.parse(content),
  }
}

const server = createServer(async (req, res) => {
  if (req.method === 'OPTIONS') {
    json(res, 204, {})
    return
  }

  if (req.method !== 'POST') {
    json(res, 404, { error: 'not_found' })
    return
  }

  try {
    const body = await readJson(req)

    if (req.url === '/api/chat') {
      const chat = apiKey ? await generateChatWithDeepSeek(body) : fallbackChat(body)
      json(res, 200, chat)
      return
    }

    if (req.url === '/api/deepseek/report') {
      const report = apiKey ? await generateWithDeepSeek(body) : fallbackReport(body)
      json(res, 200, report)
      return
    }

    json(res, 404, { error: 'not_found' })
  } catch (error) {
    json(res, 500, {
      error: 'proxy_request_failed',
      message: error instanceof Error ? error.message : String(error),
    })
  }
})

server.listen(port, '127.0.0.1', () => {
  console.log(`DeepSeek report proxy listening on http://127.0.0.1:${port}`)
  if (!apiKey) {
    console.log('DEEPSEEK_API_KEY is not set. Using fallback reports.')
  }
})
