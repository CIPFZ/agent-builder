package runtime

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
)

// ContextCompactionStatus returns the SQLite-authoritative session snapshot.
// compact.* events only tell clients when to invalidate and refetch it.
func (r *runtimeService) ContextCompactionStatus(ctx context.Context, sessionID string) (RuntimeContextCompactionStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeContextCompactionStatus{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	if sessionID == "" {
		return RuntimeContextCompactionStatus{}, errors.New("session id is required")
	}
	if err := r.ensureContextManager(ctx); err != nil {
		return RuntimeContextCompactionStatus{}, err
	}

	status := RuntimeContextCompactionStatus{SessionID: sessionID, UpdatedAt: time.Now().UTC().UnixMilli()}
	liveOperation := r.compactOperationActive(sessionID)
	boundaries, err := r.contextStore.ListBoundariesBySession(ctx, sessionID)
	if err != nil {
		return RuntimeContextCompactionStatus{}, err
	}
	for i := range boundaries {
		boundary := boundaries[i]
		if boundary.Kind != "full" && boundary.Kind != "session_memory" {
			continue
		}
		detail := runtimeCompactBoundaryDetailFromContext(boundary)
		switch boundary.Status {
		case contextmgr.ProjectionStatusStarted:
			if !liveOperation {
				boundary.Status = contextmgr.ProjectionStatusFailed
				boundary.Error = "runtime restarted before compact completed"
				boundary.CompletedAt = status.UpdatedAt
				if stored, storeErr := r.contextStore.UpsertBoundary(ctx, boundary); storeErr == nil {
					failed := runtimeCompactBoundaryDetailFromContext(stored)
					status.LatestFailed = &failed
				}
				continue
			}
			status.ActiveOperation = &RuntimeCompactOperation{
				ID: boundary.ID, SessionID: sessionID, TurnID: boundary.TurnID,
				Kind: boundary.Kind, Trigger: boundary.Trigger, Stage: "summarizing",
				Status: boundary.Status, StartedAt: boundary.CreatedAt,
				ElapsedMillis: maxInt64(0, status.UpdatedAt-boundary.CreatedAt),
			}
		case contextmgr.ProjectionStatusCompleted:
			status.LatestCompleted = &detail
		case contextmgr.ProjectionStatusFailed:
			status.LatestFailed = &detail
		}
	}
	if status.ActiveOperation == nil && liveOperation {
		status.ActiveOperation = &RuntimeCompactOperation{
			ID: "active:" + sessionID, SessionID: sessionID, Kind: "compact", Trigger: "runtime",
			Stage: "preparing", Status: contextmgr.ProjectionStatusStarted, StartedAt: status.UpdatedAt,
		}
	}
	if attempts, attemptErr := r.contextStore.ListReactiveAttemptsBySession(ctx, sessionID); attemptErr == nil && len(attempts) > 0 {
		latestAttempt := attempts[0]
		if status.LatestFailed != nil && latestAttempt.TurnID == status.LatestFailed.TurnID {
			status.LatestFailed.WillRetry = latestAttempt.WillRetry
			status.LatestFailed.CircuitOpen = latestAttempt.CircuitOpen
		}
		if status.ActiveOperation != nil && latestAttempt.TurnID == status.ActiveOperation.TurnID {
			status.ActiveOperation.WillRetry = latestAttempt.WillRetry
		}
	}
	if circuit, circuitErr := r.contextStore.GetCircuitState(ctx, sessionID); circuitErr == nil {
		status.CircuitOpen = circuit.Open
		status.ConsecutiveFailures = circuit.FailureCount
	} else if !errors.Is(circuitErr, sql.ErrNoRows) {
		return RuntimeContextCompactionStatus{}, circuitErr
	} else {
		status.CircuitOpen = r.isCompactCircuitOpen(sessionID)
	}
	if memory, memoryErr := r.contextStore.LatestSessionMemory(ctx, sessionID); memoryErr == nil {
		mapped := runtimeSessionMemoryRevision(memory)
		status.LatestSessionMemory = &mapped
	} else if !errors.Is(memoryErr, sql.ErrNoRows) {
		return RuntimeContextCompactionStatus{}, memoryErr
	}
	gov := r.contextGovernanceFor(ctx, sessionID, "")
	status.ResolvedPolicy = RuntimeResolvedCompactionPolicy{
		AutoCompactEnabled: gov.AutoCompactEnabled, AutoCompactPercent: gov.AutoCompactPercent,
		MicrocompactEnabled: gov.MicrocompactEnabled, MicrocompactIdleMinutes: gov.MicrocompactIdleMinutes,
		MicrocompactKeepRecent: gov.MicrocompactKeepRecent, SessionMemoryEnabled: gov.SessionMemoryEnabled,
		SummaryModel: firstNonEmpty(gov.SummaryModel, config.ContextGovernanceSummaryModelSession),
	}
	return status, nil
}

func runtimeSessionMemoryRevision(memory contextmgr.SessionMemoryRevision) RuntimeSessionMemoryRevision {
	return RuntimeSessionMemoryRevision{
		ID: memory.ID, SessionID: memory.SessionID, TurnID: memory.TurnID, Revision: memory.Revision,
		Status: memory.Status, LastSummarizedMessageID: memory.LastSummarizedMessageID,
		SourceMessageCount: memory.SourceMessageCount, SourceTokenEstimate: memory.SourceTokenEstimate,
		SourceToolCallCount: memory.SourceToolCallCount, Provider: memory.Provider, Model: memory.Model,
		CreatedAt: memory.CreatedAt, CompletedAt: memory.CompletedAt, Error: memory.Error,
	}
}
