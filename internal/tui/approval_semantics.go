package tui

import (
	"fmt"
	"strings"
)

type approvalSemanticDescription struct {
	Title   string
	Details []string
}

func describeApprovalSemantics(approval approvalRenderState) approvalSemanticDescription {
	title := "Permission Required"
	switch approvalSemanticKind(approval.ToolName) {
	case "command":
		title = "Command Approval"
	case "file-edit":
		title = "File Edit Approval"
	case "read":
		title = "Read Approval"
	case "web-fetch":
		title = "Web Fetch Approval"
	case "skill":
		title = "Skill Approval"
	case "task":
		title = "Task Approval"
	}

	details := make([]string, 0, 3)
	if title != "Permission Required" {
		details = append(details, "Permission Required")
	}
	if label, value, ok := primaryApprovalInput(approval); ok {
		details = append(details, fmt.Sprintf("%s: %s", label, value))
	} else if strings.TrimSpace(approval.ToolInput) != "" {
		details = append(details, "Input: "+strings.TrimSpace(approval.ToolInput))
	}
	if strings.TrimSpace(approval.ToolName) != "" {
		details = append(details, "Tool: "+strings.TrimSpace(approval.ToolName))
	}
	return approvalSemanticDescription{Title: title, Details: details}
}

func approvalSemanticKind(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "system.run", "Bash", "PowerShell":
		return "command"
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		return "file-edit"
	case "Read", "Glob", "Grep", "LS", "mcp__filesystem__read_file":
		return "read"
	case "WebFetch":
		return "web-fetch"
	case "Skill":
		return "skill"
	case "Task", "Agent", "agent.task":
		return "task"
	default:
		return "generic"
	}
}

func primaryApprovalInput(approval approvalRenderState) (string, string, bool) {
	if label, value, ok := primaryToolInputFromName(approval.ToolName, approval.ToolInput, approval.ToolInputObject); ok {
		return approvalLabelTitle(label), value, true
	}

	switch strings.TrimSpace(approval.ToolName) {
	case "system.run":
		if strings.TrimSpace(approval.ToolInput) != "" {
			return "Command", strings.TrimSpace(approval.ToolInput), true
		}
	case "Skill":
		if value, ok := approval.ToolInputObject["skill"].(string); ok && strings.TrimSpace(value) != "" {
			return "Skill", strings.TrimSpace(value), true
		}
	case "agent.task":
		if value, ok := approval.ToolInputObject["prompt"].(string); ok && strings.TrimSpace(value) != "" {
			return "Prompt", strings.TrimSpace(value), true
		}
	}

	return "", "", false
}

func approvalLabelTitle(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	switch strings.ToLower(label) {
	case "url":
		return "URL"
	case "id":
		return "ID"
	}
	runes := []rune(label)
	first := strings.ToUpper(string(runes[0]))
	if len(runes) == 1 {
		return first
	}
	return first + string(runes[1:])
}
