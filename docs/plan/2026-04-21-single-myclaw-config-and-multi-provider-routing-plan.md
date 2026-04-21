# Single Myclaw Config And Multi-Provider Routing Plan

Date: 2026-04-21

## Goal

Replace the current multi-source configuration model with a single production-grade `configs/myclaw.json` design, while adding multi-provider LLM support, profile-based model routing, and explicit `agent type -> profile` binding.

This plan intentionally diverges from Claude Code configuration semantics. The runtime parity goal remains for agent behavior, but the configuration subsystem becomes a `myclaw`-native design.

## Design Decisions

- `configs/myclaw.json` is the only formal config file
- `.claude/settings*.json` is no longer read
- environment variables override `myclaw.json`
- `myclaw.json` supports both direct values and `${ENV_VAR}` interpolation
- multiple providers may coexist at once
- the runtime uses a default profile for the main thread
- subagents resolve their profile by explicit `agent type -> profile` routing
- profile selection may later be overridden explicitly by runtime inputs, but the default behavior is deterministic and static

## Required Behavior

### Config File

`configs/myclaw.json` must define:

- `config.version`
- `server`
- `llm.providers`
- `llm.profiles`
- `llm.routing`
- `permissions`
- `mcp`
- `compact`
- optional `session`
- optional `logging`

### LLM Providers

Each provider definition must support:

- provider name
- protocol
- base URL
- API key
- model defaults where needed
- headers
- timeout
- retry policy
- enable/disable switch

Supported protocol families in this implementation phase:

- `openai-compatible`
- `anthropic`

### Profiles

Profiles are logical execution presets that bind:

- provider
- model
- streaming behavior
- protocol-specific options when necessary

### Routing

Routing must support:

- global default profile
- explicit `agent type -> profile` bindings
- deterministic fallback to default profile when no binding exists

## Files To Modify

### Config Layer

- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `configs/myclaw.example.json`
- Modify: `configs/myclaw.json`

### LLM Layer

- Modify: `internal/llm/factory.go`
- Modify: `internal/llm/client.go`
- Modify: `internal/llm/openai_compatible.go`
- Create: `internal/llm/anthropic.go`
- Create: `internal/llm/provider_routing.go`
- Create: `internal/llm/provider_routing_test.go`
- Create: `internal/llm/anthropic_test.go`

### Runtime And Agent Routing

- Modify: `internal/runtime/runner.go`
- Modify: `internal/tools/agent_tool.go`
- Modify: `internal/agent/manager.go`
- Modify: `internal/queryengine/model_resolution.go`
- Create or modify tests in:
  - `internal/runtime/runner_test.go`
  - `internal/tools/agent_tool_test.go`
  - `internal/agent/manager_test.go`

### Documentation

- Modify: `README.md` if config usage is documented there
- Add or update: `docs/plan/*` only if implementation notes need follow-up

## Implementation Phases

### Phase 1: Replace Config Model

#### Task 1.1: Define New Config Schema In Tests

Write failing tests for:

- loading only from `configs/myclaw.json`
- no longer reading user settings path
- no longer reading `.claude/settings.json`
- no longer reading `.claude/settings.local.json`
- environment variable override precedence
- `${ENV_VAR}` interpolation within `myclaw.json`
- provider/profile/routing schema parsing

Expected outputs:

- config loads from a single file source
- legacy external config sources are ignored
- parsed config contains multiple providers and profiles

#### Task 1.2: Refactor `internal/config/config.go`

Implement:

- new config structs
- single-file loading from `configs/myclaw.json`
- environment interpolation support
- environment override support
- schema validation by explicit Go checks after JSON unmarshal

Validation must fail clearly for:

- missing `llm.default_profile` or routing default
- unknown profile references
- unknown provider references
- empty provider protocol
- empty API key after env resolution when provider is enabled

#### Task 1.3: Rewrite Example Config

Update:

- `configs/myclaw.example.json`
- `configs/myclaw.json`

The example must show:

- one OpenAI-compatible provider
- one Anthropic provider
- multiple profiles
- routing for agent types such as `frontend`, `review`, `worker`

### Phase 2: Provider-Aware LLM Factory

