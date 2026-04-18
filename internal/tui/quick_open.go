package tui

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const dialogKindQuickOpen = "quick-open"

const (
	quickOpenCommandPrefix = "command:"
	quickOpenSessionPrefix = "session:"
	quickOpenTaskPrefix    = "task:"
	quickOpenMCPPrefix     = "mcp:"
	quickOpenFilePrefix    = "file:"
)

const (
	quickOpenPreviewLines = 8
	quickOpenFileLimit    = 64
)

func (m *Model) openQuickOpenDialog() {
	if bridge, ok := m.bridge.(taskBridge); ok {
		m.taskPanel = bridge.TaskPanelSnapshot()
	}
	m.quickOpen.BaseItems = quickOpenBaseItems(m)
	m.quickOpen.WorkspaceRoots = m.quickOpenWorkspaceRoots()
	m.quickOpen.FileIndex = indexQuickOpenFiles(m.quickOpen.WorkspaceRoots)
	m.quickOpen.OriginalInput = m.input
	m.quickOpen.OriginalCursor = m.cursorPos
	initialItems := m.quickOpenDialogItems("")
	m.dialog.open(dialogSpec{
		Kind:           dialogKindQuickOpen,
		Title:          "Quick Open",
		Subtitle:       "Search commands, sessions, tasks, MCP servers, and workspace files",
		QueryEnabled:   true,
		Items:          initialItems,
		EmptyText:      "No quick open items",
		FooterHint:     "Type to filter | Enter open | Tab mention | Shift+Tab insert path | Esc close",
		VisibleCount:   len(initialItems),
		OriginalInput:  m.input,
		OriginalCursor: m.cursorPos,
	})
	m.updateQuickOpenPreview()
	m.clearSuggestions()
}

func quickOpenBaseItems(m *Model) []dialogItem {
	items := make([]dialogItem, 0, len(localSlashCommandSpecs)+8)
	for _, command := range localSlashCommandSpecs {
		items = append(items, dialogItem{
			Label:       "/" + command.Name,
			Value:       quickOpenCommandPrefix + command.Name,
			Description: "command | " + command.Description,
		})
	}
	if bridge, ok := m.bridge.(sessionResumeBridge); ok {
		for _, item := range sessionResumeItems(bridge.SessionSnapshots()) {
			items = append(items, dialogItem{
				Label:       item.Label,
				Value:       quickOpenSessionPrefix + item.Value,
				Description: "session | " + item.Description,
			})
		}
	}
	if len(m.taskPanel.Tasks) > 0 {
		for _, item := range taskDialogItems(m.taskPanel) {
			items = append(items, dialogItem{
				Label:       item.Label,
				Value:       quickOpenTaskPrefix + item.Value,
				Description: "task | " + item.Description,
			})
		}
	} else if bridge, ok := m.bridge.(taskBridge); ok {
		snapshot := bridge.TaskPanelSnapshot()
		for _, item := range taskDialogItems(snapshot) {
			items = append(items, dialogItem{
				Label:       item.Label,
				Value:       quickOpenTaskPrefix + item.Value,
				Description: "task | " + item.Description,
			})
		}
	}
	if bridge, ok := m.bridge.(mcpStatusBridge); ok {
		for _, server := range bridge.MCPSnapshot().Servers {
			items = append(items, dialogItem{
				Label:       server.Name,
				Value:       quickOpenMCPPrefix + server.Name,
				Description: "mcp | " + mcpServerSummary(server),
			})
		}
	}
	return items
}

func (m *Model) quickOpenWorkspaceRoots() []string {
	if bridge, ok := m.bridge.(platformStatusBridge); ok {
		roots := bridge.PlatformStatusSnapshot().WorkspaceRoots
		if len(roots) > 0 {
			return append([]string(nil), roots...)
		}
	}
	return append([]string(nil), m.quickOpen.WorkspaceRoots...)
}

func indexQuickOpenFiles(roots []string) []quickOpenFile {
	var files []quickOpenFile
	multiRoot := len(roots) > 1
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" {
			continue
		}
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		rootLabel := filepath.Base(root)
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if shouldSkipQuickOpenDir(name) && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || shouldSkipQuickOpenFile(name) {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			display := filepath.ToSlash(rel)
			if multiRoot {
				display = filepath.ToSlash(filepath.Join(rootLabel, rel))
			}
			files = append(files, quickOpenFile{
				DisplayPath:  display,
				AbsolutePath: path,
				RootLabel:    rootLabel,
			})
			return nil
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].DisplayPath < files[j].DisplayPath
	})
	return files
}

func shouldSkipQuickOpenDir(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "build", "bin", "obj", "coverage":
		return true
	default:
		return false
	}
}

func shouldSkipQuickOpenFile(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".dll") || strings.HasSuffix(lower, ".so") || strings.HasSuffix(lower, ".dylib")
}

func (m *Model) quickOpenDialogItems(query string) []dialogItem {
	items := append([]dialogItem(nil), m.quickOpen.BaseItems...)
	query = strings.TrimSpace(query)
	if query == "" {
		return items
	}
	for _, file := range m.matchQuickOpenFiles(query, quickOpenFileLimit) {
		items = append(items, dialogItem{
			Label:       file.DisplayPath,
			Value:       quickOpenFilePrefix + file.AbsolutePath,
			Description: "file | workspace",
		})
	}
	previewLines := m.quickOpenPreviewLines()
	if len(previewLines) > 0 {
		items = append(items, dialogItem{Label: "Preview", Description: m.quickOpen.PreviewTitle, Value: query, Disabled: true})
		for _, line := range previewLines {
			items = append(items, dialogItem{Label: line, Value: query, Disabled: true})
		}
	}
	return items
}

