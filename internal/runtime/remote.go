package runtime

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/session"
)

type RemoteTrustState string

const (
	RemoteTrustUnknown   RemoteTrustState = "unknown"
	RemoteTrustUntrusted RemoteTrustState = "untrusted"
	RemoteTrustTrusted   RemoteTrustState = "trusted"
	RemoteTrustRevoked   RemoteTrustState = "revoked"
	RemoteTrustExpired   RemoteTrustState = "expired"
)

type RemoteLivenessState string

const (
	RemoteLivenessConnected    RemoteLivenessState = "connected"
	RemoteLivenessStale        RemoteLivenessState = "stale"
	RemoteLivenessDisconnected RemoteLivenessState = "disconnected"
	RemoteLivenessReconnecting RemoteLivenessState = "reconnecting"
	RemoteLivenessExpired      RemoteLivenessState = "expired"
)

type RemoteApprovalStatus string

const (
	RemoteApprovalPending   RemoteApprovalStatus = "pending"
	RemoteApprovalForwarded RemoteApprovalStatus = "forwarded"
	RemoteApprovalResolved  RemoteApprovalStatus = "resolved"
	RemoteApprovalExpired   RemoteApprovalStatus = "expired"
)

const (
	RemoteStaleAfter      = 30 * time.Second
	RemoteReconnectWindow = 2 * time.Minute
)

type RemoteIdentity struct {
	ConnectionID      string
	SessionID         string
	ClientIdentity    string
	DeviceID          string
	UserID            string
	AgentID           string
	TransportKind     string
	TrustState        RemoteTrustState
	LivenessState     RemoteLivenessState
	ConnectedAt       time.Time
	DisconnectedAt    time.Time
	LastHeartbeatAt   time.Time
	ReconnectDeadline time.Time
	Capabilities      []string
	Correlation       map[string]string
}

type RemoteApprovalCorrelation struct {
	LocalApprovalID     string
	RemoteCorrelationID string
	ConnectionID        string
	ClientIdentity      string
	DeviceID            string
	Status              RemoteApprovalStatus
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ExpiresAt           time.Time
	DecisionPayload     map[string]any
}

type RemoteSnapshot struct {
	SessionID            string
	Identities           []RemoteIdentity
	ApprovalCorrelations []RemoteApprovalCorrelation
}

type RemoteManager struct {
	mu       sync.Mutex
	sessions *session.Manager
}

func NewRemoteManager(sessions *session.Manager) *RemoteManager {
	return &RemoteManager{sessions: sessions}
}

func NormalizeRemoteTrustState(state RemoteTrustState) RemoteTrustState {
	switch RemoteTrustState(strings.ToLower(strings.TrimSpace(string(state)))) {
	case RemoteTrustUntrusted:
		return RemoteTrustUntrusted
	case RemoteTrustTrusted:
		return RemoteTrustTrusted
	case RemoteTrustRevoked:
		return RemoteTrustRevoked
	case RemoteTrustExpired:
		return RemoteTrustExpired
	default:
		return RemoteTrustUnknown
	}
}

func NormalizeRemoteLivenessState(state RemoteLivenessState) RemoteLivenessState {
	switch RemoteLivenessState(strings.ToLower(strings.TrimSpace(string(state)))) {
	case RemoteLivenessStale:
		return RemoteLivenessStale
	case RemoteLivenessDisconnected:
		return RemoteLivenessDisconnected
	case RemoteLivenessReconnecting:
		return RemoteLivenessReconnecting
	case RemoteLivenessExpired:
		return RemoteLivenessExpired
	default:
		return RemoteLivenessConnected
	}
}

func NormalizeRemoteApprovalStatus(status RemoteApprovalStatus) RemoteApprovalStatus {
	switch RemoteApprovalStatus(strings.ToLower(strings.TrimSpace(string(status)))) {
	case RemoteApprovalForwarded:
		return RemoteApprovalForwarded
	case RemoteApprovalResolved:
		return RemoteApprovalResolved
	case RemoteApprovalExpired:
		return RemoteApprovalExpired
	default:
		return RemoteApprovalPending
	}
}

