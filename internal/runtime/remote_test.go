package runtime

import (
	"reflect"
	"testing"
	"time"

	"myclaw/internal/approval"
	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	storememory "myclaw/internal/store/memory"
	"myclaw/internal/tools"
	"myclaw/internal/workspace"
)

func TestRemoteIdentityNormalizationAndTrustTransitions(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	identity := NormalizeRemoteIdentity(RemoteIdentity{
		ConnectionID:    " conn-1 ",
		SessionID:       " session-1 ",
		ClientIdentity:  " sdk ",
		DeviceID:        " device-1 ",
		UserID:          " user-1 ",
		AgentID:         " main ",
		TransportKind:   " websocket ",
		TrustState:      RemoteTrustState("surprise"),
		LivenessState:   RemoteLivenessState("surprise"),
		ConnectedAt:     now,
		LastHeartbeatAt: now,
		Capabilities:    []string{"approval_forwarding", "approval_forwarding", "heartbeat"},
		Correlation:     map[string]string{" request_id ": " abc "},
	})

	if identity.ConnectionID != "conn-1" || identity.SessionID != "session-1" || identity.TransportKind != "websocket" {
		t.Fatalf("normalized identity = %#v", identity)
	}
	if identity.TrustState != RemoteTrustUnknown || identity.LivenessState != RemoteLivenessConnected {
		t.Fatalf("states = %q/%q, want defaults", identity.TrustState, identity.LivenessState)
	}
	if !reflect.DeepEqual(identity.Capabilities, []string{"approval_forwarding", "heartbeat"}) {
		t.Fatalf("capabilities = %#v", identity.Capabilities)
	}
	if identity.Correlation["request_id"] != "abc" {
		t.Fatalf("correlation = %#v", identity.Correlation)
	}

	for _, state := range []RemoteTrustState{
		RemoteTrustUnknown,
		RemoteTrustUntrusted,
		RemoteTrustTrusted,
		RemoteTrustRevoked,
		RemoteTrustExpired,
	} {
		if got := NormalizeRemoteTrustState(state); got != state {
			t.Fatalf("NormalizeRemoteTrustState(%q) = %q", state, got)
		}
	}
}

