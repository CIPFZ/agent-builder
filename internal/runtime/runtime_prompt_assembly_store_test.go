package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CIPFZ/agent-builder/internal/agent"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/db"
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
	if len(turnAssemblies[0].Sections) != 2 || turnAssemblies[0].Sections[0].Kind != "stable_base" || turnAssemblies[0].Sections[1].CachePolicy != "turn_dynamic" {
		t.Fatalf("sections round trip = %#v", turnAssemblies[0].Sections)
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
	service.contextStore = contextmgr.NewSQLStore(conn)
	service.contextManager = contextmgr.NewManager(contextmgr.ManagerOptions{Store: service.contextStore})
	service.promptAssemblies = newRuntimePromptAssemblyStore(conn)
	service.eventStore = newRuntimeEventStore(conn)
	service.compactBoundaries = newRuntimeCompactBoundaryStore(conn)

	err = service.recordPromptAssembly(context.Background(), agent.PromptAssemblySnapshot{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Step:      2,
		Provider:  "openai",
		Model:     "test-model",
		Sections: []agent.PromptSectionSummary{{
			ID:            "stable_base",
			Name:          "Stable base",
			Kind:          "stable_base",
			Role:          "system",
			Order:         1,
			CachePolicy:   "stable",
			Source:        "embedded_template",
			Hash:          "sha256:stable",
			TokenEstimate: 20,
			Redacted:      true,
		}},
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
			LoadedNames:    []string{"agent-builder-config"},
			XMLPresent:     true,
			XMLHash:        "sha256:skills",
		},
		MCP: agent.PromptMCPSummary{
			ServerCount:      1,
			InstructionCount: 1,
			Servers:          []string{"docs"},
			ServerListHash:   "sha256:mcp-servers",
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
	if assembly.ProjectionID != "ctxproj_turn-1_step_002" {
		t.Fatalf("assembly projection id = %#v", assembly)
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
	if len(assembly.Sections) != 1 || assembly.Sections[0].Kind != "stable_base" || assembly.Sections[0].RawStored {
		t.Fatalf("assembly sections = %#v", assembly.Sections)
	}

	events, err := service.eventStore.ListTurn(context.Background(), "turn-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Events) != 1 || events.Events[0].Type != "prompt.assembly.recorded" {
		t.Fatalf("events = %#v", events.Events)
	}
	projections, err := service.contextStore.ListProjectionsByTurn(context.Background(), "turn-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(projections) != 1 || projections[0].ID != assembly.ProjectionID || projections[0].ProjectedMessageCount != 3 {
		t.Fatalf("projections = %#v", projections)
	}
	_, err = service.contextStore.UpsertBoundary(context.Background(), contextmgr.Boundary{
		ID:           "boundary-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ProjectionID: assembly.ProjectionID,
		Kind:         "micro",
		Trigger:      "tool_result_limit",
		Status:       "completed",
		SummaryRef:   "ctx://summary",
		MessageRefs:  []string{"message-1"},
		ToolCallRefs: []string{"tool-1"},
		CreatedAt:    1300,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.contextStore.UpsertSnipBoundary(context.Background(), contextmgr.SnipBoundary{
		ID:                 "snip-1",
		SessionID:          "session-1",
		TurnID:             "turn-1",
		ProjectionID:       assembly.ProjectionID,
		RemovedMessageRefs: []string{"message-2", "message-3"},
		SummaryRef:         "ctx://snip",
		Reason:             "manual",
		CreatedAt:          1301,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.contextStore.UpsertContentReplacement(context.Background(), contextmgr.ContentReplacement{
		ID:                         "replacement-1",
		SessionID:                  "session-1",
		TurnID:                     "turn-1",
		ProjectionID:               assembly.ProjectionID,
		ToolCallID:                 "tool-1",
		Kind:                       "tool_result",
		OriginalRef:                "ctx://tool-output",
		ReplacementText:            "[compacted]",
		OriginalEstimatedTokens:    1000,
		ReplacementEstimatedTokens: 5,
		Reason:                     "microcompact_tool_result_limit",
		CreatedAt:                  1302,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.contextStore.UpsertReactiveAttempt(context.Background(), contextmgr.ReactiveAttempt{
		ID:           "reactive-1",
		SessionID:    "session-1",
		TurnID:       "turn-1",
		ProjectionID: assembly.ProjectionID,
		Attempt:      1,
		Action:       "projection_reduction",
		Status:       "completed",
		CreatedAt:    1303,
	})
	if err != nil {
		t.Fatal(err)
	}
	enriched, err := service.enrichPromptAssembliesWithContext(context.Background(), assemblies)
	if err != nil {
		t.Fatal(err)
	}
	if len(enriched[0].ContextBoundaries) != 1 || enriched[0].ContextBoundaries[0].Kind != "micro" {
		t.Fatalf("context boundaries were not enriched: %#v", enriched[0].ContextBoundaries)
	}
	if len(enriched[0].SnipBoundaries) != 1 || len(enriched[0].SnipBoundaries[0].RemovedMessageRefs) != 2 {
		t.Fatalf("snip boundaries were not enriched: %#v", enriched[0].SnipBoundaries)
	}
	if len(enriched[0].Replacements) != 1 || enriched[0].Replacements[0].ToolCallID != "tool-1" {
		t.Fatalf("replacements were not enriched: %#v", enriched[0].Replacements)
	}
	if len(enriched[0].ReactiveAttempts) != 1 || enriched[0].ReactiveAttempts[0].Action != "projection_reduction" {
		t.Fatalf("reactive attempts were not enriched: %#v", enriched[0].ReactiveAttempts)
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
		ID:           id,
		SessionID:    sessionID,
		TurnID:       turnID,
		ProjectionID: "projection-" + id,
		Step:         step,
		Provider:     "openai",
		Model:        "test-model",
		Sections: []RuntimePromptSectionSummary{{
			ID:            "stable_base",
			Name:          "Stable base",
			Kind:          "stable_base",
			Role:          "system",
			Order:         1,
			CachePolicy:   "stable",
			Source:        "embedded_template",
			Hash:          "sha256:stable",
			TokenEstimate: 20,
			Redacted:      true,
			RawStored:     false,
		}, {
			ID:            "environment_info",
			Name:          "Environment",
			Kind:          "environment_info",
			Role:          "system",
			Order:         2,
			CachePolicy:   "turn_dynamic",
			Source:        "runtime_environment",
			Hash:          "sha256:env",
			TokenEstimate: 8,
			Redacted:      true,
			RawStored:     false,
		}},
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
			LoadedNames:      []string{"agent-builder-config"},
			XMLPresent:       true,
			XMLHash:          "sha256:skills",
			RawContentStored: false,
		},
		MCP: RuntimePromptMCPSummary{
			ServerCount:      1,
			InstructionCount: 1,
			Servers:          []string{"docs"},
			ServerListHash:   "sha256:mcp-servers",
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
