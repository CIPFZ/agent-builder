package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/session"
	"github.com/stretchr/testify/require"
)

type blockingTitleModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	tools   int
}

func (m *blockingTitleModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, fmt.Errorf("Generate is not implemented")
}

func (m *blockingTitleModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.tools = len(call.Tools)
	m.once.Do(func() { close(m.started) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.release:
		return nil, errors.New("title generation failed")
	}
}

func (m *blockingTitleModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, fmt.Errorf("GenerateObject is not implemented")
}

func (m *blockingTitleModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, fmt.Errorf("StreamObject is not implemented")
}
func (m *blockingTitleModel) Provider() string { return "test" }
func (m *blockingTitleModel) Model() string    { return "blocking-title-model" }

func TestTitleAgentOutlivesFirstTurnAndHasNoTools(t *testing.T) {
	env := testEnv(t)
	mainModel := &scriptedSummarizerModel{streamFunc: streamText("main response")}
	titleModel := &blockingTitleModel{started: make(chan struct{}), release: make(chan struct{})}
	agent := testSessionAgent(env, mainModel, titleModel, "main system prompt")
	sess, err := env.sessions.Create(t.Context(), "New chat")
	require.NoError(t, err)
	runCtx, cancelRun := context.WithCancel(t.Context())

	startedAt := time.Now()
	_, err = agent.Run(runCtx, SessionAgentCall{SessionID: sess.ID, TurnID: "turn-1", Prompt: "Please inspect the streaming response lifecycle"})
	require.NoError(t, err)
	require.Less(t, time.Since(startedAt), time.Second, "conversation waited for the Title Agent")

	select {
	case <-titleModel.started:
	case <-time.After(time.Second):
		t.Fatal("Title Agent did not start")
	}
	require.Zero(t, titleModel.tools, "Title Agent received tool schemas")
	pending, err := env.sessions.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Equal(t, session.TitleSourceFallbackPending, pending.TitleSource)
	require.Equal(t, FallbackSessionTitle("Please inspect the streaming response lifecycle"), pending.Title)

	cancelRun()
	require.Eventually(t, func() bool {
		updated, getErr := env.sessions.Get(t.Context(), sess.ID)
		return getErr == nil && updated.TitleSource == session.TitleSourceFallback
	}, time.Second, 10*time.Millisecond)
}

func TestTitleAgentCanReplaceFallbackAfterFirstTurn(t *testing.T) {
	env := testEnv(t)
	mainModel := &scriptedSummarizerModel{streamFunc: streamText("main response")}
	started := make(chan struct{})
	release := make(chan struct{})
	titleModel := &scriptedSummarizerModel{streamFunc: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return streamText("Independent title")(ctx, call)
		}
	}}
	agent := testSessionAgent(env, mainModel, titleModel, "main system prompt")
	sess, err := env.sessions.Create(t.Context(), "New chat")
	require.NoError(t, err)

	_, err = agent.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, TurnID: "turn-1", Prompt: "Please inspect the title lifecycle"})
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Title Agent did not start")
	}
	pending, err := env.sessions.Get(t.Context(), sess.ID)
	require.NoError(t, err)
	require.Equal(t, session.TitleSourceFallbackPending, pending.TitleSource)
	close(release)
	require.Eventually(t, func() bool {
		updated, getErr := env.sessions.Get(t.Context(), sess.ID)
		return getErr == nil && updated.TitleSource == session.TitleSourceAgent && updated.Title == "Independent title"
	}, time.Second, 10*time.Millisecond)
}

func TestNormalizeSessionTitleEnforcesPlainTextUnicodeLimit(t *testing.T) {
	input := "# \"<think>ignore this</think>" + string(make([]rune, 0)) + "这是一个包含换行\n和多余空格的标题 " + "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ\""
	title := NormalizeSessionTitle(input)
	require.NotContains(t, title, "think")
	require.NotContains(t, title, "\n")
	require.LessOrEqual(t, len([]rune(title)), titleAgentMaxChars)
	require.True(t, len([]rune(title)) == titleAgentMaxChars)
	require.Equal(t, '…', []rune(title)[titleAgentMaxChars-1])
}