func (m *Model) matchQuickOpenFiles(query string, limit int) []quickOpenFile {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	exact := make([]quickOpenFile, 0, limit)
	fuzzy := make([]quickOpenFile, 0, limit)
	for _, file := range m.quickOpen.FileIndex {
		haystack := strings.ToLower(file.DisplayPath)
		switch {
		case strings.Contains(haystack, query):
			exact = append(exact, file)
		case isSubsequence(haystack, query):
			fuzzy = append(fuzzy, file)
		}
		if len(exact)+len(fuzzy) >= limit {
			break
		}
	}
	return append(exact, fuzzy...)
}

func (m *Model) updateQuickOpenDialog(preserveSelection bool) {
	if m.dialog.Kind != dialogKindQuickOpen {
		return
	}
	currentValue := m.dialog.Picker.Current().Value
	items := m.quickOpenDialogItems(m.dialog.Picker.Query)
	m.dialog.Items = append([]dialogItem(nil), items...)
	m.dialog.Picker.Items = append([]dialogItem(nil), items...)
	if !preserveSelection || !selectDialogItemValue(&m.dialog.Picker, currentValue) {
		m.dialog.Picker.resetFocus()
	}
	m.updateQuickOpenPreview()
	currentValue = m.dialog.Picker.Current().Value
	items = m.quickOpenDialogItems(m.dialog.Picker.Query)
	m.dialog.Items = append([]dialogItem(nil), items...)
	m.dialog.Picker.Items = append([]dialogItem(nil), items...)
	_ = selectDialogItemValue(&m.dialog.Picker, currentValue)
	m.dialog.syncPickerSelection()
}

func selectDialogItemValue(picker *listPickerState, value string) bool {
	if value == "" {
		return false
	}
	items := picker.filteredItems()
	for i, item := range items {
		if item.Value == value {
			picker.SelectedIndex = i
			picker.ensureVisible()
			return true
		}
	}
	return false
}

func (m *Model) updateQuickOpenPreview() {
	m.quickOpen.PreviewTitle = ""
	m.quickOpen.PreviewContent = ""
	item := m.dialog.Picker.Current()
	if !strings.HasPrefix(item.Value, quickOpenFilePrefix) {
		return
	}
	path := strings.TrimPrefix(item.Value, quickOpenFilePrefix)
	lines, err := readQuickOpenPreview(path, quickOpenPreviewLines)
	if err != nil {
		m.quickOpen.PreviewTitle = item.Label
		m.quickOpen.PreviewContent = "(preview unavailable)"
		return
	}
	m.quickOpen.PreviewTitle = item.Label
	m.quickOpen.PreviewContent = strings.Join(lines, "\n")
}

func readQuickOpenPreview(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024), 64*1024)
	lines := make([]string, 0, limit)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.ContainsRune(line, '\x00') {
			return nil, errors.New("binary preview unavailable")
		}
		lines = append(lines, line)
		if len(lines) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func (m Model) quickOpenPreviewLines() []string {
	if strings.TrimSpace(m.quickOpen.PreviewContent) == "" {
		return nil
	}
	return strings.Split(m.quickOpen.PreviewContent, "\n")
}

func (m *Model) acceptQuickOpenItem(item dialogItem, keyType string) {
	switch {
	case strings.HasPrefix(item.Value, quickOpenCommandPrefix):
		command := strings.TrimPrefix(item.Value, quickOpenCommandPrefix)
		m.handleLocalCommand("/" + command)
	case strings.HasPrefix(item.Value, quickOpenSessionPrefix):
		m.acceptSessionResumeItem(dialogItem{Value: strings.TrimPrefix(item.Value, quickOpenSessionPrefix)})
	case strings.HasPrefix(item.Value, quickOpenTaskPrefix):
		m.acceptTaskItem(dialogItem{Value: strings.TrimPrefix(item.Value, quickOpenTaskPrefix)})
	case strings.HasPrefix(item.Value, quickOpenMCPPrefix):
		m.acceptMCPItem(dialogItem{Value: strings.TrimPrefix(item.Value, quickOpenMCPPrefix)})
	case strings.HasPrefix(item.Value, quickOpenFilePrefix):
		m.acceptQuickOpenFile(item, keyType)
	}
}

func (m *Model) acceptQuickOpenFile(item dialogItem, keyType string) {
	absolute := strings.TrimPrefix(item.Value, quickOpenFilePrefix)
	switch keyType {
	case "tab":
		m.restoreDialogInput(m.quickOpen.OriginalInput + "@" + item.Label + " ")
	case "shift+tab":
		m.restoreDialogInput(m.quickOpen.OriginalInput + item.Label + " ")
	default:
		if m.quickOpen.OpenFile == nil {
			m.applyBridgeError(errors.New("quick open file opener is not configured"))
			return
		}
		if err := m.quickOpen.OpenFile(absolute); err != nil {
			m.applyBridgeError(err)
		}
	}
}

func (m *Model) restoreDialogInput(text string) {
	m.input = text
	m.cursorPos = len([]rune(m.input))
	m.historyIndex = -1
	m.clearSuggestions()
}
