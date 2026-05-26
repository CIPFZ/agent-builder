package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/runtimeapi"
)

const (
	runtimeRefKindOutput                = "output"
	runtimeRefKindArtifact              = "artifact"
	runtimeRefKindDiff                  = "diff"
	runtimeRefKindCompactOriginalOutput = "compact_original_output"
	runtimeRefKindShellJobOutput        = "shell_job_output"
	runtimeRefKindTaskArtifact          = "task_artifact"

	runtimeRefStorageInline = "inline"
	runtimeRefStorageFile   = "file"

	runtimeRefRedactionSafe     = "safe"
	runtimeRefRedactionRedacted = "redacted"
	runtimeRefRedactionUnsafe   = "unsafe"

	runtimeRefInlineLimit = 8 * 1024
)

var errRuntimeRefNotFound = errors.New("runtime ref not found")

type runtimeRefStore struct {
	db   *sql.DB
	root string
}

type runtimeRefCreateRequest struct {
	ID                string
	SessionID         string
	TurnID            string
	ToolCallID        string
	TaskID            string
	SandboxDecisionID string
	SandboxMode       string
	SandboxStatus     string
	Kind              string
	MediaType         string
	ContentType       string
	Payload           []byte
	Summary           string
	StorageKind       string
	StoragePath       string
	CreatedAt         time.Time
}

func newRuntimeRefStore(db *sql.DB, dataDir string) runtimeRefStore {
	root := filepath.Join(dataDir, "runtime_refs")
	return runtimeRefStore{db: db, root: filepath.Clean(root)}
}

