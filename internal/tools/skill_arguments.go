package tools

import (
	"sort"
	"strings"
)

// SubstituteSkillArguments is the exported wrapper used by tests and by the
// skill loader. It keeps Claude-style raw ARGUMENTS appending semantics while
// parsing quoted arguments shell-style.
func SubstituteSkillArguments(
	content string,
	args string,
	appendIfNoPlaceholder bool,
	argumentNames []string,
) string {
	return substituteSkillArguments(content, args, appendIfNoPlaceholder, argumentNames)
}

func substituteSkillArguments(
	content string,
	args string,
	appendIfNoPlaceholder bool,
	argumentNames []string,
) string {
	if strings.TrimSpace(args) == "" {
		return content
	}
	parsedArgs := parseSkillArguments(args)
	original := content
	content = replaceSkillArgumentPlaceholders(content, args, parsedArgs, argumentNames)
	if content == original && appendIfNoPlaceholder {
		content += "\n\nARGUMENTS: " + args
	}
	return content
}

func parseSkillArguments(args string) []string {
	if strings.TrimSpace(args) == "" {
		return nil
	}
	tokens := make([]string, 0, 4)
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for i := 0; i < len(args); i++ {
		ch := args[i]
		switch {
		case escaped:
			current.WriteByte(ch)
			escaped = false
		case ch == '\\' && !inSingle:
			escaped = true
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case isSkillArgumentSeparator(ch) && !inSingle && !inDouble:
			flush()
		default:
			current.WriteByte(ch)
		}
	}
	if escaped {
		current.WriteByte('\\')
	}
	flush()
	return tokens
}

func replaceSkillArgumentPlaceholders(
	content string,
	rawArgs string,
	parsedArgs []string,
	argumentNames []string,
) string {
	if content == "" {
		return content
	}
	names := append([]string(nil), argumentNames...)
	sort.SliceStable(names, func(i, j int) bool {
		if len(names[i]) == len(names[j]) {
			return names[i] < names[j]
		}
		return len(names[i]) > len(names[j])
	})

	var out strings.Builder
	for i := 0; i < len(content); {
		if content[i] != '$' {
			out.WriteByte(content[i])
			i++
			continue
		}

		if replacement, consumed, ok := matchSkillArgumentToken(content[i:], rawArgs, parsedArgs); ok {
			out.WriteString(replacement)
			i += consumed
			continue
		}

		if replacement, consumed, ok := matchSkillNamedArgumentToken(content[i:], parsedArgs, names); ok {
			out.WriteString(replacement)
			i += consumed
			continue
		}

		out.WriteByte(content[i])
		i++
	}
	return out.String()
}

func matchSkillArgumentToken(content, rawArgs string, parsedArgs []string) (string, int, bool) {
	if strings.HasPrefix(content, "$ARGUMENTS[") {
		end := len("$ARGUMENTS[")
		indexStart := end
		for end < len(content) && content[end] >= '0' && content[end] <= '9' {
			end++
		}
		if end > indexStart && end < len(content) && content[end] == ']' {
			index := parsePositiveIndex(content[indexStart:end])
			return skillArgumentValue(parsedArgs, index), end + 1, true
		}
	}
	if strings.HasPrefix(content, "$ARGUMENTS") {
		return rawArgs, len("$ARGUMENTS"), true
	}
	if len(content) > 1 && content[1] >= '0' && content[1] <= '9' {
		end := 1
		for end < len(content) && content[end] >= '0' && content[end] <= '9' {
			end++
		}
		if end == len(content) || !isSkillArgumentContinuation(content[end]) {
			return skillArgumentValue(parsedArgs, parsePositiveIndex(content[1:end])), end, true
		}
	}
	return "", 0, false
}

func matchSkillNamedArgumentToken(content string, parsedArgs []string, names []string) (string, int, bool) {
	for i, name := range names {
		if name == "" || len(content) < len(name)+1 {
			continue
		}
		if !strings.HasPrefix(content[1:], name) {
			continue
		}
		next := 1 + len(name)
		if next < len(content) && isSkillArgumentContinuation(content[next]) {
			continue
		}
		return skillArgumentValue(parsedArgs, i), next, true
	}
	return "", 0, false
}

func skillArgumentValue(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func parsePositiveIndex(value string) int {
	index := 0
	for i := 0; i < len(value); i++ {
		index = index*10 + int(value[i]-'0')
	}
	return index
}

func isSkillArgumentSeparator(ch byte) bool {
	switch ch {
	case '\t', '\n', '\r', '\v', '\f', ' ':
		return true
	default:
		return false
	}
}

func isSkillArgumentContinuation(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_' ||
		ch == '['
}
