package tui

import (
	"strings"
	"sync"
)

type clientStore struct {
	mu             sync.RWMutex
	session        platformStatusSnapshot
	mcp            mcpSnapshot
	tasks          taskPanelSnapshot
	approvals      map[string]approvalView
	lastApprovalID string
}

type approvalView struct {
	ID          string
	ToolName    string
	ToolInput   string
	Status      string
	Reason      string
	SessionID   string
	RunID       string
	UpdatedText string
}

func newClientStore() *clientStore {
	return &clientStore{
		approvals: make(map[string]approvalView),
	}
}

func (s *clientStore) setSession(snapshot platformStatusSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = snapshot
}

func (s *clientStore) sessionSnapshot() platformStatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

func (s *clientStore) setMCP(snapshot mcpSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcp = snapshot
}

func (s *clientStore) mcpSnapshot() mcpSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mcp
}

func (s *clientStore) setTasks(snapshot taskPanelSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = snapshot
}

func (s *clientStore) taskSnapshot() taskPanelSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks
}

func (s *clientStore) applyApproval(view approvalView) {
	if strings.TrimSpace(view.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvals[view.ID] = view
	s.lastApprovalID = view.ID
}

func (s *clientStore) latestApproval() (approvalView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastApprovalID == "" {
		return approvalView{}, false
	}
	view, ok := s.approvals[s.lastApprovalID]
	return view, ok
}
