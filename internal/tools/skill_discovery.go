package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type SkillShellRequest struct {
	SkillName string
	Shell     FrontmatterShell
	Command   string
	BaseDir   string
	SessionID string
	AppState  map[string]any
}

type SkillShellExecutor func(context.Context, SkillShellRequest) (string, error)

type SkillDiscoveryOptions struct {
	CWD             string
	ConfigHome      string
	ManagedRoot     string
	AdditionalDirs  []string
	IncludeManaged  bool
	IncludeUser     bool
	IncludeProject  bool
	IncludeExplicit bool
	IncludeLegacy   bool
	BareMode        bool
	SkillsLocked    bool
}

type DynamicSkillDirectory struct {
	SkillDir   string
	SkillNames []string
}

var skillDiscoveryState = struct {
	sync.RWMutex
	dynamicSkillDirs               map[string]struct{}
	dynamicSkills                  map[string]skillCommand
	dynamicSkillOrder              []string
	conditionalSkills              map[string]skillCommand
	activatedConditionalSkillNames map[string]struct{}
}{
	dynamicSkillDirs:               make(map[string]struct{}),
	dynamicSkills:                  make(map[string]skillCommand),
	dynamicSkillOrder:              nil,
	conditionalSkills:              make(map[string]skillCommand),
	activatedConditionalSkillNames: make(map[string]struct{}),
}

var (
	skillShellBlockPattern  = regexp.MustCompile("(?s)```!\\s*\\n?(.*?)\\n?```")
	skillShellInlinePattern = regexp.MustCompile("(^|\\s)!`([^`]+)`")
)

// DiscoverSkillDirsForPaths walks upward from the provided file paths and
// returns nested `.claude/skills` directories below the cwd boundary.
func DiscoverSkillDirsForPaths(filePaths []string, cwd string) []string {
	resolvedCwd := normalizeSkillDiscoveryPath(cwd, "")
	if resolvedCwd == "" {
		return nil
	}
	found := make(map[string]struct{})
	dirs := make([]string, 0)

	for _, filePath := range filePaths {
		resolvedFile := normalizeSkillDiscoveryPath(filePath, resolvedCwd)
		if resolvedFile == "" {
			continue
		}
		for current := filepath.Dir(resolvedFile); ; current = filepath.Dir(current) {
			if current == resolvedCwd {
				break
			}
			rel, err := filepath.Rel(resolvedCwd, current)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				break
			}
			skillDir := filepath.Join(current, ".claude", "skills")
			if isSimpleGitignoredSkillDir(skillDir, resolvedCwd) {
				break
			}
			if _, exists := found[skillDir]; !exists {
				if _, err := os.Stat(skillDir); err == nil {
					found[skillDir] = struct{}{}
					dirs = append(dirs, skillDir)
				}
			}
			if parent := filepath.Dir(current); parent == current {
				break
			}
		}
	}

	sort.SliceStable(dirs, func(i, j int) bool {
		left := strings.Count(filepath.Clean(dirs[i]), string(filepath.Separator))
		right := strings.Count(filepath.Clean(dirs[j]), string(filepath.Separator))
		if left != right {
			return left > right
		}
		return dirs[i] < dirs[j]
	})
	return dirs
}

// AddSkillDirectories loads skill commands from discovered directories and
// merges them into the process-wide dynamic skill registry.
func AddSkillDirectories(dirs []string) []DynamicSkillDirectory {
	if len(dirs) == 0 {
		return nil
	}

	skillDiscoveryState.Lock()
	defer skillDiscoveryState.Unlock()

	addedByDir := make([]DynamicSkillDirectory, 0)
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := normalizeSkillDiscoveryPath(dirs[i], "")
		if dir == "" {
			continue
		}
		if _, seen := skillDiscoveryState.dynamicSkillDirs[dir]; !seen {
			skillDiscoveryState.dynamicSkillDirs[dir] = struct{}{}
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}
			command := ParseSkillFile(entry.Name(), skillPath, string(data))
			if registerDynamicSkillLocked(command) {
				names = append(names, command.Name)
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			addedByDir = append(addedByDir, DynamicSkillDirectory{SkillDir: dir, SkillNames: names})
		}
	}
	sort.SliceStable(addedByDir, func(i, j int) bool {
		return addedByDir[i].SkillDir < addedByDir[j].SkillDir
	})
	return addedByDir
}

