package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	"myclaw/internal/queryengine"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

type captureSink struct {
	events []RuntimeEvent
}

func (s *captureSink) Emit(event RuntimeEvent) error {
	s.events = append(s.events, event)
	return nil
}

type captureMemoryClient struct {
	lastRequest llm.GenerateRequest
}

type captureHook struct {
	events []orchestration.Event
}

func (h *captureHook) Handle(_ context.Context, event orchestration.Event) error {
	h.events = append(h.events, event)
	return nil
}

type preToolUseHookFunc func(context.Context, queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error)

func (f preToolUseHookFunc) BeforeToolUse(ctx context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
	return f(ctx, request)
}

type postToolUseHookFunc func(context.Context, queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error)

func (f postToolUseHookFunc) AfterToolUse(ctx context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
	return f(ctx, request)
}

func (c *captureMemoryClient) Stream(_ context.Context, req llm.GenerateRequest, handler llm.StreamHandler) error {
	c.lastRequest = req
	if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: "ok"}); err != nil {
		return err
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

type scriptedClient struct {
	responses []string
	call      int
}

func (c *scriptedClient) Stream(_ context.Context, _ llm.GenerateRequest, handler llm.StreamHandler) error {
	if c.call >= len(c.responses) {
		return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
	}
	content := c.responses[c.call]
	c.call++
	if err := handler.OnEvent(llm.StreamEvent{Type: "text.delta", Delta: content}); err != nil {
		return err
	}
	return handler.OnEvent(llm.StreamEvent{Type: "message.end"})
}

func TestRunnerHandleUserMessageDoesNotEmitRunErrorWhenApprovalIsRequired(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})

	sink := &captureSink{}
	err = runner.HandleUserMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected denied tool execution to return an error")
	}

	foundPermissionRequired := false
	for _, event := range sink.events {
		if event.Type == "run.error" {
			t.Fatalf("events = %#v, did not want run.error for approval-required flow", sink.events)
		}
		if event.Type == "permission.required" {
			foundPermissionRequired = true
		}
	}
	if !foundPermissionRequired {
		t.Fatalf("events = %#v, want permission.required event", sink.events)
	}
}

func TestRunnerHandleUserMessageCompactsStoredHistory(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "first"},
		{"assistant", "one"},
		{"user", "second"},
		{"assistant", "two"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         3,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	if len(messages) == 0 || messages[0].Role != "summary" {
		t.Fatalf("messages = %#v, want compacted summary at the front", messages)
	}
}

func TestNewRunnerWithOptionsEnablesDefaultCompaction(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for i := 0; i < 6; i++ {
		if _, err := sessions.AppendMessage(sess.ID, "user", strings.Repeat("x", 80)); err != nil {
			t.Fatalf("append seed message %d: %v", i, err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "trigger default compact")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	found := false
	for _, event := range sink.events {
		if event.Type == "compact.boundary" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %#v, want default runner to emit compact.boundary", sink.events)
	}
}

func TestRunnerHandleUserMessageDenyRuleOverridesGlobalAllow(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run rm -rf /tmp/demo")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{
					ToolName: "system.run",
					Action:   permissions.ActionDeny,
					Match: permissions.Match{
						CommandContains: []string{"rm -rf"},
					},
				},
			},
		},
	})

	sink := &captureSink{}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	var toolResult *RuntimeEvent
	for i := range sink.events {
		if sink.events[i].Type == "tool.result" {
			toolResult = &sink.events[i]
			break
		}
	}
	if toolResult == nil || toolResult.Message == nil {
		t.Fatalf("events = %#v, want denied tool result", sink.events)
	}
	if len(toolResult.Message.Blocks) == 0 || !toolResult.Message.Blocks[0].IsError {
		t.Fatalf("tool result blocks = %#v, want error tool result", toolResult.Message.Blocks)
	}
}

func TestNewRunnerWithOptionsPassesPreToolUseHookToQueryEngine(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool upper original")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			if request.ToolName != "text.upper" || request.ToolInput != "original" {
				t.Fatalf("pre hook request = %#v, want text.upper original", request)
			}
			return queryengine.PreToolUseHookResult{UpdatedInput: "rewritten"}, true, nil
		}),
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	found := false
	for _, message := range messages {
		if message.Role == "tool" && message.Content == "text.upper: REWRITTEN" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("messages = %#v, want tool output from PreToolUse updated input", messages)
	}
}

func TestNewRunnerWithOptionsPassesPostToolUseHookToQueryEngine(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool upper hello")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseHook: postToolUseHookFunc(func(_ context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
			if request.ToolName != "text.upper" || request.ToolOutput != "HELLO" {
				t.Fatalf("post hook request = %#v, want text.upper HELLO", request)
			}
			return queryengine.PostToolUseHookResult{AdditionalContexts: []string{"runtime post context"}}, true, nil
		}),
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw == nil || block.Raw["type"] != "hook_additional_context" {
				continue
			}
			contexts, ok := block.Raw["content"].([]string)
			if ok && len(contexts) == 1 && contexts[0] == "runtime post context" {
				return
			}
		}
	}
	t.Fatalf("messages = %#v, want PostToolUse additional context block from runner option", messages)
}

