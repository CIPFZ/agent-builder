# Agentic Operations Client Prototype

This is the Phase 1 UI prototype for the Crush-based agentic operations client.

The prototype is intentionally mock-driven. It does not connect to the Crush
runtime, SSH targets, MCP servers, or real model providers yet.

## Scope

The current screen models an SSH troubleshooting assistant:

- run list and progress;
- active agents and capabilities;
- conversation with agent reasoning steps;
- SSH command and MCP search evidence;
- run timeline;
- approval prompt preview;
- recommendation/report panel;
- runtime contract preview for future `RunEvent`, `ToolCall`,
  `PermissionRequest`, and `Artifact` events.

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

Build:

```bash
npm run build
```

Lint:

```bash
npm run lint
```

## Next Steps

- Extract mock run data into typed fixtures.
- Add a mock event stream to simulate run progression.
- Add responsive polish for narrow desktop windows.
- Replace mock events with Crush runtime API events in a later phase.
