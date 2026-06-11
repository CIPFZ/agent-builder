package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"
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
// storedPath is the expected disk path for the full content.
func Truncate(content string, maxChars int, storedPath string) (string, bool) {
	totalChars := len(content)
	if totalChars <= maxChars {
		return content, false
	}

	headSize := int(float64(maxChars) * headRatio)
	tailSize := int(float64(maxChars) * tailRatio)

	if hasTailPriority(content[totalChars-tailSize*2:]) {
		tailSize = int(float64(maxChars) * tailErrorRatio)
		if tailSize > totalChars/2 {
			tailSize = totalChars / 2
		}
		headSize = maxChars - tailSize
	}

	headSize = safeUTF8Cut(content, headSize)
	tailStart := totalChars - tailSize
	tailStart = safeUTF8Cut(content, tailStart)
	tailContent := content[tailStart:]

	headContent := content[:headSize]
	previewBody := headContent + tailContent
	hasMore := (headSize+tailSize < totalChars)

	return buildPersistedOutput(previewBody, totalChars, storedPath, len(headContent), len(tailContent), hasMore), true
}

// safeUTF8Cut adjusts a byte position backward to a valid UTF-8 boundary.
func safeUTF8Cut(s string, pos int) int {
	if pos <= 0 {
		return 0
	}
	if pos >= len(s) {
		return len(s)
	}
	for pos > 0 && !utf8.RuneStart(s[pos]) {
		pos--
	}
	return pos
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
func buildPersistedOutput(previewBody string, totalChars int, storedPath string, headSize, tailSize int, hasMore bool) string {
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
	fmt.Fprintf(&b, "Full output saved to: %s\n", storedPath)
	b.WriteString("Use the view tool with offset and limit to access specific sections of this output.\n\n")
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
