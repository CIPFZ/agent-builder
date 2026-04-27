# React Operator UI Console Test And Validation Plan

Date: 2026-04-25

## 1. Validation Goal

Validate that the React Operator UI can operate the current `myclaw` runtime through `myclawd`.

The validation goal is not visual parity with Claude Code.

The validation goal is capability visibility and control-plane correctness.

## 2. Automated Validation

Claude Code should add automated validation where practical.

Recommended:

- TypeScript typecheck
- frontend unit tests for protocol reducers
- websocket client request/response tests with mock server
- component tests for approval, tool card, task panel, and MCP inventory
- Playwright smoke test against local dev UI with a mocked or real `myclawd`

## 3. Manual Validation

The user should be able to validate the current implemented effects.

Required manual flow:

1. Start `myclawd`.
2. Start React UI dev server.
3. Connect the UI to `myclawd`.
4. Send a simple prompt and verify streaming response.
5. Trigger a file read or search and verify tool card/result display.
6. Trigger shell command flow and verify progress/result display.
7. Trigger SSH flow if configured and verify conservative approval/result display.
8. Trigger approval-required flow and verify approve/reject.
9. Open MCP panel and verify servers/tools/prompts/resources/skills.
10. Trigger or inspect skills visibility.
11. Spawn a subagent and verify task panel updates.
12. Inspect runtime/session status panel.

## 4. Real Usage Feedback Format

When the user tests, collect:

- which page or panel was used
- which runtime capability was tested
- what event/tool/result was expected
- what appeared in the UI
- whether the action completed, failed, or hung
- whether the issue is UI rendering, protocol data, or backend behavior

## 5. Backend Gap Validation

For every missing UI surface, classify it as:

- frontend implementation gap
- `myclawd` protocol gap
- runtime capability gap
- intentional non-goal

Do not mark a UI feature complete when it relies on mocked backend state.

## 6. Minimum Test Commands

Expected commands after implementation:

```bash
go test ./internal/gateway ./internal/protocol/... ./internal/runtime ./internal/queryengine
```

Frontend commands depend on the chosen package manager and must be documented after scaffold.

Expected examples:

```bash
npm install
npm run typecheck
npm test
npm run build
```

If the repo chooses `pnpm` or another manager, update these commands to match reality.
