Launch a runtime-owned subagent task when an independent agent can make progress without sharing the full parent context.

Use this tool for work such as broad code search, isolated investigation, parallel verification, or focused analysis that can return a compact result to the parent turn. Do not use it for a single small edit, a question that needs immediate user confirmation, or work you can complete directly with the context already available.

Input fields:

- `prompt` is required. Give the subagent a complete, self-contained task and state the expected result.
- `description` is optional. Use a short title that can be shown in task lists and timeline entries.
- `role` is optional. It selects an Agent Builder agent role by id.
- `subagent_type` is optional and is an alias for `role`.

If both `role` and `subagent_type` are provided, `role` wins. If neither is provided, Agent Builder uses the default task agent role. Unknown roles fail clearly and list the available role ids.

This first version does not support background, team, fork, remote, swarm, isolation, or custom working-directory behavior through this tool. Keep the delegated task bounded and report findings, artifacts, and blockers in the subagent result.
