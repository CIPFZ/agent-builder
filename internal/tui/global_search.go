package tui

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const dialogKindGlobalSearch = "global-search"

const (
	globalSearchMatchPrefix   = "search:"
	globalSearchPreviewLines  = 9
	globalSearchContextRadius = 1
	globalSearchMaxPerFile    = 10
	globalSearchMaxTotal      = 500
)

func (m *Model) openGlobalSearchDialog() {
	m.globalSearch.OriginalInput = m.input
	m.globalSearch.OriginalCursor = m.cursorPos
	m.globalSearch.WorkspaceRoots = m.globalSearchWorkspaceRoots()
	m.globalSearch.Matches = nil
	m.globalSearch.Searching = false
	m.globalSearch.Truncated = false
	m.globalSearch.PreviewTitle = ""
	m.globalSearch.PreviewContent = ""
	m.dialog.open(dialogSpec{
		Kind:           dialogKindGlobalSearch,
		Title:          "Global Search",
		Subtitle:       "Type workspace text to search",
		QueryEnabled:   true,
		Items:          m.globalSearchDialogItems(),
		EmptyText:      "Type to search workspace",
		FooterHint:     "Type to filter | Enter open | Tab mention | Shift+Tab insert path | Esc close",
		VisibleCount:   12,
		OriginalInput:  m.input,
		OriginalCursor: m.cursorPos,
	})
	m.clearSuggestions()
}

func (m *Model) globalSearchWorkspaceRoots() []string {
	if bridge, ok := m.bridge.(platformStatusBridge); ok {
		roots := bridge.PlatformStatusSnapshot().WorkspaceRoots
		if len(roots) > 0 {
			return append([]string(nil), roots...)
		}
	}
	return append([]string(nil), m.globalSearch.WorkspaceRoots...)
}

func (m *Model) triggerGlobalSearch() tea.Cmd {
	if m.dialog.Kind != dialogKindGlobalSearch {
		return nil
	}
	query := strings.TrimSpace(m.dialog.Picker.Query)
	m.globalSearch.Generation++
	generation := m.globalSearch.Generation
	m.globalSearch.PreviewTitle = ""
	m.globalSearch.PreviewContent = ""
	if query == "" {
		m.globalSearch.Matches = nil
		m.globalSearch.Searching = false
		m.globalSearch.Truncated = false
		m.updateGlobalSearchDialog(false)
		return nil
	}
	if m.globalSearch.Search == nil {
		m.applyBridgeError(errors.New("workspace search is not configured"))
		return nil
	}
	m.globalSearch.Searching = true
	m.updateGlobalSearchDialog(false)
	roots := append([]string(nil), m.globalSearch.WorkspaceRoots...)
	return func() tea.Msg {
		result, err := m.globalSearch.Search(workspaceSearchRequest{
			Query: query,
			Roots: roots,
		})
		return globalSearchResultsMsg{
			Generation: generation,
			Result:     result,
			Err:        err,
		}
	}
}

func (m *Model) applyGlobalSearchResults(msg globalSearchResultsMsg) {
	if msg.Generation != m.globalSearch.Generation {
		return
	}
	m.globalSearch.Searching = false
	if msg.Err != nil {
		m.applyBridgeError(msg.Err)
		m.globalSearch.Matches = nil
		m.globalSearch.Truncated = false
		m.updateGlobalSearchDialog(false)
		return
	}
	m.globalSearch.Matches = append([]workspaceSearchMatch(nil), msg.Result.Matches...)
	m.globalSearch.Truncated = msg.Result.Truncated
	m.updateGlobalSearchDialog(false)
}

func (m *Model) updateGlobalSearchDialog(preserveSelection bool) {
	if m.dialog.Kind != dialogKindGlobalSearch {
		return
	}
	currentValue := m.dialog.Picker.Current().Value
	items := m.globalSearchDialogItems()
	m.dialog.Items = append([]dialogItem(nil), items...)
	m.dialog.Picker.Items = append([]dialogItem(nil), items...)
	if !preserveSelection || !selectDialogItemValue(&m.dialog.Picker, currentValue) {
		m.dialog.Picker.resetFocus()
	}
	m.updateGlobalSearchPreview()
	currentValue = m.dialog.Picker.Current().Value
	items = m.globalSearchDialogItems()
	m.dialog.Items = append([]dialogItem(nil), items...)
	m.dialog.Picker.Items = append([]dialogItem(nil), items...)
	_ = selectDialogItemValue(&m.dialog.Picker, currentValue)
	m.dialog.syncPickerSelection()
}

