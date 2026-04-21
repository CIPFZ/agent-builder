package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/approval"
	"myclaw/internal/runtime"
)

func TestApprovalRequiredOpensOverlayAndApprovalUpdatedClosesIt(t *testing.T) {
	state := newTUIState()
	request := approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd"}

	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "permission.required", Approval: &request})

	if state.pendingApproval == nil || state.pendingApproval.ID != "approval-1" {
		t.Fatalf("pending approval = %#v, want approval-1", state.pendingApproval)
	}
	if !state.approvalDialog.active() {
		t.Fatal("approval dialog inactive after permission.required")
	}

	state.applyRuntimeEvent(runtime.RuntimeEvent{Type: "approval.updated"})

	if state.pendingApproval != nil {
		t.Fatalf("pending approval = %#v, want nil", state.pendingApproval)
	}
	if state.approvalDialog.active() {
		t.Fatal("approval dialog active after approval.updated")
	}
}

func TestApprovalOverlayConsumesInputAndEnterApprovesSelection(t *testing.T) {
	bridge := &fakeBridge{}
	model := modelWithApproval(bridge)
	model.input = "draft"
	model.cursorPos = len(model.input)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = updated.(Model)
	if model.input != "draft" {
		t.Fatalf("input = %q, want unchanged while approval dialog is active", model.input)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.approved) != 1 || bridge.approved[0] != "approval-1" {
		t.Fatalf("approved = %#v, want approval-1", bridge.approved)
	}
	if model.pendingApproval != nil || model.approvalDialog.active() {
		t.Fatalf("approval state = pending %#v active %v, want cleared", model.pendingApproval, model.approvalDialog.active())
	}
	if !model.busy {
		t.Fatal("busy = false after approve, want true")
	}
}

func TestApprovalOverlayEscapeRejectsAndCloses(t *testing.T) {
	bridge := &fakeBridge{}
	model := modelWithApproval(bridge)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model = updated.(Model)

	if len(bridge.rejected) != 1 || bridge.rejected[0] != "approval-1" {
		t.Fatalf("rejected = %#v, want approval-1", bridge.rejected)
	}
	if model.pendingApproval != nil || model.approvalDialog.active() {
		t.Fatalf("approval state = pending %#v active %v, want cleared", model.pendingApproval, model.approvalDialog.active())
	}
	if model.busy {
		t.Fatal("busy = true after reject, want false")
	}
}

func TestApprovalOverlayCanSelectRejectWithDownEnter(t *testing.T) {
	bridge := &fakeBridge{}
	model := modelWithApproval(bridge)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	if len(bridge.rejected) != 1 || bridge.rejected[0] != "approval-1" {
		t.Fatalf("rejected = %#v, want approval-1", bridge.rejected)
	}
	if len(bridge.approved) != 0 {
		t.Fatalf("approved = %#v, want none", bridge.approved)
	}
}

func TestApprovalOverlayRendersAfterPromptAndHidesCommandDialog(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openHelpDialog()
	if !model.dialog.active() {
		t.Fatal("help dialog inactive before approval")
	}

	request := approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd", Reason: "needs approval"}
	model.applyRuntimeEvent(runtime.RuntimeEvent{Type: "permission.required", Approval: &request})

	if model.dialog.active() {
		t.Fatal("command dialog still active after approval dialog opened")
	}

	view := model.View()
	for _, want := range []string{"Permission Required", "Approve once", "Reject", "Esc reject"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
	if approvalIndex, promptIndex := strings.LastIndex(view, "Permission Required"), strings.Index(view, "> "); approvalIndex <= promptIndex {
		t.Fatalf("approval dialog should render after prompt: approval index %d prompt index %d view %q", approvalIndex, promptIndex, view)
	}
	if strings.Contains(view, "Available local TUI commands") {
		t.Fatalf("view includes command dialog while approval is active: %q", view)
	}
}

func modelWithApproval(bridge *fakeBridge) Model {
	model := NewModel(bridge)
	request := approval.Request{ID: "approval-1", ToolName: "system.run", ToolInput: "pwd", Reason: "needs approval"}
	updated, _ := model.Update(RuntimeEventMsg{Event: runtime.RuntimeEvent{Type: "permission.required", Approval: &request}})
	return updated.(Model)
}
