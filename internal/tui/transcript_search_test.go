package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTranscriptSearchCtrlFActivatesSearchMode(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.setSize(80, 16)

	updated, cmd := model.Update(testKey(keyCtrlF))
	model = updated.(Model)

	if cmd != nil {
		t.Fatalf("ctrl+f cmd = %v, want nil", cmd)
	}
	if !model.viewport.Search.Active || !model.viewport.TranscriptMode {
		t.Fatalf("search state = %#v transcriptMode=%v, want active transcript search", model.viewport.Search, model.viewport.TranscriptMode)
	}
	if !contains(model.viewContent(), "Search:") {
		t.Fatalf("view missing search prompt: %q", model.viewContent())
	}
}

func TestTranscriptSearchQueryScrollsToMatchingLine(t *testing.T) {
	model := transcriptSearchModelWithMessages(18, map[int]string{3: "needle in old message"})
	model.setSize(80, 16)
	if contains(model.viewContent(), "needle in old message") {
		t.Fatalf("baseline short viewport unexpectedly contains old needle: %q", model.viewContent())
	}

	model = updateSearchKeys(model, testKey(keyCtrlF))
	model = typeSearchRunes(model, "needle")
	view := model.viewContent()

	if !contains(view, "needle in old message") {
		t.Fatalf("search view missing matching old message: %q", view)
	}
	if !contains(view, "Search: needle 1/1") {
		t.Fatalf("search view missing match count: %q", view)
	}
}

func TestTranscriptSearchEnterAndCtrlPNavigateMatches(t *testing.T) {
	model := transcriptSearchModelWithMessages(18, map[int]string{
		3:  "needle first",
		16: "needle second",
	})
	model.setSize(80, 16)
	model = updateSearchKeys(model, testKey(keyCtrlF))
	model = typeSearchRunes(model, "needle")
	firstView := model.viewContent()
	if !contains(firstView, "needle first") || contains(firstView, "needle second") {
		t.Fatalf("initial search view = %q, want first match", firstView)
	}

	model = updateSearchKeys(model, testKey(keyEnter))
	nextView := model.viewContent()
	if !contains(nextView, "needle second") || contains(nextView, "needle first") || !contains(nextView, "Search: needle 2/2") {
		t.Fatalf("next search view = %q, want second match", nextView)
	}

	model = updateSearchKeys(model, testKey(keyCtrlP))
	prevView := model.viewContent()
	if !contains(prevView, "needle first") || contains(prevView, "needle second") || !contains(prevView, "Search: needle 1/2") {
		t.Fatalf("previous search view = %q, want first match", prevView)
	}
}

func TestTranscriptSearchNoMatchesAndBackspace(t *testing.T) {
	model := transcriptSearchModelWithMessages(8, nil)
	model.setSize(80, 16)
	model = updateSearchKeys(model, testKey(keyCtrlF))
	model = typeSearchRunes(model, "missing")

	if !contains(model.viewContent(), "Search: missing 0/0") {
		t.Fatalf("view missing no-match status: %q", model.viewContent())
	}

	model = updateSearchKeys(model, testKey(keyBackspace))
	if !contains(model.viewContent(), "Search: missin 0/0") {
		t.Fatalf("backspace did not update query: %q", model.viewContent())
	}
}

func TestTranscriptSearchEscAndCtrlCExitWithoutQuitting(t *testing.T) {
	model := transcriptSearchModelWithMessages(8, nil)
	model.setSize(80, 16)
	model = updateSearchKeys(model, testKey(keyCtrlF))

	updated, cmd := model.Update(testKey(keyEscape))
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("esc cmd = %v, want nil", cmd)
	}
	if model.viewport.Search.Active || model.viewport.TranscriptMode {
		t.Fatalf("esc did not exit search: %#v transcriptMode=%v", model.viewport.Search, model.viewport.TranscriptMode)
	}

	model = updateSearchKeys(model, testKey(keyCtrlF))
	updated, cmd = model.Update(testKey(keyCtrlC))
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("ctrl+c in search cmd = %v, want nil", cmd)
	}
	if model.viewport.Search.Active {
		t.Fatalf("ctrl+c did not exit search")
	}
}

func transcriptSearchModelWithMessages(count int, overrides map[int]string) Model {
	model := NewModel(&fakeBridge{})
	for i := 1; i <= count; i++ {
		content := fmt.Sprintf("message-%02d", i)
		if overrides != nil && overrides[i] != "" {
			content = overrides[i]
		}
		model.transcript = append(model.transcript, transcriptEntry{
			Role:    "assistant",
			Content: content,
		})
	}
	return model
}

func typeSearchRunes(model Model, text string) Model {
	for _, r := range text {
		model = updateSearchKeys(model, testKeyRunes(string(r)))
	}
	return model
}

func updateSearchKeys(model Model, msg tea.KeyMsg) Model {
	updated, _ := model.Update(msg)
	return updated.(Model)
}
