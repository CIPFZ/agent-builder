package tui

import (
	"fmt"
	"sort"
	"strings"
)

const (
	dialogKindTasks      = "tasks"
	dialogKindTaskDetail = "task-detail"
)

func (m *Model) openTasksDialog() {
	bridge, ok := m.bridge.(taskBridge)
	if !ok {
		m.dialog.open(dialogSpec{
			Kind:       dialogKindTasks,
			Title:      "Tasks",
			Subtitle:   "Task workbench is not available for this bridge",
			EmptyText:  "No delegated tasks",
			FooterHint: "Esc close",
		})
		return
	}

	m.taskPanel = bridge.TaskPanelSnapshot()
	m.dialog.open(dialogSpec{
		Kind:         dialogKindTasks,
		Title:        "Tasks",
		Subtitle:     taskPanelSubtitle(m.taskPanel),
		QueryEnabled: true,
		Items:        taskDialogItems(m.taskPanel),
		EmptyText:    "No delegated tasks",
		FooterHint:   "Type to filter | Enter details | Esc close",
		VisibleCount: 7,
	})
	m.clearSuggestions()
}

func taskPanelSubtitle(snapshot taskPanelSnapshot) string {
	parts := []string{
		fmt.Sprintf("running %d", snapshot.RunningCount),
		fmt.Sprintf("completed %d", snapshot.CompletedCount),
	}
	if snapshot.FailedCount > 0 {
		parts = append(parts, fmt.Sprintf("failed %d", snapshot.FailedCount))
	}
	if snapshot.StoppedCount > 0 {
		parts = append(parts, fmt.Sprintf("stopped %d", snapshot.StoppedCount))
	}
	if snapshot.SessionID != "" {
		parts = append(parts, snapshot.SessionID)
	}
	return strings.Join(parts, " | ")
}

func taskDialogItems(snapshot taskPanelSnapshot) []dialogItem {
	tasks := append([]taskSnapshot(nil), snapshot.Tasks...)
	sort.SliceStable(tasks, func(i, j int) bool {
		if taskStatusRank(tasks[i].Status) != taskStatusRank(tasks[j].Status) {
			return taskStatusRank(tasks[i].Status) < taskStatusRank(tasks[j].Status)
		}
		return tasks[i].RunID < tasks[j].RunID
	})

	items := make([]dialogItem, 0, len(tasks))
	for _, task := range tasks {
		label := strings.TrimSpace(task.Label)
		if label == "" {
			label = task.RunID
		}
		descriptionParts := []string{strings.TrimSpace(task.Status)}
		if task.RecommendedAction != "" {
			descriptionParts = append(descriptionParts, strings.TrimSpace(task.RecommendedAction))
		}
		if task.DecisionPriority != "" {
			descriptionParts = append(descriptionParts, strings.TrimSpace(task.DecisionPriority))
		}
		if task.LastAssistant != "" {
			descriptionParts = append(descriptionParts, truncateResumeText(task.LastAssistant, 60))
		} else if task.Prompt != "" {
			descriptionParts = append(descriptionParts, truncateResumeText(task.Prompt, 60))
		}
		items = append(items, dialogItem{
			Label:       label,
			Description: strings.Join(descriptionParts, " | "),
			Value:       task.RunID,
		})
	}
	return items
}

func taskStatusRank(status string) int {
	switch strings.TrimSpace(status) {
	case "running":
		return 0
	case "failed":
		return 1
	case "stopped":
		return 2
	case "completed":
		return 3
	default:
		return 4
	}
}

func (m *Model) acceptTaskItem(item dialogItem) {
	task, ok := m.taskPanel.task(item.Value)
	if !ok {
		return
	}
	m.openTaskDetailDialog(task)
}

func (m *Model) openTaskDetailDialog(task taskSnapshot) {
	items := []dialogItem{
		{Label: "Run ID", Description: valueOrUnset(task.RunID), Disabled: true},
		{Label: "Status", Description: valueOrUnset(task.Status), Disabled: true},
		{Label: "Label", Description: valueOrUnset(task.Label), Disabled: true},
		{Label: "Prompt", Description: valueOrUnset(task.Prompt), Disabled: true},
		{Label: "Child session", Description: valueOrUnset(task.ChildSessionID), Disabled: true},
	}
	for _, row := range []struct {
		label string
		value string
	}{
		{"Last event", task.LastEvent},
		{"Next action", task.NextAction},
		{"Recommended role", task.RecommendedRole},
		{"Recommended action", task.RecommendedAction},
		{"Priority", task.DecisionPriority},
		{"Reason", task.DecisionReason},
		{"Last assistant", task.LastAssistant},
		{"Output", task.Output},
		{"Error", task.Error},
		{"Messages", fmt.Sprintf("%d", task.MessageCount)},
		{"Control messages", fmt.Sprintf("%d", task.ControlMessageCount)},
	} {
		if strings.TrimSpace(row.value) == "" || row.value == "0" {
			continue
		}
		items = append(items, dialogItem{Label: row.label, Description: row.value, Disabled: true})
	}
	m.dialog.open(dialogSpec{
		Kind:         dialogKindTaskDetail,
		Title:        "Task details",
		Subtitle:     "Delegated task state and recommendations",
		Items:        items,
		FooterHint:   "Esc close",
		VisibleCount: len(items),
	})
}

func (snapshot taskPanelSnapshot) task(runID string) (taskSnapshot, bool) {
	for _, task := range snapshot.Tasks {
		if task.RunID == runID {
			return task, true
		}
	}
	return taskSnapshot{}, false
}
