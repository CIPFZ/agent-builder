package queryengine_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

type captureSink struct {
	events []queryengine.Event
}

func (s *captureSink) Emit(event queryengine.Event) error {
	s.events = append(s.events, event)
	return nil
}

type blockingClient struct {
	started chan struct{}
}

func (c *blockingClient) Stream(ctx context.Context, _ llm.GenerateRequest, _ llm.StreamHandler) error {
	close(c.started)
	<-ctx.Done()
	return ctx.Err()
}

type scriptedClient struct {
	mu       sync.Mutex
	scripts  [][]llm.StreamEvent
	requests []llm.GenerateRequest
}

func (c *scriptedClient) Stream(ctx context.Context, req llm.GenerateRequest, handler llm.StreamHandler) error {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	if len(c.scripts) == 0 {
		c.mu.Unlock()
		return errors.New("no scripted response remaining")
	}
	events := c.scripts[0]
	c.scripts = c.scripts[1:]
	c.mu.Unlock()

	for _, event := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := handler.OnEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func (c *scriptedClient) Requests() []llm.GenerateRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]llm.GenerateRequest(nil), c.requests...)
}

type stubToolForQueryEngine struct {
	def         tools.Definition
	enabled     bool
	readOnly    bool
	destructive bool
	shouldDefer bool
	alwaysLoad  bool
	promptText  string
	searchHint  string
}

type scriptedToolForQueryEngine struct {
	stubToolForQueryEngine
	output string
}

func (t stubToolForQueryEngine) Definition() tools.Definition {
	return t.def
}

func (t stubToolForQueryEngine) Invoke(_ context.Context, _ session.Session, _ string) (string, error) {
	return "ok", nil
}

func (t scriptedToolForQueryEngine) Invoke(_ context.Context, _ session.Session, _ string) (string, error) {
	if t.output != "" {
		return t.output, nil
	}
	return "ok", nil
}

func (t stubToolForQueryEngine) IsEnabled() bool {
	return t.enabled
}

func (t stubToolForQueryEngine) IsReadOnly(_ string) bool {
	return t.readOnly
}

func (t stubToolForQueryEngine) IsDestructive(_ string) bool {
	return t.destructive
}

func (t stubToolForQueryEngine) ShouldDefer() bool {
	return t.shouldDefer
}

func (t stubToolForQueryEngine) AlwaysLoad() bool {
	return t.alwaysLoad
}

func (t stubToolForQueryEngine) PromptDescription() string {
	if t.promptText != "" {
		return t.promptText
	}
	return t.def.Description
}

func (t stubToolForQueryEngine) SearchHint() string {
	if t.searchHint != "" {
		return t.searchHint
	}
	return t.def.SearchHint
}

type inputProcessorFunc func(context.Context, session.Session, string) (string, bool, error)

func (f inputProcessorFunc) Process(ctx context.Context, sess session.Session, input string) (queryengine.ProcessResult, error) {
	result, shouldQuery, err := f(ctx, sess, input)
	return queryengine.ProcessResult{
		NormalizedInput: result,
		ShouldQuery:     shouldQuery,
	}, err
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestQueryEngineSubmitMessageCompletesAssistantTurn(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(sink.events) < 3 {
		t.Fatalf("event count = %d, want lifecycle and message events", len(sink.events))
	}
	if sink.events[0].Type != "agent.lifecycle.start" {
		t.Fatalf("first event = %q, want agent.lifecycle.start", sink.events[0].Type)
	}
	if sink.events[len(sink.events)-1].Type != "agent.lifecycle.end" {
		t.Fatalf("last event = %q, want agent.lifecycle.end", sink.events[len(sink.events)-1].Type)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	if got := messages[len(messages)-1]; got.Role != "assistant" || !strings.Contains(got.Content, "Received: hello") {
		t.Fatalf("last message = %#v, want assistant reply", got)
	}
}

func TestQueryEngineSubmitMessageCreatesApprovalForDeniedTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})

	sink := &captureSink{}
	err = engine.SubmitMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected approval-required tool request to fail")
	}

	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}
	if requests[0].Category != string(permissions.CategoryApproval) {
		t.Fatalf("approval category = %q, want %q", requests[0].Category, permissions.CategoryApproval)
	}
	found := false
	for _, event := range sink.events {
		if event.Type == "permission.required" && event.Approval != nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected permission.required event")
	}
	for _, event := range sink.events {
		if event.Type == "run.error" {
			t.Fatalf("events = %#v, did not want run.error for approval-required flow", sink.events)
		}
	}
	state := engine.State()
	if len(state.PermissionDenials) != 1 {
		t.Fatalf("permission denials = %#v, want one item", state.PermissionDenials)
	}
	if state.PermissionDenials[0].ToolName != "system.run" {
		t.Fatalf("permission denial tool = %#v, want system.run", state.PermissionDenials[0])
	}
}

