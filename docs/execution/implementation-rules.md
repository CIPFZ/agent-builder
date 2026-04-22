# Implementation Rules

## Rule 1: Claude Source First

For every major capability:

1. identify the Claude Code source module that owns the semantics
2. review that source before changing Go code
3. define the Go ownership boundary to match the source semantics as closely as practical

## Rule 2: Runtime First, UI Later

Do not solve backend capability gaps in frontend code.

If a capability is real, it must exist first in:

- Go runtime
- `myclawd` protocol

Only then can the React UI consume it.

## Rule 3: TUI Is A Lightweight Shell

The Go TUI is not the primary parity target.

Allowed TUI investments:

- prompt input
- basic message flow
- approval handling
- basic task views
- basic runtime debugging

Avoid heavy investment in:

- advanced UI parity
- visual fidelity work
- deep interaction systems that duplicate planned React UI effort

## Rule 4: Prefer Stable Contracts Over Ad Hoc Shortcuts

Do not add one-off client-specific paths when a shared protocol is needed.

This applies especially to:

- approvals
- task control
- tool progress
- runtime inventory

## Rule 5: Test Before Expansion

For each new backend capability:

1. add or update focused tests
2. verify the failing condition if possible
3. implement the minimal aligned change
4. run focused tests
5. run the relevant functional flow

## Rule 6: Build The Execution Surface As First-Class Runtime Capability

Capabilities like ssh, docker, and db must not remain "just shell snippets".

Each must have:

- explicit runtime contract
- permission and approval integration
- progress and result semantics
- control-plane support

## Rule 7: Delete Stale Planning Aggressively

Keep only documentation that still has direct execution value for the active architecture and roadmap.

Do not preserve old planning just because it exists.

## Rule 8: This Agent Owns Planning And Review

For this collaboration mode, the primary responsibility of this agent is:

- produce plans
- produce reviews
- validate architecture direction

Implementation and code execution can be delegated to Claude Code or another implementation agent, but planning quality here must be high enough that implementation can proceed with minimal ambiguity.

## Rule 9: Every Plan Must Be Jointly Aligned To Three Inputs

Before writing any new plan, always align it against all three of these inputs together:

1. the concrete user requirement
2. the current `myclaw` target architecture
3. the relevant Claude Code source modules that define the intended semantics

Do not write plans from only one of these perspectives.

## Rule 10: Plans Must Be Detailed, Accurate, And Executable

Every plan must be detailed enough for downstream implementation to execute reliably.

That means each plan should make clear:

- objective and scope
- source references in Claude Code
- current Go-side ownership points
- target design
- module and file impact
- sequencing
- risks and non-goals
- acceptance criteria
- validation approach

Do not produce vague roadmap-only planning when the next implementation step requires an executable design.

## Rule 11: Save Plans Under docs/ As Task-Scoped Design Artifacts

All new plans must be written into `docs/`.

Preferred structure:

- one task, one dedicated folder when the task is large enough to justify it
- one task, one dedicated markdown file when the task is small

The default pattern for substantial work should be:

- `docs/tasks/<task-name>/design.md`
- optional companion files in the same folder when needed

## Rule 12: Planning Comes Before Implementation

For each meaningful task:

1. understand the requirement
2. inspect the current architecture and code ownership
3. inspect the corresponding Claude Code source
4. write the plan into `docs/`
5. review the plan for correctness and completeness
6. only then hand implementation off

Do not jump directly from request to implementation guidance without writing the plan artifact first.
