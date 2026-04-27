package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) startTranscriptSearch() {
	m.viewport.Search = transcriptSearchState{Active: true}
	m.viewport.TranscriptMode = true
	m.viewport.StickyBottom = false
	m.clearSuggestions()
	m.refreshTranscriptSearch()
}

func (m *Model) closeTranscriptSearch() {
	m.viewport.Search = transcriptSearchState{}
	m.viewport.TranscriptMode = false
}

func (m *Model) handleTranscriptSearchKey(msg tea.KeyMsg) bool {
	if !m.viewport.Search.Active {
		return false
	}
	switch keyEventType(msg) {
	case keyEscape, keyCtrlC:
		m.closeTranscriptSearch()
	case keyEnter, keyCtrlN:
		m.moveTranscriptSearchSelection(1)
	case keyCtrlP:
		m.moveTranscriptSearchSelection(-1)
	case keyBackspace:
		m.backspaceTranscriptSearchQuery()
	case keySpace:
		m.appendTranscriptSearchQuery(" ")
	case keyRunes:
		m.appendTranscriptSearchQuery(string(keyEventRunes(msg)))
	default:
		return false
	}
	return true
}

func (m *Model) appendTranscriptSearchQuery(text string) {
	m.viewport.Search.Query += text
	m.viewport.Search.SelectedIndex = 0
	m.refreshTranscriptSearch()
}

func (m *Model) backspaceTranscriptSearchQuery() {
	runes := []rune(m.viewport.Search.Query)
	if len(runes) == 0 {
		m.closeTranscriptSearch()
		return
	}
	m.viewport.Search.Query = string(runes[:len(runes)-1])
	m.viewport.Search.SelectedIndex = 0
	m.refreshTranscriptSearch()
}

func (m *Model) moveTranscriptSearchSelection(delta int) {
	search := &m.viewport.Search
	if search.MatchCount == 0 {
		return
	}
	search.SelectedIndex = (search.SelectedIndex + delta) % search.MatchCount
	if search.SelectedIndex < 0 {
		search.SelectedIndex += search.MatchCount
	}
	m.scrollToTranscriptSearchMatch()
}

func (m *Model) refreshTranscriptSearch() {
	search := &m.viewport.Search
	if !search.Active {
		return
	}
	lines := m.transcriptSearchLines()
	query := strings.ToLower(strings.TrimSpace(search.Query))
	search.MatchLines = search.MatchLines[:0]
	if query != "" {
		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), query) {
				search.MatchLines = append(search.MatchLines, i)
			}
		}
	}
	search.MatchCount = len(search.MatchLines)
	if search.MatchCount == 0 {
		search.SelectedIndex = 0
		return
	}
	if search.SelectedIndex >= search.MatchCount {
		search.SelectedIndex = search.MatchCount - 1
	}
	if search.SelectedIndex < 0 {
		search.SelectedIndex = 0
	}
	m.scrollToTranscriptSearchMatch()
}

func (m *Model) scrollToTranscriptSearchMatch() {
	search := &m.viewport.Search
	if !search.Active || search.MatchCount == 0 {
		return
	}
	lines := m.transcriptSearchLines()
	if len(lines) == 0 {
		return
	}
	targetLine := search.MatchLines[search.SelectedIndex]
	snapshot := newRenderSnapshot(*m, m.width)
	visibleLines := snapshot.transcriptVisibleLines()
	if visibleLines <= 0 {
		visibleLines = len(lines)
	}
	if visibleLines > 1 {
		visibleLines--
	}
	scrollOffset := len(lines) - targetLine - 1
	m.viewport.ScrollOffset = clampViewportOffset(len(lines), visibleLines, scrollOffset)
	m.viewport.StickyBottom = false
	m.viewport.NewMessages = 0
}

func (m Model) transcriptSearchLines() []string {
	snapshot := newRenderSnapshot(m, m.width)
	return newRenderer().renderTranscriptLines(snapshot)
}