func TestRunnerHandleUserMessageCompactsByEstimatedTokens(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"assistant", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{"tool", "tool result"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello again")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	if len(messages) == 0 || messages[0].Role != "summary" {
		t.Fatalf("messages = %#v, want summary after token-based compaction", messages)
	}
}

func TestRunnerHandleUserMessageSavesSessionMemoryOnCompaction(t *testing.T) {
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
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:         99,
			MaxEstimatedTokens:  20,
			PreserveRecentTurns: 2,
			SummaryPrefix:       "Summary:",
		}),
		MemoryService: memSvc,
	})

	sink := &captureSink{}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if len(memSvc.List(sess.ID)) == 0 {
		t.Fatal("expected at least one saved memory after compaction")
	}
	foundMemorySaved := false
	for _, event := range sink.events {
		if event.Type == "memory.saved" {
			foundMemorySaved = true
			break
		}
	}
	if !foundMemorySaved {
		t.Fatal("expected memory.saved event")
	}
}

func TestRunnerHandleUserMessageInjectsSessionMemoryIntoPrompt(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "use what you remember")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	memSvc := memory.NewService()
	memSvc.SaveCompactionSummary(sess, session.Message{
		ID:        "summary-1",
		SessionID: sess.ID,
		Role:      "summary",
		Content:   "Summary: remember the deployment checklist",
	})

	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MemoryService:    memSvc,
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if len(client.lastRequest.Context.MemoryLines) == 0 {
		t.Fatal("expected memory lines to be injected into prompt context")
	}
}

func TestRunnerHandleUserMessageInjectsTypedSessionMemoriesIntoPrompt(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "use typed memory")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	memSvc := memory.NewService()
	memSvc.Save(sess, memory.Entry{Type: memory.TypeTask, Content: "Task: finish deployment"})
	memSvc.Save(sess, memory.Entry{Type: memory.TypeInstruction, Content: "Instruction: ask before destructive commands"})

	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MemoryService:    memSvc,
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if len(client.lastRequest.Context.MemoryByType[memory.TypeTask]) != 1 {
		t.Fatalf("typed task memories = %#v", client.lastRequest.Context.MemoryByType)
	}
	if len(client.lastRequest.Context.MemoryByType[memory.TypeInstruction]) != 1 {
		t.Fatalf("typed instruction memories = %#v", client.lastRequest.Context.MemoryByType)
	}
}

func TestRunnerHandleUserMessagePassesConfiguredMainLoopModelIntoGenerateRequest(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "use configured model")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-opus-4-6",
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if client.lastRequest.Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want %q", client.lastRequest.Model, "claude-opus-4-6")
	}
}

func TestRunnerHandleUserMessageUsesThirdPartyDefaultSonnetForHaikuInPlanMode(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "use provider-sensitive sonnet default")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "haiku",
		LLMProvider:   "openai-compatible",
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if client.lastRequest.Model != "claude-sonnet-4-5" {
		t.Fatalf("request model = %q, want provider-sensitive sonnet fallback", client.lastRequest.Model)
	}
}

func TestRunnerSetSessionMainLoopModelOverridePersistsAndAffectsQueries(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "use session model override")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})

	if err := runner.SetSessionMainLoopModelOverride(sess.ID, "claude-opus-4-6"); err != nil {
		t.Fatalf("set session main loop model override: %v", err)
	}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if client.lastRequest.Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want session override", client.lastRequest.Model)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.MainLoopModelOverride != "claude-opus-4-6" {
		t.Fatalf("metadata = %#v, want persisted session override", refreshed.Metadata)
	}
}

func TestRunnerClearSessionMainLoopModelOverrideFallsBackToBaseModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "fallback to base model")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})

	if err := runner.SetSessionMainLoopModelOverride(sess.ID, "claude-opus-4-6"); err != nil {
		t.Fatalf("set session main loop model override: %v", err)
	}
	if err := runner.ClearSessionMainLoopModelOverride(sess.ID); err != nil {
		t.Fatalf("clear session main loop model override: %v", err)
	}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if client.lastRequest.Model != "claude-sonnet-4-6" {
		t.Fatalf("request model = %q, want base model after clearing override", client.lastRequest.Model)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.MainLoopModelOverride != "" {
		t.Fatalf("metadata = %#v, want cleared session override", refreshed.Metadata)
	}
}

func TestRunnerExposesBaseOverrideAndResolvedMainLoopModelState(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})
	if err := runner.SetSessionMainLoopModelOverride(sess.ID, "claude-opus-4-6"); err != nil {
		t.Fatalf("set session main loop model override: %v", err)
	}

	if got := runner.BaseMainLoopModelForSession(sess.ID); got != "claude-sonnet-4-6" {
		t.Fatalf("base main loop model = %q, want base model", got)
	}
	if got := runner.SessionMainLoopModelOverride(sess.ID); got != "claude-opus-4-6" {
		t.Fatalf("session model override = %q, want override model", got)
	}
	if got := runner.ResolvedMainLoopModelForSession(sess.ID); got != "claude-opus-4-6" {
		t.Fatalf("resolved main loop model = %q, want resolved override model", got)
	}
}

