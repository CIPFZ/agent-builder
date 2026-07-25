package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

func TestDiagnosticIncidentsMergeProviderFailureEvidenceAndRecovery(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "diagnostic provider")
	now := time.Now().UTC()
	failed := RuntimeTurn{ID: "turn-provider-429", SessionID: sessionID, Status: turnStatusFailed, Provider: "openai", Model: "model-a", StartedAt: now.Add(-time.Minute).UnixMilli(), FinishedAt: now.Add(-50 * time.Second).UnixMilli(), Error: "provider returned 429 rate limit"}
	if _, err := service.turns.Upsert(ctx, failed); err != nil {
		t.Fatal(err)
	}
	if err := service.eventStore.Append(ctx, runtimeapi.Event{Sequence: 1, ID: "event-provider-failed", Type: runtimeapi.EventTurnFailed, SessionID: sessionID, TurnID: failed.ID, CreatedAt: now.Add(-49 * time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status": "failed"}}); err != nil {
		t.Fatal(err)
	}
	if err := newRuntimeAuditStore(service.turns.db).Append(ctx, RuntimeAuditEvent{ID: "audit-provider-failed", SessionID: sessionID, TurnID: failed.ID, Type: "failed", CreatedAt: now.Add(-48 * time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"error": failed.Error}}); err != nil {
		t.Fatal(err)
	}
	resumed := RuntimeTurn{ID: "turn-provider-retry", SessionID: sessionID, Status: turnStatusCompleted, StartedAt: now.Add(-40 * time.Second).UnixMilli(), FinishedAt: now.Add(-30 * time.Second).UnixMilli()}
	if _, err := service.turns.Upsert(ctx, resumed); err != nil {
		t.Fatal(err)
	}
	if _, err := service.recoveryLinks.Insert(ctx, runtimeRecoveryLink{ID: "recovery-provider", SourceTurnID: failed.ID, ResumedTurnID: resumed.ID, Action: "retry_recoverable_error", CreatedAt: now.Add(-29 * time.Second).Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}

	resp, err := service.DiagnosticIncidents(ctx, RuntimeDiagnosticIncidentsRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Incidents) != 1 {
		t.Fatalf("incidents = %#v", resp.Incidents)
	}
	incident := resp.Incidents[0]
	if incident.ID != "turn:"+failed.ID || incident.Kind != diagnosticKindProviderFailure || incident.ErrorCode != recoverableErrorRateLimited || !incident.Resolved || incident.Status != "recovered" {
		t.Fatalf("incident = %#v", incident)
	}
	sources := map[string]bool{}
	for _, evidence := range incident.Evidence {
		sources[evidence.Source] = true
	}
	for _, source := range []string{"runtime_turns", "runtime_events", "runtime_recovery_links"} {
		if !sources[source] {
			t.Fatalf("missing %s evidence in %#v", source, incident.Evidence)
		}
	}
	if len(incident.Evidence) > 5 {
		t.Fatalf("evidence was not compacted: %#v", incident.Evidence)
	}
	if incident.RecommendedCheckID != "" || incident.Recoverable || incident.Cause == "" || incident.Resolution == "" {
		t.Fatalf("provider guidance = %#v", incident)
	}
	for _, action := range incident.Actions {
		if action.Kind == "retry" {
			t.Fatalf("diagnostics must not retry provider calls: %#v", incident.Actions)
		}
	}
}

func TestDiagnosticActionsExplainFailuresWithoutRetryingWork(t *testing.T) {
	if isRelevantDiagnosticEvent("message.completed") {
		t.Fatal("ordinary message completion must not appear in a diagnostic fault chain")
	}
	evidence := []RuntimeDiagnosticEvidence{{ID: "evidence", Source: "runtime_turns", Kind: turnStatusFailed, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}}
	base := RuntimeTurn{ID: "turn-guidance", SessionID: "session-guidance", Status: turnStatusFailed, StartedAt: time.Now().Add(-time.Second).UnixMilli(), FinishedAt: time.Now().UnixMilli()}

	rateLimited := base
	rateLimited.Error = "provider returned 429 rate limit"
	incident, ok := buildTurnDiagnosticIncident(rateLimited, evidence, false, "")
	if !ok || incident.RecommendedCheckID != "" || hasDiagnosticAction(incident.Actions, "retry") || !hasDiagnosticAction(incident.Actions, "open_session") {
		t.Fatalf("rate limited incident = %#v", incident)
	}

	network := base
	network.ID = "turn-network"
	network.Error = "connection timeout"
	incident, ok = buildTurnDiagnosticIncident(network, evidence, false, "")
	if !ok || incident.RecommendedCheckID != diagnosticCheckProvider || hasDiagnosticAction(incident.Actions, "retry") {
		t.Fatalf("network incident = %#v", incident)
	}

	interrupted := base
	interrupted.ID = "turn-interrupted"
	interrupted.Status = turnStatusInterrupted
	interrupted.Error = "runtime stopped before final reply"
	incident, ok = buildTurnDiagnosticIncident(interrupted, evidence, false, "")
	if !ok || hasDiagnosticAction(incident.Actions, "resume") || !hasDiagnosticAction(incident.Actions, "open_session") || !hasDiagnosticAction(incident.Actions, "mark_done") {
		t.Fatalf("interrupted incident = %#v", incident)
	}
}

func TestDiagnosticIncidentsMergeFailedToolIntoTurnAndKeepIndependentTool(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "diagnostic tools")
	now := time.Now().UTC()
	if _, err := service.turns.Upsert(ctx, RuntimeTurn{ID: "turn-with-tool", SessionID: sessionID, Status: turnStatusFailed, StartedAt: now.Add(-time.Minute).UnixMilli(), FinishedAt: now.Add(-50 * time.Second).UnixMilli(), Error: "tool delivery failed"}); err != nil {
		t.Fatal(err)
	}
	createFailedTool(t, service, sessionID, "turn-with-tool", "tool-merged", now.Add(-55*time.Second))
	createFailedTool(t, service, sessionID, "turn-missing", "tool-independent", now.Add(-45*time.Second))

	resp, err := service.DiagnosticIncidents(ctx, RuntimeDiagnosticIncidentsRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, incident := range resp.Incidents {
		ids[incident.ID] = true
	}
	if !ids["turn:turn-with-tool"] || !ids["tool:tool-independent"] || ids["tool:tool-merged"] {
		t.Fatalf("incident ids = %#v", ids)
	}
}

func TestDiagnosticIncidentsExcludeExpectedStopsAndHandledCommandFailure(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "diagnostic exclusions")
	now := time.Now().UTC()
	for _, turn := range []RuntimeTurn{
		{ID: "turn-cancelled", SessionID: sessionID, Status: turnStatusCancelled, StartedAt: now.Add(-time.Minute).UnixMilli(), FinishedAt: now.Add(-50 * time.Second).UnixMilli(), Error: "user cancelled"},
		{ID: "turn-permission", SessionID: sessionID, Status: turnStatusFailed, StartedAt: now.Add(-49 * time.Second).UnixMilli(), FinishedAt: now.Add(-48 * time.Second).UnixMilli(), Error: "permission denied by user"},
		{ID: "turn-completed", SessionID: sessionID, Status: turnStatusCompleted, StartedAt: now.Add(-47 * time.Second).UnixMilli(), FinishedAt: now.Add(-40 * time.Second).UnixMilli()},
	} {
		if _, err := service.turns.Upsert(ctx, turn); err != nil {
			t.Fatal(err)
		}
	}
	createFailedTool(t, service, sessionID, "turn-completed", "tool-expected-exit", now.Add(-45*time.Second))
	resp, err := service.DiagnosticIncidents(ctx, RuntimeDiagnosticIncidentsRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Incidents) != 0 {
		t.Fatalf("excluded incidents leaked: %#v", resp.Incidents)
	}
}

func TestDiagnosticSupportInformationIsAllowlisted(t *testing.T) {
	service, cleanup := runtimeRecoveryActionTestService(t)
	defer cleanup()
	ctx := context.Background()
	sessionID := runtimeRecoveryActionTestSession(t, service, "support info")
	secret := "sk-super-secret-message-body"
	now := time.Now().UTC()
	if _, err := service.turns.Upsert(ctx, RuntimeTurn{ID: "turn-support", SessionID: sessionID, Status: turnStatusFailed, Provider: "openai", Model: "model-a", StartedAt: now.Add(-time.Second).UnixMilli(), FinishedAt: now.UnixMilli(), Error: "authentication failed " + secret}); err != nil {
		t.Fatal(err)
	}
	support, err := service.DiagnosticSupportInformation(ctx, "turn:turn-support")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, "prompt_preview", "stdout", "stderr", "environment"} {
		if strings.Contains(strings.ToLower(support.Text), strings.ToLower(forbidden)) {
			t.Fatalf("support info contains %q: %s", forbidden, support.Text)
		}
	}
}

