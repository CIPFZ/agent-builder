package queryengine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"myclaw/internal/approval"
	"myclaw/internal/compaction"
	"myclaw/internal/llm"
	"myclaw/internal/memory"
	"myclaw/internal/model"
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
	err    error
}

type permissionRewritingToolForQueryEngine struct {
	stubToolForQueryEngine
	updatedInput       string
	updatedInputObject map[string]any
	requiresApproval   bool
	category           permissions.Category
	invocations        *[]string
}

type capturingToolForQueryEngine struct {
	stubToolForQueryEngine
	invocations *[]string
}

type structuredCapturingToolForQueryEngine struct {
	stubToolForQueryEngine
	permissionInputs *[]map[string]any
	invocations      *[]map[string]any
}

type observableBackfillToolForQueryEngine struct {
	stubToolForQueryEngine
	invocations *[]map[string]any
}

type contextualPermissionToolForQueryEngine struct {
	stubToolForQueryEngine
	gotContext *tools.ToolUseContext
}

type contextualExecutionToolForQueryEngine struct {
	stubToolForQueryEngine
	gotContexts        *[]tools.ToolUseContext
	canUseToolRequest  *tools.CanUseToolRequest
	canUseToolDecision *permissions.Decision
}

type autoClassifierToolForQueryEngine struct {
	stubToolForQueryEngine
}

type permissionHookFunc func(context.Context, queryengine.PermissionHookRequest) (permissions.Decision, bool, error)

func (f permissionHookFunc) CheckPermission(ctx context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
	return f(ctx, request)
}

type preToolUseHookFunc func(context.Context, queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error)

func (f preToolUseHookFunc) BeforeToolUse(ctx context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
	return f(ctx, request)
}

type postToolUseHookFunc func(context.Context, queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error)

func (f postToolUseHookFunc) AfterToolUse(ctx context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
	return f(ctx, request)
}

type postToolUseFailureHookFunc func(context.Context, queryengine.PostToolUseFailureHookRequest) (queryengine.PostToolUseFailureHookResult, bool, error)

func (f postToolUseFailureHookFunc) AfterToolUseFailure(ctx context.Context, request queryengine.PostToolUseFailureHookRequest) (queryengine.PostToolUseFailureHookResult, bool, error) {
	return f(ctx, request)
}

type sessionStartCompactHookFunc func(context.Context, session.Session) ([]session.Message, error)

func (f sessionStartCompactHookFunc) ProcessSessionStartCompact(ctx context.Context, sess session.Session) ([]session.Message, error) {
	return f(ctx, sess)
}

type permissionUpdatePersisterFunc func(context.Context, session.Session, []permissions.PermissionUpdate) error

func (f permissionUpdatePersisterFunc) PersistPermissionUpdates(ctx context.Context, sess session.Session, updates []permissions.PermissionUpdate) error {
	return f(ctx, sess, updates)
}

func (t stubToolForQueryEngine) Definition() tools.Definition {
	return t.def
}

func (t stubToolForQueryEngine) Invoke(_ context.Context, _ session.Session, _ string) (string, error) {
	return "ok", nil
}

func (t scriptedToolForQueryEngine) Invoke(_ context.Context, _ session.Session, _ string) (string, error) {
	if t.err != nil {
		return "", t.err
	}
	if t.output != "" {
		return t.output, nil
	}
	return "ok", nil
}

func (t permissionRewritingToolForQueryEngine) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	if t.invocations != nil {
		*t.invocations = append(*t.invocations, input)
	}
	return input, nil
}

func (t permissionRewritingToolForQueryEngine) CheckPermissions(_ context.Context, _ session.Session, _ string, _ permissions.Policy) (permissions.Decision, error) {
	return permissions.Decision{
		Allowed:            !t.requiresApproval,
		RequiresApproval:   t.requiresApproval,
		Category:           t.category,
		UpdatedInput:       t.updatedInput,
		UpdatedInputObject: t.updatedInputObject,
		Reason:             "tool permission check returned updated input",
	}, nil
}

func (t capturingToolForQueryEngine) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	if t.invocations != nil {
		*t.invocations = append(*t.invocations, input)
	}
	return input, nil
}

func (t structuredCapturingToolForQueryEngine) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return input, nil
}

func (t structuredCapturingToolForQueryEngine) InvokeWithInput(_ context.Context, _ session.Session, input map[string]any) (string, error) {
	if t.invocations != nil {
		*t.invocations = append(*t.invocations, cloneAnyMap(input))
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (t structuredCapturingToolForQueryEngine) CheckPermissionsWithInput(_ context.Context, _ session.Session, input map[string]any, _ permissions.Policy) (permissions.Decision, error) {
	if t.permissionInputs != nil {
		*t.permissionInputs = append(*t.permissionInputs, cloneAnyMap(input))
	}
	updated := cloneAnyMap(input)
	updated["command"] = "checked-structured"
	return permissions.Decision{
		Allowed:            true,
		UpdatedInputObject: updated,
		Reason:             "structured permission check",
	}, nil
}

func (t observableBackfillToolForQueryEngine) BackfillObservableInput(input map[string]any) {
	input["command"] = "overwritten-for-display"
	input["display_command"] = "checked-structured --display"
}

func (t observableBackfillToolForQueryEngine) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return input, nil
}

func (t observableBackfillToolForQueryEngine) InvokeWithInput(_ context.Context, _ session.Session, input map[string]any) (string, error) {
	if t.invocations != nil {
		*t.invocations = append(*t.invocations, cloneAnyMap(input))
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (t contextualPermissionToolForQueryEngine) CheckPermissionsWithContext(_ context.Context, toolCtx tools.ToolUseContext) (permissions.Decision, error) {
	if t.gotContext != nil {
		toolCtx.SetAppState(func(previous map[string]any) map[string]any {
			next := make(map[string]any, len(previous)+1)
			for key, value := range previous {
				next[key] = value
			}
			next["contextual_tool_seen"] = toolCtx.ToolName
			return next
		})
		if toolCtx.ToolDecisions != nil {
			toolCtx.ToolDecisions[toolCtx.ToolName] = tools.ToolDecision{Source: "tool", Decision: "accept", TimestampUnixMilli: 456}
		}
		_, _ = toolCtx.RequestPrompt("contextual.tool", "inspect", tools.PromptRequest{
			Message: "confirm contextual permission",
			Options: []string{"yes", "no"},
		})
		toolCtx.ReportProgress(tools.ToolProgress{ToolUseID: "toolu-context", Type: "hook_progress", Message: "contextual check"})
		cloned := toolCtx
		cloned.InputObject = cloneAnyMap(toolCtx.InputObject)
		cloned.AvailableTools = append([]tools.Definition(nil), toolCtx.AvailableTools...)
		cloned.Messages = append([]session.Message(nil), toolCtx.Messages...)
		*t.gotContext = cloned
	}
	return permissions.Decision{Allowed: true}, nil
}

func (t contextualExecutionToolForQueryEngine) InvokeWithContext(_ context.Context, toolCtx tools.ToolUseContext) (tools.ToolResult, error) {
	if t.gotContexts != nil {
		cloned := toolCtx
		cloned.InputObject = cloneAnyMap(toolCtx.InputObject)
		cloned.AvailableTools = append([]tools.Definition(nil), toolCtx.AvailableTools...)
		cloned.Messages = append([]session.Message(nil), toolCtx.Messages...)
		cloned.AppState = cloneAnyMap(toolCtx.AppState)
		*t.gotContexts = append(*t.gotContexts, cloned)
	}
	if t.canUseToolRequest != nil {
		decision, err := toolCtx.CanUseTool(toolCtx.AbortContext, *t.canUseToolRequest)
		if err != nil {
			return tools.ToolResult{}, err
		}
		if t.canUseToolDecision != nil {
			*t.canUseToolDecision = decision
		}
	}
	toolCtx.ReportProgress(tools.ToolProgress{ToolUseID: toolCtx.ToolUseID, Type: "tool_progress", Message: "executing " + toolCtx.ToolName})
	return tools.ToolResult{
		Output: "output from " + toolCtx.ToolName,
		ContextModifier: func(next tools.ToolUseContext) tools.ToolUseContext {
			if next.AppState == nil {
				next.AppState = make(map[string]any)
			}
			next.AppState["last_tool"] = toolCtx.ToolName
			return next
		},
	}, nil
}

func (t autoClassifierToolForQueryEngine) ToAutoClassifierInput(input string) any {
	return "classifier:" + input
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

func TestQueryEngineToolExecutionPersistsLinkedToolUseAndResultBlocks(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool upper hello world")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var toolUseID string
	for _, message := range messages {
		if message.Role != "assistant" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockToolUse && block.Name == "text.upper" && block.Input == "hello world" {
				toolUseID = block.ID
			}
		}
	}
	if toolUseID == "" {
		t.Fatalf("messages = %#v, want assistant tool_use block for text.upper", messages)
	}

	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockToolResult && block.ToolUseID == toolUseID && block.Content == "HELLO WORLD" {
				return
			}
		}
	}
	t.Fatalf("messages = %#v, want tool_result block linked to tool_use_id %q", messages, toolUseID)
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
	updated, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if updated.Metadata.PendingApprovalID != requests[0].ID {
		t.Fatalf("metadata = %#v, want pending approval id %q", updated.Metadata, requests[0].ID)
	}
	if updated.Metadata.PendingApprovalStatus != string(approval.StatusPending) {
		t.Fatalf("metadata = %#v, want pending approval status", updated.Metadata)
	}
}

func TestQueryEngineSubmitMessageTreatsDenyRuleAsFinalDenial(t *testing.T) {
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
					Match: permissions.Match{
						CommandContains: []string{"pwd"},
					},
				},
			},
		},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 0 {
		t.Fatalf("approval count = %d, want 0 for final deny rule", len(requests))
	}
	var toolResult *session.Message
	for _, event := range sink.events {
		if event.Type == "tool.result" {
			toolResult = event.Message
			break
		}
	}
	if toolResult == nil {
		t.Fatalf("events = %#v, want denied tool result", sink.events)
	}
	if len(toolResult.Blocks) == 0 || !toolResult.Blocks[0].IsError {
		t.Fatalf("tool result blocks = %#v, want error tool result", toolResult.Blocks)
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

func TestQueryEngineApproveAndContinuePreservesProviderToolUseIdentity(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "tool run pwd")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{
					Type:              "tool.call",
					ToolName:          "system.run",
					ToolInput:         "pwd",
					ToolUseID:         "provider-tool-use-1",
					ProviderMessageID: "provider-message-1",
				},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
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
	if requests[0].ToolUseID != "provider-tool-use-1" {
		t.Fatalf("approval tool use id = %q, want provider-tool-use-1", requests[0].ToolUseID)
	}
	if requests[0].ProviderMessageID != "provider-message-1" {
		t.Fatalf("approval provider message id = %q, want provider-message-1", requests[0].ProviderMessageID)
	}

	if err := engine.ApproveAndContinue(context.Background(), requests[0].ID, &captureSink{}); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var foundToolUse bool
	var foundToolResult bool
	for _, message := range messages {
		for _, block := range message.Blocks {
			if message.Role == "assistant" &&
				message.ProviderMessageID == "provider-message-1" &&
				block.Type == model.MessageBlockToolUse &&
				block.ID == "provider-tool-use-1" &&
				block.Name == "system.run" &&
				block.Input == "pwd" {
				foundToolUse = true
			}
			if message.Role == "tool" &&
				block.Type == model.MessageBlockToolResult &&
				block.ToolUseID == "provider-tool-use-1" &&
				block.Content == "C:\\approved\\pwd" {
				foundToolResult = true
			}
		}
	}
	if !foundToolUse {
		t.Fatalf("messages = %#v, want assistant tool_use block with provider identity", messages)
	}
	if !foundToolResult {
		t.Fatalf("messages = %#v, want tool_result block linked to provider tool_use_id", messages)
	}
}

func TestQueryEngineToolPermissionUpdatedInputDrivesExecutionTranscriptAndEvents(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "rewrite tool input")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput: "rewritten",
			invocations:  &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(invocations) != 1 || invocations[0] != "rewritten" {
		t.Fatalf("invocations = %#v, want exactly rewritten input", invocations)
	}
	var sawToolCalled bool
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "rewrite.echo" {
			sawToolCalled = true
			if event.ToolInput != "rewritten" {
				t.Fatalf("tool.called input = %q, want rewritten", event.ToolInput)
			}
		}
	}
	if !sawToolCalled {
		t.Fatalf("events = %#v, want tool.called for rewrite.echo", sink.events)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var foundToolUse bool
	var foundToolResult bool
	for _, message := range messages {
		for _, block := range message.Blocks {
			if message.Role == "assistant" &&
				block.Type == model.MessageBlockToolUse &&
				block.Name == "rewrite.echo" &&
				block.Input == "rewritten" {
				foundToolUse = true
			}
			if message.Role == "tool" &&
				block.Type == model.MessageBlockToolResult &&
				block.Content == "rewritten" {
				foundToolResult = true
			}
		}
	}
	if !foundToolUse {
		t.Fatalf("messages = %#v, want tool_use block with updated input", messages)
	}
	if !foundToolResult {
		t.Fatalf("messages = %#v, want tool_result from updated input", messages)
	}
}

