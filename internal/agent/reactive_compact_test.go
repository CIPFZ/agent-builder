package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/stretchr/testify/require"
)

// scriptedReactiveModel scripts one Stream call per attempt: attempts before
// the success point return a "prompt too long" error the agent recognises as
// context-length; the success point returns a normal short text response.
type scriptedReactiveModel struct {
	mu           sync.Mutex
	failsRemain  int
	failMessage  string
	finalContent string
	calls        int
	callsCap     int // 0 = no cap
}

func (m *scriptedReactiveModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, fmt.Errorf("Generate is not implemented")
}
func (m *scriptedReactiveModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.mu.Lock()
	m.calls++
	call_i := m.calls
	remain := m.failsRemain
	failMsg := m.failMessage
	final := m.finalContent
	cap_i := m.callsCap
	m.mu.Unlock()
	if cap_i > 0 && call_i > cap_i {
		return nil, errors.New("scripted model exceeded call cap")
	}
	if remain > 0 && call_i <= remain {
		if failMsg == "" {
			failMsg = "prompt too long: history exceeds context window"
		}
		return nil, errors.New(failMsg)
	}
	// success: emit a small text finish
	return func(yield func(fantasy.StreamPart) bool) {
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t1"}) {
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "t1", Delta: final}) {
			return
		}
		if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "t1"}) {
			return
		}
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	}, nil
}
func (m *scriptedReactiveModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, fmt.Errorf("GenerateObject is not implemented")
}
func (m *scriptedReactiveModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, fmt.Errorf("StreamObject is not implemented")
}
func (m *scriptedReactiveModel) Provider() string { return "test" }
func (m *scriptedReactiveModel) Model() string    { return "reactive-test-model" }

// callCount returns how many Stream invocations landed on the model so tests
// can assert on retry behaviour.
func (m *scriptedReactiveModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type reactiveCallRecord struct {
	Attempt  int
	Error    string
	Messages int
}

// mockReactiveCompactor records every ReactiveCompact invocation and, per
// test, may fail, mark circuit open, or simply succeed. It is deliberately
// small: WP4's runtime side is exercised in the runtime package tests; here
// we only verify the agent loop's behaviour around the callback contract.
type mockReactiveCompactor struct {
	mu                   sync.Mutex
	Calls                []reactiveCallRecord
	ErrOnAttempt         map[int]error
	CircuitOpenOnAttempt map[int]bool
}

func (c *mockReactiveCompactor) ReactiveCompact(ctx context.Context, snap ReactiveCompactSnapshot) (ReactiveCompactResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = append(c.Calls, reactiveCallRecord{
		Attempt:  snap.Attempt,
		Error:    snap.Error,
		Messages: len(snap.Messages),
	})
	if err, ok := c.ErrOnAttempt[snap.Attempt]; ok {
		return ReactiveCompactResult{}, err
	}
	if c.CircuitOpenOnAttempt[snap.Attempt] {
		return ReactiveCompactResult{CircuitOpen: true}, nil
	}
	action := "projection_reduction"
	if snap.Attempt >= 2 {
		action = "full_compact"
	}
	return ReactiveCompactResult{Action: action}, nil
}

func (c *mockReactiveCompactor) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Calls)
}

// newReactiveAgentTest builds a sessionAgent wired with a scripted model and
// a mockReactiveCompactor, plus a starter user message.
func newReactiveAgentTest(t *testing.T, model *scriptedReactiveModel, compactor *mockReactiveCompactor) (*sessionAgent, string) {
	t.Helper()
	env := testEnv(t)
	a := testSessionAgent(env, model, model, "test prompt").(*sessionAgent)
	a.reactiveCompactor = compactor

	sess, err := env.sessions.Create(t.Context(), "reactive-test")
	require.NoError(t, err)
	_, err = env.messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "please summarize"}},
	})
	require.NoError(t, err)
	return a, sess.ID
}

// TestReactiveRetryAttempt1SuccessAfterProjectionReduction verifies the
// simplest recovery path: the first Stream call fails with a context-length
// error, the reactive callback runs once (attempt=1, projection reduction),
// and the second Stream call succeeds. The agent must return the final
// success result and not surface the initial PTL error.
func TestReactiveRetryAttempt1SuccessAfterProjectionReduction(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain:  1,
		failMessage:  "prompt too long: try again",
		finalContent: "ok after attempt-1 reduction",
	}
	compactor := &mockReactiveCompactor{}
	a, sessID := newReactiveAgentTest(t, model, compactor)

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.NoError(t, err)

	require.Equal(t, 2, model.callCount(), "expected retry after PTL")
	require.Equal(t, 1, compactor.callCount(), "expected one reactive attempt")
	require.Equal(t, 1, compactor.Calls[0].Attempt)
	require.Contains(t, compactor.Calls[0].Error, "prompt too long")
}

