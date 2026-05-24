# Development Baseline

Status: historical local baseline. This file records the 2026-05-16 pre-refactor
environment and should not be used as the current build/test status. For
current verification, run the commands in `AGENTS.md` and inspect the latest
implementation summary or CI results.

Baseline date: 2026-05-16.

This document records the current local build, run, and test status before
starting runtime or client changes.

## Environment

Workspace:

```text
C:\Users\ytq\work\ai\crush
```

Git:

```text
branch: main
commit: fe478134
```

Go:

```text
go version go1.26.3 windows/amd64
GOVERSION=go1.26.3
GOTOOLCHAIN=go1.26.3+auto
GOPROXY=https://proxy.golang.org,direct
GOOS=windows
GOARCH=amd64
CGO_ENABLED=0
GOEXPERIMENT=
GOMOD=C:\Users\ytq\work\ai\crush\go.mod
```

Shell:

```text
PowerShell 5.1.19041.6456
```

Notes:

- The machine had Go 1.25.6 installed under `C:\Program Files\Go\bin\go.exe`.
- The project requires Go 1.26.3 in `go.mod`.
- User-level Go toolchain config was updated to:

```text
GOTOOLCHAIN=go1.26.3+auto
GOPROXY=https://proxy.golang.org,direct
```

This lets Go use the required 1.26.3 toolchain without replacing the base Go
installation.

## Build

Command:

```powershell
go build .
```

Result:

```text
PASS
```

Duration:

```text
35.6s
```

## CLI Smoke Checks

Command:

```powershell
go run . --help
```

Result:

```text
PASS
```

The command printed the root help, including commands such as `run`, `server`,
`models`, `session`, `logs`, `dirs`, `login`, and `update-providers`.

Command:

```powershell
go run . dirs
```

Result:

```text
PASS
```

Output:

```text
C:\Users\ytq\.config\crush
C:\Users\ytq\AppData\Local\crush
C:\Users\ytq\work\ai\crush
```

Command:

```powershell
go run . models --help
```

Result:

```text
PASS
```

## Focused Tests

Command:

```powershell
go test ./internal/config ./internal/hooks ./internal/permission ./internal/skills ./internal/session
```

Result:

```text
PASS
```

Packages:

```text
ok   github.com/charmbracelet/crush/internal/config     7.403s
ok   github.com/charmbracelet/crush/internal/hooks      3.497s
ok   github.com/charmbracelet/crush/internal/permission 0.983s
ok   github.com/charmbracelet/crush/internal/skills     1.146s
?    github.com/charmbracelet/crush/internal/session    [no test files]
```

Command:

```powershell
go test ./internal/agent/tools
```

Result:

```text
FAIL
```

Failing tests:

```text
TestBashTool_CustomAutoBackgroundThreshold
TestBackgroundShell_ConcurrentAccess
TestBackgroundShell_AutoBackground/long_command_stays_in_background
```

Observed errors:

```text
Should be true
exit status 127
```

## Full Test Suite

Command:

```powershell
go test ./...
```

Result:

```text
FAIL
```

Failing packages:

```text
github.com/charmbracelet/crush/internal/agent/tools
github.com/charmbracelet/crush/internal/shell
```

Failing tests in `internal/agent/tools`:

```text
TestBashTool_CustomAutoBackgroundThreshold
TestBackgroundShell_ConcurrentAccess
TestBackgroundShell_AutoBackground/long_command_stays_in_background
```

Failing tests in `internal/shell`:

```text
TestDispatch_BashShebang
TestDispatch_ShebangPassesExitCode
```

Focused reproduction:

```powershell
go test ./internal/shell -run TestDispatch_BashShebang -count=1 -v
```

Output:

```text
=== RUN   TestDispatch_BashShebang
    dispatch_test.go:328: Run returned error: exit status 1 (stderr="<3>WSL (26566 - Relay) ERROR: CreateProcessCommon:818: execvpe(/bin/bash) failed: No such file or directory\n")
--- FAIL: TestDispatch_BashShebang (0.13s)
FAIL
```

## Baseline Interpretation

The project builds successfully on this machine with Go 1.26.3.

Basic CLI commands work:

- `go run . --help`
- `go run . dirs`
- `go run . models --help`

The full test suite does not currently pass on this Windows environment. The
observed failures are concentrated in shell/bash/background-job tests and appear
related to missing WSL `/bin/bash` support:

```text
execvpe(/bin/bash) failed: No such file or directory
exit status 127
```

This is a baseline environment issue, not a result of runtime/client changes.

## Known Baseline Issues

- Windows shell tests expect `/bin/bash` through WSL, but this machine's WSL
  environment does not provide `/bin/bash`.
- `internal/agent/tools` background shell tests fail with exit status 127.
- `go test ./...` should be treated as blocked until the shell environment is
  fixed or tests are adjusted/skipped for this Windows setup.

## Recommended Next Step

Historical recommendation from the baseline date:

1. Keep this baseline as the pre-change reference.
2. Create the first `client/` React prototype for the SSH troubleshooting
   assistant using mock events.
3. Separately decide whether to install/fix WSL bash or document Windows shell
   test limitations before relying on `go test ./...` as a required gate.

Current direction has superseded this: the product path is the Go runtime plus
React/Wails client, with compact/tool-budget/policy/task governance as the next
runtime roadmap.
