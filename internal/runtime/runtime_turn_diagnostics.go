package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/tools/scheduler"
)

var explicitArtifactPathPattern = regexp.MustCompile(`(?i)(?:[A-Z]:[\\/]|/[A-Za-z0-9._ -]+[\\/])(?:[^\s"'<>|:*?]+[\\/])*[^\s"'<>|:*?]+\.(?:md|txt|json|yaml|yml|csv|tsv|html|css|js|jsx|ts|tsx|go|py|rs|java|kt|c|cc|cpp|h|hpp|cs|xml|toml|ini|sql|sh|ps1|bat|cmd|docx|xlsx|pptx|pdf)`)
var shellRedirectArtifactPattern = regexp.MustCompile(`(?i)(?:^|[\s;&|])(?:>|>>|1>|1>>)\s*("[A-Z]:[\\/][^"<>\r\n|?*]+\.[A-Za-z0-9]{1,12}"|'[A-Z]:[\\/][^'<>\r\n|?*]+\.[A-Za-z0-9]{1,12}'|[A-Z]:[\\/][^\s"<>\r\n|?*]+\.[A-Za-z0-9]{1,12})`)

func buildRuntimeTurnDiagnostics(turn RuntimeTurn, messages []RuntimeMessage, toolCalls []RuntimeToolCall, permissions []RuntimePermissionRequest, events []RuntimeEvent) RuntimeTurnDiagnostics {
	now := time.Now().UTC().UnixMilli()
	diag := RuntimeTurnDiagnostics{
		TurnID:             turn.ID,
		SessionID:          turn.SessionID,
		Status:             turn.Status,
		StartedAt:          turn.StartedAt,
		FinishedAt:         turn.FinishedAt,
		DurationMS:         turn.DurationMS,
		ComputedAt:         now,
		ToolCountsByStatus: map[string]int{},
		ToolCountsByKind:   map[string]int{},
	}
	if diag.DurationMS == 0 && turn.StartedAt > 0 && turn.FinishedAt > 0 {
		diag.DurationMS = turn.FinishedAt - turn.StartedAt
		if diag.DurationMS < 0 {
			diag.DurationMS = 0
		}
	}
	if !isFinalTurnStatus(turn.Status) && turn.StartedAt > 0 {
		diag.RunningDurationMS = now - turn.StartedAt
		if diag.RunningDurationMS < 0 {
			diag.RunningDurationMS = 0
		}
	}
	expectedSet := map[string]struct{}{}
	producedSet := map[string]struct{}{}

	for _, path := range extractExplicitArtifactPaths(turn.PromptPreview) {
		expectedSet[path] = struct{}{}
	}
	if turn.UserMessageID != "" {
		for _, msg := range messages {
			if msg.ID != turn.UserMessageID {
				continue
			}
			for _, path := range extractExplicitArtifactPaths(runtimeMessagePlainText(msg)) {
				expectedSet[path] = struct{}{}
			}
			break
		}
	}

	sortedCalls := append([]RuntimeToolCall(nil), toolCalls...)
	slices.SortFunc(sortedCalls, func(a, b RuntimeToolCall) int {
		if a.StartedAt != b.StartedAt {
			return cmpInt64(a.StartedAt, b.StartedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	for _, call := range sortedCalls {
		status := strings.TrimSpace(call.Status)
		if status != "" {
			diag.ToolCountsByStatus[status]++
		}
		kind := normalizedDiagnosticToolKind(call)
		if kind != "" {
			diag.ToolCountsByKind[kind]++
		}
		switch call.Status {
		case string(scheduler.ToolCallFailed):
			diag.FailedToolCount++
		case string(scheduler.ToolCallDenied):
			diag.DeniedToolCount++
		case string(scheduler.ToolCallCancelled):
			diag.CancelledToolCount++
		}
		if isShellToolCall(call) && diagnosticShellExitCode(call) != nil && *diagnosticShellExitCode(call) != 0 {
			diag.NonzeroExitShellCount++
		}
		if strings.TrimSpace(call.Status) != "" {
			diag.LastToolID = call.ID
			diag.LastToolStatus = call.Status
			diag.LastToolTitle = firstNonEmpty(call.Display.Title, call.Display.Detail, call.Name)
		}
		for _, path := range producedArtifactsFromToolCall(call) {
			producedSet[path] = struct{}{}
		}
		applyArtifactConfidenceFromToolCall(&diag, call)
	}
	if len(diag.ToolCountsByStatus) == 0 {
		diag.ToolCountsByStatus = nil
	}
	if len(diag.ToolCountsByKind) == 0 {
		diag.ToolCountsByKind = nil
	}

	diag.PermissionCounts = permissionCountsForTurn(turn.ID, permissions)
	diag.LastRuntimeEventAt, diag.LastRuntimeEventSequence = lastRuntimeEventForTurn(events)

	diag.ExpectedArtifacts = sortedMapKeys(expectedSet)
	diag.ProducedArtifacts = sortedMapKeys(producedSet)
	if len(diag.ProducedArtifacts) > diag.ArtifactConfidenceSummary.ProducedToolMetadata {
		diag.ArtifactConfidenceSummary.ProducedToolMetadata = len(diag.ProducedArtifacts)
	}
	if isFinalTurnStatus(turn.Status) && len(diag.ExpectedArtifacts) > 0 {
		diag.ArtifactVerificationAt = now
		for _, expected := range diag.ExpectedArtifacts {
			if artifactExistsOnDisk(expected) {
				diag.VerifiedArtifacts = append(diag.VerifiedArtifacts, expected)
				diag.ArtifactConfidenceSummary.LocalVerifiedFile++
				continue
			}
			diag.MissingArtifacts = append(diag.MissingArtifacts, expected)
			diag.UnverifiedArtifacts = append(diag.UnverifiedArtifacts, expected)
		}
	}
	if turn.Status == turnStatusCompleted && len(diag.MissingArtifacts) > 0 {
		setArtifactWarning(&diag, producedSet)
	}
	diag.ArtifactCounts = RuntimeArtifactCounts{
		Expected:             len(diag.ExpectedArtifacts),
		Produced:             len(diag.ProducedArtifacts),
		Verified:             len(diag.VerifiedArtifacts),
		Missing:              len(diag.MissingArtifacts),
		LocalDeliverables:    len(diag.ProducedArtifacts),
		RuntimeRefs:          diag.ArtifactConfidenceSummary.RuntimeOutputRefs,
		ProducedMetadataRefs: diag.ArtifactConfidenceSummary.ProducedToolMetadata,
		StructuredRefs:       diag.ArtifactConfidenceSummary.StructuredMCPCustomRefs,
	}
	if len(diag.ExpectedArtifacts) > 0 && len(diag.ProducedArtifacts) == 0 {
		diag.ArtifactConfidenceSummary.UnknownNotDetected = len(diag.ExpectedArtifacts)
	}
	return diag
}

func permissionCountsForTurn(turnID string, permissions []RuntimePermissionRequest) RuntimePermissionCounts {
	var counts RuntimePermissionCounts
	for _, perm := range permissions {
		if turnID != "" && perm.TurnID != turnID {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(perm.Status)) {
		case "", permissionStatusPending, "ask":
			counts.Pending++
		case permissionStatusAllowedOnce, permissionStatusAllowedSession, "allowed", "allow", "allow_once", "allow_for_session":
			counts.Allowed++
		case permissionStatusDenied, "deny":
			counts.Denied++
		case permissionStatusExpired:
			counts.Expired++
		case permissionStatusCancelled, "canceled":
			counts.Cancelled++
		}
	}
	return counts
}

func lastRuntimeEventForTurn(events []RuntimeEvent) (int64, int64) {
	var lastAt, lastSequence int64
	for _, event := range events {
		if event.Sequence >= lastSequence {
			lastSequence = event.Sequence
			if parsed := runtimeEventCreatedAtMillis(event.CreatedAt); parsed > 0 {
				lastAt = parsed
			}
		}
	}
	return lastAt, lastSequence
}

func runtimeEventCreatedAtMillis(createdAt string) int64 {
	createdAt = strings.TrimSpace(createdAt)
	if createdAt == "" {
		return 0
	}
	if parsed, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		return parsed.UTC().UnixMilli()
	}
	if parsed, err := time.Parse(time.RFC3339, createdAt); err == nil {
		return parsed.UTC().UnixMilli()
	}
	return 0
}

func normalizedDiagnosticToolKind(call RuntimeToolCall) string {
	kind := strings.TrimSpace(call.Display.Kind)
	if kind != "" {
		return kind
	}
	if isShellToolCall(call) {
		return "shell"
	}
	return "generic"
}

func diagnosticShellExitCode(call RuntimeToolCall) *int {
	if call.Display.ExitCode != nil {
		return call.Display.ExitCode
	}
	if call.ExitCode != 0 {
		return &call.ExitCode
	}
	return nil
}

func applyArtifactConfidenceFromToolCall(diag *RuntimeTurnDiagnostics, call RuntimeToolCall) {
	metadataRefs := normalizeArtifactPaths(call.ArtifactRefs)
	if len(call.Display.ArtifactRefs) > len(metadataRefs) {
		metadataRefs = normalizeArtifactPaths(call.Display.ArtifactRefs)
	}
	diag.ArtifactConfidenceSummary.ProducedToolMetadata += len(metadataRefs)
	diag.ArtifactConfidenceSummary.RuntimeOutputRefs += len(call.OutputRefs)
	if isCustomArtifactToolCall(call) {
		diag.ArtifactConfidenceSummary.StructuredMCPCustomRefs += len(artifactPathsFromStructuredToolOutput(call))
	}
}

func extractExplicitArtifactPaths(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	matches := explicitArtifactPathPattern.FindAllString(text, -1)
	seen := map[string]struct{}{}
	for _, match := range matches {
		path := normalizeArtifactPath(match)
		if path == "" {
			continue
		}
		seen[path] = struct{}{}
	}
	return sortedMapKeys(seen)
}

func producedArtifactsFromToolCall(call RuntimeToolCall) []string {
	name := strings.ToLower(strings.TrimSpace(call.Name))
	if call.Status != string(scheduler.ToolCallCompleted) {
		return nil
	}
	paths := normalizeArtifactPaths(call.ArtifactRefs)
	if isShellToolCall(call) {
		paths = append(paths, artifactPathsFromStructuredToolOutput(call)...)
		paths = append(paths, shellCreatedArtifactPaths(call.Command)...)
		paths = append(paths, shellCreatedArtifactPaths(call.InputSummary)...)
		return uniqueSortedStrings(paths)
	}
	if isCustomArtifactToolCall(call) {
		paths = append(paths, artifactPathsFromStructuredToolOutput(call)...)
	}
	switch name {
	case "write", "download":
	default:
		if call.Display.Kind != "file_write" {
			if isCustomArtifactToolCall(call) {
				if target := normalizeArtifactPath(call.Display.Target); target != "" {
					paths = append(paths, target)
				}
			}
			return uniqueSortedStrings(paths)
		}
	}
	if target := normalizeArtifactPath(call.Display.Target); target != "" {
		paths = append(paths, target)
	}
	for _, path := range explicitArtifactPathsFromToolInput(call.InputSummary, []string{"file_path", "filepath", "path", "file", "target"}) {
		paths = append(paths, path)
	}
	return uniqueSortedStrings(paths)
}

func artifactExistsOnDisk(path string) bool {
	if normalizeArtifactPath(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func setArtifactWarning(diag *RuntimeTurnDiagnostics, producedSet map[string]struct{}) {
	for _, missing := range diag.MissingArtifacts {
		if _, ok := producedSet[missing]; ok {
			diag.Warning = "expected artifact was produced by a tool but is missing on disk"
			if len(diag.MissingArtifacts) > 1 {
				diag.Warning = "expected artifacts were produced by tools but are missing on disk"
			}
			diag.WarningReason = "produced_artifact_missing_on_disk"
			diag.WarningSource = "filesystem"
			return
		}
	}
	diag.Warning = "expected artifact was not produced"
	if len(diag.MissingArtifacts) > 1 {
		diag.Warning = "expected artifacts were not produced"
	}
	diag.WarningReason = "expected_artifact_not_produced"
	diag.WarningSource = "tool_metadata"
}

func isShellToolCall(call RuntimeToolCall) bool {
	name := strings.ToLower(strings.TrimSpace(call.Name))
	source := strings.ToLower(strings.TrimSpace(call.Source))
	return call.Display.Kind == "shell" || source == "shell" || call.Command != "" || call.Risk == "execute" ||
		name == "bash" || name == "shell" || name == "cmd" || name == "powershell" || name == "pwsh"
}

func isCustomArtifactToolCall(call RuntimeToolCall) bool {
	source := strings.ToLower(strings.TrimSpace(call.Source))
	return source == "mcp" || source == "plugin" || source == "custom"
}

func shellCreatedArtifactPaths(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	var paths []string
	for _, match := range shellRedirectArtifactPattern.FindAllStringSubmatch(command, -1) {
		if len(match) > 1 {
			if path := normalizeArtifactPath(match[1]); path != "" {
				paths = append(paths, path)
			}
		}
	}
	lower := strings.ToLower(command)
	for _, verb := range []string{"set-content", "out-file", "add-content", "new-item"} {
		if !strings.Contains(lower, verb) {
			continue
		}
		for _, path := range extractExplicitArtifactPaths(command) {
			paths = append(paths, path)
		}
	}
	return uniqueSortedStrings(paths)
}

func artifactPathsFromStructuredToolOutput(call RuntimeToolCall) []string {
	var paths []string
	for _, text := range []string{call.Structured, call.OutputSummary, call.ModelContent} {
		paths = append(paths, artifactPathsFromStructuredJSON(text)...)
	}
	return uniqueSortedStrings(paths)
}

func artifactPathsFromStructuredJSON(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" || (!strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[")) {
		return nil
	}
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil
	}
	var paths []string
	collectArtifactPathsFromValue(payload, "", &paths)
	return uniqueSortedStrings(paths)
}

func collectArtifactPathsFromValue(value any, key string, paths *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			collectArtifactPathsFromValue(childValue, childKey, paths)
		}
	case []any:
		for _, childValue := range typed {
			collectArtifactPathsFromValue(childValue, key, paths)
		}
	case string:
		if !isArtifactPathField(key) {
			return
		}
		if path := normalizeArtifactPath(typed); path != "" {
			*paths = append(*paths, path)
		}
	}
}

