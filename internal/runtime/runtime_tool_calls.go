package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mcptools "github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

type runtimeToolCallStore = *scheduler.Scheduler

func NewRuntimeToolCallStore() scheduler.Store {
	return scheduler.NewMemoryStore()
}

func NewRuntimeToolCallStoreForDB(db *sql.DB) scheduler.Store {
	return newRuntimeSQLiteToolCallStore(db)
}

func (r *runtimeService) ToolCall(ctx context.Context, toolCallID string) (RuntimeToolCallResponse, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return RuntimeToolCallResponse{}, errors.New("tool call id is required")
	}
	call, err := r.toolCalls.GetCall(ctx, toolCallID)
	if err != nil {
		return RuntimeToolCallResponse{}, fmt.Errorf("tool call %s was not found: %w", toolCallID, err)
	}
	return RuntimeToolCallResponse{ToolCall: toRuntimeToolCall(call)}, nil
}

func (r *runtimeService) TurnToolCalls(ctx context.Context, turnID string) (RuntimeToolCallsResponse, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return RuntimeToolCallsResponse{}, errors.New("turn id is required")
	}
	calls, err := r.toolCalls.ListCalls(ctx, turnID)
	if err != nil {
		return RuntimeToolCallsResponse{}, err
	}
	out := make([]RuntimeToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, toRuntimeToolCall(call))
	}
	return RuntimeToolCallsResponse{ToolCalls: out}, nil
}

// reconcileTerminalRuntimeToolCalls replays persisted tool results before a
// turn becomes terminal. Scheduler callbacks normally finish calls eagerly,
// but message persistence is the durable evidence delivered to the model. A
// missed or reordered recorder callback must not leave canonical conversation
// state showing a tool as running after the turn has finished.
func (r *runtimeService) reconcileTerminalRuntimeToolCalls(ctx context.Context, sessionID, turnID string) []scheduler.ToolCall {
	if r.toolCalls == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return nil
	}
	messages, err := r.runtime.ListSessionMessages(ctx, r.workspace.ID, sessionID)
	if err != nil {
		return nil
	}
	var reconciled []scheduler.ToolCall
	for _, persisted := range messages {
		msg := toAPITypeMessage(persisted)
		for _, result := range msg.ToolResults() {
			call, getErr := r.toolCalls.GetCall(ctx, result.ToolCallID)
			if getErr != nil || call.TurnID != turnID || isFinalToolCallStatus(string(call.Status)) {
				continue
			}
			status := scheduler.ToolCallCompleted
			errText := ""
			if result.IsError {
				status = scheduler.ToolCallFailed
				errText = preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit)
			}
			updated, completeErr := r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
				ToolCallID:    result.ToolCallID,
				Status:        status,
				OutputSummary: preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit),
				IsError:       result.IsError,
				Error:         errText,
			})
			if completeErr == nil {
				reconciled = append(reconciled, updated)
			}
		}
	}
	return reconciled
}

func cancelUnfinishedRuntimeToolCalls(ctx context.Context, calls *scheduler.Scheduler, db *sql.DB) ([]scheduler.ToolCall, error) {
	if calls == nil || db == nil {
		return nil, nil
	}
	unfinished, err := listUnfinishedRuntimeToolCalls(ctx, db)
	if err != nil {
		return nil, err
	}
	cancelled := make([]scheduler.ToolCall, 0, len(unfinished))
	for _, call := range unfinished {
		if isFinalToolCallStatus(string(call.Status)) {
			continue
		}
		if err := calls.CancelCall(ctx, call.ID); err != nil {
			return nil, err
		}
		updated, err := calls.GetCall(ctx, call.ID)
		if err != nil {
			return nil, err
		}
		cancelled = append(cancelled, updated)
	}
	return cancelled, nil
}