func LoadClaudeSkillDirectories(opts SkillDiscoveryOptions) []SkillCommand {
	dirs := claudeSkillSourceDirs(opts)
	seenFileIDs := make(map[string]struct{})
	loaded := make([]SkillCommand, 0)
	for _, dir := range dirs {
		for _, command := range loadSkillsFromDirectory(dir.path, dir.legacy) {
			fileID := skillFileIdentity(command.Path)
			if fileID != "" {
				if _, seen := seenFileIDs[fileID]; seen {
					continue
				}
				seenFileIDs[fileID] = struct{}{}
			}
			registerLoadedSkill(command)
			loaded = append(loaded, command)
		}
	}
	return loaded
}

type claudeSkillSourceDir struct {
	path   string
	legacy bool
}

func claudeSkillSourceDirs(opts SkillDiscoveryOptions) []claudeSkillSourceDir {
	if opts.SkillsLocked {
		return nil
	}
	out := make([]claudeSkillSourceDir, 0)
	if opts.BareMode {
		if opts.IncludeExplicit {
			for _, dir := range opts.AdditionalDirs {
				out = append(out, claudeSkillSourceDir{path: filepath.Join(dir, ".claude", "skills")})
			}
		}
		return out
	}
	if opts.IncludeManaged && opts.ManagedRoot != "" {
		out = append(out, claudeSkillSourceDir{path: filepath.Join(opts.ManagedRoot, ".claude", "skills")})
	}
	if opts.IncludeUser && opts.ConfigHome != "" {
		out = append(out, claudeSkillSourceDir{path: filepath.Join(opts.ConfigHome, "skills")})
	}
	if opts.IncludeProject {
		for _, dir := range projectSkillDirsUpToHome(opts.CWD) {
			out = append(out, claudeSkillSourceDir{path: dir})
		}
	}
	if opts.IncludeExplicit {
		for _, dir := range opts.AdditionalDirs {
			out = append(out, claudeSkillSourceDir{path: filepath.Join(dir, ".claude", "skills")})
		}
	}
	if opts.IncludeLegacy {
		for _, dir := range legacyCommandDirsUpToHome(opts.CWD) {
			out = append(out, claudeSkillSourceDir{path: dir, legacy: true})
		}
	}
	return out
}

func projectSkillDirsUpToHome(cwd string) []string {
	start := normalizeSkillDiscoveryPath(cwd, "")
	if start == "" {
		return nil
	}
	dirs := make([]string, 0)
	for current := start; ; {
		dirs = append(dirs, filepath.Join(current, ".claude", "skills"))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dirs
}

func legacyCommandDirsUpToHome(cwd string) []string {
	start := normalizeSkillDiscoveryPath(cwd, "")
	if start == "" {
		return nil
	}
	dirs := make([]string, 0)
	for current := start; ; {
		dirs = append(dirs, filepath.Join(current, ".claude", "commands"))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dirs
}

func loadSkillsFromDirectory(dir string, legacy bool) []skillCommand {
	dir = normalizeSkillDiscoveryPath(dir, "")
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]skillCommand, 0)
	for _, entry := range entries {
		entryPath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			skillPath := filepath.Join(entryPath, "SKILL.md")
			data, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}
			out = append(out, ParseSkillFile(entry.Name(), skillPath, string(data)))
			continue
		}
		if legacy && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			data, err := os.ReadFile(entryPath)
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			out = append(out, ParseSkillFile(name, entryPath, string(data)))
		}
	}
	return out
}

func skillFileIdentity(path string) string {
	path = normalizeSkillDiscoveryPath(path, "")
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if info, err := os.Stat(path); err == nil {
		return filepath.Clean(path) + "|" + info.ModTime().UTC().String() + "|" + fmt.Sprint(info.Size())
	}
	return filepath.Clean(path)
}

