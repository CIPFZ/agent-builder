package approval

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

type Request struct {
	ID            string
	SessionID     string
	RunID         string
	UserMessageID string
	ToolName      string
	ToolInput     string
	Category      string
	RuleSource    string
	Reason        string
	Status        Status
	CreatedAt     time.Time
}

type Manager struct {
	nextID atomic.Uint64
	mu     sync.RWMutex
	items  map[string]Request
}

func NewManager() *Manager {
	return &Manager{
		items: make(map[string]Request),
	}
}

func (m *Manager) Create(sessionID, runID, userMessageID, toolName, toolInput, reason, category, ruleSource string) Request {
	request := Request{
		ID:            fmt.Sprintf("approval-%06d", m.nextID.Add(1)),
		SessionID:     sessionID,
		RunID:         runID,
		UserMessageID: userMessageID,
		ToolName:      toolName,
		ToolInput:     toolInput,
		Category:      category,
		RuleSource:    ruleSource,
		Reason:        reason,
		Status:        StatusPending,
		CreatedAt:     time.Now().UTC(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[request.ID] = request
	return request
}

func (m *Manager) ListBySession(sessionID string) []Request {
	return m.ListBySessionAndStatus(sessionID, "")
}

func (m *Manager) ListBySessionAndStatus(sessionID string, status Status) []Request {
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]Request, 0, len(m.items))
	for _, item := range m.items {
		if item.SessionID != sessionID {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (m *Manager) Get(id string) (Request, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.items[id]
	return item, ok
}

func (m *Manager) UpdateStatus(id string, status Status) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[id]
	if !ok {
		return Request{}, fmt.Errorf("approval %q not found", id)
	}
	item.Status = status
	m.items[id] = item
	return item, nil
}

func (m *Manager) ClearBySessionAndStatus(sessionID string, status Status) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleared := 0
	for id, item := range m.items {
		if item.SessionID != sessionID {
			continue
		}
		if status == "" {
			if item.Status == StatusPending {
				continue
			}
		} else if item.Status != status {
			continue
		}
		delete(m.items, id)
		cleared++
	}
	return cleared
}
