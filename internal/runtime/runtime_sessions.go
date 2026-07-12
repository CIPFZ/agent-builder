package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/session"
)

func (r *runtimeService) Sessions(ctx context.Context) (RuntimeSessionsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionsResponse{}, err
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	activeID := r.sessionID
	r.mu.Unlock()

	sessions, err := r.runtime.ListSessions(ctx, wsID)
	if err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to list Agent Builder sessions: %w", err)
	}
	return RuntimeSessionsResponse{Sessions: toRuntimeSessions(sessions, activeID, wsID)}, nil
}

func (r *runtimeService) Session(ctx context.Context, sessionID string) (RuntimeSessionResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionResponse{}, errors.New("session id is required")
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	activeID := r.sessionID
	r.mu.Unlock()

	sess, err := r.runtime.GetSession(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeSessionResponse{}, fmt.Errorf("failed to read Agent Builder session: %w", err)
	}
	return RuntimeSessionResponse{Session: toRuntimeSession(sess, activeID, wsID)}, nil
}

func (r *runtimeService) CreateSession(ctx context.Context, req RuntimeSessionCreateRequest) (RuntimeSessionResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionResponse{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "New chat"
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	activeProjectID := r.activeProjectID
	r.mu.Unlock()
	projectID, scope, workdir, canonicalWorkdir, workdirExists, err := r.normalizeSessionCreationContext(ctx, req.ProjectID, req.Scope, req.Workdir, activeProjectID)
	if err != nil {
		return RuntimeSessionResponse{}, err
	}
	sess, err := r.runtime.CreateSessionWithScopeAndWorkdir(ctx, wsID, title, projectID, scope, workdir, canonicalWorkdir, workdirExists)
	if err != nil {
		return RuntimeSessionResponse{}, fmt.Errorf("failed to create Agent Builder session: %w", err)
	}
	r.mu.Lock()
	r.sessionID = sess.ID
	r.mu.Unlock()
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventSessionCreated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sess.ID,
		Payload: map[string]any{
			"title":  sess.Title,
			"active": true,
		},
	})
	return RuntimeSessionResponse{Session: toRuntimeSession(sess, sess.ID, wsID)}, nil
}

func (r *runtimeService) SelectSession(ctx context.Context, sessionID string) (RuntimeStatus, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeStatus{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeStatus{}, errors.New("session id is required")
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	sess, err := r.runtime.GetSession(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to select Agent Builder session: %w", err)
	}
	r.mu.Lock()
	r.sessionID = sess.ID
	r.mu.Unlock()
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventSessionUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sess.ID,
		Payload: map[string]any{
			"title":  sess.Title,
			"active": true,
		},
	})
	return r.Status(ctx)
}

func (r *runtimeService) RenameSession(ctx context.Context, req RuntimeSessionUpdateRequest) (RuntimeSessionsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionsResponse{}, err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	title := strings.TrimSpace(req.Title)
	if sessionID == "" {
		return RuntimeSessionsResponse{}, errors.New("session id is required")
	}
	if title == "" {
		return RuntimeSessionsResponse{}, errors.New("session title is required")
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()

	sess, err := r.runtime.GetSession(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to read Agent Builder session: %w", err)
	}
	sess.Title = title
	if sess.Scope == "" {
		sess.Scope = "project"
	}
	if sess.Scope == "project" && sess.ProjectID == "" {
		r.mu.Lock()
		sess.ProjectID = r.activeProjectID
		r.mu.Unlock()
	}
	if _, err := r.runtime.SaveSession(ctx, wsID, sess); err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to rename Agent Builder session: %w", err)
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventSessionUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Payload: map[string]any{
			"title": title,
		},
	})
	return r.Sessions(ctx)
}