func TestQueryEngineToolPermissionStructuredUpdatedInputDrivesExecutionTranscriptAndEvents(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "rewrite structured tool input")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: `{"command":"original","cwd":"/tmp"}`},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInputObject: map[string]any{
				"command": "rewritten",
				"cwd":     "/workspace",
			},
			invocations: &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want exactly one structured invocation", invocations)
	}
	assertJSONInput(t, invocations[0], map[string]any{
		"command": "rewritten",
		"cwd":     "/workspace",
	})

	var sawToolCalled bool
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "rewrite.echo" {
			sawToolCalled = true
			assertJSONInput(t, event.ToolInput, map[string]any{
				"command": "rewritten",
				"cwd":     "/workspace",
			})
		}
	}
	if !sawToolCalled {
		t.Fatalf("events = %#v, want tool.called for rewrite.echo", sink.events)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var foundToolUse bool
	for _, message := range messages {
		for _, block := range message.Blocks {
			if message.Role == "assistant" &&
				block.Type == model.MessageBlockToolUse &&
				block.Name == "rewrite.echo" {
				assertJSONInput(t, block.Input, map[string]any{
					"command": "rewritten",
					"cwd":     "/workspace",
				})
				foundToolUse = true
			}
		}
	}
	if !foundToolUse {
		t.Fatalf("messages = %#v, want tool_use block with structured updated input", messages)
	}
}

func TestQueryEngineStructuredToolReceivesParsedInputForPermissionAndInvoke(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run structured parsed tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "structured.echo", ToolInput: `{"command":"original","cwd":"/tmp"}`},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var permissionInputs []map[string]any
	var invocations []map[string]any
	registry := tools.NewRegistry(
		structuredCapturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input.", Enabled: true},
				enabled: true,
			},
			permissionInputs: &permissionInputs,
			invocations:      &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(permissionInputs) != 1 {
		t.Fatalf("permission inputs = %#v, want one parsed input", permissionInputs)
	}
	assertAnyMap(t, permissionInputs[0], map[string]any{
		"command": "original",
		"cwd":     "/tmp",
	})
	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want one parsed invocation", invocations)
	}
	assertAnyMap(t, invocations[0], map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})
}

func TestQueryEnginePreToolUseUpdatedInputRunsBeforePermissionChecks(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook rewritten structured tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "structured.echo", ToolInputObject: map[string]any{
					"command": "original",
					"cwd":     "/tmp",
				}},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var preHookInputs []map[string]any
	var permissionInputs []map[string]any
	var invocations []map[string]any
	registry := tools.NewRegistry(
		structuredCapturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input.", Enabled: true},
				enabled: true,
			},
			permissionInputs: &permissionInputs,
			invocations:      &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			preHookInputs = append(preHookInputs, cloneAnyMap(request.ToolInputObject))
			return queryengine.PreToolUseHookResult{
				UpdatedInputObject: map[string]any{
					"command": "pre-rewritten",
					"cwd":     "/workspace",
				},
			}, true, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(preHookInputs) != 1 {
		t.Fatalf("pre hook inputs = %#v, want one input", preHookInputs)
	}
	assertAnyMap(t, preHookInputs[0], map[string]any{
		"command": "original",
		"cwd":     "/tmp",
	})
	if len(permissionInputs) != 1 {
		t.Fatalf("permission inputs = %#v, want one input", permissionInputs)
	}
	assertAnyMap(t, permissionInputs[0], map[string]any{
		"command": "pre-rewritten",
		"cwd":     "/workspace",
	})
	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want one invocation", invocations)
	}
	assertAnyMap(t, invocations[0], map[string]any{
		"command": "checked-structured",
		"cwd":     "/workspace",
	})

	var sawToolCalled bool
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "structured.echo" {
			sawToolCalled = true
			assertAnyMap(t, event.ToolInputObject, map[string]any{
				"command": "checked-structured",
				"cwd":     "/workspace",
			})
		}
	}
	if !sawToolCalled {
		t.Fatalf("events = %#v, want tool.called for structured.echo", sink.events)
	}
}

func TestQueryEnginePreToolUseDenyContinuesWithErrorToolResult(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook denied tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original", ToolUseID: "toolu-pre-deny"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "handled denial"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		capturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			if request.ToolInput != "original" {
				t.Fatalf("pre hook input = %q, want original", request.ToolInput)
			}
			return queryengine.PreToolUseHookResult{
				HasPermissionDecision: true,
				PermissionDecision: permissions.Decision{
					Allowed:          false,
					RequiresApproval: false,
					Reason:           "blocked by PreToolUse hook",
					DecisionReason: permissions.DecisionReason{
						Type:       permissions.DecisionReasonHook,
						HookName:   "PreToolUse:rewrite.echo",
						HookSource: "test",
						Reason:     "blocked by PreToolUse hook",
					},
				},
			}, true, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(invocations) != 0 {
		t.Fatalf("invocations = %#v, want denied PreToolUse hook to skip tool execution", invocations)
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var sawErrorToolResult bool
	for _, message := range messages {
		for _, block := range message.Blocks {
			if message.Role == "tool" &&
				block.Type == model.MessageBlockToolResult &&
				block.ToolUseID == "toolu-pre-deny" &&
				block.IsError &&
				block.Content == "blocked by PreToolUse hook" {
				sawErrorToolResult = true
			}
		}
	}
	if !sawErrorToolResult {
		t.Fatalf("messages = %#v, want PreToolUse deny error tool_result", messages)
	}
	for _, event := range sink.events {
		if event.Type == "run.error" {
			t.Fatalf("events = %#v, want PreToolUse denial to continue without run.error", sink.events)
		}
	}
}

func TestQueryEnginePreToolUseBlockingErrorDeniesWithClaudeHookMessage(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook blocking tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original", ToolUseID: "toolu-pre-block"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "handled blocking"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		capturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			return queryengine.PreToolUseHookResult{BlockingError: "pre hook blocked"}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if len(invocations) != 0 {
		t.Fatalf("invocations = %#v, want blocking PreToolUse hook to skip tool execution", invocations)
	}
	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockToolResult &&
				block.ToolUseID == "toolu-pre-block" &&
				block.IsError &&
				strings.Contains(block.Content, "pre hook blocked") {
				return
			}
		}
	}
	t.Fatalf("messages = %#v, want PreToolUse blocking error tool_result", messages)
}

func TestQueryEnginePreToolUseAllowBypassesToolAskButKeepsUpdatedInput(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook allowed tool ask")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput:     "tool-ask-updated-input",
			requiresApproval: true,
			category:         permissions.CategoryApproval,
			invocations:      &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			if request.ToolInput != "original" {
				t.Fatalf("pre hook input = %q, want original", request.ToolInput)
			}
			return queryengine.PreToolUseHookResult{
				HasPermissionDecision: true,
				PermissionDecision: permissions.Decision{
					Allowed: true,
					Reason:  "allowed by PreToolUse hook",
					DecisionReason: permissions.DecisionReason{
						Type:     permissions.DecisionReasonHook,
						HookName: "PreToolUse:rewrite.echo",
						Reason:   "allowed by PreToolUse hook",
					},
				},
				UpdatedInput: "pre-allow-updated-input",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if approvals := approvalManager.ListBySession(sess.ID); len(approvals) != 0 {
		t.Fatalf("approvals = %#v, want PreToolUse allow to bypass tool-level ask", approvals)
	}
	if len(invocations) != 1 || invocations[0] != "pre-allow-updated-input" {
		t.Fatalf("invocations = %#v, want PreToolUse allow updated input", invocations)
	}
}

func TestQueryEnginePreToolUseAskUsesUpdatedInputForApproval(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook ask")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{{
			{Type: "tool.call", ToolName: "rewrite.echo", ToolInputObject: map[string]any{
				"command": "original",
				"cwd":     "/tmp",
			}},
			{Type: "message.end"},
		}},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			assertAnyMap(t, request.ToolInputObject, map[string]any{
				"command": "original",
				"cwd":     "/tmp",
			})
			return queryengine.PreToolUseHookResult{
				HasPermissionDecision: true,
				PermissionDecision: permissions.Decision{
					RequiresApproval: true,
					Category:         permissions.CategoryApproval,
					Reason:           "PreToolUse hook wants a human",
					DecisionReason: permissions.DecisionReason{
						Type:       permissions.DecisionReasonHook,
						HookName:   "PreToolUse:rewrite.echo",
						HookSource: "test",
						Reason:     "PreToolUse hook wants a human",
					},
				},
				UpdatedInputObject: map[string]any{
					"command": "pre-ask-updated",
					"cwd":     "/workspace",
				},
			}, true, nil
		}),
	})

	sink := &captureSink{}
	err = engine.SubmitMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected PreToolUse ask to require approval")
	}
	if len(invocations) != 0 {
		t.Fatalf("invocations = %#v, want PreToolUse ask not to invoke before approval", invocations)
	}
	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}
	if requests[0].Reason != "PreToolUse hook wants a human" {
		t.Fatalf("approval reason = %q, want PreToolUse hook reason", requests[0].Reason)
	}
	assertAnyMap(t, requests[0].ToolInputObject, map[string]any{
		"command": "pre-ask-updated",
		"cwd":     "/workspace",
	})
	assertJSONInput(t, requests[0].ToolInput, map[string]any{
		"command": "pre-ask-updated",
		"cwd":     "/workspace",
	})
	for _, event := range sink.events {
		if event.Type == "permission.required" {
			assertJSONInput(t, event.ToolInput, map[string]any{
				"command": "pre-ask-updated",
				"cwd":     "/workspace",
			})
			if event.DecisionReason != "PreToolUse hook wants a human" {
				t.Fatalf("decision reason = %q, want PreToolUse hook reason", event.DecisionReason)
			}
			return
		}
	}
	t.Fatalf("events = %#v, want permission.required", sink.events)
}

func TestQueryEnginePreToolUseAddsAdditionalContextBlock(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook context tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-pre-context"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			if request.ToolName != "text.upper" || request.ToolInput != "hello" {
				t.Fatalf("pre hook request = %#v, want text.upper hello", request)
			}
			return queryengine.PreToolUseHookResult{
				AdditionalContexts: []string{"pre hook context"},
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw == nil ||
				block.Raw["type"] != "hook_additional_context" ||
				block.Raw["hookName"] != "PreToolUse:text.upper" ||
				block.Raw["toolUseID"] != "toolu-pre-context" ||
				block.Raw["hookEvent"] != "PreToolUse" {
				continue
			}
			contexts, ok := block.Raw["content"].([]string)
			if ok && len(contexts) == 1 && contexts[0] == "pre hook context" {
				return
			}
		}
	}
	t.Fatalf("messages = %#v, want PreToolUse additional context block", messages)
}

func TestQueryEnginePreToolUsePreventContinuationStopsNextModelPass(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook stop tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-pre-stop"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "should not be requested"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			return queryengine.PreToolUseHookResult{
				PreventContinuation: true,
				StopReason:          "stop after pre hook",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if got := len(client.Requests()); got != 1 {
		t.Fatalf("model request count = %d, want PreToolUse preventContinuation to stop before second model pass", got)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw == nil ||
				block.Raw["type"] != "hook_stopped_continuation" ||
				block.Raw["hookName"] != "PreToolUse:text.upper" ||
				block.Raw["toolUseID"] != "toolu-pre-stop" ||
				block.Raw["hookEvent"] != "PreToolUse" ||
				block.Raw["message"] != "stop after pre hook" {
				continue
			}
			return
		}
	}
	t.Fatalf("messages = %#v, want PreToolUse stopped continuation block", messages)
}

