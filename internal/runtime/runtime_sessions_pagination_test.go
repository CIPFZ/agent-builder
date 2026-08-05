package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
)

func TestRuntimeSessionPageIsBoundedAndStableAcrossEqualTimestamps(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	conn, err := h.service.workspaceDB(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := h.service.runtime.GetWorkspace(h.service.workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Release(workspace.Cfg.Config().Options.DataDirectory) }()
	if _, err := conn.ExecContext(h.ctx, `DELETE FROM sessions`); err != nil {
		t.Fatal(err)
	}
	planRows, err := conn.QueryContext(h.ctx, `EXPLAIN QUERY PLAN SELECT id FROM sessions WHERE parent_session_id IS NULL AND deleted_at IS NULL ORDER BY pinned DESC, updated_at DESC, id DESC LIMIT 51`)
	if err != nil {
		t.Fatal(err)
	}
	usesPageIndex := false
	for planRows.Next() {
		var id, parent, unused int
		var detail string
		if err := planRows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		usesPageIndex = usesPageIndex || strings.Contains(detail, "idx_sessions_root_active_page")
	}
	_ = planRows.Close()
	if !usesPageIndex {
		t.Fatal("Session first-page query did not use the covering keyset index")
	}
	const total = 1000
	tx, err := conn.BeginTx(h.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.PrepareContext(h.ctx, `
INSERT INTO sessions (id, title, scope, workdir, status, title_source, pinned, updated_at, created_at)
VALUES (?, ?, 'standalone', 'C:/work', 'active', 'user', ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("session-%03d", i)
		pinned := 0
		if i%37 == 0 {
			pinned = 1
		}
		// Ten rows intentionally share each timestamp so the ID tie-breaker is
		// required to avoid duplicates or omissions.
		updatedAt := int64(1_000_000 + i/10)
		if _, err := stmt.ExecContext(h.ctx, id, id, pinned, updatedAt, updatedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	h.service.mu.Lock()
	h.service.sessionID = "session-001"
	h.service.mu.Unlock()

	initial, err := h.service.Sessions(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Sessions) > 50 || !initial.HasMore || initial.NextCursor == "" {
		t.Fatalf("initial page size=%d hasMore=%v cursor=%q", len(initial.Sessions), initial.HasMore, initial.NextCursor)
	}
	activeFound := false
	for _, session := range initial.Sessions {
		activeFound = activeFound || session.ID == "session-001" && session.Active
	}
	if !activeFound {
		t.Fatal("focused old Session was not retained in the bounded initial page")
	}
	encoded, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 256*1024 {
		t.Fatalf("initial Session payload = %d bytes", len(encoded))
	}

	seen := map[string]bool{}
	cursor := ""
	for {
		page, err := h.service.SessionPage(h.ctx, RuntimeSessionPageRequest{Cursor: cursor, Limit: 17})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Sessions) > 17 {
			t.Fatalf("page size = %d", len(page.Sessions))
		}
		for _, session := range page.Sessions {
			if seen[session.ID] {
				t.Fatalf("duplicate Session %s", session.ID)
			}
			seen[session.ID] = true
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			t.Fatalf("non-advancing cursor %q", page.NextCursor)
		}
		cursor = page.NextCursor
	}
	if len(seen) != total {
		t.Fatalf("paged Sessions=%d want=%d", len(seen), total)
	}
}

func TestRuntimeSessionPageRejectsInvalidCursorAndCapsLimit(t *testing.T) {
	h := newRuntimeScenarioHarness(t)
	h.attachBackend()
	if _, err := h.service.SessionPage(h.ctx, RuntimeSessionPageRequest{Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
	page, err := h.service.SessionPage(h.ctx, RuntimeSessionPageRequest{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Sessions) > 100 {
		t.Fatalf("uncapped page size = %d", len(page.Sessions))
	}
}
