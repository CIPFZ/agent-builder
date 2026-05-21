# Legacy Crush Inventory

This inventory is the Phase 1 output for the project structure refactor. It
classifies current files and directories before any move, delete, or rename is
performed.

Status labels:

| Label | Meaning |
| --- | --- |
| keep | Required on the current product path. |
| migrate | Required, but should move to a new package or directory. |
| legacy | Keep temporarily for Crush CLI/TUI compatibility, but keep out of the client runtime path. |
| archive | Historical or reference material; move to `docs/archive/` after confirmation. |
| delete | Clearly unused or generated duplication that can be removed after references are gone. |

## Summary

The repository is still rooted in the original Crush CLI/TUI application while
also carrying the Agent Builder desktop PoC. The highest-risk coupling is that
the current desktop runtime implementation lives under
`desktop/` and still consumes Bubble Tea `tea.Msg` events through
`internal/backend` and `internal/ui` event shapes.

The immediate migration path should keep the existing working build intact,
then narrow the product path around:

```text
client/ -> desktop adapter -> internal/runtime -> internal/agent/tools
```

## Root Directory

| Path | Label | Reason | Next action |
| --- | --- | --- | --- |
| `AGENTS.md` | keep | Active repository instructions for development. | Keep at root. |
| `README.md` | migrate | Mostly original Crush README plus Agent Builder desktop notes. It mixes product docs, CLI docs, config docs, and desktop build notes. | Split into Agent Builder README plus legacy Crush reference docs. |
| `CLA.md` | archive | Looks like upstream/legal or project reference material, not part of the Agent Builder runtime path. | Confirm legal/product need, then move to `docs/archive/` or `docs/reference/`. |
| `LICENSE.md` | keep | Required project license material. | Keep at root. |
| `main.go` | legacy | Root CLI entry point for Crush command tree. Not part of desktop runtime path. | Keep until CLI adapter boundary exists; later move under `internal/adapters/cli` or a `cmd/` package if the CLI remains supported. |
| `go.mod`, `go.sum` | keep | Root Go module for core packages, tests, and current CLI. Should become the single Go module used by desktop too. | Keep and make desktop depend on this module directly after single-layer desktop migration. |
| `Taskfile.yaml` | migrate | Still centered on Crush CLI tasks (`build`, `run`, schema, release), but also contains useful project tasks. | Split or rename tasks into product, CLI legacy, and generated-code sections. |
| `crush.json` | keep | Current project-level Crush config for gopls/LSP. Useful while the repo is still developed with Crush. | Keep for now; document that it is a developer config, not Agent Builder runtime config. |
| `schema.json` | migrate | Generated Crush config schema from `go run main.go schema`; not runtime-critical for the desktop client. | Move to `docs/reference/` or generate on demand once config ownership is clarified. |
| `sqlc.yaml` | keep | Used by `task sqlc` and `internal/db/sql`. DB remains on the product path. | Keep at root unless SQL generation is moved under scripts. |
| `.goreleaser.yml` | archive | Upstream Crush release pipeline: packages `crush`, completions, man pages, AUR/Homebrew/NPM/etc. Not suitable for Agent Builder desktop releases. | Archive or replace with desktop release packaging after Wails path is stable. |
| `flake.nix`, `flake.lock`, `.envrc` | archive | Nix dev shell is useful only if the team actively uses it; current metadata still says "Crush development environment". | Confirm team usage. If unused, archive/delete; if used, update to Agent Builder wording and desktop dependencies. |
| `.golangci.yml` | keep | Active lint config used by `task lint`. | Keep. |
| `.gitattributes`, `.gitignore` | keep | Repository hygiene files. | Keep; update after desktop flattening removes nested generated paths. |
| `.github/` | keep | CI and repository automation likely still needed. | Keep, then audit workflows after package moves. |
| `.agents/` | keep | Local agent/skill metadata. | Keep. |
| `scripts/` | keep | Contains active log capitalization and labeler scripts referenced by root tasks/automation. | Keep; review scripts after CLI release tasks are split. |

## Desktop Directory

