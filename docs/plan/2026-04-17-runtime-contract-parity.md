# Runtime Contract Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the Go runtime to Claude Code runtime-contract parity module by module, starting with bundled skill runtime behavior rather than prompt-only similarity.

**Architecture:** Each module is executed behind a review gate. Before any code change, compare the current Go implementation against the corresponding `claude-code` source, lock the target runtime contract, identify the exact gaps, then execute a TDD red-green-refactor cycle until the whole module is complete. Modules should be finished end-to-end before moving on so the user does not need to manage intermediate decisions.

**Tech Stack:** Go, `go test`, Claude Code TypeScript source under `claude-code/src`, Codex subagents for bounded review or implementation tasks.

---

## Execution Rule

- [ ] **Step 1: Review gate before every module**

Review the Go implementation and the matching `claude-code` source before writing production code.

Required outputs:
- target files in Go and Claude
- target runtime contract
- exact gaps
- tests that will prove parity
- module completion criteria

- [ ] **Step 2: TDD gate**

Write or extend tests for the module first, run them, and confirm they fail for the intended reason before implementation.

Run: `go test ./internal/tools/...`
Expected: targeted tests fail before code changes, unrelated packages remain untouched

- [ ] **Step 3: Implement the module completely**

Implement the smallest set of production changes needed to satisfy the runtime contract for the whole module, not a partial slice.

- [ ] **Step 4: Verify the module**

Run the narrow test set first, then the broader package regression suite.

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: all relevant tests pass

- [ ] **Step 5: Report only after module completion**

Do not interrupt the user for sub-decisions unless a true blocker appears. Report:
- what runtime contract is now matched
- what tests prove it
- what remains for the next module

## Module 1: Bundled Skill Runtime Contract Parity

**Files:**
- Modify: `internal/tools/bundled_skills.go`
- Modify: `internal/tools/bundled_skills_test.go`
- Modify: `internal/tools/extended_tools.go`
- Modify: `internal/tools/extended_tools_test.go`
- Modify: `internal/runtime/runner.go`
- Test: `internal/tools/bundled_skills_test.go`
- Test: `internal/tools/extended_tools_test.go`
- Test: `internal/runtime/runner_test.go`

- [ ] **Step 1: Lock the Claude runtime contract**

Review:
- `claude-code/src/skills/bundledSkills.ts`
- `claude-code/src/skills/bundled/*.ts`

Capture these required behaviors:
- bundled skill `files` are lazily extracted to a deterministic skill directory
- bundled skill prompt is prefixed with `Base directory for this skill: <dir>` when extraction succeeds
- bundled skill registration accepts runtime-backed prompt providers
- runtime initialization passes real environment-dependent options into bundled skill registration rather than empty defaults

- [ ] **Step 2: Write failing tests for bundled file extraction parity**

Add tests that prove:
- bundled skills with `Files` get a readable extracted directory
- skill invocation injects the base-directory prefix for bundled skills, not only filesystem skills
- extraction happens lazily on invocation rather than at registration time

Run: `go test ./internal/tools/...`
Expected: FAIL in the new bundled-skill extraction/base-dir tests

- [ ] **Step 3: Write failing tests for runtime-backed bundled options**

Add tests that prove runtime initialization does not call `InitClaudeBundledSkills` with an empty options struct when runtime state is available.

Run: `go test ./internal/runtime -run Bundled`
Expected: FAIL showing bundled skill init is still fed empty defaults

- [ ] **Step 4: Implement bundled file extraction support**

Port the Claude behavior into Go runtime:
- add bundled skill extraction directory handling
- materialize bundled reference files safely
- prefix bundled prompts with the extracted base directory

- [ ] **Step 5: Implement runtime-backed bundled option wiring**

Thread real runtime-derived inputs into `InitClaudeBundledSkills`, starting with values already available in the Go runtime and preserving current behavior where data is not yet implemented.

- [ ] **Step 6: Run focused tests**

Run: `go test ./internal/tools/... -run "Bundled|Skill"`
Expected: PASS

- [ ] **Step 7: Run module regression**

Run: `go test ./internal/tools/... ./internal/runtime ./internal/queryengine`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add docs/plan/2026-04-17-runtime-contract-parity.md internal/tools/bundled_skills.go internal/tools/bundled_skills_test.go internal/tools/extended_tools.go internal/tools/extended_tools_test.go internal/runtime/runner.go internal/runtime/runner_test.go
git commit -m "fix: align bundled skill runtime contract"
```
