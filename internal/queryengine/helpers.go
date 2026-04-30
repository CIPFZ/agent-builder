package queryengine

import (
	"encoding/json"
	"myclaw/internal/model"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
	"sort"
	"strings"
)

func defaultFileReadingLimits(limits tools.ResourceLimits) tools.ResourceLimits {
	if limits.MaxTokens == 0 {
		limits.MaxTokens = 120000
	}
	if limits.MaxSizeBytes == 0 {
		limits.MaxSizeBytes = 10 * 1024 * 1024
	}
	return limits
}

func defaultGlobLimits(limits tools.ResourceLimits) tools.ResourceLimits {
	if limits.MaxResults == 0 {
		limits.MaxResults = 10000
	}
	return limits
}

func cloneAgentDefinitions(definitions map[string]tools.AgentDefinition) map[string]tools.AgentDefinition {
	if definitions == nil {
		return nil
	}
	out := make(map[string]tools.AgentDefinition, len(definitions))
	for name, definition := range definitions {
		out[name] = definition
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func compactAndSortStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cloneToolDecisions(decisions map[string]tools.ToolDecision) map[string]tools.ToolDecision {
	if decisions == nil {
		return nil
	}
	out := make(map[string]tools.ToolDecision, len(decisions))
	for name, decision := range decisions {
		out[name] = decision
	}
	return out
}

func parseToolCallBlock(content string) (string, string, bool) {
	raw := strings.TrimSpace(content)
	start := strings.Index(raw, "<tool_call>")
	end := strings.Index(raw, "</tool_call>")
	if start < 0 || end < 0 || end < start {
		return "", "", false
	}
	body := strings.TrimSpace(raw[start+len("<tool_call>") : end])
	var name string
	var input string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "input:"):
			input = strings.TrimSpace(strings.TrimPrefix(line, "input:"))
		}
	}
	if name == "" || input == "" {
		return "", "", false
	}
	return name, input, true
}

func normalizedToolInput(input string, inputObject map[string]any) string {
	if input != "" || inputObject == nil {
		return input
	}
	encoded, err := json.Marshal(inputObject)
	if err != nil {
		return input
	}
	return string(encoded)
}

func normalizedToolInputObject(input string, inputObject map[string]any) map[string]any {
	if inputObject != nil {
		return cloneAnyMap(inputObject)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return nil
	}
	if parsed == nil {
		return nil
	}
	return parsed
}

func (q *QueryEngine) observableToolInput(name, input string, inputObject map[string]any) (string, map[string]any) {
	normalizedObject := normalizedToolInputObject(input, inputObject)
	if normalizedObject == nil {
		return input, nil
	}
	observableObject := q.tools.BackfillObservableInput(name, normalizedObject)
	encoded, err := json.Marshal(observableObject)
	if err != nil {
		return input, observableObject
	}
	return string(encoded), observableObject
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMaps(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	cloned := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned = append(cloned, cloneAnyMap(item))
	}
	return cloned
}

func stringSetFromAny(value any) map[string]struct{} {
	out := make(map[string]struct{})
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out[item] = struct{}{}
			}
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if text = strings.TrimSpace(text); text != "" {
					out[text] = struct{}{}
				}
			}
		}
	case map[string]bool:
		for item, enabled := range typed {
			if enabled {
				if item = strings.TrimSpace(item); item != "" {
					out[item] = struct{}{}
				}
			}
		}
	case map[string]struct{}:
		for item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out[item] = struct{}{}
			}
		}
	}
	return out
}

func stringSetKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func stringListFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if text = strings.TrimSpace(text); text != "" {
					out = append(out, text)
				}
			}
		}
		return out
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return nil
		}
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapField(value map[string]any, key string) string {
	if value == nil {
		return ""
	}
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}

func messageBlocksFromContentMaps(input []map[string]any) []model.MessageBlock {
	if len(input) == 0 {
		return nil
	}
	blocks := make([]model.MessageBlock, 0, len(input))
	for _, item := range input {
		if item == nil {
			continue
		}
		blockType, _ := item["type"].(string)
		switch model.MessageBlockType(blockType) {
		case model.MessageBlockText:
			text, _ := item["text"].(string)
			blocks = append(blocks, model.MessageBlock{
				Type: model.MessageBlockText,
				Text: text,
			})
		default:
			blocks = append(blocks, model.MessageBlock{
				Type: model.MessageBlockType(blockType),
				Raw:  cloneAnyMap(item),
			})
		}
	}
	return blocks
}

func isMCPToolDefinition(def tools.Definition) bool {
	return strings.EqualFold(strings.TrimSpace(def.Source), "mcp") || strings.HasPrefix(strings.TrimSpace(def.Name), "mcp__")
}

func estimateMessagesTokens(messages []session.Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTextTokens(msg.Content)
	}
	return total
}

func estimateTextTokens(content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}
	return (len(content) + 3) / 4
}

func resolveWorkDir(sess session.Session, loader *workspace.Loader) string {
	if root := strings.TrimSpace(sess.Metadata.AgentWorktreePath); root != "" {
		return root
	}
	if root := strings.TrimSpace(sess.Metadata.AgentCWD); root != "" {
		return root
	}
	if loader == nil {
		return sess.Key
	}
	ctx, err := loader.Load()
	if err != nil || ctx.Root == "" {
		return sess.Key
	}
	return ctx.Root
}

func fallbackRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "user", "assistant", "tool", "system", "summary":
		return role
	default:
		return "assistant"
	}
}

func cloneSessionMessages(messages []session.Message) []session.Message {
	out := make([]session.Message, len(messages))
	copy(out, messages)
	return out
}