func TestQueryEnginePreToolUseAddsHookMessagesCancelledAndExecutionError(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run pre hook rich messages")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-pre-rich"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PreToolUseHook: preToolUseHookFunc(func(_ context.Context, request queryengine.PreToolUseHookRequest) (queryengine.PreToolUseHookResult, bool, error) {
			return queryengine.PreToolUseHookResult{
				HookMessages: []map[string]any{{
					"type":      "hook_progress",
					"hookName":  "PreToolUse:text.upper",
					"toolUseID": "toolu-pre-rich",
					"hookEvent": "PreToolUse",
					"message":   "checking input",
				}},
				Cancelled:      true,
				ExecutionError: "pre hook failed after progress",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	assertToolMessageRawBlock(t, messages, "hook_progress", "PreToolUse:text.upper", "toolu-pre-rich", map[string]any{
		"hookEvent": "PreToolUse",
		"message":   "checking input",
	})
	assertToolMessageRawBlock(t, messages, "hook_cancelled", "PreToolUse:text.upper", "toolu-pre-rich", map[string]any{
		"hookEvent": "PreToolUse",
	})
	assertToolMessageRawBlock(t, messages, "hook_error_during_execution", "PreToolUse:text.upper", "toolu-pre-rich", map[string]any{
		"hookEvent": "PreToolUse",
		"content":   "pre hook failed after progress",
	})
}

func TestQueryEngineStreamEventObjectInputDrivesStructuredToolAndTranscript(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run native object input tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{
					Type:     "tool.call",
					ToolName: "structured.echo",
					ToolInputObject: map[string]any{
						"command": "original",
						"cwd":     "/tmp",
					},
				},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var permissionInputs []map[string]any
	var invocations []map[string]any
	registry := tools.NewRegistry(
		structuredCapturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input.", Enabled: true},
				enabled: true,
			},
			permissionInputs: &permissionInputs,
			invocations:      &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(permissionInputs) != 1 {
		t.Fatalf("permission inputs = %#v, want one native object permission input", permissionInputs)
	}
	assertAnyMap(t, permissionInputs[0], map[string]any{
		"command": "original",
		"cwd":     "/tmp",
	})
	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want one native object invocation", invocations)
	}
	assertAnyMap(t, invocations[0], map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})

	var sawToolCalled bool
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "structured.echo" {
			sawToolCalled = true
			assertJSONInput(t, event.ToolInput, map[string]any{
				"command": "checked-structured",
				"cwd":     "/tmp",
			})
			assertAnyMap(t, event.ToolInputObject, map[string]any{
				"command": "checked-structured",
				"cwd":     "/tmp",
			})
		}
	}
	if !sawToolCalled {
		t.Fatalf("events = %#v, want tool.called for structured.echo", sink.events)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var foundToolUse bool
	for _, message := range messages {
		for _, block := range message.Blocks {
			if message.Role == "assistant" &&
				block.Type == model.MessageBlockToolUse &&
				block.Name == "structured.echo" {
				assertJSONInput(t, block.Input, map[string]any{
					"command": "checked-structured",
					"cwd":     "/tmp",
				})
				assertAnyMap(t, block.InputObject, map[string]any{
					"command": "checked-structured",
					"cwd":     "/tmp",
				})
				foundToolUse = true
			}
		}
	}
	if !foundToolUse {
		t.Fatalf("messages = %#v, want tool_use block with native object input", messages)
	}
}

func TestQueryEngineBackfillsObservableToolInputWithoutChangingExecutionInput(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run observable structured tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{
					Type:     "tool.call",
					ToolName: "structured.echo",
					ToolInputObject: map[string]any{
						"command": "original",
						"cwd":     "/tmp",
					},
					ToolUseID:         "provider-tool-use-1",
					ProviderMessageID: "provider-message-1",
				},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []map[string]any
	registry := tools.NewRegistry(
		observableBackfillToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want exactly one invocation", invocations)
	}
	assertAnyMap(t, invocations[0], map[string]any{
		"command": "original",
		"cwd":     "/tmp",
	})

	var sawToolCalled bool
	for _, event := range sink.events {
		if event.Type == "tool.called" && event.ToolName == "structured.echo" {
			sawToolCalled = true
			assertAnyMap(t, event.ToolInputObject, map[string]any{
				"command":         "original",
				"cwd":             "/tmp",
				"display_command": "checked-structured --display",
			})
		}
	}
	if !sawToolCalled {
		t.Fatalf("events = %#v, want tool.called for structured.echo", sink.events)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var foundToolUse bool
	for _, message := range messages {
		for _, block := range message.Blocks {
			if message.Role == "assistant" &&
				block.Type == model.MessageBlockToolUse &&
				block.Name == "structured.echo" {
				foundToolUse = true
				assertAnyMap(t, block.InputObject, map[string]any{
					"command":         "original",
					"cwd":             "/tmp",
					"display_command": "checked-structured --display",
				})
			}
		}
	}
	if !foundToolUse {
		t.Fatalf("messages = %#v, want observable backfilled tool_use block", messages)
	}
}

func TestQueryEnginePostToolUseAddsBlockingAndAdditionalContextBlocks(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run post hook context tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-post-context"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseHook: postToolUseHookFunc(func(_ context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
			if request.ToolName != "text.upper" || request.ToolOutput != "ok" {
				t.Fatalf("post hook request = %#v, want text.upper ok", request)
			}
			return queryengine.PostToolUseHookResult{
				BlockingError:      "post hook warning",
				AdditionalContexts: []string{"post hook context"},
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var sawBlocking bool
	var sawAdditional bool
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw == nil {
				continue
			}
			if block.Raw["type"] == "hook_blocking_error" &&
				block.Raw["hookName"] == "PostToolUse:text.upper" &&
				block.Raw["toolUseID"] == "toolu-post-context" &&
				block.Raw["blockingError"] == "post hook warning" {
				sawBlocking = true
			}
			if block.Raw["type"] == "hook_additional_context" &&
				block.Raw["hookName"] == "PostToolUse:text.upper" &&
				block.Raw["toolUseID"] == "toolu-post-context" {
				if contexts, ok := block.Raw["content"].([]string); ok && len(contexts) == 1 && contexts[0] == "post hook context" {
					sawAdditional = true
				}
			}
		}
	}
	if !sawBlocking || !sawAdditional {
		t.Fatalf("messages = %#v, want PostToolUse blocking and additional context blocks", messages)
	}
}

func TestQueryEnginePostToolUsePreventContinuationStopsNextModelPass(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run post hook stop tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-post-stop"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "should not be requested"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseHook: postToolUseHookFunc(func(_ context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
			return queryengine.PostToolUseHookResult{
				PreventContinuation: true,
				StopReason:          "stop after post hook",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if requests := client.Requests(); len(requests) != 1 {
		t.Fatalf("model requests = %d, want PostToolUse preventContinuation to stop after first pass", len(requests))
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var sawStopped bool
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw != nil &&
				block.Raw["type"] == "hook_stopped_continuation" &&
				block.Raw["hookName"] == "PostToolUse:text.upper" &&
				block.Raw["toolUseID"] == "toolu-post-stop" &&
				block.Raw["message"] == "stop after post hook" {
				sawStopped = true
			}
		}
	}
	if !sawStopped {
		t.Fatalf("messages = %#v, want PostToolUse stopped continuation block", messages)
	}
}

func TestQueryEnginePostToolUseAddsHookMessagesCancelledAndExecutionError(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run post hook rich messages")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-post-rich"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseHook: postToolUseHookFunc(func(_ context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
			return queryengine.PostToolUseHookResult{
				HookMessages: []map[string]any{
					{
						"type":      "hook_progress",
						"hookName":  "PostToolUse:text.upper",
						"toolUseID": "toolu-post-rich",
						"hookEvent": "PostToolUse",
						"message":   "checking output",
					},
					{
						"type":          "hook_blocking_error",
						"hookName":      "PostToolUse:text.upper",
						"toolUseID":     "toolu-post-rich",
						"hookEvent":     "PostToolUse",
						"blockingError": "duplicate block should be skipped",
					},
				},
				BlockingError:  "post hook warning",
				Cancelled:      true,
				ExecutionError: "post hook failed after progress",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	assertToolMessageRawBlock(t, messages, "hook_progress", "PostToolUse:text.upper", "toolu-post-rich", map[string]any{
		"hookEvent": "PostToolUse",
		"message":   "checking output",
	})
	assertToolMessageRawBlock(t, messages, "hook_blocking_error", "PostToolUse:text.upper", "toolu-post-rich", map[string]any{
		"hookEvent":     "PostToolUse",
		"blockingError": "post hook warning",
	})
	assertToolMessageRawBlock(t, messages, "hook_cancelled", "PostToolUse:text.upper", "toolu-post-rich", map[string]any{
		"hookEvent": "PostToolUse",
	})
	assertToolMessageRawBlock(t, messages, "hook_error_during_execution", "PostToolUse:text.upper", "toolu-post-rich", map[string]any{
		"hookEvent": "PostToolUse",
		"content":   "post hook failed after progress",
	})
	if countToolMessageRawBlocks(messages, "hook_blocking_error", "PostToolUse:text.upper", "toolu-post-rich") != 1 {
		t.Fatalf("messages = %#v, want one non-duplicated PostToolUse hook_blocking_error", messages)
	}
}

func TestQueryEnginePostToolUseUpdatesMCPToolOutput(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run post hook mcp output update")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "mcp__filesystem__read_resource", ToolInput: "read secret", ToolUseID: "toolu-mcp-update"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def: tools.Definition{
					Name:        "mcp__filesystem__read_resource",
					Description: "Read MCP resource.",
					Source:      "mcp",
					Enabled:     true,
				},
				enabled:  true,
				readOnly: true,
			},
			output: "original mcp output",
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseHook: postToolUseHookFunc(func(_ context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
			if request.ToolOutput != "original mcp output" {
				t.Fatalf("post hook tool output = %q, want original mcp output", request.ToolOutput)
			}
			return queryengine.PostToolUseHookResult{UpdatedMCPToolOutput: "updated mcp output"}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockToolResult && block.ToolUseID == "toolu-mcp-update" {
				if block.Content != "updated mcp output" {
					t.Fatalf("tool_result content = %q, want updated mcp output", block.Content)
				}
				if message.Content != "mcp__filesystem__read_resource: updated mcp output" {
					t.Fatalf("tool message content = %q, want updated mcp output", message.Content)
				}
				return
			}
		}
	}
	t.Fatalf("messages = %#v, want MCP tool_result", messages)
}

func TestQueryEnginePostToolUseIgnoresUpdatedMCPToolOutputForNonMCPTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run post hook non mcp output update")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "text.upper", ToolInput: "hello", ToolUseID: "toolu-non-mcp-update"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
				enabled:  true,
				readOnly: true,
			},
			output: "original non-mcp output",
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseHook: postToolUseHookFunc(func(_ context.Context, request queryengine.PostToolUseHookRequest) (queryengine.PostToolUseHookResult, bool, error) {
			return queryengine.PostToolUseHookResult{UpdatedMCPToolOutput: "should be ignored"}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockToolResult && block.ToolUseID == "toolu-non-mcp-update" {
				if block.Content != "original non-mcp output" {
					t.Fatalf("tool_result content = %q, want original non-mcp output", block.Content)
				}
				return
			}
		}
	}
	t.Fatalf("messages = %#v, want non-MCP tool_result", messages)
}

