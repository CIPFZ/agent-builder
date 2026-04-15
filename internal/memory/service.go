package memory

import (
	"sync"
	"time"

	"myclaw/internal/model"
)

type Item struct {
	ID        string
	SessionID string
	AgentID   string
	Type      Type
	Content   string
	CreatedAt time.Time
}

type Type string

const (
	TypeSummary     Type = "summary"
	TypeTask        Type = "task"
	TypeInstruction Type = "instruction"
)

type Entry struct {
	Type    Type
	Content string
}

type Service struct {
	mu        sync.RWMutex
	nextID    int
	bySession map[string][]Item
}

func NewService() *Service {
	return &Service{
		bySession: make(map[string][]Item),
	}
}

func (s *Service) SaveCompactionSummary(session model.Session, summary model.Message) ([]Item, bool) {
	if summary.Role != "summary" && !(summary.Role == "user" && summary.IsCompactSummary) {
		return nil, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.bySession[session.ID][:0]
	for _, item := range s.bySession[session.ID] {
		if item.Type == TypeSummary {
			continue
		}
		filtered = append(filtered, item)
	}
	s.bySession[session.ID] = filtered

	s.nextID++
	item := Item{
		ID:        buildID(s.nextID),
		SessionID: session.ID,
		AgentID:   session.AgentID,
		Type:      TypeSummary,
		Content:   summary.Content,
		CreatedAt: time.Now().UTC(),
	}
	s.bySession[session.ID] = append(s.bySession[session.ID], item)
	return append([]Item(nil), s.bySession[session.ID]...), true
}

func (s *Service) Save(session model.Session, entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	item := Item{
		ID:        buildID(s.nextID),
		SessionID: session.ID,
		AgentID:   session.AgentID,
		Type:      entry.Type,
		Content:   entry.Content,
		CreatedAt: time.Now().UTC(),
	}
	s.bySession[session.ID] = append(s.bySession[session.ID], item)
}

func (s *Service) List(sessionID string) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.bySession[sessionID]
	return append([]Item(nil), items...)
}

func buildID(n int) string {
	return "memory-" + time.Now().UTC().Format("20060102150405") + "-" + string(rune('a'+(n%26)))
}
