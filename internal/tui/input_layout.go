package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type inputVisualLine struct {
	Start int
	End   int
	Text  string
	Width int
}

func buildInputVisualLines(input string, width int) []inputVisualLine {
	if width < 1 {
		width = 1
	}
	runes := []rune(input)
	lines := make([]inputVisualLine, 0, 4)
	var text strings.Builder
	start := 0
	lineWidth := 0

	appendLine := func(end int) {
		lines = append(lines, inputVisualLine{
			Start: start,
			End:   end,
			Text:  text.String(),
			Width: lineWidth,
		})
		text.Reset()
		lineWidth = 0
		start = end
	}

	if len(runes) == 0 {
		return []inputVisualLine{{Start: 0, End: 0, Text: "", Width: 0}}
	}

	for i, r := range runes {
		if r == '\n' {
			appendLine(i)
			start = i + 1
			continue
		}
		cellWidth := lipgloss.Width(string(r))
		if lineWidth+cellWidth > width && text.Len() > 0 {
			appendLine(i)
		}
		text.WriteRune(r)
		lineWidth += cellWidth
	}
	appendLine(len(runes))
	if len(runes) > 0 && runes[len(runes)-1] == '\n' {
		lines = append(lines, inputVisualLine{
			Start: len(runes),
			End:   len(runes),
			Text:  "",
			Width: 0,
		})
	}
	return lines
}

func inputCursorLineIndex(lines []inputVisualLine, cursor int) int {
	if len(lines) == 0 {
		return 0
	}
	index := 0
	for i := range lines {
		if lines[i].Start <= cursor {
			index = i
			continue
		}
		break
	}
	return index
}

func inputCursorColumn(line inputVisualLine, input string, cursor int) int {
	runes := []rune(input)
	if cursor < line.Start {
		return 0
	}
	if cursor > line.End {
		cursor = line.End
	}
	width := 0
	for _, r := range runes[line.Start:cursor] {
		width += lipgloss.Width(string(r))
	}
	return width
}

func inputCursorPositionForColumn(line inputVisualLine, input string, column int) int {
	if column <= 0 {
		return line.Start
	}
	runes := []rune(input)
	width := 0
	position := line.Start
	for i, r := range runes[line.Start:line.End] {
		cellWidth := lipgloss.Width(string(r))
		if width+cellWidth > column {
			break
		}
		width += cellWidth
		position = line.Start + i + 1
	}
	return position
}