func isArtifactPathField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "path", "file_path", "filepath", "file", "target", "display_target", "displaytarget", "artifact", "artifact_path", "artifactpath", "output_path", "outputpath", "uri", "ref", "refs", "artifact_ref", "artifactrefs", "artifact_refs":
		return true
	default:
		return strings.Contains(normalized, "artifact") || strings.HasSuffix(normalized, "path")
	}
}

func explicitArtifactPathsFromToolInput(input string, keys []string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err == nil {
		var paths []string
		for _, key := range keys {
			if value, ok := payload[key].(string); ok {
				if path := normalizeArtifactPath(value); path != "" {
					paths = append(paths, path)
				}
			}
		}
		return uniqueSortedStrings(paths)
	}
	return extractExplicitArtifactPaths(input)
}

func runtimeMessagePlainText(msg RuntimeMessage) string {
	var parts []string
	if strings.TrimSpace(msg.Content) != "" {
		parts = append(parts, msg.Content)
	}
	for _, part := range msg.Parts {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeArtifactPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if normalized := normalizeArtifactPath(path); normalized != "" {
			out = append(out, normalized)
		}
	}
	return uniqueSortedStrings(out)
}

func normalizeArtifactPath(path string) string {
	path = strings.Trim(strings.TrimSpace(path), ".,;:)]}")
	path = strings.Trim(path, "`\"'")
	if path == "" || filepath.Ext(path) == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return filepath.Clean(path)
	}
	return ""
}

func sortedMapKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	return sortedMapKeys(seen)
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
