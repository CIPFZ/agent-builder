package approval

import (
	"fmt"
	"strconv"
	"strings"
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
	ID                string
	SessionID         string
	RunID             string
	UserMessageID     string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Category          string
	RuleSource        string
	Reason            string
	DecisionReason    string
	AcceptFeedback    string
	ContentBlocks     []map[string]any
	Status            Status
	CreatedAt         time.Time
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
	return m.CreateWithToolMetadata(sessionID, runID, userMessageID, toolName, toolInput, "", "", reason, category, ruleSource)
}

func (m *Manager) CreateWithToolMetadata(sessionID, runID, userMessageID, toolName, toolInput, toolUseID, providerMessageID, reason, category, ruleSource string) Request {
	return m.CreateWithToolInputObject(sessionID, runID, userMessageID, toolName, toolInput, nil, toolUseID, providerMessageID, reason, category, ruleSource)
}

func (m *Manager) CreateWithToolInputObject(sessionID, runID, userMessageID, toolName, toolInput string, toolInputObject map[string]any, toolUseID, providerMessageID, reason, category, ruleSource string) Request {
	return m.CreateWithDecisionReason(sessionID, runID, userMessageID, toolName, toolInput, toolInputObject, toolUseID, providerMessageID, reason, "", category, ruleSource)
}

func (m *Manager) CreateWithDecisionReason(sessionID, runID, userMessageID, toolName, toolInput string, toolInputObject map[string]any, toolUseID, providerMessageID, reason, decisionReason, category, ruleSource string) Request {
	return m.CreateWithPromptMetadata(sessionID, runID, userMessageID, toolName, toolInput, toolInputObject, toolUseID, providerMessageID, reason, decisionReason, "", nil, category, ruleSource)
}

func (m *Manager) CreateWithPromptMetadata(sessionID, runID, userMessageID, toolName, toolInput string, toolInputObject map[string]any, toolUseID, providerMessageID, reason, decisionReason, acceptFeedback string, contentBlocks []map[string]any, category, ruleSource string) Request {
	request := Request{
		ID:                fmt.Sprintf("approval-%06d", m.nextID.Add(1)),
		SessionID:         sessionID,
		RunID:             runID,
		UserMessageID:     userMessageID,
		ToolName:          toolName,
		ToolInput:         toolInput,
		ToolInputObject:   cloneAnyMap(toolInputObject),
		ToolUseID:         toolUseID,
		ProviderMessageID: providerMessageID,
		Category:          category,
		RuleSource:        ruleSource,
		Reason:            reason,
		DecisionReason:    decisionReason,
		AcceptFeedback:    acceptFeedback,
		ContentBlocks:     cloneAnyMaps(contentBlocks),
		Status:            StatusPending,
		CreatedAt:         time.Now().UTC(),
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

func (m *Manager) UpdatePromptMetadata(id, acceptFeedback string, contentBlocks []map[string]any) (Request, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[id]
	if !ok {
		return Request{}, fmt.Errorf("approval %q not found", id)
	}
	item.AcceptFeedback = acceptFeedback
	item.ContentBlocks = cloneAnyMaps(contentBlocks)
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

func (m *Manager) Restore(request Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if request.CreatedAt.IsZero() {
		request.CreatedAt = time.Now().UTC()
	}
	m.items[request.ID] = request
	if n, ok := parseApprovalCounter(request.ID); ok {
		for {
			current := m.nextID.Load()
			if n <= current {
				break
			}
			if m.nextID.CompareAndSwap(current, n) {
				break
			}
		}
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMaps(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	cloned := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned = append(cloned, cloneAnyMap(item))
	}
	return cloned
}

func parseApprovalCounter(id string) (uint64, bool) {
	if !strings.HasPrefix(id, "approval-") {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(id, "approval-"), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