func TestRemoteLivenessHeartbeatReconnectAndExpiry(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(manager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{})

	identity, err := runner.UpsertRemoteIdentity(sess.ID, RemoteIdentity{
		ConnectionID:      "conn-1",
		ClientIdentity:    "sdk",
		DeviceID:          "device-1",
		AgentID:           "main",
		TransportKind:     "websocket",
		TrustState:        RemoteTrustUntrusted,
		ConnectedAt:       now,
		LastHeartbeatAt:   now,
		ReconnectDeadline: now.Add(RemoteReconnectWindow),
		Capabilities:      []string{"heartbeat"},
	})
	if err != nil {
		t.Fatalf("upsert remote identity: %v", err)
	}
	if identity.LivenessState != RemoteLivenessConnected {
		t.Fatalf("liveness = %q, want connected", identity.LivenessState)
	}

	stale := runner.RemoteSnapshotAt(sess.ID, now.Add(RemoteStaleAfter).Add(time.Second))
	if stale.Identities[0].LivenessState != RemoteLivenessStale {
		t.Fatalf("stale snapshot = %#v", stale.Identities[0])
	}

	reconnecting, err := runner.MarkRemoteReconnecting(sess.ID, "conn-1", now.Add(5*time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("mark reconnecting: %v", err)
	}
	if reconnecting.LivenessState != RemoteLivenessReconnecting || !reconnecting.ReconnectDeadline.Equal(now.Add(time.Minute)) {
		t.Fatalf("reconnecting identity = %#v", reconnecting)
	}

	heartbeat, err := runner.RecordRemoteHeartbeat(sess.ID, "conn-1", now.Add(10*time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if heartbeat.LivenessState != RemoteLivenessConnected || !heartbeat.LastHeartbeatAt.Equal(now.Add(10*time.Second)) {
		t.Fatalf("heartbeat identity = %#v", heartbeat)
	}

	expired := runner.RemoteSnapshotAt(sess.ID, now.Add(2*time.Minute))
	if expired.Identities[0].LivenessState != RemoteLivenessExpired {
		t.Fatalf("expired snapshot = %#v", expired.Identities[0])
	}
}

func TestRemoteIdentityUpsertPreservesExistingTrustState(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(manager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{})

	if _, err := runner.UpsertRemoteIdentity(sess.ID, RemoteIdentity{
		ConnectionID:      "conn-1",
		ClientIdentity:    "sdk",
		DeviceID:          "device-1",
		AgentID:           "main",
		TransportKind:     "websocket",
		TrustState:        RemoteTrustUnknown,
		ConnectedAt:       now,
		LastHeartbeatAt:   now,
		ReconnectDeadline: now.Add(RemoteReconnectWindow),
		Capabilities:      []string{"heartbeat"},
	}); err != nil {
		t.Fatalf("upsert remote identity: %v", err)
	}
	if _, err := runner.UpdateRemoteTrust(sess.ID, "conn-1", RemoteTrustTrusted); err != nil {
		t.Fatalf("update trust: %v", err)
	}

	updated, err := runner.UpsertRemoteIdentity(sess.ID, RemoteIdentity{
		ConnectionID:      "conn-1",
		ClientIdentity:    "sdk",
		DeviceID:          "device-1",
		AgentID:           "main",
		TransportKind:     "websocket",
		TrustState:        RemoteTrustUnknown,
		ConnectedAt:       now.Add(time.Second),
		LastHeartbeatAt:   now.Add(time.Second),
		ReconnectDeadline: now.Add(RemoteReconnectWindow),
		Capabilities:      []string{"heartbeat", "approval_forwarding"},
	})
	if err != nil {
		t.Fatalf("upsert existing remote identity: %v", err)
	}
	if updated.TrustState != RemoteTrustTrusted {
		t.Fatalf("trust state after upsert = %q, want trusted", updated.TrustState)
	}

	if _, err := runner.UpdateRemoteTrust(sess.ID, "conn-1", RemoteTrustRevoked); err != nil {
		t.Fatalf("update trust revoked: %v", err)
	}
	revoked, err := runner.UpsertRemoteIdentity(sess.ID, RemoteIdentity{
		ConnectionID:      "conn-1",
		ClientIdentity:    "sdk",
		DeviceID:          "device-1",
		AgentID:           "main",
		TransportKind:     "websocket",
		TrustState:        RemoteTrustUnknown,
		ConnectedAt:       now.Add(2 * time.Second),
		LastHeartbeatAt:   now.Add(2 * time.Second),
		ReconnectDeadline: now.Add(RemoteReconnectWindow),
		Capabilities:      []string{"heartbeat"},
	})
	if err != nil {
		t.Fatalf("upsert revoked remote identity: %v", err)
	}
	if revoked.TrustState != RemoteTrustRevoked {
		t.Fatalf("trust state after revoked upsert = %q, want revoked", revoked.TrustState)
	}
}

func TestRemoteStateAndApprovalCorrelationRecoverThroughSessionStore(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	approvalManager := approval.NewManager()
	request := approvalManager.Create(sess.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	runner := NewRunnerWithOptions(manager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{ApprovalManager: approvalManager})

	if _, err := runner.UpsertRemoteIdentity(sess.ID, RemoteIdentity{
		ConnectionID:      "conn-1",
		ClientIdentity:    "sdk",
		DeviceID:          "device-1",
		AgentID:           "main",
		TransportKind:     "websocket",
		TrustState:        RemoteTrustTrusted,
		ConnectedAt:       now,
		LastHeartbeatAt:   now,
		ReconnectDeadline: now.Add(RemoteReconnectWindow),
		Capabilities:      []string{"approval_forwarding"},
	}); err != nil {
		t.Fatalf("upsert remote identity: %v", err)
	}
	correlation, err := runner.RecordRemoteApprovalCorrelation(sess.ID, RemoteApprovalCorrelation{
		LocalApprovalID:     request.ID,
		RemoteCorrelationID: "remote-approval-1",
		ConnectionID:        "conn-1",
		ClientIdentity:      "sdk",
		DeviceID:            "device-1",
		Status:              RemoteApprovalPending,
		CreatedAt:           now,
		UpdatedAt:           now,
		ExpiresAt:           now.Add(time.Minute),
		DecisionPayload:     map[string]any{"source": "remote"},
	})
	if err != nil {
		t.Fatalf("record approval correlation: %v", err)
	}
	if correlation.DecisionPayload["source"] != "remote" {
		t.Fatalf("correlation payload = %#v", correlation)
	}

	recoveredManager := session.NewManager(store)
	recoveredSession, ok := recoveredManager.GetByID(sess.ID)
	if !ok {
		t.Fatal("recovered session missing")
	}
	recovered := NewRunnerWithOptions(recoveredManager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{})
	snapshot := recovered.RemoteSnapshotAt(recoveredSession.ID, now.Add(10*time.Second))
	if len(snapshot.Identities) != 1 || snapshot.Identities[0].TrustState != RemoteTrustTrusted {
		t.Fatalf("recovered remote identities = %#v", snapshot.Identities)
	}
	if len(snapshot.ApprovalCorrelations) != 1 || snapshot.ApprovalCorrelations[0].RemoteCorrelationID != "remote-approval-1" {
		t.Fatalf("recovered approval correlations = %#v", snapshot.ApprovalCorrelations)
	}
}

func TestRecordRemoteApprovalCorrelationValidatesLocalApprovalAuthority(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	store := storememory.NewSessionStore()
	manager := session.NewManager(store)
	sess := manager.GetOrCreateMain("main")
	other := manager.CreateSession("main")
	approvalManager := approval.NewManager()
	otherRequest := approvalManager.Create(other.ID, "run-1", "msg-1", "system.run", "pwd", "approval required", "approval", "")
	validRequest := approvalManager.Create(sess.ID, "run-2", "msg-2", "system.run", "pwd", "approval required", "approval", "")
	runner := NewRunnerWithOptions(manager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{ApprovalManager: approvalManager})

	_, err := runner.RecordRemoteApprovalCorrelation(sess.ID, RemoteApprovalCorrelation{
		LocalApprovalID:     "approval-missing",
		RemoteCorrelationID: "remote-missing",
		ConnectionID:        "conn-1",
		Status:              RemoteApprovalPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err == nil {
		t.Fatal("record missing approval correlation succeeded, want error")
	}
	if snapshot := runner.RemoteSnapshotAt(sess.ID, now); len(snapshot.ApprovalCorrelations) != 0 {
		t.Fatalf("snapshot after missing approval correlation = %#v, want no records", snapshot.ApprovalCorrelations)
	}

	_, err = runner.RecordRemoteApprovalCorrelation(sess.ID, RemoteApprovalCorrelation{
		LocalApprovalID:     otherRequest.ID,
		RemoteCorrelationID: "remote-other-session",
		ConnectionID:        "conn-1",
		Status:              RemoteApprovalPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err == nil {
		t.Fatal("record cross-session approval correlation succeeded, want error")
	}
	if snapshot := runner.RemoteSnapshotAt(sess.ID, now); len(snapshot.ApprovalCorrelations) != 0 {
		t.Fatalf("snapshot after cross-session approval correlation = %#v, want no records", snapshot.ApprovalCorrelations)
	}

	if _, err := approvalManager.UpdateStatus(validRequest.ID, approval.StatusApproved); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	_, err = runner.RecordRemoteApprovalCorrelation(sess.ID, RemoteApprovalCorrelation{
		LocalApprovalID:     validRequest.ID,
		RemoteCorrelationID: "remote-approved",
		ConnectionID:        "conn-1",
		Status:              RemoteApprovalPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err == nil {
		t.Fatal("record terminal approval correlation succeeded, want error")
	}

	pendingRequest := approvalManager.Create(sess.ID, "run-3", "msg-3", "system.run", "pwd", "approval required", "approval", "")
	if _, err := runner.RecordRemoteApprovalCorrelation(sess.ID, RemoteApprovalCorrelation{
		LocalApprovalID:     pendingRequest.ID,
		RemoteCorrelationID: "remote-pending",
		ConnectionID:        "conn-1",
		Status:              RemoteApprovalPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}); err != nil {
		t.Fatalf("record pending approval correlation: %v", err)
	}
	if snapshot := runner.RemoteSnapshotAt(sess.ID, now); len(snapshot.ApprovalCorrelations) != 1 {
		t.Fatalf("snapshot after pending approval correlation = %#v, want one record", snapshot.ApprovalCorrelations)
	}
}

func TestTrustedRemoteDoesNotGrantToolPermission(t *testing.T) {
	manager := session.NewManager(nil)
	sess := manager.GetOrCreateMain("main")
	runner := NewRunnerWithOptions(manager, llm.NewMockClient(), workspace.NewLoader(""), tools.NewRegistry(), Options{
		PermissionPolicy: permissions.Policy{
			Mode:  permissions.ModeDangerFullAccess,
			Rules: []permissions.Rule{{ToolName: "Bash", Action: permissions.ActionDeny, Source: string(permissions.RuleSourceSession)}},
		},
	})
	if _, err := runner.UpsertRemoteIdentity(sess.ID, RemoteIdentity{
		ConnectionID:   "conn-1",
		DeviceID:       "device-1",
		ClientIdentity: "sdk",
		TrustState:     RemoteTrustTrusted,
	}); err != nil {
		t.Fatalf("upsert trusted remote: %v", err)
	}

	decision := runner.PermissionPolicyForSession(sess.ID).Evaluate(permissions.Request{
		ToolName: "Bash",
		Command:  "pwd",
	})
	if decision.Allowed || decision.RequiresApproval || decision.Category != permissions.CategoryRuleDenied {
		t.Fatalf("permission decision = %#v, trusted remote must not bypass deny", decision)
	}
}
