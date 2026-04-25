package tui

import (
	"os"
	"path/filepath"
	"testing"

	"myclaw/internal/runtime"
	"myclaw/internal/session"
)

func TestSlashClearClearsVisibleConversationWithoutSendingRuntimeMessage(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.transcript = []transcriptEntry{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	model.events = append(model.events, "old event")
	model.busy = true
	model.input = "/clear"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if len(model.transcript) != 0 {
		t.Fatalf("transcript = %#v, want cleared", model.transcript)
	}
	if model.busy {
		t.Fatal("busy = true, want false after local clear")
	}
	if model.input != "" || model.cursorPos != 0 {
		t.Fatalf("input/cursor = %q/%d, want cleared", model.input, model.cursorPos)
	}
	if len(model.events) == 0 || model.events[len(model.events)-1] != "conversation cleared" {
		t.Fatalf("events = %#v, want trailing conversation cleared", model.events)
	}
}

func TestSlashSessionOpensSessionDialogWithRuntimeMetadata(t *testing.T) {
	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			SessionID:        "main-000001",
			SessionKey:       "agent:main:main",
			AgentID:          "main",
			IsMain:           true,
			WorkspaceRoots:   []string{"C:/repo", "C:/repo/subdir"},
			ModelOverride:    "claude-opus-4-6",
			MCPServerCount:   2,
			MCPToolCount:     5,
			MCPPromptCount:   1,
			MCPResourceCount: 3,
		},
	}
	model := NewModel(bridge, ModelConfig{
		SessionID: "main-000001",
		LLMLabel:  "openai-compatible / LongCat-Flash-Chat",
		LogPath:   "logs/myclaw.jsonl",
	})
	model.input = "/session"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if !model.dialog.active() || model.dialog.Title != "Session" {
		t.Fatalf("dialog = %#v, want session dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{
		"Session",
		"main-000001",
		"LongCat-Flash-Chat",
		"logs/myclaw.jsonl",
		"agent:main:main",
		"main",
		"main session",
		"C:/repo",
		"C:/repo/subdir",
		"claude-opus-4-6",
		"2 servers",
		"5 tools",
		"1 prompts",
		"3 resources",
	} {
		if !contains(view, want) {
			t.Fatalf("session view missing %q: %q", want, view)
		}
	}
}

func TestSlashDebugOpensDiagnosticsDialogWithLatestState(t *testing.T) {
	model := NewModel(&fakeBridge{})
	updated, _ := model.Update(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "agent.lifecycle.start"})})
	model = updated.(Model)
	updated, _ = model.Update(BridgeErrMsg{Err: assertErr("boom")})
	model = updated.(Model)
	model.input = "/debug"
	model.cursorPos = len([]rune(model.input))

	updated, _ = model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Diagnostics" {
		t.Fatalf("dialog = %#v, want diagnostics dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"Diagnostics", "Last event", "agent.lifecycle.start", "Last error", "boom", "Event count", "1"} {
		if !contains(view, want) {
			t.Fatalf("debug view missing %q: %q", want, view)
		}
	}
}

func TestSlashCompactOpensCompactionDialogWithRecentEvents(t *testing.T) {
	model := NewModel(&fakeBridge{
		compactionStatus: compactionSnapshot{
			EstimatedTokens:      140,
			WarningThreshold:     160,
			ErrorThreshold:       192,
			AutoCompactThreshold: 200,
			BlockingThreshold:    248,
			LastCompactionReason: "message-limit",
			LastCompactedAtLabel: "2026-04-19 10:30 UTC",
		},
	})
	updated, _ := model.Update(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "compact.warning"})})
	model = updated.(Model)
	updated, _ = model.Update(RuntimeEventMsg{Event: clientEventFromRuntimeEvent(runtime.RuntimeEvent{Type: "compact.cleaned"})})
	model = updated.(Model)
	model.input = "/compact"
	model.cursorPos = len([]rune(model.input))

	updated, _ = model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Compaction" {
		t.Fatalf("dialog = %#v, want compaction dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{
		"Compaction",
		"Manual compaction",
		"Microcompact tool output",
		"140 tokens",
		"message-limit",
		"2026-04-19 10:30 UTC",
		"compact.warning",
		"compact.cleaned",
	} {
		if !contains(view, want) {
			t.Fatalf("compact view missing %q: %q", want, view)
		}
	}
}