| Path | Label | Reason | Next action |
| --- | --- | --- | --- |
| `desktop/` | migrate | Current Wails app is nested one level too deep. The plan requires single-layer `desktop/`. | Move Wails shell files into `desktop/` in Phase 2. |
| `desktop/main.go` | migrate | Wails desktop entry point, belongs in the desktop adapter. | Move to `desktop/main.go`. |
| `desktop/runtime_bridge.go` | migrate | Wails-facing bridge code. It should stay desktop-owned, but at single-layer path. | Move to `desktop/runtime_bridge.go`; later reduce it to adapter calls into `internal/runtime`. |
| `desktop/runtime_bridge_test.go`, `runtime_bridge_live_test.go` | migrate | Tests cover the bridge and should move with the bridge. | Move to `desktop/`; update imports after runtime extraction. |
| `desktop/runtime_service.go` | migrate | Generic runtime service assembly, not a desktop concern. | Move to `internal/runtime/service.go`. |
| `desktop/runtime_service_types.go` | migrate | Generic runtime service contracts and state. | Move to `internal/runtime/service_types.go` or merge into `service.go`. |
| `desktop/runtime_contract_types.go` | migrate | Runtime API contract types overlap with `internal/runtimeapi`. | Consolidate into `internal/runtimeapi` or `internal/runtime/contract_types.go`. |
| `desktop/runtime_internal_types.go` | migrate | Runtime-private types. | Move to `internal/runtime/internal_types.go`. |
| `desktop/runtime_lifecycle.go` | migrate | Runtime lifecycle is not Wails-specific. | Move to `internal/runtime/lifecycle.go`. |
| `desktop/runtime_status.go` | migrate | Generic runtime status handling. | Move to `internal/runtime/status.go`. |
| `desktop/runtime_turns.go` | migrate | Generic turn lifecycle. | Move to `internal/runtime/turns.go`. |
| `desktop/runtime_sessions.go` | migrate | Generic session operations. | Move to `internal/runtime/sessions.go`. |
| `desktop/runtime_events.go` | migrate | Generic event recording but currently consumes `pubsub.Event[tea.Msg]`. | Move only after replacing `tea.Msg` with runtime-native events or isolating conversion in a TUI adapter. |
| `desktop/runtime_permissions.go` | migrate | Generic permission request/decision handling. | Move to `internal/runtime/permissions.go`; later pair with `PermissionPolicy`. |
| `desktop/runtime_audit.go`, `runtime_audit_writer.go`, `runtime_audit_test.go` | migrate | Generic audit storage/writing. | Move to `internal/runtime/audit*.go`. |
| `desktop/runtime_capabilities.go` | migrate | Generic capability reporting. | Move to `internal/runtime/capabilities.go`. |
| `desktop/runtime_skills.go`, `runtime_skill_config.go` | migrate | Generic skill listing/configuration. | Move to `internal/runtime/skills.go` and keep config ownership explicit. |
| `desktop/runtime_mcp.go`, `runtime_mcp_config.go` | migrate | Generic MCP runtime operations/config. | Move to `internal/runtime/mcp*.go`. |
| `desktop/runtime_model.go`, `runtime_model_config.go` | migrate | Generic model config/runtime operations. | Move to `internal/runtime/model*.go`. |
| `desktop/runtime_http.go`, `runtime_http_test.go` | migrate | Local HTTP adapter is useful, but should not be owned by the Wails package. | Move to `internal/adapters/http` or `internal/runtime/http.go` depending on final adapter boundary. |
| `desktop/runtime_sse.go` | migrate | SSE transport adapter. | Move to `internal/adapters/http` or `internal/runtime/sse.go`; consume runtime events only. |
| `desktop/runtime_mapping.go`, `runtime_utils.go` | migrate | Mostly conversion/helper logic for runtime contracts. | Move into `internal/runtime` or `internal/runtimeapi` after duplication is reviewed. |
| `desktop/go.mod`, `go.sum` | legacy | Independent desktop Go module retained for a conservative Phase 2 move; it now uses `replace github.com/charmbracelet/crush => ..`. | Remove in a later pass after desktop builds from the root module. |
| `desktop/README.md` | migrate | Useful desktop build/smoke instructions, but path references will be wrong after flattening. | Move/update to `desktop/README.md` or merge into root docs. |
| `desktop/Taskfile.yml` | migrate | Wails build tasks for the nested app. | Move/update to `desktop/Taskfile.yml` or root `Taskfile.yaml`. |
| `desktop/.gitignore` | migrate | Desktop-specific generated artifacts. | Merge into root `.gitignore` or keep as `desktop/.gitignore`. |
| `desktop/scripts/sync-client-dist.mjs` | migrate | Active build helper that syncs `client/dist` into Wails assets. | Move to `desktop/scripts/` and update paths. |
| `desktop/scripts/phase2-smoke.ps1` | migrate | Active packaged desktop smoke test. | Move to `desktop/scripts/` and update paths. |
| `desktop/frontend/package.json`, `package-lock.json` | delete | Minimal placeholder package next to embedded `frontend/dist`; real React app is `client/`. | Remove after Wails asset embedding no longer expects this package metadata. |
| `desktop/frontend/dist/` | delete | Generated copy of `client/dist`. | Keep ignored/generated only; do not treat as source. |
| `desktop/build/` | migrate | Wails packaging assets and generated platform scaffolding. | Move to `desktop/build/`; later delete generated platform files that Wails can regenerate. |
| `desktop/build/windows`, `build/darwin`, `build/linux` | migrate | Desktop packaging assets. | Move with desktop shell. |
| `desktop/build/android`, `build/ios` | archive | Mobile scaffolding is not on the current desktop product path. | Archive or delete after confirming Wails mobile targets are out of scope. |
| `desktop/build/docker` | archive | Cross/server Dockerfiles appear inherited from Wails template or old packaging. | Archive unless current release pipeline needs them. |

