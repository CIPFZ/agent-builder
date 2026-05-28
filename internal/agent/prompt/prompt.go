package prompt

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/filepathext"
	"github.com/charmbracelet/crush/internal/home"
	"github.com/charmbracelet/crush/internal/shell"
	"github.com/charmbracelet/crush/internal/skills"
)

// Prompt represents a template-based prompt generator.
type Prompt struct {
	name       string
	template   string
	now        func() time.Time
	platform   string
	workingDir string
	skills     []*skills.Skill
}

type PromptDat struct {
	Provider      string
	Model         string
	Config        config.Config
	WorkingDir    string
	IsGitRepo     bool
	Platform      string
	Date          string
	GitStatus     string
	ContextFiles  []ContextFile
	AvailSkillXML string
}

type ContextFile struct {
	Path    string
	Content string
}

type ContextSourceKind string

const (
	ContextSourceManaged       ContextSourceKind = "managed_instructions"
	ContextSourceUser          ContextSourceKind = "user_memory"
	ContextSourceProjectAgents ContextSourceKind = "project_agents"
	ContextSourceProjectClaude ContextSourceKind = "project_claude"
	ContextSourceProject       ContextSourceKind = ContextSourceProjectAgents
	ContextSourceDotClaude     ContextSourceKind = "dot_claude"
	ContextSourceClaudeRule    ContextSourceKind = "claude_rule"
	ContextSourceLocal         ContextSourceKind = "local_memory"
	ContextSourceSkill         ContextSourceKind = "skill_context"
	ContextSourceMCP           ContextSourceKind = "mcp_resource_context"
	ContextSourceFile          ContextSourceKind = "context_file"
	ContextSourceGenerated     ContextSourceKind = "generated_context"
	ContextSourceReadFile      ContextSourceKind = "read_file_state"
	ContextSourceCompact       ContextSourceKind = "compact_reinjected_context"
)

type ContextSourceState string

const (
	ContextStateUnavailable ContextSourceState = "unavailable"
	ContextStateDisabled    ContextSourceState = "disabled"
	ContextStateUnloaded    ContextSourceState = "unloaded"
	ContextStateLoading     ContextSourceState = "loading"
	ContextStateLoaded      ContextSourceState = "loaded"
	ContextStateFailed      ContextSourceState = "failed"
)

type ContextSource struct {
	ID             string
	Kind           ContextSourceKind
	Name           string
	Path           string
	URI            string
	Scope          string
	Enabled        bool
	State          ContextSourceState
	Reason         string
	Diagnostics    string
	Error          string
	ContentSummary string
	TokenEstimate  int
	LoadedAt       time.Time
	Provenance     string
	ParentID       string
	RuleGlobs      []string
	SizeBytes      int64
	MTimeUnix      int64
	ContentHash    string
	Content        string
}

type ContextLoadResult struct {
	Sources      []ContextSource
	ContextFiles []ContextFile
}

const (
	maxContextFileBytes    = 40000
	maxContextIncludeDepth = 5
)

type Option func(*Prompt)

func WithTimeFunc(fn func() time.Time) Option {
	return func(p *Prompt) {
		p.now = fn
	}
}

func WithPlatform(platform string) Option {
	return func(p *Prompt) {
		p.platform = platform
	}
}

func WithWorkingDir(workingDir string) Option {
	return func(p *Prompt) {
		p.workingDir = workingDir
	}
}

func WithSkills(activeSkills []*skills.Skill) Option {
	return func(p *Prompt) {
		p.skills = activeSkills
	}
}

