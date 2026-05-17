# Agentic Operations Client Prototype

This is the Phase 1 UI prototype for the Crush-based agentic operations client.

The prototype is intentionally mock-driven. It does not connect to the Crush
runtime, SSH targets, MCP servers, or real model providers yet.

## Scope

The current screen models an SSH troubleshooting assistant:

- run list and progress;
- active agents and capabilities;
- conversation with event-driven agent reasoning steps;
- SSH command and MCP search evidence;
- run timeline;
- approval prompt preview;
- recommendation/report panel;
- runtime contract preview for future `RunEvent`, `ToolCall`,
  `PermissionRequest`, and `Artifact` events.
- SOP fixture selection;
- SSH target configuration mock.

The `Start replay` button replays typed mock `RunEvent` values. The UI updates
from those events instead of relying on static screen data, which keeps the
prototype aligned with the later SSE runtime contract.

The SOP and SSH target controls are mock configuration surfaces. They model the
future `skill + script + MCP` plugin shape without running real SSH commands.

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

- Add controls for approval decisions in the mock runtime.
- Generate event fixtures from the selected SOP instead of replaying one fixed
  event script.
- Add DeepSeek report generation for the Phase 1 acceptance build.
- Replace mock events with Crush runtime API events in a later phase.
