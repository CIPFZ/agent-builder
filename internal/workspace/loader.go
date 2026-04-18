package workspace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Context struct {
	Root  string
	Files []File
}

type File struct {
	Name    string
	Path    string
	Type    string
	Content string
}

type Loader struct {
	root       string
	managedDir string
	userDir    string
}

type Option func(*Loader)

func WithManagedInstructionDir(path string) Option {
	return func(l *Loader) {
		l.managedDir = strings.TrimSpace(path)
	}
}

func WithUserInstructionDir(path string) Option {
	return func(l *Loader) {
		l.userDir = strings.TrimSpace(path)
	}
}

func NewLoader(root string, options ...Option) *Loader {
	loader := &Loader{root: filepath.Clean(root)}
	for _, option := range options {
		if option != nil {
			option(loader)
		}
	}
	return loader
}

func (l *Loader) Load() (Context, error) {
	ctx := Context{
		Root: filepath.Clean(strings.TrimSpace(l.root)),
	}
	if strings.TrimSpace(ctx.Root) == "" || ctx.Root == "." {
		return ctx, nil
	}

	seen := make(map[string]struct{})
	var files []File

	for _, dir := range []string{l.managedDir, l.userDir} {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		items, err := loadInstructionFile(filepath.Join(dir, "CLAUDE.md"), seen)
		if err != nil {
			return Context{}, err
		}
		files = append(files, items...)
	}

	for _, dir := range instructionDirs(ctx.Root) {
		for _, candidate := range []string{
			filepath.Join(dir, "CLAUDE.md"),
			filepath.Join(dir, ".claude", "CLAUDE.md"),
		} {
			items, err := loadInstructionFile(candidate, seen)
			if err != nil {
				return Context{}, err
			}
			files = append(files, items...)
		}
		rules, err := loadRuleFiles(filepath.Join(dir, ".claude", "rules"), seen)
		if err != nil {
			return Context{}, err
		}
		files = append(files, rules...)
		items, err := loadInstructionFile(filepath.Join(dir, "CLAUDE.local.md"), seen)
		if err != nil {
			return Context{}, err
		}
		files = append(files, items...)
	}

	for _, name := range []string{"AGENTS.md", "SOUL.md", "TOOLS.md"} {
		path := filepath.Join(ctx.Root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Context{}, err
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		files = append(files, File{
			Name:    name,
			Path:    path,
			Type:    "workspace",
			Content: content,
		})
	}

	ctx.Files = files
	return ctx, nil
}

func instructionDirs(root string) []string {
	root = filepath.Clean(root)
	if strings.TrimSpace(root) == "" {
		return nil
	}
	stop := instructionStopBoundary(root)
	stack := []string{}
	current := root
	for {
		stack = append(stack, current)
		if stop != "" && samePath(current, stop) {
			break
		}
		parent := filepath.Dir(current)
		if samePath(parent, current) {
			break
		}
		current = parent
	}
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack
}

func instructionStopBoundary(root string) string {
	current := filepath.Clean(root)
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if samePath(parent, current) {
			return current
		}
		current = parent
	}
}

func loadInstructionFile(path string, seen map[string]struct{}) ([]File, error) {
	normalized := normalizePath(path)
	if _, ok := seen[normalized]; ok {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen[normalized] = struct{}{}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	files := []File{}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if includePath, ok := parseIncludeDirective(trimmed); ok {
			included, err := loadInstructionFile(resolveIncludePath(path, includePath), seen)
			if err != nil {
				return nil, err
			}
			files = append(files, included...)
			continue
		}
		body = append(body, line)
	}
	content := strings.TrimSpace(strings.Join(body, "\n"))
	if content == "" {
		return files, nil
	}
	files = append(files, File{
		Name:    filepath.Base(path),
		Path:    path,
		Type:    "instruction",
		Content: content,
	})
	return files, nil
}

func loadRuleFiles(root string, seen map[string]struct{}) ([]File, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	paths := []string{}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(paths)

	files := []File{}
	for _, path := range paths {
		items, err := loadInstructionFile(path, seen)
		if err != nil {
			return nil, err
		}
		files = append(files, items...)
	}
	return files, nil
}

func parseIncludeDirective(line string) (string, bool) {
	if !strings.HasPrefix(line, "@") {
		return "", false
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, "@"))
	if target == "" {
		return "", false
	}
	return target, true
}

func resolveIncludePath(basePath, includePath string) string {
	includePath = strings.TrimSpace(includePath)
	switch {
	case filepath.IsAbs(includePath):
		return filepath.Clean(includePath)
	case strings.HasPrefix(includePath, "~/"):
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return filepath.Clean(includePath[2:])
		}
		return filepath.Join(home, includePath[2:])
	default:
		return filepath.Join(filepath.Dir(basePath), includePath)
	}
}

func normalizePath(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func samePath(left, right string) bool {
	return normalizePath(left) == normalizePath(right)
}
