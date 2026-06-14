package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ResultPersister handles writing tool results to disk and TTL cleanup.
type ResultPersister struct {
	resultsDir         string
	perSessionMaxBytes int64
	ttlDays            int
}

// NewResultPersister creates a new ResultPersister.
func NewResultPersister(resultsDir string, perSessionMaxBytes int64, ttlDays int) *ResultPersister {
	return &ResultPersister{
		resultsDir:         resultsDir,
		perSessionMaxBytes: perSessionMaxBytes,
		ttlDays:            ttlDays,
	}
}

// Persist writes content to resultsDir/{sessionID}/{toolCallID}.txt
// using atomic write (tmp file + rename). Returns the stored path.
func (p *ResultPersister) Persist(sessionID, toolCallID, content string) (string, error) {
	sessionDir := filepath.Join(p.resultsDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}

	if err := p.checkSessionSpace(sessionDir, int64(len(content))); err != nil {
		return "", err
	}

	filePath := filepath.Join(sessionDir, toolCallID+".txt")
	tmpPath := filePath + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write tmp file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename tmp file: %w", err)
	}

	return filePath, nil
}

// checkSessionSpace returns an error if the session directory would exceed the limit
// after writing newBytes of content.
func (p *ResultPersister) checkSessionSpace(sessionDir string, newBytes int64) error {
	var totalSize int64
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
		}
	}
	if totalSize+newBytes > p.perSessionMaxBytes {
		return fmt.Errorf("session limit reached: %d bytes (max %d)", totalSize, p.perSessionMaxBytes)
	}
	return nil
}

// CleanupOldFiles removes tool result files older than ttlDays.
func (p *ResultPersister) CleanupOldFiles() {
	entries, err := os.ReadDir(p.resultsDir)
	if err != nil {
		return
	}
	if p.ttlDays <= 0 {
		for _, entry := range entries {
			if entry.IsDir() {
				os.RemoveAll(filepath.Join(p.resultsDir, entry.Name()))
			}
		}
		return
	}

	cutoff := time.Now().Add(-time.Duration(p.ttlDays) * 24 * time.Hour)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDir := filepath.Join(p.resultsDir, entry.Name())
		sessionInfo, err := entry.Info()
		if err != nil {
			continue
		}

		if sessionInfo.ModTime().Before(cutoff) {
			os.RemoveAll(sessionDir)
			continue
		}

		files, err := os.ReadDir(sessionDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			info, err := file.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(sessionDir, file.Name()))
			}
		}

		removeIfEmpty(sessionDir)
	}
}

// removeIfEmpty removes a directory if it contains no entries.
func removeIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}
	os.Remove(dir)
}
