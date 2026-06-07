package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/runtimeapi"
	"github.com/charmbracelet/crush/internal/session"
)

func (r *runtimeService) Sessions(ctx context.Context) (RuntimeSessionsResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSessionsResponse{}, err
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	activeID := r.sessionID
	r.mu.Unlock()

	sessions, err := r.runtime.ListSessions(ctx, wsID)
	if err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to list Crush sessions: %w", err)
	}
	return RuntimeSessionsResponse{Sessions: toRuntimeSessions(sessions, activeID)}, nil
}

func (r *runtimeService) Session(ctx context.Context, sessionID string) (RuntimeSessionResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
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
		return RuntimeSessionResponse{}, fmt.Errorf("failed to read Crush session: %w", err)
	}
	return RuntimeSessionResponse{Session: toRuntimeSession(sess, activeID)}, nil
}

func (r *runtimeService) SelectSession(ctx context.Context, sessionID string) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
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
		return RuntimeStatus{}, fmt.Errorf("failed to select Crush session: %w", err)
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
	if err := r.ensureStarted(ctx); err != nil {
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
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to read Crush session: %w", err)
	}
	sess.Title = title
	if _, err := r.runtime.SaveSession(ctx, wsID, sess); err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to rename Crush session: %w", err)
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
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSessionsResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionsResponse{}, errors.New("session id is required")
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	activeID := r.sessionID
	r.mu.Unlock()

	if err := r.runtime.DeleteSession(ctx, wsID, sessionID); err != nil {
		return RuntimeSessionsResponse{}, fmt.Errorf("failed to delete Crush session: %w", err)
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
	})
	return r.Sessions(ctx)
}

func (r *runtimeService) SessionMessages(ctx context.Context, sessionID string) (RuntimeMessagesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
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
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeSessionActivityResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return RuntimeSessionActivityResponse{}, errors.New("session id is required")
	}
	r.mu.Lock()
	wsID := r.workspace.ID
	policy := r.policy
	r.mu.Unlock()

	messages, err := r.sessionMessages(ctx, wsID, sessionID)
	if err != nil {
		return RuntimeSessionActivityResponse{}, err
	}
	if policy.Mode == "" {
		if policyResp, err := r.GetPolicy(ctx); err == nil {
			policy = policyResp.Policy
		} else {
			policy = defaultRuntimePolicy()
		}
	}

	var turns []RuntimeTurn
	if r.turns.db != nil {
		turns, err = r.turns.ListBySession(ctx, sessionID)
		if err != nil {
			return RuntimeSessionActivityResponse{}, err
		}
	}

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
			return RuntimeSessionActivityResponse{}, err
		}
		permissions, err = r.permissionStore.ListBySession(ctx, sessionID)
		if err != nil {
			return RuntimeSessionActivityResponse{}, err
		}
	} else {
		r.mu.Lock()
		for _, pending := range r.permissions {
			if pending.Permission.SessionID == sessionID {
				permissions = append(permissions, pending.Permission)
			}
		}
		r.mu.Unlock()
	}

	permissionsByTurn := map[string][]RuntimePermissionRequest{}
	for _, perm := range permissions {
		permissionsByTurn[perm.TurnID] = append(permissionsByTurn[perm.TurnID], perm)
	}
	eventsByTurn := r.sessionActivityEventsByTurn(ctx, sessionID)
	for i := range turns {
		turns[i].Diagnostics = buildRuntimeTurnDiagnostics(turns[i], messages.Messages, toolCallsByTurn[turns[i].ID], permissionsByTurn[turns[i].ID], eventsByTurn[turns[i].ID])
	}

	return RuntimeSessionActivityResponse{
		SessionID:   sessionID,
		Messages:    messages.Messages,
		Turns:       turns,
		ToolCalls:   toolCalls,
		Permissions: permissions,
		Policy:      policy,
	}, nil
}

func (r *runtimeService) sessionActivityEventsByTurn(ctx context.Context, sessionID string) map[string][]RuntimeEvent {
	out := map[string][]RuntimeEvent{}
	if r.eventStore.db != nil {
		if resp, err := r.eventStore.ListSession(ctx, sessionID, 0); err == nil {
			for _, event := range resp.Events {
				if event.TurnID != "" {
					out[event.TurnID] = append(out[event.TurnID], event)
				}
			}
			return out
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.SessionID == sessionID && event.TurnID != "" {
			out[event.TurnID] = append(out[event.TurnID], event)
		}
	}
	return out
}

func (r *runtimeService) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
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
		return RuntimeMessagesResponse{}, fmt.Errorf("failed to list Crush session messages: %w", err)
	}

	runtimeMessages := make([]RuntimeMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.IsSummaryMessage || msg.Role == message.System {
			continue
		}
		runtimeMessage := toRuntimeMessage(toProtoMessage(msg))
		if !isDisplayableRuntimeMessage(runtimeMessage) {
			continue
		}
		runtimeMessages = append(runtimeMessages, runtimeMessage)
	}

	return RuntimeMessagesResponse{Messages: runtimeMessages}, nil
}

func (r *runtimeService) NewChat(ctx context.Context, title string) (RuntimeStatus, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeStatus{}, err
	}

	r.mu.Lock()
	r.sessionID = ""
	r.mu.Unlock()

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
		return RuntimeUsage{}, fmt.Errorf("failed to read Crush session usage: %w", err)
	}
	return RuntimeUsage{
		PromptTokens:     sess.PromptTokens,
		CompletionTokens: sess.CompletionTokens,
		TotalTokens:      sess.PromptTokens + sess.CompletionTokens,
		Cost:             sess.Cost,
	}, nil
}

func toRuntimeSessions(sessions []session.Session, activeID string) []RuntimeSession {
	out := make([]RuntimeSession, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, toRuntimeSession(sess, activeID))
	}
	return out
}

func toRuntimeSession(sess session.Session, activeID string) RuntimeSession {
	usage := RuntimeUsage{
		PromptTokens:     sess.PromptTokens,
		CompletionTokens: sess.CompletionTokens,
		TotalTokens:      sess.PromptTokens + sess.CompletionTokens,
		Cost:             sess.Cost,
	}
	return RuntimeSession{
		ID:               sess.ID,
		Title:            firstNonEmpty(sess.Title, "New chat"),
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