func TestHelpDialogListsOnlyImplementedLocalCommandDescriptions(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.openHelpDialog()
	view := model.viewContent()

	for _, want := range []string{"/clear", "Clear visible conversation", "/session", "Show session details", "/compact", "Run manual compaction", "/debug", "Show diagnostics"} {
		if !contains(view, want) {
			t.Fatalf("help view missing %q: %q", want, view)
		}
	}
	if contains(view, "pending") {
		t.Fatalf("help view still contains pending marker: %q", view)
	}
}

func TestSlashClearAliasesUseLocalClearCommand(t *testing.T) {
	for _, input := range []string{"/reset", "/new"} {
		bridge := &fakeBridge{}
		model := NewModel(bridge)
		model.transcript = []transcriptEntry{{Role: "user", Content: "hello"}}
		model.input = input
		model.cursorPos = len([]rune(model.input))

		updated, _ := model.Update(testKey(keyEnter))
		model = updated.(Model)

		if len(bridge.sent) != 0 {
			t.Fatalf("%s sent = %#v, want no runtime message", input, bridge.sent)
		}
		if len(model.transcript) != 0 {
			t.Fatalf("%s transcript = %#v, want cleared", input, model.transcript)
		}
	}
}

func TestSlashCompactAcceptsOptionalInstructionsAsLocalCommand(t *testing.T) {
	bridge := &fakeBridge{}
	model := NewModel(bridge)
	model.input = "/compact summarize tool output only"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if len(bridge.sent) != 0 {
		t.Fatalf("sent = %#v, want no runtime message", bridge.sent)
	}
	if !model.dialog.active() || model.dialog.Title != "Compaction" {
		t.Fatalf("dialog = %#v, want compaction dialog", model.dialog)
	}
	if !contains(model.dialog.Subtitle, "summarize tool output only") {
		t.Fatalf("compact subtitle = %q, want custom instructions", model.dialog.Subtitle)
	}
	found := false
	for _, item := range model.dialog.Items {
		if item.Value == "compact:summarize tool output only" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dialog items = %#v, want custom compact action", model.dialog.Items)
	}
}

func TestCompactionDialogEnterRunsManualCompactionAndAppendsTranscriptNotice(t *testing.T) {
	bridge := &fakeBridge{
		compactResult: compactionActionResult{
			Changed:        true,
			Reason:         "message-limit",
			OriginalCount:  9,
			CompactedCount: 3,
		},
	}
	model := NewModel(bridge)
	model.input = "/compact keep decisions only"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyEnter))
	model = updated.(Model)

	if len(bridge.compacts) != 1 || bridge.compacts[0] != "keep decisions only" {
		t.Fatalf("compacts = %#v, want keep decisions only", bridge.compacts)
	}
	if model.dialog.active() {
		t.Fatalf("dialog still active = %#v, want closed after compaction", model.dialog)
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].Kind != messageKindCompact {
		t.Fatalf("transcript = %#v, want compact transcript entry", model.transcript)
	}
	if !contains(model.transcript[len(model.transcript)-1].Content, "Conversation compacted") {
		t.Fatalf("last transcript = %#v, want compaction notice", model.transcript[len(model.transcript)-1])
	}
	if !contains(model.activity.Label, "Compaction completed") {
		t.Fatalf("activity = %#v, want completed label", model.activity)
	}
}

func TestCompactionDialogCanRunMicrocompact(t *testing.T) {
	bridge := &fakeBridge{
		microcompactResult: compactionActionResult{
			Changed:        true,
			Reason:         "microcompact",
			OriginalCount:  6,
			CompactedCount: 6,
		},
	}
	model := NewModel(bridge)
	model.openCompactionDialog("")
	model.dialog.Picker.setQuery("microcompact")

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if bridge.microcompactCount != 1 {
		t.Fatalf("microcompactCount = %d, want 1", bridge.microcompactCount)
	}
	if len(model.transcript) == 0 || model.transcript[len(model.transcript)-1].Kind != messageKindCompact {
		t.Fatalf("transcript = %#v, want compact transcript entry", model.transcript)
	}
}

