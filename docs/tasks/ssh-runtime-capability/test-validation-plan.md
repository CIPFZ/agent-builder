# SSH Runtime Capability Test And Validation Plan

Date: 2026-04-22

## 1. Validation Goal

Prove that SSH is not only implemented, but implemented as a reliable runtime capability with correct tool semantics, approval behavior, event propagation, and operator-visible output.

## 2. Test Strategy

Use four validation layers:

1. parser and schema tests
2. executor and tool unit tests
3. query/runtime integration tests
4. controlled functional verification

The implementation is not accepted if it only passes unit tests but fails the end-to-end runtime flow.

## 3. Unit Tests

Primary test file:

- `internal/tools/system/ssh_test.go`

## 3.1 Input Parsing

Required cases:

- JSON input with required fields only
- JSON input with optional fields
- plain string or malformed input rejection
- timeout conversion from milliseconds
- whitespace trimming on `host`, `user`, `command`, and `workdir`

Expected assertions:

- required fields are preserved exactly
- absent optional fields do not produce invalid default values
- invalid input returns an actionable error

## 3.2 Definition And Inspection

Required cases:

- `Definition().Name == "SSH"`
- schema contains the required fields
- `IsReadOnly(...) == false`
- `IsDestructive(...) == true`

Expected assertions:

- SSH is always treated as high-sensitivity in v1

## 3.3 Command Construction

Required cases:

- host only
- host + user
- host + user + port
- host + identity file
- host + workdir wrapping

Expected assertions:

- generated process args are stable
- host target formatting is correct
- `workdir` wrapping does not corrupt the original command

## 3.4 Timeout And Result Shaping

Required cases:

- successful execution
- non-zero exit
- timeout
- stderr-only failure

Expected assertions:

- stdout and stderr remain separated internally
- timeout marks `TimedOut`
- result captures `ExitCode`
- final output text and structured content are both populated correctly

## 3.5 Progress Emission

Required cases:

- success path
- failure path
- timeout path

Expected assertions:

- progress sequence contains expected SSH states
- terminal state is emitted exactly once
- progress data includes host and command context

## 4. Integration Tests

Primary packages:

- `internal/queryengine`
- `internal/runtime`
- `internal/gateway`

## 4.1 Registry And Runtime Assembly

Required cases:

- default runtime includes `SSH`
- registry search can find `SSH`
- runtime tool exposure reports correct metadata

## 4.2 Permission And Approval Flow

Required cases:

- policy denies SSH and approval is required
- approval record preserves structured SSH input
- rejection path surfaces correct denial reason
- approval acceptance resumes tool execution correctly

Expected assertions:

- `permission.required` contains `tool_input_object`
- host and command survive the approval round-trip

## 4.3 Query Engine Transcript Flow

Required cases:

- `tool.called` emitted for SSH
- transcript contains tool-use block with structured input
- transcript contains tool-result block after completion

Expected assertions:

- SSH fits the standard tool lifecycle
- no SSH-specific transcript workaround is required

## 4.4 Gateway Event Flow

Required cases:

- gateway forwards `tool.called`
- gateway forwards SSH progress
- gateway forwards `tool.result`
- gateway forwards `permission.required`

Expected assertions:

- current clients can observe SSH activity without any SSH-only side channel

## 5. Test Doubles And Harness Guidance

Do not rely only on live SSH in automated tests.

Recommended test seams:

- fake SSH executor behind interface
- fake progress collector
- fake event sink for query/runtime/gateway tests

Reason:

- live SSH introduces environment instability
- most semantic verification should be deterministic

## 6. Functional Validation

Use a controlled host, preferably a disposable local VM/container or a dedicated test host with known credentials.

## 6.1 Minimum Functional Scenario

1. invoke `SSH` with a simple remote command such as `pwd`
2. verify approval requirement is triggered
3. approve the request
4. verify tool progress is visible
5. verify stdout, stderr, exit code, and duration are surfaced

## 6.2 Required Functional Scenarios

Scenario A:

- command: `pwd`
- purpose: basic success path

Scenario B:

- command: `cd /tmp && ls`
- purpose: remote workdir handling

Scenario C:

- command: a non-existent command
- purpose: non-zero exit and stderr behavior

Scenario D:

- command: a deliberate long-running command with short timeout
- purpose: timeout handling

## 6.3 Gateway Verification

During at least one functional run, verify:

- websocket client receives `tool.called`
- websocket client receives SSH progress payloads
- websocket client receives final `tool.result`

## 7. Exit Criteria

The SSH module passes validation only if all of the following are true:

- unit tests pass
- integration tests pass
- functional success path passes
- functional failure path passes
- approval and gateway payloads preserve structured SSH context

## 8. Non-Acceptable Shortcuts

The module is not considered validated if:

- only happy-path unit tests exist
- live-manual testing is used without deterministic unit coverage
- approval flow is not tested
- gateway forwarding is assumed rather than verified
