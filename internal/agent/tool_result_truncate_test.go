package agent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestTruncate_UnderThreshold_ReturnsOriginal(t *testing.T) {
	content := "short output"
	result, persisted := Truncate(content, 16000, "/tmp/test.txt")
	require.False(t, persisted)
	require.Equal(t, content, result)
}

func TestTruncate_OverThreshold_ReturnsPersistedOutput(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("line of output that is reasonably long\n")
	}
	content := sb.String()
	maxChars := 100

	result, persisted := Truncate(content, maxChars, "runtime://objects/test_id")
	require.True(t, persisted)
	require.Contains(t, result, "<persisted-output>")
	require.Contains(t, result, "</persisted-output>")
	require.Contains(t, result, "runtime://objects/test_id")
	require.Less(t, len(result), len(content))
}

func TestTruncate_ErrorInTail_IncreasesTailSize(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.WriteString("normal output line\n")
	}
	sb.WriteString("Error: something went wrong\n")
	sb.WriteString("fatal: process terminated\n")
	content := sb.String()
	maxChars := 1000

	result, persisted := Truncate(content, maxChars, "runtime://objects/test")
	require.True(t, persisted)
	require.Contains(t, result, "Error: something went wrong")
	require.Contains(t, result, "fatal: process terminated")
}

func TestTruncate_JSONInTail_IncreasesTailSize(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.WriteString("normal output line\n")
	}
	sb.WriteString(`{"key": "value", "nested": {"a": 1}}`)
	content := sb.String()
	maxChars := 1000

	result, persisted := Truncate(content, maxChars, "runtime://objects/test")
	require.True(t, persisted)
	require.Contains(t, result, `{"key": "value"`)
}

func TestTruncate_UTF8Boundary_TruncatesSafely(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("hello world line\n")
	}
	sb.WriteString("café café café café ")
	content := sb.String()
	maxChars := len(content) - 3

	result, persisted := Truncate(content, maxChars, "runtime://objects/test")
	require.True(t, persisted)
	require.True(t, utf8.ValidString(result))
}

func TestTruncate_EmptyContent(t *testing.T) {
	result, persisted := Truncate("", 100, "runtime://objects/test")
	require.False(t, persisted)
	require.Equal(t, "", result)
}
