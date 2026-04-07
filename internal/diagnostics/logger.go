package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Component string         `json:"component"`
	Event     string         `json:"event"`
	Message   string         `json:"message,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	path string
	mu   sync.Mutex
	file *os.File
}

func NewLogger(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{path: path, file: file}, nil
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Logger) Log(entry Entry) error {
	if l == nil || l.file == nil {
		return nil
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return err
	}
	return l.file.Sync()
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