func NewPrompt(name, promptTemplate string, opts ...Option) (*Prompt, error) {
	p := &Prompt{
		name:     name,
		template: promptTemplate,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

func (p *Prompt) Build(ctx context.Context, provider, model string, store *config.ConfigStore) (string, error) {
	t, err := template.New(p.name).Parse(p.template)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var sb strings.Builder
	d, err := p.promptData(ctx, provider, model, store)
	if err != nil {
		return "", err
	}
	if err := t.Execute(&sb, d); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return sb.String(), nil
}

func processFile(filePath string) *ContextFile {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	return &ContextFile{
		Path:    filePath,
		Content: string(content),
	}
}

// expandPath expands ~ and environment variables in file paths
func expandPath(path string, store *config.ConfigStore) string {
	path = home.Long(path)
	// Handle environment variable expansion using the same pattern as config
	if strings.HasPrefix(path, "$") {
		if expanded, err := store.Resolver().ResolveValue(path); err == nil {
			path = expanded
		}
	}

	return path
}

func LoadContextSources(ctx context.Context, store *config.ConfigStore, activeSkills []*skills.Skill, mcpInstructions []string) ContextLoadResult {
	var result ContextLoadResult
	if store == nil || store.Config() == nil || store.Config().Options == nil {
		return result
	}
	result.Sources = append(result.Sources, managedContextSource())
	seen := map[string]struct{}{
		result.Sources[0].ID: {},
	}
	for _, pth := range orderedContextPaths(discoverContextPaths(store)) {
		for _, source := range loadContextPath(ctx, pth, store, nil, 0, map[string]struct{}{}) {
			key := strings.ToLower(source.ID)
			if _, ok := seen[key]; ok {
				duplicate := source
				duplicate.State = ContextStateDisabled
				duplicate.Reason = "duplicate_suppressed"
				duplicate.Diagnostics = "context source duplicates an earlier loaded source"
				result.Sources = append(result.Sources, duplicate)
				continue
			}
			seen[key] = struct{}{}
			result.Sources = append(result.Sources, source)
		}
	}
	if len(activeSkills) > 0 {
		xml := skills.ToPromptXML(activeSkills)
		result.Sources = append(result.Sources, ContextSource{
			ID:             "skill:available",
			Kind:           ContextSourceSkill,
			Name:           "Available skills",
			URI:            "crush://skills",
			Scope:          "runtime",
			Enabled:        true,
			State:          ContextStateLoaded,
			Reason:         "runtime_selected",
			ContentSummary: fmt.Sprintf("%d active skill sources available", len(activeSkills)),
			TokenEstimate:  skills.ApproxTokenCount(xml),
			LoadedAt:       time.Now().UTC(),
			Content:        xml,
		})
	}
	for i, instruction := range mcpInstructions {
		if strings.TrimSpace(instruction) == "" {
			continue
		}
		result.Sources = append(result.Sources, ContextSource{
			ID:             fmt.Sprintf("mcp:instructions:%d", i+1),
			Kind:           ContextSourceMCP,
			Name:           "MCP instructions",
			URI:            fmt.Sprintf("mcp://instructions/%d", i+1),
			Scope:          "runtime",
			Enabled:        true,
			State:          ContextStateLoaded,
			Reason:         "server_instructions",
			ContentSummary: summarizeContent(instruction),
			TokenEstimate:  skills.ApproxTokenCount(instruction),
			LoadedAt:       time.Now().UTC(),
			Content:        instruction,
		})
	}
	for _, source := range result.Sources {
		if source.Kind == ContextSourceProjectAgents || source.Kind == ContextSourceProjectClaude || source.Kind == ContextSourceDotClaude || source.Kind == ContextSourceClaudeRule || source.Kind == ContextSourceLocal || source.Kind == ContextSourceUser || source.Kind == ContextSourceFile {
			if source.State == ContextStateLoaded {
				result.ContextFiles = append(result.ContextFiles, ContextFile{Path: source.Path, Content: source.Content})
			}
		}
	}
	return result
}

func discoverContextPaths(store *config.ConfigStore) []string {
	if store == nil || store.Config() == nil || store.Config().Options == nil {
		return nil
	}
	var paths []string
	if _, err := os.Stat(home.Long("~/.claude/CLAUDE.md")); err == nil {
		paths = append(paths, "~/.claude/CLAUDE.md")
	}
	paths = append(paths, store.Config().Options.ContextPaths...)
	paths = append(paths, discoveredInstructionPaths(store.WorkingDir())...)
	return paths
}

func discoveredInstructionPaths(workingDir string) []string {
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return nil
	}
	var dirs []string
	for {
		dirs = append(dirs, abs)
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	slices.Reverse(dirs)
	var paths []string
	for _, dir := range dirs {
		for _, rel := range []string{"AGENTS.md", "CLAUDE.md", filepath.Join(".claude", "CLAUDE.md")} {
			path := filepath.Join(dir, rel)
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
			}
		}
		rulesDir := filepath.Join(dir, ".claude", "rules")
		rules, _ := filepath.Glob(filepath.Join(rulesDir, "*.md"))
		slices.Sort(rules)
		paths = append(paths, rules...)
		for _, rel := range []string{"AGENTS.local.md", "CLAUDE.local.md"} {
			path := filepath.Join(dir, rel)
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func managedContextSource() ContextSource {
	return ContextSource{
		ID:             "managed:coder",
		Kind:           ContextSourceManaged,
		Name:           "Coder system defaults",
		Scope:          "runtime",
		Enabled:        true,
		State:          ContextStateLoaded,
		Reason:         "embedded_template",
		ContentSummary: "Embedded coder system prompt defaults.",
		LoadedAt:       time.Now().UTC(),
	}
}

func orderedContextPaths(paths []string) []string {
	unique := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(filepath.ToSlash(p))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, p)
	}

	slices.SortStableFunc(unique, func(a, b string) int {
		aSlash := filepath.ToSlash(a)
		bSlash := filepath.ToSlash(b)
		if cmp := cmp.Compare(contextPathPriority(aSlash), contextPathPriority(bSlash)); cmp != 0 {
			return cmp
		}
		return strings.Compare(strings.ToLower(aSlash), strings.ToLower(bSlash))
	})
	return unique
}

func contextPathPriority(path string) int {
	switch {
	case isUserContextPath(path):
		return 0
	case isProjectInstructionFile(path, "agents.md"):
		return 10
	case isProjectInstructionFile(path, "claude.md"):
		return 20
	case strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/.claude/claude.md"):
		return 30
	case strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/.claude/rules/") || strings.HasPrefix(strings.ToLower(filepath.ToSlash(path)), ".claude/rules/"):
		return 35
	case isProjectInstructionPath(path) && !isLocalInstructionPath(path):
		return 36
	case isLocalInstructionPath(path):
		return 40
	default:
		return 50
	}
}

func isProjectInstructionFile(path, name string) bool {
	return strings.EqualFold(filepath.Base(filepath.ToSlash(path)), name)
}

func loadContextPath(ctx context.Context, configured string, store *config.ConfigStore, parent *ContextSource, depth int, includeStack map[string]struct{}) []ContextSource {
	abs, err := resolveContextPath(configured, store)
	if err != nil {
		return []ContextSource{unavailableContextSource(configured, "", classifyContextKind(configured), "path_invalid", err.Error(), parent)}
	}
	if !contextPathAllowed(store.WorkingDir(), configured, abs, parent == nil && filepath.IsAbs(configured)) {
		return []ContextSource{unavailableContextSource(configured, abs, classifyContextKind(configured), "outside_workspace", "context path is outside the workspace", parent)}
	}
	return loadResolvedContextPath(ctx, configured, abs, store, parent, depth, includeStack)
}

func resolveContextPath(configured string, store *config.ConfigStore) (string, error) {
	expanded := expandPath(configured, store)
	fullPath := expanded
	if !filepath.IsAbs(expanded) {
		fullPath = filepathext.SmartJoin(store.WorkingDir(), expanded)
	}
	abs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func loadResolvedContextPath(ctx context.Context, configured, abs string, store *config.ConfigStore, parent *ContextSource, depth int, includeStack map[string]struct{}) []ContextSource {
	info, err := os.Stat(abs)
	if err != nil {
		state := ContextStateUnavailable
		reason := "missing"
		diagnostics := "context path does not exist"
		if !os.IsNotExist(err) {
			state = ContextStateFailed
			reason = "stat_failed"
			diagnostics = err.Error()
		}
		return []ContextSource{{
			ID:          contextSourceIDForPath(configured, abs),
			Kind:        classifyContextKind(configured),
			Name:        contextSourceName(configured),
			Path:        filepath.ToSlash(abs),
			Scope:       contextSourceScope(configured),
			Enabled:     true,
			State:       state,
			Reason:      reason,
			Diagnostics: diagnostics,
			Error:       errorForState(state, diagnostics),
			ParentID:    parentID(parent),
			Provenance:  provenance(parent, configured),
		}}
	}
	if !info.IsDir() {
		return loadContextFile(ctx, configured, abs, classifyContextKindForPath(configured, abs), store, parent, depth, includeStack)
	}
	var sources []ContextSource
	walkErr := filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			sources = append(sources, unavailableContextSource(configured, path, ContextSourceFile, "walk_failed", err.Error(), parent))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		sources = append(sources, loadContextFile(ctx, configured, path, ContextSourceFile, store, parent, depth, includeStack)...)
		return nil
	})
	if walkErr != nil && ctx.Err() == nil {
		sources = append(sources, unavailableContextSource(configured, abs, ContextSourceFile, "walk_failed", walkErr.Error(), parent))
	}
	if len(sources) == 0 {
		return []ContextSource{{
			ID:             contextSourceIDForPath(configured, abs),
			Kind:           ContextSourceFile,
			Name:           contextSourceName(configured),
			Path:           filepath.ToSlash(abs),
			Scope:          contextSourceScope(configured),
			Enabled:        true,
			State:          ContextStateUnavailable,
			Reason:         "empty_directory",
			Diagnostics:    "context directory contains no files",
			ContentSummary: "No context files were loaded.",
			ParentID:       parentID(parent),
			Provenance:     provenance(parent, configured),
		}}
	}
	return sources
}

func loadContextFile(ctx context.Context, configured, abs string, kind ContextSourceKind, store *config.ConfigStore, parent *ContextSource, depth int, includeStack map[string]struct{}) []ContextSource {
	source := ContextSource{
		ID:         contextSourceIDForPath(configured, abs),
		Kind:       kind,
		Name:       contextSourceName(abs),
		Path:       filepath.ToSlash(abs),
		Scope:      contextSourceScope(configured),
		Enabled:    true,
		State:      ContextStateLoading,
		Reason:     "runtime_context_load",
		ParentID:   parentID(parent),
		Provenance: provenance(parent, configured),
	}
	if _, ok := includeStack[strings.ToLower(filepath.Clean(abs))]; ok {
		source.State = ContextStateFailed
		source.Reason = "include_cycle"
		source.Diagnostics = "include cycle detected"
		source.Error = source.Diagnostics
		return []ContextSource{source}
	}
	if depth > maxContextIncludeDepth {
		source.State = ContextStateFailed
		source.Reason = "include_depth_exceeded"
		source.Diagnostics = fmt.Sprintf("include depth exceeds %d", maxContextIncludeDepth)
		source.Error = source.Diagnostics
		return []ContextSource{source}
	}
	info, statErr := os.Stat(abs)
	if statErr == nil {
		source.SizeBytes = info.Size()
		source.MTimeUnix = info.ModTime().Unix()
		if info.Size() > maxContextFileBytes {
			source.State = ContextStateFailed
			source.Reason = "file_too_large"
			source.Diagnostics = fmt.Sprintf("context source is %d bytes; limit is %d bytes", info.Size(), maxContextFileBytes)
			source.Error = source.Diagnostics
			return []ContextSource{source}
		}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		source.State = ContextStateFailed
		source.Reason = "read_failed"
		source.Diagnostics = err.Error()
		source.Error = err.Error()
		return []ContextSource{source}
	}
	if !utf8.Valid(data) || containsNUL(data) {
		source.State = ContextStateFailed
		source.Reason = "binary_or_non_utf8"
		source.Diagnostics = "context source is not valid UTF-8 text"
		source.Error = source.Diagnostics
		return []ContextSource{source}
	}
	content := string(data)
	content, globs := parseFrontmatter(content)
	source.RuleGlobs = globs
	if len(globs) > 0 && !ruleMatchesWorkspace(globs, abs) {
		source.State = ContextStateDisabled
		source.Reason = "frontmatter_path_not_matched"
		source.Diagnostics = "rule frontmatter paths do not match this workspace path"
		return []ContextSource{source}
	}
	sum := sha256.Sum256(data)
	source.ContentHash = "sha256:" + hex.EncodeToString(sum[:])
	source.State = ContextStateLoaded
	source.Content = content
	source.ContentSummary = summarizeContent(content)
	source.TokenEstimate = skills.ApproxTokenCount(content)
	source.LoadedAt = time.Now().UTC()
	stack := cloneStringSet(includeStack)
	stack[strings.ToLower(filepath.Clean(abs))] = struct{}{}
	includes := extractIncludes(content)
	var sources []ContextSource
	for _, inc := range includes {
		includePath := resolveIncludePath(abs, inc)
		if includePath == "" {
			continue
		}
		if !contextPathAllowed(store.WorkingDir(), includePath, includePath, true) {
			sources = append(sources, unavailableContextSource(inc, includePath, kind, "include_outside_scope", "include path is outside the including source scope", &source))
			continue
		}
		sources = append(sources, loadResolvedContextPath(ctx, includePath, includePath, store, &source, depth+1, stack)...)
	}
	sources = append(sources, source)
	return sources
}

func unavailableContextSource(configured, path string, kind ContextSourceKind, reason, diagnostics string, parent *ContextSource) ContextSource {
	return ContextSource{
		ID:          contextSourceIDForPath(configured, path),
		Kind:        kind,
		Name:        contextSourceName(configured),
		Path:        filepath.ToSlash(path),
		Scope:       contextSourceScope(configured),
		Enabled:     true,
		State:       ContextStateUnavailable,
		Reason:      reason,
		Diagnostics: diagnostics,
		ParentID:    parentID(parent),
		Provenance:  provenance(parent, configured),
	}
}

func contextPathAllowed(workingDir, configured, abs string, allowAncestor bool) bool {
	if isUserContextPath(configured) {
		return true
	}
	workspaceAbs, err := filepath.Abs(workingDir)
	if err != nil {
		return false
	}
	if allowAncestor {
		base := contextSourceRoot(abs)
		baseClean := strings.ToLower(filepath.Clean(base))
		workspaceClean := strings.ToLower(filepath.Clean(workspaceAbs))
		absClean := strings.ToLower(filepath.Clean(abs))
		if workspaceClean == baseClean || strings.HasPrefix(workspaceClean, baseClean+string(filepath.Separator)) ||
			absClean == workspaceClean || strings.HasPrefix(absClean, workspaceClean+string(filepath.Separator)) {
			return true
		}
		relToWorkspace, err := filepath.Rel(base, workspaceAbs)
		if err == nil && relToWorkspace != ".." && !strings.HasPrefix(relToWorkspace, ".."+string(filepath.Separator)) && !filepath.IsAbs(relToWorkspace) {
			return true
		}
		if relToWorkspace == ".." || strings.HasPrefix(relToWorkspace, ".."+string(filepath.Separator)) {
			relFromWorkspace, err := filepath.Rel(workspaceAbs, abs)
			if err == nil && relFromWorkspace != ".." && !strings.HasPrefix(relFromWorkspace, ".."+string(filepath.Separator)) && !filepath.IsAbs(relFromWorkspace) {
				return true
			}
		}
	}
	rel, err := filepath.Rel(workspaceAbs, abs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func contextSourceRoot(path string) string {
	clean := filepath.Clean(path)
	lower := strings.ToLower(clean)
	marker := string(filepath.Separator) + ".claude" + string(filepath.Separator)
	if idx := strings.Index(lower, marker); idx > 0 {
		return clean[:idx]
	}
	if info, err := os.Stat(clean); err == nil && info.IsDir() {
		return clean
	}
	return filepath.Dir(clean)
}

func classifyContextKind(path string) ContextSourceKind {
	return classifyContextKindForPath(path, path)
}

func classifyContextKindForPath(configured, abs string) ContextSourceKind {
	path := configured
	if abs != "" {
		path = abs
	}
	slash := filepath.ToSlash(path)
	lower := strings.ToLower(slash)
	switch {
	case isUserContextPath(configured):
		return ContextSourceUser
	case isLocalInstructionPath(slash):
		return ContextSourceLocal
	case strings.Contains(lower, "/.claude/rules/") || strings.HasPrefix(lower, ".claude/rules/"):
		return ContextSourceClaudeRule
	case strings.Contains(lower, "/.claude/claude.md") || strings.HasSuffix(lower, ".claude/claude.md"):
		return ContextSourceDotClaude
	case strings.EqualFold(filepath.Base(slash), "AGENTS.md"):
		return ContextSourceProjectAgents
	case strings.EqualFold(filepath.Base(slash), "CLAUDE.md"):
		return ContextSourceProjectClaude
	default:
		return ContextSourceFile
	}
}

func isUserContextPath(path string) bool {
	return strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "$")
}

func isProjectInstructionPath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	lower := strings.ToLower(filepath.ToSlash(path))
	return base == "agents.md" || base == "claude.md" || strings.Contains(lower, "/.claude/rules/") || strings.HasPrefix(lower, ".claude/rules/") || strings.HasPrefix(lower, ".agents/rules/")
}

func isLocalInstructionPath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	return base == "agents.local.md" || base == "claude.local.md"
}

