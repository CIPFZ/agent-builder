package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/memory"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const runtimeProjectMemoryTokenBudget = 1800

func (r *runtimeService) ProjectMemories(ctx context.Context, projectID string) (RuntimeMemoryListResponse, error) {
	service, root, projectID, err := r.projectMemoryService(ctx, projectID)
	if err != nil {
		return RuntimeMemoryListResponse{}, err
	}
	records, err := service.List(ctx, true)
	if err != nil {
		return RuntimeMemoryListResponse{}, err
	}
	return RuntimeMemoryListResponse{ProjectID: projectID, Root: root, Records: runtimeMemoryRecords(records, false)}, nil
}

func (r *runtimeService) ProjectMemory(ctx context.Context, memoryID string) (RuntimeMemoryDetailResponse, error) {
	service, _, _, err := r.projectMemoryService(ctx, "")
	if err != nil {
		return RuntimeMemoryDetailResponse{}, err
	}
	record, err := service.Get(ctx, memoryID)
	if err != nil {
		return RuntimeMemoryDetailResponse{}, err
	}
	return RuntimeMemoryDetailResponse{Record: runtimeMemoryRecord(record, true)}, nil
}

func (r *runtimeService) CreateProjectMemory(ctx context.Context, req RuntimeMemoryCreateRequest) (RuntimeMemoryRecord, error) {
	service, _, projectID, err := r.projectMemoryService(ctx, req.ProjectID)
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	record, err := service.Create(ctx, memory.WriteRequest{
		RelativePath:    req.RelativePath,
		Type:            req.Type,
		Title:           req.Title,
		Description:     req.Description,
		Tags:            req.Tags,
		Content:         req.Content,
		SourceSessionID: req.SourceSessionID,
		SourceTurnID:    req.SourceTurnID,
		Confidence:      req.Confidence,
	})
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	r.publishMemoryRecordEvent(runtimeapi.EventMemoryRecordCreated, projectID, record, "")
	return runtimeMemoryRecord(record, false), nil
}

func (r *runtimeService) UpdateProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryUpdateRequest) (RuntimeMemoryRecord, error) {
	service, _, projectID, err := r.projectMemoryService(ctx, "")
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	record, err := service.Update(ctx, memoryID, memory.WriteRequest{
		RelativePath: req.RelativePath,
		Type:         req.Type,
		Title:        req.Title,
		Description:  req.Description,
		Tags:         req.Tags,
		Content:      req.Content,
	})
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	r.publishMemoryRecordEvent(runtimeapi.EventMemoryRecordUpdated, projectID, record, "")
	return runtimeMemoryRecord(record, false), nil
}

func (r *runtimeService) DisableProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryDisableRequest) (RuntimeMemoryRecord, error) {
	service, _, projectID, err := r.projectMemoryService(ctx, "")
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	record, err := service.Disable(ctx, memoryID, req.Enabled)
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	r.publishMemoryRecordEvent(runtimeapi.EventMemoryRecordDisabled, projectID, record, fmt.Sprintf("enabled=%t", req.Enabled))
	return runtimeMemoryRecord(record, false), nil
}

func (r *runtimeService) DeleteProjectMemory(ctx context.Context, memoryID string, req RuntimeMemoryDeleteRequest) (RuntimeMemoryRecord, error) {
	service, _, projectID, err := r.projectMemoryService(ctx, "")
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	record, err := service.Delete(ctx, memoryID)
	if err != nil {
		return RuntimeMemoryRecord{}, err
	}
	r.publishMemoryRecordEvent(runtimeapi.EventMemoryRecordDeleted, projectID, record, req.Reason)
	return runtimeMemoryRecord(record, false), nil
}

func (r *runtimeService) RefreshProjectMemoryIndex(ctx context.Context, projectID string) (RuntimeMemoryIndexResponse, error) {
	service, _, projectID, err := r.projectMemoryService(ctx, projectID)
	if err != nil {
		return RuntimeMemoryIndexResponse{}, err
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventMemoryIndexStarted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"project_id": projectID}})
	result, err := service.RebuildIndex(ctx)
	if err != nil {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventMemoryIndexFailed, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"project_id": projectID, "error": err.Error()}})
		return RuntimeMemoryIndexResponse{}, err
	}
	resp := runtimeMemoryIndexResponse(result)
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventMemoryIndexCompleted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"project_id": projectID, "indexed": resp.Indexed, "deleted": resp.Deleted, "failed": resp.Failed}})
	return resp, nil
}

