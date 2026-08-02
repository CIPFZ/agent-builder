package agent

import (
	"fmt"
	"strings"
)

const (
	headRatio                 = 0.7
	tailRatio                 = 0.3
	tailErrorRatio            = 0.5
	persistedOutputTag        = "<persisted-output>"
	persistedOutputClosingTag = "</persisted-output>"
)

var errorKeywords = []string{
	"Error:", "error:", "fatal:", "Fatal:", "panic:", "PANIC:",
	"Traceback", "traceback", "Exception:", "exception:",
	"FAIL:", "FAILED:", "signal:", "SIGSEGV", "SIGABRT",
}

// Truncate applies head+tail truncation to content. If content is within
// maxChars, returns (content, false). Otherwise returns a <persisted-output>
// XML block with head+tail preview and (result, true).
// storedRef is the authoritative Runtime Object URI for the full content.
func Truncate(content string, maxChars int, storedRef string) (string, bool) {
	runes := []rune(content)
	totalChars := len(runes)
	if totalChars <= maxChars {
		return content, false
	}

	headSize := int(float64(maxChars) * headRatio)
	tailSize := int(float64(maxChars) * tailRatio)

	tailCheckStart := totalChars - tailSize*2
	if tailCheckStart < 0 {
		tailCheckStart = 0
	}
	if hasTailPriority(string(runes[tailCheckStart:])) {
		tailSize = int(float64(maxChars) * tailErrorRatio)
		if tailSize > totalChars/2 {
			tailSize = totalChars / 2
		}
		headSize = maxChars - tailSize
	}

	tailStart := totalChars - tailSize
	tailContent := string(runes[tailStart:])

	headContent := string(runes[:headSize])
	previewBody := headContent + tailContent
	hasMore := (headSize+tailSize < totalChars)

	return buildPersistedOutput(previewBody, totalChars, storedRef, len(headContent), len(tailContent), hasMore), true
}

// hasTailPriority checks if the tail portion contains error keywords
// or appears to end with valid JSON.
func hasTailPriority(tail string) bool {
	for _, kw := range errorKeywords {
		if strings.Contains(tail, kw) {
			return true
		}
	}
	trimmed := strings.TrimSpace(tail)
	return (strings.HasSuffix(trimmed, "}") || strings.HasSuffix(trimmed, "]"))
}

// buildPersistedOutput constructs the <persisted-output> XML block.
func buildPersistedOutput(previewBody string, totalChars int, storedRef string, headSize, tailSize int, hasMore bool) string {
	sizeKB := float64(totalChars) / 1024
	var sizeStr string
	if sizeKB >= 1024 {
		sizeStr = fmt.Sprintf("%.1f MB", sizeKB/1024)
	} else {
		sizeStr = fmt.Sprintf("%.1f KB", sizeKB)
	}

	var b strings.Builder
	b.WriteString(persistedOutputTag + "\n")
	fmt.Fprintf(&b, "This tool result was too large (%d characters, %s).\n", totalChars, sizeStr)
	fmt.Fprintf(&b, "Full output retained as Runtime object: %s\n", storedRef)
	b.WriteString("The full content remains available through the tool output reference.\n\n")
	fmt.Fprintf(&b, "Preview (head %d + tail %d chars):\n", headSize, tailSize)
	b.WriteString(previewBody)
	if hasMore {
		b.WriteString("\n...\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString(persistedOutputClosingTag)
	return b.String()
}