func contextSourceScope(path string) string {
	switch {
	case isUserContextPath(path):
		return "user"
	case isLocalInstructionPath(path):
		return "local"
	case isProjectInstructionPath(path):
		return "project"
	default:
		return "workspace"
	}
}

func contextSourceName(path string) string {
	base := filepath.Base(filepath.ToSlash(path))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return path
	}
	return base
}

func contextSourceID(configured, abs string) string {
	return contextSourceIDForPath(configured, abs)
}

func contextSourceIDForPath(configured, abs string) string {
	key := filepath.ToSlash(abs)
	if key == "" {
		key = configured
	}
	return string(classifyContextKindForPath(configured, abs)) + ":" + strings.ToLower(key)
}

func containsNUL(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func parseFrontmatter(content string) (string, []string) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return content, nil
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return content, nil
	}
	block := normalized[4 : 4+end]
	rest := strings.TrimPrefix(normalized[4+end:], "\n---")
	rest = strings.TrimPrefix(rest, "\n")
	var paths []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "paths:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, line[:len("paths:")]))
		value = strings.Trim(value, "[]")
		for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			part = strings.Trim(strings.TrimSpace(part), `"'`)
			if part != "" {
				paths = append(paths, part)
			}
		}
	}
	return rest, paths
}

func ruleMatchesWorkspace(globs []string, abs string) bool {
	if len(globs) == 0 {
		return true
	}
	base := filepath.Dir(filepath.Dir(filepath.Dir(abs)))
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, glob := range globs {
		glob = strings.TrimSpace(filepath.ToSlash(glob))
		if glob == "" || glob == "**" {
			return true
		}
		if ok, _ := filepath.Match(glob, rel); ok {
			return true
		}
		if strings.HasSuffix(glob, "/**") && strings.HasPrefix(rel, strings.TrimSuffix(glob, "/**")+"/") {
			return true
		}
	}
	return false
}

