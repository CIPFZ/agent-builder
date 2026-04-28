package tools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestAgentTaskToolSpawnAndStatus(t *testing.T) {
	manager := agent.NewManager()
	tool := tools.NewAgentTaskTool(manager, func(_ context.Context, _ session.Session, _ agent.RunContext, prompt string) (string, error) {
		return "handled: " + prompt, nil
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	got, err := tool.Invoke(context.Background(), sess, "research: investigate failing test")
	if err != nil {
		t.Fatalf("spawn agent task: %v", err)
	}
	if !containsAll(got, []string{"action=spawned", "status=completed", `label="research"`, "child_session=", "output=handled: investigate failing test"}) {
		t.Fatalf("spawn output = %q, want completed structured spawn summary", got)
	}

	runs := manager.List()
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if _, err := manager.Wait(context.Background(), runs[0].ID, 2*time.Second); err != nil {
		t.Fatalf("wait run: %v", err)
	}

	status, err := tool.Invoke(context.Background(), sess, "status "+runs[0].ID)
	if err != nil {
		t.Fatalf("status agent task: %v", err)
	}
	if !containsAll(status, []string{"action=status", "status=completed", "output=handled: investigate failing test"}) {
		t.Fatalf("status output = %q, want structured completed run output", status)
	}
}

func TestAgentTaskToolListFiltersByParentSession(t *testing.T) {
	manager := agent.NewManager()
	tool := tools.NewAgentTaskTool(manager, func(_ context.Context, _ session.Session, _ agent.RunContext, prompt string) (string, error) {
		return prompt, nil
	})
	sessions := session.NewManager(nil)
	mainSess := sessions.GetOrCreateMain("main")
	otherSess := sessions.CreateChild("other", "agent:other:main")

	if _, err := tool.Invoke(context.Background(), mainSess, "one: main prompt"); err != nil {
		t.Fatalf("spawn main task: %v", err)
	}
	if _, err := tool.Invoke(context.Background(), otherSess, "two: other prompt"); err != nil {
		t.Fatalf("spawn other task: %v", err)
	}

	list, err := tool.Invoke(context.Background(), mainSess, "list")
	if err != nil {
		t.Fatalf("list agent tasks: %v", err)
	}
	if !strings.Contains(list, "one") {
		t.Fatalf("list output = %q, want current session run", list)
	}
	if strings.Contains(list, "two") {
		t.Fatalf("list output = %q, should not include other session run", list)
	}
}

func TestAgentTaskToolMetadataMatchesAdvancedToolShape(t *testing.T) {
	tool := tools.NewAgentTaskTool(agent.NewManager(), nil)
	if !tool.ShouldDefer() {
		t.Fatal("expected agent tool to be deferrable")
	}
	if !tool.AlwaysLoad() {
		t.Fatal("expected agent tool to be always-loaded")
	}
	if !strings.Contains(tool.PromptDescription(), "subagent") {
		t.Fatalf("prompt description = %q, want subagent guidance", tool.PromptDescription())
	}
	if !strings.Contains(tool.SearchHint(), "delegate") {
		t.Fatalf("search hint = %q, want delegation language", tool.SearchHint())
	}
}

func TestAgentTaskToolAutoClassifierInputMatchesClaudeAgentProjection(t *testing.T) {
	tool := tools.NewAgentTaskTool(agent.NewManager(), nil)

	classifier, ok := any(tool).(tools.AutoClassifyingTool)
	if !ok {
		t.Fatal("AgentTaskTool must expose a Claude-style auto classifier input projection")
	}

	got := classifier.ToAutoClassifierInput(`{"subagent_type":"explorer","mode":"plan","prompt":"inspect permissions"}`)
	want := "(explorer, mode=plan): inspect permissions"
	if got != want {
		t.Fatalf("structured classifier input = %#v, want %q", got, want)
	}

	got = classifier.ToAutoClassifierInput("research: inspect permissions")
	want = "(research): inspect permissions"
	if got != want {
		t.Fatalf("legacy classifier input = %#v, want %q", got, want)
	}
}

func TestAgentTaskToolSteerAppendsControlMessage(t *testing.T) {
	manager := agent.NewManager()
	block := make(chan struct{})
	tool := tools.NewAgentTaskTool(manager, func(_ context.Context, _ session.Session, runCtx agent.RunContext, _ string) (string, error) {
		<-block
		messages := manager.ControlMessages(runCtx.RunID)
		if len(messages) == 0 {
			return "missing controls", nil
		}
		return messages[len(messages)-1], nil
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	if _, err := tool.Invoke(context.Background(), sess, "research: investigate failing test"); err != nil {
		t.Fatalf("spawn agent task: %v", err)
	}
	runs := manager.List()
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}

	got, err := tool.Invoke(context.Background(), sess, "steer "+runs[0].ID+" use safer plan")
	if err != nil {
		t.Fatalf("steer agent task: %v", err)
	}
	if !containsAll(got, []string{"action=steered", "id=" + runs[0].ID}) {
		t.Fatalf("steer output = %q, want steer confirmation", got)
	}

	close(block)
	result, err := manager.Wait(context.Background(), runs[0].ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait steered run: %v", err)
	}
	if result.Output != "use safer plan" {
		t.Fatalf("run output = %q, want latest steer message", result.Output)
	}
}

func TestAgentTaskToolStopCancelsRunningRun(t *testing.T) {
	manager := agent.NewManager()
	block := make(chan struct{})
	tool := tools.NewAgentTaskTool(manager, func(ctx context.Context, _ session.Session, _ agent.RunContext, _ string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-block:
			return "done", nil
		}
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	if _, err := tool.Invoke(context.Background(), sess, "research: investigate failing test"); err != nil {
		t.Fatalf("spawn agent task: %v", err)
	}
	runs := manager.List()
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}

	got, err := tool.Invoke(context.Background(), sess, "stop "+runs[0].ID)
	if err != nil {
		t.Fatalf("stop agent task: %v", err)
	}
	if !containsAll(got, []string{"action=stopped", "id=" + runs[0].ID}) {
		t.Fatalf("stop output = %q, want stop confirmation", got)
	}

	result, err := manager.Wait(context.Background(), runs[0].ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait stopped run: %v", err)
	}
	if result.Status != agent.StatusStopped {
		t.Fatalf("run status = %q, want stopped", result.Status)
	}
	close(block)
}

func TestAgentTaskToolWaitReturnsCompletedRunSummary(t *testing.T) {
	manager := agent.NewManager()
	tool := tools.NewAgentTaskTool(manager, func(_ context.Context, _ session.Session, _ agent.RunContext, prompt string) (string, error) {
		return "handled: " + prompt, nil
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	if _, err := tool.Invoke(context.Background(), sess, "research: investigate failing test"); err != nil {
		t.Fatalf("spawn agent task: %v", err)
	}
	runs := manager.List()
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}

	got, err := tool.Invoke(context.Background(), sess, "wait "+runs[0].ID)
	if err != nil {
		t.Fatalf("wait agent task: %v", err)
	}
	if !containsAll(got, []string{"action=wait", "status=completed", "output=handled: investigate failing test"}) {
		t.Fatalf("wait output = %q, want structured completed run summary", got)
	}
}

func TestAgentTaskToolResumeReusesChildSession(t *testing.T) {
	manager := agent.NewManager()
	tool := tools.NewAgentTaskTool(manager, func(_ context.Context, _ session.Session, runCtx agent.RunContext, prompt string) (string, error) {
		return runCtx.ChildSessionID + "|" + prompt, nil
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	if _, err := tool.Invoke(context.Background(), sess, "research: first pass"); err != nil {
		t.Fatalf("spawn agent task: %v", err)
	}
	firstRuns := manager.List()
	if len(firstRuns) != 1 {
		t.Fatalf("run count = %d, want 1", len(firstRuns))
	}
	first, err := manager.Wait(context.Background(), firstRuns[0].ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait first run: %v", err)
	}

	got, err := tool.Invoke(context.Background(), sess, "resume "+first.ID+" second pass")
	if err != nil {
		t.Fatalf("resume agent task: %v", err)
	}
	if !containsAll(got, []string{"action=resumed", "from=" + first.ID, "child_session=" + first.ChildSessionID}) {
		t.Fatalf("resume output = %q, want structured resume confirmation", got)
	}

	allRuns := manager.List()
	if len(allRuns) != 2 {
		t.Fatalf("run count = %d, want 2", len(allRuns))
	}
	var resumed agent.Run
	for _, run := range allRuns {
		if run.ID != first.ID {
			resumed = run
		}
	}
	result, err := manager.Wait(context.Background(), resumed.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("wait resumed run: %v", err)
	}
	if result.ChildSessionID != first.ChildSessionID {
		t.Fatalf("child session = %q, want %q", result.ChildSessionID, first.ChildSessionID)
	}
	if !strings.Contains(result.Output, "second pass") {
		t.Fatalf("resumed output = %q, want resumed prompt", result.Output)
	}
}

func TestAgentTaskToolInvokeWithContextUsesAgentTaskExecutor(t *testing.T) {
	var got tools.AgentTaskRequest
	tool := tools.NewAgentTaskTool(agent.NewManager(), nil)
	sess := session.NewManager(nil).GetOrCreateMain("main")

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  sess,
		ToolName: "agent.task",
		Input:    `{"description":"research","prompt":"inspect auth flow","subagent_type":"researcher","isolation":"worktree"}`,
		AppState: map[string]any{
			"agentTaskExecutor": tools.AgentTaskExecutor(func(_ context.Context, request tools.AgentTaskRequest) (tools.ToolResult, error) {
				got = request
				return tools.ToolResult{Output: `{"status":"spawned","runId":"agent-1"}`}, nil
			}),
		},
	})
	if err != nil {
		t.Fatalf("invoke with context: %v", err)
	}
	if got.Label != "research" || got.Prompt != "inspect auth flow" || got.AgentType != "researcher" || got.Isolation != "worktree" {
		t.Fatalf("request = %#v, want parsed structured agent task", got)
	}
	if result.Output != `{"status":"spawned","runId":"agent-1"}` {
		t.Fatalf("result = %#v, want executor output", result)
	}
}

func TestAgentTaskToolStructuredPromptProjectsIsolationControls(t *testing.T) {
	projected, ok := tools.ProjectStructuredAgentTaskInputForTest(`{
		"description":"research",
		"prompt":"inspect auth flow",
		"subagent_type":"researcher",
		"run_in_background":true,
		"isolation":"worktree",
		"cwd":"C:/repo/services/api",
		"remote_boundary":"remote:disabled",
		"allowed_tools":["Read","Grep"],
		"permission_mode":"ask",
		"output_file":"C:/tmp/research.log"
	}`)
	if !ok {
		t.Fatal("expected structured input to parse")
	}
	if !projected.RunInBackground || projected.Isolation != "worktree" || projected.CWD != "C:/repo/services/api" {
		t.Fatalf("projection = %#v, want parsed isolation controls", projected)
	}
	if projected.RemoteIsolationBoundary != "remote:disabled" || projected.PermissionMode != "ask" || projected.OutputFile != "C:/tmp/research.log" {
		t.Fatalf("projection = %#v, want parsed boundary, permission, output controls", projected)
	}
	if strings.Join(projected.AllowedTools, ",") != "Read,Grep" {
		t.Fatalf("allowed tools = %#v, want Read,Grep", projected.AllowedTools)
	}
}

func TestAgentTaskToolInvokeWithContextFallsBackWhenExecutorMissing(t *testing.T) {
	manager := agent.NewManager()
	tool := tools.NewAgentTaskTool(manager, func(_ context.Context, _ session.Session, _ agent.RunContext, prompt string) (string, error) {
		return "handled: " + prompt, nil
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  sess,
		ToolName: "agent.task",
		Input:    "research: investigate failing test",
	})
	if err != nil {
		t.Fatalf("invoke with context fallback: %v", err)
	}
	if !containsAll(result.Output, []string{"action=spawned", "status=completed", "output=handled: investigate failing test"}) {
		t.Fatalf("result = %#v, want fallback spawn summary", result)
	}
}

func TestAgentTaskToolStructuredPromptProjectsBackgroundFlag(t *testing.T) {
	projected, ok := tools.ProjectStructuredAgentTaskInputForTest(`{"description":"research","prompt":"inspect auth flow","subagent_type":"researcher","run_in_background":true}`)
	if !ok {
		t.Fatal("expected structured input to parse")
	}
	if !projected.RunInBackground {
		t.Fatalf("projection = %#v, want background flag", projected)
	}
	if projected.AgentType != "researcher" || projected.Label != "research" || projected.Prompt != "inspect auth flow" {
		t.Fatalf("projection = %#v, want parsed fields", projected)
	}
}

func TestAgentTaskToolStructuredPromptProjectsIsolationFlag(t *testing.T) {
	projected, ok := tools.ProjectStructuredAgentTaskInputForTest(`{"description":"research","prompt":"inspect auth flow","subagent_type":"researcher","isolation":"worktree"}`)
	if !ok {
		t.Fatal("expected structured input to parse")
	}
	if projected.Isolation != "worktree" {
		t.Fatalf("projection = %#v, want worktree isolation", projected)
	}
}

func TestAgentTaskToolResumeRejectsRunningRun(t *testing.T) {
	manager := agent.NewManager()
	blocked := make(chan struct{})
	tool := tools.NewAgentTaskTool(manager, func(ctx context.Context, _ session.Session, _ agent.RunContext, prompt string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-blocked:
			return "handled: " + prompt, nil
		}
	})
	sess := session.NewManager(nil).GetOrCreateMain("main")

	if _, err := tool.Invoke(context.Background(), sess, "research: first pass"); err != nil {
		t.Fatalf("spawn agent task: %v", err)
	}
	runs := manager.List()
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	defer close(blocked)

	_, err := tool.Invoke(context.Background(), sess, "resume "+runs[0].ID+" second pass")
	if err == nil {
		t.Fatal("expected resume to reject running run")
	}
	if !strings.Contains(err.Error(), "still running") || !strings.Contains(err.Error(), runs[0].ID) {
		t.Fatalf("error = %v, want running-run rejection mentioning %q", err, runs[0].ID)
	}
}
