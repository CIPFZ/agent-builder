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

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	runtimeObjectKindOutput                = "output"
	runtimeObjectKindInput                 = "input"
	runtimeObjectKindArtifact              = "artifact"
	runtimeObjectKindDiff                  = "diff"
	runtimeObjectKindCompactOriginalOutput = "compact_original_output"
	runtimeObjectKindShellJobOutput        = "shell_job_output"
	runtimeObjectKindTaskArtifact          = "task_artifact"

	runtimeObjectStorageInline = "inline"
	runtimeObjectStorageFile   = "file"

	runtimeObjectRedactionSafe     = "safe"
	runtimeObjectRedactionRedacted = "redacted"
	runtimeObjectRedactionUnsafe   = "unsafe"

	runtimeObjectInlineLimit = 8 * 1024
)

var errRuntimeObjectNotFound = errors.New("runtime object not found")

type runtimeObjectStore struct {
	db      *sql.DB
	dataDir string
}

type runtimeObjectCreateRequest struct {
	ID                string
	ProjectID         string
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

func newRuntimeObjectStore(db *sql.DB, dataDir string) runtimeObjectStore {
	return runtimeObjectStore{db: db, dataDir: filepath.Clean(dataDir)}
}

func (s runtimeObjectStore) Create(ctx context.Context, req runtimeObjectCreateRequest) (RuntimeObject, error) {
	if s.db == nil {
		return RuntimeObject{}, errors.New("runtime object database is not available")
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Kind = strings.TrimSpace(req.Kind)
	if req.ProjectID == "" {
		return RuntimeObject{}, errors.New("runtime object project id is required")
	}
	if req.SessionID == "" {
		return RuntimeObject{}, errors.New("runtime object session id is required")
	}
	if req.Kind == "" {
		return RuntimeObject{}, errors.New("runtime object kind is required")
	}
	if req.ID == "" {
		req.ID = newRuntimeObjectID()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	redactedPayload := redactRuntimeString("", string(req.Payload))
	redactionStatus := runtimeObjectRedactionSafe
	if redactedPayload != string(req.Payload) {
		redactionStatus = runtimeObjectRedactionUnsafe
	}
	ref := RuntimeObject{
		ID:                req.ID,
		URI:               "runtime://objects/" + req.ID,
		ProjectID:         req.ProjectID,
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
		StorageKind:       runtimeObjectStorageInline,
		RedactionStatus:   redactionStatus,
		CreatedAt:         req.CreatedAt.UnixMilli(),
		CanReadContent:    redactionStatus == runtimeObjectRedactionSafe,
	}
	if req.StoragePath != "" {
		ref.StorageKind = firstNonEmpty(req.StorageKind, runtimeObjectStorageFile)
		ref.StoragePath = filepath.ToSlash(req.StoragePath)
		if _, err := s.resolveStoragePath(ref.ProjectID, ref.StoragePath); err != nil {
			return RuntimeObject{}, err
		}
	} else if len(req.Payload) > runtimeObjectInlineLimit {
		ref.StorageKind = runtimeObjectStorageFile
		rel, abs, err := s.storagePathForID(ref.ProjectID, ref.ID)
		if err != nil {
			return RuntimeObject{}, err
		}
		// TODO(runtime): add reference-aware garbage collection once session
		// retention policies exist. Refs are intentionally durable today.
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return RuntimeObject{}, fmt.Errorf("failed to create runtime object storage: %w", err)
		}
		if err := os.WriteFile(abs, req.Payload, 0o600); err != nil {
			return RuntimeObject{}, fmt.Errorf("failed to write runtime object payload: %w", err)
		}
		ref.StoragePath = rel
	} else {
		ref.InlinePayload = string(req.Payload)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO objects (
    id, uri, project_id, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    uri = excluded.uri,
	project_id = excluded.project_id,
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
		ref.ProjectID,
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
		return RuntimeObject{}, fmt.Errorf("failed to store runtime object: %w", err)
	}
	return s.Get(ctx, ref.ID)
}

func (s runtimeObjectStore) Get(ctx context.Context, idOrURI string) (RuntimeObject, error) {
	if s.db == nil {
		return RuntimeObject{}, errors.New("runtime object database is not available")
	}
	idOrURI = normalizeRuntimeObjectID(idOrURI)
	row := s.db.QueryRowContext(ctx, `
SELECT id, uri, project_id, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM objects
WHERE id = ? OR uri = ?`, idOrURI, idOrURI)
	ref, err := scanRuntimeObject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeObject{}, errRuntimeObjectNotFound
	}
	if err != nil {
		return RuntimeObject{}, err
	}
	return runtimeObjectMetadataOnly(ref), nil
}

func (s runtimeObjectStore) List(ctx context.Context, req RuntimeObjectListRequest) ([]RuntimeObject, error) {
	if s.db == nil {
		return nil, errors.New("runtime object database is not available")
	}
	query := `
SELECT id, uri, project_id, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM objects`
	var clauses []string
	var args []any
	if req.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, req.ProjectID)
	}
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
		return nil, fmt.Errorf("failed to list runtime objects: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var refs []RuntimeObject
	for rows.Next() {
		ref, err := scanRuntimeObject(rows)
		if err != nil {
			return nil, err
		}
		refs = append(refs, runtimeObjectMetadataOnly(ref))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runtime objects: %w", err)
	}
	return refs, nil
}

func (s runtimeObjectStore) ReadContent(ctx context.Context, idOrURI string) (RuntimeObjectContentResponse, error) {
	ref, err := s.Get(ctx, idOrURI)
	if err != nil {
		return RuntimeObjectContentResponse{}, err
	}
	fullRef, err := s.getWithPayload(ctx, idOrURI)
	if err != nil {
		return RuntimeObjectContentResponse{}, err
	}
	content := fullRef.InlinePayload
	if ref.StorageKind == runtimeObjectStorageFile {
		path, err := s.resolveStoragePath(ref.ProjectID, ref.StoragePath)
		if err != nil {
			return RuntimeObjectContentResponse{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return RuntimeObjectContentResponse{}, fmt.Errorf("failed to read runtime object payload: %w", err)
		}
		content = string(data)
	}
	redacted := false
	if ref.RedactionStatus != runtimeObjectRedactionSafe {
		content = redactRuntimeString("", content)
		redacted = true
		ref.CanReadContent = false
	} else {
		ref.CanReadContent = true
	}
	return RuntimeObjectContentResponse{Object: ref, Content: content, Redacted: redacted}, nil
}

func (s runtimeObjectStore) getWithPayload(ctx context.Context, idOrURI string) (RuntimeObject, error) {
	idOrURI = normalizeRuntimeObjectID(idOrURI)
	row := s.db.QueryRowContext(ctx, `
SELECT id, uri, project_id, session_id, turn_id, tool_call_id, task_id, sandbox_decision_id,
    sandbox_mode, sandbox_status, kind, media_type,
    content_type, size_bytes, estimated_tokens, preview, summary,
    storage_kind, storage_path, inline_payload, redaction_status, created_at
FROM objects
WHERE id = ? OR uri = ?`, idOrURI, idOrURI)
	ref, err := scanRuntimeObject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeObject{}, errRuntimeObjectNotFound
	}
	return ref, err
}

func (s runtimeObjectStore) storagePathForID(projectID, id string) (string, string, error) {
	safeID := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(id)
	if safeID == "" || safeID == "." || safeID == ".." {
		return "", "", errors.New("invalid runtime object id")
	}
	rel := filepath.ToSlash(filepath.Join(safeID[:minInt(2, len(safeID))], safeID+".blob"))
	abs, err := s.resolveStoragePath(projectID, rel)
	return rel, abs, err
}

func (s runtimeObjectStore) resolveStoragePath(projectID, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", errors.New("runtime object storage path is required")
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, ":") {
		return "", errors.New("runtime object storage path must be relative")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", errors.New("runtime object storage path traversal rejected")
	}
	dataLayout, err := newApplicationDataLayout(s.dataDir)
	if err != nil {
		return "", err
	}
	projectLayout, err := dataLayout.Project(projectID)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(projectLayout.ObjectsDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanRel))
	if err != nil {
		return "", err
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(filepath.Separator)) {
		return "", errors.New("runtime object storage path escapes runtime data directory")
	}
	return pathAbs, nil
}

type runtimeObjectScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeObject(scanner runtimeObjectScanner) (RuntimeObject, error) {
	var ref RuntimeObject
	var turnID, toolCallID, taskID, sandboxDecisionID, sandboxMode, sandboxStatus, mediaType, contentType, previewText, summary, storagePath, inlinePayload sql.NullString
	if err := scanner.Scan(
		&ref.ID,
		&ref.URI,
		&ref.ProjectID,
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
		return RuntimeObject{}, err
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
	ref.CanReadContent = ref.RedactionStatus == runtimeObjectRedactionSafe
	return ref, nil
}

func newRuntimeObjectID() string {
	return "object_" + newRequestID()
}

func normalizeRuntimeObjectID(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "runtime://objects/")
}

func (r *runtimeService) ensureRuntimeObjectStore(ctx context.Context) (runtimeObjectStore, error) {
	if r.objects.db != nil {
		return r.objects, nil
	}
	if r.turns.db != nil {
		dataDir := os.TempDir()
		r.objects = newRuntimeObjectStore(r.turns.db, dataDir)
		return r.objects, nil
	}
	// Never bootstrap the workspace just to open a ref store — see
	// workspaceDBIfStarted for why. Ref creation is a side effect of
	// tool result recording, so silently no-op when no workspace has
	// been attached: callers (createRuntimeObject → createToolOutputRefs)
	// already ignore Create errors and skip persistence.
	db, err := r.workspaceDBIfStarted(ctx)
	if err != nil {
		return runtimeObjectStore{}, err
	}
	if db == nil {
		return runtimeObjectStore{}, errors.New("runtime object database is not available")
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
	r.objects = newRuntimeObjectStore(db, dataDir)
	return r.objects, nil
}

func (r *runtimeService) createRuntimeObject(ctx context.Context, req runtimeObjectCreateRequest) (RuntimeObject, error) {
	r.mu.Lock()
	projectID := r.activeProjectID
	workspaceID := ""
	if r.workspace != nil {
		workspaceID = r.workspace.ID
	}
	r.mu.Unlock()
	req.ProjectID = firstNonEmpty(req.ProjectID, projectID, workspaceID)
	store, err := r.ensureRuntimeObjectStore(ctx)
	if err != nil {
		return RuntimeObject{}, err
	}
	ref, err := store.Create(ctx, req)
	if err != nil {
		return RuntimeObject{}, err
	}
	r.publishRuntimeObjectCreated(ref)
	return ref, nil
}

func (r *runtimeService) publishRuntimeObjectCreated(ref RuntimeObject) {
	payload := runtimeObjectEventPayload(ref)
	eventType := runtimeapi.EventOutputRefCreated
	if ref.Kind == runtimeObjectKindArtifact || ref.Kind == runtimeObjectKindTaskArtifact {
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
	if ref.ToolCallID != "" && (ref.Kind == runtimeObjectKindOutput || ref.Kind == runtimeObjectKindShellJobOutput || ref.Kind == runtimeObjectKindDiff || ref.Kind == runtimeObjectKindCompactOriginalOutput) {
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

func runtimeObjectEventPayload(ref RuntimeObject) map[string]any {
	return map[string]any{
		"id":                  ref.ID,
		"uri":                 ref.URI,
		"project_id":          ref.ProjectID,
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

func runtimeObjectMetadataOnly(ref RuntimeObject) RuntimeObject {
	ref.InlinePayload = ""
	return ref
}

func (r *runtimeService) Objects(ctx context.Context, req RuntimeObjectListRequest) (RuntimeObjectsResponse, error) {
	store, err := r.ensureRuntimeObjectStore(ctx)
	if err != nil {
		return RuntimeObjectsResponse{}, err
	}
	objects, err := store.List(ctx, req)
	if err != nil {
		return RuntimeObjectsResponse{}, err
	}
	return RuntimeObjectsResponse{Objects: objects}, nil
}

func (r *runtimeService) Object(ctx context.Context, id string) (RuntimeObjectResponse, error) {
	store, err := r.ensureRuntimeObjectStore(ctx)
	if err != nil {
		return RuntimeObjectResponse{}, err
	}
	ref, err := store.Get(ctx, id)
	if err != nil {
		return RuntimeObjectResponse{}, err
	}
	return RuntimeObjectResponse{Object: ref}, nil
}

func (r *runtimeService) ReadObjectContent(ctx context.Context, id string) (RuntimeObjectContentResponse, error) {
	store, err := r.ensureRuntimeObjectStore(ctx)
	if err != nil {
		return RuntimeObjectContentResponse{}, err
	}
	return store.ReadContent(ctx, id)
}
