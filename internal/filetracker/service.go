// Package filetracker provides functionality to track file reads in sessions.
package filetracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
)

// Service defines the interface for tracking file reads in sessions.
type Service interface {
	// RecordRead records when a file was read.
	RecordRead(ctx context.Context, sessionID, path string)
	RecordReadState(ctx context.Context, state ReadState)

	// LastReadTime returns when a file was last read.
	// Returns zero time if never read.
	LastReadTime(ctx context.Context, sessionID, path string) time.Time

	// ListReadFiles returns the paths of all files read in a session.
	ListReadFiles(ctx context.Context, sessionID string) ([]string, error)
}

type ReadState struct {
	SessionID     string
	TurnID        string
	ToolCallID    string
	Path          string
	Offset        int
	Limit         int
	Partial       bool
	TokenEstimate int
	State         string
	Reason        string
}

type service struct {
	q *db.Queries
}

// NewService creates a new file tracker service.
func NewService(q *db.Queries) Service {
	return &service{q: q}
}

// RecordRead records when a file was read.
func (s *service) RecordRead(ctx context.Context, sessionID, path string) {
	s.RecordReadState(ctx, ReadState{SessionID: sessionID, Path: path, State: "recorded"})
}

func (s *service) RecordReadState(ctx context.Context, state ReadState) {
	if state.State == "" {
		state.State = "recorded"
	}
	meta := fileReadMetadata(state.Path)
	if err := s.q.RecordFileRead(ctx, db.RecordFileReadParams{
		SessionID:     state.SessionID,
		Path:          relpath(state.Path),
		TurnID:        state.TurnID,
		ToolCallID:    state.ToolCallID,
		SizeBytes:     meta.size,
		ContentHash:   meta.hash,
		MtimeUnix:     meta.mtime,
		Offset:        int64(state.Offset),
		ReadLimit:     int64(state.Limit),
		Partial:       boolInt64(state.Partial),
		TokenEstimate: int64(state.TokenEstimate),
		State:         state.State,
		Reason:        state.Reason,
	}); err != nil {
		slog.Error("Error recording file read", "error", err, "file", state.Path)
	}
	// Runtime event/audit publication is owned by the runtime scheduler layer;
	// this store records only durable read-file state.
}

// LastReadTime returns when a file was last read.
// Returns zero time if never read.
func (s *service) LastReadTime(ctx context.Context, sessionID, path string) time.Time {
	readFile, err := s.q.GetFileRead(ctx, db.GetFileReadParams{
		SessionID: sessionID,
		Path:      relpath(path),
	})
	if err != nil {
		return time.Time{}
	}

	return time.Unix(readFile.ReadAt, 0)
}

type fileMetadata struct {
	size  int64
	hash  string
	mtime int64
}

func fileReadMetadata(path string) fileMetadata {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return fileMetadata{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileMetadata{size: info.Size(), mtime: info.ModTime().Unix()}
	}
	sum := sha256.Sum256(data)
	return fileMetadata{
		size:  info.Size(),
		hash:  "sha256:" + hex.EncodeToString(sum[:]),
		mtime: info.ModTime().Unix(),
	}
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func relpath(path string) string {
	path = filepath.Clean(path)
	basepath, err := os.Getwd()
	if err != nil {
		slog.Warn("Error getting basepath", "error", err)
		return path
	}
	relpath, err := filepath.Rel(basepath, path)
	if err != nil {
		slog.Warn("Error getting relpath", "error", err)
		return path
	}
	return relpath
}

// ListReadFiles returns the paths of all files read in a session.
func (s *service) ListReadFiles(ctx context.Context, sessionID string) ([]string, error) {
	readFiles, err := s.q.ListSessionReadFiles(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("listing read files: %w", err)
	}

	basepath, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	paths := make([]string, 0, len(readFiles))
	for _, rf := range readFiles {
		paths = append(paths, filepath.Join(basepath, rf.Path))
	}
	return paths, nil
}
