package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
)

func TestGovernedLanguageModelBoundsStreamingRequestLifetime(t *testing.T) {
	t.Parallel()
	governor := newTestModelGovernor(2)
	unblock := make(chan struct{})
	model := &blockingLanguageModel{entered: make(chan struct{}, 10), unblock: unblock}
	ctx := WithModelResourceGovernor(context.Background(), governor)
	governed := governLanguageModel(ctx, model)

	var done sync.WaitGroup
	for index := 0; index < 10; index++ {
		done.Add(1)
		go func() {
			defer done.Done()
			stream, err := governed.Stream(context.Background(), fantasy.Call{})
			if err != nil {
				t.Errorf("Stream: %v", err)
				return
			}
			for range stream {
			}
		}()
	}
	for index := 0; index < 2; index++ {
		select {
		case <-model.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("admitted model stream did not start")
		}
	}
	select {
	case <-model.entered:
		t.Fatal("third model stream started before a request lease was released")
	case <-time.After(100 * time.Millisecond):
	}
	close(unblock)
	done.Wait()
	if got := governor.maximum.Load(); got != 2 {
		t.Fatalf("maximum in-flight model requests = %d, want 2", got)
	}
	if got := governor.active.Load(); got != 0 {
		t.Fatalf("model leases after streams completed = %d", got)
	}
}

func TestGovernedLanguageModelAdmissionHonorsCancellation(t *testing.T) {
	t.Parallel()
	governor := newTestModelGovernor(1)
	release, err := governor.AcquireModel(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	model := &blockingLanguageModel{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	governed := governLanguageModel(WithModelResourceGovernor(ctx, governor), model)
	if _, err := governed.Generate(ctx, fantasy.Call{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate error = %v, want context cancellation", err)
	}
	if model.generateCalls.Load() != 0 {
		t.Fatal("cancelled request reached the underlying provider")
	}
}

func TestGovernedLanguageModelAccountsEncodedPromptBytes(t *testing.T) {
	t.Parallel()
	governor := newTestModelGovernor(1)
	model := &blockingLanguageModel{}
	governed := governLanguageModel(WithModelResourceGovernor(context.Background(), governor), model)
	call := fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage(strings.Repeat("payload-", 1024))}}
	if _, err := governed.Generate(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if got := governor.lastBytes.Load(); got < 8*1024 {
		t.Fatalf("model payload lease = %d bytes, want encoded prompt bytes", got)
	}
}

func TestToolResourceClassSeparatesResidentResourceKinds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source string
		name   string
		want   string
	}{
		{source: "shell", name: "bash", want: ToolResourceShell},
		{source: "shell", name: "job_output", want: ""},
		{source: "shell", name: "job_kill", want: ""},
		{source: "mcp", name: "mcp_playwright_browser_navigate", want: ToolResourceHeavy},
		{source: "mcp", name: "mcp_computer_click", want: ToolResourceHeavy},
		{source: "builtin", name: "computer_click", want: ToolResourceBrowser},
		{source: "builtin", name: "write", want: ToolResourceHeavy},
		{source: "mcp", name: "mcp_database_query", want: ToolResourceHeavy},
		{source: "builtin", name: "view", want: ""},
		{source: "builtin", name: "grep", want: ""},
	}
	for _, test := range tests {
		if got := toolResourceClass(test.source, test.name); got != test.want {
			t.Fatalf("toolResourceClass(%q, %q) = %q, want %q", test.source, test.name, got, test.want)
		}
	}
}

type testModelGovernor struct {
	sem       chan struct{}
	active    atomic.Int64
	maximum   atomic.Int64
	lastBytes atomic.Int64
}

func newTestModelGovernor(limit int) *testModelGovernor {
	return &testModelGovernor{sem: make(chan struct{}, limit)}
}

func (g *testModelGovernor) AcquireModel(ctx context.Context, payloadBytes int64) (func(), error) {
	g.lastBytes.Store(payloadBytes)
	select {
	case g.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	current := g.active.Add(1)
	for {
		observed := g.maximum.Load()
		if current <= observed || g.maximum.CompareAndSwap(observed, current) {
			break
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			g.active.Add(-1)
			<-g.sem
		})
	}, nil
}

type blockingLanguageModel struct {
	entered       chan struct{}
	unblock       <-chan struct{}
	generateCalls atomic.Int64
}

func (m *blockingLanguageModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	m.generateCalls.Add(1)
	return &fantasy.Response{}, nil
}

func (m *blockingLanguageModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	return func(func(fantasy.StreamPart) bool) {
		if m.entered != nil {
			m.entered <- struct{}{}
		}
		if m.unblock != nil {
			<-m.unblock
		}
	}, nil
}

func (m *blockingLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return &fantasy.ObjectResponse{}, nil
}

func (m *blockingLanguageModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return func(func(fantasy.ObjectStreamPart) bool) {}, nil
}

func (m *blockingLanguageModel) Provider() string { return "test" }
func (m *blockingLanguageModel) Model() string    { return "test" }
