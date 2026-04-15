package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

func TestRuntimeBridgeStreamsAssistantEvents(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	bridge := NewRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("hello"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "assistant.delta", "message.created")
	assertHasEventType(t, events, "assistant.delta")
	assertHasEventType(t, events, "message.created")
}

func TestRuntimeBridgeSurfacesPermissionRequired(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	bridge := NewRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "permission.required")
	assertHasEventType(t, events, "permission.required")
}

func TestRuntimeBridgeDispatchesToolProgress(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	bridge := NewRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 32)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "tool.progress", "tool.result")
	var progress runtime.RuntimeEvent
	for _, event := range events {
		if event.Type == "tool.progress" {
			progress = event
			break
		}
	}
	if progress.Progress == nil || progress.Progress.ToolUseID == "" {
		t.Fatalf("progress event = %#v, want tool progress with tool use id", progress)
	}
}

func TestRuntimeBridgeDoesNotSurfaceBridgeErrorForPermissionRequired(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
	})
	bridge := NewRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	deadline := time.After(1500 * time.Millisecond)
	for {
		select {
		case raw := <-ch:
			switch msg := raw.(type) {
			case RuntimeEventMsg:
				if msg.Event.Type == "permission.required" {
					grace := time.After(250 * time.Millisecond)
					for {
						select {
						case followup := <-ch:
							if errMsg, ok := followup.(BridgeErrMsg); ok {
								t.Fatalf("unexpected BridgeErrMsg for approval-required flow: %v", errMsg.Err)
							}
						case <-grace:
							return
						}
					}
				}
			case BridgeErrMsg:
				t.Fatalf("unexpected BridgeErrMsg for approval-required flow: %v", msg.Err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for permission.required")
		}
	}
}

func TestRuntimeBridgeApproveContinuesBlockedExecution(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{
			Mode: permissions.ModeAsk,
			Rules: []permissions.Rule{{
				ToolName: "text.upper",
				Action:   permissions.ActionAsk,
				Match: permissions.Match{
					CommandContains: []string{"hello world"},
				},
			}},
		},
		ApprovalManager: approval.NewManager(),
	})
	bridge := NewRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 32)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool upper hello world"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}

	events := waitForEventTypes(t, ch, 2*time.Second, "permission.required")
	var approvalID string
	for _, event := range events {
		if event.Type == "permission.required" && event.Approval != nil {
			approvalID = event.Approval.ID
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id")
	}

	if err := bridge.Approve(approvalID); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	events = waitForEventTypes(t, ch, 2*time.Second, "tool.result", "message.created")
	assertHasEventType(t, events, "tool.result")
	assertHasEventType(t, events, "message.created")
}

func waitForRuntimeEvents(t *testing.T, ch <-chan tea.Msg, min int, timeout time.Duration) []runtime.RuntimeEvent {
	t.Helper()
	deadline := time.After(timeout)
	events := make([]runtime.RuntimeEvent, 0, min)
	for len(events) < min {
		select {
		case raw := <-ch:
			if msg, ok := raw.(RuntimeEventMsg); ok {
				events = append(events, msg.Event)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %d runtime events, got %#v", min, events)
		}
	}
	return events
}

func waitForEventTypes(t *testing.T, ch <-chan tea.Msg, timeout time.Duration, wants ...string) []runtime.RuntimeEvent {
	t.Helper()
	deadline := time.After(timeout)
	events := make([]runtime.RuntimeEvent, 0, len(wants)+2)
	seen := make(map[string]bool, len(wants))
	for {
		done := true
		for _, want := range wants {
			if !seen[want] {
				done = false
				break
			}
		}
		if done {
			return events
		}

		select {
		case raw := <-ch:
			if msg, ok := raw.(RuntimeEventMsg); ok {
				events = append(events, msg.Event)
				seen[msg.Event.Type] = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event types %v, got %#v", wants, events)
		}
	}
}

func assertHasEventType(t *testing.T, events []runtime.RuntimeEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type == want {
			return
		}
	}
	t.Fatalf("events missing %q: %#v", want, events)
}

func TestRuntimeBridgeRejectMarksApprovalRejected(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeAsk},
		ApprovalManager:  approval.NewManager(),
	})
	bridge := NewRuntimeBridge(sessions, runner, "main")

	ch := make(chan tea.Msg, 16)
	bridge.Attach(func(msg tea.Msg) { ch <- msg })

	if err := bridge.SendUserMessage("tool run pwd"); err != nil {
		t.Fatalf("SendUserMessage: %v", err)
	}
	events := waitForEventTypes(t, ch, 2*time.Second, "permission.required")
	var approvalID string
	for _, event := range events {
		if event.Type == "permission.required" && event.Approval != nil {
			approvalID = event.Approval.ID
			break
		}
	}
	if approvalID == "" {
		t.Fatal("expected approval id")
	}
	if err := bridge.Reject(approvalID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	request, ok := runner.ApprovalManager().Get(approvalID)
	if !ok || request.Status != approval.StatusRejected {
		t.Fatalf("approval after reject = %#v, ok=%v", request, ok)
	}
}

func TestRuntimeBridgeContextCancellationStopsSend(t *testing.T) {
	sessions := session.NewManager(nil)
	runner := runtime.NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, runtime.Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bridge := NewRuntimeBridgeWithContext(ctx, sessions, runner, "main", nil)
	if err := bridge.SendUserMessage("hello"); err == nil {
		t.Fatal("expected canceled bridge to reject send")
	}
}