func TestQueryEngineSubmitMessageCarriesRuleSourceIntoApproval(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		ApprovalManager: approvalManager,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{
					ToolName: "system.run",
					Source:   string(permissions.RuleSourceSession),
					Action:   permissions.ActionDeny,
				},
			},
		},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial request to require denial/approval path")
	}

	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}
	if requests[0].RuleSource != string(permissions.RuleSourceSession) {
		t.Fatalf("approval rule source = %q, want %q", requests[0].RuleSource, permissions.RuleSourceSession)
	}
}

func TestQueryEngineApproveAndContinueCompletesPendingToolTurn(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial request to require approval")
	}
	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}

	sink := &captureSink{}
	if err := engine.ApproveAndContinue(context.Background(), requests[0].ID, sink); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "Using tool result: system.run:") {
		t.Fatalf("last message = %#v, want assistant continuation", last)
	}
}

func TestQueryEngineApproveAndContinueStabilizesAfterApprovedToolInsteadOfExpandingNewCommand(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "system.run", ToolInput: "pwd"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "system.run", ToolInput: "dir logs\\myclaw.jsonl"},
				{Type: "message.end"},
			},
		},
	}

	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:         tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true},
				enabled:     true,
				destructive: true,
			},
			output: "C:\\approved\\pwd",
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial request to require approval")
	}
	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}

	sink := &captureSink{}
	if err := engine.ApproveAndContinue(context.Background(), requests[0].ID, sink); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}

	toolCalledCount := 0
	for _, event := range sink.events {
		if event.Type == "tool.called" {
			toolCalledCount++
		}
		if event.Type == "permission.required" {
			t.Fatalf("events = %#v, did not want a second permission.required after approved tool", sink.events)
		}
	}
	if toolCalledCount != 1 {
		t.Fatalf("tool called count = %d, want only approved tool execution", toolCalledCount)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" || last.Content != "Using tool result: system.run: C:\\approved\\pwd" {
		t.Fatalf("last message = %#v, want stabilized assistant continuation using approved tool result", last)
	}
}

func TestQueryEngineSubmitMessageSupportsMultipleToolPasses(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run multi tool plan")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "first pass"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "second pass"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MaxTurns:         3,
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	toolResults := 0
	for _, event := range sink.events {
		if event.Type == "tool.result" {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Fatalf("tool result count = %d, want 2", toolResults)
	}

	messages := engine.Messages(sess.ID)
	if got := messages[len(messages)-1]; got.Role != "assistant" || got.Content != "done" {
		t.Fatalf("last message = %#v, want final assistant completion", got)
	}

	state := engine.State()
	if state.LastModelPassCount != 3 {
		t.Fatalf("last model pass count = %d, want 3", state.LastModelPassCount)
	}
	if state.MaxTurns != 3 {
		t.Fatalf("max turns = %d, want 3", state.MaxTurns)
	}
}

func TestQueryEngineSubmitMessageStopsAtMaxTurns(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "loop forever")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "first"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "second"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "third"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MaxTurns:         2,
	})

	sink := &captureSink{}
	err = engine.SubmitMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected max turns to stop repeated tool loop")
	}
	if !strings.Contains(err.Error(), "maximum number of turns") {
		t.Fatalf("error = %v, want maximum number of turns", err)
	}

	state := engine.State()
	if !state.MaxTurnsExceeded {
		t.Fatalf("state = %#v, want max turns exceeded", state)
	}
	if state.LastModelPassCount != 2 {
		t.Fatalf("last model pass count = %d, want 2", state.LastModelPassCount)
	}
}

