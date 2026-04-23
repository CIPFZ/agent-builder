# MCP Runtime Capability Task

Date: 2026-04-23

## 1. Task Purpose

This is the single execution document for Claude Code to implement the MCP runtime capability module.

Claude Code should use this file as the primary entrypoint.

This file consolidates:

- scope
- non-goals
- Claude Code source alignment
- current Go ownership boundary
- implementation order
- validation requirements
- delivery format

Claude Code should not invent a new MCP architecture outside this document and the referenced implementation rules.

## 2. Required Reading

Before writing code, read these files in order:

1. `docs/execution/implementation-rules.md`
2. `docs/tasks/mcp-runtime-capability/task.md`
3. `docs/tasks/mcp-runtime-capability/design.md`
4. `docs/tasks/mcp-runtime-capability/source-alignment.md`
5. `docs/tasks/mcp-runtime-capability/implementation-plan.md`
6. `docs/tasks/mcp-runtime-capability/test-validation-plan.md`
7. `docs/tasks/mcp-runtime-capability/review-checklist.md`

After reading, output a short execution summary before coding.

That summary must include:

- module objective
- module non-goals
- Claude semantic alignment points
- target Go files
- planned implementation order
- planned test and validation steps

## 3. Objective

Turn the existing partial MCP implementation in `myclaw` into a first-class runtime capability that can be bootstrapped, dynamically refreshed, authenticated, and exposed through the shared `myclawd` control plane.

## 4. Current Reality

`myclaw` already has meaningful MCP internals:

- MCP connection model in runtime/tool context
- dynamic MCP tool discovery and registration
- MCP prompt/resource discovery
- MCP OAuth storage and authenticator primitives
- reconnect and invalidation support
- MCP auth pseudo-tool generation
- MCP prompt-to-skill projection

But the current implementation is incomplete as a module because:

- normal app/config bootstrap does not expose a real MCP server configuration surface
- MCP runtime is not yet a clearly managed module in `myclawd`
- gateway/control-plane surfaces for MCP inventory, auth, and reconnect are not yet explicit
- the downstream implementation target is not yet documented as one coherent task

## 5. In Scope

- MCP connection configuration model for `myclaw`
- bootstrap wiring from config into runtime
- runtime discovery and initial registration of MCP tools/prompts/resources/skills
- MCP OAuth store and authenticator integration
- reconnect and invalidation semantics
- MCP auth-required pseudo-tool semantics
- MCP resource and prompt support on the shared runtime path
- MCP prompt/skill inventory behavior
- `myclawd` control-plane support for MCP state and runtime management
- focused unit, integration, and functional tests

## 6. Out Of Scope

Do not implement any of the following in this task:

- React MCP settings UI
- advanced visual MCP management UI
- plugin marketplace work
- Claude bridge / remote cloud session work
- generic Docker or database execution work
- redesign of the entire skills system outside MCP touchpoints
- speculative non-MCP protocol redesign

## 7. Architecture Constraints

- MCP must remain a shared runtime capability, not a frontend-owned feature.
- Do not add client-specific MCP logic when a shared runtime/control-plane contract is needed.
- Do not hide MCP access behind ad hoc shell snippets.
- Keep tool/resource/prompt/auth/reconnect semantics on shared runtime paths.
- Do not over-invest in Go TUI behavior.
- Do not expand this task into general skills or plugin architecture work.

## 8. Claude Code Semantic Alignment

Use these Claude Code source areas as semantic references:

- `claude-code/src/Tool.ts`
- `claude-code/src/QueryEngine.ts`
- `claude-code/src/services/mcp/client.ts`
- `claude-code/src/services/mcp/auth.ts`
- `claude-code/src/tools/MCPTool/MCPTool.ts`
- `claude-code/src/tools/McpAuthTool/McpAuthTool.ts`
- `claude-code/src/tools/ListMcpResourcesTool/ListMcpResourcesTool.ts`
- `claude-code/src/tools/ReadMcpResourceTool/ReadMcpResourceTool.ts`
- `claude-code/src/tools/SkillTool/SkillTool.ts`
- `claude-code/src/services/mcp/MCPConnectionManager.tsx`

