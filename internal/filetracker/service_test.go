package filetracker

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/stretchr/testify/require"
)

type testEnv struct {
	ctx context.Context
	q   *db.Queries
	svc Service
}

func setupTest(t *testing.T) *testEnv {
	t.Helper()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	return &testEnv{
		ctx: t.Context(),
		q:   q,
		svc: NewService(q),
	}
}

func (e *testEnv) createSession(t *testing.T, sessionID string) {
	t.Helper()
	_, err := e.q.CreateSession(e.ctx, db.CreateSessionParams{
		ID:               sessionID,
		Title:            "Test Session",
		Scope:            "standalone",
		Workdir:          sql.NullString{String: ".", Valid: true},
		CanonicalWorkdir: sql.NullString{String: ".", Valid: true},
		WorkdirExists:    1,
	})
	require.NoError(t, err)
}

func TestService_RecordRead(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-1"
	path := "/path/to/file.go"
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, path)

	lastRead := env.svc.LastReadTime(env.ctx, sessionID, path)
	require.False(t, lastRead.IsZero(), "expected non-zero time after recording read")
	require.WithinDuration(t, time.Now(), lastRead, 2*time.Second)
}

func TestService_LastReadTime_NotFound(t *testing.T) {
	env := setupTest(t)

	lastRead := env.svc.LastReadTime(env.ctx, "nonexistent-session", "/nonexistent/path")
	require.True(t, lastRead.IsZero(), "expected zero time for unread file")
}

func TestService_RecordRead_UpdatesTimestamp(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-2"
	path := "/path/to/file.go"
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, path)
	firstRead := env.svc.LastReadTime(env.ctx, sessionID, path)
	require.False(t, firstRead.IsZero())

	synctest.Test(t, func(t *testing.T) {
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		env.svc.RecordRead(env.ctx, sessionID, path)
		secondRead := env.svc.LastReadTime(env.ctx, sessionID, path)

		require.False(t, secondRead.Before(firstRead), "second read time should not be before first")
	})
}

func TestService_RecordRead_DifferentSessions(t *testing.T) {
	env := setupTest(t)

	path := "/shared/file.go"
	session1, session2 := "session-1", "session-2"
	env.createSession(t, session1)
	env.createSession(t, session2)

	env.svc.RecordRead(env.ctx, session1, path)

	lastRead1 := env.svc.LastReadTime(env.ctx, session1, path)
	require.False(t, lastRead1.IsZero())

	lastRead2 := env.svc.LastReadTime(env.ctx, session2, path)
	require.True(t, lastRead2.IsZero(), "session 2 should not see session 1's read")
}

func TestService_RecordRead_DifferentPaths(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-3"
	path1, path2 := "/path/to/file1.go", "/path/to/file2.go"
	env.createSession(t, sessionID)

	env.svc.RecordRead(env.ctx, sessionID, path1)

	lastRead1 := env.svc.LastReadTime(env.ctx, sessionID, path1)
	require.False(t, lastRead1.IsZero())

	lastRead2 := env.svc.LastReadTime(env.ctx, sessionID, path2)
	require.True(t, lastRead2.IsZero(), "path2 should not be recorded")
}

func TestService_MarkSessionStale(t *testing.T) {
	t.Parallel()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	q := db.New(conn)
	svc := NewServiceWithConn(q, conn)

	sessionID := "stale-test"
	_, err = q.CreateSession(t.Context(), db.CreateSessionParams{
		ID:               sessionID,
		Title:            "Test",
		Scope:            "standalone",
		Workdir:          sql.NullString{String: ".", Valid: true},
		CanonicalWorkdir: sql.NullString{String: ".", Valid: true},
		WorkdirExists:    1,
	})
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "hello.go")
	require.NoError(t, os.WriteFile(path, []byte("package hello"), 0o644))

	svc.RecordRead(t.Context(), sessionID, path)

	// Baseline: the row exists with state=recorded.
	got, err := q.GetFileRead(t.Context(), db.GetFileReadParams{
		SessionID: sessionID,
		Path:      relpath(path),
	})
	require.NoError(t, err)
	require.Equal(t, "recorded", got.State)

	// After MarkSessionStale, state flips to "stale" and reason is set.
	require.NoError(t, svc.MarkSessionStale(t.Context(), sessionID, "compact_boundary"))

	got2, err := q.GetFileRead(t.Context(), db.GetFileReadParams{
		SessionID: sessionID,
		Path:      relpath(path),
	})
	require.NoError(t, err)
	require.Equal(t, "stale", got2.State)
	require.Equal(t, "compact_boundary", got2.Reason)
}

func TestService_MarkSessionStale_NoConnIsNoop(t *testing.T) {
	t.Parallel()

	env := setupTest(t)
	// NewService (without conn) makes MarkSessionStale a silent no-op —
	// callers of the compact path still succeed even when the DB path is
	// unavailable in tests.
	require.NoError(t, env.svc.MarkSessionStale(env.ctx, "any-session", "compact_boundary"))
}

func TestService_RecordReadStateRecordsMetadata(t *testing.T) {
	env := setupTest(t)

	sessionID := "test-session-metadata"
	env.createSession(t, sessionID)
	path := filepath.Join(t.TempDir(), "file.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\n"), 0o644))

	env.svc.RecordReadState(env.ctx, ReadState{
		SessionID:     sessionID,
		TurnID:        "turn-1",
		ToolCallID:    "tool-1",
		Path:          path,
		Offset:        10,
		Limit:         20,
		Partial:       true,
		TokenEstimate: 3,
		State:         "recorded",
		Reason:        "view_tool",
	})

	read, err := env.q.GetFileRead(env.ctx, db.GetFileReadParams{SessionID: sessionID, Path: relpath(path)})
	require.NoError(t, err)
	require.Equal(t, "turn-1", read.TurnID)
	require.Equal(t, "tool-1", read.ToolCallID)
	require.Equal(t, int64(10), read.Offset)
	require.Equal(t, int64(20), read.ReadLimit)
	require.Equal(t, int64(1), read.Partial)
	require.Equal(t, int64(3), read.TokenEstimate)
	require.NotEmpty(t, read.ContentHash)
	require.Greater(t, read.SizeBytes, int64(0))
}