func NormalizeRemoteIdentity(identity RemoteIdentity) RemoteIdentity {
	identity.ConnectionID = strings.TrimSpace(identity.ConnectionID)
	identity.SessionID = strings.TrimSpace(identity.SessionID)
	identity.ClientIdentity = strings.TrimSpace(identity.ClientIdentity)
	identity.DeviceID = strings.TrimSpace(identity.DeviceID)
	identity.UserID = strings.TrimSpace(identity.UserID)
	identity.AgentID = strings.TrimSpace(identity.AgentID)
	identity.TransportKind = strings.ToLower(strings.TrimSpace(identity.TransportKind))
	identity.TrustState = NormalizeRemoteTrustState(identity.TrustState)
	identity.LivenessState = NormalizeRemoteLivenessState(identity.LivenessState)
	identity.Capabilities = compactRemoteStrings(identity.Capabilities)
	identity.Correlation = normalizeRemoteCorrelation(identity.Correlation)
	return identity
}

func NormalizeRemoteApprovalCorrelation(record RemoteApprovalCorrelation) RemoteApprovalCorrelation {
	record.LocalApprovalID = strings.TrimSpace(record.LocalApprovalID)
	record.RemoteCorrelationID = strings.TrimSpace(record.RemoteCorrelationID)
	record.ConnectionID = strings.TrimSpace(record.ConnectionID)
	record.ClientIdentity = strings.TrimSpace(record.ClientIdentity)
	record.DeviceID = strings.TrimSpace(record.DeviceID)
	record.Status = NormalizeRemoteApprovalStatus(record.Status)
	record.DecisionPayload = cloneAnyMap(record.DecisionPayload)
	return record
}

func (m *RemoteManager) UpsertIdentity(sessionID string, identity RemoteIdentity) (RemoteIdentity, error) {
	if m == nil || m.sessions == nil {
		return RemoteIdentity{}, fmt.Errorf("remote manager is not configured")
	}
	identity.SessionID = sessionID
	identity = NormalizeRemoteIdentity(identity)
	if identity.ConnectionID == "" {
		return RemoteIdentity{}, fmt.Errorf("remote identity requires connection_id")
	}
	if identity.ConnectedAt.IsZero() {
		identity.ConnectedAt = time.Now().UTC()
	}
	if identity.LastHeartbeatAt.IsZero() {
		identity.LastHeartbeatAt = identity.ConnectedAt
	}
	if identity.ReconnectDeadline.IsZero() {
		identity.ReconnectDeadline = identity.LastHeartbeatAt.Add(RemoteReconnectWindow)
	}
	identity.LivenessState = RemoteLivenessConnected
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		for _, existing := range identitiesFromMetadata(metadata.RemoteIdentities) {
			if existing.ConnectionID == identity.ConnectionID {
				identity.TrustState = existing.TrustState
				break
			}
		}
		metadata.RemoteIdentities = upsertRemoteIdentityMetadata(metadata.RemoteIdentities, identityToMetadata(identity))
	}); err != nil {
		return RemoteIdentity{}, err
	}
	return identity, nil
}

func (m *RemoteManager) RecordHeartbeat(sessionID, connectionID string, at, deadline time.Time) (RemoteIdentity, error) {
	return m.updateIdentity(sessionID, connectionID, func(identity RemoteIdentity) RemoteIdentity {
		if at.IsZero() {
			at = time.Now().UTC()
		}
		identity.LastHeartbeatAt = at
		identity.LivenessState = RemoteLivenessConnected
		identity.DisconnectedAt = time.Time{}
		if deadline.IsZero() {
			deadline = at.Add(RemoteReconnectWindow)
		}
		identity.ReconnectDeadline = deadline
		return identity
	})
}

func (m *RemoteManager) MarkReconnecting(sessionID, connectionID string, at, deadline time.Time) (RemoteIdentity, error) {
	return m.updateIdentity(sessionID, connectionID, func(identity RemoteIdentity) RemoteIdentity {
		if at.IsZero() {
			at = time.Now().UTC()
		}
		identity.LivenessState = RemoteLivenessReconnecting
		identity.LastHeartbeatAt = at
		if deadline.IsZero() {
			deadline = at.Add(RemoteReconnectWindow)
		}
		identity.ReconnectDeadline = deadline
		return identity
	})
}

func (m *RemoteManager) Disconnect(sessionID, connectionID string, at time.Time) (RemoteIdentity, error) {
	return m.updateIdentity(sessionID, connectionID, func(identity RemoteIdentity) RemoteIdentity {
		if at.IsZero() {
			at = time.Now().UTC()
		}
		identity.LivenessState = RemoteLivenessDisconnected
		identity.DisconnectedAt = at
		return identity
	})
}

