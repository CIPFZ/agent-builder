package queryengine

import (
	"context"
	"fmt"
	"myclaw/internal/model"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"sort"
	"strings"
	"time"
)

func (q *QueryEngine) ExtensionLifecycleRecords() []tools.ExtensionLifecycleRecord {
	if q == nil {
		return nil
	}
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	out := make([]tools.ExtensionLifecycleRecord, 0, len(q.extensionLifecycle))
	for _, record := range q.extensionLifecycle {
		out = append(out, tools.NormalizeExtensionLifecycleRecord(record))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}

func (q *QueryEngine) CommandLifecycleState(sess session.Session, input string) (ExtensionCommand, bool) {
	name := commandNameForError(input)
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "/")
	if name == "" {
		return ExtensionCommand{}, false
	}
	for _, command := range q.extensionCommands(sess.ID) {
		if command.Name == name {
			return command, true
		}
	}
	return ExtensionCommand{}, false
}

func (q *QueryEngine) DisableExtension(target tools.ExtensionLifecycleRecord) (tools.ExtensionLifecycleOperationResult, error) {
	return q.setExtensionLifecycleState("disable", target, tools.ExtensionStateDisabled, "", tools.ExtensionRecoveryPersistedOverlay)
}

func (q *QueryEngine) EnableExtension(target tools.ExtensionLifecycleRecord) (tools.ExtensionLifecycleOperationResult, error) {
	record := tools.NormalizeExtensionLifecycleRecord(target)
	if err := validateExtensionLifecycleTarget(record); err != nil {
		return tools.ExtensionLifecycleOperationResult{}, err
	}
	q.toolContextMu.Lock()
	delete(q.extensionLifecycle, record.Key())
	q.toolContextMu.Unlock()
	q.persistExtensionLifecycleOverlay()
	record.State = tools.ExtensionStateActive
	record.RecoveryBehavior = tools.ExtensionRecoveryRebuildFromDiscovery
	return tools.ExtensionLifecycleOperationResult{Operation: "enable", Record: record}, nil
}

func (q *QueryEngine) ReloadExtension(ctx context.Context, target tools.ExtensionLifecycleRecord) (tools.ExtensionLifecycleOperationResult, error) {
	record := tools.NormalizeExtensionLifecycleRecord(target)
	if err := validateExtensionLifecycleTarget(record); err != nil {
		return tools.ExtensionLifecycleOperationResult{}, err
	}
	if record.Type == tools.ExtensionTypeLSPBoundary || record.Source == "lsp" {
		err := fmt.Errorf("reload unsupported for extension source %q type %q", record.Source, record.Type)
		record.State = tools.ExtensionStateDegraded
		record.LastError = err.Error()
		record.RecoveryBehavior = tools.ExtensionRecoveryUnsupported
		return tools.ExtensionLifecycleOperationResult{Operation: "reload", Record: record, Unsupported: true, Message: err.Error()}, err
	}
	if record.Type == tools.ExtensionTypeMCPServer {
		if _, err := q.ReconnectMCP(ctx, record.Name); err != nil {
			_, _ = q.MarkExtensionFailed(record, err.Error())
			return tools.ExtensionLifecycleOperationResult{}, err
		}
	}
	return q.setExtensionLifecycleState("reload", record, tools.ExtensionStateReloaded, "", tools.ExtensionRecoveryRebuildFromDiscovery)
}

func (q *QueryEngine) MarkExtensionDegraded(target tools.ExtensionLifecycleRecord, message string) (tools.ExtensionLifecycleOperationResult, error) {
	return q.setExtensionLifecycleState("mark_degraded", target, tools.ExtensionStateDegraded, message, tools.ExtensionRecoveryPersistedOverlay)
}

func (q *QueryEngine) MarkExtensionFailed(target tools.ExtensionLifecycleRecord, message string) (tools.ExtensionLifecycleOperationResult, error) {
	return q.setExtensionLifecycleState("mark_failed", target, tools.ExtensionStateFailed, message, tools.ExtensionRecoveryPersistedOverlay)
}

func (q *QueryEngine) setExtensionLifecycleState(operation string, target tools.ExtensionLifecycleRecord, state, message, recovery string) (tools.ExtensionLifecycleOperationResult, error) {
	record := tools.NormalizeExtensionLifecycleRecord(target)
	if err := validateExtensionLifecycleTarget(record); err != nil {
		return tools.ExtensionLifecycleOperationResult{}, err
	}
	record.State = tools.NormalizeExtensionState(state)
	record.LastError = strings.TrimSpace(message)
	record.LastUpdated = time.Now().UTC()
	record.RecoveryBehavior = recovery
	record = tools.NormalizeExtensionLifecycleRecord(record)
	q.toolContextMu.Lock()
	if q.extensionLifecycle == nil {
		q.extensionLifecycle = make(map[string]tools.ExtensionLifecycleRecord)
	}
	q.extensionLifecycle[record.Key()] = record
	q.toolContextMu.Unlock()
	q.persistExtensionLifecycleOverlay()
	return tools.ExtensionLifecycleOperationResult{Operation: operation, Record: record}, nil
}

