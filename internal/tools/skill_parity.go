package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type SkillCommand = skillCommand

type InvokedSkillInfo struct {
	SkillName string
	SkillPath string
	Content   string
	InvokedAt time.Time
	AgentID   string
}

type SkillForkRequest struct {
	Command     SkillCommand
	Args        string
	ToolContext ToolUseContext
}

type SkillForkExecutor = func(context.Context, SkillForkRequest) (ToolResult, error)

func AddInvokedSkill(appState map[string]any, info InvokedSkillInfo) {
	if appState == nil {
		return
	}
	invoked, _ := appState["invokedSkills"].([]any)
	invoked = append(invoked, map[string]any{
		"skillName": info.SkillName,
		"skillPath": info.SkillPath,
		"content":   info.Content,
		"invokedAt": info.InvokedAt.UTC().Format(time.RFC3339Nano),
		"agentId":   info.AgentID,
	})
	appState["invokedSkills"] = invoked
}

func GetInvokedSkillsForAgent(appState map[string]any, agentID string) []InvokedSkillInfo {
	if appState == nil {
		return nil
	}
	items := make([]InvokedSkillInfo, 0)
	for _, raw := range anySlice(appState["invokedSkills"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringField(item, "agentId") != agentID {
			continue
		}
		items = append(items, InvokedSkillInfo{
			SkillName: stringField(item, "skillName"),
			SkillPath: stringField(item, "skillPath"),
			Content:   stringField(item, "content"),
			AgentID:   stringField(item, "agentId"),
		})
	}
	return items
}

func ClearInvokedSkillsForAgent(appState map[string]any, agentID string) {
	if appState == nil {
		return
	}
	invoked := anySlice(appState["invokedSkills"])
	if len(invoked) == 0 {
		return
	}
	filtered := make([]any, 0, len(invoked))
	for _, raw := range invoked {
		item, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		if stringField(item, "agentId") == agentID {
			continue
		}
		filtered = append(filtered, raw)
	}
	appState["invokedSkills"] = filtered
}

func skillForkExecutorFromAppState(appState map[string]any) SkillForkExecutor {
	if appState == nil {
		return nil
	}
	executor, _ := appState["skillForkExecutor"].(SkillForkExecutor)
	return executor
}

func skillBaseDir(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

func substituteSkillArguments(content, args string, argumentNames []string) string {
	if args == "" {
		return content
	}
	parsedArgs := splitSkillArguments(args)
	original := content
	for i, name := range argumentNames {
		if name == "" {
			continue
		}
		content = strings.ReplaceAll(content, "$"+name, valueAt(parsedArgs, i))
	}
	content = replaceIndexedSkillArguments(content, parsedArgs)
	content = strings.ReplaceAll(content, "$ARGUMENTS", args)
	if content == original {
		content += "\n\nARGUMENTS: " + args
	}
	return content
}

func replaceIndexedSkillArguments(content string, parsedArgs []string) string {
	for i, arg := range parsedArgs {
		index := fmt.Sprintf("%d", i)
		content = strings.ReplaceAll(content, "$"+index, arg)
		content = strings.ReplaceAll(content, "$ARGUMENTS["+index+"]", arg)
	}
	return content
}

func splitSkillArguments(args string) []string {
	fields := strings.Fields(strings.TrimSpace(args))
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func splitSkillFrontmatterList(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	return strings.Fields(value)
}

func valueAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}
