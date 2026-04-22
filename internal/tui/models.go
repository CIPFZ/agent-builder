package tui

import (
	"fmt"
	"strings"
)

const dialogKindModel = "model"

type modelOption struct {
	Value       string
	Label       string
	Description string
}

var tuiModelOptions = []modelOption{
	{Value: "default", Label: "Default", Description: "Use the configured default session model"},
	{Value: "sonnet", Label: "Sonnet", Description: "Best for everyday coding tasks"},
	{Value: "sonnet[1m]", Label: "Sonnet (1M context)", Description: "Sonnet for long sessions and large codebases"},
	{Value: "opus", Label: "Opus", Description: "Most capable for complex work"},
	{Value: "opus[1m]", Label: "Opus (1M context)", Description: "Opus for long sessions and large codebases"},
	{Value: "haiku", Label: "Haiku", Description: "Fastest for quick answers"},
}

func findModelOption(value string, snapshot platformStatusSnapshot) (modelOption, bool) {
	normalized := normalizeModelOption(value)
	for _, option := range modelOptionsForSnapshot(snapshot) {
		if normalizeModelOption(option.Value) == normalized {
			return option, true
		}
	}
	return modelOption{}, false
}

func normalizeModelOption(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (m *Model) applyModelSelection(value string) {
	value = normalizeModelOption(value)
	if value == "" {
		return
	}
	snapshot := platformStatusSnapshot{}
	if provider, ok := m.bridge.(platformStatusBridge); ok {
		snapshot = provider.PlatformStatusSnapshot()
	}
	if value == "default" {
		if err := m.bridge.ClearSessionModel(); err != nil {
			m.applyBridgeError(err)
			return
		}
		m.busy = false
		m.activity.Label = "Model switched to default"
		m.transcript = append(m.transcript, transcriptEntry{
			Kind:    messageKindSystem,
			Role:    "system",
			Content: "Model switched to default",
		})
		m.noteTranscriptAppended()
		return
	}
	if err := m.bridge.SetSessionModel(value); err != nil {
		m.applyBridgeError(err)
		return
	}
	selected, _ := findModelOption(value, snapshot)
	label := value
	if strings.TrimSpace(selected.Label) != "" {
		label = selected.Label
	}
	transcriptLabel := value
	if strings.TrimSpace(selected.Value) != "" {
		transcriptLabel = selected.Value
	}
	m.busy = false
	m.activity.Label = "Model switched to " + label
	m.transcript = append(m.transcript, transcriptEntry{
		Kind:    messageKindSystem,
		Role:    "system",
		Content: "Model switched to " + transcriptLabel,
	})
	m.noteTranscriptAppended()
}

func modelDialogItems(snapshot platformStatusSnapshot) []dialogItem {
	options := modelOptionsForSnapshot(snapshot)
	items := make([]dialogItem, 0, len(options))
	current := normalizeModelOption(snapshot.ModelOverride)
	if current == "" {
		current = normalizeModelOption(snapshot.ResolvedModel)
	}
	for _, option := range options {
		description := option.Description
		if normalizeModelOption(option.Value) == current {
			description += " (current)"
		}
		items = append(items, dialogItem{
			Label:       option.Label,
			Value:       option.Value,
			Description: description,
		})
	}
	return items
}

func modelOptionsForSnapshot(snapshot platformStatusSnapshot) []modelOption {
	if len(snapshot.AvailableModels) == 0 {
		return append([]modelOption(nil), tuiModelOptions...)
	}
	options := make([]modelOption, 0, len(snapshot.AvailableModels)+1)
	options = append(options, modelOption{
		Value:       "default",
		Label:       "Default",
		Description: "Use the configured default session model",
	})
	for _, available := range snapshot.AvailableModels {
		value := strings.TrimSpace(available.Value)
		if value == "" {
			continue
		}
		label := strings.TrimSpace(available.Label)
		if label == "" {
			label = value
		}
		description := strings.TrimSpace(available.Description)
		if description == "" {
			description = value
		}
		if available.ContextWindowTokens > 0 {
			description = fmt.Sprintf("%s | ctx %d", description, available.ContextWindowTokens)
		}
		options = append(options, modelOption{
			Value:       value,
			Label:       label,
			Description: description,
		})
	}
	return options
}
