package runtime

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

const (
	runtimeActiveSessionStatusLimit      = 500
	runtimeActiveSessionStatusFieldBytes = 256
)

func activeSessionStatusFromTurn(turn RuntimeTurn) RuntimeActiveSessionStatus {
	phase, label := "model", "Generating response"
	if turn.Status == turnStatusQueued {
		phase, label = "scheduler", "Waiting for model capacity"
	} else if turn.Status == turnStatusWaitingPermission {
		phase, label = "permission", "Waiting for approval"
	} else if turn.Status == turnStatusCancelling {
		phase, label = "cancelling", "Cancelling"
	}
	return RuntimeActiveSessionStatus{
		SessionID: turn.SessionID, Status: turn.Status, Phase: phase,
		ProgressLabel: label, ActiveTurnID: turn.ID, UpdatedAt: turn.StartedAt,
	}
}

func (r *runtimeService) activeSessionStatusesLocked() []RuntimeActiveSessionStatus {
	result := make([]RuntimeActiveSessionStatus, 0, len(r.activeSessionStatuses))
	for _, status := range r.activeSessionStatuses {
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt == result[j].UpdatedAt {
			return result[i].SessionID < result[j].SessionID
		}
		return result[i].UpdatedAt > result[j].UpdatedAt
	})
	return result
}

func (r *runtimeService) reduceActiveSessionStatusLocked(event RuntimeEvent) {
	if strings.TrimSpace(event.SessionID) == "" {
		return
	}
	status, exists := r.activeSessionStatuses[event.SessionID]
	if exists && event.Type != runtimeapi.EventTurnStarted && event.TurnID != "" && status.ActiveTurnID != "" && event.TurnID != status.ActiveTurnID {
		// A terminal or progress event from an older Turn must not clear or
		// overwrite the status of a newer Turn in the same Session.
		return
	}
	status.SessionID = event.SessionID
	status.ActiveTurnID = firstNonEmpty(event.TurnID, status.ActiveTurnID)
	status.UpdatedAt = parseRuntimeEventMillis(event.CreatedAt)
	if status.UpdatedAt == 0 {
		status.UpdatedAt = time.Now().UnixMilli()
	}
	if projectID := runtimeStatusPayloadString(event.Payload, "project_id", "projectId"); projectID != "" {
		status.ProjectID = projectID
	}

	switch event.Type {
	case runtimeapi.EventTurnStarted:
		status.Status, status.Phase, status.ProgressLabel, status.Unread = turnStatusRunning, "model", "Generating response", false
	case runtimeapi.EventPermissionRequested:
		status.Status, status.Phase, status.ProgressLabel = turnStatusWaitingPermission, "permission", "Waiting for approval"
	case runtimeapi.EventPermissionDecided, runtimeapi.EventPermissionPolicyApplied:
		status.Status, status.Phase, status.ProgressLabel = turnStatusRunning, "model", "Generating response"
	case runtimeapi.EventToolCallStarted:
		status.Status, status.Phase = turnStatusRunning, "tool"
		status.ProgressLabel = firstNonEmpty(runtimeStatusPayloadString(event.Payload, "name", "tool"), "Running tool")
	case runtimeapi.EventCompactStarted:
		status.Status, status.Phase, status.ProgressLabel = turnStatusRunning, "compact", "Compacting context"
	case runtimeapi.EventTaskStarted, runtimeapi.EventTaskProgress:
		status.Status, status.Phase = turnStatusRunning, "task"
		status.ProgressLabel = firstNonEmpty(runtimeStatusPayloadString(event.Payload, "label", "name", "status"), "Running task")
	case runtimeapi.EventTurnFailed, runtimeapi.EventTurnInterrupted:
		status.Status, status.Phase, status.ProgressLabel, status.Unread = "attention", "attention", firstNonEmpty(runtimeStatusPayloadString(event.Payload, "error"), "Needs attention"), true
	case runtimeapi.EventTurnCompleted, runtimeapi.EventTurnCancelled, runtimeapi.EventSessionDeleted:
		delete(r.activeSessionStatuses, event.SessionID)
		return
	default:
		if !exists {
			return
		}
	}
	r.upsertActiveSessionStatusLocked(status)
}

func (r *runtimeService) upsertActiveSessionStatusLocked(status RuntimeActiveSessionStatus) {
	status.SessionID = truncateRuntimeStatusField(status.SessionID)
	if status.SessionID == "" {
		return
	}
	status.ProjectID = truncateRuntimeStatusField(status.ProjectID)
	status.Status = truncateRuntimeStatusField(status.Status)
	status.Phase = truncateRuntimeStatusField(status.Phase)
	status.ProgressLabel = truncateRuntimeStatusField(status.ProgressLabel)
	status.ActiveTurnID = truncateRuntimeStatusField(status.ActiveTurnID)
	if status.UpdatedAt == 0 {
		status.UpdatedAt = time.Now().UnixMilli()
	}
	if previous, ok := r.activeSessionStatuses[status.SessionID]; ok && activeSessionStatusEqual(previous, status) {
		return
	}
	r.activeSessionRevision++
	status.Revision = r.activeSessionRevision
	if r.activeSessionStatuses == nil {
		r.activeSessionStatuses = make(map[string]RuntimeActiveSessionStatus)
	}
	r.activeSessionStatuses[status.SessionID] = status
	for len(r.activeSessionStatuses) > runtimeActiveSessionStatusLimit {
		r.evictOldestActiveSessionStatusLocked()
	}
}

func activeSessionStatusEqual(a, b RuntimeActiveSessionStatus) bool {
	return a.SessionID == b.SessionID && a.ProjectID == b.ProjectID && a.Status == b.Status &&
		a.Phase == b.Phase && a.ProgressLabel == b.ProgressLabel && a.ActiveTurnID == b.ActiveTurnID &&
		a.UpdatedAt == b.UpdatedAt && a.Unread == b.Unread
}

func (r *runtimeService) evictOldestActiveSessionStatusLocked() {
	oldestID := ""
	oldestAt := int64(0)
	for id, status := range r.activeSessionStatuses {
		if status.Status == "attention" {
			continue
		}
		if oldestID == "" || status.UpdatedAt < oldestAt || (status.UpdatedAt == oldestAt && id < oldestID) {
			oldestID, oldestAt = id, status.UpdatedAt
		}
	}
	if oldestID == "" {
		for id, status := range r.activeSessionStatuses {
			if oldestID == "" || status.UpdatedAt < oldestAt || (status.UpdatedAt == oldestAt && id < oldestID) {
				oldestID, oldestAt = id, status.UpdatedAt
			}
		}
	}
	delete(r.activeSessionStatuses, oldestID)
}

func runtimeStatusPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateRuntimeStatusField(value string) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "")
	if len(value) <= runtimeActiveSessionStatusFieldBytes {
		return value
	}
	value = value[:runtimeActiveSessionStatusFieldBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
