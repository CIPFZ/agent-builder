package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/db"
)

func TestRuntimePromptAssemblyStoreRoundTripAndOrders(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	store := newRuntimePromptAssemblyStore(conn)

	secretText := "sk-secret full prompt should not be stored"
	for _, assembly := range []RuntimePromptAssembly{
		promptAssemblyFixture("assembly-1", "session-1", "turn-1", 2, 2000),
		promptAssemblyFixture("assembly-2", "session-1", "turn-1", 1, 1000),
		promptAssemblyFixture("assembly-3", "session-1", "turn-2", 1, 3000),
	} {
		stored, err := store.Upsert(context.Background(), assembly)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Messages.RawPromptStored || stored.Skills.RawContentStored || stored.MCP.RawContentStored {
			t.Fatalf("raw content flags should remain false: %#v", stored)
		}
	}

	turnAssemblies, err := store.ListByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(turnAssemblies) != 2 || turnAssemblies[0].Step != 1 || turnAssemblies[1].Step != 2 {
		t.Fatalf("turn assemblies order = %#v", turnAssemblies)
	}
	if turnAssemblies[0].System.Hash == "" || len(turnAssemblies[0].ContextSources) != 1 || len(turnAssemblies[0].Compact) != 1 {
		t.Fatalf("decoded assembly = %#v", turnAssemblies[0])
	}

	sessionAssemblies, err := store.ListBySession(context.Background(), "session-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionAssemblies) != 2 || sessionAssemblies[0].ID != "assembly-1" || sessionAssemblies[1].ID != "assembly-3" {
		t.Fatalf("session assemblies order/limit = %#v", sessionAssemblies)
	}

	payload, err := json.Marshal(sessionAssemblies)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), secretText) || strings.Contains(string(payload), "sk-secret") || strings.Contains(string(payload), "full prompt") {
		t.Fatalf("prompt assembly store leaked raw text: %s", payload)
	}
}

func TestRuntimeRecordPromptAssemblyStoresSummaryEventAndFailedContext(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	conn, err := db.Connect(context.Background(), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Release(dataDir)
	})
	service := newRuntimeService()
	service.promptAssemblies = newRuntimePromptAssemblyStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)

	err = service.recordPromptAssembly(context.Background(), agent.PromptAssemblySnapshot{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Step:      2,
		Provider:  "openai",
		Model:     "test-model",
		System: agent.PromptSystemSummary{
			Source:        "runtime",
			Hash:          "sha256:system",
			PromptPrefix:  true,
			SourceRefs:    []string{"system:runtime"},
			TokenEstimate: 10,
		},
		Messages: agent.PromptMessageSummary{
			Count:                3,
			ByRole:               map[string]int{"user": 1, "assistant": 1, "tool": 1},
			ToolResultCount:      1,
			DeliveredToolResults: 1,
			TokenEstimate:        80,
		},
		Tools: agent.PromptToolSummary{
			Selected:      []string{"bash"},
			Omitted:       []string{"webfetch"},
			SelectedCount: 1,
			OmittedCount:  1,
		},
		Skills: agent.PromptSkillSummary{
			AvailableCount: 1,
			LoadedCount:    1,
			LoadedNames:    []string{"crush-config"},
			XMLPresent:     true,
			XMLHash:        "sha256:skills",
		},
		MCP: agent.PromptMCPSummary{
			ServerCount:      1,
			InstructionCount: 1,
			Servers:          []string{"docs"},
			InstructionHash:  "sha256:mcp",
		},
		CreatedAt: 1234,
	})
	if err != nil {
		t.Fatal(err)
	}

	assemblies, err := service.promptAssemblies.ListByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(assemblies) != 1 {
		t.Fatalf("assemblies = %#v", assemblies)
	}
	assembly := assemblies[0]
	if assembly.ID != "prompt_turn-1_step_002" || assembly.Provider != "openai" || assembly.Model != "test-model" {
		t.Fatalf("assembly identity = %#v", assembly)
	}
	if assembly.Messages.RawPromptStored || assembly.Skills.RawContentStored || assembly.MCP.RawContentStored {
		t.Fatalf("assembly should be summary-only: %#v", assembly)
	}
	if len(assembly.ContextSources) != 1 || assembly.ContextSources[0].State != capabilityStateFailed {
		t.Fatalf("failed context source was not recorded: %#v", assembly.ContextSources)
	}
	if assembly.Tools.SelectedCount != 1 || assembly.Tools.OmittedCount != 1 || assembly.System.Hash != "sha256:system" {
		t.Fatalf("assembly summaries = %#v", assembly)
	}

	events, err := service.eventStore.ListTurn(context.Background(), "turn-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 1 || events.Events[0].Type != "prompt.assembly.recorded" {
		t.Fatalf("events = %#v", events.Events)
	}
	rawEvent, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawEvent), "sha256:system") || strings.Contains(string(rawEvent), "sk-secret") || strings.Contains(string(rawEvent), "full prompt") {
		t.Fatalf("prompt assembly event leaked prompt details: %s", rawEvent)
	}
}

func promptAssemblyFixture(id, sessionID, turnID string, step int, createdAt int64) RuntimePromptAssembly {
	return RuntimePromptAssembly{
		ID:        id,
		SessionID: sessionID,
		TurnID:    turnID,
		Step:      step,
		Provider:  "openai",
		Model:     "test-model",
		System: RuntimePromptSystemSummary{
			Source:        "runtime",
			Hash:          "sha256:system",
			PromptPrefix:  true,
			SourceRefs:    []string{"system:runtime"},
			Redacted:      true,
			TokenEstimate: 12,
		},
		Messages: RuntimePromptMessageSummary{
			Count:           2,
			ByRole:          map[string]int{"user": 1, "assistant": 1},
			RawPromptStored: false,
		},
		Tools: RuntimePromptToolSummary{
			Selected:      []string{"bash"},
			Omitted:       []string{"webfetch"},
			SelectedCount: 1,
			OmittedCount:  1,
		},
		Skills: RuntimePromptSkillSummary{
			LoadedCount:      1,
			LoadedNames:      []string{"crush-config"},
			XMLPresent:       true,
			XMLHash:          "sha256:skills",
			RawContentStored: false,
		},
		MCP: RuntimePromptMCPSummary{
			ServerCount:      1,
			InstructionCount: 1,
			Servers:          []string{"docs"},
			InstructionHash:  "sha256:mcp",
			RawContentStored: false,
		},
		ContextSources: []RuntimeContextSource{{
			ID:          "context-1",
			Kind:        "project_memory",
			Name:        "AGENTS.md",
			Enabled:     true,
			State:       capabilityStateLoaded,
			ContentHash: "sha256:context",
		}},
		Compact: []RuntimeCompactBoundary{{
			ID:         "compact-1",
			SessionID:  sessionID,
			TurnID:     turnID,
			Kind:       "microcompact",
			Trigger:    "budget",
			Status:     "completed",
			SummaryRef: "ref-compact",
		}},
		Budget: RuntimeBudgetReport{
			ContextSources:       RuntimeBudgetBucket{Count: 1, EstimatedTokens: 12},
			SelectedToolSchemas:  RuntimeBudgetBucket{Count: 1, EstimatedTokens: 8},
			TotalEstimatedTokens: 20,
		},
		CreatedAt: createdAt,
	}
}
