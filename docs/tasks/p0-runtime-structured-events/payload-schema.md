# P0 Runtime Structured Events Payload Schema

Date: 2026-04-28

Runtime event payloads are projected by `runtime.RuntimeEvent.Payload()` and serialized by gateway `runtimeSink` for event families that do not require legacy compatibility shaping.

Stable fields:

- `type`: runtime event name.
- `session_id`, `session_key`, `agent_id`: session identity when available.
- `run_id`: runtime turn/run identity.
- `message`: message object with `id`, `role`, `content`, and `created_at` when a message is attached.
- `message_id`, `message_role`, `message_content`: scalar message compatibility fields.
- `delta`: assistant stream delta.
- `tool_use_id`, `provider_message_id`, `tool_name`, `tool_input`, `tool_input_object`, `tool_error`: tool lifecycle identity and result state.
- `progress`: tool progress object for runtime-native consumers.
- `structured_content`: structured tool result content, deep-cloned by payload projection.
- `meta`: tool/runtime metadata.
- `decision_reason`, `decision_reason_details`, `accept_feedback`, `content_blocks`: permission and approval prompt metadata.
- `error` and `message`: error text for runtime and existing client compatibility.
- `approval_id`, `approval_status`, `status`, `reason`, `category`, `rule_source`: approval lifecycle state.

Compatibility notes:

- Existing TUI/gateway clients consume `approval.updated`, so `runtime.EventApprovalResolved` is intentionally aligned to that event name.
- Existing compaction consumers use `compact.memory_saved` for the summary/memory-save lifecycle, so `runtime.EventCompactSummary` is intentionally aligned to that event name.
- `tool.progress` keeps its legacy top-level progress payload shape in gateway serialization for current TUI compatibility.
