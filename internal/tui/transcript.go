package tui

func (m *Model) applyRuntimeEvent(event clientEvent) {
	m.tuiState.applyRuntimeEvent(event)
	m.refreshTranscriptSearch()
}

func (m *Model) updateRuntimeEvent(event clientEvent) {
	m.applyRuntimeEvent(event)
}