func listUnfinishedRuntimeToolCalls(ctx context.Context, db *sql.DB) ([]scheduler.ToolCall, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, turn_id, session_id, message_id, name, source, capability_id, status,
    job_id, command, command_ref, command_byte_length, risk, policy_reason, policy_mode, policy_profile, policy_headless,
    policy_headless_reason, policy_rule_id, policy_rule_source, policy_scope_kind, policy_scope_value, policy_target_summary,
    shell_risk, shell_reason, sandbox_decision_id, sandbox_mode, sandbox_status,
    sandbox_executor, sandbox_reason, sandbox_error, exit_code, job_status, job_started_at, job_finished_at,
    input_summary, input_ref, input_byte_length, output_summary, model_content, structured_output, stdout, stderr, is_error,
    output_refs_json, artifact_refs_json, diff_refs_json,
    compacted, compact_ref, compact_boundary_id, compact_original_estimated_tokens, compacted_at,
    started_at, finished_at, error
FROM runtime_tool_calls
WHERE status NOT IN ('completed', 'failed', 'cancelled', 'denied')
ORDER BY started_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list unfinished runtime tool calls: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []scheduler.ToolCall
	for rows.Next() {
		call, err := scanRuntimeToolCall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate unfinished runtime tool calls: %w", err)
	}
	return out, nil
}

func (r *runtimeService) recordToolCallsFromMessage(ctx context.Context, msg apitypes.Message, turnID string, createdAt time.Time) {
	if r.toolCalls == nil {
		return
	}
	for _, call := range msg.ToolCalls() {
		if call.ID == "" {
			continue
		}
		if existing, err := r.toolCalls.GetCall(ctx, call.ID); err == nil && isFinalToolCallStatus(string(existing.Status)) {
			continue
		}
		_, _ = r.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
			ID:           call.ID,
			SessionID:    msg.SessionID,
			TurnID:       turnID,
			MessageID:    msg.ID,
			Name:         call.Name,
			Source:       classifyToolSource(call.Name),
			CapabilityID: capabilityIDForToolName(call.Name),
			InputSummary: preview(call.Input, runtimePartPreviewLimit),
		})
	}
	for _, result := range msg.ToolResults() {
		if result.ToolCallID == "" {
			continue
		}
		status := scheduler.ToolCallCompleted
		errText := ""
		if result.IsError {
			status = scheduler.ToolCallFailed
			errText = preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit)
		}
		if existing, err := r.toolCalls.GetCall(ctx, result.ToolCallID); err == nil && isFinalToolCallStatus(string(existing.Status)) {
			continue
		}
		_, err := r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
			ToolCallID:    result.ToolCallID,
			Status:        status,
			OutputSummary: preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit),
			IsError:       result.IsError,
			Error:         errText,
		})
		if err == nil {
			continue
		}
		_, _ = r.toolCalls.CreateCall(ctx, scheduler.ToolCallRequest{
			ID:           result.ToolCallID,
			SessionID:    msg.SessionID,
			TurnID:       turnID,
			MessageID:    msg.ID,
			Name:         result.Name,
			Source:       classifyToolSource(result.Name),
			CapabilityID: capabilityIDForToolName(result.Name),
		})
		_, _ = r.toolCalls.CompleteCall(ctx, scheduler.ToolCallResult{
			ToolCallID:    result.ToolCallID,
			Status:        status,
			OutputSummary: preview(firstNonEmpty(result.Content, result.Data), runtimePartPreviewLimit),
			IsError:       result.IsError,
			Error:         errText,
		})
	}
	_ = createdAt
}