func validateExtensionLifecycleTarget(record tools.ExtensionLifecycleRecord) error {
	if record.Key() == "" {
		return fmt.Errorf("extension lifecycle target requires type, source, and name")
	}
	return nil
}

func lifecycleRecordsMap(records []tools.ExtensionLifecycleRecord) map[string]tools.ExtensionLifecycleRecord {
	out := make(map[string]tools.ExtensionLifecycleRecord)
	for _, record := range records {
		record = tools.NormalizeExtensionLifecycleRecord(record)
		if key := record.Key(); key != "" {
			out[key] = record
		}
	}
	return out
}

func mergeLifecycleRecords(groups ...[]tools.ExtensionLifecycleRecord) []tools.ExtensionLifecycleRecord {
	byKey := make(map[string]tools.ExtensionLifecycleRecord)
	for _, records := range groups {
		for _, record := range records {
			record = tools.NormalizeExtensionLifecycleRecord(record)
			if key := record.Key(); key != "" {
				byKey[key] = record
			}
		}
	}
	out := make([]tools.ExtensionLifecycleRecord, 0, len(byKey))
	for _, record := range byKey {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func recoveredExtensionLifecycleRecords(sessions *session.Manager) []tools.ExtensionLifecycleRecord {
	if sessions == nil {
		return nil
	}
	var out []tools.ExtensionLifecycleRecord
	for _, sess := range sessions.ListSessions() {
		out = append(out, lifecycleRecordsFromMetadata(sess.Metadata.ExtensionLifecycleOverlays)...)
	}
	return out
}

func lifecycleRecordsFromMetadata(records []model.ExtensionLifecycleMetadata) []tools.ExtensionLifecycleRecord {
	out := make([]tools.ExtensionLifecycleRecord, 0, len(records))
	for _, record := range records {
		out = append(out, tools.NormalizeExtensionLifecycleRecord(tools.ExtensionLifecycleRecord{
			Type:             record.Type,
			Source:           record.Source,
			Name:             record.Name,
			Version:          record.Version,
			State:            record.State,
			Capabilities:     append([]string(nil), record.Capabilities...),
			LastError:        record.LastError,
			LastUpdated:      record.LastUpdated,
			RecoveryBehavior: record.RecoveryBehavior,
		}))
	}
	return out
}

func lifecycleRecordsToMetadata(records []tools.ExtensionLifecycleRecord) []model.ExtensionLifecycleMetadata {
	out := make([]model.ExtensionLifecycleMetadata, 0, len(records))
	for _, record := range records {
		record = tools.NormalizeExtensionLifecycleRecord(record)
		if record.Key() == "" || !shouldPersistExtensionLifecycle(record) {
			continue
		}
		out = append(out, model.ExtensionLifecycleMetadata{
			Type:             record.Type,
			Source:           record.Source,
			Name:             record.Name,
			Version:          record.Version,
			State:            record.State,
			Capabilities:     append([]string(nil), record.Capabilities...),
			LastError:        record.LastError,
			LastUpdated:      record.LastUpdated,
			RecoveryBehavior: record.RecoveryBehavior,
		})
	}
	return out
}

func shouldPersistExtensionLifecycle(record tools.ExtensionLifecycleRecord) bool {
	switch record.State {
	case tools.ExtensionStateDisabled, tools.ExtensionStateDegraded, tools.ExtensionStateFailed:
		return true
	default:
		return record.RecoveryBehavior == tools.ExtensionRecoveryPersistedOverlay
	}
}

func (q *QueryEngine) persistExtensionLifecycleOverlay() {
	if q == nil || q.sessions == nil {
		return
	}
	records := lifecycleRecordsToMetadata(q.ExtensionLifecycleRecords())
	for _, sess := range q.sessions.ListSessions() {
		_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
			metadata.ExtensionLifecycleOverlays = append([]model.ExtensionLifecycleMetadata(nil), records...)
		})
	}
}

func (q *QueryEngine) lifecycleRecord(record tools.ExtensionLifecycleRecord) (tools.ExtensionLifecycleRecord, bool) {
	if q == nil {
		return tools.ExtensionLifecycleRecord{}, false
	}
	record = tools.NormalizeExtensionLifecycleRecord(record)
	if record.Key() == "" {
		return tools.ExtensionLifecycleRecord{}, false
	}
	q.toolContextMu.Lock()
	defer q.toolContextMu.Unlock()
	found, ok := q.extensionLifecycle[record.Key()]
	if !ok {
		return tools.ExtensionLifecycleRecord{}, false
	}
	return tools.NormalizeExtensionLifecycleRecord(found), true
}

