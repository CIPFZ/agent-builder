package memory

import (
	"strings"
	"testing"
)

func TestParseRenderMarkdownFrontmatter(t *testing.T) {
	raw := []byte(`---
id: mem_test
title: Testing policy
type: feedback
description: Prefer real database tests
tags:
  - Testing
  - database
created_at: 2026-06-29T00:00:00Z
updated_at: 2026-06-29T00:00:00Z
confidence: 0.8
---

Use the real database for integration tests.
`)
	doc, err := ParseMarkdown(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Frontmatter.Type != TypeFeedback || doc.Frontmatter.Tags[0] != "testing" {
		t.Fatalf("frontmatter not normalized: %#v", doc.Frontmatter)
	}
	rendered, err := RenderMarkdown(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "Use the real database") {
		t.Fatalf("rendered markdown lost body: %s", rendered)
	}
}

func TestParseMarkdownRejectsInvalidFrontmatter(t *testing.T) {
	_, err := ParseMarkdown([]byte("---\ntitle: Missing id\n---\nbody\n"))
	if err == nil {
		t.Fatal("expected missing id error")
	}
}
