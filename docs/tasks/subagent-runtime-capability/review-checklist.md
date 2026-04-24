# Subagent Runtime Capability Review Checklist

Date: 2026-04-24

## 1. Review Goal

Use this checklist when reviewing the downstream Claude Code implementation of the subagent module.

The review focus is:

- missing lifecycle behavior
- semantic drift from Claude Code delegated-task patterns
- control-plane gaps
- test blind spots

## 2. Scope Integrity

- Confirm the task stayed within delegated-task runtime and control-plane scope.
- Confirm no major React or TUI work was mixed into the module.
- Confirm no Docker or database functionality was opportunistically folded into this task.

## 3. Lifecycle Ledger

- Confirm delegated tasks have stable IDs and preserved lineage.
- Confirm lifecycle status is separated from control action semantics.
- Confirm terminal result and error state are preserved in a reviewable way.
- Confirm list and status paths do not have to infer lifecycle from ambiguous payload fields.

## 4. Runtime Control And Resume

- Confirm control input reaches real runtime-owned delegated runs.
- Confirm stop acts on running delegated tasks predictably.
- Confirm resume reuses the original child session when valid.
- Confirm invalid resume states fail safely.
- Confirm worktree and permission semantics survive resume.

## 5. Tool Contract

- Confirm `agent.task` exposes a stable delegated-task result contract.
- Confirm background-launch payloads are explicit and not ad hoc.
- Confirm fast-finish inline behavior is intentional and documented if preserved.
- Confirm transcript behavior remains transport-neutral.

## 6. Fork And Isolation Semantics

- Confirm fork behavior is explicit and intentionally routed.
- Confirm nested fork misuse is still blocked where required.
- Confirm worktree-isolated delegated runs preserve workspace context and cleanup behavior.

## 7. myclawd Control Plane

- Confirm `spawn_subagent` reaches the runtime option surface required by the plan.
- Confirm list and status payloads are rich enough for future UI work.
- Confirm stop, steer, and resume reuse shared runtime ownership.
- Confirm wait or close semantics exist if the implementation plan requires them.
- Confirm orchestration hooks remain aligned after payload normalization.

## 8. Claude Alignment

- Confirm behavior remains semantically aligned to:
  - stable task identity
  - background observability
  - continue or resume by task identity
  - explicit fork behavior
  - transport-neutral notifications and results

## 9. Test Coverage

- Confirm lifecycle tests exist.
- Confirm real control-input behavior is tested against runtime-owned delegated runs.
- Confirm resume and worktree reuse tests exist.
- Confirm gateway and orchestration tests exist.
- Confirm at least one background flow and one resume flow were run or simulated.

## 10. Merge Blockers

Treat any of the following as blocking:

- delegated-task state is still too thin for reliable review or UI consumption
- `subagent_steer` still does not affect real runtime-owned delegated runs
- lifecycle status and control actions are still conflated in the stable payload shape
- resume no longer preserves child-session continuity
- `myclawd` still lacks a usable delegated-task control surface after the module claims completion
- tests only cover helper functions while runtime lifecycle wiring remains unverified