func (s runtimeRefStore) Create(ctx context.Context, req runtimeRefCreateRequest) (RuntimeRef, error) {
	if s.db == nil {
		return RuntimeRef{}, errors.New("runtime ref database is not available")
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Kind = strings.TrimSpace(req.Kind)
	if req.SessionID == "" {
		return RuntimeRef{}, errors.New("runtime ref session id is required")
	}
	if req.Kind == "" {
		return RuntimeRef{}, errors.New("runtime ref kind is required")
	}
	if req.ID == "" {
		req.ID = newRuntimeRefID()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	redactedPayload := redactRuntimeString("", string(req.Payload))
	redactionStatus := runtimeRefRedactionSafe
	if redactedPayload != string(req.Payload) {
		redactionStatus = runtimeRefRedactionUnsafe
	}
	ref := RuntimeRef{
		ID:                req.ID,
		URI:               "runtime://refs/" + req.ID,
		SessionID:         req.SessionID,
		TurnID:            strings.TrimSpace(req.TurnID),
		ToolCallID:        strings.TrimSpace(req.ToolCallID),
		TaskID:            strings.TrimSpace(req.TaskID),
		SandboxDecisionID: strings.TrimSpace(req.SandboxDecisionID),
		SandboxMode:       strings.TrimSpace(req.SandboxMode),
		SandboxStatus:     strings.TrimSpace(req.SandboxStatus),
		Kind:              req.Kind,
		MediaType:         firstNonEmpty(req.MediaType, "text/plain"),
		ContentType:       firstNonEmpty(req.ContentType, req.MediaType, "text/plain"),
		SizeBytes:         int64(len(req.Payload)),
		EstimatedTokens:   estimateRuntimeTokens(string(req.Payload)),
		Preview:           preview(redactedPayload, runtimePartPreviewLimit),
		Summary:           preview(redactRuntimeString("summary", firstNonEmpty(req.Summary, redactedPayload)), auditPreviewLimit),
		StorageKind:       runtimeRefStorageInline,
		RedactionStatus:   redactionStatus,
		CreatedAt:         req.CreatedAt.UnixMilli(),
		CanReadContent:    redactionStatus == runtimeRefRedactionSafe,
	}
	if req.StoragePath != "" {
		ref.StorageKind = firstNonEmpty(req.StorageKind, runtimeRefStorageFile)
		ref.StoragePath = filepath.ToSlash(req.StoragePath)
		if _, err := s.resolveStoragePath(ref.StoragePath); err != nil {
			return RuntimeRef{}, err
		}
	} else if len(req.Payload) > runtimeRefInlineLimit {
		ref.StorageKind = runtimeRefStorageFile
		rel, abs, err := s.storagePathForID(ref.ID)
		if err != nil {
			return RuntimeRef{}, err
		}
		// TODO(runtime): add reference-aware garbage collection once session
		// retention policies exist. Refs are intentionally durable today.
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return RuntimeRef{}, fmt.Errorf("failed to create runtime ref storage: %w", err)
		}
		if err := os.WriteFile(abs, req.Payload, 0o600); err != nil {
			return RuntimeRef{}, fmt.Errorf("failed to write runtime ref payload: %w", err)
		}
		ref.StoragePath = rel
	} else {
		ref.InlinePayload = string(req.Payload)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_refs (
    id, uri, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    uri = excluded.uri,
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    tool_call_id = excluded.tool_call_id,
    task_id = excluded.task_id,
    sandbox_decision_id = excluded.sandbox_decision_id,
    sandbox_mode = excluded.sandbox_mode,
    sandbox_status = excluded.sandbox_status,
    kind = excluded.kind,
    media_type = excluded.media_type,
    content_type = excluded.content_type,
    size_bytes = excluded.size_bytes,
    estimated_tokens = excluded.estimated_tokens,
    preview = excluded.preview,
    summary = excluded.summary,
    storage_kind = excluded.storage_kind,
    storage_path = excluded.storage_path,
    inline_payload = excluded.inline_payload,
    redaction_status = excluded.redaction_status,
    created_at = excluded.created_at`,
		ref.ID,
		ref.URI,
		ref.SessionID,
		nullableString(ref.TurnID),
		nullableString(ref.ToolCallID),
		nullableString(ref.TaskID),
		nullableString(ref.SandboxDecisionID),
		nullableString(ref.SandboxMode),
		nullableString(ref.SandboxStatus),
		ref.Kind,
		nullableString(ref.MediaType),
		nullableString(ref.ContentType),
		ref.SizeBytes,
		ref.EstimatedTokens,
		nullableString(ref.Preview),
		nullableString(ref.Summary),
		ref.StorageKind,
		nullableString(ref.StoragePath),
		nullableString(ref.InlinePayload),
		ref.RedactionStatus,
		ref.CreatedAt,
	)
	if err != nil {
		return RuntimeRef{}, fmt.Errorf("failed to store runtime ref: %w", err)
	}
	return s.Get(ctx, ref.ID)
}

func (s runtimeRefStore) Get(ctx context.Context, idOrURI string) (RuntimeRef, error) {
	if s.db == nil {
		return RuntimeRef{}, errors.New("runtime ref database is not available")
	}
	idOrURI = normalizeRuntimeRefID(idOrURI)
	row := s.db.QueryRowContext(ctx, `
SELECT id, uri, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM runtime_refs
WHERE id = ? OR uri = ?`, idOrURI, idOrURI)
	ref, err := scanRuntimeRef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeRef{}, errRuntimeRefNotFound
	}
	if err != nil {
		return RuntimeRef{}, err
	}
	return runtimeRefMetadataOnly(ref), nil
}

func (s runtimeRefStore) List(ctx context.Context, req RuntimeRefListRequest) ([]RuntimeRef, error) {
	if s.db == nil {
		return nil, errors.New("runtime ref database is not available")
	}
	query := `
SELECT id, uri, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM runtime_refs`
	var clauses []string
	var args []any
	if req.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, req.SessionID)
	}
	if req.TurnID != "" {
		clauses = append(clauses, "turn_id = ?")
		args = append(args, req.TurnID)
	}
	if req.ToolCallID != "" {
		clauses = append(clauses, "tool_call_id = ?")
		args = append(args, req.ToolCallID)
	}
	if req.TaskID != "" {
		clauses = append(clauses, "task_id = ?")
		args = append(args, req.TaskID)
	}
	if req.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, req.Kind)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime refs: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var refs []RuntimeRef
	for rows.Next() {
		ref, err := scanRuntimeRef(rows)
		if err != nil {
			return nil, err
		}
		refs = append(refs, runtimeRefMetadataOnly(ref))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime refs: %w", err)
	}
	return refs, nil
}

func (s runtimeRefStore) ReadContent(ctx context.Context, idOrURI string) (RuntimeRefContentResponse, error) {
	ref, err := s.Get(ctx, idOrURI)
	if err != nil {
		return RuntimeRefContentResponse{}, err
	}
	fullRef, err := s.getWithPayload(ctx, idOrURI)
	if err != nil {
		return RuntimeRefContentResponse{}, err
	}
	content := fullRef.InlinePayload
	if ref.StorageKind == runtimeRefStorageFile {
		path, err := s.resolveStoragePath(ref.StoragePath)
		if err != nil {
			return RuntimeRefContentResponse{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return RuntimeRefContentResponse{}, fmt.Errorf("failed to read runtime ref payload: %w", err)
		}
		content = string(data)
	}
	redacted := false
	if ref.RedactionStatus != runtimeRefRedactionSafe {
		content = redactRuntimeString("", content)
		redacted = true
		ref.CanReadContent = false
	} else {
		ref.CanReadContent = true
	}
	return RuntimeRefContentResponse{Ref: ref, Content: content, Redacted: redacted}, nil
}

func (s runtimeRefStore) getWithPayload(ctx context.Context, idOrURI string) (RuntimeRef, error) {
	idOrURI = normalizeRuntimeRefID(idOrURI)
	row := s.db.QueryRowContext(ctx, `
SELECT id, uri, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM runtime_refs
WHERE id = ? OR uri = ?`, idOrURI, idOrURI)
	ref, err := scanRuntimeRef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeRef{}, errRuntimeRefNotFound
	}
	return ref, err
}

func (s runtimeRefStore) storagePathForID(id string) (string, string, error) {
	safeID := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(id)
	if safeID == "" || safeID == "." || safeID == ".." {
		return "", "", errors.New("invalid runtime ref id")
	}
	rel := filepath.ToSlash(filepath.Join(safeID[:minInt(2, len(safeID))], safeID+".blob"))
	abs, err := s.resolveStoragePath(rel)
	return rel, abs, err
}

func (s runtimeRefStore) resolveStoragePath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errors.New("runtime ref storage path is required")
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, ":") {
		return "", errors.New("runtime ref storage path must be relative")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("runtime ref storage path traversal rejected")
	}
	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanRel))
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(filepath.Separator)) {
		return "", errors.New("runtime ref storage path escapes runtime data directory")
	}
	return pathAbs, nil
}

type runtimeRefScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeRef(scanner runtimeRefScanner) (RuntimeRef, error) {
	var ref RuntimeRef
	var turnID, toolCallID, taskID, sandboxDecisionID, sandboxMode, sandboxStatus, mediaType, contentType, previewText, summary, storagePath, inlinePayload sql.NullString
	if err := scanner.Scan(
		&ref.ID,
		&ref.URI,
		&ref.SessionID,
		&turnID,
		&toolCallID,
		&taskID,
		&sandboxDecisionID,
		&sandboxMode,
		&sandboxStatus,
		&ref.Kind,
		&mediaType,
		&contentType,
		&ref.SizeBytes,
		&ref.EstimatedTokens,
		&previewText,
		&summary,
		&ref.StorageKind,
		&storagePath,
		&inlinePayload,
		&ref.RedactionStatus,
		&ref.CreatedAt,
	); err != nil {
		return RuntimeRef{}, err
	}
	ref.TurnID = turnID.String
	ref.ToolCallID = toolCallID.String
	ref.TaskID = taskID.String
	ref.SandboxDecisionID = sandboxDecisionID.String
	ref.SandboxMode = sandboxMode.String
	ref.SandboxStatus = sandboxStatus.String
	ref.MediaType = mediaType.String
	ref.ContentType = contentType.String
	ref.Preview = previewText.String
	ref.Summary = summary.String
	ref.StoragePath = storagePath.String
	ref.InlinePayload = inlinePayload.String
	ref.CanReadContent = ref.RedactionStatus == runtimeRefRedactionSafe
	return ref, nil
}

func newRuntimeRefID() string {
	return "ref_" + newRequestID()
}

func normalizeRuntimeRefID(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "runtime://refs/")
}

func (r *runtimeService) ensureRuntimeRefStore(ctx context.Context) (runtimeRefStore, error) {
	if r.refs.db != nil {
		return r.refs, nil
	}
	if r.turns.db != nil {
		dataDir := os.TempDir()
		r.refs = newRuntimeRefStore(r.turns.db, dataDir)
		return r.refs, nil
	}
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return runtimeRefStore{}, err
	}
	dataDir := ""
	r.mu.Lock()
	if r.workspace != nil {
		dataDir = r.workspace.DataDir
	}
	r.mu.Unlock()
	if dataDir == "" {
		dataDir = os.TempDir()
	}
	r.refs = newRuntimeRefStore(db, dataDir)
	return r.refs, nil
}

func (r *runtimeService) createRuntimeRef(ctx context.Context, req runtimeRefCreateRequest) (RuntimeRef, error) {
	store, err := r.ensureRuntimeRefStore(ctx)
	if err != nil {
		return RuntimeRef{}, err
	}
	ref, err := store.Create(ctx, req)
	if err != nil {
		return RuntimeRef{}, err
	}
	r.publishRuntimeRefCreated(ref)
	return ref, nil
}

func (r *runtimeService) publishRuntimeRefCreated(ref RuntimeRef) {
	payload := runtimeRefEventPayload(ref)
	eventType := runtimeapi.EventOutputRefCreated
	if ref.Kind == runtimeRefKindArtifact || ref.Kind == runtimeRefKindTaskArtifact {
		eventType = runtimeapi.EventArtifactRefCreated
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:         newRuntimeEventID(),
		Type:       eventType,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  ref.SessionID,
		TurnID:     ref.TurnID,
		ToolCallID: ref.ToolCallID,
		Payload:    payload,
	})
	if ref.ToolCallID != "" && (ref.Kind == runtimeRefKindOutput || ref.Kind == runtimeRefKindShellJobOutput || ref.Kind == runtimeRefKindDiff || ref.Kind == runtimeRefKindCompactOriginalOutput) {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:         newRuntimeEventID(),
			Type:       runtimeapi.EventToolOutputRefCreated,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			SessionID:  ref.SessionID,
			TurnID:     ref.TurnID,
			ToolCallID: ref.ToolCallID,
			Payload:    payload,
		})
	}
}

func runtimeRefEventPayload(ref RuntimeRef) map[string]any {
	return map[string]any{
		"id":                  ref.ID,
		"uri":                 ref.URI,
		"session_id":          ref.SessionID,
		"turn_id":             ref.TurnID,
		"tool_call_id":        ref.ToolCallID,
		"task_id":             ref.TaskID,
		"sandbox_decision_id": ref.SandboxDecisionID,
		"sandbox_mode":        ref.SandboxMode,
		"sandbox_status":      ref.SandboxStatus,
		"kind":                ref.Kind,
		"media_type":          ref.MediaType,
		"content_type":        ref.ContentType,
		"size_bytes":          ref.SizeBytes,
		"estimated_tokens":    ref.EstimatedTokens,
		"preview":             ref.Preview,
		"summary":             ref.Summary,
		"storage_kind":        ref.StorageKind,
		"redaction_status":    ref.RedactionStatus,
		"created_at":          ref.CreatedAt,
	}
}

func runtimeRefMetadataOnly(ref RuntimeRef) RuntimeRef {
	ref.InlinePayload = ""
	return ref
}

func (r *runtimeService) Refs(ctx context.Context, req RuntimeRefListRequest) (RuntimeRefsResponse, error) {
	store, err := r.ensureRuntimeRefStore(ctx)
	if err != nil {
		return RuntimeRefsResponse{}, err
	}
	refs, err := store.List(ctx, req)
	if err != nil {
		return RuntimeRefsResponse{}, err
	}
	return RuntimeRefsResponse{Refs: refs}, nil
}

func (r *runtimeService) Ref(ctx context.Context, id string) (RuntimeRefResponse, error) {
	store, err := r.ensureRuntimeRefStore(ctx)
	if err != nil {
		return RuntimeRefResponse{}, err
	}
	ref, err := store.Get(ctx, id)
	if err != nil {
		return RuntimeRefResponse{}, err
	}
	return RuntimeRefResponse{Ref: ref}, nil
}

func (r *runtimeService) ReadRefContent(ctx context.Context, id string) (RuntimeRefContentResponse, error) {
	store, err := r.ensureRuntimeRefStore(ctx)
	if err != nil {
		return RuntimeRefContentResponse{}, err
	}
	return store.ReadContent(ctx, id)
}