func TestQueryEngineStabilizesRepeatedIdenticalToolCallLoop(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "uppercase hello world")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello world"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello world"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MaxTurns:         8,
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	toolResults := 0
	foundAssistant := false
	for _, event := range sink.events {
		if event.Type == "tool.result" {
			toolResults++
		}
		if event.Type == "message.created" && event.Message != nil && event.Message.Role == "assistant" && event.Message.Content == "Using tool result: text.upper: HELLO WORLD" {
			foundAssistant = true
		}
	}
	if toolResults != 1 {
		t.Fatalf("tool result count = %d, want 1 before loop stabilization", toolResults)
	}
	if !foundAssistant {
		t.Fatalf("events = %#v, want stabilized assistant reply using tool result", sink.events)
	}
}

func TestQueryEngineStabilizesRepeatedDeferredToolDelegationLoop(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "delegate summary work")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "agent.task", ToolInput: "summarize current session"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "agent.task", ToolInput: "summarize current session again"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MaxTurns:         8,
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	toolResults := 0
	foundAssistant := false
	for _, event := range sink.events {
		if event.Type == "tool.result" {
			toolResults++
		}
		if event.Type == "message.created" && event.Message != nil && event.Message.Role == "assistant" && strings.Contains(event.Message.Content, "Using tool result: agent.task:") {
			foundAssistant = true
		}
	}
	if toolResults != 1 {
		t.Fatalf("tool result count = %d, want 1 before deferred loop stabilization", toolResults)
	}
	if !foundAssistant {
		t.Fatalf("events = %#v, want stabilized assistant reply using agent.task result", sink.events)
	}
}

func TestQueryEngineSubmitMessageCompactsHistoryAndSavesMemory(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"assistant", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{"user", "cccccccccccccccccccccccccccccccccccccccc"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello memory")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	memSvc := memory.NewService()
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		MemoryService:   memSvc,
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(memSvc.List(sess.ID)) == 0 {
		t.Fatal("expected memory to be saved after compaction")
	}
	found := false
	for _, event := range sink.events {
		if event.Type == "memory.saved" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected memory.saved event")
	}
}

func TestQueryEngineSubmitMessageEmitsCompactBoundaryEvent(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 40)},
		{"assistant", strings.Repeat("b", 40)},
		{"user", strings.Repeat("c", 40)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello boundary")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.boundary" && event.Message != nil && event.Message.Role == "system" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.boundary event", sink.events)
	}

	state := engine.State()
	if state.CompactBoundaryCount != 1 {
		t.Fatalf("compact boundary count = %d, want 1", state.CompactBoundaryCount)
	}
	if state.LastCompactBoundaryID == "" {
		t.Fatalf("state = %#v, want last compact boundary id", state)
	}
}

func TestQueryEngineSubmitMessageCanReplaySnipAfterCompactBoundary(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 40)},
		{"assistant", strings.Repeat("b", 40)},
		{"user", strings.Repeat("c", 40)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello snip")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		SnipReplay: func(boundary session.Message, store []session.Message) *queryengine.SnipReplayResult {
			if boundary.Role != "system" {
				t.Fatalf("boundary = %#v, want system role", boundary)
			}
			if len(store) < 2 {
				t.Fatalf("store len = %d, want compacted messages before replay", len(store))
			}
			return &queryengine.SnipReplayResult{
				Messages: append([]session.Message(nil), store[len(store)-2:]...),
				Executed: true,
			}
		},
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages := engine.Messages(sess.ID)
	if len(messages) < 2 {
		t.Fatalf("messages len = %d, want replayed messages", len(messages))
	}
	if messages[0].Role == "summary" {
		t.Fatalf("messages = %#v, want replayed tail without original compacted summary", messages)
	}
}

func TestQueryEngineEmitsCompactionReplayEventAndTracksState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 40)},
		{"assistant", strings.Repeat("b", 40)},
		{"user", strings.Repeat("c", 40)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello replay event")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		SnipReplay: func(_ session.Message, store []session.Message) *queryengine.SnipReplayResult {
			return &queryengine.SnipReplayResult{
				Messages: append([]session.Message(nil), store[len(store)-2:]...),
				Executed: true,
			}
		},
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.replayed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.replayed event", sink.events)
	}

	state := engine.State()
	if !state.LastCompactionReplayExecuted {
		t.Fatalf("state = %#v, want replay executed", state)
	}
	if state.LastCompactionReplayCount == 0 {
		t.Fatalf("state = %#v, want replay message count", state)
	}
}

