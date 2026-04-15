package tui

import "myclaw/internal/runtime"

type Bridge interface {
	SendUserMessage(string) error
	Approve(string) error
	Reject(string) error
}

type RuntimeEventMsg struct {
	Event runtime.RuntimeEvent
}

type BridgeErrMsg struct {
	Err error
}

type transcriptEntry struct {
	Role       string
	Content    string
	Streaming  bool
	ToolName   string
	ToolInput  string
	ToolStatus string
}

type ModelConfig struct {
	SessionID string
	LLMLabel  string
	LogPath   string
}

type diagnosticsState struct {
	SessionID  string
	LLMLabel   string
	LogPath    string
	LastEvent  string
	LastError  string
	EventCount int
	LastMsg    string
}

type activityState struct {
	Label string
}

type inputState struct {
	input         string
	cursorPos     int
	history       []string
	historyIndex  int
	suggestions   []string
	selectedIndex int
}

func newInputState() inputState {
	return inputState{
		history:      make([]string, 0, 32),
		historyIndex: -1,
	}
}

type renderer struct{}

func newRenderer() renderer {
	return renderer{}
}
