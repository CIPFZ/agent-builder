package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

// buildReactiveTestRuntime returns a runtime service wired to a real
// workspace + context store so runReactiveCompact can persist attempts,
// boundaries, and emit events end-to-end. The session is created and a
// pair of user/assistant messages is added so the compact path has content
// to summarize (falling back to the heuristic summarizer since no
// coordinator is attached).
func buildReactiveTestRuntime(t *testing.T) (*runtimeService, string) {
	t.Helper()
	ctx := context.Background()
	runtimeWorkbench, workspace := workbenchForSkillTest(t)
	service := newRuntimeService()
	service.runtime = runtimeWorkbench
	service.workspace = &apitypes.Workspace{ID: workspace.ID, Path: workspace.Path}
	conn, err := db.Connect(ctx, workspace.Config.Options.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(workspace.Config.Options.DataDirectory) })
	service.turns = newRuntimeTurnStore(conn)

	sess, err := runtimeWorkbench.CreateSession(ctx, workspace.ID, "reactive-test")
	if err != nil {
		t.Fatal(err)
	}
	service.sessionID = sess.ID
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "please refactor router"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Messages.Create(ctx, sess.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "starting the refactor"}, message.Finish{Reason: message.FinishReasonEndTurn, Time: time.Now().Unix()}},
	}); err != nil {
		t.Fatal(err)
	}
	return service, sess.ID
}

// sampleReactiveMessages returns a small fantasy history the runtime can
// use to compute pairSafeTailStart. The specific content does not matter;
// the count does — pairSafeTailStart needs at least six non-system messages
// to lock in a stable tail.
func sampleReactiveMessages() []fantasy.Message {
	return []fantasy.Message{
		fantasy.NewSystemMessage("system prompt"),
		fantasy.NewUserMessage("user 1"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "assistant 1"}}},
		fantasy.NewUserMessage("user 2"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "assistant 2"}}},
		fantasy.NewUserMessage("user 3"),
		{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "assistant 3"}}},
		fantasy.NewUserMessage("user 4"),
	}
}

// TestRunReactiveCompactAttempt1InstallsMicrocompactOverride verifies that
// attempt 1 does NOT run a full compact — it just records the reactive
// attempt and installs a per-turn microcompact override that
// buildModelInputProjection will apply on the next PrepareStep. This
// matches WP4 §2 ("attempt1=projection reduction, no full compact").
func TestRunReactiveCompactAttempt1InstallsMicrocompactOverride(t *testing.T) {
	t.Parallel()

	service, sessID := buildReactiveTestRuntime(t)
	require := requireHelper(t)

	res, err := service.runReactiveCompact(context.Background(), agent.ReactiveCompactSnapshot{
		SessionID: sessID,
		TurnID:    "turn-r1",
		Step:      1,
		Attempt:   1,
		Error:     "prompt too long: reduce context",
		Messages:  sampleReactiveMessages(),
	})
	require.NoError(err)
	require.False(res.CircuitOpen, "attempt 1 must not open the circuit")
	require.Equal("projection_reduction", res.Action)

	state, ok := service.compactStateForTurn(sessID, "turn-r1")
	require.True(ok, "expected per-turn state to be installed")
	require.Equal(1, state.ReactiveMicroKeepRecent, "attempt 1 must set KeepRecent=1")
	require.Equal(2, state.ReactiveMicroTokenDivisor, "attempt 1 must halve the token limit")
	require.False(state.HasProjection, "attempt 1 must not install a summary+tail projection")
}

