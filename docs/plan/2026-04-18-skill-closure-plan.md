# Skill Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close all remaining Claude Code skill runtime parity gaps in the Go runtime so skill discovery, listing, filtering, and invocation semantics match `claude-code` 1:1.

**Architecture:** Introduce a Claude-shaped skill command catalog in Go, route listing and `Skill` resolution through that catalog, and then align dynamic discovery/policy refresh behavior with Claude’s source precedence and gating rules. Execute in TDD order: failing tests first, minimal implementation, then integration verification.

**Tech Stack:** Go, standard library testing, existing queryengine/runtime/tooling packages, local `claude-code` TypeScript source as parity oracle.

---

### Task 1: Skill Command Catalog Parity

**Files:**
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/registry.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/extended_tools.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/queryengine.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/extended_tools_test.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/tool_lifecycle_test.go`

- [ ] **Step 1: Write failing tests for Claude-style command filtering**

Add tests that prove:
- skill listing excludes prompt commands without Claude-required metadata
- plugin/MCP skills only appear when they have explicit description or `when_to_use`
- `disable-model-invocation` items are excluded from model-invocable listing
- MCP prompts are not treated as MCP skills for skill listing

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:
```powershell
go test ./internal/tools ./internal/queryengine -run "Test(BuildSkillListing|QueryEngineSkillListing|SkillTool).*"
```

Expected: FAIL because current Go filtering still uses ad hoc listing semantics.

- [ ] **Step 3: Expand the Go command model to carry Claude filtering metadata**

Add Claude-relevant fields to the Go command representation used by runtime/queryengine skill listing, mirroring the minimum needed from `claude-code/src/types/command.ts`:
- `HasUserSpecifiedDescription`
- `WhenToUse`
- `DisableModelInvocation`
- `UserInvocable`
- `LoadedFrom`
- `IsHidden`
- `Source`

- [ ] **Step 4: Route skill listing through Claude-style filtering rules**

Implement catalog helpers equivalent to:
- `getMcpSkillCommands`
- `getSkillToolCommands`
- `getSlashCommandToolSkills`

and make queryengine skill listing consume these helpers instead of raw concatenation.

- [ ] **Step 5: Re-run targeted tests**

Run:
```powershell
go test ./internal/tools ./internal/queryengine -run "Test(BuildSkillListing|QueryEngineSkillListing|SkillTool).*"
```

Expected: PASS for the new listing/filtering cases.

### Task 2: SkillTool Exposure And Invocation Parity

**Files:**
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/extended_tools.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/queryengine.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/extended_tools_test.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/tool_lifecycle_test.go`

- [ ] **Step 1: Write failing tests for unified command-surface resolution**

Add tests that prove:
- `Skill` invocation only resolves names reachable through Claude-equivalent command exposure
- hidden/non-user-invocable skills do not leak into listing
- MCP prompt skills are not exposed as model-invocable skills unless they are MCP skills
- `Skill` execution still succeeds for valid bundled/plugin/local/MCP skill commands

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:
```powershell
go test ./internal/tools ./internal/queryengine -run "TestSkillTool|TestQueryEngineSkillListing"
```

Expected: FAIL on the new command-surface parity cases.

- [ ] **Step 3: Replace ad hoc `resolveSkillCommand` reachability with catalog-aware lookup**

Keep the existing execution backends, but gate visible/resolvable model-invocable skills through the same catalog used for listing. Preserve legitimate direct resolution paths required by runtime state such as bundled and dynamically activated skills.

- [ ] **Step 4: Re-run targeted tests**

Run:
```powershell
go test ./internal/tools ./internal/queryengine -run "TestSkillTool|TestQueryEngineSkillListing"
```

Expected: PASS for all SkillTool exposure and invocation parity tests.

### Task 3: Discovery, Policy, And Dynamic Refresh Parity

**Files:**
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/skill_discovery.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/queryengine.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/skill_discovery_test.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/tools/extended_tools_test.go`
- Modify: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/tool_lifecycle_test.go`

- [ ] **Step 1: Write failing tests for Claude discovery parity**

Add tests that prove:
- file identity dedupe follows `realpath` semantics rather than path+mtime+size
- dynamic skill discovery respects project/plugin policy gates
- command surface refreshes after dynamic skill changes without rebuilding the entire engine
- conditional/path-based activation keeps Claude visibility behavior

- [ ] **Step 2: Run targeted tests to verify they fail**

Run:
```powershell
go test ./internal/tools ./internal/queryengine -run "Test(LoadClaudeSkillDirectories|DiscoverSkillDirs|AddSkillDirectories|QueryEngineInjectsSkillListing).*"
```

Expected: FAIL on the new policy/dedupe/refresh cases.

- [ ] **Step 3: Implement Claude-style discovery and refresh behavior**

Align:
- discovery gating
- first-wins source precedence
- file identity dedupe
- dynamic skill activation refresh hooks

so command availability updates mid-session like Claude’s `getCommands()`.

- [ ] **Step 4: Re-run targeted tests**

Run:
```powershell
go test ./internal/tools ./internal/queryengine -run "Test(LoadClaudeSkillDirectories|DiscoverSkillDirs|AddSkillDirectories|QueryEngineInjectsSkillListing).*"
```

Expected: PASS for discovery and dynamic refresh parity.

### Task 4: Full Skill Regression Verification

**Files:**
- Verify only: `C:/Users/ytq/work/ai/agent-builder/internal/tools/...`
- Verify only: `C:/Users/ytq/work/ai/agent-builder/internal/queryengine/...`
- Verify only: `C:/Users/ytq/work/ai/agent-builder/internal/runtime/...`

- [ ] **Step 1: Run the full skill-focused test suites**

Run:
```powershell
go test ./internal/tools ./internal/queryengine ./internal/runtime
```

Expected: PASS with no skill parity regressions.

- [ ] **Step 2: Run a repo-wide focused parity smoke check**

Run:
```powershell
go test ./...
```

Expected: PASS, or if unrelated pre-existing failures exist, isolate and report them with evidence.

- [ ] **Step 3: Review final diff against the three parity goals**

Verify the resulting implementation now matches the intended closure goals:
- command catalog parity
- SkillTool exposure/invocation parity
- discovery/policy/dynamic refresh parity

