package agents

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Definition struct {
	AgentType       string
	Description     string
	Tools           []string
	DisallowedTools []string
	SystemPrompt    string
	Model           string
	Effort          string
	PermissionMode  string
	MaxTurns        int
	Background      bool
	Isolation       string
	InitialPrompt   string
	MemoryScope     string
	Source          string
}

type DiscoveryOptions struct {
	CWD            string
	ConfigHome     string
	ManagedRoot    string
	AdditionalDirs []string
	IncludeManaged bool
	IncludeUser    bool
	IncludeProject bool
	IncludeExplicit bool
	BareMode       bool
}

type LoadResult struct {
	Active []Definition
	All    []Definition
}

type sourceDir struct {
	path   string
	source string
}

func LoadClaudeAgentDefinitions(opts DiscoveryOptions) LoadResult {
	dirs := claudeAgentSourceDirs(opts)
	all := make([]Definition, 0)
	activeByName := make(map[string]Definition)
	for _, dir := range dirs {
		definitions := loadAgentsFromDirectory(dir)
		all = append(all, definitions...)
		for _, definition := range definitions {
			activeByName[definition.AgentType] = definition
		}
	}
	active := make([]Definition, 0, len(activeByName))
	for _, definition := range activeByName {
		active = append(active, definition)
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].AgentType < active[j].AgentType
	})
	return LoadResult{
		Active: active,
		All:    all,
	}
}

func ParseAgentFile(path string, data []byte) Definition {
	return parseAgentFile(path, string(data))
}

func claudeAgentSourceDirs(opts DiscoveryOptions) []sourceDir {
	if opts.BareMode {
		if !opts.IncludeExplicit {
			return nil
		}
		out := make([]sourceDir, 0, len(opts.AdditionalDirs))
		for _, dir := range opts.AdditionalDirs {
			out = append(out, sourceDir{path: filepath.Join(dir, ".claude", "agents"), source: "project"})
		}
		return out
	}
	out := make([]sourceDir, 0)
	if opts.IncludeManaged && strings.TrimSpace(opts.ManagedRoot) != "" {
		out = append(out, sourceDir{path: filepath.Join(opts.ManagedRoot, ".claude", "agents"), source: "managed"})
	}
	if opts.IncludeUser && strings.TrimSpace(opts.ConfigHome) != "" {
		out = append(out, sourceDir{path: filepath.Join(opts.ConfigHome, "agents"), source: "user"})
	}
	if opts.IncludeProject {
		for _, dir := range projectAgentDirsUpToRoot(opts.CWD) {
			out = append(out, sourceDir{path: dir, source: "project"})
		}
	}
	if opts.IncludeExplicit {
		for _, dir := range opts.AdditionalDirs {
			out = append(out, sourceDir{path: filepath.Join(dir, ".claude", "agents"), source: "project"})
		}
	}
	return out
}

func projectAgentDirsUpToRoot(cwd string) []string {
	start := normalizePath(cwd, "")
	if start == "" {
		return nil
	}
	dirs := make([]string, 0)
	for current := start; ; {
		dirs = append(dirs, filepath.Join(current, ".claude", "agents"))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dirs
}

func loadAgentsFromDirectory(dir sourceDir) []Definition {
	path := normalizePath(dir.path, "")
	if path == "" {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	out := make([]Definition, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		fullPath := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		definition := ParseAgentFile(fullPath, data)
		if definition.AgentType == "" || definition.Description == "" {
			continue
		}
		definition.Source = dir.source
		out = append(out, definition)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AgentType < out[j].AgentType
	})
	return out
}

func parseAgentFile(path, data string) Definition {
	frontmatter, body, ok := splitFrontmatter(data)
	if !ok {
		return Definition{}
	}
	fields := parseFrontmatterFields(frontmatter)
	model := strings.TrimSpace(stringField(fields, "model"))
	if strings.EqualFold(model, "inherit") {
		model = ""
	}
	return Definition{
		AgentType:       strings.TrimSpace(stringField(fields, "name")),
		Description:     strings.TrimSpace(stringField(fields, "description")),
		Tools:           parseListField(fields["tools"]),
		DisallowedTools: parseListField(fields["disallowedtools"]),
		SystemPrompt:    strings.TrimSpace(body),
		Model:           model,
		Effort:          strings.TrimSpace(stringField(fields, "effort")),
		PermissionMode:  strings.TrimSpace(stringField(fields, "permissionmode")),
		MaxTurns:        parseIntField(fields["maxturns"]),
		Background:      parseBoolField(fields["background"]),
		Isolation:       strings.TrimSpace(stringField(fields, "isolation")),
		InitialPrompt:   strings.TrimSpace(stringField(fields, "initialprompt")),
		MemoryScope:     strings.TrimSpace(stringField(fields, "memory")),
		Source:          path,
	}
}

func splitFrontmatter(data string) (string, string, bool) {
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
	body := strings.TrimPrefix(rest[closing+len("\n---"):], "\n")
	return frontmatter, body, true
}

func parseFrontmatterFields(frontmatter string) map[string]any {
	lines := strings.Split(frontmatter, "\n")
	fields := make(map[string]any)
	for i := 0; i < len(lines); {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
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
			fields[key] = value
			i++
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

func parseListField(value any) []string {
	switch typed := value.(type) {
	case string:
		return splitList(typed)
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, splitList(item)...)
		}
		return out
	default:
		return nil
	}
}

func splitList(value string) []string {
	value = strings.ReplaceAll(value, ",", " ")
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func parseIntField(value any) int {
	text := strings.TrimSpace(stringField(map[string]any{"value": value}, "value"))
	if text == "" {
		return 0
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return n
}

func parseBoolField(value any) bool {
	text := strings.TrimSpace(stringField(map[string]any{"value": value}, "value"))
	if text == "" {
		return false
	}
	parsed, err := strconv.ParseBool(text)
	return err == nil && parsed
}

func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	switch typed := fields[key].(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func normalizePath(path, base string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}
	if base != "" {
		return filepath.Clean(filepath.Join(base, trimmed))
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return filepath.Clean(trimmed)
	}
	return filepath.Clean(abs)
}
