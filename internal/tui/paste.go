package tui

import (
	"fmt"
	"regexp"
	"strings"
)

const pasteThreshold = 800

var (
	ansiEscapePattern      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	pastedTextRefPattern   = regexp.MustCompile(`\[Pasted text #([0-9]+)(?: \+[0-9]+ lines)?\]`)
	bracketedPasteSequence = regexp.MustCompile(`\x1b\[200~([\s\S]*)\x1b\[201~`)
)

type pasteContent struct {
	ID      int
	Type    string
	Content string
}

type pasteState struct {
	nextID   int
	contents map[int]pasteContent
}

func newPasteState() pasteState {
	return pasteState{
		nextID:   1,
		contents: make(map[int]pasteContent),
	}
}

func (s *pasteState) addText(content string, numLines int) string {
	if s.contents == nil {
		*s = newPasteState()
	}
	id := s.nextID
	s.nextID++
	s.contents[id] = pasteContent{ID: id, Type: "text", Content: content}
	return formatPastedTextRef(id, numLines)
}

func (s *pasteState) expandReferences(input string) string {
	return pastedTextRefPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := pastedTextRefPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		var id int
		if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
			return match
		}
		content, ok := s.contents[id]
		if !ok || content.Type != "text" {
			return match
		}
		return content.Content
	})
}

func formatPastedTextRef(id int, numLines int) string {
	if numLines == 0 {
		return fmt.Sprintf("[Pasted text #%d]", id)
	}
	return fmt.Sprintf("[Pasted text #%d +%d lines]", id, numLines)
}

func sanitizePastedText(text string) string {
	text = extractBracketedPaste(text)
	text = ansiEscapePattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	return text
}

func extractBracketedPaste(text string) string {
	match := bracketedPasteSequence.FindStringSubmatch(text)
	if len(match) == 2 {
		return match[1]
	}
	return text
}

func pastedTextRefNumLines(text string) int {
	return strings.Count(text, "\n") + strings.Count(text, "\r")
}
