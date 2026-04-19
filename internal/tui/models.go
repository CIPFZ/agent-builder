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

func findModelOption(value string) (modelOption, bool) {
	normalized := normalizeModelOption(value)
	for _, option := range tuiModelOptions {
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
	if _, ok := findModelOption(value); !ok {
		m.applyBridgeError(fmt.Errorf("unknown model option %q", value))
		return
	}
	if err := m.bridge.SetSessionModel(value); err != nil {
		m.applyBridgeError(err)
		return
	}
	m.busy = false
	m.activity.Label = "Model switched to " + value
	m.transcript = append(m.transcript, transcriptEntry{
		Kind:    messageKindSystem,
		Role:    "system",
		Content: "Model switched to " + value,
	})
	m.noteTranscriptAppended()
}

func modelDialogItems(snapshot platformStatusSnapshot) []dialogItem {
	items := make([]dialogItem, 0, len(tuiModelOptions))
	current := normalizeModelOption(snapshot.ModelOverride)
	if current == "" {
		current = "default"
	}
	for _, option := range tuiModelOptions {
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