Carry over these semantics:

- MCP inventory is discovered and normalized centrally
- auth-required servers surface a first-class authenticate action rather than silently disappearing
- reconnect refreshes dynamic tool/prompt/resource inventory
- MCP prompts/resources reuse generic runtime surfaces rather than custom client hacks
- prompt and skill projection remain transport-neutral runtime behaviors

## 9. Go Ownership Boundary

Current primary files:

- `internal/tools/mcp_client.go`
- `internal/tools/mcp_dynamic.go`
- `internal/tools/mcp_oauth.go`
- `internal/tools/extended_tools.go`
- `internal/queryengine/queryengine.go`
- `internal/runtime/runner.go`
- `internal/app/bootstrap.go`
- `internal/config/config.go`
- `internal/gateway/server.go`
- `internal/protocol/ws/message.go`

Likely test ownership:

- `internal/tools/mcp_dynamic_test.go`
- `internal/tools/mcp_oauth_test.go`
- `internal/queryengine/tool_lifecycle_test.go`
- `internal/queryengine/queryengine_test.go`
- `internal/runtime/runner_test.go`
- `internal/gateway/server_test.go`
- new config/bootstrap tests where needed

## 10. Required Module Behaviors

The completed module must provide:

1. explicit MCP server configuration and bootstrap reachability
2. stable runtime discovery of tools, prompts, skills, and resources
3. auth-required behavior that surfaces actionable authenticate paths
4. reconnect behavior that refreshes inventory and runtime state
5. generic resource/prompt/skill behavior without SSH- or UI-style side protocols
6. `myclawd`-visible MCP inventory and management semantics

## 11. Required Implementation Order

Implement in this order:

1. define config/bootstrap ownership for MCP servers
2. normalize runtime discovery/bootstrap path
3. harden auth-required and reconnect behavior
4. expose MCP runtime state through `myclawd`
5. add focused tests
6. run functional validation with representative stdio/http fixtures

Do not jump ahead to broader skills or operator UI work.

## 12. Validation Requirements

At minimum, cover:

- config loading of MCP server definitions
- bootstrap wiring into runtime options
- discovery of tools/prompts/resources/skills
- auth-required pseudo-tool exposure
- OAuth store persistence behavior
- reconnect and list invalidation behavior
- runtime inventory visibility
- gateway/control-plane MCP visibility and actions

Functional validation must include at least:

1. stdio MCP server discovery success case
2. HTTP MCP auth-required case
3. reconnect after tool/resource/prompt invalidation
4. prompt and resource access through shared runtime path

## 13. Delivery Requirements

When implementation is complete, output:

- actual files created or updated
- implementation status mapped to `implementation-plan.md`
- tests executed and their results
- functional validation executed and its results
- remaining risks
- any code/document mismatch found during implementation

## 14. Start Prompt For Claude Code

Use the following prompt to start work:

```text
You are implementing the myclaw MCP runtime capability module.

Before coding, read these files in order:
1. docs/execution/implementation-rules.md
2. docs/tasks/mcp-runtime-capability/task.md
3. docs/tasks/mcp-runtime-capability/design.md
4. docs/tasks/mcp-runtime-capability/source-alignment.md
5. docs/tasks/mcp-runtime-capability/implementation-plan.md
6. docs/tasks/mcp-runtime-capability/test-validation-plan.md
7. docs/tasks/mcp-runtime-capability/review-checklist.md

After reading, output a short execution summary covering:
- objective
- non-goals
- Claude semantic alignment points
- target Go files
- implementation order
- test and validation plan

Then implement directly without asking me to make architecture decisions.

Constraints:
- keep MCP on the shared runtime and control-plane path
- do not solve backend MCP gaps in frontend code
- do not expand scope into general UI work
- auth-required behavior must remain first-class and explicit
- reconnect must refresh dynamic MCP inventory

When finished, report:
- files changed
- completion mapped to implementation-plan.md
- tests run
- validation run
- remaining risks
```

## 15. Review Rule

After Claude Code finishes implementation, return to this planning/review workflow and review the code against:

- `docs/tasks/mcp-runtime-capability/review-checklist.md`

This task is not complete until that review closes.