func TestQueryEnginePostToolUseFailureAddsHookBlocksAndContinues(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run failing post hook tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "failing.tool", ToolInput: "boom", ToolUseID: "toolu-failure"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "handled failure"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:      tools.Definition{Name: "failing.tool", Description: "Fails.", Enabled: true},
				enabled:  true,
				readOnly: true,
			},
			err: errors.New("tool exploded"),
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PostToolUseFailureHook: postToolUseFailureHookFunc(func(_ context.Context, request queryengine.PostToolUseFailureHookRequest) (queryengine.PostToolUseFailureHookResult, bool, error) {
			if request.ToolName != "failing.tool" || request.Error != "tool exploded" {
				t.Fatalf("failure hook request = %#v, want failing.tool tool exploded", request)
			}
			return queryengine.PostToolUseFailureHookResult{
				HookMessages: []map[string]any{{
					"type":      "hook_progress",
					"message":   "inspecting failure",
					"hookName":  "PostToolUseFailure:failing.tool",
					"toolUseID": "toolu-failure",
					"hookEvent": "PostToolUseFailure",
				}},
				BlockingError:      "failure hook block",
				AdditionalContexts: []string{"failure context"},
				Cancelled:          true,
				ExecutionError:     "failure hook crashed",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if got := len(client.Requests()); got != 2 {
		t.Fatalf("model request count = %d, want tool failure to continue to model", got)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	var sawErrorToolResult bool
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Type == model.MessageBlockToolResult &&
				block.ToolUseID == "toolu-failure" &&
				block.IsError &&
				block.Content == "tool exploded" {
				sawErrorToolResult = true
			}
		}
	}
	if !sawErrorToolResult {
		t.Fatalf("messages = %#v, want error tool_result for failed tool", messages)
	}
	assertToolMessageRawBlock(t, messages, "hook_progress", "PostToolUseFailure:failing.tool", "toolu-failure", map[string]any{
		"hookEvent": "PostToolUseFailure",
		"message":   "inspecting failure",
	})
	assertToolMessageRawBlock(t, messages, "hook_blocking_error", "PostToolUseFailure:failing.tool", "toolu-failure", map[string]any{
		"hookEvent":     "PostToolUseFailure",
		"blockingError": "failure hook block",
	})
	assertToolMessageRawBlock(t, messages, "hook_additional_context", "PostToolUseFailure:failing.tool", "toolu-failure", map[string]any{
		"hookEvent": "PostToolUseFailure",
	})
	assertToolMessageRawBlock(t, messages, "hook_cancelled", "PostToolUseFailure:failing.tool", "toolu-failure", map[string]any{
		"hookEvent": "PostToolUseFailure",
	})
	assertToolMessageRawBlock(t, messages, "hook_error_during_execution", "PostToolUseFailure:failing.tool", "toolu-failure", map[string]any{
		"hookEvent": "PostToolUseFailure",
		"content":   "failure hook crashed",
	})
}

func TestQueryEnginePassesToolUseContextToContextualPermissionTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if _, err := sessions.AppendMessage(sess.ID, "assistant", "seed history"); err != nil {
		t.Fatalf("append seed message: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "run contextual permission tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "contextual.tool", ToolInputObject: map[string]any{
					"command": "inspect",
					"cwd":     "/tmp",
				}, ToolUseID: "toolu-context"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var got tools.ToolUseContext
	var promptRequest tools.PromptRequest
	var progress []tools.ToolProgress
	var notifications []tools.Notification
	registry := tools.NewRegistry(
		contextualPermissionToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "contextual.tool", Description: "Uses full context.", Enabled: true},
				enabled: true,
			},
			gotContext: &got,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		MainLoopModel:    "sonnet",
		LLMProvider:      "anthropic",
		Debug:            true,
		Verbose:          true,
		ThinkingConfig: map[string]any{
			"type":          "enabled",
			"budget_tokens": 2048,
		},
		AgentDefinitions: tools.AgentDefinitions{
			ActiveAgents:      []string{"explorer"},
			AllowedAgentTypes: []string{"explorer", "worker"},
		},
		MaxBudgetUSD:            3.5,
		IsNonInteractiveSession: true,
		QueryTracking: tools.QueryTracking{
			ChainID: "chain-1",
			Depth:   2,
		},
		RequireCanUseTool: true,
		FileReadingLimits: tools.ResourceLimits{
			MaxTokens:    2048,
			MaxSizeBytes: 4096,
		},
		GlobLimits: tools.ResourceLimits{
			MaxResults: 32,
		},
		MCPClients: []tools.MCPConnection{{Name: "filesystem", Type: "stdio", BaseURL: "mcp://filesystem"}},
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {{URI: "file:///workspace", Name: "workspace", Description: "Workspace files"}},
		},
		RequestPrompt: func(sourceName, toolInputSummary string, request tools.PromptRequest) (tools.PromptResponse, error) {
			if sourceName != "contextual.tool" || toolInputSummary != "inspect" {
				t.Fatalf("request prompt source/summary = %q/%q, want contextual.tool/inspect", sourceName, toolInputSummary)
			}
			promptRequest = request
			return tools.PromptResponse{Value: "yes"}, nil
		},
		ReportToolProgress: func(item tools.ToolProgress) {
			progress = append(progress, item)
		},
		AddNotification: func(item tools.Notification) {
			notifications = append(notifications, item)
		},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if got.AbortContext == nil || got.Session.ID != sess.ID || got.ToolName != "contextual.tool" || got.ToolUseID != "toolu-context" || got.AgentID != sess.AgentID {
		t.Fatalf("tool context = %#v, want session/tool/agent metadata", got)
	}
	assertAnyMap(t, got.InputObject, map[string]any{"command": "inspect", "cwd": "/tmp"})
	if got.Policy.Mode != permissions.ModeWorkspaceWrite || got.MainLoopModel != "claude-sonnet-4-5" || got.LLMProvider != "anthropic" {
		t.Fatalf("tool context = %#v, want policy/model/provider metadata", got)
	}
	if !got.Debug || !got.Verbose || !got.IsNonInteractive || !got.RequireCanUseTool {
		t.Fatalf("tool context = %#v, want Claude-style option flags", got)
	}
	if got.MaxBudgetUSD != 3.5 || got.QueryTracking.ChainID != "chain-1" || got.QueryTracking.Depth != 2 {
		t.Fatalf("tool context = %#v, want budget/query tracking metadata", got)
	}
	if got.ThinkingConfig["budget_tokens"] != 2048 || len(got.AgentDefinitions.ActiveAgents) != 1 || got.AgentDefinitions.ActiveAgents[0] != "explorer" {
		t.Fatalf("tool context = %#v, want thinking config and agent definitions", got)
	}
	if len(got.Messages) < 2 {
		t.Fatalf("tool context messages = %#v, want current history", got.Messages)
	}
	if len(got.AvailableTools) != 1 || got.AvailableTools[0].Name != "contextual.tool" {
		t.Fatalf("available tools = %#v, want contextual tool definition", got.AvailableTools)
	}
	if got.SetAppState == nil || got.ToolDecisions == nil || got.RequestPrompt == nil || got.ReportProgress == nil {
		t.Fatalf("tool context = %#v, want appState/toolDecisions/progress callbacks", got)
	}
	got.AddNotification(tools.Notification{Key: "notice-1", Priority: "immediate", Message: "context notification"})
	if len(notifications) != 1 || notifications[0].Key != "notice-1" {
		t.Fatalf("notifications = %#v, want AddNotification callback", notifications)
	}
	refreshed := got.RefreshTools()
	if len(refreshed) != 1 || refreshed[0].Name != "contextual.tool" {
		t.Fatalf("refreshed tools = %#v, want current exposed tools", refreshed)
	}
	if got.ToolDecisions["contextual.tool"].Decision != "accept" {
		t.Fatalf("tool decisions = %#v, want contextual tool decision mutation", got.ToolDecisions)
	}
	if got.FileReadingLimits.MaxTokens != 2048 || got.FileReadingLimits.MaxSizeBytes != 4096 || got.GlobLimits.MaxResults != 32 {
		t.Fatalf("tool limits = file %#v glob %#v, want configured file/glob limits", got.FileReadingLimits, got.GlobLimits)
	}
	if len(got.MCPClients) != 1 || got.MCPClients[0].Name != "filesystem" {
		t.Fatalf("mcp clients = %#v, want configured client", got.MCPClients)
	}
	if len(got.MCPResources["filesystem"]) != 1 || got.MCPResources["filesystem"][0].URI != "file:///workspace" {
		t.Fatalf("mcp resources = %#v, want configured resource", got.MCPResources)
	}
	if promptRequest.Message != "confirm contextual permission" || len(promptRequest.Options) != 2 {
		t.Fatalf("prompt request = %#v, want contextual prompt request", promptRequest)
	}
	if len(progress) != 1 || progress[0].ToolUseID != "toolu-context" || progress[0].Message != "contextual check" {
		t.Fatalf("progress = %#v, want contextual progress event", progress)
	}
}

func TestQueryEngineAppliesToolExecutionContextModifierToNextToolContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run contextual execution tools")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "contextual.first", ToolInput: "first", ToolUseID: "toolu-first"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "contextual.second", ToolInput: "second", ToolUseID: "toolu-second"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var got []tools.ToolUseContext
	var progress []tools.ToolProgress
	registry := tools.NewRegistry(
		contextualExecutionToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "contextual.first", Description: "First contextual exec.", Enabled: true},
				enabled: true,
			},
			gotContexts: &got,
		},
		contextualExecutionToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "contextual.second", Description: "Second contextual exec.", Enabled: true},
				enabled: true,
			},
			gotContexts: &got,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		ReportToolProgress: func(item tools.ToolProgress) {
			progress = append(progress, item)
		},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("contexts = %#v, want two contextual tool executions", got)
	}
	if got[0].ToolName != "contextual.first" || got[0].ToolUseID != "toolu-first" {
		t.Fatalf("first context = %#v, want first tool metadata", got[0])
	}
	if got[1].ToolName != "contextual.second" || got[1].ToolUseID != "toolu-second" {
		t.Fatalf("second context = %#v, want second tool metadata", got[1])
	}
	if got[1].AppState["last_tool"] != "contextual.first" {
		t.Fatalf("second context app state = %#v, want first tool context modifier applied", got[1].AppState)
	}
	if len(progress) != 2 || progress[0].ToolUseID != "toolu-first" || progress[1].ToolUseID != "toolu-second" {
		t.Fatalf("progress = %#v, want execution progress from both tools", progress)
	}
}

func TestQueryEngineToolUseContextCanUseToolReusesPolicyEvaluation(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run delegated permission check")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "contextual.caller", ToolInput: "check", ToolUseID: "toolu-caller"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var decision permissions.Decision
	registry := tools.NewRegistry(
		contextualExecutionToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "contextual.caller", Description: "Calls canUseTool.", Enabled: true},
				enabled: true,
			},
			canUseToolRequest: &tools.CanUseToolRequest{
				ToolName: "system.run",
				Input:    "rm -rf /tmp/demo",
			},
			canUseToolDecision: &decision,
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true, Destructive: true},
			enabled:     true,
			destructive: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "system.run", Action: permissions.ActionDeny, Source: "local"},
			},
		},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if decision.Allowed || decision.RequiresApproval || decision.Category != permissions.CategoryRuleDenied {
		t.Fatalf("canUseTool decision = %#v, want policy deny for nested tool check", decision)
	}
}

func TestQueryEngineToolUseContextCanUseToolAppliesPermissionHook(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run delegated hook permission check")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "contextual.caller", ToolInput: "check", ToolUseID: "toolu-caller"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var decision permissions.Decision
	var hookCalled bool
	registry := tools.NewRegistry(
		contextualExecutionToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "contextual.caller", Description: "Calls canUseTool.", Enabled: true},
				enabled: true,
			},
			canUseToolRequest: &tools.CanUseToolRequest{
				ToolName:          "rewrite.echo",
				Input:             "original",
				ToolUseID:         "toolu-nested",
				ProviderMessageID: "msg-nested",
			},
			canUseToolDecision: &decision,
		},
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput:     "tool-rewritten-before-hook",
			requiresApproval: true,
			category:         permissions.CategoryApproval,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			hookCalled = true
			if request.ToolName != "rewrite.echo" || request.ToolInput != "tool-rewritten-before-hook" {
				t.Fatalf("permission hook request = %#v, want rewritten nested canUseTool input", request)
			}
			if request.ToolUseID != "toolu-nested" || request.ProviderMessageID != "msg-nested" {
				t.Fatalf("permission hook request metadata = %#v, want nested canUseTool ids", request)
			}
			return permissions.Decision{
				Allowed:      true,
				UpdatedInput: "hook-approved-rewrite",
				Reason:       "allowed by nested PermissionRequest hook",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if !hookCalled {
		t.Fatal("expected PermissionHook to handle nested canUseTool ask")
	}
	if !decision.Allowed || decision.UpdatedInput != "hook-approved-rewrite" {
		t.Fatalf("canUseTool decision = %#v, want hook-approved updated input", decision)
	}
}

func TestQueryEngineToolUseContextCanUseToolAppliesPermissionHookToPolicyAsk(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run delegated policy permission check")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "contextual.caller", ToolInput: "check", ToolUseID: "toolu-caller"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var decision permissions.Decision
	var hookCalled bool
	registry := tools.NewRegistry(
		contextualExecutionToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "contextual.caller", Description: "Calls canUseTool.", Enabled: true},
				enabled: true,
			},
			canUseToolRequest: &tools.CanUseToolRequest{
				ToolName: "system.run",
				Input:    "touch demo.txt",
			},
			canUseToolDecision: &decision,
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true, Destructive: true},
			enabled:     true,
			destructive: true,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			hookCalled = true
			if request.ToolName != "system.run" || request.ToolInput != "touch demo.txt" {
				t.Fatalf("permission hook request = %#v, want policy ask canUseTool input", request)
			}
			return permissions.Decision{
				Allowed:      true,
				UpdatedInput: "touch approved.txt",
				Reason:       "allowed by nested policy hook",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if !hookCalled {
		t.Fatal("expected PermissionHook to handle nested policy canUseTool ask")
	}
	if !decision.Allowed || decision.UpdatedInput != "touch approved.txt" {
		t.Fatalf("canUseTool decision = %#v, want policy hook-approved updated input", decision)
	}
}

func TestQueryEngineAutoModePassesToolClassifierInputToPolicy(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "run classifier allowed command")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "system.run", ToolInput: "cat README.md", ToolUseID: "toolu-auto"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		autoClassifierToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:      tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true},
				enabled:  true,
				readOnly: true,
			},
		},
	)
	var classifierInputs []any
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAuto,
			AutoClassifier: func(req permissions.Request) (permissions.Decision, bool) {
				classifierInputs = append(classifierInputs, req.AutoClassifierInput)
				if req.AutoClassifierInput == "classifier:cat README.md" {
					return permissions.Decision{
						Allowed: true,
						DecisionReason: permissions.DecisionReason{
							Type:       permissions.DecisionReasonClassifier,
							Classifier: "auto-mode",
							Reason:     "classifier allowed read",
						},
					}, true
				}
				return permissions.Decision{
					RequiresApproval: true,
					Category:         permissions.CategoryApproval,
					Reason:           "classifier requires confirmation",
				}, true
			},
		},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if len(classifierInputs) != 1 || classifierInputs[0] != "classifier:cat README.md" {
		t.Fatalf("classifier inputs = %#v, want tool-projected classifier input", classifierInputs)
	}
}

func TestQueryEngineToolPermissionUpdatedInputPersistsThroughApproval(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "rewrite tool input with approval")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput:     "rewritten-before-approval",
			requiresApproval: true,
			category:         permissions.CategoryApproval,
			invocations:      &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial request to require approval")
	}
	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}
	if requests[0].ToolInput != "rewritten-before-approval" {
		t.Fatalf("approval tool input = %q, want updated input", requests[0].ToolInput)
	}

	if err := engine.ApproveAndContinue(context.Background(), requests[0].ID, &captureSink{}); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}
	if len(invocations) != 1 || invocations[0] != "rewritten-before-approval" {
		t.Fatalf("invocations = %#v, want approved execution with updated input", invocations)
	}
}

func TestQueryEngineToolPermissionStructuredUpdatedInputPersistsThroughApproval(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "rewrite structured tool input with approval")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: `{"command":"original","cwd":"/tmp"}`},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInputObject: map[string]any{
				"command": "rewritten-before-approval",
				"cwd":     "/workspace",
			},
			requiresApproval: true,
			category:         permissions.CategoryApproval,
			invocations:      &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial request to require approval")
	}
	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}
	assertJSONInput(t, requests[0].ToolInput, map[string]any{
		"command": "rewritten-before-approval",
		"cwd":     "/workspace",
	})

	if err := engine.ApproveAndContinue(context.Background(), requests[0].ID, &captureSink{}); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want approved execution with structured updated input", invocations)
	}
	assertJSONInput(t, invocations[0], map[string]any{
		"command": "rewritten-before-approval",
		"cwd":     "/workspace",
	})
}