func TestQueryEngineEmitsCompactionMemorySavedEventAndTracksState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 40)},
		{"assistant", strings.Repeat("b", 40)},
		{"user", strings.Repeat("c", 40)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello memory event")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		MemoryService:   memory.NewService(),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.memory_saved" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.memory_saved event", sink.events)
	}

	state := engine.State()
	if !state.LastCompactionMemorySaved {
		t.Fatalf("state = %#v, want memory saved", state)
	}
	if state.LastCompactionSummaryID == "" {
		t.Fatalf("state = %#v, want summary id", state)
	}
}

func TestQueryEngineEmitsCompactionCleanupEventAndTracksState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 40)},
		{"assistant", strings.Repeat("b", 40)},
		{"user", strings.Repeat("c", 40)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello cleanup event")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		PostCompactCleanup: func(_ session.Message, messages []session.Message) *queryengine.PostCompactCleanupResult {
			if len(messages) < 2 {
				t.Fatalf("messages len = %d, want compacted state before cleanup", len(messages))
			}
			return &queryengine.PostCompactCleanupResult{
				Messages: append([]session.Message(nil), messages[len(messages)-2:]...),
				Executed: true,
			}
		},
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.cleaned" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.cleaned event", sink.events)
	}

	state := engine.State()
	if !state.LastCompactionCleanupExecuted {
		t.Fatalf("state = %#v, want cleanup executed", state)
	}
	if state.LastCompactionCleanupCount == 0 {
		t.Fatalf("state = %#v, want cleanup count", state)
	}
}

func TestQueryEngineStateTracksCompactionAnalysis(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 120)},
		{"assistant", strings.Repeat("b", 120)},
		{"assistant", strings.Repeat("c", 96)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello analysis")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:            999,
			ContextWindowTokens:    100,
			WarningBufferTokens:    20,
			ErrorBufferTokens:      10,
			AutoCompactBufferTokens: 5,
			BlockingBufferTokens:   2,
			PreserveRecentTurns:    2,
			SummaryPrefix:          "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	state := engine.State()
	if state.LastEstimatedContextTokens == 0 {
		t.Fatalf("state = %#v, want estimated context tokens", state)
	}
	if state.ContextWindowTokens != 100 {
		t.Fatalf("context window = %d, want 100", state.ContextWindowTokens)
	}
	if state.WarningThresholdTokens != 80 {
		t.Fatalf("warning threshold = %d, want 80", state.WarningThresholdTokens)
	}
	if !state.IsAboveWarningThreshold {
		t.Fatalf("state = %#v, want warning threshold tripped", state)
	}
	if state.IsAboveErrorThreshold {
		t.Fatalf("state = %#v, want error threshold to stay false", state)
	}
	if state.IsAboveAutoCompactThreshold {
		t.Fatalf("state = %#v, want auto compact threshold to stay false", state)
	}
	if state.IsAtBlockingContextLimit {
		t.Fatalf("state = %#v, want blocking threshold to stay false", state)
	}
}

func TestQueryEngineStateTracksStructuredCompactionResult(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "first request"},
		{"assistant", "first response"},
		{"user", "second request"},
		{"assistant", "second response"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello compact result")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         3,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	state := engine.State()
	if state.LastCompactionReason != string(compaction.ReasonMessageLimit) {
		t.Fatalf("last compaction reason = %q, want %q", state.LastCompactionReason, compaction.ReasonMessageLimit)
	}
	if state.LastCompactionOriginalCount != 5 {
		t.Fatalf("last compaction original count = %d, want 5", state.LastCompactionOriginalCount)
	}
	if state.LastCompactionResultCount != 3 {
		t.Fatalf("last compaction result count = %d, want 3", state.LastCompactionResultCount)
	}
}

