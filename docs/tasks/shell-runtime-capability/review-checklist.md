# Shell Runtime Capability Review Checklist

Date: 2026-04-25

Use this checklist when reviewing the shell runtime capability implementation.

## 1. Scope Check

- Does the implementation stay within shell runtime capability boundaries?
- Did the implementation avoid expanding into Docker, database, React UI, or TUI work?
- Is shell treated as a shared runtime capability instead of a client-owned behavior?

## 2. Claude Alignment Check

- Does shell remain a first-class tool/runtime surface?
- Is approval behavior still owned by shared runtime policy?
- Does lifecycle flow through shared runtime events?
- Is session/worktree context preserved?

## 3. Runtime Contract Check

- Are shell tool inputs and outputs explicit?
- Is failure behavior coherent and reviewable?
- Are structured result fields preserved where runtime already supports them?
- Is progress behavior handled on shared runtime paths?

## 4. Permission Check

- Is shell classified as a high-sensitivity execution surface?
- Is approval determined centrally rather than by gateway/UI logic?
- Are policy boundaries testable?

## 5. Control-Plane Check

- Does `myclawd` expose shell lifecycle events through shared websocket contracts?
- Are `tool.called`, `tool.progress`, and `tool.result` observable for shell execution?
- Is there any shell-specific client hack that should instead belong in runtime?

## 6. Session / Worktree Check

- Does shell execution respect main-session and child-session context?
- Does worktree execution use the correct working directory semantics?
- Are child-session execution boundaries coherent with subagent behavior?

## 7. Test Check

- Were shell tool tests added or updated?
- Were runtime and gateway tests added or updated?
- Was at least one functional validation path run?
- Are there any untested approval or lifecycle branches left behind?

## 8. Merge Bar

Do not approve if any blocking issue remains in:

- approval centralization
- gateway lifecycle visibility
- worktree/session execution semantics
- structured shell result propagation
