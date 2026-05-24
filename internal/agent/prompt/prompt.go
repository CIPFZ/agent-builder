package prompt

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
	"time"

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
	ContextSourceManaged   ContextSourceKind = "managed"
	ContextSourceUser      ContextSourceKind = "user"
	ContextSourceProject   ContextSourceKind = "project"
	ContextSourceLocal     ContextSourceKind = "local"
	ContextSourceSkill     ContextSourceKind = "skill"
	ContextSourceMCP       ContextSourceKind = "mcp"
	ContextSourceFile      ContextSourceKind = "file"
	ContextSourceGenerated ContextSourceKind = "generated"
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
	Content        string
}

type ContextLoadResult struct {
	Sources      []ContextSource
	ContextFiles []ContextFile
}

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
	for _, pth := range orderedContextPaths(store.Config().Options.ContextPaths) {
		for _, source := range loadContextPath(ctx, pth, store) {
			key := strings.ToLower(source.ID)
			if _, ok := seen[key]; ok {
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
		if source.Kind == ContextSourceProject || source.Kind == ContextSourceLocal || source.Kind == ContextSourceUser || source.Kind == ContextSourceFile {
			if source.State == ContextStateLoaded {
				result.ContextFiles = append(result.ContextFiles, ContextFile{Path: source.Path, Content: source.Content})
			}
		}
	}
	return result
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
	case isProjectInstructionPath(path) && !isLocalInstructionPath(path):
		return 30
	case isLocalInstructionPath(path):
		return 40
	default:
		return 50
	}
}

func isProjectInstructionFile(path, name string) bool {
	return strings.EqualFold(filepath.Base(filepath.ToSlash(path)), name)
}

func loadContextPath(ctx context.Context, configured string, store *config.ConfigStore) []ContextSource {
	expanded := expandPath(configured, store)
	fullPath := filepathext.SmartJoin(store.WorkingDir(), expanded)
	abs, err := filepath.Abs(fullPath)
	if err != nil {
		return []ContextSource{unavailableContextSource(configured, "", classifyContextKind(configured), "path_invalid", err.Error())}
	}
	if !pathAllowed(store.WorkingDir(), configured, abs) {
		return []ContextSource{unavailableContextSource(configured, abs, classifyContextKind(configured), "outside_workspace", "context path is outside the workspace")}
	}
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
			ID:          contextSourceID(configured, abs),
			Kind:        classifyContextKind(configured),
			Name:        contextSourceName(configured),
			Path:        filepath.ToSlash(abs),
			Scope:       contextSourceScope(configured),
			Enabled:     true,
			State:       state,
			Reason:      reason,
			Diagnostics: diagnostics,
			Error:       errorForState(state, diagnostics),
		}}
	}
	if !info.IsDir() {
		return []ContextSource{loadContextFile(ctx, configured, abs, classifyContextKind(configured))}
	}
	var sources []ContextSource
	walkErr := filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			sources = append(sources, unavailableContextSource(configured, path, ContextSourceFile, "walk_failed", err.Error()))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		sources = append(sources, loadContextFile(ctx, configured, path, ContextSourceFile))
		return nil
	})
	if walkErr != nil && ctx.Err() == nil {
		sources = append(sources, unavailableContextSource(configured, abs, ContextSourceFile, "walk_failed", walkErr.Error()))
	}
	if len(sources) == 0 {
		return []ContextSource{{
			ID:             contextSourceID(configured, abs),
			Kind:           ContextSourceFile,
			Name:           contextSourceName(configured),
			Path:           filepath.ToSlash(abs),
			Scope:          contextSourceScope(configured),
			Enabled:        true,
			State:          ContextStateUnavailable,
			Reason:         "empty_directory",
			Diagnostics:    "context directory contains no files",
			ContentSummary: "No context files were loaded.",
		}}
	}
	return sources
}

func loadContextFile(_ context.Context, configured, abs string, kind ContextSourceKind) ContextSource {
	source := ContextSource{
		ID:      contextSourceID(configured, abs),
		Kind:    kind,
		Name:    contextSourceName(abs),
		Path:    filepath.ToSlash(abs),
		Scope:   contextSourceScope(configured),
		Enabled: true,
		State:   ContextStateLoading,
		Reason:  "runtime_context_load",
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		source.State = ContextStateFailed
		source.Reason = "read_failed"
		source.Diagnostics = err.Error()
		source.Error = err.Error()
		return source
	}
	content := string(data)
	source.State = ContextStateLoaded
	source.Content = content
	source.ContentSummary = summarizeContent(content)
	source.TokenEstimate = skills.ApproxTokenCount(content)
	source.LoadedAt = time.Now().UTC()
	return source
}

func unavailableContextSource(configured, path string, kind ContextSourceKind, reason, diagnostics string) ContextSource {
	return ContextSource{
		ID:          contextSourceID(configured, path),
		Kind:        kind,
		Name:        contextSourceName(configured),
		Path:        filepath.ToSlash(path),
		Scope:       contextSourceScope(configured),
		Enabled:     true,
		State:       ContextStateUnavailable,
		Reason:      reason,
		Diagnostics: diagnostics,
	}
}

func pathAllowed(workingDir, configured, abs string) bool {
	if isUserContextPath(configured) {
		return true
	}
	workspaceAbs, err := filepath.Abs(workingDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(workspaceAbs, abs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func classifyContextKind(path string) ContextSourceKind {
	slash := filepath.ToSlash(path)
	switch {
	case isUserContextPath(slash):
		return ContextSourceUser
	case isLocalInstructionPath(slash):
		return ContextSourceLocal
	case isProjectInstructionPath(slash):
		return ContextSourceProject
	default:
		return ContextSourceFile
	}
}

func isUserContextPath(path string) bool {
	return strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "$")
}

func isProjectInstructionPath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	return base == "agents.md" || base == "claude.md" || strings.HasPrefix(strings.ToLower(filepath.ToSlash(path)), ".agents/rules/")
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
	key := filepath.ToSlash(abs)
	if key == "" {
		key = configured
	}
	return string(classifyContextKind(configured)) + ":" + strings.ToLower(key)
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
