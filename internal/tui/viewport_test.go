package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

func TestViewportShowsTranscriptTailWhenTerminalIsShort(t *testing.T) {
	model := viewportModelWithMessages(14)
	model.setSize(80, 16)

	view := model.viewContent()

	if contains(view, "message-01") {
		t.Fatalf("view contains oldest message while short viewport should show tail: %q", view)
	}
	if !contains(view, "message-14") {
		t.Fatalf("view missing newest message: %q", view)
	}
}

func TestViewportPageUpAndEndControlTranscriptWindow(t *testing.T) {
	model := viewportModelWithMessages(18)
	model.setSize(80, 16)

	for i := 0; i < 4; i++ {
		updated, _ := model.Update(testKey(keyPgUp))
		model = updated.(Model)
	}
	view := model.viewContent()
	if !contains(view, "message-01") {
		t.Fatalf("page up view missing oldest message: %q", view)
	}
	if contains(view, "message-18") {
		t.Fatalf("page up view still contains newest message: %q", view)
	}

	updated, _ := model.Update(testKey(keyEnd))
	model = updated.(Model)
	view = model.viewContent()
	if contains(view, "message-01") || !contains(view, "message-18") {
		t.Fatalf("end view = %q, want bottom of transcript", view)
	}
}

func TestViewportKeepsScrollPositionWhenNewMessageArrivesWhileScrolledUp(t *testing.T) {
	model := viewportModelWithMessages(18)
	model.setSize(80, 16)
	updated, _ := model.Update(testKey(keyPgUp))
	model = updated.(Model)
	before := model.viewContent()

	model.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:    "message.created",
		Message: &session.Message{Role: "assistant", Content: "new-message"},
	}))

	view := model.viewContent()
	if contains(view, "new-message") {
		t.Fatalf("scrolled viewport followed new message unexpectedly: %q", view)
	}
	if contains(before, "message-12") && !contains(view, "message-12") {
		t.Fatalf("scrolled viewport shifted after new message: before=%q after=%q", before, view)
	}
	if !contains(view, "1 new message") {
		t.Fatalf("scrolled viewport missing new message indicator: %q", view)
	}
}

func TestViewportFollowsNewMessagesAtBottom(t *testing.T) {
	model := viewportModelWithMessages(18)
	model.setSize(80, 16)

	model.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:    "message.created",
		Message: &session.Message{Role: "assistant", Content: "new-message"},
	}))

	view := model.viewContent()
	if !contains(view, "new-message") {
		t.Fatalf("bottom viewport did not follow new message: %q", view)
	}
	if contains(view, "new message") {
		t.Fatalf("bottom viewport should not show new message indicator: %q", view)
	}
}

func TestViewportUserSubmitReturnsToBottom(t *testing.T) {
	model := viewportModelWithMessages(18)
	model.setSize(80, 16)
	for i := 0; i < 4; i++ {
		updated, _ := model.Update(testKey(keyPgUp))
		model = updated.(Model)
	}
	model.input = "my question"
	model.cursorPos = len(model.input)

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	view := model.viewContent()
	if !contains(view, "my question") || contains(view, "new message") {
		t.Fatalf("submit view = %q, want bottom with submitted input and no new message indicator", view)
	}
}

func TestViewportClearConversationResetsScrollState(t *testing.T) {
	model := viewportModelWithMessages(18)
	model.setSize(80, 16)
	for i := 0; i < 4; i++ {
		updated, _ := model.Update(testKey(keyPgUp))
		model = updated.(Model)
	}
	model.applyRuntimeEvent(clientEventFromRuntimeEvent(runtime.RuntimeEvent{
		Type:    "message.created",
		Message: &session.Message{Role: "assistant", Content: "new-message"},
	}))

	model.clearVisibleConversation()

	view := model.viewContent()
	if contains(view, "new message") || contains(view, "scrolled") {
		t.Fatalf("cleared conversation view still has viewport status: %q", view)
	}
}

func TestViewportMouseWheelScrollsTranscript(t *testing.T) {
	model := viewportModelWithMessages(18)
	model.setSize(80, 16)

	updated, _ := model.Update(testMouseWheel(tea.MouseWheelUp))
	model = updated.(Model)
	view := model.viewContent()

	if contains(view, "message-18") {
		t.Fatalf("single wheel step should move away from newest message: %q", view)
	}
	if !contains(view, "message-15") {
		t.Fatalf("single wheel step should reveal recent history: %q", view)
	}
}

func TestViewportTranscriptModeEscExitsWithoutQuitting(t *testing.T) {
	model := NewModel(&fakeBridge{})

	updated, cmd := model.Update(testKey(keyCtrlO))
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("ctrl+o cmd = %v, want nil", cmd)
	}
	if !model.viewport.TranscriptMode {
		t.Fatalf("ctrl+o did not enter transcript mode")
	}
	if !contains(model.viewContent(), "Transcript") {
		t.Fatalf("transcript mode view missing footer hint: %q", model.viewContent())
	}

	updated, cmd = model.Update(testKey(keyEscape))
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("esc in transcript mode cmd = %v, want nil", cmd)
	}
	if model.viewport.TranscriptMode {
		t.Fatalf("esc did not exit transcript mode")
	}
}

func viewportModelWithMessages(count int) Model {
	model := NewModel(&fakeBridge{})
	for i := 1; i <= count; i++ {
		model.transcript = append(model.transcript, transcriptEntry{
			Role:    "assistant",
			Content: fmt.Sprintf("message-%02d", i),
		})
	}
	return model
}