func (m *RemoteManager) UpdateTrust(sessionID, connectionID string, state RemoteTrustState) (RemoteIdentity, error) {
	return m.updateIdentity(sessionID, connectionID, func(identity RemoteIdentity) RemoteIdentity {
		identity.TrustState = NormalizeRemoteTrustState(state)
		return identity
	})
}

func (m *RemoteManager) RecordApprovalCorrelation(sessionID string, record RemoteApprovalCorrelation) (RemoteApprovalCorrelation, error) {
	if m == nil || m.sessions == nil {
		return RemoteApprovalCorrelation{}, fmt.Errorf("remote manager is not configured")
	}
	record = NormalizeRemoteApprovalCorrelation(record)
	if record.LocalApprovalID == "" || record.RemoteCorrelationID == "" {
		return RemoteApprovalCorrelation{}, fmt.Errorf("remote approval correlation requires local and remote ids")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		metadata.RemoteApprovalCorrelations = upsertRemoteApprovalMetadata(metadata.RemoteApprovalCorrelations, approvalToMetadata(record))
	}); err != nil {
		return RemoteApprovalCorrelation{}, err
	}
	return record, nil
}

func (m *RemoteManager) SnapshotAt(sessionID string, at time.Time) RemoteSnapshot {
	if m == nil || m.sessions == nil {
		return RemoteSnapshot{SessionID: sessionID}
	}
	sess, ok := m.sessions.GetByID(sessionID)
	if !ok {
		return RemoteSnapshot{SessionID: sessionID}
	}
	identities := identitiesFromMetadata(sess.Metadata.RemoteIdentities)
	for i := range identities {
		identities[i] = deriveRemoteLiveness(identities[i], at)
	}
	sort.Slice(identities, func(i, j int) bool {
		return identities[i].ConnectionID < identities[j].ConnectionID
	})
	correlations := approvalsFromMetadata(sess.Metadata.RemoteApprovalCorrelations)
	sort.Slice(correlations, func(i, j int) bool {
		return correlations[i].RemoteCorrelationID < correlations[j].RemoteCorrelationID
	})
	return RemoteSnapshot{
		SessionID:            sessionID,
		Identities:           identities,
		ApprovalCorrelations: correlations,
	}
}

func (m *RemoteManager) updateIdentity(sessionID, connectionID string, update func(RemoteIdentity) RemoteIdentity) (RemoteIdentity, error) {
	if m == nil || m.sessions == nil {
		return RemoteIdentity{}, fmt.Errorf("remote manager is not configured")
	}
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return RemoteIdentity{}, fmt.Errorf("remote identity requires connection_id")
	}
	var updated RemoteIdentity
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		identities := identitiesFromMetadata(metadata.RemoteIdentities)
		for i, identity := range identities {
			if identity.ConnectionID != connectionID {
				continue
			}
			updated = NormalizeRemoteIdentity(update(identity))
			identities[i] = updated
			metadata.RemoteIdentities = identitiesToMetadata(identities)
			return
		}
	}); err != nil {
		return RemoteIdentity{}, err
	}
	if updated.ConnectionID == "" {
		return RemoteIdentity{}, fmt.Errorf("remote identity %q not found", connectionID)
	}
	return updated, nil
}

func deriveRemoteLiveness(identity RemoteIdentity, at time.Time) RemoteIdentity {
	if at.IsZero() || identity.LivenessState == RemoteLivenessDisconnected || identity.LivenessState == RemoteLivenessExpired {
		return identity
	}
	if !identity.ReconnectDeadline.IsZero() && at.After(identity.ReconnectDeadline) {
		identity.LivenessState = RemoteLivenessExpired
		return identity
	}
	if identity.LivenessState == RemoteLivenessConnected && !identity.LastHeartbeatAt.IsZero() && at.Sub(identity.LastHeartbeatAt) > RemoteStaleAfter {
		identity.LivenessState = RemoteLivenessStale
	}
	return identity
}