func TestRunnerBaseMainLoopModelForSessionLatchesInitialModelWithoutQuery(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})

	if got := runner.BaseMainLoopModelForSession(sess.ID); got != "claude-sonnet-4-6" {
		t.Fatalf("base main loop model = %q, want base model", got)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.InitialMainLoopModel != "claude-sonnet-4-6" {
		t.Fatalf("metadata = %#v, want initial model latched without query", refreshed.Metadata)
	}
}

func TestRunnerBaseMainLoopModelForSessionResolvesConfiguredAliasWithoutQuery(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "sonnet",
	})

	if got := runner.BaseMainLoopModelForSession(sess.ID); got != "claude-sonnet-4-6" {
		t.Fatalf("base main loop model = %q, want resolved default sonnet model", got)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.InitialMainLoopModel != "claude-sonnet-4-6" {
		t.Fatalf("metadata = %#v, want resolved initial model latched without query", refreshed.Metadata)
	}
}

func TestRunnerSpawnSubagentUsesDerivedPermissionPolicy(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{
			Mode:         permissions.ModeDangerFullAccess,
			SubagentMode: permissions.ModeAsk,
		},
	})

	run, err := runner.SpawnSubagent(context.Background(), parent, "restricted", "tool run pwd")
	if err != nil {
		t.Fatalf("spawn subagent: %v", err)
	}

	result, err := runner.AgentManager().Wait(context.Background(), run.ID, 5*time.Second)
	if err != nil {
		t.Fatalf("wait subagent: %v", err)
	}
	if result.Status != agent.StatusFailed {
		t.Fatalf("subagent status = %q, want %q", result.Status, agent.StatusFailed)
	}
}

func TestRunnerHandleUserMessageUsesSessionOverridePolicy(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	runner.SetSessionPermissionPolicy(sess.ID, permissions.Policy{Mode: permissions.ModeAsk}, false)

	err = runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{})
	if err == nil {
		t.Fatal("expected session-specific policy to deny tool execution")
	}
}

func TestRunnerHandleUserMessageCreatesApprovalRequestWhenPermissionRequired(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	approvalManager := approval.NewManager()
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		ApprovalManager:  approvalManager,
	})

	sink := &captureSink{}
	err = runner.HandleUserMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected permission denial to return error")
	}

	items := approvalManager.ListBySession(sess.ID)
	if len(items) != 1 {
		t.Fatalf("approval request count = %d, want 1", len(items))
	}
	if items[0].ToolName != "system.run" {
		t.Fatalf("approval tool = %q, want system.run", items[0].ToolName)
	}
	found := false
	for _, event := range sink.events {
		if event.Type == "permission.required" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected permission.required event")
	}
}

func TestRunnerApproveRequestContinuesBlockedToolExecution(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool upper hello world")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	approvalManager := approval.NewManager()
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{
				{
					ToolName: "text.upper",
					Action:   permissions.ActionAsk,
					Match: permissions.Match{
						CommandContains: []string{"hello world"},
					},
				},
			},
		},
		ApprovalManager: approvalManager,
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial execution to require approval")
	}

	items := approvalManager.ListBySession(sess.ID)
	if len(items) != 1 {
		t.Fatalf("approval request count = %d, want 1", len(items))
	}

	sink := &captureSink{}
	if err := runner.ApproveAndContinue(context.Background(), items[0].ID, sink); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	foundTool := false
	foundAssistant := false
	for _, message := range messages {
		if message.Role == "tool" && message.Content == "text.upper: HELLO WORLD" {
			foundTool = true
		}
		if message.Role == "assistant" && message.Content == "Using tool result: text.upper: HELLO WORLD" {
			foundAssistant = true
		}
	}
	if !foundTool {
		t.Fatal("expected tool message after approval")
	}
	if !foundAssistant {
		t.Fatal("expected final assistant reply after approval")
	}
}

func TestRunnerSetSessionPermissionPolicyCascadeUpdatesExistingSubagentSessions(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{
			Mode:         permissions.ModeDangerFullAccess,
			SubagentMode: permissions.ModeWorkspaceWrite,
		},
	})

	run, err := runner.SpawnSubagent(context.Background(), parent, "child", "hello child")
	if err != nil {
		t.Fatalf("spawn subagent: %v", err)
	}
	if _, err := runner.AgentManager().Wait(context.Background(), run.ID, 5*time.Second); err != nil {
		t.Fatalf("wait subagent: %v", err)
	}

	before := runner.PermissionPolicyForSession(run.ChildSessionID)
	if before.Mode != permissions.ModeWorkspaceWrite {
		t.Fatalf("before cascade mode = %q, want %q", before.Mode, permissions.ModeWorkspaceWrite)
	}

	runner.SetSessionPermissionPolicy(parent.ID, permissions.Policy{Mode: permissions.ModeAsk}, true)

	after := runner.PermissionPolicyForSession(run.ChildSessionID)
	if after.Mode != permissions.ModeAsk {
		t.Fatalf("after cascade mode = %q, want %q", after.Mode, permissions.ModeAsk)
	}
}

