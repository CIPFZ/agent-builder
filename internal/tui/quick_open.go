package tui

import "strings"

const dialogKindQuickOpen = "quick-open"

const (
	quickOpenCommandPrefix = "command:"
	quickOpenSessionPrefix = "session:"
	quickOpenTaskPrefix    = "task:"
	quickOpenMCPPrefix     = "mcp:"
)

func (m *Model) openQuickOpenDialog() {
	if bridge, ok := m.bridge.(taskBridge); ok {
		m.taskPanel = bridge.TaskPanelSnapshot()
	}
	items := quickOpenItems(m)
	m.dialog.open(dialogSpec{
		Kind:         dialogKindQuickOpen,
		Title:        "Quick Open",
		Subtitle:     "Search commands, sessions, tasks, and MCP servers",
		QueryEnabled: true,
		Items:        items,
		EmptyText:    "No quick open items",
		FooterHint:   "Type to filter | Enter open | Esc close",
		VisibleCount: len(items),
	})
	m.clearSuggestions()
}

func quickOpenItems(m *Model) []dialogItem {
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

func (m *Model) acceptQuickOpenItem(item dialogItem) {
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
	}
}
