package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"myclaw/internal/session"
)

type fileTool struct {
	name        string
	description string
	schema      map[string]any
	readOnly    bool
	handler     func(context.Context, session.Session, map[string]any) (string, error)
}

func NewReadTool() Tool {
	return &fileTool{
		name:        "Read",
		description: "Reads a file from the local filesystem.",
		readOnly:    true,
		schema: objectSchema(map[string]any{
			"file_path": map[string]any{"type": "string"},
			"offset":    map[string]any{"type": "number"},
			"limit":     map[string]any{"type": "number"},
		}, []string{"file_path"}),
		handler: readFileTool,
	}
}

func NewWriteTool() Tool {
	return &fileTool{
		name:        "Write",
		description: "Writes a file to the local filesystem.",
		schema: objectSchema(map[string]any{
			"file_path": map[string]any{"type": "string"},
			"content":   map[string]any{"type": "string"},
		}, []string{"file_path", "content"}),
		handler: writeFileTool,
	}
}

func NewEditTool() Tool {
	return &fileTool{
		name:        "Edit",
		description: "Performs exact string replacements in files.",
		schema: objectSchema(map[string]any{
			"file_path":   map[string]any{"type": "string"},
			"old_string":  map[string]any{"type": "string"},
			"new_string":  map[string]any{"type": "string"},
			"replace_all": map[string]any{"type": "boolean"},
		}, []string{"file_path", "old_string", "new_string"}),
		handler: editFileTool,
	}
}

func NewMultiEditTool() Tool {
	return &fileTool{
		name:        "MultiEdit",
		description: "Performs multiple exact string replacements in one file.",
		schema: objectSchema(map[string]any{
			"file_path": map[string]any{"type": "string"},
			"edits": map[string]any{
				"type": "array",
				"items": objectSchema(map[string]any{
					"old_string":  map[string]any{"type": "string"},
					"new_string":  map[string]any{"type": "string"},
					"replace_all": map[string]any{"type": "boolean"},
				}, []string{"old_string", "new_string"}),
			},
		}, []string{"file_path", "edits"}),
		handler: multiEditFileTool,
	}
}

func NewGlobTool() Tool {
	return &fileTool{
		name:        "Glob",
		description: "Finds files by glob pattern.",
		readOnly:    true,
		schema: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string"},
			"path":    map[string]any{"type": "string"},
		}, []string{"pattern"}),
		handler: globTool,
	}
}

func NewGrepTool() Tool {
	return &fileTool{
		name:        "Grep",
		description: "Searches file contents for a pattern.",
		readOnly:    true,
		schema: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string"},
			"path":    map[string]any{"type": "string"},
			"glob":    map[string]any{"type": "string"},
		}, []string{"pattern"}),
		handler: grepTool,
	}
}

func NewLSTool() Tool {
	return &fileTool{
		name:        "LS",
		description: "Lists files and directories.",
		readOnly:    true,
		schema: objectSchema(map[string]any{
			"path": map[string]any{"type": "string"},
		}, []string{"path"}),
		handler: lsTool,
	}
}

func NewTodoWriteTool() Tool {
	return &fileTool{
		name:        "TodoWrite",
		description: "Creates and updates the current task todo list.",
		schema: objectSchema(map[string]any{
			"todos": map[string]any{"type": "array"},
		}, []string{"todos"}),
		handler: todoWriteTool,
	}
}

func (t *fileTool) Definition() Definition {
	return Definition{Name: t.name, Description: t.description, InputSchema: t.schema, Enabled: true, ReadOnly: t.readOnly, Destructive: !t.readOnly}
}

func (t *fileTool) Invoke(ctx context.Context, sess session.Session, input string) (string, error) {
	object, err := objectInput(input)
	if err != nil {
		return "", err
	}
	return t.handler(ctx, sess, object)
}

func (t *fileTool) InvokeWithInput(ctx context.Context, sess session.Session, input map[string]any) (string, error) {
	return t.handler(ctx, sess, cloneAnyMap(input))
}

func (t *fileTool) IsEnabled() bool           { return true }
func (t *fileTool) IsReadOnly(string) bool    { return t.readOnly }
func (t *fileTool) IsDestructive(string) bool { return !t.readOnly }
func (t *fileTool) ShouldDefer() bool         { return false }
func (t *fileTool) AlwaysLoad() bool          { return false }
func (t *fileTool) PromptDescription() string { return t.description }
func (t *fileTool) SearchHint() string        { return strings.ToLower(t.name + " " + t.description) }

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func objectInput(input string) (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &object); err != nil {
		return nil, fmt.Errorf("expected JSON object input: %w", err)
	}
	return object, nil
}

