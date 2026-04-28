package memory_test

import (
	"testing"
	"time"

	"myclaw/internal/memory"
	"myclaw/internal/model"
)

func TestServiceSaveSummaryMemoryOnCompaction(t *testing.T) {
	service := memory.NewService()
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	memories, saved := service.SaveCompactionSummary(session, model.Message{
		ID:        "summary-1",
		SessionID: session.ID,
		Role:      "summary",
		Content:   "Summary: finished auth fix and pending docs update",
		CreatedAt: time.Unix(10, 0).UTC(),
	})

	if !saved {
		t.Fatal("expected summary memory to be saved")
	}
	if len(memories) != 1 {
		t.Fatalf("memory count = %d, want 1", len(memories))
	}
	if memories[0].SessionID != session.ID {
		t.Fatalf("memory session id = %q, want %q", memories[0].SessionID, session.ID)
	}
}

func TestServiceSaveClaudeCompactSummaryMemoryOnCompaction(t *testing.T) {
	service := memory.NewService()
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	memories, saved := service.SaveCompactionSummary(session, model.Message{
		ID:               "summary-1",
		SessionID:        session.ID,
		Role:             "user",
		Content:          "This session is being continued from a previous conversation that ran out of context.",
		IsCompactSummary: true,
		CreatedAt:        time.Unix(10, 0).UTC(),
	})

	if !saved {
		t.Fatal("expected Claude compact summary memory to be saved")
	}
	if len(memories) != 1 {
		t.Fatalf("memory count = %d, want 1", len(memories))
	}
	if memories[0].Content == "" || memories[0].Content != "This session is being continued from a previous conversation that ran out of context." {
		t.Fatalf("memory content = %q, want compact summary content", memories[0].Content)
	}
}

func TestServiceSkipsNonSummaryMessages(t *testing.T) {
	service := memory.NewService()
	session := model.Session{ID: "main-000001"}

	memories, saved := service.SaveCompactionSummary(session, model.Message{
		ID:        "msg-1",
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "plain answer",
	})

	if saved {
		t.Fatal("expected non-summary message to skip memory save")
	}
	if len(memories) != 0 {
		t.Fatalf("memory count = %d, want 0", len(memories))
	}
}

func TestServiceListReturnsCopies(t *testing.T) {
	service := memory.NewService()
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
	}
	service.SaveCompactionSummary(session, model.Message{
		ID:        "summary-1",
		SessionID: session.ID,
		Role:      "summary",
		Content:   "Summary: one",
	})

	first := service.List(session.ID)
	second := service.List(session.ID)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("memory counts = %d/%d, want 1/1", len(first), len(second))
	}
	first[0].Content = "mutated"
	if second[0].Content != "Summary: one" {
		t.Fatalf("expected independent copies, got %q", second[0].Content)
	}
}

func TestServiceSavesTypedMemories(t *testing.T) {
	service := memory.NewService()
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
	}

	service.Save(session, memory.Entry{
		Type:    memory.TypeTask,
		Content: "Task: finish deployment checklist",
	})
	service.Save(session, memory.Entry{
		Type:    memory.TypeInstruction,
		Content: "Instruction: ask before destructive commands",
	})

	items := service.List(session.ID)
	if len(items) != 2 {
		t.Fatalf("memory count = %d, want 2", len(items))
	}
	if items[0].Type != memory.TypeTask {
		t.Fatalf("first memory type = %q, want task", items[0].Type)
	}
	if items[1].Type != memory.TypeInstruction {
		t.Fatalf("second memory type = %q, want instruction", items[1].Type)
	}
}

func TestServiceSaveCompactionSummaryReplacesOlderSummaryMemory(t *testing.T) {
	service := memory.NewService()
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}

	service.SaveCompactionSummary(session, model.Message{
		ID:        "summary-1",
		SessionID: session.ID,
		Role:      "summary",
		Content:   "Summary: first summary",
		CreatedAt: time.Unix(10, 0).UTC(),
	})
	memories, saved := service.SaveCompactionSummary(session, model.Message{
		ID:        "summary-2",
		SessionID: session.ID,
		Role:      "summary",
		Content:   "Summary: second summary",
		CreatedAt: time.Unix(20, 0).UTC(),
	})

	if !saved {
		t.Fatal("expected second summary memory to be saved")
	}
	if len(memories) != 1 {
		t.Fatalf("memory count = %d, want 1 latest summary only", len(memories))
	}
	if memories[0].Content != "Summary: second summary" {
		t.Fatalf("memory content = %q, want latest summary", memories[0].Content)
	}
}

func TestServiceRecoversCompactionSummaryFromSessionMetadataAfterRestart(t *testing.T) {
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
		IsMain:  true,
	}
	service := memory.NewService()
	memories, saved := service.SaveCompactionSummary(session, model.Message{
		ID:        "summary-1",
		SessionID: session.ID,
		Role:      "summary",
		Content:   "Summary: persisted across restart",
		CreatedAt: time.Unix(10, 0).UTC(),
	})
	if !saved || len(memories) != 1 {
		t.Fatalf("save summary memories = %#v saved=%v", memories, saved)
	}
	session.Metadata.MemoryItems = memory.MetadataFromItems(memories)

	restarted := memory.NewService()
	restarted.RecoverSession(session)
	recovered := restarted.List(session.ID)

	if len(recovered) != 1 {
		t.Fatalf("recovered memory count = %d, want 1", len(recovered))
	}
	if recovered[0].Content != "Summary: persisted across restart" || recovered[0].Type != memory.TypeSummary {
		t.Fatalf("recovered memory = %#v", recovered[0])
	}
}