func (r *runtimeService) DeleteSession(ctx context.Context, sessionID string) (RuntimeSessionsResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionsResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionsResponse{}, errors.New("session id is required")
	}
	r.mu.Lock()
	activeID := r.sessionID
	r.mu.Unlock()
	canonicalBeforeDelete, err := r.buildSessionConversationSnapshotV2(ctx, sessionID, RuntimeCanonicalConversationSnapshotRequest{})
	if err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to capture canonical session tombstones: %w", err)
	}
	tombstones := canonicalConversationTombstoneRefs(canonicalBeforeDelete)

	r.closeRuntimeTerminalsForSession(sessionID, "closed", "session deleted")
	conn, err := r.workspaceDB(ctx)
	if err != nil {
		return RuntimeSessionsResponse{}, err
	}
	if runtimeDeleteMode(ctx, conn) == runtimeDeleteModeSoft {
		now := time.Now().UnixMilli()
		if _, err := conn.ExecContext(ctx, `UPDATE sessions SET deleted_at = ?, status = 'deleted' WHERE id = ? AND deleted_at IS NULL`, now, sessionID); err != nil {
			return RuntimeSessionsResponse{}, fmt.Errorf("failed to soft delete Agent Builder session: %w", err)
		}
	} else if err := purgeRuntimeSession(ctx, conn, sessionID, r.refs.root); err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to purge Agent Builder session: %w", err)
	}
	if sessionID == activeID {
		r.mu.Lock()
		r.sessionID = ""
		r.mu.Unlock()
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventSessionDeleted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Payload:   map[string]any{"entity_refs": tombstones},
	})
	return r.Sessions(ctx)
}

func (r *runtimeService) SessionMessages(ctx context.Context, sessionID string) (RuntimeMessagesResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeMessagesResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeMessagesResponse{}, errors.New("session id is required")
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	return r.sessionMessages(ctx, wsID, sessionID)
}

func (r *runtimeService) SessionActivity(ctx context.Context, sessionID string) (RuntimeSessionActivityResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionActivityResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionActivityResponse{}, errors.New("session id is required")
	}
	activity, err := r.hydrateSessionActivity(ctx, sessionID, "", 0)
	if err != nil {
		return RuntimeSessionActivityResponse{}, err
	}
	return RuntimeSessionActivityResponse{
		SessionID:   activity.SessionID,
		Messages:    activity.Messages,
		Turns:       activity.Turns,
		ToolCalls:   activity.ToolCalls,
		Permissions: activity.Permissions,
		Policy:      activity.Policy,
	}, nil
}

func (r *runtimeService) SessionActivityWindow(ctx context.Context, sessionID string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	return r.SessionActivityCursorWindow(ctx, sessionID, "", limit)
}

func (r *runtimeService) SessionActivityCursorWindow(ctx context.Context, sessionID string, cursor string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeSessionActivityWindowResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionActivityWindowResponse{}, errors.New("session id is required")
	}
	return r.hydrateSessionActivity(ctx, sessionID, strings.TrimSpace(cursor), limit)
}

func (r *runtimeService) TurnActivity(ctx context.Context, turnID string) (RuntimeTurnActivityResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeTurnActivityResponse{}, err
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeTurnActivityResponse{}, errors.New("turn id is required")
	}
	turn, err := r.runtimeTurnForActivity(ctx, turnID)
	if err != nil {
		return RuntimeTurnActivityResponse{}, err
	}
	policy, err := r.activityPolicy(ctx)
	if err != nil {
		return RuntimeTurnActivityResponse{}, err
	}
	activity, err := r.hydrateActivityForSelection(ctx, turn.SessionID, []RuntimeTurn{turn}, policy, runtimeActivitySelection{
		turnIDs: map[string]struct{}{turn.ID: {}},
	})
	if err != nil {
		return RuntimeTurnActivityResponse{}, err
	}
	return RuntimeTurnActivityResponse{
		SessionID:   activity.SessionID,
		TurnID:      turn.ID,
		Messages:    activity.Messages,
		Turns:       activity.Turns,
		ToolCalls:   activity.ToolCalls,
		Permissions: activity.Permissions,
		Events:      activity.Events,
		Policy:      activity.Policy,
	}, nil
}

