package runtimeapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEndpointsFreezePhase2MinimalAPI(t *testing.T) {
	t.Parallel()

	expected := []Endpoint{
		{Method: MethodGet, Path: "/v1/runtime/status"},
		{Method: MethodGet, Path: "/v1/recovery/status"},
		{Method: MethodPost, Path: "/v1/recovery/turns/{turn_id}/resume"},
		{Method: MethodPost, Path: "/v1/recovery/turns/{turn_id}/discard"},
		{Method: MethodPost, Path: "/v1/recovery/errors/{error_id}/retry"},
		{Method: MethodGet, Path: "/v1/config/model"},
		{Method: MethodPut, Path: "/v1/config/model"},
		{Method: MethodPost, Path: "/v1/config/model/verify"},
		{Method: MethodGet, Path: "/v1/config/models"},
		{Method: MethodGet, Path: "/v1/projects/{project_id}/memory"},
		{Method: MethodPost, Path: "/v1/projects/{project_id}/memory"},
		{Method: MethodPost, Path: "/v1/projects/{project_id}/memory/refresh"},
		{Method: MethodGet, Path: "/v1/projects/{project_id}/memory/diagnostics"},
		{Method: MethodGet, Path: "/v1/memory/{memory_id}"},
		{Method: MethodPut, Path: "/v1/memory/{memory_id}"},
		{Method: MethodPatch, Path: "/v1/memory/{memory_id}"},
		{Method: MethodPost, Path: "/v1/memory/{memory_id}/disable"},
		{Method: MethodDelete, Path: "/v1/memory/{memory_id}"},
		{Method: MethodGet, Path: "/v1/sessions"},
		{Method: MethodPost, Path: "/v1/sessions"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/messages"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/context-usage"},
		{Method: MethodPost, Path: "/v1/sessions/{session_id}/compact"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/activity-window"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/run-projection"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/todos"},
		{Method: MethodPost, Path: "/v1/sessions/{session_id}/turns"},
		{Method: MethodGet, Path: "/v1/turns"},
		{Method: MethodGet, Path: "/v1/runs"},
		{Method: MethodGet, Path: "/v1/runs/{run_id}"},
		{Method: MethodPost, Path: "/v1/runs/{run_id}/checkpoints/{checkpoint_id}/acknowledge"},
		{Method: MethodPost, Path: "/v1/runs/{run_id}/checkpoints/{checkpoint_id}/discard"},
		{Method: MethodPost, Path: "/v1/runs/{run_id}/checkpoints/{checkpoint_id}/resume"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/activity"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/todos"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/tool-calls"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/compact"},
		{Method: MethodGet, Path: "/v1/hooks"},
		{Method: MethodGet, Path: "/v1/hook-executions"},
		{Method: MethodGet, Path: "/v1/hook-executions/{execution_id}"},
		{Method: MethodGet, Path: "/v1/sandbox/decisions"},
		{Method: MethodGet, Path: "/v1/sandbox/decisions/{decision_id}"},
		{Method: MethodGet, Path: "/v1/turns/{turn_id}/agent-tasks"},
		{Method: MethodGet, Path: "/v1/tool-calls/{tool_call_id}"},
		{Method: MethodGet, Path: "/v1/refs"},
		{Method: MethodGet, Path: "/v1/refs/{ref_id}"},
		{Method: MethodGet, Path: "/v1/refs/{ref_id}/content"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/agent-tasks"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/messages"},
		{Method: MethodPost, Path: "/v1/agent-tasks/{task_id}/messages"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/result"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/output"},
		{Method: MethodPost, Path: "/v1/agent-tasks/{task_id}/cancel"},
		{Method: MethodPost, Path: "/v1/agent-tasks/{task_id}/follow-up"},
		{Method: MethodGet, Path: "/v1/agent-tasks/{task_id}/effective-scope"},
		{Method: MethodPost, Path: "/v1/turns/{turn_id}/cancel"},
		{Method: MethodPost, Path: "/v1/turns/{turn_id}/interrupted/done"},
		{Method: MethodGet, Path: "/v1/worktrees"},
		{Method: MethodPost, Path: "/v1/worktrees"},
		{Method: MethodGet, Path: "/v1/worktrees/{worktree_id}"},
		{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/enter"},
		{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/exit"},
		{Method: MethodPost, Path: "/v1/worktrees/{worktree_id}/cleanup"},
		{Method: MethodGet, Path: "/v1/permissions"},
		{Method: MethodPost, Path: "/v1/permissions/{permission_id}/decision"},
		{Method: MethodGet, Path: "/v1/policy"},
		{Method: MethodPut, Path: "/v1/policy"},
		{Method: MethodGet, Path: "/v1/settings/context-governance"},
		{Method: MethodPut, Path: "/v1/settings/context-governance"},
		{Method: MethodGet, Path: "/v1/capabilities"},
		{Method: MethodPost, Path: "/v1/capabilities/{capability_id}/refresh"},
		{Method: MethodPost, Path: "/v1/tools/search"},
		{Method: MethodGet, Path: "/v1/context/sources"},
		{Method: MethodGet, Path: "/v1/read-files"},
		{Method: MethodGet, Path: "/v1/skills"},
		{Method: MethodPost, Path: "/v1/skills"},
		{Method: MethodPost, Path: "/v1/skills/refresh"},
		{Method: MethodPost, Path: "/v1/skills/paths"},
		{Method: MethodGet, Path: "/v1/mcp/servers"},
		{Method: MethodPut, Path: "/v1/mcp/servers/{server_name}"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/enabled"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/refresh"},
		{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/tools"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/tools/{tool_name}/enabled"},
		{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/resources"},
		{Method: MethodGet, Path: "/v1/mcp/servers/{server_name}/prompts"},
		{Method: MethodGet, Path: "/v1/mcp/requests"},
		{Method: MethodGet, Path: "/v1/mcp/requests/{request_id}"},
		{Method: MethodPost, Path: "/v1/mcp/requests/{request_id}/decision"},
		{Method: MethodPost, Path: "/v1/mcp/servers/{server_name}/retry"},
		{Method: MethodGet, Path: "/v1/audit/turns/{turn_id}"},
		{Method: MethodGet, Path: "/v1/audit/sessions/{session_id}"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/compact"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/output"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/output/events"},
		{Method: MethodGet, Path: "/v1/sessions/{session_id}/output/stream"},
		{Method: MethodGet, Path: "/v1/events"},
	}

	if len(Endpoints) != len(expected) {
		t.Fatalf("Endpoints len = %d, want %d", len(Endpoints), len(expected))
	}
	for _, endpoint := range expected {
		if !slices.Contains(Endpoints, endpoint) {
			t.Fatalf("Endpoints missing %#v", endpoint)
		}
	}
}

func TestEventTypesFreezePhase2Schema(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		EventRecoveryStatusChanged,
		EventRecoveryTurnResumed,
		EventRecoveryTurnDiscarded,
		EventRecoveryErrorClassified,
		EventRecoveryRetryStarted,
		EventRecoveryRetryCompleted,
		EventRecoveryRetryFailed,
		EventRecoveryCompactRetryStarted,
		EventRecoveryCompactRetryCompleted,
		EventRecoveryCompactRetryFailed,
		EventSessionCreated,
		EventSessionSelectionCleared,
		EventTurnStarted,
		EventMessageCreated,
		EventToolCallStarted,
		EventOutputRefCreated,
		EventArtifactRefCreated,
		EventToolOutputRefCreated,
		EventToolSearchPerformed,
		EventToolDiscoverySelected,
		EventToolDiscoveryOmitted,
		EventSchedulerDeadlockPrevented,
		EventTaskProgress,
		EventWorktreeCreated,
		EventWorktreeEntered,
		EventWorktreeExited,
		EventWorktreeCleaned,
		EventWorktreeCleanupFailed,
		EventWorktreePolicyDenied,
		EventPermissionRequested,
		EventPermissionPolicyApplied,
		EventPolicyRuleMatched,
		EventPolicyRuleDenied,
		EventPolicyRuleAsk,
		EventShellPolicyClassified,
		EventHookDiscovered,
		EventHookConfigured,
		EventHookExecutionStarted,
		EventHookExecutionCompleted,
		EventHookExecutionSkipped,
		EventHookExecutionBlocked,
		EventHookExecutionFailed,
		EventHookContextInjected,
		EventHookInputRewritten,
		EventTodoUpdated,
		EventCapabilityLoading,
		EventCapabilityLoaded,
		EventCapabilityFailed,
		EventContextUsageUpdated,
		EventPromptAssemblyRecorded,
		EventCompactStarted,
		EventCompactCompleted,
		EventCompactFailed,
		EventSkillDiscoveryCompleted,
		EventMCPServerConnected,
		EventMCPServerBlocked,
		EventMCPAuthRequested,
		EventMCPAuthCompleted,
		EventMCPAuthDenied,
		EventMCPAuthFailed,
		EventMCPElicitationRequested,
		EventMCPElicitationCompleted,
		EventMCPElicitationDenied,
		EventMCPElicitationFailed,
		EventUsageUpdated,
		EventAuditRecorded,
		EventMemoryIndexStarted,
		EventMemoryIndexCompleted,
		EventMemoryIndexFailed,
		EventMemoryRecordCreated,
		EventMemoryRecordUpdated,
		EventMemoryRecordDisabled,
		EventMemoryRecordDeleted,
		EventMemoryRecordInjected,
		EventMemoryRecordSkipped,
	} {
		if !IsEventType(eventType) {
			t.Fatalf("IsEventType(%q) = false", eventType)
		}
	}
	if IsEventType("message") {
		t.Fatal("legacy event type should not be part of the Phase 2 schema")
	}
}

func TestEventValidateRequiresStableEnvelope(t *testing.T) {
	t.Parallel()

	event := NewEvent("event-1", EventMessageCreated, time.Date(2026, 5, 18, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	event.SessionID = "session-1"
	event.MessageID = "message-1"
	event.Payload = map[string]any{"role": "assistant"}

	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}

	event.Type = "message"
	if err := event.Validate(); err == nil {
		t.Fatal("Validate accepted legacy event type")
	}
}

func TestOutputTextDeltaIsEphemeral(t *testing.T) {
	t.Parallel()

	if !IsEventType(EventOutputTextDelta) {
		t.Fatal("EventOutputTextDelta must be registered as an event type")
	}
	if !IsEphemeralEventType(EventOutputTextDelta) {
		t.Fatal("output.text.delta must be classified as ephemeral")
	}
	// Sanity: none of the load-bearing persisted types leak into ephemeral.
	for _, persisted := range []string{EventMessageCreated, EventMessageUpdated, EventMessageCompleted, EventToolCallStarted, EventToolCallCompleted, EventTurnStarted, EventTurnCompleted} {
		if IsEphemeralEventType(persisted) {
			t.Fatalf("%q must not be classified ephemeral", persisted)
		}
	}
}

// TestNoOrphanEventTypes guards against event type constants that are
// declared in the const block and registered in EventTypes but never
// referenced by any producer or consumer anywhere else in the Go source
// tree (internal/ and desktop/, excluding _test.go files and this package's
// own contract.go, where the constant is necessarily declared/registered).
// Such an "orphan" event is either dead code left behind after a refactor,
// or a sign a feature's event wiring was never finished — either way it
// should be caught here rather than discovered by grep during a future
// cleanup pass.
//
// This walks the source tree at test time (rather than using go:embed,
// which cannot reach outside this package's directory) because the set of
// files to scan spans the whole repo and changes as the codebase evolves.
func TestNoOrphanEventTypes(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine contract_test.go path via runtime.Caller")
	}
	pkgDir := filepath.Dir(thisFile)
	contractPath := filepath.Join(pkgDir, "contract.go")
	repoRoot := filepath.Join(pkgDir, "..", "..")

	identifierByValue, err := eventIdentifiersByValue(contractPath)
	if err != nil {
		t.Fatalf("failed to parse event identifiers from %s: %v", contractPath, err)
	}

	sources, err := goSourceFiles(repoRoot, []string{"internal", "desktop"}, contractPath)
	if err != nil {
		t.Fatalf("failed to collect source tree: %v", err)
	}

	var orphans []string
	for _, value := range EventTypes {
		name, ok := identifierByValue[value]
		if !ok {
			t.Fatalf("EventTypes value %q has no matching const identifier in contract.go", value)
		}
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		referenced := false
		for _, content := range sources {
			if pattern.MatchString(content) {
				referenced = true
				break
			}
		}
		if !referenced {
			orphans = append(orphans, name)
		}
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Fatalf("orphan event types (declared/registered but never referenced outside contract.go and *_test.go): %v", orphans)
	}
}

// eventIdentifiersByValue parses contract.go and returns a map from each
// string constant's value (e.g. "compact.started") to its Go identifier
// (e.g. "EventCompactStarted"), covering every `Name = "literal"` const spec
// in the file regardless of which const block it lives in.
func eventIdentifiersByValue(path string) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				out[value] = name.Name
			}
		}
	}
	return out, nil
}

// goSourceFiles reads every non-test .go file under the given subdirectories
// of root (skipping excludePath, which is contract.go itself — the file
// where every event identifier is necessarily declared).
func goSourceFiles(root string, subdirs []string, excludePath string) ([]string, error) {
	excludeAbs, err := filepath.Abs(excludePath)
	if err != nil {
		return nil, err
	}
	var contents []string
	for _, subdir := range subdirs {
		dir := filepath.Join(root, subdir)
		walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Skip vendored/build/dependency directories that are not
				// part of this repo's own source.
				switch d.Name() {
				case "node_modules", "dist", "build", "bin", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if abs == excludeAbs {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			contents = append(contents, string(data))
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return contents, nil
}