func TestRunnerSetSessionPermissionPolicyWithoutCascadeKeepsExistingSubagentPolicy(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{
			Mode:         permissions.ModeDangerFullAccess,
			SubagentMode: permissions.ModeWorkspaceWrite,
		},
	})

	run, err := runner.SpawnSubagent(context.Background(), parent, "child", "hello child")
	if err != nil {
		t.Fatalf("spawn subagent: %v", err)
	}
	if _, err := runner.AgentManager().Wait(context.Background(), run.ID, 5*time.Second); err != nil {
		t.Fatalf("wait subagent: %v", err)
	}

	runner.SetSessionPermissionPolicy(parent.ID, permissions.Policy{Mode: permissions.ModeAsk}, false)

	childPolicy := runner.PermissionPolicyForSession(run.ChildSessionID)
	if childPolicy.Mode != permissions.ModeWorkspaceWrite {
		t.Fatalf("child policy mode = %q, want %q", childPolicy.Mode, permissions.ModeWorkspaceWrite)
	}
}

func TestRunnerEmitsEventsToOrchestrationHook(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool upper hello world")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	hook := &captureHook{}
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		Orchestrator:     hook,
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, nil); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if len(hook.events) == 0 {
		t.Fatal("expected orchestration hook events")
	}
	foundToolCalled := false
	foundMessageCreated := false
	for _, event := range hook.events {
		if event.Type == "tool.called" && event.ToolName == "text.upper" {
			foundToolCalled = true
		}
		if event.Type == "message.created" && event.SessionID == sess.ID {
			foundMessageCreated = true
		}
	}
	if !foundToolCalled {
		t.Fatal("expected tool.called orchestration event")
	}
	if !foundMessageCreated {
		t.Fatal("expected message.created orchestration event")
	}
}

func TestRunnerHandleUserMessageExecutesToolCallEncodedInAssistantText(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "what is the current directory?")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	client := &scriptedClient{
		responses: []string{
			strings.Join([]string{
				"<tool_call>",
				"name: system.run",
				"input: pwd",
				"</tool_call>",
			}, "\n"),
			"The current directory is shown above.",
		},
	}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	foundTool := false
	foundAssistant := false
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "system.run" && event.ToolInput == "pwd" {
			foundTool = true
		}
		if event.Type == "message.created" && event.Message != nil && event.Message.Role == "assistant" && event.Message.Content == "The current directory is shown above." {
			foundAssistant = true
		}
	}
	if !foundTool {
		t.Fatal("expected tool.called event for parsed tool call content")
	}
	if !foundAssistant {
		t.Fatal("expected final assistant reply after parsed tool call content")
	}
}

func TestRunnerHandleUserMessageExecutesToolCallEmbeddedInAssistantText(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "please uppercase hello world")
	if err != nil {
		t.Fatalf("append message: %v", err)
	}

	client := &scriptedClient{
		responses: []string{
			strings.Join([]string{
				"I'll use a tool for that.",
				"<tool_call>",
				"name: text.upper",
				"input: hello world",
				"</tool_call>",
			}, "\n"),
			"Final answer: HELLO WORLD",
		},
	}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	foundTool := false
	foundAssistant := false
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "text.upper" && event.ToolInput == "hello world" {
			foundTool = true
		}
		if event.Type == "message.created" && event.Message != nil && event.Message.Role == "assistant" && event.Message.Content == "Final answer: HELLO WORLD" {
			foundAssistant = true
		}
	}
	if !foundTool {
		t.Fatal("expected tool.called event for embedded tool call content")
	}
	if !foundAssistant {
		t.Fatal("expected final assistant reply after embedded tool call content")
	}
}

