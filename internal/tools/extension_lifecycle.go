package tools

import (
	"sort"
	"strings"
	"time"
)

const (
	ExtensionTypeTool        = "tool"
	ExtensionTypeCommand     = "command"
	ExtensionTypeSkill       = "skill"
	ExtensionTypeMCPServer   = "mcp_server"
	ExtensionTypeLSPBoundary = "lsp_boundary"

	ExtensionStateDiscovered = "discovered"
	ExtensionStateLoaded     = "loaded"
	ExtensionStateActive     = "active"
	ExtensionStateDegraded   = "degraded"
	ExtensionStateDisabled   = "disabled"
	ExtensionStateFailed     = "failed"
	ExtensionStateUnloaded   = "unloaded"
	ExtensionStateReloaded   = "reloaded"

	ExtensionRecoveryRebuildFromDiscovery = "rebuildFromDiscovery"
	ExtensionRecoveryPersistedOverlay     = "persistedOverlay"
	ExtensionRecoveryManual               = "manual"
	ExtensionRecoveryUnsupported          = "unsupported"
)

type ExtensionLifecycleRecord struct {
	Type             string
	Source           string
	Name             string
	Version          string
	State            string
	Capabilities     []string
	LastError        string
	LastUpdated      time.Time
	RecoveryBehavior string
}

type ExtensionLifecycleOperationResult struct {
	Operation   string
	Record      ExtensionLifecycleRecord
	Unsupported bool
	Message     string
}

func (r ExtensionLifecycleRecord) Key() string {
	return ExtensionLifecycleKey(r.Type, r.Source, r.Name)
}

func ExtensionLifecycleKey(extensionType, source, name string) string {
	extensionType = strings.ToLower(strings.TrimSpace(extensionType))
	source = strings.ToLower(strings.TrimSpace(source))
	name = strings.TrimSpace(name)
	if extensionType == "" || source == "" || name == "" {
		return ""
	}
	return extensionType + "|" + source + "|" + name
}

func NormalizeExtensionLifecycleRecord(record ExtensionLifecycleRecord) ExtensionLifecycleRecord {
	record.Type = strings.ToLower(strings.TrimSpace(record.Type))
	record.Source = strings.ToLower(strings.TrimSpace(record.Source))
	record.Name = strings.TrimSpace(record.Name)
	record.Version = strings.TrimSpace(record.Version)
	record.State = NormalizeExtensionState(record.State)
	record.Capabilities = compactSortedLifecycleStrings(record.Capabilities)
	record.LastError = strings.TrimSpace(record.LastError)
	record.RecoveryBehavior = normalizeExtensionRecovery(record.RecoveryBehavior)
	return record
}

func NormalizeExtensionState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case ExtensionStateLoaded:
		return ExtensionStateLoaded
	case ExtensionStateActive:
		return ExtensionStateActive
	case ExtensionStateDegraded:
		return ExtensionStateDegraded
	case ExtensionStateDisabled:
		return ExtensionStateDisabled
	case ExtensionStateFailed:
		return ExtensionStateFailed
	case ExtensionStateUnloaded:
		return ExtensionStateUnloaded
	case ExtensionStateReloaded:
		return ExtensionStateReloaded
	default:
		return ExtensionStateDiscovered
	}
}

func normalizeExtensionRecovery(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case strings.ToLower(ExtensionRecoveryPersistedOverlay):
		return ExtensionRecoveryPersistedOverlay
	case strings.ToLower(ExtensionRecoveryManual):
		return ExtensionRecoveryManual
	case strings.ToLower(ExtensionRecoveryUnsupported):
		return ExtensionRecoveryUnsupported
	default:
		return ExtensionRecoveryRebuildFromDiscovery
	}
}

func compactSortedLifecycleStrings(values []string) []string {
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
