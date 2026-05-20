package main

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
		sessions, err := r.runtime.ListSessions(ctx, wsID)
		if err != nil {
			return RuntimeSessionsResponse{}, fmt.Errorf("failed to list Crush sessions after delete: %w", err)
		}
		nextID := ""
		if len(sessions) == 0 {
			sess, err := r.runtime.CreateSession(ctx, wsID, "New chat")
			if err != nil {
				return RuntimeSessionsResponse{}, fmt.Errorf("failed to create replacement Crush session: %w", err)
			}
			nextID = sess.ID
		} else {
			nextID = sessions[0].ID
		}
		r.mu.Lock()
		r.sessionID = nextID
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

func (r *runtimeService) Messages(ctx context.Context) (RuntimeMessagesResponse, error) {
	if err := r.ensureStarted(ctx); err != nil {
		return RuntimeMessagesResponse{}, err
	}

	r.mu.Lock()
	wsID := r.workspace.ID
	sessionID := r.sessionID
	r.mu.Unlock()
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
	wsID := r.workspace.ID
	r.mu.Unlock()

	sess, err := r.runtime.CreateSession(ctx, wsID, firstNonEmpty(strings.TrimSpace(title), "New chat"))
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("failed to create session: %w", err)
	}

	r.mu.Lock()
	r.sessionID = sess.ID
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