func TestRunnerResumeSubagentRejectsUnfinishedContinuationState(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")
	child := sessions.CreateChild(parent.AgentID, "agent:main:child:1")
	if _, err := sessions.AppendMessage(child.ID, "user", "unfinished prompt"); err != nil {
		t.Fatalf("append unfinished child message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	run, err := runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		ChildSessionID:  child.ID,
		ChildSessionKey: child.Key,
		Label:           "research",
		Prompt:          "first pass",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := runner.AgentManager().Wait(context.Background(), run.ID, 2*time.Second); err != nil {
		t.Fatalf("wait seeded run: %v", err)
	}

	_, err = runner.ResumeSubagent(context.Background(), run.ID, "research", "second pass")
	if err == nil {
		t.Fatal("expected resume to reject unfinished continuation state")
	}
	if !strings.Contains(err.Error(), "not ready for a new prompt") {
		t.Fatalf("error = %v, want continuation-state rejection", err)
	}
}

func TestRunnerResumeSubagentRejectsTrailingToolContinuationState(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")
	child := sessions.CreateChild(parent.AgentID, "agent:main:child:1")
	if _, err := sessions.AppendMessage(child.ID, "user", "run the tool"); err != nil {
		t.Fatalf("append child user message: %v", err)
	}
	if _, err := sessions.AppendMessage(child.ID, "tool", "tool result"); err != nil {
		t.Fatalf("append child tool message: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	run, err := runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		ChildSessionID:  child.ID,
		ChildSessionKey: child.Key,
		Label:           "research",
		Prompt:          "first pass",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := runner.AgentManager().Wait(context.Background(), run.ID, 2*time.Second); err != nil {
		t.Fatalf("wait seeded run: %v", err)
	}

	_, err = runner.ResumeSubagent(context.Background(), run.ID, "research", "second pass")
	if err == nil {
		t.Fatal("expected resume to reject trailing tool continuation state")
	}
	if !strings.Contains(err.Error(), "not ready for a new prompt") {
		t.Fatalf("error = %v, want continuation-state rejection", err)
	}
}

func TestRunnerResumeSubagentRejectsPendingApprovalContinuationState(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")
	child := sessions.CreateChild(parent.AgentID, "agent:main:child:1")
	if _, err := sessions.AppendMessage(child.ID, "assistant", "waiting for approval"); err != nil {
		t.Fatalf("append child assistant message: %v", err)
	}

	approvalManager := approval.NewManager()
	request := approvalManager.Create(child.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ApprovalManager:  approvalManager,
	})
	run, err := runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		ChildSessionID:  child.ID,
		ChildSessionKey: child.Key,
		Label:           "research",
		Prompt:          "first pass",
		Run: func(context.Context, agent.RunContext) (string, error) {
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := runner.AgentManager().Wait(context.Background(), run.ID, 2*time.Second); err != nil {
		t.Fatalf("wait seeded run: %v", err)
	}

	_, err = runner.ResumeSubagent(context.Background(), run.ID, "research", "second pass")
	if err == nil {
		t.Fatal("expected resume to reject pending approval continuation state")
	}
	if !strings.Contains(err.Error(), "pending approval") || !strings.Contains(err.Error(), request.ID) {
		t.Fatalf("error = %v, want pending approval rejection mentioning %q", err, request.ID)
	}
}

func TestRunnerResumeSubagentRejectsAlreadyRunningRun(t *testing.T) {
	sessions := session.NewManager(nil)
	parent := sessions.GetOrCreateMain("main")
	child := sessions.CreateChild(parent.AgentID, "agent:main:child:1")

	blocked := make(chan struct{})
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	run, err := runner.AgentManager().Spawn(context.Background(), agent.SpawnRequest{
		ParentSessionID: parent.ID,
		ParentAgentID:   parent.AgentID,
		ChildSessionID:  child.ID,
		ChildSessionKey: child.Key,
		Label:           "research",
		Prompt:          "first pass",
		Run: func(context.Context, agent.RunContext) (string, error) {
			<-blocked
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("seed running run: %v", err)
	}
	defer close(blocked)

	_, err = runner.ResumeSubagent(context.Background(), run.ID, "research", "second pass")
	if err == nil {
		t.Fatal("expected resume to reject already running subagent")
	}
	if !strings.Contains(err.Error(), "still running") || !strings.Contains(err.Error(), run.ID) {
		t.Fatalf("error = %v, want running-run rejection mentioning %q", err, run.ID)
	}
}

func TestRunnerUpdateApprovalStatusClearsPendingApprovalMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000001"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "system.run"
		metadata.PendingApprovalAcceptFeedback = "Explain why this command is needed"
		metadata.PendingApprovalContentBlocks = []map[string]any{{"type": "text", "text": "prompt block"}}
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}

	approvalManager := approval.NewManager()
	request := approvalManager.Create(sess.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ApprovalManager:  approvalManager,
	})

	updated, err := runner.UpdateApprovalStatus(request.ID, approval.StatusRejected)
	if err != nil {
		t.Fatalf("update approval status: %v", err)
	}
	if updated.Status != approval.StatusRejected {
		t.Fatalf("approval status = %q, want rejected", updated.Status)
	}

	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.PendingApprovalID != "" {
		t.Fatalf("metadata = %#v, want pending approval cleared", refreshed.Metadata)
	}
	if refreshed.Metadata.PendingApprovalStatus != "" {
		t.Fatalf("metadata = %#v, want pending approval status cleared", refreshed.Metadata)
	}
	if refreshed.Metadata.PendingApprovalAcceptFeedback != "" || refreshed.Metadata.PendingApprovalContentBlocks != nil {
		t.Fatalf("metadata = %#v, want pending approval prompt metadata cleared", refreshed.Metadata)
	}
}

func TestRunnerUpdateApprovalPromptMetadataPersistsPendingApprovalMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000001"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "system.run"
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}
	approvalManager := approval.NewManager()
	request := approvalManager.Create(sess.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ApprovalManager:  approvalManager,
	})

	updated, err := runner.UpdateApprovalPromptMetadata(request.ID, "approved with context", []map[string]any{{
		"type": "text",
		"text": "reviewer note",
	}})
	if err != nil {
		t.Fatalf("update approval prompt metadata: %v", err)
	}
	if updated.AcceptFeedback != "approved with context" {
		t.Fatalf("accept feedback = %q, want stored feedback", updated.AcceptFeedback)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.PendingApprovalAcceptFeedback != "approved with context" {
		t.Fatalf("metadata = %#v, want pending approval feedback stored", refreshed.Metadata)
	}
	if len(refreshed.Metadata.PendingApprovalContentBlocks) != 1 || refreshed.Metadata.PendingApprovalContentBlocks[0]["text"] != "reviewer note" {
		t.Fatalf("metadata = %#v, want pending approval content blocks stored", refreshed.Metadata)
	}
}

func TestNewRunnerWithOptionsRestoresPendingApprovalFromSessionMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000007"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "text.upper"
		metadata.PendingApprovalToolInput = "hello world"
		metadata.PendingApprovalToolUseID = "provider-tool-use-7"
		metadata.PendingApprovalProviderMsgID = "provider-message-7"
		metadata.PendingApprovalReason = "approval required"
		metadata.PendingApprovalRunID = "run-000123"
		metadata.PendingApprovalUserMessageID = "msg-000001"
		metadata.PendingApprovalCategory = "approval"
		metadata.PendingApprovalRuleSource = "session"
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	request, ok := runner.ApprovalManager().Get("approval-000007")
	if !ok {
		t.Fatal("expected restored pending approval")
	}
	if request.ToolName != "text.upper" || request.ToolInput != "hello world" {
		t.Fatalf("request = %#v, want restored tool metadata", request)
	}
	if request.ToolUseID != "provider-tool-use-7" || request.ProviderMessageID != "provider-message-7" {
		t.Fatalf("request = %#v, want restored provider tool identity", request)
	}
	if request.RunID != "run-000123" || request.UserMessageID != "msg-000001" {
		t.Fatalf("request = %#v, want restored run/user ids", request)
	}
}