func toRuntimeToolCall(call scheduler.ToolCall) RuntimeToolCall {
	var finishedAt int64
	if !call.FinishedAt.IsZero() {
		finishedAt = call.FinishedAt.UnixMilli()
	}
	var jobStartedAt int64
	if !call.JobStartedAt.IsZero() {
		jobStartedAt = call.JobStartedAt.UnixMilli()
	}
	var jobFinishedAt int64
	if !call.JobFinishedAt.IsZero() {
		jobFinishedAt = call.JobFinishedAt.UnixMilli()
	}
	var compactedAt int64
	if !call.CompactedAt.IsZero() {
		compactedAt = call.CompactedAt.UnixMilli()
	}
	redacted := redactRuntimePayload(map[string]any{
		"input":                  call.InputSummary,
		"output":                 call.OutputSummary,
		"model_content":          call.ModelContent,
		"structured":             call.Structured,
		"stdout":                 call.Stdout,
		"stderr":                 call.Stderr,
		"error":                  call.Error,
		"command":                call.Command,
		"policy_reason":          call.PolicyReason,
		"policy_target_summary":  call.PolicyTargetSummary,
		"policy_headless_reason": call.PolicyHeadlessReason,
		"shell_reason":           call.ShellReason,
		"sandbox_reason":         call.SandboxReason,
		"sandbox_error":          call.SandboxError,
	})
	display := runtimeToolCallDisplay(call, redacted, finishedAt)
	status := runtimeNormalizeToolStatus(string(call.Status), display.Kind, call.ExitCode, call.IsError, stringFromMap(redacted, "error"))
	quiet := runtimeToolQuiet(display.Kind, status)
	groupable := runtimeToolGroupable(display.Kind, status)
	groupKey := runtimeToolGroupKey(call.TurnID)
	return RuntimeToolCall{
		ID:                             call.ID,
		SessionID:                      call.SessionID,
		TurnID:                         call.TurnID,
		MessageID:                      call.MessageID,
		Name:                           call.Name,
		Source:                         string(call.Source),
		CapabilityID:                   call.CapabilityID,
		Kind:                           display.Kind,
		JobID:                          call.JobID,
		Command:                        stringFromMap(redacted, "command"),
		Risk:                           call.Risk,
		PolicyReason:                   stringFromMap(redacted, "policy_reason"),
		PolicyMode:                     call.PolicyMode,
		PolicyProfile:                  call.PolicyProfile,
		PolicyHeadless:                 call.PolicyHeadless,
		PolicyHeadlessReason:           stringFromMap(redacted, "policy_headless_reason"),
		PolicyRuleID:                   call.PolicyRuleID,
		PolicyRuleSource:               call.PolicyRuleSource,
		PolicyScopeKind:                call.PolicyScopeKind,
		PolicyScopeValue:               call.PolicyScopeValue,
		PolicyTargetSummary:            stringFromMap(redacted, "policy_target_summary"),
		ShellRisk:                      call.ShellRisk,
		ShellReason:                    stringFromMap(redacted, "shell_reason"),
		SandboxDecisionID:              call.SandboxDecisionID,
		SandboxMode:                    call.SandboxMode,
		SandboxStatus:                  call.SandboxStatus,
		SandboxExecutor:                call.SandboxExecutor,
		SandboxReason:                  stringFromMap(redacted, "sandbox_reason"),
		SandboxError:                   stringFromMap(redacted, "sandbox_error"),
		ExitCode:                       call.ExitCode,
		JobStatus:                      call.JobStatus,
		JobStartedAt:                   jobStartedAt,
		JobFinishedAt:                  jobFinishedAt,
		Status:                         status,
		InputSummary:                   stringFromMap(redacted, "input"),
		OutputSummary:                  stringFromMap(redacted, "output"),
		ModelContent:                   stringFromMap(redacted, "model_content"),
		Structured:                     stringFromMap(redacted, "structured"),
		Stdout:                         stringFromMap(redacted, "stdout"),
		Stderr:                         stringFromMap(redacted, "stderr"),
		OutputRefs:                     append([]string(nil), call.OutputRefs...),
		ArtifactRefs:                   append([]string(nil), call.ArtifactRefs...),
		DiffRefs:                       append([]string(nil), call.DiffRefs...),
		IsError:                        call.IsError,
		Compacted:                      call.Compacted,
		CompactRef:                     call.CompactRef,
		CompactBoundaryID:              call.CompactBoundaryID,
		CompactOriginalEstimatedTokens: call.CompactOriginalEstimatedTokens,
		CompactedAt:                    compactedAt,
		StartedAt:                      call.StartedAt.UnixMilli(),
		FinishedAt:                     finishedAt,
		Error:                          stringFromMap(redacted, "error"),
		Display:                        display,
		GroupKey:                       groupKey,
		Groupable:                      groupable,
		Quiet:                          quiet,
		DefaultExpanded:                runtimeToolDefaultExpanded(status),
	}
}

