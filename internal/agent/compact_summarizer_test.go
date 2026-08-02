package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/stretchr/testify/require"
)

// scriptedSummarizerModel is a tiny fantasy.LanguageModel test double whose
// streamFunc returns the deltas the test wants to observe.
type scriptedSummarizerModel struct {
	streamFunc func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error)
	calls      int
}

func (m *scriptedSummarizerModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	return nil, fmt.Errorf("Generate is not implemented")
}

func (m *scriptedSummarizerModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	m.calls++
	return m.streamFunc(ctx, call)
}

func (m *scriptedSummarizerModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, fmt.Errorf("GenerateObject is not implemented")
}

func (m *scriptedSummarizerModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, fmt.Errorf("StreamObject is not implemented")
}
func (m *scriptedSummarizerModel) Provider() string { return "test" }
func (m *scriptedSummarizerModel) Model() string    { return "compact-test-model" }

// newCompactTestAgent returns a sessionAgent wired with a scripted model
// and just enough backing services to run GenerateCompactSummary. The
// environment does not need a coordinator because we invoke the summarizer
// directly.
func newCompactTestAgent(t *testing.T, largeModel fantasy.LanguageModel) *sessionAgent {
	t.Helper()
	env := testEnv(t)
	agent := testSessionAgent(env, largeModel, largeModel, "test prompt").(*sessionAgent)
	return agent
}

func compactTestMessages() []message.Message {
	return []message.Message{
		{
			ID:   "u1",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Please refactor the http router."},
			},
		},
		{
			ID:   "a1",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Analyzing the router package."},
			},
		},
		{
			ID:   "u2",
			Role: message.User,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Also add a new /health endpoint."},
			},
		},
		{
			ID:   "a2",
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: "Working on /health endpoint."},
			},
		},
	}
}

// streamText yields a single text-delta and finishes cleanly. The text end
// event is emitted so fantasy assembles the content into Response.Content.
func streamText(text string) func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	return func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
		return func(yield func(fantasy.StreamPart) bool) {
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "t1"}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "t1", Delta: text}) {
				return
			}
			if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "t1"}) {
				return
			}
			yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
		}, nil
	}
}

func TestGenerateCompactSummaryHappyPath(t *testing.T) {
	t.Parallel()

	summaryBody := "1. Primary Request and Intent\nRefactor router.\n" +
		"2. Key Technical Concepts\n- HTTP routing\n" +
		"3. Files and Code Sections\n- router/router.go\n" +
		"4. Errors and Fixes\n- none yet\n" +
		"5. Problem Solving\n- initial scan complete\n" +
		"6. All User Messages\n- refactor router\n- add /health\n" +
		"7. Pending Tasks\n- write /health\n" +
		"8. Current Work\n- editing router.go\n" +
		"9. Optional Next Step\n- verify tests"

	model := &scriptedSummarizerModel{streamFunc: streamText(summaryBody)}
	agent := newCompactTestAgent(t, model)

	res, err := agent.GenerateCompactSummary(t.Context(), CompactSummaryRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Messages:  compactTestMessages(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Summary)
	require.Contains(t, res.Summary, "Primary Request and Intent")
	require.Contains(t, res.Summary, "All User Messages")
	require.Equal(t, 1, model.calls)
}

func TestGenerateCompactSummaryStripsAnalysisBlock(t *testing.T) {
	t.Parallel()

	// Model helpfully wraps its answer in <analysis>...</analysis><summary>...</summary>.
	// After post-processing the analysis block must be gone.
	summaryBody := "<analysis>\nMulti-line scratchpad the caller does not want to see.\nStep by step reasoning here.\n</analysis>\n\n<summary>\n1. Primary Request and Intent\nDo the thing.\n</summary>"

	model := &scriptedSummarizerModel{streamFunc: streamText(summaryBody)}
	agent := newCompactTestAgent(t, model)

	res, err := agent.GenerateCompactSummary(t.Context(), CompactSummaryRequest{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Messages:  compactTestMessages(),
	})
	require.NoError(t, err)
	require.NotContains(t, res.Summary, "<analysis>")
	require.NotContains(t, res.Summary, "</analysis>")
	require.NotContains(t, res.Summary, "scratchpad the caller does not want to see")
	require.NotContains(t, res.Summary, "<summary>")
	require.NotContains(t, res.Summary, "</summary>")
	require.Contains(t, res.Summary, "Primary Request and Intent")
}

func TestGenerateCompactSummaryTimeoutBubblesUp(t *testing.T) {
	t.Parallel()

	// Model never emits a Finish event — it just blocks until the caller's
	// context deadline fires. We use a very short deadline via ctx.
	model := &scriptedSummarizerModel{
		streamFunc: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				<-ctx.Done()
			}, nil
		},
	}
	agent := newCompactTestAgent(t, model)

	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	_, err := agent.GenerateCompactSummary(ctx, CompactSummaryRequest{
		SessionID: "s1", TurnID: "t1", Messages: compactTestMessages(),
	})
	require.Error(t, err)
}

