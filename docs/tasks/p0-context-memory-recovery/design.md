# P0 Context Memory Recovery Design

Contracts:

- Workspace instructions are deterministic context inputs.
- Memory injection is explicit and ordered.
- Transcript blocks preserve tool-use and tool-result identity.
- Pending approvals are persisted and rehydrated.
- Invoked skills are recoverable into tool app state.
- Compaction boundaries are first-class recovery anchors.
- Read-file state and context cache boundaries are explicit, even when implementation remains minimal.