func TestSlashTasksOpensTaskWorkbenchDialog(t *testing.T) {
	bridge := &fakeBridge{
		taskPanel: taskPanelSnapshot{
			SessionID:      "main-000001",
			RunningCount:   1,
			CompletedCount: 1,
			Tasks: []taskSnapshot{
				{
					RunID:             "agent-000001",
					Label:             "research",
					Prompt:            "inspect tui gaps",
					Status:            "running",
					RecommendedAction: "monitor",
					DecisionPriority:  "high",
					LastAssistant:     "Inspecting TUI code",
				},
				{
					RunID:             "agent-000002",
					Label:             "verify",
					Prompt:            "run tests",
					Status:            "completed",
					RecommendedAction: "close",
					DecisionPriority:  "low",
					LastAssistant:     "All tests passed",
				},
			},
		},
	}
	model := NewModel(bridge)
	model.input = "/tasks"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Tasks" {
		t.Fatalf("dialog = %#v, want tasks dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"Tasks", "running 1", "completed 1", "research", "verify", "monitor", "close"} {
		if !contains(view, want) {
			t.Fatalf("tasks view missing %q: %q", want, view)
		}
	}
}

func TestSlashKeysOpensKeybindingsDialog(t *testing.T) {
	model := NewModel(&fakeBridge{})
	model.input = "/keys"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Keybindings" {
		t.Fatalf("dialog = %#v, want keybindings dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{
		"Keybindings",
		"Ctrl+S",
		"stash",
		"Ctrl+G",
		"external editor",
		"Ctrl+X Ctrl+E",
		"Ctrl+R",
		"history search",
		"Ctrl+F",
		"transcript search",
		"Ctrl+O",
		"transcript mode",
		"Shift+Up",
		"message actions",
		"Ctrl+Y",
		"approve",
		"Ctrl+N",
		"reject",
	} {
		if !contains(view, want) {
			t.Fatalf("keybindings view missing %q: %q", want, view)
		}
	}
}

func TestTasksDialogSelectionOpensTaskDetailDialog(t *testing.T) {
	bridge := &fakeBridge{
		taskPanel: taskPanelSnapshot{
			SessionID: "main-000001",
			Tasks: []taskSnapshot{
				{
					RunID:               "agent-000001",
					Label:               "research",
					Prompt:              "inspect tui gaps",
					Status:              "running",
					ChildSessionID:      "session-agent-1",
					LastEvent:           "tool.called",
					RecommendedRole:     "reviewer",
					RecommendedAction:   "monitor",
					DecisionPriority:    "high",
					DecisionReason:      "waiting for output",
					LastAssistant:       "Inspecting TUI code",
					MessageCount:        4,
					ControlMessageCount: 1,
				},
			},
		},
	}
	model := NewModel(bridge)
	model.openTasksDialog()

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Task details" {
		t.Fatalf("dialog = %#v, want task detail dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"Task details", "agent-000001", "reviewer", "monitor", "waiting for output", "Inspecting TUI code"} {
		if !contains(view, want) {
			t.Fatalf("task detail view missing %q: %q", want, view)
		}
	}
}

