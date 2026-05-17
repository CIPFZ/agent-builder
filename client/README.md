# Agentic Operations Client Prototype

This is the Phase 1 UI prototype for the Crush-based agentic operations client.

The prototype does not connect to the Crush runtime, SSH targets, or MCP servers
yet. Chat uses the configured LLM proxy.

## Scope

The current screen is intentionally chat-first:

- Claude Desktop inspired left rail and centered chat surface;
- model picker in the composer;
- model settings drawer for protocol, URL, API key, and advanced proxy;
- real chat requests through the local DeepSeek/OpenAI-compatible proxy;
- operations workspace entry kept as a secondary surface for later SSH/SOP work.

The previous SSH troubleshooting dashboard is no longer the first screen. That
flow will return behind the Operations entry after the basic chat and model
configuration experience is accepted.

## Stack

- React
- TypeScript
- Vite
- Ant Design
- Ant Design X

## Commands

Install dependencies:

```bash
npm install
```

Run the development server:

```bash
npm run dev -- --host 127.0.0.1 --port 5173
```

Run the local report proxy:

```bash
npm run dev:api
```

Local DeepSeek configuration:

```powershell
Copy-Item server\deepseek.config.example.json server\deepseek.local.json
```

Edit `server/deepseek.local.json`:

```json
{
  "protocol": "openai",
  "url": "https://api.deepseek.com",
  "apiKey": "your-key",
  "models": ["deepseek-v4-flash"],
  "proxy": "",
  "port": 4177
}
```

Then start the proxy:

```powershell
npm run dev:api
```

`server/deepseek.local.json` is ignored by Git. `protocol` can be `openai` or
`anthropic`. For DeepSeek, use:

- OpenAI-compatible: `protocol=openai`, `url=https://api.deepseek.com`
- Anthropic-compatible: `protocol=anthropic`,
  `url=https://api.deepseek.com/anthropic`

Environment variables can still override the file when needed:
`DEEPSEEK_CONFIG`, `DEEPSEEK_PROTOCOL`, `DEEPSEEK_API_KEY`,
`DEEPSEEK_API_BASE`, `DEEPSEEK_PROXY`, and `DEEPSEEK_PROXY_PORT`.

`proxy` is reserved for the advanced connection setting. The current Node proxy
records the value and warns if it is set, but does not route `fetch` through it
yet.

If no key is configured, chat requests fail with a clear configuration error.

Build:

```bash
npm run build
```

Lint:

```bash
npm run lint
```

## Next Steps

- Persist connection settings locally.
- Move the proxy into the Go runtime provider layer.
- Add streaming responses.
- Reintroduce SSH/SOP as a secondary Operations workflow after chat acceptance.
