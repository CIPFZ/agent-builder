package tools

import (
	"fmt"
	"strings"
)

type FrontmatterShell string

const (
	FrontmatterShellBash       FrontmatterShell = "bash"
	FrontmatterShellPowershell FrontmatterShell = "powershell"
)

// ParseSkillFile parses a Claude-style skill markdown file and returns the
// command model used by the loader/runtime.
func ParseSkillFile(name, path, data string) SkillCommand {
	return parseSkillFile(name, path, data)
}

func parseSkillFile(name, path, data string) skillCommand {
	command := skillCommand{Name: name, Path: path, Content: data, UserInvocable: true}
	frontmatter, body, ok := splitSkillFrontmatter(data)
	if !ok {
		command.Description = extractSkillDescription(body)
		command.Content = body
		return command
	}

	fields := parseSkillFrontmatterFields(frontmatter)
	if displayName := stringFieldAny(fields, "name"); displayName != "" {
		command.DisplayName = displayName
	}
	if description := stringFieldAny(fields, "description"); description != "" {
		command.Description = description
		command.HasUserSpecifiedDescription = true
	} else {
		command.Description = extractSkillDescription(body)
	}
	command.WhenToUse = stringFieldAny(fields, "when_to_use")
	command.Version = stringFieldAny(fields, "version")
	if _, ok := fields["user-invocable"]; ok {
		command.UserInvocable = parseBooleanFrontmatterAny(fields["user-invocable"])
	}
	command.AllowedTools = parseSkillFrontmatterList(fields["allowed-tools"], false)
	command.ArgumentHint = stringFieldAny(fields, "argument-hint")
	command.ArgumentNames = parseSkillFrontmatterList(fields["arguments"], false)
	model := strings.TrimSpace(stringFieldAny(fields, "model"))
	if strings.EqualFold(model, "inherit") {
		model = ""
	}
	command.Model = model
	command.DisableModelInvocation = parseBooleanFrontmatterAny(fields["disable-model-invocation"])
	command.Context = strings.ToLower(strings.TrimSpace(stringFieldAny(fields, "context")))
	command.Agent = strings.TrimSpace(stringFieldAny(fields, "agent"))
	command.Effort = strings.TrimSpace(stringFieldAny(fields, "effort"))
	command.Hooks = parseSkillHooksFrontmatter(fields["hooks"])
	command.Paths = parseSkillFrontmatterList(fields["paths"], true)
	command.Shell = parseSkillShellFrontmatter(fields["shell"], path)
	command.Content = body
	return command
}

func splitSkillFrontmatter(data string) (string, string, bool) {
	normalized := strings.ReplaceAll(data, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", data, false
	}
	rest := strings.TrimPrefix(normalized, "---\n")
	closing := strings.Index(rest, "\n---")
	if closing < 0 {
		return "", data, false
	}
	frontmatter := rest[:closing]
	body := rest[closing+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	return frontmatter, body, true
}

func parseSkillFrontmatterFields(frontmatter string) map[string]any {
	lines := strings.Split(frontmatter, "\n")
	fields := make(map[string]any)
	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			i++
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			i++
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value != "" {
			fields[key] = parseSkillFrontmatterScalar(value)
			i++
			continue
		}
		if key == "hooks" {
			block, next := collectSkillFrontmatterIndentedBlock(lines, i+1)
			fields[key] = block
			i = next
			continue
		}

		items := make([]string, 0)
		j := i + 1
		for j < len(lines) {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				j++
				continue
			}
			if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
				break
			}
			trimmed := strings.TrimSpace(next)
			if strings.HasPrefix(trimmed, "- ") {
				item := strings.TrimSpace(trimmed[2:])
				if item != "" {
					items = append(items, item)
				}
			}
			j++
		}
		if len(items) > 0 {
			fields[key] = items
		} else {
			fields[key] = ""
		}
		i = j
	}
	return fields
}

func collectSkillFrontmatterIndentedBlock(lines []string, start int) (string, int) {
	block := make([]string, 0)
	j := start
	for j < len(lines) {
		next := lines[j]
		if strings.TrimSpace(next) == "" {
			block = append(block, "")
			j++
			continue
		}
		if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
			break
		}
		block = append(block, strings.TrimRight(next, " \t\r"))
		j++
	}
	return strings.Join(block, "\n"), j
}

func parseSkillFrontmatterScalar(value string) any {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		if inner == "" {
			return []string{}
		}
		return splitSkillFrontmatterList(inner)
	}
	return trimmed
}

func parseSkillFrontmatterList(value any, expandPaths bool) []string {
	switch typed := value.(type) {
	case string:
		if expandPaths {
			return splitSkillPathFrontmatterList(typed)
		}
		return splitSkillFrontmatterList(typed)
	case []string:
		if expandPaths {
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				out = append(out, splitSkillPathFrontmatterList(item)...)
			}
			return out
		}
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			for _, part := range splitSkillFrontmatterList(item) {
				out = append(out, part)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if expandPaths {
					out = append(out, splitSkillPathFrontmatterList(text)...)
				} else {
					out = append(out, splitSkillFrontmatterList(text)...)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func parseSkillHooksFrontmatter(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return map[string]any{"raw": typed}
	case []string:
		if len(typed) == 0 {
			return nil
		}
		return map[string]any{"raw": strings.Join(typed, "\n")}
	case []any:
		if len(typed) == 0 {
			return nil
		}
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return nil
		}
		return map[string]any{"raw": strings.Join(parts, "\n")}
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		return typed
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text == "" {
			return nil
		}
		return map[string]any{"raw": text}
	}
}

func splitSkillFrontmatterList(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	fields := parseSkillArguments(value)
	return filterEmptyStrings(fields)
}

func splitSkillPathFrontmatterList(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	parts := make([]string, 0)
	current := strings.Builder{}
	depth := 0
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch ch {
		case '{':
			depth++
			current.WriteByte(ch)
		case '}':
			if depth > 0 {
				depth--
			}
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					parts = append(parts, trimmed)
				}
				current.Reset()
				continue
			}
			current.WriteByte(ch)
		default:
			current.WriteByte(ch)
		}
	}
	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		parts = append(parts, trimmed)
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, expandSkillBraces(part)...)
	}
	return filterEmptyStrings(out)
}

func expandSkillBraces(pattern string) []string {
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		return []string{pattern}
	}
	close := strings.IndexByte(pattern[open+1:], '}')
	if close < 0 {
		return []string{pattern}
	}
	close += open + 1
	prefix := pattern[:open]
	suffix := pattern[close+1:]
	alternatives := strings.Split(pattern[open+1:close], ",")
	out := make([]string, 0, len(alternatives))
	for _, alt := range alternatives {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		out = append(out, expandSkillBraces(prefix+alt+suffix)...)
	}
	if len(out) == 0 {
		return []string{prefix + suffix}
	}
	return out
}

func parseSkillShellFrontmatter(value any, _ string) FrontmatterShell {
	shell := strings.ToLower(strings.TrimSpace(stringFieldAny(map[string]any{"shell": value}, "shell")))
	switch shell {
	case "", string(FrontmatterShellBash):
		if shell == "" {
			return ""
		}
		return FrontmatterShellBash
	case string(FrontmatterShellPowershell):
		return FrontmatterShellPowershell
	default:
		return ""
	}
}

func stringFieldAny(object map[string]any, key string) string {
	if object == nil {
		return ""
	}
	value, _ := object[key]
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		if value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func parseBooleanFrontmatterAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func filterEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func extractSkillDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
		if len(trimmed) > 100 {
			return trimmed[:97] + "..."
		}
		return trimmed
	}
	return ""
}