func (r *runtimeService) hydrateSessionActivity(ctx context.Context, sessionID, cursor string, limit int) (RuntimeSessionActivityWindowResponse, error) {
	policy, err := r.activityPolicy(ctx)
	if err != nil {
		return RuntimeSessionActivityWindowResponse{}, err
	}
	var turns []RuntimeTurn
	if r.turns.db != nil {
		turns, err = r.turns.ListBySession(ctx, sessionID)
		if err != nil {
			return RuntimeSessionActivityWindowResponse{}, err
		}
	}
	selection := runtimeActivitySelection{includeAll: limit <= 0 && cursor == ""}
	window := RuntimeActivityWindow{Limit: limit, Cursor: cursor, FromStart: true, ToEnd: true}
	if selection.includeAll {
		selection.turnIDs = runtimeActivityTurnIDSet(turns)
	} else {
		selection, window, err = r.selectActivityWindow(ctx, sessionID, turns, cursor, limit)
		if err != nil {
			return RuntimeSessionActivityWindowResponse{}, err
		}
	}
	activity, err := r.hydrateActivityForSelection(ctx, sessionID, turns, policy, selection)
	if err != nil {
		return RuntimeSessionActivityWindowResponse{}, err
	}
	activity.Window = window
	return activity, nil
}

func (r *runtimeService) activityPolicy(ctx context.Context) (RuntimePolicy, error) {
	r.mu.Lock()
	policy := r.policy
	r.mu.Unlock()
	if policy.Mode != "" {
		return policy, nil
	}
	policyResp, err := r.GetPolicy(ctx)
	if err == nil {
		return policyResp.Policy, nil
	}
	return defaultRuntimePolicy(), nil
}

func (r *runtimeService) runtimeTurnForActivity(ctx context.Context, turnID string) (RuntimeTurn, error) {
	r.mu.Lock()
	state, active := r.requests[turnID]
	r.mu.Unlock()
	if active && !state.Finished {
		return runtimeTurnFromRequestState(turnID, state), nil
	}
	turn, err := r.turns.Get(ctx, turnID)
	if err != nil {
		return RuntimeTurn{}, fmt.Errorf("turn %s was not found: %w", turnID, err)
	}
	return turn, nil
}

type runtimeActivityEvidence struct {
	kind       string
	id         string
	cursor     string
	rank       int
	timestamp  int64
	turnID     string
	messageID  string
	toolCallID string
	permID     string
	sequence   int64
}

type runtimeActivitySelection struct {
	includeAll     bool
	turnIDs        map[string]struct{}
	messageIDs     map[string]struct{}
	toolCallIDs    map[string]struct{}
	permissionIDs  map[string]struct{}
	eventSequences map[int64]struct{}
}