## Runtime And Client Product Path

| Path | Label | Reason | Next action |
| --- | --- | --- | --- |
| `client/` | keep | Main React client product path. | Keep; later reorganize by feature. |
| `client/src/runtime/` | keep | TypeScript runtime contracts/adapters. | Keep and align with `internal/runtimeapi`. |
| `client/src/api/chat.ts` | migrate | API client belongs with runtime client or chat feature. | Move during React feature organization. |
| `client/src/components/chat/` | migrate | Product feature UI. | Move to `client/src/features/chat/`. |
| `client/src/components/settings/` | migrate | Product feature UI. | Move to `client/src/features/settings/`. |
| `client/src/hooks/` | migrate | Hooks are mixed app/runtime/feature concerns. | Move runtime subscriptions to runtime/app layer; chat hooks to chat feature. |
| `client/src/assets/react.svg`, `client/src/assets/vite.svg` | delete | Vite starter assets, not product assets. | Delete after confirming unused. |
| `client/src/assets/hero.png` | keep | Product visual asset if referenced by current UI. | Keep while referenced. |
| `client/server/deepseek-proxy.mjs` | archive | Demo/local proxy for DeepSeek, not the desktop runtime path. | Archive or move to `docs/examples/` if still useful. |
| `client/server/deepseek.config.example.json` | archive | Example config for the demo proxy. | Archive with the proxy. |
| `internal/runtimeapi/` | keep | Existing runtime contract package and tests. | Keep; use as anchor while extracting `internal/runtime`. |
| `internal/db/` | keep | SQLite persistence remains on product path; includes runtime audit migration. | Keep. |
| `internal/session/`, `internal/message/`, `internal/permission/` | keep | Core runtime domain packages. | Keep and connect to `internal/runtime`. |
| `internal/agent/`, `internal/agent/tools/` | keep | Core agent loop and built-in tools. | Keep; later split tools into `internal/tools` only when scheduler work begins. |
| `internal/config/`, `internal/hooks/`, `internal/skills/`, `internal/lsp/` | keep | Product path services still needed by runtime and agent behavior. | Keep. |
| `internal/workspace/`, `internal/projects/`, `internal/filetracker/` | keep | Workspace/session supporting services. | Keep; review adapter assumptions later. |
| `internal/env/`, `internal/home/`, `internal/osprocess/`, `internal/fsext/`, `internal/filepathext/`, `internal/dns/` | migrate | Platform utility packages. | Move under `internal/platform/` in a later low-risk pass. |

## CLI/TUI And Legacy Crush Surface

