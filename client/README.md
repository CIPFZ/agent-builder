# Agentic Operations Client Prototype

This is the Phase 1 UI prototype for the Crush-based agentic operations client.

The prototype is intentionally mock-driven. It does not connect to the Crush
runtime, SSH targets, MCP servers, or real model providers yet.

## Scope

The current screen is intentionally chat-first:

- Claude Desktop inspired left rail and centered chat surface;
- model picker in the composer;
- model settings drawer for provider, model, local proxy API base, and
  temperature;
- real chat requests through the local DeepSeek/OpenAI-compatible proxy;
- fallback reply path when the proxy or API key is unavailable;
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
  "apiBase": "https://api.deepseek.com",
  "apiKey": "your-key",
  "model": "deepseek-v4-flash",
  "port": 4177
}
```

Then start the proxy:

```powershell
npm run dev:api
```

`server/deepseek.local.json` is ignored by Git. Environment variables can still
override the file when needed: `DEEPSEEK_CONFIG`, `DEEPSEEK_API_KEY`,
`DEEPSEEK_MODEL`, `DEEPSEEK_API_BASE`, and `DEEPSEEK_PROXY_PORT`.

If no key is configured, the proxy returns a deterministic fallback chat/report
for local acceptance testing.

Build:

```bash
npm run build
```

Lint:

```bash
npm run lint
```

## Next Steps

- Persist model settings locally.
- Move the proxy into the Go runtime provider layer.
- Add streaming responses.
- Reintroduce SSH/SOP as a secondary Operations workflow after chat acceptance.