func registerLoadedSkill(command skillCommand) {
	skillDiscoveryState.Lock()
	defer skillDiscoveryState.Unlock()
	registerDynamicSkillLocked(command)
}

// ActivateConditionalSkillsForPaths activates pending skills whose paths
// frontmatter matches the provided file paths.
func ActivateConditionalSkillsForPaths(filePaths []string, cwd string) []string {
	skillDiscoveryState.Lock()
	defer skillDiscoveryState.Unlock()

	if len(skillDiscoveryState.conditionalSkills) == 0 {
		return nil
	}

	resolvedCwd := normalizeSkillDiscoveryPath(cwd, cwd)
	if resolvedCwd == "" {
		resolvedCwd = cwd
	}
	activated := make([]string, 0)
	for name, skill := range skillDiscoveryState.conditionalSkills {
		if skill.Name == "" || len(skill.Paths) == 0 {
			continue
		}
		if matchesAnySkillPathPattern(skill.Paths, filePaths, resolvedCwd) {
			if _, exists := skillDiscoveryState.dynamicSkills[name]; !exists {
				skillDiscoveryState.dynamicSkillOrder = append(skillDiscoveryState.dynamicSkillOrder, name)
			}
			skillDiscoveryState.dynamicSkills[name] = skill
			delete(skillDiscoveryState.conditionalSkills, name)
			skillDiscoveryState.activatedConditionalSkillNames[name] = struct{}{}
			activated = append(activated, name)
		}
	}
	sort.Strings(activated)
	return activated
}

