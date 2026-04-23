# MCP Runtime Capability Review Checklist

Date: 2026-04-23

## 1. Review Goal

Use this checklist when reviewing the downstream Claude Code implementation of the MCP module.

The review focus is:

- missing lifecycle behavior
- semantic drift from Claude Code MCP runtime patterns
- control-plane gaps
- test blind spots

## 2. Scope Integrity

- Confirm the task stayed within MCP runtime/control-plane scope.
- Confirm no major UI work was mixed into the module.
- Confirm no Docker/DB functionality was opportunistically folded into this task.

## 3. Config And Bootstrap

- Confirm MCP server definitions can be configured through normal `myclaw` config.
- Confirm bootstrap passes configured MCP connections into runtime options.
- Confirm config validation rejects invalid server definitions early.
- Confirm stdio and HTTP-style connection definitions are both covered if documented as supported.

## 4. Discovery And Dynamic Inventory

- Confirm discovery remains centralized.
- Confirm tools, prompts, resources, and derived skills are all captured.
- Confirm runtime snapshots and inventory reflect configured/discovered servers consistently.
- Confirm reconnect can replace stale inventory rather than append duplicate state.

## 5. Auth And OAuth

- Confirm auth-required servers surface an explicit authenticate action.
- Confirm challenge/scope/resource-metadata fields survive where needed.
- Confirm successful auth flow leads to reconnect and real tool replacement.
- Confirm auth-required behavior does not rely on frontend-only state.

## 6. Prompt / Resource / Skill Semantics

- Confirm MCP resources are still accessed through generic runtime tools.
- Confirm MCP prompts remain on generic runtime surfaces rather than custom protocol hacks.
- Confirm MCP prompt-to-skill projection remains explicit and intentional.
- Confirm MCP-derived skills do not leak into unrelated paths unexpectedly.

## 7. myclawd Control Plane

- Confirm MCP state is visible through `myclawd`.
- Confirm explicit MCP management actions exist where the plan requires them.
- Confirm gateway behavior reuses shared runtime/control-plane ownership.
- Confirm no client-specific MCP lifecycle logic was introduced.

## 8. Claude Alignment

- Confirm behavior remains semantically aligned to:
  - centralized discovery
  - first-class auth-required actions
  - reconnect-driven inventory refresh
  - generic prompt/resource/skill exposure

## 9. Test Coverage

- Confirm config/bootstrap tests exist.
- Confirm discovery and reconnect tests exist.
- Confirm OAuth/auth-required tests exist.
- Confirm gateway/control-plane MCP tests exist.
- Confirm at least one representative stdio case and one auth-required HTTP case were run or simulated.

## 10. Merge Blockers

Treat any of the following as blocking:

- MCP cannot be configured through normal startup
- auth-required servers disappear instead of surfacing explicit authenticate behavior
- reconnect does not refresh inventory correctly
- `myclawd` has no usable MCP state/control surface after the module claims completion
- tests only cover helper functions while lifecycle wiring remains unverified