func (r *runtimeService) ProjectMemoryDiagnostics(ctx context.Context, projectID string) (RuntimeMemoryDiagnostics, error) {
	service, root, projectID, err := r.projectMemoryService(ctx, projectID)
	if err != nil {
		return RuntimeMemoryDiagnostics{}, err
	}
	records, err := service.List(ctx, true)
	if err != nil {
		return RuntimeMemoryDiagnostics{}, err
	}
	diag := RuntimeMemoryDiagnostics{ProjectID: projectID, Root: root, RecordCount: len(records)}
	for _, record := range records {
		if record.DeletedAt != "" {
			diag.Deleted++
		} else if record.Enabled {
			diag.Enabled++
		} else {
			diag.Disabled++
		}
		if record.LastIndexedAt > diag.LastIndexed {
			diag.LastIndexed = record.LastIndexedAt
		}
	}
	return diag, nil
}

func (r *runtimeService) projectMemoryService(ctx context.Context, projectID string) (memory.Service, string, string, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return memory.Service{}, "", "", err
	}
	cfg, workspaceID, err := r.workspaceConfig(ctx)
	if err != nil {
		return memory.Service{}, "", "", err
	}
	projectID = strings.TrimSpace(firstNonEmpty(projectID, workspaceID))
	if projectID == "" {
		return memory.Service{}, "", "", errors.New("project id is required")
	}
	root, err := memory.Root(cfg.Config().Options.DataDirectory)
	if err != nil {
		return memory.Service{}, "", "", err
	}
	db, err := r.workspaceDB(ctx)
	if err != nil {
		return memory.Service{}, "", "", err
	}
	return memory.NewService(memory.NewStore(db), projectID, root), root, projectID, nil
}

func runtimeMemoryRecord(record memory.Record, includeContent bool) RuntimeMemoryRecord {
	out := RuntimeMemoryRecord{
		ID:             record.ID,
		ProjectID:      record.ProjectID,
		RelativePath:   filepath.ToSlash(record.RelativePath),
		AbsolutePath:   record.AbsolutePath,
		Type:           record.Type,
		Title:          record.Title,
		Description:    record.Description,
		Tags:           append([]string(nil), record.Tags...),
		Enabled:        record.Enabled,
		DeletedAt:      record.DeletedAt,
		ContentHash:    record.ContentHash,
		TokenEstimate:  record.TokenEstimate,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
		LastIndexedAt:  record.LastIndexedAt,
		LastInjectedAt: record.LastInjectedAt,
		Preview:        record.Preview,
	}
	if includeContent {
		out.Content = record.Content
	}
	return out
}

func runtimeMemoryRecords(records []memory.Record, includeContent bool) []RuntimeMemoryRecord {
	out := make([]RuntimeMemoryRecord, 0, len(records))
	for _, record := range records {
		out = append(out, runtimeMemoryRecord(record, includeContent))
	}
	return out
}

func runtimeMemoryIndexResponse(result memory.IndexResult) RuntimeMemoryIndexResponse {
	return RuntimeMemoryIndexResponse{
		ProjectID: result.ProjectID,
		Indexed:   result.Indexed,
		Deleted:   result.Deleted,
		Failed:    result.Failed,
		Issues:    runtimeMemoryIssues(result.Issues),
		StartedAt: result.StartedAt,
		EndedAt:   result.EndedAt,
	}
}

func runtimeMemoryIssues(issues []memory.ScanIssue) []RuntimeMemoryIssue {
	out := make([]RuntimeMemoryIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, RuntimeMemoryIssue{RelativePath: issue.RelativePath, Path: issue.Path, Error: issue.Error})
	}
	return out
}

func (r *runtimeService) publishMemoryRecordEvent(eventType, projectID string, record memory.Record, reason string) {
	payload := map[string]any{
		"project_id":     projectID,
		"memory_id":      record.ID,
		"relative_path":  record.RelativePath,
		"type":           record.Type,
		"title":          record.Title,
		"content_hash":   record.ContentHash,
		"enabled":        record.Enabled,
		"deleted_at":     record.DeletedAt,
		"token_estimate": record.TokenEstimate,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: eventType, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: payload})
}