func TestSlashMCPOpensFilterableServerDialog(t *testing.T) {
	bridge := &fakeBridge{
		mcpStatus: mcpSnapshot{
			Servers: []mcpServerSnapshot{
				{
					Name:          "filesystem",
					TransportType: "stdio",
					Endpoint:      "local command",
					Enabled:       true,
					Tools:         []string{"read_file", "write_file"},
					Prompts:       []string{"summarize"},
					Resources:     []string{"file://README.md"},
				},
				{
					Name:          "figma",
					TransportType: "sse",
					Endpoint:      "https://figma.example/mcp",
					Enabled:       true,
					Tools:         []string{"get_design"},
					Resources:     []string{"figma://file/123"},
				},
			},
		},
	}
	model := NewModel(bridge)
	model.input = "/mcp"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "MCP" {
		t.Fatalf("dialog = %#v, want MCP dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"MCP", "filesystem", "figma", "2 tools", "1 prompts", "1 resources", "Type to filter"} {
		if !contains(view, want) {
			t.Fatalf("mcp view missing %q: %q", want, view)
		}
	}
}

func TestMCPDialogSelectionOpensServerDetailDialog(t *testing.T) {
	bridge := &fakeBridge{
		mcpStatus: mcpSnapshot{
			Servers: []mcpServerSnapshot{
				{
					Name:          "filesystem",
					TransportType: "stdio",
					Endpoint:      "npx @modelcontextprotocol/server-filesystem C:/repo",
					Enabled:       true,
					Tools:         []string{"read_file", "write_file"},
					Prompts:       []string{"summarize"},
					Resources:     []string{"file://README.md", "file://docs/plan.md"},
				},
			},
		},
	}
	model := NewModel(bridge)
	model.openMCPDialog()

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "MCP server" {
		t.Fatalf("dialog = %#v, want MCP server detail dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{
		"MCP server",
		"filesystem",
		"stdio",
		"enabled",
		"read_file",
		"write_file",
		"summarize",
		"file://README.md",
		"file://docs/plan.md",
	} {
		if !contains(view, want) {
			t.Fatalf("mcp detail view missing %q: %q", want, view)
		}
	}
}

func TestSlashOpenOpensQuickOpenDialog(t *testing.T) {
	bridge := &fakeBridge{
		sessionSnapshots: []sessionSnapshot{
			{
				Session:          session.Session{ID: "main-000002", Key: "agent:main:main", AgentID: "main", IsMain: true},
				FirstUserMessage: "resume target",
				MessageCount:     3,
				LastMessage:      "latest answer",
			},
		},
		taskPanel: taskPanelSnapshot{
			SessionID: "main-000001",
			Tasks: []taskSnapshot{{
				RunID:        "agent-000001",
				Label:        "research",
				Status:       "running",
				Prompt:       "inspect quick open",
				MessageCount: 2,
			}},
		},
		mcpStatus: mcpSnapshot{
			Servers: []mcpServerSnapshot{{Name: "filesystem", Enabled: true, Tools: []string{"read_file"}}},
		},
	}
	model := NewModel(bridge)
	model.input = "/open"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Quick Open" {
		t.Fatalf("dialog = %#v, want quick open dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"Quick Open", "/model", "resume target", "research", "filesystem", "Type to filter"} {
		if !contains(view, want) {
			t.Fatalf("quick open view missing %q: %q", want, view)
		}
	}
}

func TestQuickOpenSelectionRoutesToExistingDialogs(t *testing.T) {
	bridge := &fakeBridge{
		taskPanel: taskPanelSnapshot{
			Tasks: []taskSnapshot{{
				RunID:             "agent-000001",
				Label:             "research",
				Status:            "running",
				Prompt:            "inspect quick open",
				RecommendedAction: "monitor",
			}},
		},
	}
	model := NewModel(bridge)
	model.openQuickOpenDialog()
	model.dialog.Picker.setQuery("research")

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Task details" {
		t.Fatalf("dialog = %#v, want task details after quick open selection", model.dialog)
	}
	if !contains(model.viewContent(), "research") {
		t.Fatalf("task details missing research: %q", model.viewContent())
	}
}

func TestQuickOpenIncludesMatchedWorkspaceFilesWithPreview(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("# Title\nquick open preview line\nthird line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}
	model := NewModel(bridge)
	model.openQuickOpenDialog()

	updated, _ := model.Update(testKeyRunes("read"))
	model = updated.(Model)

	view := model.viewContent()
	for _, want := range []string{"README.md", "file | workspace", "# Title", "quick open preview line"} {
		if !contains(view, want) {
			t.Fatalf("quick open file view missing %q: %q", want, view)
		}
	}
}

func TestQuickOpenEnterOpensFocusedWorkspaceFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "plan.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var opened string
	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}
	model := NewModel(bridge, ModelConfig{
		OpenFile: func(path string) error {
			opened = path
			return nil
		},
	})
	model.openQuickOpenDialog()

	updated, _ := model.Update(testKeyRunes("plan"))
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyEnter))
	model = updated.(Model)

	if opened != target {
		t.Fatalf("opened = %q, want %q", opened, target)
	}
	if model.dialog.active() {
		t.Fatalf("dialog still active after open: %#v", model.dialog)
	}
}

func TestQuickOpenTabAndShiftTabInsertWorkspaceFileReference(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("guide"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}

	model := NewModel(bridge)
	model.input = "inspect "
	model.cursorPos = len([]rune(model.input))
	model.openQuickOpenDialog()
	updated, _ := model.Update(testKeyRunes("guide"))
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyTab))
	model = updated.(Model)

	if model.input != "inspect @docs/guide.md " {
		t.Fatalf("input after tab = %q, want mention insert", model.input)
	}

	model = NewModel(bridge)
	model.input = "inspect "
	model.cursorPos = len([]rune(model.input))
	model.openQuickOpenDialog()
	updated, _ = model.Update(testKeyRunes("guide"))
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyShiftTab))
	model = updated.(Model)

	if model.input != "inspect docs/guide.md " {
		t.Fatalf("input after shift-tab = %q, want path insert", model.input)
	}
}