func (r *runtimeService) selectActivityWindow(ctx context.Context, sessionID string, turns []RuntimeTurn, cursor string, limit int) (runtimeActivitySelection, RuntimeActivityWindow, error) {
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	messages, err := r.sessionMessages(ctx, wsID, sessionID)
	if err != nil {
		return runtimeActivitySelection{}, RuntimeActivityWindow{}, err
	}
	messageTurnIDs := runtimeActivityMessageTurnIDs(turns)
	evidence := make([]runtimeActivityEvidence, 0, len(messages.Messages)+len(turns))
	for _, msg := range messages.Messages {
		entry := runtimeActivityEvidence{kind: "message", id: msg.ID, rank: 10, timestamp: msg.CreatedAt, messageID: msg.ID, turnID: messageTurnIDs[msg.ID]}
		entry.cursor = runtimeActivityCursor(entry)
		evidence = append(evidence, entry)
	}
	for _, turn := range turns {
		entry := runtimeActivityEvidence{kind: "turn", id: turn.ID, rank: 20, timestamp: firstPositiveInt64(turn.StartedAt, turn.FinishedAt), turnID: turn.ID}
		entry.cursor = runtimeActivityCursor(entry)
		evidence = append(evidence, entry)
	}
	if r.toolCalls != nil {
		for _, turn := range turns {
			calls, err := r.toolCalls.ListCalls(ctx, turn.ID)
			if err != nil {
				continue
			}
			for _, call := range calls {
				runtimeCall := toRuntimeToolCall(call)
				entry := runtimeActivityEvidence{kind: "tool", id: runtimeCall.ID, rank: 30, timestamp: firstPositiveInt64(runtimeCall.StartedAt, runtimeCall.FinishedAt), turnID: runtimeCall.TurnID, messageID: runtimeCall.MessageID, toolCallID: runtimeCall.ID}
				entry.cursor = runtimeActivityCursor(entry)
				evidence = append(evidence, entry)
			}
		}
	}
	if r.permissionStore.db != nil {
		if _, err := r.reconcilePendingPermissions(ctx); err != nil {
			return runtimeActivitySelection{}, RuntimeActivityWindow{}, err
		}
		permissions, err := r.permissionStore.ListBySession(ctx, sessionID)
		if err != nil {
			return runtimeActivitySelection{}, RuntimeActivityWindow{}, err
		}
		for _, perm := range permissions {
			entry := runtimeActivityEvidence{kind: "permission", id: perm.ID, rank: 40, timestamp: firstPositiveInt64(perm.CreatedAt, perm.DecidedAt), turnID: perm.TurnID, toolCallID: perm.ToolCallID, permID: perm.ID}
			entry.cursor = runtimeActivityCursor(entry)
			evidence = append(evidence, entry)
		}
	} else {
		r.mu.Lock()
		for _, pending := range r.permissions {
			perm := pending.Permission
			if perm.SessionID != sessionID {
				continue
			}
			entry := runtimeActivityEvidence{kind: "permission", id: perm.ID, rank: 40, timestamp: firstPositiveInt64(perm.CreatedAt, perm.DecidedAt), turnID: perm.TurnID, toolCallID: perm.ToolCallID, permID: perm.ID}
			entry.cursor = runtimeActivityCursor(entry)
			evidence = append(evidence, entry)
		}
		r.mu.Unlock()
	}
	events := []RuntimeEvent{}
	if r.eventStore.db != nil {
		if resp, err := r.eventStore.ListSession(ctx, sessionID, 0); err == nil {
			events = resp.Events
		}
	} else {
		r.mu.Lock()
		for _, event := range r.events {
			if event.SessionID == sessionID {
				events = append(events, event)
			}
		}
		r.mu.Unlock()
	}
	for _, event := range events {
		entry := runtimeActivityEvidence{kind: "event", id: strconv.FormatInt(event.Sequence, 10), rank: 50, timestamp: runtimeEventMillis(event.CreatedAt), turnID: event.TurnID, messageID: event.MessageID, toolCallID: event.ToolCallID, sequence: event.Sequence}
		entry.cursor = runtimeActivityCursor(entry)
		evidence = append(evidence, entry)
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].timestamp != evidence[j].timestamp {
			return evidence[i].timestamp < evidence[j].timestamp
		}
		if evidence[i].rank != evidence[j].rank {
			return evidence[i].rank < evidence[j].rank
		}
		if evidence[i].kind != evidence[j].kind {
			return evidence[i].kind < evidence[j].kind
		}
		return evidence[i].id < evidence[j].id
	})
	for i := range evidence {
		evidence[i].cursor = runtimeActivityCursor(evidence[i])
	}
	selection := runtimeActivitySelection{
		turnIDs:        map[string]struct{}{},
		messageIDs:     map[string]struct{}{},
		toolCallIDs:    map[string]struct{}{},
		permissionIDs:  map[string]struct{}{},
		eventSequences: map[int64]struct{}{},
	}
	window := RuntimeActivityWindow{Limit: limit, Cursor: cursor, FromStart: true, ToEnd: true, EvidenceCount: len(evidence)}
	if len(evidence) == 0 {
		return selection, window, nil
	}
	end := len(evidence)
	if cursor != "" {
		cursorIndex := sort.Search(len(evidence), func(i int) bool {
			return runtimeActivityCompareCursor(evidence[i].cursor, cursor) > 0
		})
		end = cursorIndex
		if end < len(evidence) {
			window.HasMoreAfter = true
			window.ToEnd = false
		}
	}
	if end > len(evidence) {
		end = len(evidence)
	}
	start := 0
	if limit > 0 && end > limit {
		start = end - limit
		window.HasMoreBefore = true
		window.FromStart = false
	}
	if start < end {
		window.FirstCursor = evidence[start].cursor
		window.LastCursor = evidence[end-1].cursor
	}
	for _, item := range evidence[start:end] {
		if item.turnID != "" {
			selection.turnIDs[item.turnID] = struct{}{}
		}
		if item.messageID != "" {
			selection.messageIDs[item.messageID] = struct{}{}
			if turnID := messageTurnIDs[item.messageID]; turnID != "" {
				selection.turnIDs[turnID] = struct{}{}
			}
		}
		if item.toolCallID != "" {
			selection.toolCallIDs[item.toolCallID] = struct{}{}
		}
		if item.permID != "" {
			selection.permissionIDs[item.permID] = struct{}{}
		}
		if item.sequence > 0 {
			selection.eventSequences[item.sequence] = struct{}{}
		}
	}
	return selection, window, nil
}

