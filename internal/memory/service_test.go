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