func TestSlashSearchOpensGlobalSearchDialog(t *testing.T) {
	root := t.TempDir()
	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}
	model := NewModel(bridge, ModelConfig{
		WorkspaceSearch: func(workspaceSearchRequest) (workspaceSearchResult, error) {
			return workspaceSearchResult{}, nil
		},
	})
	model.input = "/search"
	model.cursorPos = len([]rune(model.input))

	updated, _ := model.Update(testKey(keyEnter))
	model = updated.(Model)

	if !model.dialog.active() || model.dialog.Title != "Global Search" {
		t.Fatalf("dialog = %#v, want global search dialog", model.dialog)
	}
	view := model.viewContent()
	for _, want := range []string{"Global Search", "Type workspace text to search", "Type to filter"} {
		if !contains(view, want) {
			t.Fatalf("global search view missing %q: %q", want, view)
		}
	}
}

func TestGlobalSearchShowsWorkspaceMatchesAndPreview(t *testing.T) {
	root := t.TempDir()
	matchPath := filepath.Join(root, "pkg", "search.go")
	if err := os.MkdirAll(filepath.Dir(matchPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(matchPath, []byte("line one\nneedle match line\nline three\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}
	model := NewModel(bridge, ModelConfig{
		WorkspaceSearch: func(req workspaceSearchRequest) (workspaceSearchResult, error) {
			return workspaceSearchResult{
				Matches: []workspaceSearchMatch{{
					DisplayPath:  "pkg/search.go",
					AbsolutePath: matchPath,
					Line:         2,
					Text:         "needle match line",
				}},
			}, nil
		},
	})
	model.openGlobalSearchDialog()

	updated, cmd := model.Update(testKeyRunes("needle"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("search cmd = nil, want search command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	view := model.viewContent()
	for _, want := range []string{"pkg/search.go:2", "needle match line", "line one", "line three"} {
		if !contains(view, want) {
			t.Fatalf("global search results missing %q: %q", want, view)
		}
	}
}

func TestGlobalSearchEnterOpensFocusedMatchAtLine(t *testing.T) {
	root := t.TempDir()
	matchPath := filepath.Join(root, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(matchPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(matchPath, []byte("guide"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var openedPath string
	var openedLine int
	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}
	model := NewModel(bridge, ModelConfig{
		OpenFileAtLine: func(path string, line int) error {
			openedPath = path
			openedLine = line
			return nil
		},
		WorkspaceSearch: func(req workspaceSearchRequest) (workspaceSearchResult, error) {
			return workspaceSearchResult{
				Matches: []workspaceSearchMatch{{
					DisplayPath:  "docs/guide.md",
					AbsolutePath: matchPath,
					Line:         7,
					Text:         "needle",
				}},
			}, nil
		},
	})
	model.openGlobalSearchDialog()

	updated, cmd := model.Update(testKeyRunes("needle"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyEnter))
	model = updated.(Model)

	if openedPath != matchPath || openedLine != 7 {
		t.Fatalf("opened = %q:%d, want %q:%d", openedPath, openedLine, matchPath, 7)
	}
}

func TestGlobalSearchTabAndShiftTabInsertMatchReference(t *testing.T) {
	root := t.TempDir()
	matchPath := filepath.Join(root, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(matchPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(matchPath, []byte("guide"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	searcher := func(req workspaceSearchRequest) (workspaceSearchResult, error) {
		return workspaceSearchResult{
			Matches: []workspaceSearchMatch{{
				DisplayPath:  "docs/guide.md",
				AbsolutePath: matchPath,
				Line:         4,
				Text:         "guide needle",
			}},
		}, nil
	}

	bridge := &fakeBridge{
		platformStatus: platformStatusSnapshot{
			WorkspaceRoots: []string{root},
		},
	}
	model := NewModel(bridge, ModelConfig{WorkspaceSearch: searcher})
	model.input = "inspect "
	model.cursorPos = len([]rune(model.input))
	model.openGlobalSearchDialog()
	updated, cmd := model.Update(testKeyRunes("needle"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyTab))
	model = updated.(Model)

	if model.input != "inspect @docs/guide.md#L4 " {
		t.Fatalf("input after tab = %q, want mention insert", model.input)
	}

	model = NewModel(bridge, ModelConfig{WorkspaceSearch: searcher})
	model.input = "inspect "
	model.cursorPos = len([]rune(model.input))
	model.openGlobalSearchDialog()
	updated, cmd = model.Update(testKeyRunes("needle"))
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, _ = model.Update(testKey(keyShiftTab))
	model = updated.(Model)

	if model.input != "inspect docs/guide.md:4 " {
		t.Fatalf("input after shift-tab = %q, want path insert", model.input)
	}
}