func (r *runtimeService) hydrateActivityForSelection(ctx context.Context, sessionID string, allTurns []RuntimeTurn, policy RuntimePolicy, selection runtimeActivitySelection) (RuntimeSessionActivityWindowResponse, error) {
	r.mu.Lock()
	wsID := r.workspace.ID
	r.mu.Unlock()
	messages, err := r.sessionMessages(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeSessionActivityWindowResponse{}, err
	}
	turns := runtimeActivityFilterTurns(allTurns, selection.turnIDs)

	toolCalls := make([]RuntimeToolCall, 0)
	if r.toolCalls != nil {
		if len(turns) > 0 {
			seen := map[string]struct{}{}
			for _, turn := range turns {
				calls, err := r.toolCalls.ListCalls(ctx, turn.ID)
				if err != nil {
					continue
				}
				for _, call := range calls {
					if _, ok := seen[call.ID]; ok {
						continue
					}
					seen[call.ID] = struct{}{}
					toolCalls = append(toolCalls, toRuntimeToolCall(call))
				}
			}
		}
	}
	toolCallsByTurn := map[string][]RuntimeToolCall{}
	for _, call := range toolCalls {
		toolCallsByTurn[call.TurnID] = append(toolCallsByTurn[call.TurnID], call)
	}

	var permissions []RuntimePermissionRequest
	if r.permissionStore.db != nil {
		if _, err := r.reconcilePendingPermissions(ctx); err != nil {
			return RuntimeSessionActivityWindowResponse{}, err
		}
		sessionPermissions, err := r.permissionStore.ListBySession(ctx, sessionID)
		if err != nil {
			return RuntimeSessionActivityWindowResponse{}, err
		}
		for _, perm := range sessionPermissions {
			if perm.TurnID == "" {
				if selection.includeAll || runtimeActivitySelected(selection.permissionIDs, perm.ID) {
					permissions = append(permissions, perm)
				}
				continue
			}
			if _, ok := selection.turnIDs[perm.TurnID]; ok {
				permissions = append(permissions, perm)
			}
		}
	} else {
		r.mu.Lock()
		for _, pending := range r.permissions {
			if pending.Permission.SessionID == sessionID {
				if pending.Permission.TurnID == "" && !selection.includeAll && !runtimeActivitySelected(selection.permissionIDs, pending.Permission.ID) {
					continue
				}
				if pending.Permission.TurnID != "" {
					if _, ok := selection.turnIDs[pending.Permission.TurnID]; !ok {
						continue
					}
				}
				permissions = append(permissions, pending.Permission)
			}
		}
		r.mu.Unlock()
	}

	permissionsByTurn := map[string][]RuntimePermissionRequest{}
	for _, perm := range permissions {
		permissionsByTurn[perm.TurnID] = append(permissionsByTurn[perm.TurnID], perm)
	}
	hooksByTurn := map[string][]RuntimeHookExecution{}
	if r.hookExecutions.db != nil {
		sessionHooks, err := r.hookExecutions.List(ctx, RuntimeHookExecutionsRequest{SessionID: sessionID})
		if err != nil {
			return RuntimeSessionActivityWindowResponse{}, err
		}
		for _, hook := range sessionHooks {
			if _, ok := selection.turnIDs[hook.TurnID]; ok {
				hooksByTurn[hook.TurnID] = append(hooksByTurn[hook.TurnID], hook)
			}
		}
	}
	eventsByTurn, events := r.activityEventsByTurn(ctx, sessionID, selection)
	for i := range turns {
		turns[i].Diagnostics = buildRuntimeTurnDiagnostics(turns[i], messages.Messages, toolCallsByTurn[turns[i].ID], permissionsByTurn[turns[i].ID], eventsByTurn[turns[i].ID], hooksByTurn[turns[i].ID])
		turns[i].Interrupted = buildRuntimeInterruptedSummary(turns[i], turns[i].Diagnostics, toolCallsByTurn[turns[i].ID])
	}

	activityMessages := messages.Messages
	if !selection.includeAll {
		activityMessages = runtimeActivityMessagesForSelection(messages.Messages, turns, selection.messageIDs)
	}
	return RuntimeSessionActivityWindowResponse{
		SessionID:   sessionID,
		Messages:    activityMessages,
		Turns:       turns,
		ToolCalls:   toolCalls,
		Permissions: permissions,
		Events:      events,
		Policy:      policy,
	}, nil
}

