package tui

const viewportDefaultPageLines = 8

func (s *tuiState) scrollTranscriptUp(lines int) {
	if lines <= 0 {
		lines = viewportDefaultPageLines
	}
	s.viewport.ScrollOffset += lines
	s.viewport.StickyBottom = false
}

func (s *tuiState) scrollTranscriptDown(lines int) {
	if lines <= 0 {
		lines = viewportDefaultPageLines
	}
	s.viewport.ScrollOffset -= lines
	if s.viewport.ScrollOffset <= 0 {
		s.scrollTranscriptBottom()
	}
}

func (s *tuiState) scrollTranscriptTop() {
	s.viewport.ScrollOffset = maxViewportOffset
	s.viewport.StickyBottom = false
}

func (s *tuiState) scrollTranscriptBottom() {
	s.viewport.ScrollOffset = 0
	s.viewport.StickyBottom = true
	s.viewport.NewMessages = 0
}

func (s *tuiState) toggleTranscriptMode() {
	s.viewport.TranscriptMode = !s.viewport.TranscriptMode
	if s.viewport.TranscriptMode {
		s.clearSuggestions()
	}
}

func (s *tuiState) exitTranscriptMode() {
	s.viewport.TranscriptMode = false
}

func (s *tuiState) toggleTranscriptHistory() {
	s.viewport.ShowAllHistory = !s.viewport.ShowAllHistory
}

const maxViewportOffset = int(^uint(0) >> 1)