func runtimeToolCallDisplay(call scheduler.ToolCall, redacted map[string]any, finishedAt int64) RuntimeToolCallDisplay {
	kind := runtimeToolDisplayKind(call)
	input := stringFromMap(redacted, "input")
	output := stringFromMap(redacted, "output")
	command := firstNonEmpty(stringFromMap(redacted, "command"), runtimeToolDisplayStringField(input, []string{"command", "cmd", "script"}))
	workingDir := runtimeToolDisplayStringField(input, []string{"cwd", "working_dir", "workdir", "effective_cwd"})
	targets := runtimeToolDisplayTargets(call, kind, input)
	target := ""
	if len(targets) > 0 {
		target = targets[0]
	}
	detail := runtimeToolDisplayDetail(kind, command, workingDir, target, input)
	title := runtimeToolDisplayTitle(call, kind)
	var durationMS int64
	if !call.StartedAt.IsZero() && finishedAt > 0 {
		durationMS = finishedAt - call.StartedAt.UnixMilli()
		if durationMS < 0 {
			durationMS = 0
		}
	}
	artifactRefs := append([]string(nil), call.ArtifactRefs...)
	diffRefs := append([]string(nil), call.DiffRefs...)
	diffCount := runtimeToolDisplayDiffCount(call, kind, diffRefs)
	artifactCount := len(artifactRefs)
	if artifactTargets := runtimeToolDisplayArtifactTargets(call, kind); len(artifactTargets) > artifactCount {
		artifactCount = len(artifactTargets)
	}
	if kind == "file_write" && len(targets) > artifactCount {
		artifactCount = len(targets)
	}
	var exitCode *int
	if kind == "shell" || call.ExitCode != 0 {
		value := call.ExitCode
		exitCode = &value
	}
	return RuntimeToolCallDisplay{
		Kind:            kind,
		Title:           title,
		Detail:          detail,
		Target:          target,
		PrimaryTarget:   target,
		Targets:         targets,
		WorkingDir:      workingDir,
		Command:         command,
		ExitCode:        exitCode,
		DurationMS:      durationMS,
		StdoutExcerpt:   stableToolExcerpt(stringFromMap(redacted, "stdout")),
		StderrExcerpt:   stableToolExcerpt(stringFromMap(redacted, "stderr")),
		InputExcerpt:    stableToolExcerpt(input),
		OutputExcerpt:   stableToolExcerpt(output),
		FailureReason:   runtimeToolDisplayFailureReason(call, redacted),
		ArtifactCount:   artifactCount,
		DiffCount:       diffCount,
		ArtifactRefs:    artifactRefs,
		DiffRefs:        diffRefs,
		ArtifactSummary: runtimeToolRefsSummary(artifactRefs, "artifact"),
		DiffSummary:     runtimeToolDisplayDiffSummary(diffCount, diffRefs),
	}
}

func runtimeToolDisplayFailureReason(call scheduler.ToolCall, redacted map[string]any) string {
	status := string(call.Status)
	kind := runtimeToolDisplayKind(call)
	shellExitFailed := kind == "shell" && call.ExitCode != 0
	if status != string(scheduler.ToolCallFailed) && status != string(scheduler.ToolCallDenied) && status != string(scheduler.ToolCallCancelled) && !call.IsError && !shellExitFailed {
		return ""
	}
	for _, value := range []string{
		stringFromMap(redacted, "error"),
		stringFromMap(redacted, "stderr"),
		stringFromMap(redacted, "output"),
		stringFromMap(redacted, "sandbox_error"),
		stringFromMap(redacted, "policy_reason"),
	} {
		if excerpt := stableToolExcerpt(value); excerpt != "" {
			return excerpt
		}
	}
	return ""
}

func runtimeToolDisplayArtifactTargets(call scheduler.ToolCall, kind string) []string {
	var targets []string
	targets = append(targets, normalizeArtifactPaths(call.ArtifactRefs)...)
	if kind == "shell" {
		targets = append(targets, shellCreatedArtifactPaths(call.Command)...)
		targets = append(targets, artifactPathsFromStructuredJSON(call.Structured)...)
		targets = append(targets, artifactPathsFromStructuredJSON(call.OutputSummary)...)
	}
	if call.Source == scheduler.ToolSourceMCP || call.Source == "plugin" || call.Source == "custom" {
		targets = append(targets, artifactPathsFromStructuredJSON(call.Structured)...)
		targets = append(targets, artifactPathsFromStructuredJSON(call.OutputSummary)...)
	}
	return uniqueSortedStrings(targets)
}