func TestTargetedDiagnosticsRejectUnrelatedChecksAndPersistenceBufferSurvivesDBFailure(t *testing.T) {
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", runtimeDevTestRoot(t, "diagnostic-persistence"))
	service := newRuntimeService()
	service.recordPersistenceDiagnostic("runtime_turns.upsert_terminal", "db_locked", context.DeadlineExceeded, "turn-x")
	resp := service.projectPersistenceDiagnostics(RuntimeDiagnosticIncidentsRequest{}, 10)
	if len(resp.Incidents) != 1 || resp.Incidents[0].Kind != diagnosticKindPersistenceFailure || resp.Incidents[0].RecommendedCheckID != diagnosticCheckSQLite {
		t.Fatalf("persistence incidents = %#v", resp.Incidents)
	}
	if _, err := service.RunTargetedDiagnostic(context.Background(), resp.Incidents[0].ID, diagnosticCheckPath); err == nil {
		t.Fatal("unrelated path check was accepted")
	}
}

func createFailedTool(t *testing.T, service *runtimeService, sessionID, turnID, toolID string, started time.Time) {
	t.Helper()
	if _, err := service.toolCalls.CreateCall(context.Background(), scheduler.ToolCallRequest{ID: toolID, SessionID: sessionID, TurnID: turnID, Name: "bash"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.toolCalls.CompleteCall(context.Background(), scheduler.ToolCallResult{ToolCallID: toolID, Status: scheduler.ToolCallFailed, ExitCode: 1, IsError: true, Error: "command failed", OutputSummary: "exit 1", JobStartedAt: started, JobFinishedAt: started.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
}

func hasDiagnosticAction(actions []RuntimeRecoveryAction, kind string) bool {
	for _, action := range actions {
		if action.Kind == kind {
			return true
		}
	}
	return false
}
