package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"myclaw/internal/model"
	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

type Bridge interface {
	SendUserMessage(string) error
	Approve(string) error
	Reject(string) error
}

type sessionResumeBridge interface {
	SessionSnapshots() []sessionSnapshot
	ResumeSession(string) (session.RecoverySnapshot, bool)
}

type taskBridge interface {
	TaskPanelSnapshot() taskPanelSnapshot
}

type platformStatusBridge interface {
	PlatformStatusSnapshot() platformStatusSnapshot
}

type mcpStatusBridge interface {
	MCPSnapshot() mcpSnapshot
}

type sessionSnapshot struct {
	Session          session.Session
	MessageCount     int
	FirstUserMessage string
	LastMessage      string
}

type platformStatusSnapshot struct {
	SessionID        string
	SessionKey       string
	AgentID          string
	IsMain           bool
	WorkspaceRoots   []string
	ModelOverride    string
	MCPServerCount   int
	MCPToolCount     int
	MCPPromptCount   int
	MCPResourceCount int
}

type mcpSnapshot struct {
	Servers []mcpServerSnapshot
}

type mcpServerSnapshot struct {
	Name          string
	TransportType string
	Endpoint      string
	Enabled       bool
	Tools         []string
	Prompts       []string
	Resources     []string
}

type taskPanelSnapshot struct {
	SessionID      string
	Tasks          []taskSnapshot
	RunningCount   int
	CompletedCount int
	FailedCount    int
	StoppedCount   int
}

type taskSnapshot struct {
	RunID               string
	Label               string
	Prompt              string
	Status              string
	ParentSessionID     string
	ChildSessionID      string
	ChildSessionKey     string
	Output              string
	Error               string
	MessageCount        int
	LastAssistant       string
	LastEvent           string
	Message             string
	NextAction          string
	RecommendedRole     string
	RecommendedAction   string
	DecisionPriority    string
	DecisionReason      string
	AutoExecutable      bool
	ControlMessageCount int
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
	SessionID      string
	LLMLabel       string
	LogPath        string
	PromptEditor   promptEditorFunc
	OpenFile       fileOpenFunc
	WorkspaceRoots []string
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

type externalEditorRequest struct {
	Prompt string
}

type promptEditorFunc func(externalEditorRequest) tea.Cmd

type externalEditorFinishedMsg struct {
	Content string
	Err     error
}

type externalEditorState struct {
	Active       bool
	PendingCtrlX bool
	PromptEditor promptEditorFunc
}

type fileOpenFunc func(path string) error

type viewportState struct {
	ScrollOffset   int
	StickyBottom   bool
	TranscriptMode bool
	ShowAllHistory bool
	NewMessages    int
	Search         transcriptSearchState
}

type quickOpenState struct {
	OpenFile       fileOpenFunc
	WorkspaceRoots []string
	BaseItems      []dialogItem
	FileIndex      []quickOpenFile
	PreviewTitle   string
	PreviewContent string
	OriginalInput  string
	OriginalCursor int
}

type quickOpenFile struct {
	DisplayPath string
	AbsolutePath string
	RootLabel   string
}

type messageActionsState struct {
	Active          bool
	SelectedIndex   int
	LastCopiedText  string
	LastCopiedLabel string
}

type toolExpansionState struct {
	expanded map[string]bool
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

type promptStashState struct {
	HasStash bool
	Input    string
	Cursor   int
	Pastes   pasteState
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
