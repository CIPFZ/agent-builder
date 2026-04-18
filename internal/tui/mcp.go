package tui

import "fmt"

const (
	dialogKindMCPList   = "mcp-list"
	dialogKindMCPDetail = "mcp-detail"
)

func (m *Model) openMCPDialog() {
	bridge, ok := m.bridge.(mcpStatusBridge)
	if !ok {
		m.dialog.open(dialogSpec{
			Kind:       dialogKindMCPList,
			Title:      "MCP",
			Subtitle:   "MCP status is not available for this bridge",
			EmptyText:  "No MCP servers",
			FooterHint: "Esc close",
		})
		return
	}
	snapshot := bridge.MCPSnapshot()
	items := make([]dialogItem, 0, len(snapshot.Servers))
	for _, server := range snapshot.Servers {
		items = append(items, dialogItem{
			Label:       server.Name,
			Value:       server.Name,
			Description: mcpServerSummary(server),
		})
	}
	m.dialog.open(dialogSpec{
		Kind:         dialogKindMCPList,
		Title:        "MCP",
		Subtitle:     "Connected MCP servers, tools, prompts, and resources",
		QueryEnabled: true,
		Items:        items,
		EmptyText:    "No MCP servers",
		FooterHint:   "Type to filter | Enter details | Esc close",
		VisibleCount: 7,
	})
}

func mcpServerSummary(server mcpServerSnapshot) string {
	status := "disabled"
	if server.Enabled {
		status = "enabled"
	}
	return fmt.Sprintf(
		"%s | %d tools | %d prompts | %d resources",
		status,
		len(server.Tools),
		len(server.Prompts),
		len(server.Resources),
	)
}

func (m *Model) acceptMCPItem(item dialogItem) {
	bridge, ok := m.bridge.(mcpStatusBridge)
	if !ok {
		return
	}
	snapshot := bridge.MCPSnapshot()
	for _, server := range snapshot.Servers {
		if server.Name == item.Value {
			m.openMCPServerDetailDialog(server)
			return
		}
	}
}

func (m *Model) openMCPServerDetailDialog(server mcpServerSnapshot) {
	items := []dialogItem{
		{Label: "Server", Description: valueOrUnset(server.Name), Disabled: true},
		{Label: "Transport", Description: valueOrUnset(server.TransportType), Disabled: true},
		{Label: "Endpoint", Description: valueOrUnset(server.Endpoint), Disabled: true},
	}
	status := "disabled"
	if server.Enabled {
		status = "enabled"
	}
	items = append(items, dialogItem{Label: "Status", Description: status, Disabled: true})
	items = appendMCPSection(items, "Tool", server.Tools)
	items = appendMCPSection(items, "Prompt", server.Prompts)
	items = appendMCPSection(items, "Resource", server.Resources)
	m.dialog.open(dialogSpec{
		Kind:         dialogKindMCPDetail,
		Title:        "MCP server",
		Subtitle:     "Detailed MCP server inventory",
		Items:        items,
		VisibleCount: len(items),
		FooterHint:   "Esc close",
	})
}

func appendMCPSection(items []dialogItem, label string, values []string) []dialogItem {
	if len(values) == 0 {
		return append(items, dialogItem{Label: label, Description: "none", Disabled: true})
	}
	for _, value := range values {
		items = append(items, dialogItem{Label: label, Description: value, Disabled: true})
	}
	return items
}
