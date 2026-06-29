package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func (r *runtimeService) resolveReadFilePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := ""
	if r.workspace != nil {
		base = r.workspace.Path
	}
	if base == "" {
		if cwd, err := os.Getwd(); err == nil {
			base = cwd
		}
	}
	return filepath.Clean(filepath.Join(base, path))
}

func reinjectedRefFromContextSource(source RuntimeContextSource) RuntimeReinjectedRef {
	return RuntimeReinjectedRef{
		ID:             source.ID,
		Kind:           reinjectionKindForContextSource(source),
		Name:           source.Name,
		Path:           source.Path,
		URI:            source.URI,
		Ref:            reinjectionRefURI(source),
		ContentSummary: source.ContentSummary,
		TokenEstimate:  source.TokenEstimate,
	}
}

func reinjectionKindForContextSource(source RuntimeContextSource) string {
	if strings.EqualFold(source.Name, "AGENTS.md") || strings.EqualFold(filepath.Base(source.Path), "AGENTS.md") {
		return "agents"
	}
	if strings.EqualFold(source.Name, "CLAUDE.md") || strings.EqualFold(filepath.Base(source.Path), "CLAUDE.md") {
		return "claude"
	}
	if source.Kind != "" {
		return "context_source:" + source.Kind
	}
	return "context_source"
}

func reinjectionRefURI(source RuntimeContextSource) string {
	switch {
	case source.URI != "":
		return source.URI
	case source.Path != "":
		return "runtime://context-sources/" + filepath.ToSlash(source.Path)
	default:
		return "runtime://context-sources/" + source.ID
	}
}

func (r *runtimeService) publishReinjectionEvent(eventType, sessionID, turnID, boundaryID string, ref RuntimeReinjectedRef) {
	payload := map[string]any{
		"compact_id":      boundaryID,
		"source_id":       ref.ID,
		"kind":            ref.Kind,
		"name":            ref.Name,
		"path":            ref.Path,
		"uri":             ref.URI,
		"ref":             ref.Ref,
		"status":          ref.Status,
		"reason":          ref.Reason,
		"error":           ref.Error,
		"token_estimate":  ref.TokenEstimate,
		"content_summary": ref.ContentSummary,
	}
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      eventType,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		TurnID:    turnID,
		Payload:   payload,
	})
	r.writeAudit(auditEntry{
		RequestID: turnID,
		Event:     strings.ReplaceAll(eventType, ".", "_"),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Extra: map[string]any{
			"compact_id":     boundaryID,
			"reinjected_ref": ref,
		},
		Error: ref.Error,
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