func TestNewRunnerWithOptionsRestoresPendingApprovalObjectInputFromSessionMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000008"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "structured.echo"
		metadata.PendingApprovalToolInput = `{"command":"checked-structured","cwd":"/tmp"}`
		metadata.PendingApprovalToolInputObject = map[string]any{
			"command": "checked-structured",
			"cwd":     "/tmp",
		}
		metadata.PendingApprovalRunID = "run-000124"
		metadata.PendingApprovalUserMessageID = "msg-000002"
		metadata.PendingApprovalCategory = "approval"
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	request, ok := runner.ApprovalManager().Get("approval-000008")
	if !ok {
		t.Fatal("expected restored pending approval")
	}
	assertAnyMap(t, request.ToolInputObject, map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})
}

func TestNewRunnerWithOptionsRestoresPendingApprovalDecisionReasonFromSessionMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000010"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "text.upper"
		metadata.PendingApprovalToolInput = "hello world"
		metadata.PendingApprovalRunID = "run-000125"
		metadata.PendingApprovalUserMessageID = "msg-000003"
		metadata.PendingApprovalCategory = "approval"
		metadata.PendingApprovalReason = "hook wants a human"
		metadata.PendingApprovalDecisionReason = "hook wants a human"
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	request, ok := runner.ApprovalManager().Get("approval-000010")
	if !ok {
		t.Fatal("expected restored pending approval")
	}
	if request.DecisionReason != "hook wants a human" {
		t.Fatalf("request decision reason = %q, want restored decision reason", request.DecisionReason)
	}
}

func TestNewRunnerWithOptionsRestoresPendingApprovalPromptMetadataFromSessionMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000011"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "text.upper"
		metadata.PendingApprovalToolInput = "hello world"
		metadata.PendingApprovalRunID = "run-000126"
		metadata.PendingApprovalUserMessageID = "msg-000004"
		metadata.PendingApprovalCategory = "approval"
		metadata.PendingApprovalReason = "hook wants a human"
		metadata.PendingApprovalAcceptFeedback = "Explain why this command is needed"
		metadata.PendingApprovalContentBlocks = []map[string]any{
			{
				"type": "text",
				"text": "approval prompt block",
			},
		}
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	request, ok := runner.ApprovalManager().Get("approval-000011")
	if !ok {
		t.Fatal("expected restored pending approval")
	}
	if request.AcceptFeedback != "Explain why this command is needed" {
		t.Fatalf("request accept feedback = %q, want restored accept feedback", request.AcceptFeedback)
	}
	if len(request.ContentBlocks) != 1 {
		t.Fatalf("request content blocks = %#v, want one restored block", request.ContentBlocks)
	}
	assertAnyMap(t, request.ContentBlocks[0], map[string]any{
		"type": "text",
		"text": "approval prompt block",
	})
}

func TestNewRunnerWithOptionsRegistersClaudeCoreToolSurface(t *testing.T) {
	sessions := session.NewManager(nil)
	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "inspect tool surface")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range client.lastRequest.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"Bash", "PowerShell",
		"Read", "Write", "Edit", "MultiEdit", "Glob", "Grep", "LS", "TodoWrite",
		"Agent", "ToolSearch",
		"WebFetch", "WebSearch",
		"ListMcpResources", "ReadMcpResource",
		"Skill", "NotebookEdit",
		"EnterPlanMode", "ExitPlanMode",
	} {
		if !names[want] {
			t.Fatalf("default tools = %#v, missing Claude core tool %q", names, want)
		}
	}
}

func TestNewRunnerWithOptionsRegistersConfiguredMCPTools(t *testing.T) {
	sessions := session.NewManager(nil)
	client := &captureMemoryClient{}
	runner := NewRunnerWithOptions(sessions, client, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPTools: map[string]tools.MCPToolsListResult{
			"filesystem": {Tools: []tools.MCPToolListItem{{
				Name:        "read_file",
				Description: "Read a file",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
			}}},
		},
	})
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "inspect mcp tool surface")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	for _, tool := range client.lastRequest.Tools {
		if tool.Name == "mcp__filesystem__read_file" {
			properties, _ := tool.InputSchema["properties"].(map[string]any)
			if properties["path"] == nil {
				t.Fatalf("mcp tool schema = %#v, want configured schema", tool.InputSchema)
			}
			return
		}
	}
	t.Fatalf("tools = %#v, want configured MCP tool", client.lastRequest.Tools)
}