func runtimeActivityTurnIDSet(turns []RuntimeTurn) map[string]struct{} {
	out := make(map[string]struct{}, len(turns))
	for _, turn := range turns {
		if turn.ID != "" {
			out[turn.ID] = struct{}{}
		}
	}
	return out
}

func runtimeActivityFilterTurns(turns []RuntimeTurn, turnIDs map[string]struct{}) []RuntimeTurn {
	if len(turnIDs) == 0 {
		return nil
	}
	out := make([]RuntimeTurn, 0, len(turnIDs))
	for _, turn := range turns {
		if _, ok := turnIDs[turn.ID]; ok {
			out = append(out, turn)
		}
	}
	return out
}

func runtimeActivityMessagesForSelection(messages []RuntimeMessage, turns []RuntimeTurn, selectedMessageIDs map[string]struct{}) []RuntimeMessage {
	messageIDs := map[string]struct{}{}
	for _, turn := range turns {
		if turn.UserMessageID != "" {
			messageIDs[turn.UserMessageID] = struct{}{}
		}
		if turn.LatestAssistantMessageID != "" {
			messageIDs[turn.LatestAssistantMessageID] = struct{}{}
		}
		if turn.LatestMessageID != "" {
			messageIDs[turn.LatestMessageID] = struct{}{}
		}
	}
	for id := range selectedMessageIDs {
		if id != "" {
			messageIDs[id] = struct{}{}
		}
	}
	out := make([]RuntimeMessage, 0, len(messageIDs))
	for _, msg := range messages {
		if _, ok := messageIDs[msg.ID]; ok {
			out = append(out, msg)
		}
	}
	return out
}

func runtimeActivitySelected[T comparable](values map[T]struct{}, value T) bool {
	_, ok := values[value]
	return ok
}

func runtimeActivityMessageTurnIDs(turns []RuntimeTurn) map[string]string {
	out := map[string]string{}
	for _, turn := range turns {
		if turn.UserMessageID != "" {
			out[turn.UserMessageID] = turn.ID
		}
		if turn.LatestAssistantMessageID != "" {
			out[turn.LatestAssistantMessageID] = turn.ID
		}
		if turn.LatestMessageID != "" {
			out[turn.LatestMessageID] = turn.ID
		}
	}
	return out
}

func runtimeActivityCursor(entry runtimeActivityEvidence) string {
	return fmt.Sprintf("v1:%020d:%03d:%s:%s", entry.timestamp, entry.rank, entry.kind, entry.id)
}

func runtimeActivityCompareCursor(left, right string) int {
	return strings.Compare(left, right)
}