func TestQueryEngineEmitsCompactionWarningEvent(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 120)},
		{"assistant", strings.Repeat("b", 120)},
		{"assistant", strings.Repeat("c", 96)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello warning")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:             999,
			ContextWindowTokens:     100,
			WarningBufferTokens:     20,
			ErrorBufferTokens:       10,
			AutoCompactBufferTokens: 5,
			BlockingBufferTokens:    2,
			PreserveRecentTurns:     2,
			SummaryPrefix:           "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.warning" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.warning event", sink.events)
	}
}

func TestQueryEngineBlocksWhenContextLimitReachedWithoutCompactionEscape(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 160)},
		{"assistant", strings.Repeat("b", 160)},
		{"assistant", strings.Repeat("c", 160)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", strings.Repeat("d", 80))
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:             999,
			ContextWindowTokens:     100,
			WarningBufferTokens:     20,
			ErrorBufferTokens:       10,
			AutoCompactBufferTokens: 5,
			BlockingBufferTokens:    2,
			PreserveRecentTurns:     10,
			SummaryPrefix:           "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	err = engine.SubmitMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected context blocking error")
	}
	if !strings.Contains(err.Error(), "context window blocking limit") {
		t.Fatalf("error = %v, want context window blocking limit", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.blocked" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.blocked event", sink.events)
	}
}

func TestQueryEngineEmitsCompactionErrorEvent(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 120)},
		{"assistant", strings.Repeat("b", 120)},
		{"assistant", strings.Repeat("c", 120)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello error")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:             999,
			ContextWindowTokens:     100,
			WarningBufferTokens:     40,
			ErrorBufferTokens:       20,
			AutoCompactBufferTokens: 5,
			BlockingBufferTokens:    2,
			PreserveRecentTurns:     2,
			SummaryPrefix:           "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.error" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.error event", sink.events)
	}
}

func TestQueryEngineEmitsCompactionAutoEventWhenCompactionRunsInAutoRange(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", strings.Repeat("a", 120)},
		{"assistant", strings.Repeat("b", 120)},
		{"assistant", strings.Repeat("c", 120)},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello auto")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:             3,
			ContextWindowTokens:     100,
			WarningBufferTokens:     40,
			ErrorBufferTokens:       20,
			AutoCompactBufferTokens: 10,
			BlockingBufferTokens:    2,
			PreserveRecentTurns:     2,
			SummaryPrefix:           "Summary:",
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.auto" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want compact.auto event", sink.events)
	}
}

func TestQueryEngineInterruptCancelsActiveTurnAndClearsState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "hang please")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &blockingClient{started: make(chan struct{})}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.SubmitMessage(context.Background(), sess, msg, &captureSink{})
	}()

	select {
	case <-client.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queryengine stream to start")
	}

	state := engine.State()
	if state.ActiveRunID == "" {
		t.Fatalf("state before interrupt = %#v, want active run", state)
	}

	engine.Interrupt()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("submit message error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for interrupted turn to exit")
	}

	state = engine.State()
	if state.ActiveRunID != "" {
		t.Fatalf("state after interrupt = %#v, want no active run", state)
	}
	if state.LastError == "" {
		t.Fatalf("state after interrupt = %#v, want recorded last error", state)
	}
}

func TestQueryEngineMessagesReturnConversationState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	inputs := []string{"hello", "introduce yourself"}
	for _, input := range inputs {
		msg, err := sessions.AppendMessage(sess.ID, "user", input)
		if err != nil {
			t.Fatalf("append user message: %v", err)
		}
		if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
			t.Fatalf("submit message %q: %v", input, err)
		}
	}

	messages := engine.Messages(sess.ID)
	if len(messages) < 4 {
		t.Fatalf("messages len = %d, want at least 4", len(messages))
	}
	if messages[0].Role != "user" {
		t.Fatalf("first message = %#v, want user", messages[0])
	}
	if messages[len(messages)-1].Role != "assistant" {
		t.Fatalf("last message = %#v, want assistant", messages[len(messages)-1])
	}

	state := engine.State()
	if state.LastRunID == "" || state.LastEvent != "agent.lifecycle.end" {
		t.Fatalf("state = %#v, want completed run metadata", state)
	}
}