func TestQueryEngineStreamEventObjectInputPersistsThroughApproval(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "approve native object input")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{
					Type:     "tool.call",
					ToolName: "structured.echo",
					ToolInputObject: map[string]any{
						"command": "original",
						"cwd":     "/tmp",
					},
				},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var permissionInputs []map[string]any
	var invocations []map[string]any
	registry := tools.NewRegistry(
		structuredCapturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "structured.echo", Description: "Echo structured input.", Enabled: true},
				enabled: true,
			},
			permissionInputs: &permissionInputs,
			invocations:      &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		ApprovalManager: approvalManager,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "structured.echo",
				Action:   permissions.ActionAsk,
			}},
		},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err == nil {
		t.Fatal("expected initial request to require approval")
	}
	requests := approvalManager.ListBySession(sess.ID)
	if len(requests) != 1 {
		t.Fatalf("approval count = %d, want 1", len(requests))
	}
	assertJSONInput(t, requests[0].ToolInput, map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})
	assertAnyMap(t, requests[0].ToolInputObject, map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})

	updated, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	assertAnyMap(t, updated.Metadata.PendingApprovalToolInputObject, map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})

	if err := engine.ApproveAndContinue(context.Background(), requests[0].ID, &captureSink{}); err != nil {
		t.Fatalf("approve and continue: %v", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want approved structured invocation", invocations)
	}
	assertAnyMap(t, invocations[0], map[string]any{
		"command": "checked-structured",
		"cwd":     "/tmp",
	})
}

func TestQueryEnginePermissionHookUpdatedInputAllowsPendingApprovalTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "rewrite tool input through hook")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		capturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		ApprovalManager: approvalManager,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionAsk,
			}},
		},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			if request.ToolName != "rewrite.echo" || request.ToolInput != "original" {
				t.Fatalf("permission hook request = %#v, want original rewrite.echo request", request)
			}
			return permissions.Decision{
				Allowed:      true,
				UpdatedInput: "hook-rewritten",
				Reason:       "allowed by PermissionRequest hook",
			}, true, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(approvalManager.ListBySession(sess.ID)) != 0 {
		t.Fatalf("approvals = %#v, want hook allow to bypass approval creation", approvalManager.ListBySession(sess.ID))
	}
	if len(invocations) != 1 || invocations[0] != "hook-rewritten" {
		t.Fatalf("invocations = %#v, want hook-updated input", invocations)
	}
	for _, event := range sink.events {
		if event.Type == "permission.required" {
			t.Fatalf("events = %#v, did not want permission.required after hook allow", sink.events)
		}
	}
}

func TestQueryEnginePermissionHookAllowAppliesUpdatedPermissionsToSessionPolicy(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg1, err := sessions.AppendMessage(sess.ID, "user", "first guarded call")
	if err != nil {
		t.Fatalf("append first user message: %v", err)
	}
	msg2, err := sessions.AppendMessage(sess.ID, "user", "second guarded call")
	if err != nil {
		t.Fatalf("append second user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "first done"},
				{Type: "message.end"},
			},
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "second done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		capturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	var hookCalls int
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionAsk,
			}},
		},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			hookCalls++
			return permissions.Decision{
				Allowed: true,
				UpdatedPermissions: []permissions.PermissionUpdate{{
					Type:        permissions.PermissionUpdateAddRules,
					Destination: permissions.PermissionUpdateDestinationSession,
					Behavior:    permissions.ActionAllow,
					Rules: []permissions.PermissionRuleValue{{
						ToolName: "rewrite.echo",
					}},
				}},
				Reason: "allowed by PermissionRequest hook with persisted rule",
			}, true, nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg1, &captureSink{}); err != nil {
		t.Fatalf("submit first message: %v", err)
	}
	if err := engine.SubmitMessage(context.Background(), sess, msg2, &captureSink{}); err != nil {
		t.Fatalf("submit second message: %v", err)
	}

	if hookCalls != 1 {
		t.Fatalf("hook calls = %d, want second call allowed by updated session policy", hookCalls)
	}
	if len(invocations) != 2 {
		t.Fatalf("invocations = %#v, want both tool calls executed", invocations)
	}
	decision := engine.PermissionPolicyForSession(sess.ID).Evaluate(permissions.Request{
		ToolName: "rewrite.echo",
		Command:  "original",
		WorkDir:  "",
	})
	if !decision.Allowed {
		t.Fatalf("session policy decision = %#v, want persisted allow rule", decision)
	}
}

func TestQueryEnginePermissionHookAllowPersistsNonSessionUpdatedPermissions(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "guarded call")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		capturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
		},
	)
	update := permissions.PermissionUpdate{
		Type:        permissions.PermissionUpdateAddRules,
		Destination: permissions.PermissionUpdateDestinationUserSettings,
		Behavior:    permissions.ActionAllow,
		Rules: []permissions.PermissionRuleValue{{
			ToolName: "rewrite.echo",
		}},
	}
	var persistedSessionID string
	var persisted []permissions.PermissionUpdate
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionAsk,
			}},
		},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			return permissions.Decision{
				Allowed:            true,
				UpdatedPermissions: []permissions.PermissionUpdate{update},
				Reason:             "allowed by PermissionRequest hook with user setting update",
			}, true, nil
		}),
		PermissionUpdatePersister: permissionUpdatePersisterFunc(func(_ context.Context, sess session.Session, updates []permissions.PermissionUpdate) error {
			persistedSessionID = sess.ID
			persisted = append([]permissions.PermissionUpdate(nil), updates...)
			return nil
		}),
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if persistedSessionID != sess.ID {
		t.Fatalf("persisted session id = %q, want %q", persistedSessionID, sess.ID)
	}
	if len(persisted) != 1 || persisted[0].Destination != permissions.PermissionUpdateDestinationUserSettings {
		t.Fatalf("persisted updates = %#v, want userSettings update", persisted)
	}
}

func TestQueryEnginePermissionHookStructuredUpdatedInputAllowsPendingApprovalTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "rewrite structured input through hook")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: `{"command":"original","cwd":"/tmp"}`},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		capturingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			invocations: &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		ApprovalManager: approvalManager,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionAsk,
			}},
		},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			if request.ToolName != "rewrite.echo" {
				t.Fatalf("permission hook request = %#v, want rewrite.echo request", request)
			}
			assertJSONInput(t, request.ToolInput, map[string]any{
				"command": "original",
				"cwd":     "/tmp",
			})
			assertAnyMap(t, request.ToolInputObject, map[string]any{
				"command": "original",
				"cwd":     "/tmp",
			})
			return permissions.Decision{
				Allowed: true,
				UpdatedInputObject: map[string]any{
					"command": "hook-rewritten",
					"cwd":     "/workspace",
				},
				Reason: "allowed by PermissionRequest hook",
			}, true, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if len(approvalManager.ListBySession(sess.ID)) != 0 {
		t.Fatalf("approvals = %#v, want hook allow to bypass approval creation", approvalManager.ListBySession(sess.ID))
	}
	if len(invocations) != 1 {
		t.Fatalf("invocations = %#v, want hook-updated structured input", invocations)
	}
	assertJSONInput(t, invocations[0], map[string]any{
		"command": "hook-rewritten",
		"cwd":     "/workspace",
	})
}

func TestQueryEnginePolicyAskOverridesToolPermissionAllow(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "try guarded rewrite tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{{
			{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
			{Type: "message.end"},
		}},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput: "tool-allowed-rewrite",
			invocations:  &invocations,
		},
	)
	approvalManager := approval.NewManager()
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		ApprovalManager: approvalManager,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionAsk,
				Source:   string(permissions.RuleSourceConfig),
			}},
		},
	})

	err = engine.SubmitMessage(context.Background(), sess, msg, &captureSink{})
	if err == nil {
		t.Fatal("expected policy ask to override tool permission allow")
	}
	if !strings.Contains(err.Error(), "requires approval") {
		t.Fatalf("error = %v, want requires approval", err)
	}
	if len(invocations) != 0 {
		t.Fatalf("invocations = %#v, want approval-gated tool not invoked", invocations)
	}
	if len(approvalManager.ListBySession(sess.ID)) != 1 {
		t.Fatalf("approvals = %#v, want policy ask approval", approvalManager.ListBySession(sess.ID))
	}
}

func TestQueryEnginePolicyDenyOverridesToolPermissionAllow(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "try denied rewrite tool")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{{
			{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
			{Type: "message.end"},
		}},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput: "tool-allowed-rewrite",
			invocations:  &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionDeny,
				Match: permissions.Match{
					CommandContains: []string{"tool-allowed-rewrite"},
				},
				Source: string(permissions.RuleSourceConfig),
			}},
		},
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if len(invocations) != 0 {
		t.Fatalf("invocations = %#v, want denied tool not invoked", invocations)
	}
	var toolResult *session.Message
	for _, event := range sink.events {
		if event.Type == "tool.result" {
			toolResult = event.Message
			break
		}
	}
	if toolResult == nil {
		t.Fatalf("events = %#v, want policy-denied error tool result", sink.events)
	}
	if len(toolResult.Blocks) == 0 || !toolResult.Blocks[0].IsError {
		t.Fatalf("tool result blocks = %#v, want error tool result", toolResult.Blocks)
	}
}

func TestQueryEnginePermissionHookHandlesToolPermissionAsk(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "hook tool ask")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput:     "tool-rewritten-before-hook",
			requiresApproval: true,
			category:         permissions.CategoryApproval,
			invocations:      &invocations,
		},
	)
	approvalManager := approval.NewManager()
	var hookCalled bool
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		ApprovalManager:  approvalManager,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			hookCalled = true
			if request.ToolName != "rewrite.echo" || request.ToolInput != "tool-rewritten-before-hook" {
				t.Fatalf("permission hook request = %#v, want tool-updated rewrite.echo request", request)
			}
			return permissions.Decision{
				Allowed:      true,
				UpdatedInput: "hook-approved-rewrite",
				Reason:       "allowed by PermissionRequest hook",
			}, true, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	if !hookCalled {
		t.Fatal("expected PermissionHook to handle tool permission ask")
	}
	if len(approvalManager.ListBySession(sess.ID)) != 0 {
		t.Fatalf("approvals = %#v, want hook allow to bypass approval creation", approvalManager.ListBySession(sess.ID))
	}
	if len(invocations) != 1 || invocations[0] != "hook-approved-rewrite" {
		t.Fatalf("invocations = %#v, want hook-updated input", invocations)
	}
}

func TestQueryEnginePermissionHookAskEmitsSerializedDecisionReason(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "hook asks with reason")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{{
			{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
			{Type: "message.end"},
		}},
	}
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			requiresApproval: true,
			category:         permissions.CategoryApproval,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		ApprovalManager: approval.NewManager(),
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
		},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			return permissions.Decision{
				RequiresApproval: true,
				Category:         permissions.CategoryApproval,
				Reason:           "hook wants a human",
				DecisionReason: permissions.DecisionReason{
					Type:     permissions.DecisionReasonHook,
					HookName: "PermissionRequest",
					Reason:   "hook wants a human",
				},
			}, true, nil
		}),
	})

	sink := &captureSink{}
	err = engine.SubmitMessage(context.Background(), sess, msg, sink)
	if err == nil {
		t.Fatal("expected hook ask to require approval")
	}
	for _, event := range sink.events {
		if event.Type == "permission.required" {
			if event.DecisionReason != "hook wants a human" {
				t.Fatalf("decision reason = %q, want serialized hook reason", event.DecisionReason)
			}
			if event.Approval == nil || event.Approval.DecisionReason != "hook wants a human" {
				t.Fatalf("approval = %#v, want serialized hook decision reason", event.Approval)
			}
			updated, ok := sessions.GetByID(sess.ID)
			if !ok {
				t.Fatalf("session %q not found", sess.ID)
			}
			if updated.Metadata.PendingApprovalDecisionReason != "hook wants a human" {
				t.Fatalf("metadata = %#v, want pending approval decision reason", updated.Metadata)
			}
			return
		}
	}
	t.Fatalf("events = %#v, want permission.required", sink.events)
}