func runtimeEventMillis(createdAt string) int64 {
	if createdAt == "" {
		return 0
	}
	if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		return parsed.UnixMilli()
	}
	return 0
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (r *runtimeService) activityEventsByTurn(ctx context.Context, sessionID string, selection runtimeActivitySelection) (map[string][]RuntimeEvent, []RuntimeEvent) {
	out := map[string][]RuntimeEvent{}
	events := []RuntimeEvent{}
	if r.eventStore.db != nil {
		if resp, err := r.eventStore.ListSession(ctx, sessionID, 0); err == nil {
			for _, event := range resp.Events {
				if event.TurnID == "" {
					if !selection.includeAll && !runtimeActivitySelected(selection.eventSequences, event.Sequence) {
						continue
					}
				} else if _, ok := selection.turnIDs[event.TurnID]; !ok {
					continue
				}
				events = append(events, event)
				if event.TurnID != "" {
					out[event.TurnID] = append(out[event.TurnID], event)
				}
			}
			return out, events
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.SessionID != sessionID {
			continue
		}
		if event.TurnID == "" {
			if !selection.includeAll && !runtimeActivitySelected(selection.eventSequences, event.Sequence) {
				continue
			}
		} else if _, ok := selection.turnIDs[event.TurnID]; !ok {
			continue
		}
		events = append(events, event)
		if event.TurnID != "" {
			out[event.TurnID] = append(out[event.TurnID], event)
		}
	}
	return out, events
}

func (r *runtimeService) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeMessagesResponse{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	sessionID := r.sessionID
	r.mu.Unlock()
	if sessionID == "" {
		return RuntimeMessagesResponse{}, nil
	}
	return r.sessionMessages(ctx, wsID, sessionID)
}

func (r *runtimeService) sessionMessages(ctx context.Context, wsID, sessionID string) (RuntimeMessagesResponse, error) {
	messages, err := r.runtime.ListSessionMessages(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeMessagesResponse{}, fmt.Errorf("failed to list Agent Builder session messages: %w", err)
	}

	runtimeMessages := make([]RuntimeMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.IsSummaryMessage || msg.Role == message.System {
			continue
		}
		runtimeMessage := toRuntimeMessage(toAPITypeMessage(msg))
		if !isDisplayableRuntimeMessage(runtimeMessage) {
			continue
		}
		runtimeMessages = append(runtimeMessages, runtimeMessage)
	}

	return RuntimeMessagesResponse{Messages: runtimeMessages}, nil
}

func (r *runtimeService) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	previousSessionID := r.sessionID
	r.sessionID = ""
	r.mu.Unlock()

	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventSessionSelectionCleared,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"previousSessionId": previousSessionID,
			"reason":            "new_chat",
		},
	})

	return r.Status(ctx)
}

func (r *runtimeService) ensureSessionTitle(ctx context.Context, workspaceID, sessionID, prompt string) error {
	sess, err := r.runtime.GetSession(ctx, workspaceID, sessionID)
	if err != nil {
		return err
	}
	if !isDefaultRuntimeSessionTitle(sess.Title) {
		return nil
	}
	title := preview(strings.TrimSpace(prompt), 48)
	if title == "" {
		return nil
	}
	sess.Title = title
	if _, err := r.runtime.SaveSession(ctx, workspaceID, sess); err != nil {
		return err
	}
	r.publishRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventSessionUpdated,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Payload: map[string]any{
			"title": title,
		},
	})
	return nil
}

func isDefaultRuntimeSessionTitle(title string) bool {
	title = strings.TrimSpace(title)
	return title == "" || title == "New chat" || title == agent.DefaultSessionName
}

func (r *runtimeService) sessionUsage(ctx context.Context, workspaceID, sessionID string) (RuntimeUsage, error) {
	sess, err := r.runtime.GetSession(ctx, workspaceID, sessionID)
	if err != nil {
		return RuntimeUsage{}, fmt.Errorf("failed to read Agent Builder session usage: %w", err)
	}
	return RuntimeUsage{
		InputTokens:  sess.PromptTokens,
		OutputTokens: sess.CompletionTokens,
		TotalTokens:  sess.PromptTokens + sess.CompletionTokens,
		Cost:         sess.Cost,
	}, nil
}

func toRuntimeSessions(sessions []session.Session, activeID, workspaceID string) []RuntimeSession {
	out := make([]RuntimeSession, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, toRuntimeSession(sess, activeID, workspaceID))
	}
	return out
}

func toRuntimeSession(sess session.Session, activeID, workspaceID string) RuntimeSession {
	usage := RuntimeUsage{
		InputTokens:  sess.PromptTokens,
		OutputTokens: sess.CompletionTokens,
		TotalTokens:  sess.PromptTokens + sess.CompletionTokens,
		Cost:         sess.Cost,
	}
	projectID, scope := normalizeRuntimeSessionOwnership(sess.ProjectID, sess.Scope)
	return RuntimeSession{
		ID:               sess.ID,
		Title:            firstNonEmpty(sess.Title, "New chat"),
		ProjectID:        projectID,
		Scope:            scope,
		Workdir:          sess.Workdir,
		WorkdirExists:    sess.WorkdirExists,
		MessageCount:     sess.MessageCount,
		PromptTokens:     sess.PromptTokens,
		CompletionTokens: sess.CompletionTokens,
		Cost:             sess.Cost,
		CreatedAt:        sess.CreatedAt,
		UpdatedAt:        sess.UpdatedAt,
		Active:           sess.ID == activeID,
		Usage:            usage,
	}
}

