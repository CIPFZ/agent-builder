package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnect_SharesConnectionForSameDataDir(t *testing.T) {
	t.Cleanup(ResetPool)

	dataDir := t.TempDir()

	conn1, err := Connect(context.Background(), dataDir)
	require.NoError(t, err)

	conn2, err := Connect(context.Background(), dataDir)
	require.NoError(t, err)

	require.Same(t, conn1, conn2, "should return the same *sql.DB for the same data dir")

	// Releasing once should not close the connection.
	require.NoError(t, Release(dataDir))
	require.NoError(t, conn1.PingContext(context.Background()), "connection should still be usable after partial release")

	// Releasing again should close it.
	require.NoError(t, Release(dataDir))
	require.Error(t, conn1.PingContext(context.Background()), "connection should be closed after final release")
}

func TestConnect_BackupAndRecreateOnSchemaGenerationMismatch(t *testing.T) {
	t.Cleanup(ResetPool)

	dataDir := t.TempDir()
	conn, err := openDB(filepath.Join(dataDir, "agent-builder.db"))
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE runtime_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL);
INSERT INTO runtime_settings (key, value, updated_at) VALUES ('schema_generation', '0', 1);
CREATE TABLE legacy_data (value TEXT);`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	recreated, err := Connect(context.Background(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = Release(dataDir) })

	var generation string
	require.NoError(t, recreated.QueryRowContext(context.Background(), `SELECT value FROM runtime_settings WHERE key = 'schema_generation'`).Scan(&generation))
	require.Equal(t, expectedSchemaGeneration, generation)
	var legacyName string
	err = recreated.QueryRowContext(context.Background(), `SELECT name FROM sqlite_schema WHERE type = 'table' AND name = 'legacy_data'`).Scan(&legacyName)
	require.ErrorIs(t, err, sql.ErrNoRows)

	matches, err := filepath.Glob(filepath.Join(dataDir, "backups", "schema-reset-*", "agent-builder.db"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	_, err = os.Stat(matches[0])
	require.NoError(t, err)
}

func TestConnect_SeparateConnectionsForDifferentDataDirs(t *testing.T) {
	t.Cleanup(ResetPool)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	conn1, err := Connect(context.Background(), dir1)
	require.NoError(t, err)

	conn2, err := Connect(context.Background(), dir2)
	require.NoError(t, err)

	require.NotSame(t, conn1, conn2, "different data dirs should get different connections")

	require.NoError(t, Release(dir1))
	require.NoError(t, Release(dir2))
}

func TestRelease_NoopForUnknownDataDir(t *testing.T) {
	t.Cleanup(ResetPool)

	require.NoError(t, Release("/nonexistent/path"), "releasing unknown data dir should not error")
}
