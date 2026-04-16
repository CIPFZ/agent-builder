package tui

import (
	"myclaw/internal/model"
	"myclaw/internal/runtime"
)

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
	Kind                string
	Role                string
	Content             string
	Streaming           bool
	Blocks              []model.MessageBlock
	ToolUseID           string
	ToolName            string
	ToolInput           string
	ToolInputObject     map[string]any
	ToolStatus          string
	ToolError           bool
	ToolProgressType    string
	ToolProgressMessage string
	ToolProgressOutput  string
	LocalStdout         string
	LocalStderr         string
}

const (
	toolStatusRunning   = "running"
	toolStatusSucceeded = "succeeded"
	toolStatusFailed    = "failed"
)

const (
	messageKindCompact      = "compact"
	messageKindError        = "error"
	messageKindLocalCommand = "local-command"
	messageKindSystem       = "system"
)

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

type viewportState struct {
	ScrollOffset   int
	StickyBottom   bool
	TranscriptMode bool
	ShowAllHistory bool
	NewMessages    int
	Search         transcriptSearchState
}

type transcriptSearchState struct {
	Active        bool
	Query         string
	MatchCount    int
	SelectedIndex int
	MatchLines    []int
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
	Title          string
	Subtitle       string
	Items          []dialogItem
	EmptyText      string
	FooterHint     string
	QueryEnabled   bool
	VisibleCount   int
	Kind           string
	OriginalInput  string
	OriginalCursor int
}

type dialogState struct {
	Title          string
	Subtitle       string
	Items          []dialogItem
	SelectedIndex  int
	EmptyText      string
	FooterHint     string
	Picker         listPickerState
	Kind           string
	OriginalInput  string
	OriginalCursor int
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
