package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	pragmas = map[string]string{
		"foreign_keys":  "ON",
		"journal_mode":  "WAL",
		"page_size":     "4096",
		"temp_store":    "MEMORY",
		"cache_size":    "-8000",
		"synchronous":   "NORMAL",
		"secure_delete": "ON",
		"busy_timeout":  "30000",
	}
)

const expectedSchemaGeneration = "2"

//go:embed schema.sql
var schemaFS embed.FS

// connEntry holds a shared database connection and its reference count.
type connEntry struct {
	db       *sql.DB
	refCount int
}

var (
	pool   = make(map[string]*connEntry)
	poolMu sync.Mutex
)

// Connect opens a SQLite database connection for the given data
// directory and initializes the final schema. If a connection to the same database
// file already exists, the existing connection is returned with its
// reference count incremented. Callers must pair each Connect with a
// [Release] when they no longer need the connection.
func Connect(ctx context.Context, dataDir string) (*sql.DB, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data.dir is not set")
	}

	dbPath := filepath.Join(dataDir, "agent-builder.db")

	// Resolve to an absolute path so that different relative paths to
	// the same file share a single connection.
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		absPath = dbPath
	}

	poolMu.Lock()
	defer poolMu.Unlock()

	if entry, ok := pool[absPath]; ok {
		entry.refCount++
		return entry.db, nil
	}

	conn, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}

	// Serialize all access through a single connection. SQLite
	// serializes writes at the file level anyway, and allowing multiple
	// pool connections to interleave writes/checkpoints (especially
	// under concurrent sub-agents) has caused WAL/header desync
	// resulting in SQLITE_NOTADB (26) on the next open.
	conn.SetMaxOpenConns(1)

	if err = conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := ensureSchema(ctx, conn); err != nil {
		if !isIncompatibleSchema(err) {
			conn.Close()
			slog.Error("Failed to initialize database schema", "error", err)
			return nil, fmt.Errorf("failed to initialize database schema: %w", err)
		}
		conn.Close()
		if err := backupAndRecreateDatabase(ctx, dbPath); err != nil {
			return nil, err
		}
		conn, err = openDB(dbPath)
		if err != nil {
			return nil, err
		}
		conn.SetMaxOpenConns(1)
		if err = conn.PingContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to connect to recreated database: %w", err)
		}
		if err := initializeSchema(ctx, conn); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to initialize recreated database schema: %w", err)
		}
	}

	pool[absPath] = &connEntry{db: conn, refCount: 1}
	return conn, nil
}

// Release decrements the reference count for the database at the given
// data directory. When the count reaches zero the underlying connection
// is closed and removed from the pool.
func Release(dataDir string) error {
	dbPath := filepath.Join(dataDir, "agent-builder.db")
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		absPath = dbPath
	}

	poolMu.Lock()
	defer poolMu.Unlock()

	entry, ok := pool[absPath]
	if !ok {
		return nil
	}

	entry.refCount--
	if entry.refCount > 0 {
		return nil
	}

	delete(pool, absPath)
	return entry.db.Close()
}

// ResetPool closes all pooled connections and clears the pool. This is
// intended for use in tests to ensure a clean state between test cases.
func ResetPool() {
	poolMu.Lock()
	defer poolMu.Unlock()
	for path, entry := range pool {
		entry.db.Close()
		delete(pool, path)
	}
}

type incompatibleSchemaError struct {
	reason string
}

func (e incompatibleSchemaError) Error() string {
	return "incompatible database schema: " + e.reason
}

func isIncompatibleSchema(err error) bool {
	_, ok := err.(incompatibleSchemaError)
	return ok
}

func ensureSchema(ctx context.Context, conn *sql.DB) error {
	var tableCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount == 0 {
		return initializeSchema(ctx, conn)
	}
	var generation string
	err := conn.QueryRowContext(ctx, `SELECT value FROM runtime_settings WHERE key = 'schema_generation'`).Scan(&generation)
	if err != nil {
		return incompatibleSchemaError{reason: "runtime_settings.schema_generation is missing"}
	}
	if generation != expectedSchemaGeneration {
		return incompatibleSchemaError{reason: fmt.Sprintf("schema_generation=%q, expected %q", generation, expectedSchemaGeneration)}
	}
	return nil
}

func initializeSchema(ctx context.Context, conn *sql.DB) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, string(schema)); err != nil {
		return err
	}
	return nil
}

func backupAndRecreateDatabase(ctx context.Context, dbPath string) error {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(absPath)
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	backupDir := filepath.Join(dir, "backups", "schema-reset-"+timestamp)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return fmt.Errorf("failed to create database backup directory: %w", err)
	}
	for _, path := range []string{absPath, absPath + "-wal", absPath + "-shm"} {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("failed to stat database file for backup: %w", err)
		}
		target := filepath.Join(backupDir, filepath.Base(path))
		if err := os.Rename(path, target); err != nil {
			return fmt.Errorf("failed to backup %s: %w", filepath.Base(path), err)
		}
		slog.Warn("Backed up incompatible Agent Builder database", "source", path, "backup", target)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}
