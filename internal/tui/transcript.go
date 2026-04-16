package tui

import "myclaw/internal/runtime"

func (m *Model) applyRuntimeEvent(event runtime.RuntimeEvent) {
	m.tuiState.applyRuntimeEvent(event)
	m.refreshTranscriptSearch()
}

func (m *Model) updateRuntimeEvent(event runtime.RuntimeEvent) {
	m.applyRuntimeEvent(event)
}