func TestNewRunnerWithOptionsDiscoversMCPClientToolsAndCallsServer(t *testing.T) {
	var calledTool bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := request["method"].(string)
		response := map[string]any{
			"jsonrpc": "2.0",
			"id":      request["id"],
		}
		switch method {
		case "initialize":
			response["result"] = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "filesystem", "version": "1.0.0"},
			}
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "tools/list":
			response["result"] = map[string]any{
				"tools": []any{map[string]any{
					"name":        "read_file",
					"description": "Read a file",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
						},
					},
				}},
			}
		case "tools/call":
			params, _ := request["params"].(map[string]any)
			if params["name"] != "read_file" {
				t.Fatalf("tool call params = %#v, want read_file", params)
			}
			args, _ := params["arguments"].(map[string]any)
			if args["path"] != "README.md" {
				t.Fatalf("tool call args = %#v, want README.md", args)
			}
			calledTool = true
			response["result"] = map[string]any{
				"content": []any{map[string]any{"type": "text", "text": "remote file contents"}},
			}
		default:
			t.Fatalf("unexpected MCP method %q", method)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke discovered mcp")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	runner := NewRunnerWithOptions(sessions, &scriptedClient{responses: []string{
		strings.Join([]string{
			"<tool_call>",
			"name: mcp__filesystem__read_file",
			"input: {\"path\":\"README.md\"}",
			"</tool_call>",
		}, "\n"),
		"done",
	}}, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MCPClients: []tools.MCPConnection{{
			Name:    "filesystem",
			Type:    "streamable_http",
			BaseURL: server.URL,
		}},
	})
	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if !calledTool {
		t.Fatal("expected MCP tools/call to be sent to discovered server")
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, "remote file contents") {
			return
		}
	}
	t.Fatalf("messages = %#v, want discovered MCP tool output", messages)
}

func TestNewRunnerWithOptionsInjectsSkillForkExecutor(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\n---\nVerify it."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke skill")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	called := false
	runner := NewRunnerWithOptions(sessions, &scriptedClient{responses: []string{
		strings.Join([]string{
			"<tool_call>",
			"name: Skill",
			"input: {\"skill\":\"verify\"}",
			"</tool_call>",
		}, "\n"),
		"done",
	}}, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		SkillRoots:       []string{root},
		SkillForkExecutor: func(_ context.Context, request tools.SkillForkRequest) (tools.ToolResult, error) {
			called = true
			if request.Command.Name != "verify" || request.Command.Agent != "verifier" {
				t.Fatalf("request = %#v, want verify verifier skill", request)
			}
			return tools.ToolResult{Output: `{"status":"forked","agent":"verifier","result":"runner executed"}`}, nil
		},
	})
	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if !called {
		t.Fatal("expected SkillForkExecutor to be called")
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, "runner executed") {
			return
		}
	}
	t.Fatalf("messages = %#v, want fork executor output", messages)
}

func TestNewRunnerWithOptionsDefaultsSkillForkToSubagentRuntime(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\n---\nVerify it."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "invoke skill")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	runner := NewRunnerWithOptions(sessions, &scriptedClient{responses: []string{
		strings.Join([]string{
			"<tool_call>",
			"name: Skill",
			"input: {\"skill\":\"verify\"}",
			"</tool_call>",
		}, "\n"),
		"subagent verified",
		"done",
	}}, workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		SkillRoots:       []string{root},
	})
	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("handle user message: %v", err)
	}
	if len(runner.AgentManager().List()) != 1 {
		t.Fatalf("runs = %#v, want one forked skill subagent run", runner.AgentManager().List())
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, "subagent verified") {
			return
		}
	}
	t.Fatalf("messages = %#v, want forked subagent result", messages)
}

