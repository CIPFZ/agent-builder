# Agent Builder Client

This directory contains the React client for Agent Builder. The next frontend
phase is a clean runtime-first rewrite backed by the Go runtime and rendered
with Ant Design plus Ant Design X.

The active technical plan is:

```text
../docs/frontend-runtime-ui-technical-plan.md
```

## Stack

- React
- TypeScript
- Vite
- Ant Design
- Ant Design X
- TanStack Query
- Zustand, for UI state only
- CSS Modules plus Ant Design theme tokens

## Runtime Boundary

The Go runtime is the source of truth for sessions, turns, messages, tool
calls, permissions, agent tasks, worktrees, MCP, skills, refs, audit, recovery,
model config, policy, and budget/context state.

React owns only UI state such as selected panels, drawers, filters, unsaved
form drafts, and composer draft text before submit.

Ant Design X is used for AI interaction primitives such as conversations,
bubbles, sender/composer, prompts, thinking, attachments, markdown, code, and
diagrams. Runtime-specific features such as tool calls, permissions, subagent
tasks, worktrees, diffs, audit, and recovery are mapped into custom Agent
Builder components built on Ant Design and Ant Design X.

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