func TestQueryEnginePolicyDenyOverridesToolPermissionHookAllow(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	msg, err := sessions.AppendMessage(sess.ID, "user", "hook allow still policy denied")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{{
			{Type: "tool.call", ToolName: "rewrite.echo", ToolInput: "original"},
			{Type: "message.end"},
		}},
	}
	var invocations []string
	registry := tools.NewRegistry(
		permissionRewritingToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def:     tools.Definition{Name: "rewrite.echo", Description: "Echo rewritten input.", Enabled: true},
				enabled: true,
			},
			updatedInput:     "tool-rewritten-before-hook",
			requiresApproval: true,
			category:         permissions.CategoryApproval,
			invocations:      &invocations,
		},
	)
	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{{
				ToolName: "rewrite.echo",
				Action:   permissions.ActionDeny,
				Match: permissions.Match{
					CommandContains: []string{"hook-denied-rewrite"},
				},
			}},
		},
		PermissionHook: permissionHookFunc(func(_ context.Context, request queryengine.PermissionHookRequest) (permissions.Decision, bool, error) {
			if request.ToolInput != "tool-rewritten-before-hook" {
				t.Fatalf("permission hook request = %#v, want tool-updated input", request)
			}
			return permissions.Decision{
				Allowed:      true,
				UpdatedInput: "hook-denied-rewrite",
				Reason:       "allowed by PermissionRequest hook",
			}, true, nil
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}
	if len(invocations) != 0 {
		t.Fatalf("invocations = %#v, want policy-denied tool not invoked", invocations)
	}
	var toolResult *session.Message
	for _, event := range sink.events {
		if event.Type == "tool.result" {
			toolResult = event.Message
			break
		}
	}
	if toolResult == nil {
		t.Fatalf("events = %#v, want policy-denied error tool result", sink.events)
	}
	if len(toolResult.Blocks) == 0 || !toolResult.Blocks[0].IsError {
		t.Fatalf("tool result blocks = %#v, want error tool result", toolResult.Blocks)
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
			def:      tools.Definition{Name: "text.upper", Description: "Uppercase text.", Enabled: true},
			enabled:  true,
			readOnly: true,
		},
		stubToolForQueryEngine{
			def:     tools.Definition{Name: "system.run", Description: "Run command.", Enabled: true},
			enabled: true,
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "agent.task", Description: "Run delegated task.", Enabled: true},
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

func TestQueryEngineRejectsDirectToolCallForBlanketDeniedTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "mcp__filesystem__read_resource", ToolInput: "read secret"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def: tools.Definition{
					Name:        "mcp__filesystem__read_resource",
					Description: "Read MCP resource.",
					Source:      "mcp",
				},
				enabled: true,
			},
			output: "secret",
		},
	)

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "mcp__filesystem", Action: permissions.ActionDeny},
			},
		},
	})

	err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{})
	if err == nil {
		t.Fatal("expected denied direct tool call to be rejected")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want unavailable-tool error", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role == "tool" {
			t.Fatalf("messages = %#v, did not want denied tool execution to append a tool message", messages)
		}
	}
}

func TestQueryEngineRejectsDirectToolCallForWildcardDeniedMcpTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "mcp__filesystem__read_resource", ToolInput: "read secret"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def: tools.Definition{
					Name:        "mcp__filesystem__read_resource",
					Description: "Read MCP resource.",
					Source:      "mcp",
				},
				enabled: true,
			},
			output: "secret",
		},
	)

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		ToolRegistry:    registry,
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{
				{ToolName: "mcp__filesystem__*", Action: permissions.ActionDeny},
			},
		},
	})

	err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{})
	if err == nil {
		t.Fatal("expected wildcard-denied direct tool call to be rejected")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want unavailable-tool error", err)
	}
}

func TestQueryEngineRejectsDirectToolCallForDisabledTool(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "hidden.tool", ToolInput: "run"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		scriptedToolForQueryEngine{
			stubToolForQueryEngine: stubToolForQueryEngine{
				def: tools.Definition{
					Name:        "hidden.tool",
					Description: "Disabled tool.",
				},
				enabled: false,
			},
			output: "hidden",
		},
	)

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{})
	if err == nil {
		t.Fatal("expected disabled direct tool call to be rejected")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want unavailable-tool error", err)
	}
}

func TestQueryEngineRejectsDirectToolSearchCallWhenDisabledByEnv(t *testing.T) {
	t.Setenv("ENABLE_TOOL_SEARCH", "false")

	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "delegate"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry()
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	err := engine.SubmitPrompt(context.Background(), sess, "find a tool", &captureSink{})
	if err == nil {
		t.Fatal("expected disabled tool.search direct call to be rejected")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("error = %v, want unavailable-tool error", err)
	}
}

func TestQueryEngineToolSearchDoesNotRevealDeniedTools(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "delegate"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
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
				{ToolName: "agent.task", Action: permissions.ActionDeny},
			},
		},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "find a delegation tool", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	foundTool := false
	for _, message := range messages {
		if message.Role == "tool" {
			foundTool = true
			if strings.Contains(message.Content, "agent.task") {
				t.Fatalf("tool message = %#v, did not want denied tool leaked through tool.search", message)
			}
		}
	}
	if !foundTool {
		t.Fatalf("messages = %#v, want tool.search tool result", messages)
	}
}

func TestQueryEngineResolvesToolCallByAlias(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "uppercase", ToolInput: "hello"},
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
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "uppercase hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	foundTool := false
	for _, message := range messages {
		if message.Role == "tool" {
			foundTool = true
			if !strings.Contains(message.Content, "uppercase: HELLO") {
				t.Fatalf("tool message = %#v, want alias-routed tool result", message)
			}
		}
	}
	if !foundTool {
		t.Fatalf("messages = %#v, want tool result message", messages)
	}
}

