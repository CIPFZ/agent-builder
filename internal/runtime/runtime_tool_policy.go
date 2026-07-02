package runtime

import (
	"strings"

	"github.com/CIPFZ/agent-builder/internal/tools/scheduler"
)

// runtimeToolPolicyContext carries the projection-side signals needed to
// finalize a tool call's status/display policy. Callers building calls from
// the scheduler alone can pass an empty ctx; the projection layer fills in
// permission and turn state when it has them available.
type runtimeToolPolicyContext struct {
	PermissionStatus string // pending | denied | cancelled | expired | ""
	TurnTerminal    bool
	TurnError       string
}

// runtimeToolKind is the single kind derivation used everywhere. Callers pass
// enough of the scheduler tool call to identify it; MCP/plugin/custom sources
// resolve to "generic" unless the name mentions a builtin verb.
func runtimeToolKind(name string, source scheduler.ToolSource, hasCommand bool, risk string) string {
	toolName := strings.ToLower(strings.TrimSpace(name))

	// 1) Static builtin registry — same mapping as capabilityIDForToolName.
	switch toolName {
	case "bash", "job_output", "job_kill":
		return "shell"
	case "view", "read":
		return "file_read"
	case "write":
		return "file_write"
	case "edit", "multiedit", "apply_patch":
		return "file_edit"
	case "glob", "grep", "ls":
		return "file_search"
	case "todos", "todo", "todowrite":
		return "todo"
	case "agent", "task_create", "agentic_fetch":
		return "agent_task"
	}

	// 2) Source-derived: MCP/plugin/custom default to generic.
	switch source {
	case scheduler.ToolSourceShell:
		return "shell"
	case scheduler.ToolSourceMCP, "plugin", "custom":
		return "generic"
	}

	// 3) Shell fallback: explicit exec risk or a command field implies shell.
	if hasCommand || risk == "execute" {
		return "shell"
	}
	switch toolName {
	case "shell", "cmd", "powershell", "pwsh", "go", "npm", "node", "python":
		return "shell"
	}

	// 4) Loose string matching only as last resort. Todo/agent_task are NOT
	// substring-matched: plugin/custom tools whose name happens to contain
	// "todo" or "agent" should not silently take on those specialized kinds.
	switch {
	case strings.Contains(toolName, "search"), strings.Contains(toolName, "find"):
		return "file_search"
	case strings.Contains(toolName, "edit"), strings.Contains(toolName, "patch"):
		return "file_edit"
	case strings.Contains(toolName, "read"), strings.Contains(toolName, "view"), strings.Contains(toolName, "open"):
		return "file_read"
	case strings.Contains(toolName, "write"), strings.Contains(toolName, "create"):
		return "file_write"
	}
	return "generic"
}

// runtimeToolPolicyDisplayKind resolves the kind for a scheduler.ToolCall
// consistently with runtimeToolKind above.
func runtimeToolPolicyDisplayKind(call scheduler.ToolCall) string {
	return runtimeToolKind(call.Name, call.Source, call.Command != "", call.Risk)
}

// runtimeToolPolicyKindForRuntime resolves the kind for a projection-level
// RuntimeToolCall (used from applyConversationToolPolicy).
func runtimeToolPolicyKindForRuntime(call RuntimeToolCall) string {
	source := scheduler.ToolSource(call.Source)
	return runtimeToolKind(call.Name, source, call.Command != "", call.Risk)
}

// runtimeNormalizeToolStatus derives the terminal status from the scheduler
// status plus any error signals.
func runtimeNormalizeToolStatus(status, kind string, exitCode int, isError bool, errText string) string {
	if status == "" {
		status = string(scheduler.ToolCallPending)
	}
	if status == string(scheduler.ToolCallCompleted) && (isError || errText != "" || (kind == "shell" && exitCode != 0)) {
		return string(scheduler.ToolCallFailed)
	}
	return status
}

// runtimeToolTerminalStatus reports whether a status is a final state.
func runtimeToolTerminalStatus(status string) bool {
	switch status {
	case string(scheduler.ToolCallCompleted),
		string(scheduler.ToolCallFailed),
		string(scheduler.ToolCallDenied),
		string(scheduler.ToolCallCancelled),
		"interrupted":
		return true
	default:
		return false
	}
}

// runtimeToolSuccess reports whether a tool call ended successfully.
func runtimeToolSuccess(status string) bool {
	return status == string(scheduler.ToolCallCompleted)
}

// runtimeToolQuiet decides whether a completed tool renders as a single
// lightweight row (no Collapse chrome). Only successful read/search tools are
// quiet; grouping is a separate axis (see runtimeToolGroupable).
func runtimeToolQuiet(kind, status string) bool {
	if !runtimeToolSuccess(status) {
		return false
	}
	return kind == "file_read" || kind == "file_search"
}

// runtimeToolGroupable decides whether a tool can join a run-of-tools group.
// Rule: any successful (completed) call, except agent_task. Failed/denied/
// cancelled/interrupted never group.
func runtimeToolGroupable(kind, status string) bool {
	if !runtimeToolSuccess(status) {
		return false
	}
	return kind != "agent_task"
}

// runtimeToolDefaultExpanded gates whether the card is expanded by default.
// Non-terminal or non-successful terminal states expand automatically so
// failures/waits are surfaced.
func runtimeToolDefaultExpanded(status string) bool {
	switch status {
	case string(scheduler.ToolCallRunning),
		string(scheduler.ToolCallWaitingPermission),
		string(scheduler.ToolCallFailed),
		string(scheduler.ToolCallDenied),
		string(scheduler.ToolCallCancelled),
		"interrupted":
		return true
	default:
		return false
	}
}

// runtimeToolGroupKey identifies a tool's "grouping cohort". Group keys drop
// messageID so tools across adjacent assistant messages can group; mixed
// kinds are allowed too so we omit kind. The adjacency and breaker rules are
// enforced by appendConversationToolItems.
func runtimeToolGroupKey(turnID string) string {
	if turnID == "" {
		return ""
	}
	return "tools:" + turnID
}

// applyRuntimeToolPolicy normalizes status and derives quiet/groupable/
// defaultExpanded/groupKey on a RuntimeToolCall. It is a pure function; the
// projection layer builds ctx from turn/permission state.
func applyRuntimeToolPolicy(call RuntimeToolCall, ctx runtimeToolPolicyContext) RuntimeToolCall {
	kind := firstNonEmpty(call.Kind, call.Display.Kind, runtimeToolPolicyKindForRuntime(call))
	call.Kind = kind
	call.Display.Kind = kind

	status := call.Status
	if status == "" {
		status = string(scheduler.ToolCallPending)
	}
	if status == string(scheduler.ToolCallCompleted) && (call.IsError || call.Error != "" || (kind == "shell" && call.ExitCode != 0)) {
		status = string(scheduler.ToolCallFailed)
	}
	switch ctx.PermissionStatus {
	case "pending":
		status = string(scheduler.ToolCallWaitingPermission)
	case "denied", "cancelled", "expired":
		status = string(scheduler.ToolCallDenied)
	}
	if ctx.TurnTerminal && !runtimeToolTerminalStatus(status) {
		status = "interrupted"
		call.Error = firstNonEmpty(call.Error, ctx.TurnError, "tool call did not finish before the turn ended")
	}
	call.Status = status

	call.Quiet = runtimeToolQuiet(kind, status)
	call.Groupable = runtimeToolGroupable(kind, status)
	call.DefaultExpanded = runtimeToolDefaultExpanded(status)
	call.GroupKey = runtimeToolGroupKey(call.TurnID)
	return call
}