func (q *QueryEngine) applyToolLifecycle(item ExtensionTool) ExtensionTool {
	item.Source = strings.ToLower(strings.TrimSpace(item.Source))
	record, ok := q.lifecycleRecord(tools.ExtensionLifecycleRecord{
		Type:         tools.ExtensionTypeTool,
		Source:       item.Source,
		Name:         item.Name,
		Version:      item.Version,
		State:        item.LifecycleState,
		Capabilities: item.Capabilities,
	})
	if !ok {
		return item
	}
	item.LifecycleState = record.State
	item.LastError = record.LastError
	item.LastUpdated = lifecycleTimeString(record.LastUpdated)
	item.RecoveryBehavior = record.RecoveryBehavior
	if len(record.Capabilities) > 0 {
		item.Capabilities = record.Capabilities
	}
	if record.Version != "" {
		item.Version = record.Version
	}
	return item
}

func (q *QueryEngine) toolLifecycleDisabled(def tools.Definition) bool {
	record, ok := q.lifecycleRecord(tools.ExtensionLifecycleRecord{
		Type:   tools.ExtensionTypeTool,
		Source: strings.ToLower(strings.TrimSpace(def.Source)),
		Name:   strings.TrimSpace(def.Name),
	})
	return ok && record.State == tools.ExtensionStateDisabled
}

func (q *QueryEngine) applyCommandLifecycle(item ExtensionCommand) ExtensionCommand {
	item.Source = strings.ToLower(strings.TrimSpace(item.Source))
	record, ok := q.lifecycleRecord(tools.ExtensionLifecycleRecord{
		Type:         tools.ExtensionTypeCommand,
		Source:       item.Source,
		Name:         item.Name,
		Version:      item.Version,
		State:        item.LifecycleState,
		Capabilities: item.Capabilities,
	})
	if !ok {
		return item
	}
	item.LifecycleState = record.State
	item.LastError = record.LastError
	item.LastUpdated = lifecycleTimeString(record.LastUpdated)
	item.RecoveryBehavior = record.RecoveryBehavior
	if len(record.Capabilities) > 0 {
		item.Capabilities = record.Capabilities
	}
	if record.Version != "" {
		item.Version = record.Version
	}
	return item
}

func (q *QueryEngine) applySkillLifecycle(item ExtensionSkill) ExtensionSkill {
	item.Source = strings.ToLower(strings.TrimSpace(item.Source))
	record, ok := q.lifecycleRecord(tools.ExtensionLifecycleRecord{
		Type:         tools.ExtensionTypeSkill,
		Source:       item.Source,
		Name:         item.Name,
		Version:      item.Version,
		State:        item.LifecycleState,
		Capabilities: item.Capabilities,
	})
	if !ok {
		return item
	}
	item.LifecycleState = record.State
	item.LastError = record.LastError
	item.LastUpdated = lifecycleTimeString(record.LastUpdated)
	item.RecoveryBehavior = record.RecoveryBehavior
	if len(record.Capabilities) > 0 {
		item.Capabilities = record.Capabilities
	}
	if record.Version != "" {
		item.Version = record.Version
	}
	return item
}

func (q *QueryEngine) applyMCPServerLifecycle(item MCPServerSnapshot) MCPServerSnapshot {
	item.Source = "mcp"
	item.LifecycleType = tools.ExtensionTypeMCPServer
	if item.LifecycleState == "" {
		switch item.Status {
		case "connected":
			item.LifecycleState = tools.ExtensionStateActive
		case "needs-auth", "error":
			item.LifecycleState = tools.ExtensionStateDegraded
		default:
			item.LifecycleState = tools.ExtensionStateLoaded
		}
	}
	item.RecoveryBehavior = tools.ExtensionRecoveryRebuildFromDiscovery
	record, ok := q.lifecycleRecord(tools.ExtensionLifecycleRecord{
		Type:         tools.ExtensionTypeMCPServer,
		Source:       item.Source,
		Name:         item.Name,
		Version:      item.Version,
		State:        item.LifecycleState,
		Capabilities: item.Capabilities,
	})
	if !ok {
		return item
	}
	item.LifecycleState = record.State
	item.LastError = record.LastError
	if item.LastError != "" {
		item.Error = item.LastError
	}
	item.LastUpdated = lifecycleTimeString(record.LastUpdated)
	item.RecoveryBehavior = record.RecoveryBehavior
	if len(record.Capabilities) > 0 {
		item.Capabilities = record.Capabilities
	}
	if record.Version != "" {
		item.Version = record.Version
	}
	return item
}

func lifecycleTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func mcpServerCapabilities(server MCPServerSnapshot) []string {
	capabilities := make([]string, 0, 4)
	if len(server.Tools) > 0 {
		capabilities = append(capabilities, "tools")
	}
	if len(server.Prompts) > 0 {
		capabilities = append(capabilities, "prompts")
	}
	if len(server.Resources) > 0 {
		capabilities = append(capabilities, "resources")
	}
	if len(server.Skills) > 0 {
		capabilities = append(capabilities, "skills")
	}
	return compactAndSortStrings(capabilities)
}