func TestQueryEngineSubmitPromptOwnsUserMessageCreation(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello from prompt", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages := engine.Messages(sess.ID)
	if len(messages) < 2 {
		t.Fatalf("messages len = %d, want at least 2", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "hello from prompt" {
		t.Fatalf("first message = %#v, want created user prompt message", messages[0])
	}
	if messages[1].Role != "assistant" {
		t.Fatalf("second message = %#v, want assistant reply", messages[1])
	}
}

func TestQueryEngineMessagesUseMutableConversationState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         3,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
	})

	for _, input := range []string{"one", "two", "three"} {
		if err := engine.SubmitPrompt(context.Background(), sess, input, &captureSink{}); err != nil {
			t.Fatalf("submit prompt %q: %v", input, err)
		}
	}

	messages := engine.Messages(sess.ID)
	if len(messages) == 0 {
		t.Fatal("expected mutable messages to be populated")
	}
	if messages[0].Role != "summary" {
		t.Fatalf("first message = %#v, want summary from compacted mutable state", messages[0])
	}

	storeMessages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found in store", sess.ID)
	}
	if len(storeMessages) != len(messages) {
		t.Fatalf("store messages len = %d, mutable len = %d, want parity", len(storeMessages), len(messages))
	}
}

func TestQueryEngineSubmitPromptUsesInputProcessor(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		InputProcessor: inputProcessorFunc(func(_ context.Context, _ session.Session, input string) (string, bool, error) {
			return strings.ToUpper(strings.TrimSpace(input)), true, nil
		}),
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "  hello processor  ", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}
	messages := engine.Messages(sess.ID)
	if len(messages) == 0 || messages[0].Content != "HELLO PROCESSOR" {
		t.Fatalf("messages = %#v, want normalized user prompt", messages)
	}
}

func TestQueryEngineSubmitPromptCanShortCircuitViaInputProcessor(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		InputProcessor: queryengine.InputProcessorFunc(func(_ context.Context, _ session.Session, _ string) (queryengine.ProcessResult, error) {
			return queryengine.ProcessResult{
				ShouldQuery: false,
				ResultText:  "local command result",
			}, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitPrompt(context.Background(), sess, "ignored", sink); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}
	messages := engine.Messages(sess.ID)
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one immediate local result message", messages)
	}
	if messages[0].Role != "assistant" || messages[0].Content != "local command result" {
		t.Fatalf("messages = %#v, want local assistant result", messages)
	}
	found := false
	for _, event := range sink.events {
		if event.Type == "message.created" && event.Message != nil && event.Message.Content == "local command result" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want immediate message.created for local result", sink.events)
	}
}