func TestQueryEngineToolSearchOnlyReturnsDeferredTools(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "delegate"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:        tools.Definition{Name: "system.run", Description: "Run command."},
			enabled:    true,
			searchHint: "delegate subtask",
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "find a delegation tool", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "system.run") {
			t.Fatalf("tool message = %#v, did not want non-deferred tool leaked through tool.search", message)
		}
		if !strings.Contains(message.Content, "agent.task") {
			t.Fatalf("tool message = %#v, want deferred tool in tool.search result", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchSupportsDirectSelectionByToolName(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "select:agent.task"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "pick the agent task tool", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "No matching tools found.") {
			t.Fatalf("tool message = %#v, did not want no-match result for direct selection", message)
		}
		if !strings.Contains(message.Content, "agent.task") {
			t.Fatalf("tool message = %#v, want directly selected deferred tool", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchSupportsDirectMultiSelectAndLoadedToolFallback(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "select:agent.task,system.run"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:        tools.Definition{Name: "system.run", Description: "Run command."},
			enabled:    true,
			searchHint: "shell command",
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "pick the tool set", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if !strings.Contains(message.Content, "agent.task") {
			t.Fatalf("tool message = %#v, want deferred selected tool", message)
		}
		if !strings.Contains(message.Content, "system.run") {
			t.Fatalf("tool message = %#v, want loaded-tool fallback selection", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchSupportsBareExactNameSelection(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "agent.task"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "agent.task", Description: "Run a subagent task."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "delegate subtask",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "pick the exact tool", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "No matching tools found.") {
			t.Fatalf("tool message = %#v, did not want no-match result for exact-name query", message)
		}
		if !strings.Contains(message.Content, "agent.task") {
			t.Fatalf("tool message = %#v, want exact-name deferred tool", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchSupportsMcpPrefixSelection(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "mcp__filesystem"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "mcp__filesystem__read_resource", Description: "Read MCP resource."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "read file",
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "mcp__filesystem__list_resources", Description: "List MCP resources."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "list files",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "pick filesystem MCP tools", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if !strings.Contains(message.Content, "mcp__filesystem__read_resource") || !strings.Contains(message.Content, "mcp__filesystem__list_resources") {
			t.Fatalf("tool message = %#v, want MCP prefix matches", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchSupportsBareExactNameLoadedToolFallback(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "system.run"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:        tools.Definition{Name: "system.run", Description: "Run command."},
			enabled:    true,
			searchHint: "shell command",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "pick system.run", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "No matching tools found.") {
			t.Fatalf("tool message = %#v, did not want no-match result for loaded-tool exact-name query", message)
		}
		if !strings.Contains(message.Content, "system.run") {
			t.Fatalf("tool message = %#v, want loaded-tool exact-name fallback", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchLimitsKeywordResultsToDefaultFive(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "delegate"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry()
	for i := 1; i <= 6; i++ {
		registry.Register(
			stubToolForQueryEngine{
				def:         tools.Definition{Name: "agent.task." + strconv.Itoa(i), Description: "Run a subagent task."},
				enabled:     true,
				shouldDefer: true,
				searchHint:  "delegate subtask",
			},
		)
	}
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "find delegation tools", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if strings.Contains(message.Content, "agent.task.6") {
			t.Fatalf("tool message = %#v, did not want more than 5 default matches", message)
		}
		for i := 1; i <= 5; i++ {
			if !strings.Contains(message.Content, "agent.task."+strconv.Itoa(i)) {
				t.Fatalf("tool message = %#v, want default top-five match agent.task.%d", message, i)
			}
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineToolSearchSupportsRequiredTerms(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "tool.call", ToolName: "tool.search", ToolInput: "+slack send"},
				{Type: "message.end"},
			},
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}
	registry := tools.NewRegistry(
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "mcp__slack__send_message", Description: "Send a Slack message."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "slack send message",
		},
		stubToolForQueryEngine{
			def:         tools.Definition{Name: "mcp__email__send_message", Description: "Send an email."},
			enabled:     true,
			shouldDefer: true,
			searchHint:  "email send message",
		},
	)
	registry.Register(tools.NewToolSearchTool(registry))

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		ToolRegistry:     registry,
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "find slack sender", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		if !strings.Contains(message.Content, "mcp__slack__send_message") {
			t.Fatalf("tool message = %#v, want Slack match", message)
		}
		if strings.Contains(message.Content, "mcp__email__send_message") {
			t.Fatalf("tool message = %#v, did not want non-required match", message)
		}
		return
	}
	t.Fatalf("messages = %#v, want tool.search tool result", messages)
}

func TestQueryEngineIncludesSessionPermissionAndWorkspaceContextLines(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("always ask before deploy"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(root),
		PermissionPolicy: permissions.Policy{
			Mode:           permissions.ModeWorkspaceWrite,
			PlanMode:       true,
			AutoMode:       true,
			WorkspaceRoots: []string{root, filepath.Join(root, "sub")},
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
	if !containsPrefix(requests[0].Context.UserContextLines, "current_date=") {
		t.Fatalf("user context lines = %#v, want current date", requests[0].Context.UserContextLines)
	}
	if !containsString(requests[0].Context.UserContextLines, "claude_md=always ask before deploy") {
		t.Fatalf("user context lines = %#v, want CLAUDE.md content", requests[0].Context.UserContextLines)
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
	if !containsString(requests[0].Context.SystemContextLines, "workspace_root="+root) {
		t.Fatalf("system context lines = %#v, want workspace root", requests[0].Context.SystemContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "workspace_roots="+root+","+filepath.Join(root, "sub")) {
		t.Fatalf("system context lines = %#v, want workspace roots", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEngineIncludesGitStatusSnapshotInSystemContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	root := t.TempDir()
	runCommand(t, root, "git", "init", "-b", "main")
	runCommand(t, root, "git", "config", "user.name", "Test User")
	runCommand(t, root, "git", "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runCommand(t, root, "git", "add", "README.md")
	runCommand(t, root, "git", "commit", "-m", "initial commit")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello\nchanged"), 0o644); err != nil {
		t.Fatalf("update README.md: %v", err)
	}

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
		WorkspaceLoader:  workspace.NewLoader(root),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if !containsPrefix(requests[0].Context.SystemContextLines, "git_status=") {
		t.Fatalf("system context lines = %#v, want git status snapshot", requests[0].Context.SystemContextLines)
	}
	for _, want := range []string{
		"This is the git status at the start of the conversation.",
		"Current branch:",
		"Main branch (you will usually use this for PRs): main",
		"Git user: Test User",
		"Status:",
		"Recent commits:",
	} {
		if !containsSubstring(requests[0].Context.SystemContextLines, want) {
			t.Fatalf("system context lines = %#v, want substring %q", requests[0].Context.SystemContextLines, want)
		}
	}
}

func TestQueryEngineTruncatesLongGitStatusSnapshots(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	root := t.TempDir()
	runCommand(t, root, "git", "init", "-b", "main")
	runCommand(t, root, "git", "config", "user.name", "Test User")
	runCommand(t, root, "git", "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runCommand(t, root, "git", "add", "README.md")
	runCommand(t, root, "git", "commit", "-m", "initial commit")
	for i := 0; i < 140; i++ {
		name := filepath.Join(root, "file-"+strings.Repeat("x", 18)+"-"+strconv.Itoa(i)+".txt")
		if err := os.WriteFile(name, []byte("changed"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

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
		WorkspaceLoader:  workspace.NewLoader(root),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if !containsSubstring(requests[0].Context.SystemContextLines, "... (truncated because it exceeds 2k characters. If you need more information, run \"git status\" using BashTool)") {
		t.Fatalf("system context lines = %#v, want truncation marker", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEngineIncludesConfiguredCacheBreakerInSystemContext(t *testing.T) {
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
		Sessions:              sessions,
		Client:                client,
		WorkspaceLoader:       workspace.NewLoader(t.TempDir()),
		PermissionPolicy:      permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		SystemPromptInjection: "force-refresh",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if !containsString(requests[0].Context.SystemContextLines, "cache_breaker=[CACHE_BREAKER: force-refresh]") {
		t.Fatalf("system context lines = %#v, want configured cache breaker", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEngineCanDisableClaudeMdUserContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("always ask before deploy"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
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
		WorkspaceLoader:  workspace.NewLoader(root),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		DisableClaudeMd:  true,
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if containsSubstring(requests[0].Context.UserContextLines, "claude_md=") {
		t.Fatalf("user context lines = %#v, did not want CLAUDE.md when disabled", requests[0].Context.UserContextLines)
	}
}

func TestQueryEngineCanDisableGitStatusSystemContext(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	root := t.TempDir()
	runCommand(t, root, "git", "init", "-b", "main")
	runCommand(t, root, "git", "config", "user.name", "Test User")
	runCommand(t, root, "git", "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runCommand(t, root, "git", "add", "README.md")
	runCommand(t, root, "git", "commit", "-m", "initial commit")
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
		WorkspaceLoader:  workspace.NewLoader(root),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		DisableGitStatus: true,
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if containsSubstring(requests[0].Context.SystemContextLines, "git_status=") {
		t.Fatalf("system context lines = %#v, did not want git status when disabled", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEngineUsesConfiguredContextProviders(t *testing.T) {
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
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader("C:/repo"),
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeWorkspaceWrite,
		},
		UserContextProvider: queryengine.UserContextProviderFunc(func(_ session.Session, _ workspace.Context) []string {
			return []string{
				"user_lane=session",
				"user_lane=custom",
			}
		}),
		SystemContextProvider: queryengine.SystemContextProviderFunc(func(_ session.Session, _ workspace.Context, _ permissions.Policy) []string {
			return []string{
				"system_lane=policy",
				"system_lane=custom",
			}
		}),
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if !containsString(requests[0].Context.UserContextLines, "user_lane=session") {
		t.Fatalf("user context lines = %#v, want custom provider output", requests[0].Context.UserContextLines)
	}
	if !containsString(requests[0].Context.UserContextLines, "user_lane=custom") {
		t.Fatalf("user context lines = %#v, want custom provider output", requests[0].Context.UserContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "system_lane=policy") {
		t.Fatalf("system context lines = %#v, want custom provider output", requests[0].Context.SystemContextLines)
	}
	if !containsString(requests[0].Context.SystemContextLines, "system_lane=custom") {
		t.Fatalf("system context lines = %#v, want custom provider output", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEngineKeepsUserAndSystemContextProvidersSeparated(t *testing.T) {
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
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeWorkspaceWrite},
		UserContextProvider: queryengine.UserContextProviderFunc(func(_ session.Session, _ workspace.Context) []string {
			return []string{"lane=user"}
		}),
		SystemContextProvider: queryengine.SystemContextProviderFunc(func(_ session.Session, _ workspace.Context, _ permissions.Policy) []string {
			return []string{"lane=system"}
		}),
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if containsString(requests[0].Context.UserContextLines, "lane=system") {
		t.Fatalf("user context lines = %#v, did not want system lane leakage", requests[0].Context.UserContextLines)
	}
	if containsString(requests[0].Context.SystemContextLines, "lane=user") {
		t.Fatalf("system context lines = %#v, did not want user lane leakage", requests[0].Context.SystemContextLines)
	}
}

func TestQueryEnginePassesPromptOverridesIntoPromptBuilder(t *testing.T) {
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
		Sessions:           sessions,
		Client:             client,
		WorkspaceLoader:    workspace.NewLoader(""),
		PermissionPolicy:   permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		CustomSystemPrompt: "custom prompt",
		AppendSystemPrompt: "append prompt",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if !strings.Contains(requests[0].Context.SystemPrompt, "custom prompt") {
		t.Fatalf("system prompt = %q, want custom prompt", requests[0].Context.SystemPrompt)
	}
	if strings.Contains(requests[0].Context.SystemPrompt, "You are myclaw agent") {
		t.Fatalf("system prompt = %q, did not want default system prompt when custom prompt is set", requests[0].Context.SystemPrompt)
	}
	if !strings.Contains(requests[0].Context.SystemPrompt, "append prompt") {
		t.Fatalf("system prompt = %q, want append prompt", requests[0].Context.SystemPrompt)
	}
}

func TestQueryEnginePassesCoordinatorPromptIntoPromptBuilder(t *testing.T) {
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
		Sessions:                sessions,
		Client:                  client,
		WorkspaceLoader:         workspace.NewLoader(""),
		PermissionPolicy:        permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		CustomSystemPrompt:      "custom prompt",
		AgentSystemPrompt:       "agent prompt",
		CoordinatorSystemPrompt: "coordinator prompt",
		AppendSystemPrompt:      "append prompt",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if !strings.Contains(requests[0].Context.SystemPrompt, "coordinator prompt") {
		t.Fatalf("system prompt = %q, want coordinator prompt", requests[0].Context.SystemPrompt)
	}
	for _, blocked := range []string{"agent prompt", "custom prompt"} {
		if strings.Contains(requests[0].Context.SystemPrompt, blocked) {
			t.Fatalf("system prompt = %q, did not want %q", requests[0].Context.SystemPrompt, blocked)
		}
	}
	if !strings.Contains(requests[0].Context.SystemPrompt, "append prompt") {
		t.Fatalf("system prompt = %q, want append prompt", requests[0].Context.SystemPrompt)
	}
}

func TestQueryEnginePassesProactiveAgentPromptModeIntoPromptBuilder(t *testing.T) {
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
		Sessions:             sessions,
		Client:               client,
		WorkspaceLoader:      workspace.NewLoader(""),
		PermissionPolicy:     permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		DefaultSystemPrompt:  []string{"default prompt"},
		AgentSystemPrompt:    "agent prompt",
		AppendSystemPrompt:   "append prompt",
		ProactiveAgentPrompt: true,
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	for _, want := range []string{"default prompt", "# Custom Agent Instructions", "agent prompt", "append prompt"} {
		if !strings.Contains(requests[0].Context.SystemPrompt, want) {
			t.Fatalf("system prompt = %q, want %q", requests[0].Context.SystemPrompt, want)
		}
	}
}

func TestQueryEnginePassesConfiguredMainLoopModelIntoGenerateRequest(t *testing.T) {
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
		MainLoopModel:    "claude-opus-4-6",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want configured main loop model", requests[0].Model)
	}
}

func TestQueryEngineSessionMainLoopModelOverrideWinsOverConfiguredMainLoopModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.InitialMainLoopModel = "claude-sonnet-4-6"
		metadata.MainLoopModelOverride = "claude-opus-4-6"
	}); err != nil {
		t.Fatalf("seed session model metadata: %v", err)
	}
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
		MainLoopModel:    "claude-sonnet-4-6",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want session override", requests[0].Model)
	}
}

func TestQueryEngineUsesDefaultOpusForOpusplanInPlanMode(t *testing.T) {
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
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "opusplan",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want default opus plan-mode resolution", requests[0].Model)
	}
}

func TestQueryEngineUsesAnthropicDefaultOpusModelEnvForOpusplanInPlanMode(t *testing.T) {
	t.Setenv("ANTHROPIC_DEFAULT_OPUS_MODEL", "claude-opus-env")

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
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "opusplan",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-opus-env" {
		t.Fatalf("request model = %q, want env-overridden default opus", requests[0].Model)
	}
}

func TestQueryEngineUpgradesHaikuToDefaultSonnetInPlanMode(t *testing.T) {
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
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "haiku",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-sonnet-4-6" {
		t.Fatalf("request model = %q, want default sonnet plan-mode resolution", requests[0].Model)
	}
}

func TestQueryEngineUsesAnthropicDefaultSonnetModelEnvForHaikuInPlanMode(t *testing.T) {
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "claude-sonnet-env")

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
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "haiku",
		LLMProvider:   "openai-compatible",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-sonnet-env" {
		t.Fatalf("request model = %q, want env-overridden default sonnet", requests[0].Model)
	}
}

func TestQueryEngineUsesThirdPartyDefaultSonnetForHaikuInPlanMode(t *testing.T) {
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
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "haiku",
		LLMProvider:   "openai-compatible",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-sonnet-4-5" {
		t.Fatalf("request model = %q, want provider-sensitive third-party sonnet fallback", requests[0].Model)
	}
}

func TestQueryEngineResolvesSonnetAliasIntoDefaultSonnetModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "ok"},
				{Type: "message.end"},
			},
		},
	}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "sonnet",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-sonnet-4-6" {
		t.Fatalf("request model = %q, want default sonnet model", requests[0].Model)
	}
}

func TestQueryEngineResolvesBestAliasIntoDefaultOpusModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "ok"},
				{Type: "message.end"},
			},
		},
	}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "best",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want default opus model", requests[0].Model)
	}
}

func TestQueryEngineResolvesOpusplanIntoDefaultSonnetOutsidePlanMode(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "ok"},
				{Type: "message.end"},
			},
		},
	}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "opusplan",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-sonnet-4-6" {
		t.Fatalf("request model = %q, want default sonnet model outside plan mode", requests[0].Model)
	}
}

func TestQueryEngineKeepsResolvedSonnetForOpusplanInPlanModeWhenContextExceeds200kTokens(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if _, err := sessions.AppendMessage(sess.ID, "user", strings.Repeat("a", 800100)); err != nil {
		t.Fatalf("append large history message: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "done"},
				{Type: "message.end"},
			},
		},
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          client,
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode:     permissions.ModeWorkspaceWrite,
			PlanMode: true,
		},
		MainLoopModel: "opusplan",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-sonnet-4-6" {
		t.Fatalf("request model = %q, want resolved default sonnet model when context exceeds 200k tokens", requests[0].Model)
	}
}

func TestQueryEngineResolvesSessionOverrideAliasIntoConcreteRuntimeModel(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.InitialMainLoopModel = "claude-sonnet-4-6"
		metadata.MainLoopModelOverride = "best"
	}); err != nil {
		t.Fatalf("seed session model metadata: %v", err)
	}
	client := &scriptedClient{
		scripts: [][]llm.StreamEvent{
			{
				{Type: "text.delta", Delta: "ok"},
				{Type: "message.end"},
			},
		},
	}
	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           client,
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "claude-sonnet-4-6",
	})

	if got := engine.ResolvedMainLoopModelForSession(sess.ID); got != "claude-opus-4-6" {
		t.Fatalf("resolved main loop model = %q, want default opus model from best alias", got)
	}
	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Model != "claude-opus-4-6" {
		t.Fatalf("request model = %q, want resolved alias override", requests[0].Model)
	}
}

func TestQueryEngineSubmitPromptBootstrapsStoredHistoryBeforeAppendingNewUserMessage(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if _, err := sessions.AppendMessage(sess.ID, "user", "first"); err != nil {
		t.Fatalf("append first message: %v", err)
	}
	if _, err := sessions.AppendMessage(sess.ID, "assistant", "second"); err != nil {
		t.Fatalf("append second message: %v", err)
	}
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

	if err := engine.SubmitPrompt(context.Background(), sess, "third", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if len(requests[0].History) != 3 {
		t.Fatalf("history length = %d, want full stored history plus new prompt", len(requests[0].History))
	}
	if requests[0].History[0].Content != "first" || requests[0].History[1].Content != "second" || requests[0].History[2].Content != "third" {
		t.Fatalf("history = %#v, want ordered stored transcript plus new prompt", requests[0].History)
	}
}

func TestQueryEngineOverrideSystemPromptWinsOverOtherPromptInputs(t *testing.T) {
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
		Sessions:             sessions,
		Client:               client,
		WorkspaceLoader:      workspace.NewLoader(""),
		PermissionPolicy:     permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		CustomSystemPrompt:   "custom prompt",
		AgentSystemPrompt:    "agent prompt",
		AppendSystemPrompt:   "append prompt",
		OverrideSystemPrompt: "override prompt",
	})

	if err := engine.SubmitPrompt(context.Background(), sess, "hello", &captureSink{}); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	requests := client.Requests()
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	if requests[0].Context.SystemPrompt != "override prompt" {
		t.Fatalf("system prompt = %q, want override prompt only", requests[0].Context.SystemPrompt)
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

func TestQueryEngineCompactionPersistsBoundaryMessageIntoSessionTranscript(t *testing.T) {
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

	sink := &captureSink{}
	if err := engine.SubmitMessage(context.Background(), sess, msg, sink); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}

	foundBoundary := false
	for _, message := range messages {
		if message.Role == "system" && message.Content == "[compact_boundary]" {
			foundBoundary = true
			break
		}
	}
	if !foundBoundary {
		t.Fatalf("messages = %#v, want persisted compact boundary in transcript", messages)
	}
}

func TestQueryEngineLoadsContinuationSafeTranscriptViewFromSessionStore(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-1", SessionID: sess.ID, Role: "user", Content: "old user", CreatedAt: now},
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(3 * time.Second)},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	messages := engine.Messages(sess.ID)
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(messages))
	}
	if messages[0].Role != "summary" || messages[0].Content != "Summary: compacted" {
		t.Fatalf("first message = %#v, want continuation summary", messages[0])
	}
	if messages[1].Content != "[compact_boundary]" {
		t.Fatalf("second message = %#v, want compact boundary", messages[1])
	}
	if messages[2].Content != "latest assistant" {
		t.Fatalf("third message = %#v, want post-boundary tail", messages[2])
	}
}

func TestQueryEngineCompactionUpdatesSessionRecoveryMetadata(t *testing.T) {
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
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello compact metadata")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

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

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	updated, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if updated.Metadata.LastCompactBoundaryID == "" {
		t.Fatalf("metadata = %#v, want compact boundary id", updated.Metadata)
	}
	if updated.Metadata.LastCompactionSummaryID == "" {
		t.Fatalf("metadata = %#v, want summary id", updated.Metadata)
	}
	if updated.Metadata.LastCompactionReason != string(compaction.ReasonMessageLimit) {
		t.Fatalf("metadata = %#v, want compaction reason %q", updated.Metadata, compaction.ReasonMessageLimit)
	}
	if updated.Metadata.LastCompactedAt.IsZero() {
		t.Fatalf("metadata = %#v, want compaction timestamp", updated.Metadata)
	}
}