// GetDynamicSkills returns all currently active dynamic skills.
func GetDynamicSkills() []SkillCommand {
	skillDiscoveryState.RLock()
	defer skillDiscoveryState.RUnlock()

	if len(skillDiscoveryState.dynamicSkills) == 0 {
		return nil
	}
	out := make([]SkillCommand, 0, len(skillDiscoveryState.dynamicSkills))
	seen := make(map[string]struct{}, len(skillDiscoveryState.dynamicSkillOrder))
	for _, name := range skillDiscoveryState.dynamicSkillOrder {
		skill, ok := skillDiscoveryState.dynamicSkills[name]
		if !ok {
			continue
		}
		out = append(out, skill)
		seen[name] = struct{}{}
	}
	for name, skill := range skillDiscoveryState.dynamicSkills {
		if _, ok := seen[name]; ok {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func lookupDynamicSkill(name string) (skillCommand, bool) {
	skillDiscoveryState.RLock()
	defer skillDiscoveryState.RUnlock()

	if skill, ok := skillDiscoveryState.dynamicSkills[name]; ok {
		return skill, true
	}
	for _, skill := range skillDiscoveryState.dynamicSkills {
		if skill.Name == name {
			return skill, true
		}
	}
	return skillCommand{}, false
}

// ClearDynamicSkills resets all dynamic skill state, primarily for tests.
func ClearDynamicSkills() {
	skillDiscoveryState.Lock()
	defer skillDiscoveryState.Unlock()

	skillDiscoveryState.dynamicSkillDirs = make(map[string]struct{})
	skillDiscoveryState.dynamicSkills = make(map[string]skillCommand)
	skillDiscoveryState.dynamicSkillOrder = nil
	skillDiscoveryState.conditionalSkills = make(map[string]skillCommand)
	skillDiscoveryState.activatedConditionalSkillNames = make(map[string]struct{})
}

func registerDynamicSkillLocked(skill skillCommand) bool {
	if skill.Name == "" {
		return false
	}
	if len(skill.Paths) > 0 && !hasSkillBeenActivatedLocked(skill.Name) {
		if _, exists := skillDiscoveryState.dynamicSkills[skill.Name]; exists {
			return false
		}
		if _, exists := skillDiscoveryState.conditionalSkills[skill.Name]; exists {
			return false
		}
		skillDiscoveryState.conditionalSkills[skill.Name] = skill
		delete(skillDiscoveryState.dynamicSkills, skill.Name)
		return false
	}
	if _, exists := skillDiscoveryState.dynamicSkills[skill.Name]; exists {
		return false
	}
	skillDiscoveryState.dynamicSkills[skill.Name] = skill
	skillDiscoveryState.dynamicSkillOrder = append(skillDiscoveryState.dynamicSkillOrder, skill.Name)
	delete(skillDiscoveryState.conditionalSkills, skill.Name)
	return true
}

func hasSkillBeenActivatedLocked(name string) bool {
	_, ok := skillDiscoveryState.activatedConditionalSkillNames[name]
	return ok
}

func matchesAnySkillPathPattern(patterns []string, filePaths []string, cwd string) bool {
	if len(patterns) == 0 || len(filePaths) == 0 {
		return false
	}
	for _, filePath := range filePaths {
		relativePath, ok := skillRelativePathForMatch(filePath, cwd)
		if !ok {
			continue
		}
		for _, pattern := range patterns {
			if skillPathPatternMatches(pattern, relativePath) {
				return true
			}
		}
	}
	return false
}

func skillRelativePathForMatch(filePath, cwd string) (string, bool) {
	resolvedFile := normalizeSkillDiscoveryPath(filePath, cwd)
	resolvedCwd := normalizeSkillDiscoveryPath(cwd, "")
	if resolvedFile == "" || resolvedCwd == "" {
		return "", false
	}
	rel, err := filepath.Rel(resolvedCwd, resolvedFile)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func skillPathPatternMatches(pattern, relPath string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if pattern == "" || relPath == "" {
		return false
	}
	patternSegments := strings.Split(pattern, "/")
	pathSegments := strings.Split(relPath, "/")
	return matchSkillPathSegments(patternSegments, pathSegments)
}

func matchSkillPathSegments(patternSegments, pathSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(pathSegments) == 0
	}
	if patternSegments[0] == "**" {
		for i := 0; i <= len(pathSegments); i++ {
			if matchSkillPathSegments(patternSegments[1:], pathSegments[i:]) {
				return true
			}
		}
		return false
	}
	if len(pathSegments) == 0 {
		return false
	}
	ok, err := filepath.Match(patternSegments[0], pathSegments[0])
	if err != nil || !ok {
		return false
	}
	return matchSkillPathSegments(patternSegments[1:], pathSegments[1:])
}

func isSimpleGitignoredSkillDir(skillDir, cwd string) bool {
	normalized := filepath.ToSlash(strings.ToLower(skillDir))
	if strings.Contains(normalized, "/node_modules/") {
		return true
	}
	if strings.Contains(normalized, "/.git/") {
		return true
	}
	root := normalizeSkillDiscoveryPath(cwd, "")
	if root == "" {
		root = cwd
	}
	current := skillDir
	for {
		ignoreFile := filepath.Join(current, ".gitignore")
		data, err := os.ReadFile(ignoreFile)
		if err == nil {
			text := strings.ToLower(string(data))
			if strings.Contains(text, "node_modules") || strings.Contains(text, ".claude/skills") {
				return true
			}
		}
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return false
}

func normalizeSkillDiscoveryPath(path string, base string) string {
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

func skillShellExecutorFromAppState(appState map[string]any) SkillShellExecutor {
	if appState == nil {
		return nil
	}
	executor, _ := appState["skillShellExecutor"].(SkillShellExecutor)
	return executor
}

func executeSkillShellCommandsInPrompt(
	ctx context.Context,
	text string,
	toolCtx ToolUseContext,
	slashCommandName string,
	baseDir string,
	shell FrontmatterShell,
) (string, error) {
	executor := skillShellExecutorFromAppState(toolCtx.AppState)
	if executor == nil {
		executor = defaultSkillShellExecutor
	}
	resolvedShell := shell
	if resolvedShell == "" {
		resolvedShell = FrontmatterShellBash
	}
	result := text
	var err error
	result, err = replaceSkillShellMatches(result, skillShellBlockPattern, func(match string, groups []string) (string, error) {
		command := strings.TrimSpace(groups[0])
		if command == "" {
			return match, nil
		}
		return executeSkillShellCommand(ctx, executor, toolCtx, slashCommandName, baseDir, resolvedShell, command)
	})
	if err != nil {
		return "", err
	}
	result, err = replaceSkillShellMatches(result, skillShellInlinePattern, func(match string, groups []string) (string, error) {
		prefix := groups[0]
		command := strings.TrimSpace(groups[1])
		if command == "" {
			return match, nil
		}
		output, err := executeSkillShellCommand(ctx, executor, toolCtx, slashCommandName, baseDir, resolvedShell, command)
		if err != nil {
			return "", err
		}
		return prefix + output, nil
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

func replaceSkillShellMatches(
	input string,
	pattern *regexp.Regexp,
	replacer func(string, []string) (string, error),
) (string, error) {
	indexes := pattern.FindAllStringSubmatchIndex(input, -1)
	if len(indexes) == 0 {
		return input, nil
	}
	var builder strings.Builder
	last := 0
	for _, loc := range indexes {
		if len(loc) < 2 {
			continue
		}
		builder.WriteString(input[last:loc[0]])
		match := input[loc[0]:loc[1]]
		groups := make([]string, 0, len(loc)/2-1)
		for i := 2; i < len(loc); i += 2 {
			if loc[i] < 0 || loc[i+1] < 0 {
				groups = append(groups, "")
				continue
			}
			groups = append(groups, input[loc[i]:loc[i+1]])
		}
		replacement, err := replacer(match, groups)
		if err != nil {
			return "", err
		}
		builder.WriteString(replacement)
		last = loc[1]
	}
	builder.WriteString(input[last:])
	return builder.String(), nil
}

func executeSkillShellCommand(
	ctx context.Context,
	executor SkillShellExecutor,
	toolCtx ToolUseContext,
	slashCommandName string,
	baseDir string,
	shell FrontmatterShell,
	command string,
) (string, error) {
	if executor == nil {
		executor = defaultSkillShellExecutor
	}
	return executor(ctx, SkillShellRequest{
		SkillName: slashCommandName,
		Shell:     shell,
		Command:   command,
		BaseDir:   baseDir,
		SessionID: toolCtx.Session.ID,
		AppState:  toolCtx.AppState,
	})
}

func skillCommandFromAppState(appState map[string]any, name string) (skillCommand, bool) {
	if appState == nil {
		return skillCommand{}, false
	}
	if command, ok := skillCommandFromAppStateValue(appState["dynamicSkills"], name); ok {
		return command, true
	}
	return skillCommand{}, false
}

func skillCommandFromAppStateValue(value any, name string) (skillCommand, bool) {
	switch typed := value.(type) {
	case nil:
		return skillCommand{}, false
	case skillCommand:
		if typed.Name == name || typed.Name == "" {
			return typed, true
		}
		return skillCommand{}, false
	case *skillCommand:
		if typed == nil {
			return skillCommand{}, false
		}
		if typed.Name == name || typed.Name == "" {
			return *typed, true
		}
		return skillCommand{}, false
	case map[string]skillCommand:
		if command, ok := typed[name]; ok {
			return command, true
		}
		for _, command := range typed {
			if command.Name == name {
				return command, true
			}
		}
		return skillCommand{}, false
	case map[string]*skillCommand:
		if command, ok := typed[name]; ok && command != nil {
			return *command, true
		}
		for _, command := range typed {
			if command != nil && command.Name == name {
				return *command, true
			}
		}
		return skillCommand{}, false
	case map[string]any:
		if entry, ok := typed[name]; ok {
			if command, ok := skillCommandFromDynamicRecord(name, entry); ok {
				return command, true
			}
		}
		if command, ok := skillCommandFromDynamicRecord(name, typed); ok {
			return command, true
		}
		for key, entry := range typed {
			if key == name {
				continue
			}
			if command, ok := skillCommandFromDynamicRecord(key, entry); ok {
				if command.Name == name {
					return command, true
				}
			}
		}
		return skillCommand{}, false
	case []skillCommand:
		for _, command := range typed {
			if command.Name == name {
				return command, true
			}
		}
		return skillCommand{}, false
	case []any:
		for _, entry := range typed {
			if command, ok := skillCommandFromDynamicValue(name, entry); ok {
				return command, true
			}
		}
		return skillCommand{}, false
	default:
		return skillCommandFromDynamicRecord(name, typed)
	}
}

func skillCommandFromDynamicValue(name string, value any) (skillCommand, bool) {
	if command, ok := skillCommandFromDynamicRecord(name, value); ok {
		return command, true
	}
	switch typed := value.(type) {
	case map[string]any:
		if child, ok := typed[name]; ok {
			return skillCommandFromDynamicRecord(name, child)
		}
	}
	return skillCommand{}, false
}

func skillCommandFromDynamicRecord(name string, value any) (skillCommand, bool) {
	switch typed := value.(type) {
	case skillCommand:
		if typed.Name == "" || typed.Name == name {
			return typed, true
		}
		return skillCommand{}, false
	case *skillCommand:
		if typed == nil {
			return skillCommand{}, false
		}
		if typed.Name == "" || typed.Name == name {
			return *typed, true
		}
		return skillCommand{}, false
	case map[string]any:
		commandName := stringFieldAny(typed, "name")
		if commandName == "" {
			commandName = stringFieldAny(typed, "skill")
		}
		if commandName == "" {
			commandName = name
		}
		if commandName == "" {
			return skillCommand{}, false
		}
		command := skillCommand{
			Name:                   commandName,
			DisplayName:            stringFieldAny(typed, "displayName"),
			Description:            stringFieldAny(typed, "description"),
			WhenToUse:              stringFieldAny(typed, "whenToUse"),
			Version:                stringFieldAny(typed, "version"),
			UserInvocable:          true,
			ArgumentHint:           stringFieldAny(typed, "argumentHint"),
			Path:                   stringFieldAny(typed, "path"),
			Content:                stringFieldAny(typed, "content"),
			ArgumentNames:          parseSkillFrontmatterList(typed["argumentNames"], false),
			AllowedTools:           parseSkillFrontmatterList(typed["allowedTools"], false),
			Model:                  stringFieldAny(typed, "model"),
			Context:                strings.ToLower(strings.TrimSpace(stringFieldAny(typed, "context"))),
			Agent:                  stringFieldAny(typed, "agent"),
			Effort:                 stringFieldAny(typed, "effort"),
			Paths:                  parseSkillFrontmatterList(typed["paths"], true),
			Shell:                  parseSkillShellFrontmatter(typed["shell"], stringFieldAny(typed, "path")),
			DisableModelInvocation: parseBooleanFrontmatterAny(typed["disableModelInvocation"]),
			Hooks:                  parseSkillHooksFrontmatter(typed["hooks"]),
		}
		if userInvocable, ok := typed["userInvocable"].(bool); ok {
			command.UserInvocable = userInvocable
		} else if text := stringFieldAny(typed, "userInvocable"); text != "" {
			command.UserInvocable = parseBooleanFrontmatterAny(text)
		}
		if command.Description == "" {
			command.Description = extractSkillDescription(command.Content)
		}
		if command.Path == "" {
			command.Path = stringFieldAny(typed, "skillPath")
		}
		if command.Model == "inherit" {
			command.Model = ""
		}
		if command.Name == "" {
			command.Name = name
		}
		return command, true
	default:
		return skillCommand{}, false
	}
}

func defaultSkillShellExecutor(ctx context.Context, request SkillShellRequest) (string, error) {
	shellName, shellArgs := defaultSkillShellCommand(request.Shell)
	cmd := exec.CommandContext(ctx, shellName, append(shellArgs, request.Command)...)
	if request.BaseDir != "" {
		cmd.Dir = request.BaseDir
	}
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("skill shell command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func defaultSkillShellCommand(shell FrontmatterShell) (string, []string) {
	switch shell {
	case FrontmatterShellPowershell:
		if runtime.GOOS == "windows" {
			return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-Command"}
		}
		return "pwsh", []string{"-NoLogo", "-NoProfile", "-Command"}
	default:
		if runtime.GOOS == "windows" {
			return "bash", []string{"-lc"}
		}
		return "bash", []string{"-lc"}
	}
}
