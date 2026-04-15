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

type dialogItem struct {
	Label       string
	Description string
	Value       string
	Disabled    bool
}

type dialogSpec struct {
	Title        string
	Subtitle     string
	Items        []dialogItem
	EmptyText    string
	FooterHint   string
	QueryEnabled bool
	VisibleCount int
}

type dialogState struct {
	Title         string
	Subtitle      string
	Items         []dialogItem
	SelectedIndex int
	EmptyText     string
	FooterHint    string
	Picker        listPickerState
}

type listPickerSpec struct {
	Items        []dialogItem
	QueryEnabled bool
	VisibleCount int
}

type listPickerState struct {
	Items            []dialogItem
	Query            string
	QueryEnabled     bool
	SelectedIndex    int
	VisibleFromIndex int
	VisibleCount     int
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
