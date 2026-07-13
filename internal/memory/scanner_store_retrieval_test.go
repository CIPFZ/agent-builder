package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestScannerStoreRebuildAndRetrieval(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "memory")
	if err := EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writeTopic(t, root, "feedback/testing.md", "mem_testing", TypeFeedback, "Testing policy", "Use real database tests", []string{"testing", "database"}, "Integration tests should use the real database.")
	writeTopic(t, root, "project/releases.md", "mem_release", TypeProject, "Release freeze", "Release freeze facts", []string{"release"}, "Release branches freeze on Wednesdays.")

	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	service := NewService(NewStore(conn), "project-1", root)
	index, err := service.RebuildIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if index.Indexed != 2 || index.Failed != 0 {
		t.Fatalf("unexpected index result: %#v", index)
	}
	results, err := service.Search(ctx, SearchRequest{Query: "database integration", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Record.ID != "mem_testing" {
		t.Fatalf("unexpected retrieval results: %#v", results)
	}
	disabled, err := service.Disable(ctx, "mem_testing", false)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled {
		t.Fatal("record should be disabled")
	}
	results, err = service.Search(ctx, SearchRequest{Query: "database integration", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("disabled memory should be skipped: %#v", results)
	}
}

func TestRebuildMarksMissingFilesDeleted(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "memory")
	if err := EnsureLayout(root); err != nil {
		t.Fatal(err)
	}
	writeTopic(t, root, "feedback/testing.md", "mem_testing", TypeFeedback, "Testing policy", "Use real database tests", nil, "Integration tests should use the real database.")
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Release(dataDir) })
	service := NewService(NewStore(conn), "project-1", root)
	if _, err := service.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "feedback", "testing.md")); err != nil {
		t.Fatal(err)
	}
	index, err := service.RebuildIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if index.Deleted != 1 {
		t.Fatalf("expected one deleted record: %#v", index)
	}
	record, err := NewStore(conn).Get(ctx, "mem_testing")
	if err != nil {
		t.Fatal(err)
	}
	if record.DeletedAt == "" {
		t.Fatal("missing file was not tombstoned")
	}
}

func writeTopic(t *testing.T, root, rel, id, typ, title, description string, tags []string, body string) {
	t.Helper()
	data, err := RenderMarkdown(Document{Frontmatter: Frontmatter{
		ID:          id,
		Title:       title,
		Type:        typ,
		Description: description,
		Tags:        tags,
		CreatedAt:   "2026-06-29T00:00:00Z",
		UpdatedAt:   "2026-06-29T00:00:00Z",
	}, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