func (m *Model) globalSearchDialogItems() []dialogItem {
	query := strings.TrimSpace(m.dialog.Picker.Query)
	if query == "" {
		if m.globalSearch.Searching {
			return []dialogItem{{Label: "Searching...", Disabled: true}}
		}
		return nil
	}
	items := make([]dialogItem, 0, len(m.globalSearch.Matches)+globalSearchPreviewLines+2)
	for _, match := range m.globalSearch.Matches {
		items = append(items, dialogItem{
			Label:       fmt.Sprintf("%s:%d", match.DisplayPath, match.Line),
			Description: strings.TrimSpace(match.Text),
			Value:       globalSearchMatchValue(match),
		})
	}
	if len(items) == 0 && m.globalSearch.Searching {
		items = append(items, dialogItem{Label: "Searching...", Disabled: true})
	}
	previewLines := globalSearchPreviewLinesForState(m.globalSearch.PreviewContent)
	if len(previewLines) > 0 {
		items = append(items, dialogItem{Label: "Preview", Description: m.globalSearch.PreviewTitle, Value: query, Disabled: true})
		for _, line := range previewLines {
			items = append(items, dialogItem{Label: line, Value: query, Disabled: true})
		}
	}
	return items
}

func globalSearchPreviewLinesForState(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func globalSearchMatchValue(match workspaceSearchMatch) string {
	return globalSearchMatchPrefix + match.AbsolutePath + "\x00" + strconv.Itoa(match.Line)
}

func parseGlobalSearchMatchValue(value string) (string, int, bool) {
	if !strings.HasPrefix(value, globalSearchMatchPrefix) {
		return "", 0, false
	}
	body := strings.TrimPrefix(value, globalSearchMatchPrefix)
	path, lineText, ok := strings.Cut(body, "\x00")
	if !ok {
		return "", 0, false
	}
	line, err := strconv.Atoi(lineText)
	if err != nil || line <= 0 {
		return "", 0, false
	}
	return path, line, true
}

func (m *Model) updateGlobalSearchPreview() {
	m.globalSearch.PreviewTitle = ""
	m.globalSearch.PreviewContent = ""
	path, line, ok := parseGlobalSearchMatchValue(m.dialog.Picker.Current().Value)
	if !ok {
		return
	}
	lines, err := readPreviewAroundLine(path, line, globalSearchContextRadius)
	if err != nil {
		m.globalSearch.PreviewTitle = fmt.Sprintf("%s:%d", filepath.Base(path), line)
		m.globalSearch.PreviewContent = "(preview unavailable)"
		return
	}
	rel := path
	for _, root := range m.globalSearch.WorkspaceRoots {
		if display, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(display, "..") {
			rel = filepath.ToSlash(display)
			break
		}
	}
	m.globalSearch.PreviewTitle = fmt.Sprintf("%s:%d", rel, line)
	m.globalSearch.PreviewContent = strings.Join(lines, "\n")
}

func readPreviewAroundLine(path string, line, context int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	start := line - context
	if start < 1 {
		start = 1
	}
	end := line + context
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024), 64*1024)
	lines := make([]string, 0, end-start+1)
	current := 0
	for scanner.Scan() {
		current++
		if current < start {
			continue
		}
		if current > end {
			break
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func (m *Model) acceptGlobalSearchItem(item dialogItem, action string) {
	path, line, ok := parseGlobalSearchMatchValue(item.Value)
	if !ok {
		return
	}
	switch action {
	case "tab":
		pathLabel := item.Label
		if cut := strings.LastIndex(pathLabel, ":"); cut >= 0 {
			pathLabel = pathLabel[:cut]
		}
		m.restoreDialogInput(m.globalSearch.OriginalInput + fmt.Sprintf("@%s#L%d ", pathLabel, line))
	case "shift+tab":
		pathLabel := item.Label
		if cut := strings.LastIndex(pathLabel, ":"); cut >= 0 {
			pathLabel = pathLabel[:cut]
		}
		m.restoreDialogInput(m.globalSearch.OriginalInput + fmt.Sprintf("%s:%d ", pathLabel, line))
	default:
		if m.globalSearch.OpenFileAtLine == nil {
			m.applyBridgeError(errors.New("global search opener is not configured"))
			return
		}
		if err := m.globalSearch.OpenFileAtLine(path, line); err != nil {
			m.applyBridgeError(err)
		}
	}
}

func defaultWorkspaceSearcher(req workspaceSearchRequest) (workspaceSearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return workspaceSearchResult{}, nil
	}
	if rgPath, err := exec.LookPath("rg"); err == nil && rgPath != "" {
		return ripgrepWorkspaceSearch(rgPath, req)
	}
	return fallbackWorkspaceSearch(req)
}

func ripgrepWorkspaceSearch(rgPath string, req workspaceSearchRequest) (workspaceSearchResult, error) {
	result := workspaceSearchResult{}
	for _, root := range req.Roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		cmd := exec.Command(rgPath, "-n", "--no-heading", "-i", "-m", strconv.Itoa(globalSearchMaxPerFile), "-F", "-e", req.Query, ".")
		cmd.Dir = root
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		exitErr := &exec.ExitError{}
		if err != nil && !errors.As(err, &exitErr) {
			return workspaceSearchResult{}, fmt.Errorf("ripgrep failed: %w", err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
		for scanner.Scan() {
			match, ok := parseRipgrepLine(scanner.Text())
			if !ok {
				continue
			}
			abs := filepath.Join(root, filepath.FromSlash(match.DisplayPath))
			match.AbsolutePath = abs
			result.Matches = append(result.Matches, match)
			if len(result.Matches) >= globalSearchMaxTotal {
				result.Truncated = true
				return result, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return workspaceSearchResult{}, err
		}
	}
	sortWorkspaceMatches(result.Matches)
	return result, nil
}

func fallbackWorkspaceSearch(req workspaceSearchRequest) (workspaceSearchResult, error) {
	lowerQuery := strings.ToLower(req.Query)
	result := workspaceSearchResult{}
	for _, root := range req.Roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		perFile := map[string]int{}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() {
				if shouldSkipQuickOpenDir(name) && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || shouldSkipQuickOpenFile(name) {
				return nil
			}
			file, openErr := os.Open(path)
			if openErr != nil {
				return nil
			}
			defer file.Close()
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			display := filepath.ToSlash(rel)
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 0, 1024), 64*1024)
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				text := scanner.Text()
				if strings.ContainsRune(text, '\x00') {
					return nil
				}
				if !strings.Contains(strings.ToLower(text), lowerQuery) {
					continue
				}
				if perFile[display] >= globalSearchMaxPerFile {
					continue
				}
				perFile[display]++
				result.Matches = append(result.Matches, workspaceSearchMatch{
					DisplayPath:  display,
					AbsolutePath: path,
					Line:         lineNo,
					Text:         text,
				})
				if len(result.Matches) >= globalSearchMaxTotal {
					result.Truncated = true
					return errors.New("truncate")
				}
			}
			return nil
		})
		if result.Truncated {
			break
		}
	}
	sortWorkspaceMatches(result.Matches)
	return result, nil
}

func sortWorkspaceMatches(matches []workspaceSearchMatch) {
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].DisplayPath == matches[j].DisplayPath {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].DisplayPath < matches[j].DisplayPath
	})
}

func parseRipgrepLine(line string) (workspaceSearchMatch, bool) {
	left := strings.Index(line, ":")
	if left < 0 {
		return workspaceSearchMatch{}, false
	}
	right := strings.Index(line[left+1:], ":")
	if right < 0 {
		return workspaceSearchMatch{}, false
	}
	right += left + 1
	displayPath := filepath.ToSlash(line[:left])
	lineNo, err := strconv.Atoi(line[left+1 : right])
	if err != nil || lineNo <= 0 {
		return workspaceSearchMatch{}, false
	}
	return workspaceSearchMatch{
		DisplayPath: displayPath,
		Line:        lineNo,
		Text:        line[right+1:],
	}, true
}