func extractIncludes(content string) []string {
	var includes []string
	inCode := false
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		fields := strings.Fields(line)
		for _, field := range fields {
			if !strings.HasPrefix(field, "@") || len(field) <= 1 {
				continue
			}
			value := strings.TrimRight(strings.TrimPrefix(field, "@"), ".,;:)]}\"'")
			if strings.HasPrefix(value, "#") || strings.Contains(value, "://") {
				continue
			}
			includes = append(includes, value)
		}
	}
	return includes
}

func resolveIncludePath(parentPath, include string) string {
	include = strings.TrimSpace(include)
	if include == "" {
		return ""
	}
	if idx := strings.Index(include, "#"); idx >= 0 {
		include = include[:idx]
	}
	include = strings.ReplaceAll(include, `\ `, " ")
	include = home.Long(include)
	if filepath.IsAbs(include) {
		return filepath.Clean(include)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(parentPath), include))
}

func cloneStringSet(values map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for key := range values {
		out[key] = struct{}{}
	}
	return out
}

func parentID(parent *ContextSource) string {
	if parent == nil {
		return ""
	}
	return parent.ID
}

func provenance(parent *ContextSource, configured string) string {
	if parent == nil {
		return "discovered:" + filepath.ToSlash(configured)
	}
	return "included_by:" + parent.ID
}