func TestQueryEngineSubmitPromptCanEmitMultipleImmediateMessages(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		InputProcessor: queryengine.InputProcessorFunc(func(_ context.Context, _ session.Session, _ string) (queryengine.ProcessResult, error) {
			return queryengine.ProcessResult{
				ShouldQuery: false,
				InputMode:   "command",
				CommandName: "local.help",
				Messages: []queryengine.ImmediateMessage{
					{Role: "assistant", Content: "Available commands: /help, /status"},
					{Role: "assistant", Content: "Tip: use /help <topic> for more detail"},
				},
			}, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitPrompt(context.Background(), sess, "/help", sink); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages := engine.Messages(sess.ID)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2 immediate messages", len(messages))
	}
	if messages[0].Content != "Available commands: /help, /status" {
		t.Fatalf("first message = %#v, want first command output", messages[0])
	}
	if messages[1].Content != "Tip: use /help <topic> for more detail" {
		t.Fatalf("second message = %#v, want second command output", messages[1])
	}

	state := engine.State()
	if state.LastInputMode != "command" {
		t.Fatalf("last input mode = %q, want command", state.LastInputMode)
	}
	if state.LastCommandName != "local.help" {
		t.Fatalf("last command name = %q, want local.help", state.LastCommandName)
	}
	if state.LastImmediateMessageCount != 2 {
		t.Fatalf("last immediate message count = %d, want 2", state.LastImmediateMessageCount)
	}
}

func TestQueryEngineSubmitPromptCanInjectMessagesBeforeQuery(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		InputProcessor: queryengine.InputProcessorFunc(func(_ context.Context, _ session.Session, _ string) (queryengine.ProcessResult, error) {
			return queryengine.ProcessResult{
				NormalizedInput: "normalized question",
				ShouldQuery:     true,
				InputMode:       "command",
				CommandName:     "attach.context",
				Messages: []queryengine.ImmediateMessage{
					{Role: "system", Content: "Attached context: README.md"},
					{Role: "assistant", Content: "Loaded context for this turn."},
				},
			}, nil
		}),
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "@README explain this project", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages := engine.Messages(sess.ID)
	if len(messages) < 4 {
		t.Fatalf("messages len = %d, want at least 4", len(messages))
	}
	if messages[0].Role != "system" || messages[0].Content != "Attached context: README.md" {
		t.Fatalf("first message = %#v, want injected system context", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "Loaded context for this turn." {
		t.Fatalf("second message = %#v, want injected assistant confirmation", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Content != "normalized question" {
		t.Fatalf("third message = %#v, want normalized user message after injected messages", messages[2])
	}

	state := engine.State()
	if state.LastInputMode != "command" {
		t.Fatalf("last input mode = %q, want command", state.LastInputMode)
	}
	if state.LastCommandName != "attach.context" {
		t.Fatalf("last command name = %q, want attach.context", state.LastCommandName)
	}
	if state.LastImmediateMessageCount != 2 {
		t.Fatalf("last immediate message count = %d, want 2", state.LastImmediateMessageCount)
	}
	if state.LastUserInput != "normalized question" {
		t.Fatalf("last user input = %q, want normalized question", state.LastUserInput)
	}
}

func TestQueryEngineUsesExposedToolPoolForPromptContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def: tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
		stubToolForQueryEngine{
			def: tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true},
			enabled: true,
		},
		stubToolForQueryEngine{
			def: tools.Definition{Name: "agent.task", Description: "Run delegated task.", Enabled: true},
			enabled:     true,
			readOnly:    true,
			shouldDefer: true,
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "text.upper", Action: permissions.ActionDeny},
			},
		},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if len(requests[0].Context.ToolLines) != 2 {
		t.Fatalf("tool lines = %#v, want only visible prompt tool pool", requests[0].Context.ToolLines)
	}
	if requests[0].Context.ToolLines[0] != "system.run: Run command." {
		t.Fatalf("tool lines = %#v, want system.run exposed after deny filtering", requests[0].Context.ToolLines)
	}
	if requests[0].Context.ToolLines[1] != "tool.search: Search available and deferred tools by capability keywords. [search hint: find tool by capability] [always-loaded]" {
		t.Fatalf("tool lines = %#v, want tool.search exposed as always-load discovery tool", requests[0].Context.ToolLines)
	}
}

func TestQueryEngineDefaultToolPoolIncludesAgentTask(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}

	joined := strings.Join(requests[0].Context.ToolLines, "\n")
	if !strings.Contains(joined, "agent.task:") {
		t.Fatalf("tool lines = %#v, want default prompt exposure to include agent.task", requests[0].Context.ToolLines)
	}
}