| Path | Label | Reason | Next action |
| --- | --- | --- | --- |
| `internal/ui/` | legacy | Bubble Tea TUI implementation with chat, dialogs, diff view, notifications, key handling, and terminal rendering. Not part of the client runtime path. | Keep untouched until a TUI adapter boundary exists. |
| `internal/ui/AGENTS.md` | keep | Required instructions when working inside TUI code. | Keep with `internal/ui`. |
| `internal/ui/model/` | legacy | Main Bubble Tea model and event handling. | Keep as TUI adapter candidate. |
| `internal/ui/chat/`, `internal/ui/dialog/`, `internal/ui/completions/`, `internal/ui/attachments/` | legacy | Terminal-specific UI features. | Keep as legacy TUI modules. |
| `internal/ui/diffview/` | legacy | TUI diff renderer and large golden corpus. | Keep while CLI/TUI tests exist; do not delete golden files without test migration. |
| `internal/ui/notification/` | legacy | Desktop/native notification code tied to TUI session flow. | Keep until runtime notification needs are separated. |
| `internal/ui/styles/`, `internal/ui/logo/`, `internal/ui/list/`, `internal/ui/image/`, `internal/ui/anim/`, `internal/ui/common/`, `internal/ui/util/`, `internal/ui/xchroma/` | legacy | Terminal presentation support packages. | Keep under TUI legacy boundary. |
| `internal/cmd/` | legacy | Cobra CLI commands, including interactive root, run, login/logout, models, schema, server, stats, sessions, projects. | Keep as CLI adapter; prevent client runtime from depending on it. |
| `internal/cmd/stats/` | legacy | Static web assets for CLI stats command. | Keep with CLI stats or archive if stats command is removed. |
| `internal/cmd/gitignore/` | legacy | Embedded gitignore templates for CLI behavior. | Keep while CLI/workspace init uses them. |
| `internal/cmd/clientserverrace/` | legacy | CLI/server race test. | Keep until client/server mode is replaced or retired. |
| `internal/commands/` | legacy | Slash/custom command parser for terminal command UX. | Split command primitives from terminal UI later; keep current package for CLI/TUI. |
| `internal/backend/events.go` | migrate | Currently exposes `pubsub.Event[tea.Msg]` to consumers. This is a runtime path leak. | Replace with runtime-native event stream; keep `tea.Msg` conversion inside TUI adapter. |
| `internal/server/` | legacy | Crush client/server transport and event wrapping around legacy app events. | Keep until local HTTP/SSE runtime adapter supersedes it. |
| `internal/client/` | legacy | Client for Crush local server mode. | Keep with legacy CLI/server path. |
| `internal/proto/`, `internal/swagger/` | legacy | Protocol/OpenAPI surface for legacy server. | Keep until new runtime API replaces or adopts it. |
| `internal/app/` | migrate | Top-level Crush app wiring mixes core services, UI/server assumptions, LSP/MCP/events. | Extract reusable service construction for `internal/runtime`; leave CLI/TUI wiring in adapter. |
| `internal/update/`, `internal/version/` | legacy | CLI release/update/version support. | Keep until Agent Builder release/version ownership is defined. |
| `internal/format/`, `internal/ansiext/`, `internal/stringext/`, `internal/diff/`, `internal/diffdetect/` | keep | Mixed utility packages; some are TUI-related, some tool/runtime-related. | Keep; classify more narrowly only when imports are changed. |

## Docs And Reference Material