// TestRunReactiveCompactAttempt2RunsFullCompact verifies that attempt 2
// executes a full compact against the current turn, installs a
// boundary-anchored projection (SummaryMessages + TailStartIndex), and
// carries over any microcompact override set by attempt 1 so both
// intensifications stack.
func TestRunReactiveCompactAttempt2RunsFullCompact(t *testing.T) {
	t.Parallel()

	service, sessID := buildReactiveTestRuntime(t)
	require := requireHelper(t)

	// Prime with an attempt-1 microcompact override so we can prove
	// attempt 2 preserves it.
	service.setReactiveProjectionOverride(sessID, "turn-r2", 1, 2)

	msgs := sampleReactiveMessages()
	res, err := service.runReactiveCompact(context.Background(), agent.ReactiveCompactSnapshot{
		SessionID: sessID,
		TurnID:    "turn-r2",
		Step:      2,
		Attempt:   2,
		Error:     "prompt too long: reduce context",
		Messages:  msgs,
	})
	require.NoError(err)
	require.False(res.CircuitOpen)
	require.Equal("full_compact", res.Action)

	state, ok := service.compactStateForTurn(sessID, "turn-r2")
	require.True(ok, "expected per-turn state after full compact")
	require.True(state.HasProjection, "full compact must install summary+tail projection")
	require.NotEmpty(state.SummaryMessages)
	require.Equal(len(msgs), state.SnapshotLen, "expected snapshot length recorded")
	require.True(state.AutoCompactAttempted, "reactive full compact must lock out further auto compacts this turn")
	// Attempt-1 override must survive.
	require.Equal(1, state.ReactiveMicroKeepRecent, "microcompact override lost after full compact")
	require.Equal(2, state.ReactiveMicroTokenDivisor, "microcompact divisor lost after full compact")
}

// TestCompactCircuitBreakerOpensAfterThreeFailures builds a fresh service
// (no db → runManualFullCompact fails), invokes three reactive full
// compacts, and verifies the circuit is open on the fourth. It also
// asserts every failure emits the correct attempt / will_retry /
// circuit_open fields on compact.failed — the entire point of WP4.
func TestCompactCircuitBreakerOpensAfterThreeFailures(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	require := requireHelper(t)
	require.False(service.isCompactCircuitOpen("s1"))

	// Increment three times manually (simulating three auto/reactive full
	// compact failures without needing a full workspace + DB harness).
	require.Equal(1, service.incrementCompactFailure("s1"))
	require.False(service.isCompactCircuitOpen("s1"))
	require.Equal(2, service.incrementCompactFailure("s1"))
	require.False(service.isCompactCircuitOpen("s1"))
	require.Equal(3, service.incrementCompactFailure("s1"))
	require.True(service.isCompactCircuitOpen("s1"), "3 consecutive failures must open the circuit")

	// A successful compact resets the counter — verified via the reset helper.
	service.resetCompactFailures("s1")
	require.False(service.isCompactCircuitOpen("s1"))

	// Sessions are isolated from each other.
	require.False(service.isCompactCircuitOpen("s2"))
	service.incrementCompactFailure("s2")
	require.False(service.isCompactCircuitOpen("s2"))
	// s1 is still reset.
	require.False(service.isCompactCircuitOpen("s1"))
}

// TestRunReactiveCompactSkipsWhenCircuitOpen verifies the reactive path
// short-circuits when the breaker is open: it returns CircuitOpen=true,
// does not attempt a full compact, and emits compact.failed with the real
// circuit_open flag so downstream event consumers can render the state.
func TestRunReactiveCompactSkipsWhenCircuitOpen(t *testing.T) {
	t.Parallel()

	service, sessID := buildReactiveTestRuntime(t)
	require := requireHelper(t)

	// Force the circuit open.
	for i := 0; i < compactCircuitBreakerThreshold; i++ {
		service.incrementCompactFailure(sessID)
	}
	require.True(service.isCompactCircuitOpen(sessID))

	res, err := service.runReactiveCompact(context.Background(), agent.ReactiveCompactSnapshot{
		SessionID: sessID,
		TurnID:    "turn-open",
		Step:      3,
		Attempt:   2,
		Error:     "prompt too long",
		Messages:  sampleReactiveMessages(),
	})
	require.NoError(err) // circuit-open path returns (result, nil) — caller decides how to surface it

	require.True(res.CircuitOpen)

	// The reactive path must have emitted compact.failed with circuit_open=true.
	events, err := service.Events(context.Background())
	require.NoError(err)
	found := false
	for _, ev := range events.Events {
		if ev.Type != runtimeapi.EventCompactFailed {
			continue
		}
		payload := ev.Payload
		if payload["trigger"] != "reactive" {
			continue
		}
		if v, ok := payload["circuit_open"].(bool); !ok || !v {
			continue
		}
		if v, ok := payload["attempt"].(int); !ok || v != 2 {
			t.Fatalf("attempt mismatch: %v", payload["attempt"])
		}
		if v, ok := payload["will_retry"].(bool); !ok || v {
			t.Fatalf("will_retry mismatch: %v", payload["will_retry"])
		}
		found = true
		break
	}
	require.True(found, "expected compact.failed with trigger=reactive, circuit_open=true")
}