func TestQueryEngineIncludesSessionPermissionAndWorkspaceContextLines(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader("C:/repo"),
		PermissionPolicy: permissions.Policy{
			Mode:           permissions.ModeWorkspaceWrite,
			PlanMode:       true,
			AutoMode:       true,
			WorkspaceRoots: []string{"C:/repo", "C:/repo/sub"},
		},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}

	if !containsString(requests[0].Context.UserContextLines, "session_id="+sess.ID) {
		t.Fatalf("user context lines = %#v, want session id", requests[0].Context.UserContextLines)
	}
	if !containsString(requests[0].Context.UserContextLines, "agent_id="+sess.AgentID) {
		t.Fatalf("user context lines = %#v, want agent id", requests[0].Context.UserContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "permission_mode=workspace-write") {
		t.Fatalf("system context lines = %#v, want permission mode", requests[0].Context.SystemContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "plan_mode=true") {
		t.Fatalf("system context lines = %#v, want plan mode", requests[0].Context.SystemContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "auto_mode=true") {
		t.Fatalf("system context lines = %#v, want auto mode", requests[0].Context.SystemContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "workspace_root=C:/repo") {
		t.Fatalf("system context lines = %#v, want workspace root", requests[0].Context.SystemContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "workspace_roots=C:/repo,C:/repo/sub") {
		t.Fatalf("system context lines = %#v, want workspace roots", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEngineStateTracksTurnTimingAndStreamingMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello metadata", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	state := engine.State()
	if state.LastUserInput != "hello metadata" {
		t.Fatalf("last user input = %q, want %q", state.LastUserInput, "hello metadata")
	}
	if state.LastTurnStartedAt.IsZero() || state.LastTurnCompletedAt.IsZero() {
		t.Fatalf("state = %#v, want populated turn timestamps", state)
	}
	if state.LastTurnDuration <= 0 {
		t.Fatalf("last turn duration = %v, want > 0", state.LastTurnDuration)
	}
	if state.StreamDeltaCount == 0 {
		t.Fatalf("stream delta count = %d, want > 0", state.StreamDeltaCount)
	}
	if state.StreamEventCount == 0 || state.LastStreamEvent != "message.end" {
		t.Fatalf("stream event state = %#v, want recorded stream lifecycle", state)
	}
	if len(state.RecentStreamEvents) == 0 {
		t.Fatalf("recent stream events = %#v, want non-empty history", state.RecentStreamEvents)
	}
	if state.LastPromptTokens == 0 || state.LastCompletionTokens == 0 || state.LastTotalTokens == 0 {
		t.Fatalf("token stats = %#v, want non-zero usage estimates", state)
	}
	if state.ActiveAssistantText != "" {
		t.Fatalf("active assistant text = %q, want cleared after completion", state.ActiveAssistantText)
	}
}

func TestQueryEngineCanEmitPartialStreamEventsWhenEnabled(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:                   sessions,
		Client:                     llm.NewMockClient(),
		WorkspaceLoader:            workspace.NewLoader(""),
		PermissionPolicy:           permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		IncludePartialStreamEvents: true,
	})

	sink := &captureSink{}
	if err := engine.SubmitPrompt(context.Background(), sess, "hello partials", sink); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "stream.event" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected stream.event when partial streaming is enabled")
	}
}

func TestQueryEngineDoesNotEmitPartialStreamEventsByDefault(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitPrompt(context.Background(), sess, "hello default stream", sink); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	for _, event := range sink.events {
		if event.Type == "stream.event" {
			t.Fatalf("unexpected stream.event in default mode: %#v", event)
		}
	}
}

func TestQueryEngineStateAccumulatesEstimatedUsageAcrossTurns(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello one", &captureSink{}); err != nil {
		t.Fatalf("submit prompt one: %v", err)
	}
	first := engine.State()
	if first.TotalEstimatedTokens == 0 || first.TurnCount != 1 {
		t.Fatalf("first state = %#v, want accumulated usage after first turn", first)
	}

	if err := engine.SubmitPrompt(context.Background(), sess, "hello two", &captureSink{}); err != nil {
		t.Fatalf("submit prompt two: %v", err)
	}
	second := engine.State()
	if second.TotalEstimatedTokens <= first.TotalEstimatedTokens {
		t.Fatalf("total estimated tokens = %d, first = %d, want growth", second.TotalEstimatedTokens, first.TotalEstimatedTokens)
	}
	if second.TurnCount != 2 {
		t.Fatalf("turn count = %d, want 2", second.TurnCount)
	}
}

func TestQueryEngineStateTracksTokenBudgetExceeded(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	engine := queryengine.New(queryengine.Config{
		Sessions:              sessions,
		Client:                llm.NewMockClient(),
		WorkspaceLoader:       workspace.NewLoader(""),
		PermissionPolicy:      permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		EstimatedTokenBudget:  5,
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "this is long enough to exceed the tiny budget", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}
	state := engine.State()
	if state.TokenBudget != 5 {
		t.Fatalf("token budget = %d, want 5", state.TokenBudget)
	}
	if !state.BudgetExceeded {
		t.Fatalf("state = %#v, want budget exceeded", state)
	}
}

func TestQueryEngineStateIsRaceSafeDuringConcurrentReads(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = engine.SubmitMessage(context.Background(), sess, msg, &captureSink{})
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = engine.State()
		}
	}()
	wg.Wait()
}
