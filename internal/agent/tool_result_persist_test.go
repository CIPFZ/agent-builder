package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersist_WritesFileAndReturnsPath(t *testing.T) {
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	p := NewResultPersister(resultsDir, 500*1024*1024, 30)

	content := "full tool output content for persistence"
	path, err := p.Persist("session-1", "toolu_001", content)
	require.NoError(t, err)
	require.Contains(t, path, "toolu_001.txt")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(data))
}

func TestPersist_CreatesSessionDir(t *testing.T) {
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	p := NewResultPersister(resultsDir, 500*1024*1024, 30)

	_, err := p.Persist("session-1", "toolu_001", "content")
	require.NoError(t, err)

	sessionDir := filepath.Join(resultsDir, "session-1")
	info, err := os.Stat(sessionDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestPersist_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	p := NewResultPersister(resultsDir, 500*1024*1024, 30)

	path, err := p.Persist("session-1", "toolu_001", "test content")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		// No .tmp files should remain after atomic write
		if e.Name() != filepath.Base(path) {
			require.NotEqual(t, ".tmp", filepath.Ext(e.Name()),
				"tmp file should not remain: %s", e.Name())
		}
	}
}

func TestPersist_SessionLimitReached(t *testing.T) {
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	p := NewResultPersister(resultsDir, 100, 30) // 100 byte limit

	// First write: small — should succeed
	_, err := p.Persist("session-1", "toolu_001", "aaaaa")
	require.NoError(t, err)

	// Second write: large — should hit session limit
	_, err = p.Persist("session-1", "toolu_002", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	require.Error(t, err)
	require.Contains(t, err.Error(), "session limit")
}

func TestCleanupOldFiles_RemovesExpiredFiles(t *testing.T) {
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	p := NewResultPersister(resultsDir, 500*1024*1024, 0) // TTL=0 means everything is expired

	path, err := p.Persist("session-1", "toolu_001", "content")
	require.NoError(t, err)
	require.FileExists(t, path)

	p.CleanupOldFiles()

	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestCleanupOldFiles_PreservesRecentFiles(t *testing.T) {
	tmpDir := t.TempDir()
	resultsDir := filepath.Join(tmpDir, "results")
	p := NewResultPersister(resultsDir, 500*1024*1024, 365) // TTL=365 days

	path, err := p.Persist("session-1", "toolu_001", "content")
	require.NoError(t, err)

	p.CleanupOldFiles()

	_, err = os.Stat(path)
	require.NoError(t, err, "recent files should be preserved")
}
