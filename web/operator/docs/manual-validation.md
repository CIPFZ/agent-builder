# React Operator UI Manual Validation

Date: 2026-04-25

## Start

1. Start `myclawd` in the repository root.
2. Start the frontend:

```bash
cd web/operator
npm install
npm run dev
```

3. Open the Vite URL and connect to `ws://127.0.0.1:18080/ws` unless your gateway is bound elsewhere.

## Validate Core Flows

1. Send a simple prompt from the `Sender` input and verify:
   - the user message appears in transcript
   - `assistant.delta` streams into the active assistant bubble
   - final `message.created` reconciles the transcript

2. Ask for a file read/search operation and verify:
   - `tool.called` and `tool.progress` appear in the Tools tab
   - the Files / Exec tab shows the file operation
   - the tool drawer exposes structured result payload when present

3. Ask for a shell command and verify:
   - tool lifecycle cards update
   - Files / Exec shows shell execution visibility
   - stdout/stderr-like progress events appear in the detail drawer

4. Ask for an SSH operation if configured and verify:
   - execution appears in the same tool lifecycle surface
   - if approval is required, it appears in Approval Center

5. Trigger an approval-required action and verify:
   - `permission.required` creates a pending card
   - approve and reject both send backend requests
   - `approval.updated` / `approval.cleared` reconcile the panel

6. Open the MCP tab and verify:
   - inventory counts render
   - each server row shows tools/prompts/resources/skills visibility

7. Trigger or inspect skill usage and verify:
   - MCP-derived skills appear in Skills
   - `Skill` tool invocations become visible in invoked skills after tool completion

8. Spawn a subagent from another client flow if available and verify:
   - `subagent.updated` / `subagent.completed` appear in the Subagents tab
   - status and last action update without refresh

9. Open the Runtime tab and verify:
   - session id/key/agent/model/permission state render
   - permission and model updates call backend methods successfully

## Expected Known Limits

- Cold reload transcript restoration is incomplete without a `session_history` method.
- Full skill catalog is incomplete without `skills_status`.
- File browser/diff explorer is incomplete without dedicated file APIs.
