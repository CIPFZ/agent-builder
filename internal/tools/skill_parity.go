package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"myclaw/internal/model"
)

type SkillCommand = skillCommand

type InvokedSkillInfo struct {
	SkillName string
	SkillPath string
	Content   string
	Hooks     any
	InvokedAt time.Time
	AgentID   string
}

type SkillForkRequest struct {
	Command     SkillCommand
	Args        string
	ToolContext ToolUseContext
}

type SkillForkExecutor = func(context.Context, SkillForkRequest) (ToolResult, error)

type invokedSkillsAttachmentPayload struct {
	Type   string                        `json:"type"`
	Skills []invokedSkillAttachmentSkill `json:"skills"`
}

type skillListingAttachmentPayload struct {
	Type       string `json:"type"`
	Content    string `json:"content"`
	SkillCount int    `json:"skillCount"`
	IsInitial  bool   `json:"isInitial"`
}

type dynamicSkillAttachmentPayload struct {
	Type        string   `json:"type"`
	SkillDir    string   `json:"skillDir"`
	SkillNames  []string `json:"skillNames"`
	DisplayPath string   `json:"displayPath"`
}

type invokedSkillAttachmentSkill struct {
	SkillName string `json:"skillName"`
	SkillPath string `json:"skillPath"`
	Content   string `json:"content"`
	AgentID   string `json:"agentId"`
	InvokedAt string `json:"invokedAt"`
}

func AddInvokedSkill(appState map[string]any, info InvokedSkillInfo) {
	if appState == nil {
		return
	}
	invoked, _ := appState["invokedSkills"].([]any)
	invoked = append(invoked, map[string]any{
		"skillName": info.SkillName,
		"skillPath": info.SkillPath,
		"content":   info.Content,
		"hooks":     info.Hooks,
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
		invokedAt, _ := time.Parse(time.RFC3339Nano, stringField(item, "invokedAt"))
		items = append(items, InvokedSkillInfo{
			SkillName: stringField(item, "skillName"),
			SkillPath: stringField(item, "skillPath"),
			Content:   stringField(item, "content"),
			Hooks:     item["hooks"],
			InvokedAt: invokedAt,
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

func BuildInvokedSkillsAttachmentMessage(messageID, sessionID string, skills []InvokedSkillInfo) model.Message {
	payload := invokedSkillsAttachmentPayload{
		Type:   "invoked_skills",
		Skills: make([]invokedSkillAttachmentSkill, 0, len(skills)),
	}
	for _, skill := range skills {
		payload.Skills = append(payload.Skills, invokedSkillAttachmentSkill{
			SkillName: skill.SkillName,
			SkillPath: skill.SkillPath,
			Content:   skill.Content,
			AgentID:   skill.AgentID,
			InvokedAt: skill.InvokedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	encoded, _ := json.Marshal(payload)
	return model.Message{
		ID:                        messageID,
		SessionID:                 sessionID,
		Role:                      "attachment",
		Subtype:                   "invoked_skills",
		Content:                   string(encoded),
		IsMeta:                    true,
		IsVisibleInTranscriptOnly: true,
	}
}

func BuildSkillListingAttachmentMessage(messageID, sessionID string, skills []SkillCommand, contextWindowTokens int, isInitial bool) model.Message {
	payload := skillListingAttachmentPayload{
		Type:       "skill_listing",
		Content:    FormatSkillListingWithinBudget(skills, contextWindowTokens),
		SkillCount: len(skills),
		IsInitial:  isInitial,
	}
	encoded, _ := json.Marshal(payload)
	return model.Message{
		ID:                        messageID,
		SessionID:                 sessionID,
		Role:                      "attachment",
		Subtype:                   "skill_listing",
		Content:                   string(encoded),
		IsMeta:                    true,
		IsVisibleInTranscriptOnly: true,
	}
}

func BuildDynamicSkillAttachmentMessage(messageID, sessionID string, dir DynamicSkillDirectory, cwd string) model.Message {
	displayPath := dir.SkillDir
	if cwd != "" {
		if rel, err := filepath.Rel(cwd, dir.SkillDir); err == nil && rel != "" {
			displayPath = rel
		}
	}
	payload := dynamicSkillAttachmentPayload{
		Type:        "dynamic_skill",
		SkillDir:    dir.SkillDir,
		SkillNames:  append([]string(nil), dir.SkillNames...),
		DisplayPath: displayPath,
	}
	encoded, _ := json.Marshal(payload)
	return model.Message{
		ID:                        messageID,
		SessionID:                 sessionID,
		Role:                      "attachment",
		Subtype:                   "dynamic_skill",
		Content:                   string(encoded),
		IsMeta:                    true,
		IsVisibleInTranscriptOnly: true,
	}
}

func FormatSkillListingWithinBudget(skills []SkillCommand, contextWindowTokens int) string {
	if len(skills) == 0 {
		return ""
	}
	entries := make([]string, 0, len(skills))
	for _, skill := range skills {
		entries = append(entries, formatSkillListingEntry(skill, maxSkillListingDescChars))
	}
	full := strings.Join(entries, "\n")
	budget := skillListingCharBudget(contextWindowTokens)
	if len(full) <= budget {
		return full
	}

	nameOverhead := 0
	for _, skill := range skills {
		nameOverhead += len(skill.Name) + len("- :")
	}
	if len(skills) > 1 {
		nameOverhead += len(skills) - 1
	}
	maxDesc := (budget - nameOverhead) / len(skills)
	if maxDesc < minSkillListingDescChars {
		names := make([]string, 0, len(skills))
		for _, skill := range skills {
			names = append(names, "- "+skill.Name)
		}
		return strings.Join(names, "\n")
	}
	entries = entries[:0]
	for _, skill := range skills {
		entries = append(entries, formatSkillListingEntry(skill, maxDesc))
	}
	return strings.Join(entries, "\n")
}

const (
	defaultSkillListingCharBudget = 8000
	maxSkillListingDescChars      = 250
	minSkillListingDescChars      = 20
)

func skillListingCharBudget(contextWindowTokens int) int {
	if contextWindowTokens <= 0 {
		return defaultSkillListingCharBudget
	}
	budget := contextWindowTokens * 4 / 100
	if budget <= 0 {
		return 1
	}
	return budget
}

func formatSkillListingEntry(skill SkillCommand, maxDescriptionChars int) string {
	description := strings.TrimSpace(skill.Description)
	if when := strings.TrimSpace(skill.WhenToUse); when != "" {
		if description != "" {
			description += " - " + when
		} else {
			description = when
		}
	}
	description = truncateSkillListingDescription(description, maxDescriptionChars)
	if description == "" {
		return "- " + skill.Name
	}
	return "- " + skill.Name + ": " + description
}

func truncateSkillListingDescription(description string, maxChars int) string {
	if maxChars <= 0 || len(description) <= maxChars {
		return description
	}
	if maxChars <= 3 {
		return description[:maxChars]
	}
	return description[:maxChars-3] + "..."
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
