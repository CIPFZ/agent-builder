# TUI Post-R7 Priority Roadmap

> **For future implementation work:** This roadmap is intentionally ordered by `real usage frequency > impact on the main workflow > implementation cost/dependencies > Claude Code semantic completeness`.

**Goal:** Define what should happen after R7, and separate high-value everyday TUI capabilities from lower-frequency or substrate-heavy parity work.

**Current state:** `main` already contains the R0-R7 baseline plus the integrated R7 overlays that were practical to land without inventing fake backend state. The remaining TUI gaps are no longer foundational shell modules; they are either workflow enhancers or platform-dependent parity surfaces.

---

## Priority Model

### P0: Daily Workflow Enhancers

These are the next TUI capabilities most likely to be used in normal repo work and most likely to improve day-to-day operator speed.

1. **File-level Quick Open**
   - Why now:
     - The command/object-level quick open surface already exists.
     - The next real user expectation is opening workspace files quickly, not just commands/tasks/sessions.
   - Expected user value:
     - Jump directly to source files from the TUI.
     - Reduce command friction when moving around the repo.
   - Dependency level:
     - Medium
     - Needs workspace file indexing and probably preview-friendly file reads.

2. **Workspace Global Search**
   - Why now:
     - Search is part of the daily core workflow in real repositories.
     - This has higher day-to-day value than remote QR/IDE surfaces.
   - Expected user value:
     - Search the workspace without dropping out of the TUI.
     - Support investigation and navigation during coding/debugging sessions.
   - Dependency level:
     - Medium
     - Likely backed by `rg` and a preview/result navigation model.

3. **Attachment Semantics**
   - Why now:
     - Claude Code's richer prompt/message semantics eventually assume attachments exist.
     - This affects real prompt composition and transcript comprehension.
   - Expected user value:
     - More realistic conversation input/output semantics.
     - Better parity for file/image/resource-aware interactions.
   - Dependency level:
     - Medium
     - Requires message model and rendering expansion, but not remote substrate.

4. **Message/Product Block Fidelity Pass 2**
   - Why now:
     - R6 improved transcript structure, but product-level message semantics are still incomplete.
     - This supports better tool output reading and future attachment work.
   - Expected user value:
     - More readable transcripts.
     - Better fidelity for thinking/tool/result/system/product blocks.
   - Dependency level:
     - Medium

### P1: Power-User Interaction Upgrades

These matter, but they are not the first capabilities most users will miss every day.

1. **Vim / Advanced Keybinding State Machine**
   - Why later:
     - High value for a subset of users.
     - More state-machine complexity than immediate general-user ROI.
   - Expected user value:
     - Faster editing/navigation for heavy terminal users.
   - Dependency level:
     - Medium to high

2. **MCP Elicitation / Prompt Overlay**
   - Why later:
     - Important for deeper MCP parity, but only matters when MCP prompt flows are active.
   - Expected user value:
     - Proper interactive MCP prompt/elicitation handling in TUI.
   - Dependency level:
     - Medium
     - The runtime already has data structures, but the user-facing overlay flow does not exist.

3. **Long-Session Virtualization / Performance Layer**
   - Why later:
     - Important when transcript size becomes a real performance bottleneck.
     - Not necessarily the next missing feature if current sessions remain manageable.
   - Expected user value:
     - Better behavior on very large transcripts.
   - Dependency level:
     - High

### P2: Platform / Product Surface Parity

These are real Claude Code semantics, but they should not be prioritized ahead of the high-frequency workflow enhancers above.

1. **Remote-only `/session` URL / QR**
   - Why not first:
     - Narrower usage than file quick open and workspace search.
     - Depends on remote session substrate.
   - Missing prerequisite:
     - Reliable remote-mode state and remote session URL source in Go runtime.

2. **`/ide` Connection Management and IDE Status Surface**
   - Why not first:
     - Useful only when IDE bridge semantics are actually present.
     - TUI should not invent fake state just to match the command surface.
   - Missing prerequisite:
     - IDE detection, connection state, and selection model in backend/runtime.

3. **Remote / Bridge / IDE Substrate**
   - Why separate:
     - This is not a small TUI tail task.
     - It is the beginning of a new platform integration stage.
   - Missing prerequisite:
     - A deliberate backend roadmap, not just TUI overlays.

---

## Recommended Stage Split

### R8: Main Workflow Enhancement

This should be the next roadmap stage because it directly improves everyday TUI usage without requiring heavy platform substrate first.

**Scope**

- File-level quick open
- Workspace global search
- Attachment semantics
- Message/product block fidelity pass 2

**Why this should be next**

- Highest day-to-day user value
- Strong alignment with real coding workflows
- Avoids stalling on remote/IDE dependencies
- Builds naturally on the already-landed quick open and transcript foundation

### R9: Advanced Interaction And Platform Integration

This should happen after R8, once the main workflow surfaces are stronger.

**Scope**

- Vim / advanced keybinding state machine
- MCP elicitation overlay
- Long-session virtualization
- Remote-only `/session`
- `/ide` surface
- Remote / bridge / IDE substrate planning or initial implementation

**Why this should be later**

- Higher complexity
- Narrower everyday usage
- More backend dependency risk
- Easier to scope correctly after R8 confirms the next most valuable workflow surfaces

---

## Recommended Worktree Order

### First: `codex/tui-r8-file-nav`

**Primary goal**

- File-level quick open

**Why first**

- Lowest ambiguity among remaining high-value features
- Builds directly on the newly integrated quick open overlay
- Easy to validate with focused TUI tests

### Second: `codex/tui-r8-workspace-search`

**Primary goal**

- Workspace global search

**Why second**

- Complements file quick open
- Clear standalone user value
- Can reuse picker/preview concepts from quick open

### Third: `codex/tui-r8-message-fidelity-2`

**Primary goal**

- Attachment semantics and richer message/product block fidelity

**Why third**

- This has broader transcript/model implications
- Better to tackle after navigation/search surfaces are settled

### Fourth: `codex/tui-r9-platform-substrate`

**Primary goal**

- Decide whether remote/IDE substrate should be built in TUI-adjacent layers or in a broader runtime/gateway phase

**Why fourth**

- This should be treated as a deliberate platform decision, not an opportunistic UI patch

---

## Do Not Prioritize Yet

The following should stay explicitly de-prioritized until the higher-value stages above are complete:

- Remote QR/session sharing surface
- `/ide` command parity
- Full remote bridge parity
- Full Vim-mode parity
- Transcript virtualization as a first-class project

---

## Decision Summary

If the next question is "what should we do now?" the answer should be:

1. **Do R8 first**
2. **Start with file-level quick open**
3. **Then add workspace global search**
4. **Then do attachment/message fidelity expansion**
5. **Leave remote/IDE work for a separate post-R8 stage**