func stringField(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func resolveSessionRelativePath(sess session.Session, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	base := strings.TrimSpace(sess.Metadata.AgentWorktreePath)
	if base == "" {
		return path
	}
	return filepath.Join(base, path)
}

func intField(input map[string]any, key string, fallback int) int {
	switch value := input[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func readFileTool(_ context.Context, sess session.Session, input map[string]any) (string, error) {
	path := resolveSessionRelativePath(sess, stringField(input, "file_path"))
	if path == "" {
		return "", fmt.Errorf("Read requires file_path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Ext(path), ".ipynb") {
		return formatNotebookCells(data)
	}
	lines := strings.Split(string(data), "\n")
	offset := intField(input, "offset", 1)
	limit := intField(input, "limit", 0)
	if offset < 1 {
		offset = 1
	}
	start := offset - 1
	if start >= len(lines) {
		return "", nil
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return strings.Join(lines[start:end], "\n"), nil
}

func formatNotebookCells(data []byte) (string, error) {
	var notebook map[string]any
	if err := json.Unmarshal(data, &notebook); err != nil {
		return "", err
	}
	cells, ok := notebook["cells"].([]any)
	if !ok {
		return "", fmt.Errorf("notebook is missing cells")
	}
	var out []string
	for idx, item := range cells {
		cell, _ := item.(map[string]any)
		if cell == nil {
			continue
		}
		id := stringField(cell, "id")
		if id == "" {
			id = fmt.Sprintf("cell-%d", idx+1)
		}
		cellType := stringField(cell, "cell_type")
		if cellType == "" {
			cellType = "code"
		}
		source := notebookCellSource(cell["source"])
		header := fmt.Sprintf(`<cell id="%s" cellType="%s"`, id, cellType)
		if execution, ok := cell["execution_count"]; ok && execution != nil {
			header += fmt.Sprintf(` execution_count="%v"`, execution)
		}
		header += ">"
		out = append(out, header+"\n"+source+"\n</cell>")
	}
	return strings.Join(out, "\n\n"), nil
}

func notebookCellSource(value any) string {
	switch source := value.(type) {
	case string:
		return source
	case []any:
		var parts []string
		for _, item := range source {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	case []string:
		return strings.Join(source, "")
	default:
		return ""
	}
}

func writeFileTool(_ context.Context, sess session.Session, input map[string]any) (string, error) {
	path := resolveSessionRelativePath(sess, stringField(input, "file_path"))
	content, _ := input["content"].(string)
	if path == "" {
		return "", fmt.Errorf("Write requires file_path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return "File written: " + path, nil
}

func editFileTool(_ context.Context, sess session.Session, input map[string]any) (string, error) {
	return applyEdits(sess, input)
}

func multiEditFileTool(_ context.Context, sess session.Session, input map[string]any) (string, error) {
	path := resolveSessionRelativePath(sess, stringField(input, "file_path"))
	edits, _ := input["edits"].([]any)
	if path == "" || len(edits) == 0 {
		return "", fmt.Errorf("MultiEdit requires file_path and edits")
	}
	count := 0
	for _, item := range edits {
		edit, _ := item.(map[string]any)
		if edit == nil {
			continue
		}
		edit["file_path"] = path
		if _, err := applyEdits(sess, edit); err != nil {
			return "", err
		}
		count++
	}
	return fmt.Sprintf("Applied %d edits to %s", count, path), nil
}

func applyEdits(sess session.Session, input map[string]any) (string, error) {
	path := resolveSessionRelativePath(sess, stringField(input, "file_path"))
	oldText, _ := input["old_string"].(string)
	newText, _ := input["new_string"].(string)
	replaceAll, _ := input["replace_all"].(bool)
	if path == "" || oldText == "" {
		return "", fmt.Errorf("Edit requires file_path and old_string")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, oldText)
	if count == 0 {
		return "", fmt.Errorf("old_string not found")
	}
	if !replaceAll && count != 1 {
		return "", fmt.Errorf("old_string occurs %d times; set replace_all to replace every occurrence", count)
	}
	limit := 1
	if replaceAll {
		limit = -1
	}
	updated := strings.Replace(content, oldText, newText, limit)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Applied edit to %s", path), nil
}

func globTool(_ context.Context, sess session.Session, input map[string]any) (string, error) {
	pattern := stringField(input, "pattern")
	root := resolveSessionRelativePath(sess, stringField(input, "path"))
	if pattern == "" {
		return "", fmt.Errorf("Glob requires pattern")
	}
	if root != "" && !filepath.IsAbs(pattern) {
		pattern = filepath.Join(root, pattern)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	return strings.Join(matches, "\n"), nil
}

func grepTool(ctx context.Context, sess session.Session, input map[string]any) (string, error) {
	pattern := stringField(input, "pattern")
	root := resolveSessionRelativePath(sess, stringField(input, "path"))
	if root == "" {
		root = "."
	}
	if pattern == "" {
		return "", fmt.Errorf("Grep requires pattern")
	}
	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), pattern) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	return strings.Join(matches, "\n"), nil
}

func lsTool(_ context.Context, sess session.Session, input map[string]any) (string, error) {
	path := resolveSessionRelativePath(sess, stringField(input, "path"))
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += string(os.PathSeparator)
		}
		lines = append(lines, name)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

func todoWriteTool(_ context.Context, _ session.Session, input map[string]any) (string, error) {
	todos, ok := input["todos"].([]any)
	if !ok {
		return "", fmt.Errorf("TodoWrite requires todos")
	}
	return fmt.Sprintf("Todos updated: %d items", len(todos)), nil
}
