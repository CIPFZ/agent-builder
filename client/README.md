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

Optional DeepSeek configuration:

```bash
cp .env.example .env.local
```

Set `DEEPSEEK_API_KEY` in `.env.local`, then load it into your shell before
starting `npm run dev:api`. If no key is configured, the proxy returns a
deterministic fallback chat/report for local acceptance testing.

PowerShell example:

```powershell
$env:DEEPSEEK_API_KEY="your-key"
$env:DEEPSEEK_MODEL="deepseek-v4-flash"
$env:DEEPSEEK_API_BASE="https://api.deepseek.com"
npm run dev:api
```

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