func runtimeToolDisplayDetail(kind, command, workingDir, target, input string) string {
	if kind == "shell" {
		return firstNonEmpty(command, input)
	}
	if workingDir != "" && target != "" {
		return target
	}
	return firstNonEmpty(target, command, input)
}

func runtimeToolDisplayTarget(input string) string {
	targets := runtimeToolDisplayTargetsFromInput(input)
	if len(targets) > 0 {
		return targets[0]
	}
	return ""
}

func runtimeToolDisplayTargets(call scheduler.ToolCall, kind, input string) []string {
	var targets []string
	switch kind {
	case "shell":
		targets = append(targets, shellCreatedArtifactPaths(firstNonEmpty(call.Command, input))...)
	case "generic":
		targets = append(targets, normalizeArtifactPaths(call.ArtifactRefs)...)
		targets = append(targets, artifactPathsFromStructuredJSON(call.Structured)...)
		targets = append(targets, artifactPathsFromStructuredJSON(call.OutputSummary)...)
		targets = append(targets, runtimeToolDisplayTargetsFromInput(input)...)
	default:
		targets = append(targets, runtimeToolDisplayTargetsFromInput(input)...)
	}
	if len(targets) == 0 && kind == "file_edit" {
		targets = normalizeArtifactPaths(call.DiffRefs)
	}
	return uniqueSortedStrings(targets)
}

func runtimeToolDisplayTargetsFromInput(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" || !strings.HasPrefix(input, "{") {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		target := runtimeToolDisplayTargetFromText(input)
		if target == "" {
			return nil
		}
		return []string{target}
	}
	var targets []string
	for _, key := range []string{"path", "file_path", "filepath", "file", "target", "uri", "pattern", "query", "glob", "display_target", "output_path", "artifact_path"} {
		targets = append(targets, runtimeToolDisplayValuesForKey(payload[key])...)
	}
	for _, key := range []string{"paths", "files", "targets", "uris", "patterns", "artifact_refs", "artifacts", "diff_refs"} {
		targets = append(targets, runtimeToolDisplayValuesForKey(payload[key])...)
	}
	if len(targets) == 0 {
		if target := runtimeToolDisplayTargetFromText(input); target != "" {
			targets = append(targets, target)
		}
	}
	return uniqueSortedStrings(targets)
}

func runtimeToolDisplayValuesForKey(value any) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed != "" {
			return []string{trimmed}
		}
	case []any:
		var values []string
		for _, item := range typed {
			values = append(values, runtimeToolDisplayValuesForKey(item)...)
		}
		return values
	case map[string]any:
		var values []string
		for _, key := range []string{"path", "file_path", "filepath", "file", "target", "uri", "ref"} {
			values = append(values, runtimeToolDisplayValuesForKey(typed[key])...)
		}
		return values
	}
	return nil
}