func TestServiceSkipsInvalidRecoveredMemoryWithoutPanic(t *testing.T) {
	service := memory.NewService()
	service.RecoverSession(model.Session{
		ID: "main-000001",
		Metadata: model.SessionMetadata{
			MemoryItems: []model.MemoryMetadata{
				{ID: "bad", Type: "not-a-valid-type", Content: "bad"},
				{ID: "empty", Type: string(memory.TypeSummary), Content: "   "},
			},
		},
	})

	if got := service.List("main-000001"); len(got) != 0 {
		t.Fatalf("recovered invalid memories = %#v, want none", got)
	}
}

func TestServiceAgentMemoryIsStoredSeparatelyFromSessionMemory(t *testing.T) {
	service := memory.NewService()
	session := model.Session{
		ID:      "main-000001",
		Key:     "agent:main:main",
		AgentID: "main",
	}

	service.Save(session, memory.Entry{
		Type:    memory.TypeTask,
		Content: "Task: session scoped task",
	})
	service.SaveAgent(memory.AgentEntry{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeProject,
		Namespace: "repo-a",
		Content:   "Persistent agent memory",
	})

	sessionItems := service.List(session.ID)
	if len(sessionItems) != 1 {
		t.Fatalf("session memory count = %d, want 1", len(sessionItems))
	}
	if sessionItems[0].Content != "Task: session scoped task" {
		t.Fatalf("session memory content = %q, want session content", sessionItems[0].Content)
	}

	agentItems := service.ListAgent(memory.AgentMemoryRef{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeProject,
		Namespace: "repo-a",
	})
	if len(agentItems) != 1 {
		t.Fatalf("agent memory count = %d, want 1", len(agentItems))
	}
	if agentItems[0].Content != "Persistent agent memory" {
		t.Fatalf("agent memory content = %q, want persistent content", agentItems[0].Content)
	}
	if agentItems[0].SessionID != "" {
		t.Fatalf("agent memory session id = %q, want empty", agentItems[0].SessionID)
	}
}

func TestServiceAgentMemorySupportsScopesWithoutLeakage(t *testing.T) {
	service := memory.NewService()

	service.SaveAgent(memory.AgentEntry{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeUser,
		Namespace: "global-user",
		Content:   "User scoped memory",
	})
	service.SaveAgent(memory.AgentEntry{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeProject,
		Namespace: "repo-a",
		Content:   "Project scoped memory",
	})
	service.SaveAgent(memory.AgentEntry{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeLocal,
		Namespace: "repo-a:machine-1",
		Content:   "Local scoped memory",
	})

	testCases := []struct {
		name      string
		ref       memory.AgentMemoryRef
		wantCount int
		wantText  string
	}{
		{
			name: "user scope",
			ref: memory.AgentMemoryRef{
				AgentType: "researcher",
				Scope:     memory.AgentMemoryScopeUser,
				Namespace: "global-user",
			},
			wantCount: 1,
			wantText:  "User scoped memory",
		},
		{
			name: "project scope",
			ref: memory.AgentMemoryRef{
				AgentType: "researcher",
				Scope:     memory.AgentMemoryScopeProject,
				Namespace: "repo-a",
			},
			wantCount: 1,
			wantText:  "Project scoped memory",
		},
		{
			name: "local scope",
			ref: memory.AgentMemoryRef{
				AgentType: "researcher",
				Scope:     memory.AgentMemoryScopeLocal,
				Namespace: "repo-a:machine-1",
			},
			wantCount: 1,
			wantText:  "Local scoped memory",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			items := service.ListAgent(tc.ref)
			if len(items) != tc.wantCount {
				t.Fatalf("agent memory count = %d, want %d", len(items), tc.wantCount)
			}
			if items[0].Content != tc.wantText {
				t.Fatalf("agent memory content = %q, want %q", items[0].Content, tc.wantText)
			}
		})
	}
}

func TestServiceAgentMemoryNamespaceDoesNotLeakAcrossProjects(t *testing.T) {
	service := memory.NewService()

	service.SaveAgent(memory.AgentEntry{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeProject,
		Namespace: "repo-a",
		Content:   "Repo A memory",
	})
	service.SaveAgent(memory.AgentEntry{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeProject,
		Namespace: "repo-b",
		Content:   "Repo B memory",
	})

	items := service.ListAgent(memory.AgentMemoryRef{
		AgentType: "researcher",
		Scope:     memory.AgentMemoryScopeProject,
		Namespace: "repo-a",
	})
	if len(items) != 1 {
		t.Fatalf("agent memory count = %d, want 1", len(items))
	}
	if items[0].Content != "Repo A memory" {
		t.Fatalf("agent memory content = %q, want repo-a content", items[0].Content)
	}
}
