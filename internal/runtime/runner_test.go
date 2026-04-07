package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/orchestration"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
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
	err = runner.HandleUserMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected deny rule to block the command")
	}
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
					Action:   permissions.ActionDeny,
				},
			},
		},
		ApprovalManager: approvalManager,
	})

	if err := runner.HandleUserMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial denied execution")
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