func summarizeContent(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if strings.TrimSpace(normalized) == "" {
		return fmt.Sprintf("Empty context source (%d bytes).", len(content))
	}
	lineCount := strings.Count(normalized, "\n")
	if !strings.HasSuffix(normalized, "\n") {
		lineCount++
	}
	return fmt.Sprintf("Context source loaded (%d bytes, %d lines).", len(content), lineCount)
}

func errorForState(state ContextSourceState, diagnostics string) string {
	if state == ContextStateFailed {
		return diagnostics
	}
	return ""
}

func (p *Prompt) promptData(ctx context.Context, provider, model string, store *config.ConfigStore) (PromptDat, error) {
	workingDir := cmp.Or(p.workingDir, store.WorkingDir())
	platform := cmp.Or(p.platform, runtime.GOOS)
	cfg := store.Config()

	var availSkillXML string
	if len(p.skills) > 0 {
		availSkillXML = skills.ToPromptXML(p.skills)
	}
	contextLoad := LoadContextSources(ctx, store, nil, nil)

	isGit := isGitRepo(store.WorkingDir())
	data := PromptDat{
		Provider:      provider,
		Model:         model,
		Config:        *cfg,
		WorkingDir:    filepath.ToSlash(workingDir),
		IsGitRepo:     isGit,
		Platform:      platform,
		Date:          p.now().Format("1/2/2006"),
		AvailSkillXML: availSkillXML,
	}
	if isGit {
		var err error
		data.GitStatus, err = getGitStatus(ctx, store.WorkingDir())
		if err != nil {
			return PromptDat{}, err
		}
	}

	data.ContextFiles = append(data.ContextFiles, contextLoad.ContextFiles...)
	return data, nil
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func getGitStatus(ctx context.Context, dir string) (string, error) {
	sh := shell.NewShell(&shell.Options{
		WorkingDir: dir,
	})
	branch, err := getGitBranch(ctx, sh)
	if err != nil {
		return "", err
	}
	status, err := getGitStatusSummary(ctx, sh)
	if err != nil {
		return "", err
	}
	commits, err := getGitRecentCommits(ctx, sh)
	if err != nil {
		return "", err
	}
	return branch + status + commits, nil
}

func getGitBranch(ctx context.Context, sh *shell.Shell) (string, error) {
	out, _, err := sh.Exec(ctx, "git branch --show-current 2>/dev/null")
	if err != nil {
		return "", nil
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "", nil
	}
	return fmt.Sprintf("Current branch: %s\n", out), nil
}

func getGitStatusSummary(ctx context.Context, sh *shell.Shell) (string, error) {
	out, _, err := sh.Exec(ctx, "git status --short 2>/dev/null | head -20")
	if err != nil {
		return "", nil
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "Status: clean\n", nil
	}
	return fmt.Sprintf("Status:\n%s\n", out), nil
}

func getGitRecentCommits(ctx context.Context, sh *shell.Shell) (string, error) {
	out, _, err := sh.Exec(ctx, "git log --oneline -n 3 2>/dev/null")
	if err != nil || out == "" {
		return "", nil
	}
	out = strings.TrimSpace(out)
	return fmt.Sprintf("Recent commits:\n%s\n", out), nil
}

func (p *Prompt) Name() string {
	return p.name
}