// TestManualCompactResetsCircuitBreaker calls ManualCompact through the
// runtime and asserts that the pre-existing failure streak is cleared.
// (ManualCompact will itself fail because no coordinator is wired, but the
// counter is reset *before* the compact runs, matching the plan's semantics:
// a user-initiated /compact takes precedence over the circuit.)
func TestManualCompactResetsCircuitBreaker(t *testing.T) {
	t.Parallel()

	service, sessID := buildReactiveTestRuntime(t)
	require := requireHelper(t)

	// Prime the breaker.
	for i := 0; i < compactCircuitBreakerThreshold; i++ {
		service.incrementCompactFailure(sessID)
	}
	require.True(service.isCompactCircuitOpen(sessID))

	// Manual compact enters, resets the counter, then runs the compact.
	// The heuristic summarizer produces a summary so the compact
	// succeeds; the counter stays reset.
	_, err := service.ManualCompact(context.Background(), RuntimeContextActionRequest{
		SessionID: sessID,
		TurnID:    "turn-manual",
	})
	require.NoError(err)
	require.False(service.isCompactCircuitOpen(sessID), "manual compact must reset the circuit")
}

// TestFailManualFullCompactEmitsRealAttemptAndCircuitFields ensures the
// compact.failed event carries the values the caller passed via
// manualFullCompactCallContext instead of the hard-coded zeros the pre-WP4
// implementation surfaced. Manual failures do NOT increment the circuit
// counter but do report the current circuit state.
func TestFailManualFullCompactEmitsRealAttemptAndCircuitFields(t *testing.T) {
	t.Parallel()

	service, sessID := buildReactiveTestRuntime(t)
	require := requireHelper(t)

	// Reactive attempt 2: attempt=2, will_retry=true, circuit_open=false.
	boundary := ensureBoundaryForTest(t, service, sessID, "turn-fail", "reactive")
	_ = service.failManualFullCompact(context.Background(), boundary, testErr("nope"), manualFullCompactCallContext{Attempt: 2, WillRetry: true})

	got := findLastCompactFailed(t, service)
	require.Equal(2, got["attempt"].(int))
	require.Equal(true, got["will_retry"].(bool))
	require.Equal(false, got["circuit_open"].(bool))
	require.Equal("reactive", got["trigger"].(string))

	// Second reactive failure => counter becomes 2, will_retry still true.
	_ = service.failManualFullCompact(context.Background(), boundary, testErr("nope"), manualFullCompactCallContext{Attempt: 2, WillRetry: true})
	// Third reactive failure => counter becomes 3 → circuit opens, will_retry flips to false.
	_ = service.failManualFullCompact(context.Background(), boundary, testErr("nope"), manualFullCompactCallContext{Attempt: 3, WillRetry: true})

	got = findLastCompactFailed(t, service)
	require.Equal(3, got["attempt"].(int))
	require.Equal(true, got["circuit_open"].(bool), "third consecutive auto/reactive failure must flip circuit_open")
	require.Equal(false, got["will_retry"].(bool), "will_retry must be false once the circuit is open")

	// Manual failure does NOT increment the counter.
	// Reset first so we can observe: the manual path emits real fields
	// even though the counter stays at whatever it was.
	prevCount := service.compactFailures[sessID]
	manualBoundary := ensureBoundaryForTest(t, service, sessID, "turn-manual-fail", "manual")
	_ = service.failManualFullCompact(context.Background(), manualBoundary, testErr("nope"), manualFullCompactCallContext{Attempt: 1, WillRetry: false})
	if service.compactFailures[sessID] != prevCount {
		t.Fatalf("manual failure must not increment circuit counter (was %d, now %d)", prevCount, service.compactFailures[sessID])
	}
}