func (r *runtimeService) injectProjectMemory(ctx context.Context, snapshot agentModelInputSnapshotLite, messages []fantasy.Message) ([]fantasy.Message, error) {
	service, _, projectID, err := r.projectMemoryService(ctx, "")
	if err != nil {
		return messages, nil
	}
	query := lastUserText(messages)
	results, err := service.Search(ctx, memory.SearchRequest{ProjectID: projectID, Query: query, Limit: 5, TokenBudget: runtimeProjectMemoryTokenBudget})
	if err != nil {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventMemoryRecordSkipped, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: snapshot.SessionID, TurnID: snapshot.TurnID, Payload: map[string]any{"project_id": projectID, "reason": "memory_search_failed", "error": err.Error()}})
		return messages, nil
	}
	if len(results) == 0 {
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventMemoryRecordSkipped, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: snapshot.SessionID, TurnID: snapshot.TurnID, Payload: map[string]any{"project_id": projectID, "reason": "no_matching_memory"}})
		return messages, nil
	}
	var b strings.Builder
	b.WriteString("<project_memory>\n")
	for _, result := range results {
		fmt.Fprintf(&b, "<memory id=%q title=%q type=%q>\n%s\n</memory>\n", result.Record.ID, result.Record.Title, result.Record.Type, strings.TrimSpace(result.Content))
		injection := memory.Injection{
			ProjectID:        projectID,
			SessionID:        snapshot.SessionID,
			TurnID:           snapshot.TurnID,
			MemoryID:         result.Record.ID,
			PromptAssemblyID: runtimePromptAssemblyID(snapshot.TurnID, snapshot.Step),
			TokenEstimate:    result.Record.TokenEstimate,
			SelectionReason:  result.SelectionReason,
		}
		_ = service.RecordInjection(ctx, injection)
		r.storeRuntimeEvent(runtimeapi.Event{ID: newRuntimeEventID(), Type: runtimeapi.EventMemoryRecordInjected, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SessionID: snapshot.SessionID, TurnID: snapshot.TurnID, Payload: map[string]any{"project_id": projectID, "memory_id": result.Record.ID, "relative_path": result.Record.RelativePath, "content_hash": result.Record.ContentHash, "token_estimate": result.Record.TokenEstimate, "selection_reason": result.SelectionReason}})
	}
	b.WriteString("</project_memory>")
	injected := fantasy.NewSystemMessage(b.String())
	return append([]fantasy.Message{injected}, messages...), nil
}

type agentModelInputSnapshotLite struct {
	SessionID string
	TurnID    string
	Step      int
}

func lastUserText(messages []fantasy.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != fantasy.MessageRoleUser {
			continue
		}
		var b strings.Builder
		for _, part := range messages[i].Content {
			if text, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
				b.WriteString(text.Text)
				b.WriteByte(' ')
			}
		}
		return b.String()
	}
	return ""
}

// memoryInjectionTokensForTurn sums the token estimates of project memory
// injections recorded for the given turn. Used by the context usage breakdown
// to fold project memory into the memory category.
func (r *runtimeService) memoryInjectionTokensForTurn(ctx context.Context, sessionID, turnID string) int {
	if strings.TrimSpace(turnID) == "" || r.turns.db == nil {
		return 0
	}
	injections, err := memory.NewStore(r.turns.db).ListInjectionsByTurn(ctx, sessionID, turnID)
	if err != nil {
		return 0
	}
	total := 0
	for _, injection := range injections {
		total += injection.TokenEstimate
	}
	return total
}

func (r *runtimeService) memoryContextSourcesForTurn(ctx context.Context, sessionID, turnID string) []RuntimeContextSource {
	if r.turns.db == nil {
		return nil
	}
	store := memory.NewStore(r.turns.db)
	injections, err := store.ListInjectionsByTurn(ctx, sessionID, turnID)
	if err != nil {
		return []RuntimeContextSource{{
			ID:             "project-memory:" + turnID,
			Kind:           "project_memory",
			Name:           "Project memory",
			Scope:          "project",
			Enabled:        true,
			State:          capabilityStateFailed,
			Reason:         "memory_injection_lookup_failed",
			Error:          err.Error(),
			ContentSummary: "Runtime could not load project memory injection metadata.",
		}}
	}
	sources := make([]RuntimeContextSource, 0, len(injections))
	for _, injection := range injections {
		record, err := store.Get(ctx, injection.MemoryID)
		if err != nil {
			state := capabilityStateFailed
			reason := "memory_record_missing"
			if errors.Is(err, sql.ErrNoRows) {
				reason = "memory_record_not_found"
			}
			sources = append(sources, RuntimeContextSource{ID: "project-memory:" + injection.MemoryID, Kind: "project_memory", Name: injection.MemoryID, Scope: "project", Enabled: true, State: state, Reason: reason, Error: err.Error()})
			continue
		}
		sources = append(sources, RuntimeContextSource{
			ID:             "project-memory:" + record.ID,
			Kind:           "project_memory",
			Name:           record.Title,
			Path:           record.RelativePath,
			Scope:          "project",
			Enabled:        record.Enabled,
			State:          capabilityStateLoaded,
			Reason:         injection.SelectionReason,
			ContentSummary: record.Description,
			TokenEstimate:  injection.TokenEstimate,
			LoadedAt:       injection.InjectedAt,
			SizeBytes:      record.SizeBytes,
			MTimeUnix:      record.MTimeUnix,
			ContentHash:    record.ContentHash,
		})
	}
	return sources
}
