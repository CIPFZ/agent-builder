package memory

import (
	"strings"
	"sync"
	"time"

	"myclaw/internal/model"
)

type Item struct {
	ID        string
	SessionID string
	AgentID   string
	AgentType string
	Scope     AgentMemoryScope
	Namespace string
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

type AgentMemoryScope string

const (
	AgentMemoryScopeUser    AgentMemoryScope = "user"
	AgentMemoryScopeProject AgentMemoryScope = "project"
	AgentMemoryScopeLocal   AgentMemoryScope = "local"
)

type Entry struct {
	Type    Type
	Content string
}

type AgentMemoryRef struct {
	AgentType string
	Scope     AgentMemoryScope
	Namespace string
}

type AgentEntry struct {
	AgentType string
	Scope     AgentMemoryScope
	Namespace string
	Content   string
}

type Service struct {
	mu        sync.RWMutex
	nextID    int
	bySession map[string][]Item
	byAgent   map[string][]Item
}

func MetadataFromItems(items []Item) []model.MemoryMetadata {
	out := make([]model.MemoryMetadata, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Content) == "" || item.Type == "" {
			continue
		}
		out = append(out, model.MemoryMetadata{
			ID:        item.ID,
			SessionID: item.SessionID,
			AgentID:   item.AgentID,
			Type:      string(item.Type),
			Content:   item.Content,
			CreatedAt: item.CreatedAt,
		})
	}
	return out
}

func (s *Service) RecoverSession(session model.Session) {
	if s == nil || session.ID == "" || len(session.Metadata.MemoryItems) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySession[session.ID] = nil
	for _, meta := range session.Metadata.MemoryItems {
		typ := Type(strings.TrimSpace(meta.Type))
		if typ != TypeSummary && typ != TypeTask && typ != TypeInstruction {
			continue
		}
		content := strings.TrimSpace(meta.Content)
		if content == "" {
			continue
		}
		id := strings.TrimSpace(meta.ID)
		if id == "" {
			s.nextID++
			id = buildID(s.nextID)
		}
		agentID := strings.TrimSpace(meta.AgentID)
		if agentID == "" {
			agentID = session.AgentID
		}
		s.bySession[session.ID] = append(s.bySession[session.ID], Item{
			ID:        id,
			SessionID: session.ID,
			AgentID:   agentID,
			Type:      typ,
			Content:   content,
			CreatedAt: meta.CreatedAt,
		})
	}
}

func NewService() *Service {
	return &Service{
		bySession: make(map[string][]Item),
		byAgent:   make(map[string][]Item),
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

func (s *Service) SaveAgent(entry AgentEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	item := Item{
		ID:        buildID(s.nextID),
		AgentType: entry.AgentType,
		Scope:     entry.Scope,
		Namespace: entry.Namespace,
		Content:   entry.Content,
		CreatedAt: time.Now().UTC(),
	}
	key := agentMemoryKey(AgentMemoryRef{
		AgentType: entry.AgentType,
		Scope:     entry.Scope,
		Namespace: entry.Namespace,
	})
	s.byAgent[key] = append(s.byAgent[key], item)
}

func (s *Service) ListAgent(ref AgentMemoryRef) []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.byAgent[agentMemoryKey(ref)]
	return append([]Item(nil), items...)
}

func BuildAgentMemoryPrompt(scope AgentMemoryScope, items []Item) string {
	scopeNote := strings.TrimSpace(agentMemoryScopeNote(scope))
	if scopeNote == "" {
		return ""
	}
	parts := []string{
		"Persistent Agent Memory:",
		scopeNote,
	}
	if len(items) > 0 {
		lines := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(item.Content); text != "" {
				lines = append(lines, text)
			}
		}
		if len(lines) > 0 {
			parts = append(parts, "Stored Memory:\n"+strings.Join(lines, "\n"))
		}
	}
	return strings.Join(parts, "\n")
}

func buildID(n int) string {
	return "memory-" + time.Now().UTC().Format("20060102150405") + "-" + string(rune('a'+(n%26)))
}

func agentMemoryKey(ref AgentMemoryRef) string {
	return string(ref.Scope) + "\x00" + ref.AgentType + "\x00" + ref.Namespace
}

func agentMemoryScopeNote(scope AgentMemoryScope) string {
	switch scope {
	case AgentMemoryScopeUser:
		return "- Since this memory is user-scope, keep learnings general since they apply across all projects"
	case AgentMemoryScopeProject:
		return "- Since this memory is project-scope and shared with your team via version control, tailor your memories to this project"
	case AgentMemoryScopeLocal:
		return "- Since this memory is local-scope (not checked into version control), tailor your memories to this project and machine"
	default:
		return ""
	}
}
