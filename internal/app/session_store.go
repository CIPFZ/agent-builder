package app

import (
	"fmt"
	"path/filepath"

	fileStore "myclaw/internal/store/file"

	"myclaw/internal/session"
)

func newPersistentSessionManager(root string) (*session.Manager, error) {
	if root == "" {
		return nil, fmt.Errorf("session store root is required")
	}
	store, err := fileStore.NewSessionStore(root)
	if err != nil {
		return nil, err
	}
	return session.NewManager(store), nil
}

func defaultSessionStoreRoot(baseDir string) string {
	if baseDir == "" {
		baseDir = "."
	}
	return filepath.Join(baseDir, "logs", "sessions")
}