| Path | Label | Reason | Next action |
| --- | --- | --- | --- |
| `docs/project-structure-refactor-plan.md` | keep | Source plan for this inventory. | Keep active. |
| `docs/legacy-crush-inventory.md` | keep | This Phase 1 inventory output. | Keep active and update as decisions change. |
| `docs/architecture-decisions.md` | keep | Active architecture decision log. | Keep active. |
| `docs/implementation-roadmap.md` | keep | Active Agent Builder roadmap. | Keep active, update after inventory decisions. |
| `docs/agentic-operations-client.md` | keep | Product/client concept document. | Keep active. |
| `docs/client-architecture-and-core-flow.md` | keep | Active client/runtime architecture documentation. | Keep active. |
| `docs/client-first-runtime-refactor.md` | keep | Active runtime refactor documentation. | Keep active. |
| `docs/client-information-architecture.md` | keep | Active client IA documentation. | Keep active. |
| `docs/client-state-recovery.md` | keep | Active client/runtime recovery notes. | Keep active. |
| `docs/client-ui-plan.md` | keep | Active UI plan. | Keep active; revise paths after desktop flattening. |
| `docs/desktop-runtime-boundary.md` | keep | Active boundary doc for desktop runtime. | Keep active. |
| `docs/desktop-runtime-root-cause-analysis.md` | archive | Historical root cause analysis for a fixed/diagnostic issue. | Move to `docs/archive/` once findings are reflected in active docs. |
| `docs/dev-baseline.md` | keep | Useful validation baseline. | Keep active. |
| `docs/phase-1-acceptance-test.md` | keep | Current desktop smoke/acceptance procedure. | Keep active, update paths after flattening. |
| `docs/phase-1-runtime-baseline.md` | archive | Phase baseline snapshot. | Archive after current migration plan supersedes it. |
| `docs/phase-2-runtime-api-boundary.md` | keep | Active runtime API boundary design. | Keep active. |
| `docs/permission-policy-model.md` | keep | Active design input for PermissionPolicy work. | Keep active. |
| `docs/tool-scheduler-design.md` | keep | Active design input for scheduler work. | Keep active. |
| `docs/turn-task-run-model.md` | keep | Active design input for turn/task model. | Keep active. |
| `docs/crush-claude-code-gap-analysis.md` | archive | Reference comparison, not an active product contract. | Archive under `docs/archive/reference-analysis/` or keep in `docs/reference-analysis/`. |
| `docs/reference-analysis/` | archive | Comparative reference research. | Move under `docs/archive/` or keep as explicitly named reference material. |
| `docs/hooks/` | migrate | User-facing Crush hooks documentation. Hooks are still product-relevant but docs are Crush-branded. | Move/update to `docs/reference/hooks/` or rewrite as Agent Builder hook docs. |

## Config And Generated Artifacts

| Path | Label | Reason | Next action |
| --- | --- | --- | --- |
| `internal/db/sql/` | keep | Source SQL for sqlc generation. | Keep. |
| `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go` | keep | Generated but checked-in DB code used by builds/tests. | Keep; regenerate via `task sqlc` after SQL changes. |
| `internal/swagger/docs.go`, `swagger.json`, `swagger.yaml` | legacy | Generated docs for legacy server API. | Keep while `task swag` and legacy server remain. |
| `internal/agent/testdata/` | keep | Agent tests depend on cassette/testdata files. | Keep. |
| `internal/ui/diffview/testdata/` | legacy | TUI golden tests depend on this corpus. | Keep until TUI tests are retired or moved. |
| `desktop/bin/` | delete | Build output if present. Not source. | Ensure ignored and delete from worktree if generated. |
| `desktop/frontend/dist/` | delete | Generated client build copy. | Ensure ignored and recreate during desktop build. |

## Migration Priority

1. Flatten `desktop` to single-layer `desktop/`.
   Move Wails entry, bridge, build config, scripts, and desktop README first.
   Keep behavior unchanged and still call the existing runtime code.

2. Remove the nested desktop Go module.
   Build desktop from the root `go.mod`, then delete
   `desktop/go.mod`/`desktop/go.sum` only after `go test ./...` and desktop
   build pass.

3. Extract generic `runtime_*` files to `internal/runtime`.
   Start with pure service/types/status/session/audit/model/skills/MCP files.
   Leave `runtime_bridge.go` in `desktop/` as the Wails adapter.

4. Replace `tea.Msg` on the client runtime path.
   The critical files are `desktop/runtime_events.go` and
   `internal/backend/events.go`. Introduce runtime-native events, then keep
   Bubble Tea conversion only in `internal/ui` or a future
   `internal/adapters/tui` package.

5. Move HTTP/SSE surfaces into an adapter boundary.
   Decide whether `runtime_http.go` and `runtime_sse.go` belong in
   `internal/adapters/http` or remain thin wrappers around `internal/runtime`.

6. Establish Tool Scheduler and PermissionPolicy.
   Extract lifecycle/policy decisions from runtime bridge code into core
   runtime/tool packages before deeper UI cleanup.

7. Reorganize React by feature.
   Move chat, sessions, permissions, settings, capabilities, skills, MCP, and
   audit into `client/src/features/`, keeping TypeScript runtime contracts in
   `client/src/runtime/`.

8. Archive or delete confirmed legacy material.
   Only archive/delete files that are already marked here and are no longer
   referenced by build, tests, docs, or active release tasks.

## Verification For This Phase

No code files were moved, deleted, or renamed for this inventory phase. The
only intended file change is:

```text
docs/legacy-crush-inventory.md
```