func runtimeToolDisplayStringField(input string, keys []string) string {
	input = strings.TrimSpace(input)
	if input == "" || !strings.HasPrefix(input, "{") {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return ""
	}
	for _, key := range keys {
		if value, ok := payload[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func runtimeToolDisplayTargetFromText(input string) string {
	for _, key := range []string{"file_path", "filepath", "path", "file", "target", "uri", "pattern", "query", "glob"} {
		marker := `"` + key + `"`
		start := strings.Index(input, marker)
		if start < 0 {
			continue
		}
		remainder := input[start+len(marker):]
		colon := strings.Index(remainder, ":")
		if colon < 0 {
			continue
		}
		remainder = strings.TrimSpace(remainder[colon+1:])
		if !strings.HasPrefix(remainder, `"`) {
			continue
		}
		remainder = remainder[1:]
		end := strings.Index(remainder, `"`)
		if end <= 0 {
			continue
		}
		if value := strings.TrimSpace(remainder[:end]); value != "" {
			return value
		}
	}
	return ""
}

func runtimeToolDisplayKind(call scheduler.ToolCall) string {
	return runtimeToolPolicyDisplayKind(call)
}

func stableToolExcerpt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const limit = 2000
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "\n... truncated ..."
}

func runtimeToolRefsSummary(refs []string, label string) string {
	if len(refs) == 0 {
		return ""
	}
	return runtimeToolCountSummary(len(refs), label+" ref")
}

func runtimeToolCountSummary(count int, label string) string {
	if count == 0 {
		return ""
	}
	if count == 1 {
		return "1 " + label
	}
	return fmt.Sprintf("%d %ss", count, label)
}

func runtimeToolDisplayDiffCount(call scheduler.ToolCall, kind string, diffRefs []string) int {
	if len(diffRefs) > 0 {
		return len(diffRefs)
	}
	if kind != "file_edit" {
		return 0
	}
	for _, text := range []string{call.Structured, call.OutputSummary, call.ModelContent} {
		if runtimeToolDisplayEditOutputHasDiffSummary(text) {
			return 1
		}
	}
	return 0
}

func runtimeToolDisplayDiffSummary(diffCount int, diffRefs []string) string {
	if len(diffRefs) > 0 {
		return runtimeToolRefsSummary(diffRefs, "diff")
	}
	return runtimeToolCountSummary(diffCount, "diff")
}

func runtimeToolDisplayEditOutputHasDiffSummary(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "{") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return false
	}
	for _, key := range []string{"diff", "additions", "removals", "edits_applied", "old_content", "new_content"} {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
}

func runtimeToolDisplayTitle(call scheduler.ToolCall, kind string) string {
	status := string(call.Status)
	switch kind {
	case "shell":
		if status == string(scheduler.ToolCallRunning) || status == string(scheduler.ToolCallPending) {
			return "正在运行 1 条命令"
		}
		if status == string(scheduler.ToolCallWaitingPermission) {
			return "等待运行 1 条命令"
		}
		return "已运行 1 条命令"
	case "file_read":
		if status == string(scheduler.ToolCallCompleted) {
			return "已读取文件"
		}
		return "读取文件"
	case "file_write":
		if status == string(scheduler.ToolCallCompleted) {
			return "已写入文件"
		}
		return "写入文件"
	case "file_edit":
		if status == string(scheduler.ToolCallCompleted) {
			return "已编辑文件"
		}
		return "编辑文件"
	case "file_search":
		if status == string(scheduler.ToolCallCompleted) {
			return "已搜索文件"
		}
		return "搜索文件"
	default:
		name := strings.TrimSpace(call.Name)
		if name == "" {
			name = "tool"
		}
		if status == string(scheduler.ToolCallRunning) || status == string(scheduler.ToolCallPending) {
			return "正在运行 " + name
		}
		if status == string(scheduler.ToolCallCompleted) {
			return "已运行 " + name
		}
		return name
	}
}

func classifyToolSource(name string) scheduler.ToolSource {
	toolName := strings.ToLower(strings.TrimSpace(name))
	switch {
	case toolName == "bash", toolName == "job_output", toolName == "job_kill":
		return scheduler.ToolSourceShell
	case strings.HasPrefix(toolName, "mcp_"):
		return scheduler.ToolSourceMCP
	case toolName == "":
		return scheduler.ToolSourceUnknown
	default:
		return scheduler.ToolSourceBuiltin
	}
}

func capabilityIDForToolName(name string) string {
	toolName := strings.TrimSpace(name)
	lower := strings.ToLower(toolName)
	if strings.HasPrefix(lower, "mcp_") {
		rest := toolName[len("mcp_"):]
		var bestServer string
		for server := range mcptools.Tools() {
			prefix := server + "_"
			if strings.HasPrefix(rest, prefix) && len(server) > len(bestServer) {
				bestServer = server
			}
		}
		if bestServer != "" {
			return "mcp:" + bestServer + ":" + strings.TrimPrefix(rest, bestServer+"_")
		}
		return ""
	}
	if toolName == "" {
		return ""
	}
	if lower == "bash" || lower == "job_output" || lower == "job_kill" {
		return "shell:" + lower
	}
	return "builtin:" + toolName
}