// TestReactiveRetryAttempt2SuccessAfterFullCompact asserts that when
// attempt 1 does not fix the PTL (the mock still fails on the second call),
// the agent invokes the callback again with attempt=2 (full compact) and
// stops after that succeeds. It also asserts the attempt number order
// (1, 2) which anchors the runtime's projection-reduction-then-full-compact
// contract.
func TestReactiveRetryAttempt2SuccessAfterFullCompact(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain:  2,
		failMessage:  "context length exceeded",
		finalContent: "ok after full compact",
	}
	compactor := &mockReactiveCompactor{}
	a, sessID := newReactiveAgentTest(t, model, compactor)

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.NoError(t, err)

	require.Equal(t, 3, model.callCount())
	require.Equal(t, 2, compactor.callCount())
	require.Equal(t, 1, compactor.Calls[0].Attempt)
	require.Equal(t, 2, compactor.Calls[1].Attempt)
}

// TestReactiveRetryExhaustedReturnsFriendlyError verifies that after
// maxReactiveCompactAttempts unsuccessful retries the agent stops and
// surfaces the friendly guidance error. This proves the retry cap and the
// error-wrapping contract downstream code depends on.
func TestReactiveRetryExhaustedReturnsFriendlyError(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain: 99, // always fail
		failMessage: "prompt too long: never enough room",
	}
	compactor := &mockReactiveCompactor{}
	a, sessID := newReactiveAgentTest(t, model, compactor)

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), errContextTooLongAfterCompact), "want friendly message, got %q", err.Error())

	// Initial attempt + 3 retries = 4 total Stream calls.
	require.Equal(t, 1+maxReactiveCompactAttempts, model.callCount())
	require.Equal(t, maxReactiveCompactAttempts, compactor.callCount())
}

// TestReactiveRetryStopsOnCircuitOpen asserts that when the callback reports
// CircuitOpen=true the agent stops retrying immediately. This is the
// runtime's mechanism for telling the agent "give up gracefully, further
// compaction will not help this session".
func TestReactiveRetryStopsOnCircuitOpen(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain: 99,
		failMessage: "prompt too long",
	}
	compactor := &mockReactiveCompactor{
		CircuitOpenOnAttempt: map[int]bool{1: true},
	}
	a, sessID := newReactiveAgentTest(t, model, compactor)

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.Error(t, err)
	// Circuit open on attempt 1 => 1 reactive call and no retry Stream.
	require.Equal(t, 1, model.callCount())
	require.Equal(t, 1, compactor.callCount())
	// The error must still be a context-length error, unwrapped: the
	// friendly banner is only added after the full retry cap.
	require.True(t, contextmgr.IsContextLengthError(err.Error()), "err = %v", err)
	require.False(t, strings.Contains(err.Error(), errContextTooLongAfterCompact), "circuit-open should not add the exhausted banner")
}

// TestReactiveRetryStopsWhenCallbackErrors verifies that if the reactive
// compactor itself returns an error (e.g. workspace failed to record the
// attempt) the agent stops retrying and returns the underlying PTL error
// unwrapped. The friendly banner is reserved for the exhausted-cap case.
func TestReactiveRetryStopsWhenCallbackErrors(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain: 99,
		failMessage: "prompt too long",
	}
	compactor := &mockReactiveCompactor{
		ErrOnAttempt: map[int]error{1: errors.New("workspace unavailable")},
	}
	a, sessID := newReactiveAgentTest(t, model, compactor)

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.Error(t, err)
	require.Equal(t, 1, model.callCount())
	require.Equal(t, 1, compactor.callCount())
	require.False(t, strings.Contains(err.Error(), errContextTooLongAfterCompact))
}

// TestReactiveRetryIgnoresNonContextLengthErrors makes sure only the
// specific PTL error family triggers reactive compaction. Any other
// provider error must bubble through untouched.
func TestReactiveRetryIgnoresNonContextLengthErrors(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain: 99,
		failMessage: "provider exploded",
	}
	compactor := &mockReactiveCompactor{}
	a, sessID := newReactiveAgentTest(t, model, compactor)

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.Error(t, err)
	require.Equal(t, 1, model.callCount())
	require.Equal(t, 0, compactor.callCount(), "non-PTL errors must not trigger reactive compact")
	require.False(t, strings.Contains(err.Error(), errContextTooLongAfterCompact))
}

// TestReactiveRetryNilCompactorFailsFast ensures that if the coordinator
// did not wire a ReactiveCompactor (older harnesses, tests) a PTL error is
// returned immediately with no retry attempted.
func TestReactiveRetryNilCompactorFailsFast(t *testing.T) {
	t.Parallel()

	model := &scriptedReactiveModel{
		failsRemain: 99,
		failMessage: "prompt too long",
	}
	a, sessID := newReactiveAgentTest(t, model, &mockReactiveCompactor{})
	// Detach the mock.
	a.reactiveCompactor = nil

	_, err := a.Run(t.Context(), SessionAgentCall{
		Prompt:    "please continue",
		SessionID: sessID,
		TurnID:    "turn-1",
	})
	require.Error(t, err)
	require.Equal(t, 1, model.callCount())
	require.False(t, strings.Contains(err.Error(), errContextTooLongAfterCompact))
}
