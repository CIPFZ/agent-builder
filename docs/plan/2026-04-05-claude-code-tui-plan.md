# Go Claude Code TUI Implementation Plan

**Goal:** deliver a usable Go TUI that feels like a minimal Claude Code session before pursuing deeper daemon/control-plane work.

**Scope for this phase:**
- single terminal TUI entry in `myclaw`
- session transcript view
- input box for user prompts
- visible runtime events for tool call/result
- inline approval interaction for blocked tool runs
- reuse existing `runtime.Runner` instead of inventing a new backend

**Explicitly out of scope for this phase:**
- openclaw-style remote control workflows
- richer orchestration dashboards
- deeper autoDream/advanced compaction work
- worktree/remote-isolated subagents

## Target user experience

When the user runs `go run ./cmd/myclaw tui`, they should get a full-screen terminal app with:

1. a scrollable transcript pane
2. a small event pane for tool/approval status
3. a text input area
4. an approval prompt area when runtime emits `permission.required`

The shortest happy path should be:

1. launch `myclaw tui`
2. type a prompt
3. see assistant text stream into the transcript
4. if a tool call happens, see it in the event area
5. if approval is required, choose approve/reject inside the TUI

## Implementation slices

### Slice 1: TUI shell
- add a `tui` CLI subcommand
- add a Bubble Tea based full-screen model
- render transcript, events, and input
- support quitting cleanly with `ctrl+c`

### Slice 2: Runtime bridge
- create a small adapter that owns a session manager and runtime runner
- send user messages into the runner
- translate runtime events into TUI messages
- append assistant/user/tool output into transcript state

### Slice 3: Approval interaction
- when `permission.required` arrives, show an approval banner/modal
- allow `a` to approve and `r` to reject the current pending approval
- continue blocked execution on approval through existing runtime path

### Slice 4: Streaming polish
- surface `assistant.delta` progressively in transcript
- separate tool and lifecycle messages into the event pane
- show busy/idle state in the footer

## Files expected to change

- `C:\Users\ytq\work\ai\myclaw\myclaw\go.mod`
- `C:\Users\ytq\work\ai\myclaw\myclaw\internal\app\cli.go`
- `C:\Users\ytq\work\ai\myclaw\myclaw\internal\app\cli_test.go`
- `C:\Users\ytq\work\ai\myclaw\myclaw\internal\tui\model.go`
- `C:\Users\ytq\work\ai\myclaw\myclaw\internal\tui\model_test.go`
- `C:\Users\ytq\work\ai\myclaw\myclaw\internal\tui\runtime_bridge.go`
- `C:\Users\ytq\work\ai\myclaw\myclaw\internal\tui\runtime_bridge_test.go`

## Verification target

This phase is done when all of the following are true:

- `go run ./cmd/myclaw tui` opens a terminal UI
- entering text triggers an assistant response
- tool calls are visible in the UI
- approval-required flows can be approved/rejected from the UI
- `go test ./... -count=1` passes