func identityToMetadata(identity RemoteIdentity) model.RemoteIdentityMetadata {
	return model.RemoteIdentityMetadata{
		ConnectionID:      identity.ConnectionID,
		SessionID:         identity.SessionID,
		ClientIdentity:    identity.ClientIdentity,
		DeviceID:          identity.DeviceID,
		UserID:            identity.UserID,
		AgentID:           identity.AgentID,
		TransportKind:     identity.TransportKind,
		TrustState:        string(identity.TrustState),
		LivenessState:     string(identity.LivenessState),
		ConnectedAt:       identity.ConnectedAt,
		DisconnectedAt:    identity.DisconnectedAt,
		LastHeartbeatAt:   identity.LastHeartbeatAt,
		ReconnectDeadline: identity.ReconnectDeadline,
		Capabilities:      append([]string(nil), identity.Capabilities...),
		Correlation:       cloneStringMap(identity.Correlation),
	}
}

func identitiesToMetadata(identities []RemoteIdentity) []model.RemoteIdentityMetadata {
	out := make([]model.RemoteIdentityMetadata, 0, len(identities))
	for _, identity := range identities {
		out = append(out, identityToMetadata(NormalizeRemoteIdentity(identity)))
	}
	return out
}

func identitiesFromMetadata(records []model.RemoteIdentityMetadata) []RemoteIdentity {
	out := make([]RemoteIdentity, 0, len(records))
	for _, record := range records {
		out = append(out, NormalizeRemoteIdentity(RemoteIdentity{
			ConnectionID:      record.ConnectionID,
			SessionID:         record.SessionID,
			ClientIdentity:    record.ClientIdentity,
			DeviceID:          record.DeviceID,
			UserID:            record.UserID,
			AgentID:           record.AgentID,
			TransportKind:     record.TransportKind,
			TrustState:        RemoteTrustState(record.TrustState),
			LivenessState:     RemoteLivenessState(record.LivenessState),
			ConnectedAt:       record.ConnectedAt,
			DisconnectedAt:    record.DisconnectedAt,
			LastHeartbeatAt:   record.LastHeartbeatAt,
			ReconnectDeadline: record.ReconnectDeadline,
			Capabilities:      append([]string(nil), record.Capabilities...),
			Correlation:       cloneStringMap(record.Correlation),
		}))
	}
	return out
}

func approvalToMetadata(record RemoteApprovalCorrelation) model.RemoteApprovalMetadata {
	return model.RemoteApprovalMetadata{
		LocalApprovalID:     record.LocalApprovalID,
		RemoteCorrelationID: record.RemoteCorrelationID,
		ConnectionID:        record.ConnectionID,
		ClientIdentity:      record.ClientIdentity,
		DeviceID:            record.DeviceID,
		Status:              string(record.Status),
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
		ExpiresAt:           record.ExpiresAt,
		DecisionPayload:     cloneAnyMap(record.DecisionPayload),
	}
}

func approvalsFromMetadata(records []model.RemoteApprovalMetadata) []RemoteApprovalCorrelation {
	out := make([]RemoteApprovalCorrelation, 0, len(records))
	for _, record := range records {
		out = append(out, NormalizeRemoteApprovalCorrelation(RemoteApprovalCorrelation{
			LocalApprovalID:     record.LocalApprovalID,
			RemoteCorrelationID: record.RemoteCorrelationID,
			ConnectionID:        record.ConnectionID,
			ClientIdentity:      record.ClientIdentity,
			DeviceID:            record.DeviceID,
			Status:              RemoteApprovalStatus(record.Status),
			CreatedAt:           record.CreatedAt,
			UpdatedAt:           record.UpdatedAt,
			ExpiresAt:           record.ExpiresAt,
			DecisionPayload:     cloneAnyMap(record.DecisionPayload),
		}))
	}
	return out
}

func upsertRemoteIdentityMetadata(records []model.RemoteIdentityMetadata, record model.RemoteIdentityMetadata) []model.RemoteIdentityMetadata {
	for i, existing := range records {
		if existing.ConnectionID == record.ConnectionID {
			out := append([]model.RemoteIdentityMetadata(nil), records...)
			out[i] = record
			return out
		}
	}
	return append(append([]model.RemoteIdentityMetadata(nil), records...), record)
}

func upsertRemoteApprovalMetadata(records []model.RemoteApprovalMetadata, record model.RemoteApprovalMetadata) []model.RemoteApprovalMetadata {
	for i, existing := range records {
		if existing.RemoteCorrelationID == record.RemoteCorrelationID {
			out := append([]model.RemoteApprovalMetadata(nil), records...)
			out[i] = record
			return out
		}
	}
	return append(append([]model.RemoteApprovalMetadata(nil), records...), record)
}

func compactRemoteStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeRemoteCorrelation(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