func TestRunnerApproveAndContinueWorksWithRestoredPendingApproval(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	user, err := sessions.AppendMessage(sess.ID, "user", "please uppercase hello world")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000009"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "text.upper"
		metadata.PendingApprovalToolInput = "hello world"
		metadata.PendingApprovalToolUseID = "provider-tool-use-9"
		metadata.PendingApprovalProviderMsgID = "provider-message-9"
		metadata.PendingApprovalReason = "approval required"
		metadata.PendingApprovalAcceptFeedback = "approved with reviewer note"
		metadata.PendingApprovalContentBlocks = []map[string]any{
			{
				"type": "text",
				"text": "extra approval block",
			},
		}
		metadata.PendingApprovalRunID = "run-000321"
		metadata.PendingApprovalUserMessageID = user.ID
		metadata.PendingApprovalCategory = "approval"
		metadata.PendingApprovalRuleSource = "session"
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	sink := &captureSink{}
	if err := runner.ApproveAndContinue(context.Background(), "approval-000009", sink); err != nil {
		t.Fatalf("approve and continue restored approval: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "Using tool result: text.upper: HELLO WORLD") {
		t.Fatalf("last message = %#v, want assistant continuation from restored approval", last)
	}
	var toolMessage session.Message
	for _, message := range messages {
		if message.Role == "tool" {
			toolMessage = message
			break
		}
	}
	if toolMessage.ID == "" {
		t.Fatalf("messages = %#v, want tool result message", messages)
	}
	if len(toolMessage.Blocks) != 3 {
		t.Fatalf("tool message blocks = %#v, want tool result plus approval feedback blocks", toolMessage.Blocks)
	}
	if toolMessage.Blocks[0].Type != model.MessageBlockToolResult || toolMessage.Blocks[0].Content != "HELLO WORLD" {
		t.Fatalf("tool result block = %#v, want tool result content", toolMessage.Blocks[0])
	}
	if toolMessage.Blocks[1].Type != model.MessageBlockText || toolMessage.Blocks[1].Text != "approved with reviewer note" {
		t.Fatalf("approval feedback block = %#v, want text feedback block", toolMessage.Blocks[1])
	}
	if toolMessage.Blocks[2].Type != model.MessageBlockText || toolMessage.Blocks[2].Text != "extra approval block" {
		t.Fatalf("approval content block = %#v, want extra content block", toolMessage.Blocks[2])
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.PendingApprovalID != "" || refreshed.Metadata.PendingApprovalStatus != "" {
		t.Fatalf("metadata = %#v, want restored pending approval metadata cleared after approval", refreshed.Metadata)
	}
	if refreshed.Metadata.PendingApprovalToolUseID != "" || refreshed.Metadata.PendingApprovalProviderMsgID != "" {
		t.Fatalf("metadata = %#v, want restored provider tool identity cleared after approval", refreshed.Metadata)
	}
}

func TestRunnerRejectAndContinueWithFeedbackReturnsErrorToolResultToModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	user, err := sessions.AppendMessage(sess.ID, "user", "please uppercase hello world")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.PendingApprovalID = "approval-000012"
		metadata.PendingApprovalStatus = string(approval.StatusPending)
		metadata.PendingApprovalToolName = "text.upper"
		metadata.PendingApprovalToolInput = "hello world"
		metadata.PendingApprovalToolUseID = "provider-tool-use-12"
		metadata.PendingApprovalProviderMsgID = "provider-message-12"
		metadata.PendingApprovalReason = "approval required"
		metadata.PendingApprovalRunID = "run-000322"
		metadata.PendingApprovalUserMessageID = user.ID
		metadata.PendingApprovalCategory = "approval"
	}); err != nil {
		t.Fatalf("seed pending approval metadata: %v", err)
	}
	approvalManager := approval.NewManager()
	approvalManager.Restore(approval.Request{
		ID:                "approval-000012",
		SessionID:         sess.ID,
		RunID:             "run-000322",
		UserMessageID:     user.ID,
		ToolName:          "text.upper",
		ToolInput:         "hello world",
		ToolUseID:         "provider-tool-use-12",
		ProviderMessageID: "provider-message-12",
		Reason:            "approval required",
		Category:          "approval",
		Status:            approval.StatusPending,
	})
	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ApprovalManager:  approvalManager,
	})

	sink := &captureSink{}
	if err := runner.RejectAndContinue(context.Background(), "approval-000012", "try a safer command", []map[string]any{{
		"type": "text",
		"text": "additional reject block",
	}}, sink); err != nil {
		t.Fatalf("reject and continue: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	var toolMessage session.Message
	for _, message := range messages {
		if message.Role == "tool" {
			toolMessage = message
			break
		}
	}
	if toolMessage.ID == "" {
		t.Fatalf("messages = %#v, want rejected tool result message", messages)
	}
	if len(toolMessage.Blocks) != 2 {
		t.Fatalf("tool message blocks = %#v, want error tool result plus reject content block", toolMessage.Blocks)
	}
	if toolMessage.Blocks[0].Type != model.MessageBlockToolResult || !toolMessage.Blocks[0].IsError {
		t.Fatalf("tool result block = %#v, want error tool result", toolMessage.Blocks[0])
	}
	if !strings.Contains(toolMessage.Blocks[0].Content, "try a safer command") {
		t.Fatalf("tool result block = %#v, want reject feedback in error message", toolMessage.Blocks[0])
	}
	if toolMessage.Blocks[1].Type != model.MessageBlockText || toolMessage.Blocks[1].Text != "additional reject block" {
		t.Fatalf("reject content block = %#v, want appended reject content block", toolMessage.Blocks[1])
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "Using tool result:") {
		t.Fatalf("last message = %#v, want assistant continuation from rejection tool result", last)
	}
	refreshed, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if refreshed.Metadata.PendingApprovalID != "" || refreshed.Metadata.PendingApprovalStatus != "" {
		t.Fatalf("metadata = %#v, want rejected pending approval metadata cleared", refreshed.Metadata)
	}
}

func assertAnyMap(t *testing.T, got, want map[string]any) {
	t.Helper()

	if got == nil {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
	if len(got) != len(want) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("input[%q] = %#v, want %#v in %#v", key, got[key], wantValue, got)
		}
	}
}
