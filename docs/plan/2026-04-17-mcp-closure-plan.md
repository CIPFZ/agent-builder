# MCP Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining Go-vs-Claude MCP runtime gaps so MCP reaches functional parity, including refresh behavior and MCP-backed skill discovery.

**Architecture:** Execute MCP parity behind a strict review gate. Before each module, compare the current Go implementation with the matching `claude-code` source, lock the exact runtime contract, write failing tests for the missing behavior, then implement the full module end-to-end before moving on. Treat `claude-code` TypeScript as the only source of truth for semantics.

**Tech Stack:** Go, `go test`, Claude Code TypeScript sources under `claude-code/src/services/mcp`, `claude-code/src/tools`, and `claude-code/src/skills`.

---

## Review Gate Rule

- [ ] **Step 1: Review current Go runtime against Claude source before every module**

Required evidence per module:
- Claude source files reviewed
- Go files that currently implement the area
- exact missing runtime semantics
- tests that will prove parity
- module completion criteria

- [ ] **Step 2: Follow TDD strictly**

For every module:
- write the targeted failing tests first
- run them and confirm failure
- implement the minimum production change set
- rerun focused tests
- rerun broader MCP regression tests

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: all relevant MCP/runtime/queryengine tests pass after each completed module

## Module 1: MCP `list_changed` Refresh Parity

**Claude sources:**
- `claude-code/src/services/mcp/useManageMCPConnections.ts`
- `claude-code/src/services/mcp/client.ts`

**Go sources:**
- `internal/tools/mcp_client.go`
- `internal/tools/extended_tools.go`
- `internal/queryengine/queryengine.go`
- `internal/queryengine/tool_lifecycle_test.go`

**Target contract:**
- when an MCP server advertises `tools.listChanged`, `prompts.listChanged`, or `resources.listChanged`, runtime refreshes the affected surfaces instead of staying stale
- prompt refresh updates MCP prompt-backed commands/skills
- resource refresh updates resource listings and any resource-derived MCP skills
- tool refresh swaps stale tool definitions with the newly discovered set

- [ ] **Step 1: Write failing tests for refresh-on-notification behavior**

Add tests that prove:
- tools are rediscovered after `tools/list_changed`
- prompts/commands are rediscovered after `prompts/list_changed`
- resources are rediscovered after `resources/list_changed`
- refresh updates queryengine-visible tool and command surfaces

Run: `go test ./internal/tools/... ./internal/queryengine -run "MCP|Lifecycle|ListChanged"`
Expected: FAIL because list-changed notifications are not yet wired

- [ ] **Step 2: Implement notification-driven rediscovery**

Implement the runtime behavior needed to:
- register notification handlers on connected transports
- invalidate cached discovery data for the affected server
- refresh tools, prompts, resources, and derived commands/skills
- replace stale definitions without disturbing unrelated servers

- [ ] **Step 3: Verify Module 1**

Run: `go test ./internal/tools/... ./internal/queryengine -run "MCP|Lifecycle|ListChanged"`
Expected: PASS

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: PASS

## Module 2: MCP `roots/list` Handler Parity

**Claude sources:**
- `claude-code/src/services/mcp/client.ts`

**Go sources:**
- `internal/tools/mcp_client.go`
- `internal/runtime/runner.go`
- `internal/queryengine/queryengine.go`

**Target contract:**
- connected MCP clients can request `roots/list`
- runtime responds with the current workspace root in `file://` form
- the handler is available as part of the MCP client lifecycle, not a one-off manual stub

- [ ] **Step 1: Write failing tests for `roots/list` handling**

Add tests that prove:
- MCP servers can request `roots/list` after connection
- the returned root matches the active workspace/current working directory
- the handler remains available across reconnects

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine -run "Roots|MCP"`
Expected: FAIL because no `roots/list` handler is currently registered

- [ ] **Step 2: Implement `roots/list` support**

Implement:
- request handler registration during connection setup
- correct workspace-root resolution
- reconnect-safe re-registration

- [ ] **Step 3: Verify Module 2**

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine -run "Roots|MCP"`
Expected: PASS

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: PASS

## Module 3: `skill://` Resource-Backed MCP Skill Parity

**Claude sources:**
- `claude-code/src/services/mcp/client.ts`
- `claude-code/src/services/mcp/useManageMCPConnections.ts`
- `claude-code/src/skills/mcpSkillBuilders.ts`
- `claude-code/src/skills/loadSkillsDir.ts`

**Go sources:**
- `internal/tools/mcp_client.go`
- `internal/tools/skill_discovery.go`
- `internal/tools/extended_tools.go`
- `internal/queryengine/queryengine.go`
- `internal/tools/skill_discovery_test.go`

**Target contract:**
- MCP resources that represent skills are converted into skill commands, not just listed as resources
- those skills refresh when resources change
- MCP prompt-backed skills and resource-backed skills coexist without name collisions breaking discovery

- [ ] **Step 1: Write failing tests for resource-backed MCP skill discovery**

Add tests that prove:
- `skill://` or equivalent Claude MCP skill resources are surfaced as skill commands
- discovered MCP skills preserve source metadata needed by the skill runtime
- `resources/list_changed` refresh replaces stale MCP skills

Run: `go test ./internal/tools/... ./internal/queryengine -run "Skill|MCP|Resource"`
Expected: FAIL because resource-backed MCP skill discovery is incomplete

- [ ] **Step 2: Implement MCP skill resource discovery**

Implement:
- resource classification for MCP skill resources
- conversion into `SkillCommand` values aligned with Claude metadata
- refresh wiring through queryengine/app-state updates

- [ ] **Step 3: Verify Module 3**

Run: `go test ./internal/tools/... ./internal/queryengine -run "Skill|MCP|Resource"`
Expected: PASS

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: PASS

## Final Verification

- [ ] **Step 1: Run the full MCP regression suite**

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: PASS

- [ ] **Step 2: Summarize remaining MCP risk**

Confirm whether any Claude MCP features remain intentionally out of scope or blocked by missing host capabilities. If none remain, mark MCP parity closed and switch to the next skill-parity module.
