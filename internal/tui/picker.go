package tui

import "strings"

const defaultPickerVisibleCount = 5

func newListPickerState(spec listPickerSpec) listPickerState {
	visibleCount := spec.VisibleCount
	if visibleCount <= 0 {
		visibleCount = defaultPickerVisibleCount
	}
	state := listPickerState{
		Items:         append([]dialogItem(nil), spec.Items...),
		QueryEnabled:  spec.QueryEnabled,
		VisibleCount:  visibleCount,
		SelectedIndex: -1,
	}
	state.resetFocus()
	return state
}

func (p *listPickerState) setQuery(query string) {
	p.Query = query
	p.resetFocus()
}

func (p *listPickerState) insertQuery(text string) {
	if !p.QueryEnabled {
		return
	}
	p.setQuery(p.Query + text)
}

func (p *listPickerState) backspaceQuery() {
	if !p.QueryEnabled || p.Query == "" {
		return
	}
	runes := []rune(p.Query)
	p.setQuery(string(runes[:len(runes)-1]))
}

func (p *listPickerState) resetFocus() {
	if p.MatchCount() == 0 {
		p.SelectedIndex = -1
		p.VisibleFromIndex = 0
		return
	}
	p.SelectedIndex = 0
	p.VisibleFromIndex = 0
}

func (p listPickerState) filteredItems() []dialogItem {
	if strings.TrimSpace(p.Query) == "" {
		return append([]dialogItem(nil), p.Items...)
	}
	query := strings.ToLower(strings.TrimSpace(p.Query))
	matches := make([]dialogItem, 0, len(p.Items))
	for _, item := range p.Items {
		haystack := strings.ToLower(item.Label + " " + item.Description + " " + item.Value)
		if strings.Contains(haystack, query) {
			matches = append(matches, item)
		}
	}
	return matches
}

func (p listPickerState) MatchCount() int {
	return len(p.filteredItems())
}

func (p listPickerState) Current() dialogItem {
	items := p.filteredItems()
	if p.SelectedIndex < 0 || p.SelectedIndex >= len(items) {
		return dialogItem{}
	}
	return items[p.SelectedIndex]
}

func (p listPickerState) VisibleItems() []dialogItem {
	items := p.filteredItems()
	if len(items) == 0 {
		return nil
	}
	visibleCount := p.VisibleCount
	if visibleCount <= 0 {
		visibleCount = defaultPickerVisibleCount
	}
	from := p.VisibleFromIndex
	if from < 0 {
		from = 0
	}
	if from > len(items) {
		from = len(items)
	}
	to := from + visibleCount
	if to > len(items) {
		to = len(items)
	}
	return append([]dialogItem(nil), items[from:to]...)
}

func (p *listPickerState) moveDown() {
	count := p.MatchCount()
	if count == 0 {
		p.SelectedIndex = -1
		p.VisibleFromIndex = 0
		return
	}
	if p.SelectedIndex < count-1 {
		p.SelectedIndex++
	}
	p.ensureVisible()
}

func (p *listPickerState) moveUp() {
	count := p.MatchCount()
	if count == 0 {
		p.SelectedIndex = -1
		p.VisibleFromIndex = 0
		return
	}
	if p.SelectedIndex > 0 {
		p.SelectedIndex--
	}
	p.ensureVisible()
}

func (p *listPickerState) pageDown() {
	count := p.MatchCount()
	if count == 0 {
		return
	}
	step := p.VisibleCount
	if step <= 0 {
		step = defaultPickerVisibleCount
	}
	p.SelectedIndex = minInt(count-1, p.SelectedIndex+step)
	p.ensureVisible()
}

func (p *listPickerState) pageUp() {
	if p.MatchCount() == 0 {
		return
	}
	step := p.VisibleCount
	if step <= 0 {
		step = defaultPickerVisibleCount
	}
	p.SelectedIndex = maxInt(0, p.SelectedIndex-step)
	p.ensureVisible()
}

func (p *listPickerState) ensureVisible() {
	count := p.MatchCount()
	if count == 0 {
		p.SelectedIndex = -1
		p.VisibleFromIndex = 0
		return
	}
	if p.SelectedIndex < 0 {
		p.SelectedIndex = 0
	}
	if p.SelectedIndex >= count {
		p.SelectedIndex = count - 1
	}
	visibleCount := p.VisibleCount
	if visibleCount <= 0 {
		visibleCount = defaultPickerVisibleCount
	}
	if p.SelectedIndex < p.VisibleFromIndex {
		p.VisibleFromIndex = p.SelectedIndex
	}
	if p.SelectedIndex >= p.VisibleFromIndex+visibleCount {
		p.VisibleFromIndex = p.SelectedIndex - visibleCount + 1
	}
	maxFrom := maxInt(0, count-visibleCount)
	if p.VisibleFromIndex > maxFrom {
		p.VisibleFromIndex = maxFrom
	}
}

func (p listPickerState) accept() (dialogItem, bool) {
	item := p.Current()
	if item.Label == "" || item.Disabled {
		return dialogItem{}, false
	}
	return item, true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