func TestGenerateCompactSummaryPTLRetryDropsOldestGroup(t *testing.T) {
	t.Parallel()

	// The first two calls return a "prompt too long" error; the third
	// succeeds. Assert that we retried and eventually returned the summary
	// text.
	attempts := 0
	model := &scriptedSummarizerModel{
		streamFunc: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("prompt too long: try again")
			}
			return streamText("1. Primary Request and Intent\nContinue work.\n")(ctx, call)
		},
	}
	agent := newCompactTestAgent(t, model)

	res, err := agent.GenerateCompactSummary(t.Context(), CompactSummaryRequest{
		SessionID: "s1", TurnID: "t1", Messages: compactTestMessages(),
	})
	require.NoError(t, err)
	require.Contains(t, res.Summary, "Continue work")
	require.GreaterOrEqual(t, attempts, 3)
}

func TestGenerateCompactSummaryNonContextErrorSurfaces(t *testing.T) {
	t.Parallel()

	model := &scriptedSummarizerModel{
		streamFunc: func(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			return nil, errors.New("provider exploded")
		},
	}
	agent := newCompactTestAgent(t, model)

	_, err := agent.GenerateCompactSummary(t.Context(), CompactSummaryRequest{
		SessionID: "s1", TurnID: "t1", Messages: compactTestMessages(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider exploded")
}

func TestDropOldestSummaryGroupBoundary(t *testing.T) {
	t.Parallel()

	assistantMsg := func(text string) fantasy.Message {
		return fantasy.Message{
			Role:    fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{fantasy.TextPart{Text: text}},
		}
	}
	msgs := []fantasy.Message{
		fantasy.NewUserMessage("first user"),
		fantasy.NewUserMessage("second user"),
		assistantMsg("first assistant"),
		fantasy.NewUserMessage("third user"),
		assistantMsg("second assistant"),
	}
	trimmed := dropOldestSummaryGroup(msgs)
	require.Less(t, len(trimmed), len(msgs))
	// First assistant + everything before it is gone; the newest user
	// message survives.
	for _, msg := range trimmed {
		text := ""
		for _, part := range msg.Content {
			if tp, ok := part.(fantasy.TextPart); ok {
				text += tp.Text
			}
		}
		require.NotContains(t, text, "first user")
		require.NotContains(t, text, "second user")
	}
}

func TestBuildCompactSummaryPromptIncludesAllSections(t *testing.T) {
	t.Parallel()

	prompt := buildCompactSummaryPrompt("please preserve API path decisions")
	for _, section := range []string{
		"1. Primary Request and Intent",
		"2. Key Technical Concepts",
		"3. Files and Code Sections",
		"4. Errors and Fixes",
		"5. Problem Solving",
		"6. All User Messages",
		"7. Pending Tasks",
		"8. Current Work",
		"9. Optional Next Step",
		"CRITICAL: Respond with TEXT ONLY",
		"Additional Instructions",
		"please preserve API path decisions",
	} {
		require.Contains(t, prompt, section, "missing section %q", section)
	}
	// verbatim requirement for Next Step
	require.True(t, strings.Contains(prompt, "verbatim"))
}

func TestStripImagesForCompactSummary(t *testing.T) {
	t.Parallel()

	msg := fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: "hello"},
			fantasy.FilePart{Filename: "shot.png", MediaType: "image/png"},
			fantasy.FilePart{Filename: "spec.pdf", MediaType: "application/pdf"},
		},
	}
	stripped := stripImagesForCompactSummary([]fantasy.Message{msg})
	require.Len(t, stripped, 1)
	kinds := ""
	for _, part := range stripped[0].Content {
		if tp, ok := part.(fantasy.TextPart); ok {
			kinds += tp.Text + "|"
		}
	}
	require.Contains(t, kinds, "hello")
	require.Contains(t, kinds, "image omitted")
	require.Contains(t, kinds, "document omitted")
}

// silence unused catwalk import when running trimmed test subsets.
var _ = catwalk.Model{}