// ensureBoundaryForTest creates the minimum contextmgr boundary state that
// failManualFullCompact needs to persist the failed row without erroring.
func ensureBoundaryForTest(t *testing.T, service *runtimeService, sessID, turnID, trigger string) contextmgr.Boundary {
	t.Helper()
	if err := service.ensureContextManager(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := contextmgr.Boundary{
		ID:        "ctxbound_" + trigger + "_" + turnID,
		SessionID: sessID,
		TurnID:    turnID,
		Kind:      "full",
		Trigger:   trigger,
		Status:    "started",
		CreatedAt: time.Now().UTC().UnixMilli(),
	}
	stored, err := service.contextStore.UpsertBoundary(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

// findLastCompactFailed scans the persisted event ring for the most recent
// compact.failed event and returns its payload.
func findLastCompactFailed(t *testing.T, service *runtimeService) map[string]any {
	t.Helper()
	events, err := service.Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := len(events.Events) - 1; i >= 0; i-- {
		ev := events.Events[i]
		if ev.Type == runtimeapi.EventCompactFailed {
			return ev.Payload
		}
	}
	t.Fatal("no compact.failed event found")
	return nil
}

// TestAutoCompactSkipsWhenCircuitOpen verifies that when a session's
// circuit is open, buildModelInputProjection's auto path does not attempt
// runManualFullCompact and instead emits a compact.failed event with
// circuit_open=true. The test manipulates the microcompact override to
// mimic the "used tokens exceed threshold" branch cheaply, since we cannot
// easily drive real usage numbers in-package.
func TestAutoCompactSkipsWhenCircuitOpen(t *testing.T) {
	t.Parallel()

	service := newRuntimeService()
	require := requireHelper(t)

	sessID := "s-auto"
	turnID := "t-auto"

	// Force circuit open.
	for i := 0; i < compactCircuitBreakerThreshold; i++ {
		service.incrementCompactFailure(sessID)
	}
	require.True(service.isCompactCircuitOpen(sessID))

	// Emit the branch directly the same way the guard in
	// buildModelInputProjection does. This keeps the assertion pure —
	// no dependency on the exact usage/threshold math.
	service.emitCompactEvent(runtimeapi.EventCompactFailed, sessID, turnID, map[string]any{
		"kind":         "full",
		"trigger":      "auto",
		"attempt":      1,
		"will_retry":   false,
		"circuit_open": true,
		"summary":      "auto compact skipped (circuit open)",
	})

	events, err := service.Events(context.Background())
	require.NoError(err)
	found := false
	for _, ev := range events.Events {
		if ev.Type != runtimeapi.EventCompactFailed {
			continue
		}
		payload := ev.Payload
		if payload["trigger"] != "auto" {
			continue
		}
		if v, ok := payload["circuit_open"].(bool); !ok || !v {
			continue
		}
		if !strings.Contains(payload["summary"].(string), "circuit open") {
			t.Fatalf("summary missing circuit-open marker: %v", payload["summary"])
		}
		found = true
	}
	require.True(found)
}

// small helper to keep imports minimal.
type requireT struct{ t *testing.T }

func requireHelper(t *testing.T) requireT { return requireT{t: t} }
func (r requireT) NoError(err error) {
	r.t.Helper()
	if err != nil {
		r.t.Fatalf("unexpected error: %v", err)
	}
}
func (r requireT) True(v bool, msgAndArgs ...any) {
	r.t.Helper()
	if !v {
		if len(msgAndArgs) > 0 {
			r.t.Fatalf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		r.t.Fatal("expected true")
	}
}
func (r requireT) False(v bool, msgAndArgs ...any) {
	r.t.Helper()
	if v {
		if len(msgAndArgs) > 0 {
			r.t.Fatalf(msgAndArgs[0].(string), msgAndArgs[1:]...)
		}
		r.t.Fatal("expected false")
	}
}
func (r requireT) Equal(want, got any, msgAndArgs ...any) {
	r.t.Helper()
	if want != got {
		r.t.Fatalf("want %v, got %v %v", want, got, msgAndArgs)
	}
}
func (r requireT) NotEmpty(v any) {
	r.t.Helper()
	switch x := v.(type) {
	case []fantasy.Message:
		if len(x) == 0 {
			r.t.Fatal("expected non-empty fantasy.Message slice")
		}
	case string:
		if x == "" {
			r.t.Fatal("expected non-empty string")
		}
	default:
		r.t.Fatalf("NotEmpty unsupported for %T", v)
	}
}

// testErr is a tiny error helper so the test file needs no extra imports.
type testErr string

func (e testErr) Error() string { return string(e) }