func normalizeRuntimeSessionOwnership(projectID, scope string) (string, string) {
	scope = strings.TrimSpace(scope)
	projectID = strings.TrimSpace(projectID)
	if scope == "standalone" {
		return "", "standalone"
	}
	return projectID, "project"
}

func (r *runtimeService) normalizeSessionOwnership(ctx context.Context, projectID, scope, activeProjectID string) (string, string, error) {
	rawScope := strings.TrimSpace(scope)
	projectID, normalizedScope := normalizeRuntimeSessionOwnership(projectID, scope)
	if normalizedScope == "standalone" {
		return "", "standalone", nil
	}
	if projectID == "" {
		projectID = strings.TrimSpace(activeProjectID)
	}
	if projectID == "" && rawScope == "" {
		return "", "standalone", nil
	}
	if projectID == "" {
		return "", "", errors.New("project session requires an active project")
	}
	store, err := r.projectStore(ctx)
	if err != nil {
		return "", "", err
	}
	if _, err := store.GetActive(ctx, projectID); err != nil {
		return "", "", fmt.Errorf("project session requires an active project: %w", err)
	}
	return projectID, "project", nil
}

func (r *runtimeService) normalizeSessionCreationContext(ctx context.Context, projectID, scope, workdir, activeProjectID string) (string, string, string, string, bool, error) {
	projectID, normalizedScope, err := r.normalizeSessionOwnership(ctx, projectID, scope, activeProjectID)
	if err != nil {
		return "", "", "", "", false, err
	}
	if normalizedScope == "standalone" {
		normalizedWorkdir, canonical, exists, err := normalizeSessionWorkdir(firstNonEmpty(workdir, runtimeStandaloneDefaultWorkdir()))
		if err != nil {
			return "", "", "", "", false, err
		}
		return "", "standalone", normalizedWorkdir, canonical, exists, nil
	}
	store, err := r.projectStore(ctx)
	if err != nil {
		return "", "", "", "", false, err
	}
	project, err := store.GetActive(ctx, projectID)
	if err != nil {
		return "", "", "", "", false, err
	}
	normalizedWorkdir, canonical, exists, err := normalizeSessionWorkdir(firstNonEmpty(workdir, project.Path))
	if err != nil {
		return "", "", "", "", false, err
	}
	if !runtimeProjectAllowsWorkdir(project, normalizedWorkdir, canonical) {
		return "", "", "", "", false, fmt.Errorf("project session workdir %q is outside project %q", normalizedWorkdir, project.Path)
	}
	return projectID, "project", normalizedWorkdir, canonical, exists, nil
}

func normalizeSessionWorkdir(path string) (string, string, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", false, errors.New("workdir is required")
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", "", false, fmt.Errorf("failed to resolve workdir: %w", err)
		}
		cleaned = abs
	}
	info, statErr := os.Stat(cleaned)
	exists := statErr == nil && info.IsDir()
	canonical := cleaned
	if exists {
		if evaluated, err := filepath.EvalSymlinks(cleaned); err == nil {
			canonical = filepath.Clean(evaluated)
		}
	}
	return cleaned, runtimeProjectCanonicalPath(canonical), exists, nil
}

func runtimeStandaloneDefaultWorkdir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return home
	}
	return runtimeDefaultWorkingDir()
}

func runtimeProjectAllowsWorkdir(project runtimeProjectRecord, workdir, canonicalWorkdir string) bool {
	for _, root := range []string{project.Path, project.GitRoot} {
		if runtimePathContains(root, workdir) || runtimeProjectCanonicalPath(root) == canonicalWorkdir || runtimePathContains(runtimeProjectCanonicalPath(root), canonicalWorkdir) {
			return true
		}
	}
	return false
}

func runtimePathContains(root, child string) bool {
	root = strings.TrimSpace(root)
	child = strings.TrimSpace(child)
	if root == "" || child == "" {
		return false
	}
	root = filepath.Clean(root)
	child = filepath.Clean(child)
	if strings.EqualFold(root, child) {
		return true
	}
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