func TestQueryEngineHydratesRecoveryStateFromPersistedSessionMetadata(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-1", SessionID: sess.ID, Role: "user", Content: "old user", CreatedAt: now},
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted", CreatedAt: now.Add(time.Second)},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(2 * time.Second)},
		{ID: "msg-2", SessionID: sess.ID, Role: "user", Content: "latest user", CreatedAt: now.Add(3 * time.Second)},
		{ID: "msg-3", SessionID: sess.ID, Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(4 * time.Second)},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastUserMessageID = "msg-2"
		metadata.LastAssistantMessageID = "msg-3"
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
		metadata.LastCompactionReason = string(compaction.ReasonMessageLimit)
		metadata.LastCompactedAt = now.Add(2 * time.Second)
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	messages := engine.Messages(sess.ID)
	state := engine.State()

	if len(messages) != 4 {
		t.Fatalf("message count = %d, want 4 continuation messages", len(messages))
	}
	if state.LastSessionID != sess.ID {
		t.Fatalf("last session id = %q, want %q", state.LastSessionID, sess.ID)
	}
	if state.MessageCount != 4 {
		t.Fatalf("message count state = %d, want 4", state.MessageCount)
	}
	if state.LastCompactBoundaryID != "compact-1" {
		t.Fatalf("state = %#v, want recovered compact boundary id", state)
	}
	if state.LastCompactionSummaryID != "summary-1" {
		t.Fatalf("state = %#v, want recovered compaction summary id", state)
	}
	if state.LastCompactionReason != string(compaction.ReasonMessageLimit) {
		t.Fatalf("state = %#v, want recovered compaction reason", state)
	}
	if state.LastCompactionPhase != "restored" {
		t.Fatalf("state = %#v, want restored compaction phase", state)
	}
	if state.LastUserInput != "latest user" {
		t.Fatalf("last user input = %q, want latest user", state.LastUserInput)
	}
	if state.LastAssistantReply != "latest assistant" {
		t.Fatalf("last assistant reply = %q, want latest assistant", state.LastAssistantReply)
	}
}

func TestQueryEngineHydratesRecoveryStateFromSynthesizedCompactionAnchors(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "latest assistant", CreatedAt: now},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastAssistantMessageID = "msg-2"
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
		metadata.LastCompactionReason = string(compaction.ReasonMessageLimit)
		metadata.LastCompactedAt = now.Add(-time.Second)
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	_ = engine.Messages(sess.ID)
	state := engine.State()
	if state.LastCompactBoundaryID != "compact-1" {
		t.Fatalf("state = %#v, want synthesized compact boundary id", state)
	}
	if state.LastCompactionSummaryID != "summary-1" {
		t.Fatalf("state = %#v, want synthesized compaction summary id", state)
	}
	if state.LastCompactionReason != string(compaction.ReasonMessageLimit) {
		t.Fatalf("state = %#v, want recovered compaction reason", state)
	}
	if state.LastCompactionPhase != "restored" {
		t.Fatalf("state = %#v, want restored compaction phase", state)
	}
}

func TestQueryEngineHydratesCompactBoundaryCountFromRecoveryAnchors(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted", CreatedAt: now},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(time.Second)},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
		metadata.LastCompactionReason = string(compaction.ReasonMessageLimit)
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	_ = engine.Messages(sess.ID)
	state := engine.State()
	if state.CompactBoundaryCount != 1 {
		t.Fatalf("state = %#v, want compact boundary count restored to 1", state)
	}
}

func TestQueryEngineMicrocompactsOlderToolResultsAtWarningThreshold(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	for _, entry := range []struct {
		role    string
		content string
	}{
		{"user", "first prompt"},
		{"tool", "system.run: " + strings.Repeat("x", 200)},
		{"assistant", "used tool"},
	} {
		if _, err := sessions.AppendMessage(sess.ID, entry.role, entry.content); err != nil {
			t.Fatalf("append seed message: %v", err)
		}
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeDangerFullAccess,
		},
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:             99,
			ContextWindowTokens:     100,
			WarningBufferTokens:     20,
			ErrorBufferTokens:       10,
			AutoCompactBufferTokens: 5,
			BlockingBufferTokens:    2,
			PreserveRecentTurns:     2,
			SummaryPrefix:           "Summary:",
		}),
	})

	sink := &captureSink{}
	if err := engine.SubmitPrompt(context.Background(), sess, strings.Repeat("z", 120), sink); err != nil {
		t.Fatalf("submit prompt: %v", err)
	}

	foundMicro := false
	for _, event := range sink.events {
		if event.Type == "compact.micro" {
			foundMicro = true
			break
		}
	}
	if !foundMicro {
		t.Fatalf("events = %#v, want compact.micro event", sink.events)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for session %q not found", sess.ID)
	}
	foundClearedTool := false
	for _, msg := range messages {
		if msg.Role == "tool" && msg.Content == "system.run: [Old tool result content cleared]" {
			foundClearedTool = true
			break
		}
	}
	if !foundClearedTool {
		t.Fatalf("messages = %#v, want cleared historical tool result", messages)
	}
}

func TestQueryEngineCompactionUsesPersistedSummaryMemoryAndTracksSummarizedAnchor(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()
	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-1", SessionID: sess.ID, Role: "user", Content: "already summarized prompt", CreatedAt: now},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "already summarized answer", CreatedAt: now.Add(time.Second)},
		{ID: "msg-3", SessionID: sess.ID, Role: "user", Content: strings.Repeat("c", 40), CreatedAt: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("replace seed messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastSummarizedMessageID = "msg-2"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello rolling summary memory")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	memSvc := memory.NewService()
	memSvc.SaveCompactionSummary(session.Session{
		ID:      sess.ID,
		Key:     sess.Key,
		AgentID: sess.AgentID,
		IsMain:  sess.IsMain,
	}, session.Message{
		ID:        "summary-prev",
		SessionID: sess.ID,
		Role:      "summary",
		Content:   "Summary: previous summarized context",
	})

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		MemoryService:   memSvc,
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:                99,
			MaxEstimatedTokens:         20,
			PreserveRecentTurns:        2,
			SummaryPrefix:              "Summary:",
			SessionMemoryMinTokens:     1,
			SessionMemoryMinTextBlocks: 1,
			SessionMemoryMaxTokens:     100,
		}),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	items := memSvc.List(sess.ID)
	if len(items) != 1 {
		t.Fatalf("memory count = %d, want single summary memory", len(items))
	}
	if items[0].Content != "Summary: previous summarized context" {
		t.Fatalf("memory content = %q, want persisted session memory summary unchanged", items[0].Content)
	}
	updated, ok := sessions.GetByID(sess.ID)
	if !ok {
		t.Fatalf("session %q not found", sess.ID)
	}
	if updated.Metadata.LastSummarizedMessageID == "" {
		t.Fatalf("metadata = %#v, want last summarized anchor", updated.Metadata)
	}
}

func TestQueryEngineSessionMemoryCompactInjectsCompactHookAndTranscriptNote(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()
	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-1", SessionID: sess.ID, Role: "user", Content: "already summarized prompt", CreatedAt: now},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "already summarized answer", CreatedAt: now.Add(time.Second)},
		{ID: "msg-3", SessionID: sess.ID, Role: "user", Content: strings.Repeat("c", 40), CreatedAt: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("replace seed messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastSummarizedMessageID = "msg-2"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	msg, err := sessions.AppendMessage(sess.ID, "user", "hello compact hook")
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}

	memSvc := memory.NewService()
	memSvc.SaveCompactionSummary(sess, session.Message{
		ID:        "summary-prev",
		SessionID: sess.ID,
		Role:      "summary",
		Content:   "Summary: previous summarized context",
	})

	engine := queryengine.New(queryengine.Config{
		Sessions:        sessions,
		Client:          llm.NewMockClient(),
		WorkspaceLoader: workspace.NewLoader(""),
		MemoryService:   memSvc,
		Compactor: compaction.NewService(compaction.Config{
			MaxMessages:                2,
			ContextWindowTokens:        1000,
			SessionMemoryMinTokens:     1,
			SessionMemoryMinTextBlocks: 1,
			SessionMemoryMaxTokens:     100,
		}),
		SessionStartCompactHook: sessionStartCompactHookFunc(func(_ context.Context, hookSession session.Session) ([]session.Message, error) {
			if hookSession.ID != sess.ID {
				t.Fatalf("hook session = %#v, want %s", hookSession, sess.ID)
			}
			return []session.Message{{ID: "hook-compact", SessionID: sess.ID, Role: "system", Content: "compact hook context"}}, nil
		}),
		TranscriptPathProvider: func(session.Session) string {
			return "C:/tmp/transcript.jsonl"
		},
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	if err := engine.SubmitMessage(context.Background(), sess, msg, &captureSink{}); err != nil {
		t.Fatalf("submit message: %v", err)
	}

	messages, ok := sessions.Messages(sess.ID)
	if !ok {
		t.Fatalf("messages for %q not found", sess.ID)
	}
	if !containsMessageContent(messages, "compact hook context") {
		t.Fatalf("messages = %#v, want compact hook context reinjected", messages)
	}
	if !containsMessageContent(messages, "C:/tmp/transcript.jsonl") {
		t.Fatalf("messages = %#v, want transcript note", messages)
	}
}

func TestQueryEngineHydratesCompactionPhaseFromRecoveryAnchorsWithoutReason(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "summary-1", SessionID: sess.ID, Role: "summary", Content: "Summary: compacted", CreatedAt: now},
		{ID: "compact-1", SessionID: sess.ID, Role: "system", Content: "[compact_boundary]", CreatedAt: now.Add(time.Second)},
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "latest assistant", CreatedAt: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	_ = engine.Messages(sess.ID)
	state := engine.State()
	if state.LastCompactionPhase != "restored" {
		t.Fatalf("state = %#v, want restored compaction phase when compaction anchors exist", state)
	}
}

func TestQueryEngineRestoresSynthesizedCompactionViewWhenTranscriptOnlyHasTail(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	now := time.Now().UTC()

	if err := sessions.ReplaceMessages(sess.ID, []session.Message{
		{ID: "msg-2", SessionID: sess.ID, Role: "assistant", Content: "tail after cleanup", CreatedAt: now},
	}); err != nil {
		t.Fatalf("replace messages: %v", err)
	}
	if err := sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
		metadata.LastAssistantMessageID = "msg-2"
		metadata.LastCompactBoundaryID = "compact-1"
		metadata.LastCompactionSummaryID = "summary-1"
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}

	engine := queryengine.New(queryengine.Config{
		Sessions:         sessions,
		Client:           llm.NewMockClient(),
		WorkspaceLoader:  workspace.NewLoader(""),
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})

	messages := engine.Messages(sess.ID)
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want synthesized summary + boundary + tail", len(messages))
	}
	if messages[0].ID != "summary-1" || messages[0].Role != "summary" {
		t.Fatalf("first message = %#v, want synthesized summary anchor", messages[0])
	}
	if messages[1].ID != "compact-1" || messages[1].Content != "[compact_boundary]" {
		t.Fatalf("second message = %#v, want synthesized compact boundary", messages[1])
	}
	if messages[2].ID != "msg-2" || messages[2].Content != "tail after cleanup" {
		t.Fatalf("third message = %#v, want preserved post-compact tail", messages[2])
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
		Sessions:             sessions,
		Client:               llm.NewMockClient(),
		WorkspaceLoader:      workspace.NewLoader(""),
		PermissionPolicy:     permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		EstimatedTokenBudget: 5,
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

func containsPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func containsMessageContent(messages []session.Message, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, want) {
			return true
		}
	}
	return false
}

func assertJSONInput(t *testing.T, got string, want map[string]any) {
	t.Helper()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("input = %q, want JSON object: %v", got, err)
	}
	assertAnyMap(t, parsed, want)
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

func assertToolMessageRawBlock(t *testing.T, messages []session.Message, blockType, hookName, toolUseID string, fields map[string]any) {
	t.Helper()
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw == nil ||
				block.Raw["type"] != blockType ||
				block.Raw["hookName"] != hookName ||
				block.Raw["toolUseID"] != toolUseID {
				continue
			}
			for key, want := range fields {
				if block.Raw[key] != want {
					t.Fatalf("block[%q] = %#v, want %#v in %#v", key, block.Raw[key], want, block.Raw)
				}
			}
			return
		}
	}
	t.Fatalf("messages = %#v, want raw block type=%q hookName=%q toolUseID=%q", messages, blockType, hookName, toolUseID)
}

func countToolMessageRawBlocks(messages []session.Message, blockType, hookName, toolUseID string) int {
	count := 0
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			if block.Raw != nil &&
				block.Raw["type"] == blockType &&
				block.Raw["hookName"] == hookName &&
				block.Raw["toolUseID"] == toolUseID {
				count++
			}
		}
	}
	return count
}

func cloneAnyMap(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func runCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %s: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
}