#### Task 2.1: Add Provider Routing Tests

Write failing tests for:

- selecting client by active profile
- selecting Anthropic client when profile points to Anthropic provider
- selecting OpenAI-compatible client when profile points to that provider
- resolving agent type to routed profile
- fallback to default profile when no explicit agent binding exists

#### Task 2.2: Introduce Routing Resolver

Add a provider/profile resolver in `internal/llm/provider_routing.go` that:

- resolves default profile
- resolves routed profile for agent type
- returns provider config plus profile config together
- fails hard on invalid references

#### Task 2.3: Refactor Factory

Change `internal/llm/factory.go` so it:

- no longer assumes one flat `LLMConfig`
- builds the correct client from resolved provider/profile
- supports both `openai-compatible` and `anthropic`

### Phase 3: Add Production Anthropic Client

#### Task 3.1: Add Anthropic Streaming Tests

Write failing tests for:

- request body shape for Anthropic messages API
- required headers such as API key and version
- streaming response parsing
- text delta events
- tool call events if supported by current runtime contract

#### Task 3.2: Implement `internal/llm/anthropic.go`

Implement a production client with:

- request assembly
- timeout handling
- authentication header support
- SSE-style streaming consumption
- event translation into the existing `StreamEvent` contract

The implementation must be production-grade, not mock-compatible glue.

### Phase 4: Agent-Type Profile Routing

#### Task 4.1: Add Routing Tests For Subagents

Write failing tests for:

- main thread using default profile
- `frontend` agent using its bound profile
- `review` agent using its bound profile
- `worker` agent using its bound profile
- unknown agent type falling back to default profile

#### Task 4.2: Thread Profile Resolution Through Runtime

Update runtime and agent spawning so:

- the main session resolves its profile once at startup
- spawned agents resolve profile from `agent type`
- run metadata records chosen provider/profile/model for observability

This work should modify:

- `internal/runtime/runner.go`
- `internal/tools/agent_tool.go`
- `internal/agent/manager.go`

### Phase 5: Clean Out Legacy Config Semantics

#### Task 5.1: Remove Legacy Config Source Logic

Delete or stop using:

- user config path loading
- project `.claude/settings.json` loading
- local `.claude/settings.local.json` loading

Tests must prove they are ignored.

#### Task 5.2: Remove Anthropic Model Env Fallback As Config Logic

Current config resolution mixes `MYCLAW_LLM_MODEL` and `ANTHROPIC_MODEL`.

Refactor so:

- model resolution happens through profile selection
- vendor-specific env vars do not silently mutate active model selection
- explicit environment override remains possible through documented `MYCLAW_*` variables only

### Phase 6: Verification

Run at minimum:

- `go test ./internal/config ./internal/llm ./internal/agent ./internal/tools ./internal/runtime`
- `go test ./...`
- `go build -o myclaw .\\cmd\\myclaw`
- `go build -o myclawd .\\cmd\\myclawd`

## Test Strategy

### Config Tests

Must cover:

- single source loading
- env override precedence
- env interpolation
- invalid schema failure
- invalid routing references

### LLM Tests

Must cover:

- provider factory selection
- request shaping
- stream event conversion
- protocol-specific auth headers

### Routing Tests

Must cover:

- default profile behavior
- routed agent behavior
- fallback behavior

## Risks

- current code assumes one flat `LLMConfig`; this refactor touches runtime wiring broadly
- Anthropic streaming event translation may expose differences in current tool-call assumptions
- subagent routing must avoid silently reusing the parent model when the new design expects explicit routed profiles

## Non-Goals For This Phase

- automatic task-type based provider routing
- dynamic policy-based model switching
- multiple config file merging
- `.claude` compatibility
- plugin-driven provider injection

## Completion Criteria

This work is done when:

- the runtime uses only `configs/myclaw.json` plus env overrides
- multiple providers can be declared together
- the active main-thread profile resolves correctly
- subagents resolve provider/profile by explicit agent type binding
- both `openai-compatible` and `anthropic` clients build and stream through the current runtime
- tests fail first and then pass after implementation
- `myclaw` and `myclawd` rebuild successfully
