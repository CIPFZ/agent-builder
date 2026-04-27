# React Operator UI Console Review Checklist

Date: 2026-04-25

Use this checklist when reviewing the React UI implementation branch.

## 1. Architecture Boundary

- Does the React UI use `myclawd` only?
- Are websocket requests and events centralized in a client module?
- Are runtime semantics kept out of frontend code?

## 2. Capability Coverage

- Chat is visible and usable.
- Tool lifecycle is visible.
- File tool effects are visible.
- Shell and SSH executions are visible.
- Approvals are actionable.
- MCP inventory is visible.
- Skills visibility exists with documented gaps.
- Subagents/tasks are visible and controllable.
- Runtime/session status is visible.

## 3. Ant Design X Usage

- `Bubble`/`Bubble.List` is used for transcript.
- `Sender` is used for the composer.
- `Conversations` or equivalent session navigation exists.
- Ant Design operational components are used for tables, drawers, forms, and timelines.

## 4. Protocol Gap Discipline

- Missing backend data is documented.
- UI does not hardcode fake runtime inventory as if it were real.
- Gaps are classified as frontend, protocol, runtime, or non-goal.

## 5. Validation

- Frontend typecheck/build passes.
- Protocol reducer/client tests exist.
- At least one smoke path validates connect -> send_message -> event render.
- Manual validation steps are documented.

## 6. Merge Blockers

Do not approve if:

- React calls runtime internals directly
- approvals are decided in frontend
- tool/task/MCP/skills state is fabricated
- UI only implements chat and ignores the already implemented runtime capabilities
- Docker or database are presented as first-class semantic modules without matching backend runtime contracts
