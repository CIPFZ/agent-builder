import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const serverDir = dirname(fileURLToPath(import.meta.url))
const defaultConfigPath = resolve(serverDir, 'deepseek.local.json')

function readLocalConfig() {
  const configPath = process.env.DEEPSEEK_CONFIG || defaultConfigPath
  try {
    return JSON.parse(readFileSync(configPath, 'utf8').replace(/^\uFEFF/, ''))
  } catch (error) {
    if (error?.code !== 'ENOENT') {
      console.warn(`Failed to read DeepSeek config at ${configPath}: ${error.message}`)
    }
    return {}
  }
}

const localConfig = readLocalConfig()
const port = Number(process.env.DEEPSEEK_PROXY_PORT || localConfig.port || 4177)
const protocol = process.env.DEEPSEEK_PROTOCOL || localConfig.protocol || 'openai'
const apiKey = process.env.DEEPSEEK_API_KEY || localConfig.apiKey
const model = process.env.DEEPSEEK_MODEL || localConfig.model || 'deepseek-v4-flash'
const apiUrl = process.env.DEEPSEEK_API_BASE || localConfig.url || localConfig.apiBase || 'https://api.deepseek.com'
const proxyUrl = process.env.DEEPSEEK_PROXY || localConfig.proxy || ''

if (proxyUrl) {
  console.warn('Proxy URL is configured but not active yet. Install a fetch proxy dispatcher before using it.')
}

function resolveConnection(requestConfig = {}) {
  return {
    protocol: requestConfig.protocol || protocol,
    url: requestConfig.url || apiUrl,
    apiKey: requestConfig.apiKey || apiKey,
    proxy: requestConfig.proxy || proxyUrl,
  }
}

function joinUrl(baseUrl, path) {
  return `${baseUrl.replace(/\/$/, '')}${path}`
}

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
  const connection = resolveConnection(requestedConfig)

  if (!connection.apiKey) {
    return fallbackChat(body)
  }

  if (connection.protocol === 'anthropic') {
    return generateChatWithAnthropicProtocol({
      apiKey: connection.apiKey,
      baseUrl: connection.url,
      messages,
      model: requestedModel,
      temperature: requestedTemperature,
    })
  }

  if (connection.protocol !== 'openai') {
    throw new Error(`Unsupported LLM protocol: ${connection.protocol}`)
  }

  const response = await fetch(joinUrl(connection.url, '/chat/completions'), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${connection.apiKey}`,
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

async function generateChatWithAnthropicProtocol({ apiKey: anthropicApiKey, baseUrl, messages, model: requestedModel, temperature }) {
  const response = await fetch(joinUrl(baseUrl, '/v1/messages'), {
    method: 'POST',
    headers: {
      'x-api-key': anthropicApiKey,
      'anthropic-version': '2023-06-01',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: requestedModel,
      max_tokens: 4096,
      messages: messages.map((message) => ({
        role: message.role === 'assistant' ? 'assistant' : 'user',
        content: message.content,
      })),
      system:
        'You are Agent Builder, a concise desktop assistant for agentic operations design, coding, and troubleshooting. Reply in Chinese unless the user asks otherwise.',
      temperature,
    }),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`Anthropic chat request failed: ${response.status} ${text}`)
  }

  const data = await response.json()
  const content = Array.isArray(data?.content)
    ? data.content
        .filter((block) => block?.type === 'text')
        .map((block) => block.text)
        .join('\n')
    : ''

  if (!content) {
    throw new Error('Anthropic chat response did not include text content')
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

  const response = await fetch(joinUrl(apiUrl, '/chat/completions'), {
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
