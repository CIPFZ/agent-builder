package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTopicPathRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"",
		"../escape.md",
		"/absolute.md",
		`C:\absolute.md`,
		"user//empty.md",
		"user/nul\x00.md",
		"user/not-text.txt",
		"MEMORY.md",
	}
	for _, tc := range cases {
		if _, _, err := ResolveTopicPath(root, tc); err == nil {
			t.Fatalf("ResolveTopicPath(%q) succeeded", tc)
		}
	}
}

func TestResolveTopicPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "user")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := ResolveTopicPath(root, "user/escape.md"); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
}
