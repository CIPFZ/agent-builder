package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/tools"
)

func TestWebFetchToolFetchesHTTPContentWithDefaultFetcherAndUsesClaudeSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><h1>Release Notes</h1><p>Ship it.</p></body></html>"))
	}))
	defer server.Close()

	tool := tools.NewWebFetchTool(nil)
	def := tool.Definition()
	properties := def.InputSchema["properties"].(map[string]any)
	if properties["url"] == nil || properties["prompt"] == nil {
		t.Fatalf("schema = %#v, want url and prompt properties", def.InputSchema)
	}

	got, err := tool.Invoke(context.Background(), session.Session{}, `{"url":"`+server.URL+`","prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("web fetch: %v", err)
	}
	if !strings.Contains(got, "Release Notes") || !strings.Contains(got, "Ship it.") {
		t.Fatalf("output = %q, want fetched page text", got)
	}
}

func TestWebFetchReturnsRedirectRetryInstruction(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("target"))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer redirector.Close()

	got, err := tools.NewWebFetchTool(nil).Invoke(context.Background(), session.Session{}, `{"url":"`+redirector.URL+`","prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("web fetch redirect: %v", err)
	}
	if !strings.Contains(got, "redirected") || !strings.Contains(got, target.URL+"/final") {
		t.Fatalf("output = %q, want retry instruction with redirected URL", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("redirect output JSON: %v", err)
	}
	redirect, _ := parsed["redirect"].(map[string]any)
	if redirect["originalUrl"] != redirector.URL || redirect["redirectUrl"] != target.URL+"/final" {
		t.Fatalf("redirect = %#v, want original and redirected URLs", redirect)
	}
}

func TestWebFetchFollowsSameOriginRedirect(t *testing.T) {
	server := httptest.NewServer(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<h1>Final page</h1>"))
	})
	server.Config.Handler = mux
	defer server.Close()

	got, err := tools.NewWebFetchTool(nil).Invoke(context.Background(), session.Session{}, `{"url":"`+server.URL+`/start","prompt":"summarize"}`)
	if err != nil {
		t.Fatalf("web fetch redirect: %v", err)
	}
	if !strings.Contains(got, "Final page") || strings.Contains(got, "redirected") {
		t.Fatalf("output = %q, want final same-origin page", got)
	}
}

func TestWebFetchDomainPermissionRules(t *testing.T) {
	tool := tools.NewWebFetchTool(nil)
	checker, ok := any(tool).(tools.ContextualPermissionCheckingTool)
	if !ok {
		t.Fatal("WebFetch must expose domain-level contextual permissions")
	}

	decision, err := checker.CheckPermissionsWithContext(context.Background(), tools.ToolUseContext{
		ToolName: "WebFetch",
		Input:    `{"url":"https://blocked.example/private","prompt":"summarize"}`,
		Policy: permissions.Policy{Rules: []permissions.Rule{{
			ToolName: "WebFetch",
			Action:   permissions.ActionDeny,
			Match:    permissions.Match{CommandContains: []string{"domain:blocked.example"}},
		}}},
	})
	if err != nil {
		t.Fatalf("check permissions: %v", err)
	}
	if decision.Allowed || decision.Category != permissions.CategoryRuleDenied {
		t.Fatalf("decision = %#v, want domain deny", decision)
	}
}

func TestMCPResourceToolsListAndReadContextResources(t *testing.T) {
	listTool := tools.NewListMcpResourcesTool()
	readTool := tools.NewReadMcpResourceTool()
	ctx := tools.ToolUseContext{
		Session:  session.Session{},
		ToolName: "ListMcpResources",
		MCPResources: map[string][]tools.MCPResource{
			"filesystem": {{URI: "file:///tmp/a", Name: "a", Description: "resource a"}},
		},
	}

	listed, err := listTool.InvokeWithContext(context.Background(), ctx)
	if err != nil {
		t.Fatalf("list mcp resources: %v", err)
	}
	if !strings.Contains(listed.Output, "filesystem") || !strings.Contains(listed.Output, "file:///tmp/a") {
		t.Fatalf("output = %q, want listed MCP resource", listed.Output)
	}

	readCtx := ctx
	readCtx.ToolName = "ReadMcpResource"
	readCtx.Input = `{"server":"filesystem","uri":"file:///tmp/a"}`
	read, err := readTool.InvokeWithContext(context.Background(), readCtx)
	if err != nil {
		t.Fatalf("read mcp resource: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(read.Output), &parsed); err != nil {
		t.Fatalf("read output JSON: %v", err)
	}
	contents, _ := parsed["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("output = %q, want Claude-style contents array", read.Output)
	}
	content := contents[0].(map[string]any)
	if content["uri"] != "file:///tmp/a" || content["text"] != "resource a" {
		t.Fatalf("content = %#v, want resource URI and text", content)
	}
}

func TestDynamicMCPToolUsesClaudeNameSchemaAndCaller(t *testing.T) {
	var gotServer, gotName string
	var gotInput map[string]any
	tool := tools.NewMCPTool(tools.MCPToolDefinition{
		Server:      "filesystem",
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
	}, func(_ context.Context, server, name string, input map[string]any) (tools.MCPToolResult, error) {
		gotServer, gotName, gotInput = server, name, input
		return tools.MCPToolResult{Content: []map[string]any{{"type": "text", "text": "file contents"}}}, nil
	})

	def := tool.Definition()
	if def.Name != "mcp__filesystem__read_file" || def.Source != "mcp" {
		t.Fatalf("definition = %#v, want Claude MCP tool name/source", def)
	}
	out, err := tool.Invoke(context.Background(), session.Session{}, `{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("invoke mcp tool: %v", err)
	}
	if gotServer != "filesystem" || gotName != "read_file" || gotInput["path"] != "README.md" {
		t.Fatalf("call = server %q name %q input %#v", gotServer, gotName, gotInput)
	}
	if !strings.Contains(out, "file contents") {
		t.Fatalf("output = %q, want text content", out)
	}
}

func TestSkillToolLoadsSkillPathAndRecordsInvocation(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/SKILL.md"
	if err := os.WriteFile(path, []byte("---\nname: demo\n---\nUse demo skill."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	appState := map[string]any{}
	tool := tools.NewSkillTool()
	properties := tool.Definition().InputSchema["properties"].(map[string]any)
	if properties["skill"] == nil || properties["args"] == nil {
		t.Fatalf("schema = %#v, want Claude skill/args inputs", tool.Definition().InputSchema)
	}
	if properties["name"] != nil || properties["path"] != nil {
		t.Fatalf("schema = %#v, should not expose Go-only name/path fields", tool.Definition().InputSchema)
	}

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{},
		AgentID:  "agent-1",
		ToolName: "Skill",
		Input:    `{"skill":"demo","path":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	if !strings.Contains(result.Output, "Use demo skill.") {
		t.Fatalf("output = %q, want skill file contents", result.Output)
	}
	invoked, _ := appState["invokedSkills"].([]any)
	if len(invoked) != 1 {
		t.Fatalf("appState = %#v, want one invoked skill recorded", appState)
	}
	record := invoked[0].(map[string]any)
	if record["skillName"] != "demo" || record["skillPath"] != path || record["content"] == "" || record["agentId"] != "agent-1" || record["invokedAt"] == "" {
		t.Fatalf("invoked skill = %#v, want Claude-style state metadata", record)
	}
}

func TestSkillToolLoadsCatalogFrontmatterAndReturnsInlineJSON(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nallowed-tools: Read,Grep\nmodel: claude-sonnet-4-5\n---\nRun verification."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	tool := tools.NewSkillTool()
	appState := map[string]any{
		"skillRoots": []any{filepath.Join(dir, "skills")},
	}

	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{AgentID: "main"},
		ToolName: "Skill",
		Input:    `{"skill":"verify","args":"unit tests"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["status"] != "inline" || parsed["commandName"] != "verify" || parsed["model"] != "claude-sonnet-4-5" {
		t.Fatalf("output = %#v, want inline skill metadata", parsed)
	}
	if !strings.Contains(parsed["content"].(string), "Run verification.") || !strings.Contains(parsed["content"].(string), "unit tests") {
		t.Fatalf("content = %q, want skill content and args", parsed["content"])
	}
}

func TestSkillToolKeepsDirectoryNameWhenFrontmatterHasDisplayName(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	skillDir := filepath.Join(root, "verify")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: Verification Display\ndescription: Run verification.\n---\nUse this skill."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"skill":"verify"}`,
		AppState: map[string]any{"skillRoots": []any{root}},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	if parsed["commandName"] != "verify" {
		t.Fatalf("commandName = %q, want directory skill name", parsed["commandName"])
	}
}

func TestSkillToolRejectsDisableModelInvocationAndReportsForkedSkills(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	blockedDir := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatalf("mkdir blocked skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedDir, "SKILL.md"), []byte("---\ndisable-model-invocation: true\n---\nSecret."), 0o644); err != nil {
		t.Fatalf("write blocked skill: %v", err)
	}
	tool := tools.NewSkillTool()
	_, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"skill":"blocked"}`,
		AppState: map[string]any{"skillRoots": []any{root}},
	})
	if err == nil || !strings.Contains(err.Error(), "disable-model-invocation") {
		t.Fatalf("error = %v, want disable-model-invocation rejection", err)
	}

	forkDir := filepath.Join(root, "forked")
	if err := os.MkdirAll(forkDir, 0o755); err != nil {
		t.Fatalf("mkdir forked skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(forkDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\n---\nFork this."), 0o644); err != nil {
		t.Fatalf("write forked skill: %v", err)
	}
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		AgentID:  "agent-main",
		Input:    `{"skill":"forked"}`,
		AppState: map[string]any{"skillRoots": []any{root}},
	})
	if err != nil {
		t.Fatalf("forked skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("forked output JSON: %v", err)
	}
	if parsed["status"] != "forked" || parsed["agent"] != "verifier" {
		t.Fatalf("output = %#v, want forked skill metadata", parsed)
	}
}

func TestSkillToolExpandsArgumentsSessionAndSkillDirPlaceholders(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "expand")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\narguments: first second\n---\nskill=${CLAUDE_SKILL_DIR}\nsession=${CLAUDE_SESSION_ID}\nargs=$ARGUMENTS\nfirst=$0\nsecond=$1"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	tool := tools.NewSkillTool()
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "session-123", AgentID: "agent-1"},
		ToolName: "Skill",
		Input:    `{"skill":"expand","args":"alpha beta"}`,
		AppState: map[string]any{"skillRoots": []any{filepath.Join(dir, "skills")}},
	})
	if err != nil {
		t.Fatalf("invoke skill: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("skill output JSON: %v\n%s", err, result.Output)
	}
	content, _ := parsed["content"].(string)
	if !strings.Contains(content, "Base directory for this skill: "+skillDir) {
		t.Fatalf("content = %q, want skill base directory prefix", content)
	}
	if !strings.Contains(content, "skill="+filepath.ToSlash(skillDir)) {
		t.Fatalf("content = %q, want skill dir expansion", content)
	}
	if !strings.Contains(content, "session=session-123") {
		t.Fatalf("content = %q, want session ID expansion", content)
	}
	if !strings.Contains(content, "args=alpha beta") || !strings.Contains(content, "first=alpha") || !strings.Contains(content, "second=beta") {
		t.Fatalf("content = %q, want argument expansion", content)
	}
}

func TestSkillInvokedSkillsAreAgentScoped(t *testing.T) {
	appState := map[string]any{}
	tools.AddInvokedSkill(appState, tools.InvokedSkillInfo{
		SkillName: "alpha",
		SkillPath: "/tmp/alpha/SKILL.md",
		Content:   "alpha content",
		AgentID:   "agent-a",
	})
	tools.AddInvokedSkill(appState, tools.InvokedSkillInfo{
		SkillName: "beta",
		SkillPath: "/tmp/beta/SKILL.md",
		Content:   "beta content",
		AgentID:   "agent-b",
	})

	agentA := tools.GetInvokedSkillsForAgent(appState, "agent-a")
	if len(agentA) != 1 || agentA[0].SkillName != "alpha" {
		t.Fatalf("agent-a skills = %#v, want alpha only", agentA)
	}

	tools.ClearInvokedSkillsForAgent(appState, "agent-a")
	agentA = tools.GetInvokedSkillsForAgent(appState, "agent-a")
	if len(agentA) != 0 {
		t.Fatalf("agent-a skills after clear = %#v, want none", agentA)
	}
	agentB := tools.GetInvokedSkillsForAgent(appState, "agent-b")
	if len(agentB) != 1 || agentB[0].SkillName != "beta" {
		t.Fatalf("agent-b skills = %#v, want beta preserved", agentB)
	}
}

func TestSkillToolUsesForkExecutorHookFromAppState(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	forkDir := filepath.Join(root, "forked")
	if err := os.MkdirAll(forkDir, 0o755); err != nil {
		t.Fatalf("mkdir forked skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(forkDir, "SKILL.md"), []byte("---\ncontext: fork\nagent: verifier\nallowed-tools: Read,Write\n---\nFork this."), 0o644); err != nil {
		t.Fatalf("write forked skill: %v", err)
	}
	called := false
	appState := map[string]any{
		"skillRoots": []any{root},
		"skillForkExecutor": func(_ context.Context, request tools.SkillForkRequest) (tools.ToolResult, error) {
			called = true
			if request.Command.Name != "forked" || request.Command.Agent != "verifier" {
				t.Fatalf("request = %#v, want forked verifier skill", request)
			}
			if request.Args != "" {
				t.Fatalf("request args = %q, want empty", request.Args)
			}
			return tools.ToolResult{Output: `{"status":"forked","agent":"verifier","result":"executed"}`}, nil
		},
	}
	result, err := tools.NewSkillTool().InvokeWithContext(context.Background(), tools.ToolUseContext{
		Session:  session.Session{ID: "session-1", AgentID: "agent-main"},
		ToolName: "Skill",
		Input:    `{"skill":"forked"}`,
		AppState: appState,
	})
	if err != nil {
		t.Fatalf("invoke forked skill: %v", err)
	}
	if !called {
		t.Fatal("expected fork executor hook to be called")
	}
	if !strings.Contains(result.Output, `"status":"forked"`) || !strings.Contains(result.Output, `"agent":"verifier"`) {
		t.Fatalf("output = %q, want fork executor output", result.Output)
	}
}

func TestWebSearchRejectsConflictingDomainFilters(t *testing.T) {
	tool := tools.NewWebSearchTool()
	_, err := tool.Invoke(context.Background(), session.Session{}, `{"query":"golang","allowed_domains":["go.dev"],"blocked_domains":["example.com"]}`)
	if err == nil || !strings.Contains(err.Error(), "allowed_domains and blocked_domains cannot both be set") {
		t.Fatalf("error = %v, want conflicting domain filters rejection", err)
	}
}

func TestReadToolFormatsNotebookCells(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.ipynb")
	notebook := `{"cells":[{"cell_type":"markdown","id":"intro","source":["# Title\n"],"metadata":{}},{"cell_type":"code","id":"run","source":["print(1)\n"],"metadata":{},"outputs":[],"execution_count":7}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(path, []byte(notebook), 0o644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	tool := tools.NewReadTool()
	got, err := tool.Invoke(context.Background(), session.Session{}, `{"file_path":"`+strings.ReplaceAll(path, `\`, `\\`)+`"}`)
	if err != nil {
		t.Fatalf("read notebook: %v", err)
	}
	if !strings.Contains(got, `<cell id="intro"`) || !strings.Contains(got, `cellType="markdown"`) || !strings.Contains(got, "print(1)") {
		t.Fatalf("notebook read output = %q, want cell-formatted notebook", got)
	}
	if strings.Contains(got, `"cells"`) {
		t.Fatalf("notebook read output = %q, did not want raw JSON", got)
	}
}

func TestNotebookEditToolRejectsUntilNotebookBackendExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.ipynb")
	notebook := `{"cells":[{"cell_type":"code","id":"cell-1","source":["print(0)\n"],"metadata":{},"outputs":[{"output_type":"stream","text":["old\n"]}],"execution_count":7}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	if err := os.WriteFile(path, []byte(notebook), 0o644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	tool := tools.NewNotebookEditTool()
	got, err := tool.Invoke(context.Background(), session.Session{}, `{"notebook_path":"`+strings.ReplaceAll(path, `\`, `\\`)+`","cell_id":"cell-1","new_source":"print(1)","edit_mode":"replace"}`)
	if err != nil {
		t.Fatalf("notebook edit: %v", err)
	}
	if !strings.Contains(got, "Notebook edited") || !strings.Contains(got, "cell-1") {
		t.Fatalf("output = %q, want edit summary", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read notebook: %v", err)
	}
	if !strings.Contains(string(data), "print(1)") || strings.Contains(string(data), "print(0)") {
		t.Fatalf("notebook = %s, want replaced source", string(data))
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse notebook: %v", err)
	}
	cell := parsed["cells"].([]any)[0].(map[string]any)
	if cell["execution_count"] != nil {
		t.Fatalf("cell = %#v, want execution_count cleared after code edit", cell)
	}
	outputs, _ := cell["outputs"].([]any)
	if len(outputs) != 0 {
		t.Fatalf("cell = %#v, want outputs cleared after code edit", cell)
	}
}

func TestNotebookEditRequiresIPynbAndInsertCellType(t *testing.T) {
	tool := tools.NewNotebookEditTool()
	textPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(textPath, []byte("notebook?"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}
	_, err := tool.Invoke(context.Background(), session.Session{}, `{"notebook_path":"`+strings.ReplaceAll(textPath, `\`, `\\`)+`","new_source":"print(1)"}`)
	if err == nil || !strings.Contains(err.Error(), ".ipynb") {
		t.Fatalf("error = %v, want non-ipynb rejection", err)
	}

	notebookPath := filepath.Join(t.TempDir(), "demo.ipynb")
	if err := os.WriteFile(notebookPath, []byte(`{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`), 0o644); err != nil {
		t.Fatalf("write notebook: %v", err)
	}
	_, err = tool.Invoke(context.Background(), session.Session{}, `{"notebook_path":"`+strings.ReplaceAll(notebookPath, `\`, `\\`)+`","new_source":"# new","edit_mode":"insert"}`)
	if err == nil || !strings.Contains(err.Error(), "cell_type") {
		t.Fatalf("error = %v, want insert cell_type rejection", err)
	}
}

func TestPlanModeToolsReturnModeTransitionMarkers(t *testing.T) {
	enter := tools.NewEnterPlanModeTool()
	exit := tools.NewExitPlanModeTool()
	if exit.Definition().ReadOnly {
		t.Fatal("ExitPlanMode must not be read-only because Claude writes/approves a plan transition")
	}
	properties := exit.Definition().InputSchema["properties"].(map[string]any)
	if properties["allowedPrompts"] == nil {
		t.Fatalf("exit schema = %#v, want allowedPrompts", exit.Definition().InputSchema)
	}

	entered, err := enter.Invoke(context.Background(), session.Session{}, `{"plan":"inspect first"}`)
	if err != nil {
		t.Fatalf("enter plan mode: %v", err)
	}
	if !strings.Contains(entered, "Plan mode entered") {
		t.Fatalf("enter output = %q, want plan mode marker", entered)
	}
	exited, err := exit.Invoke(context.Background(), session.Session{}, `{"plan":"approved"}`)
	if err != nil {
		t.Fatalf("exit plan mode: %v", err)
	}
	if !strings.Contains(exited, "Plan mode exited") {
		t.Fatalf("exit output = %q, want plan mode exit marker", exited)
	}
}

func TestExitPlanModeContextWritesPlanAndRestoresAppState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.md")
	appState := map[string]any{
		"toolPermissionContext": map[string]any{
			"mode":        "plan",
			"prePlanMode": "default",
		},
	}
	tool := tools.NewExitPlanModeTool()
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"plan":"1. Run tests","planFilePath":"` + strings.ReplaceAll(path, `\`, `\\`) + `","allowedPrompts":[{"tool":"Bash","prompt":"run tests"}]}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("exit plan mode: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan file: %v", err)
	}
	if string(data) != "1. Run tests" {
		t.Fatalf("plan file = %q, want written plan", string(data))
	}
	if !strings.Contains(result.Output, `"plan":"1. Run tests"`) || !strings.Contains(result.Output, `"allowedPrompts"`) {
		t.Fatalf("output = %q, want Claude-style plan output JSON", result.Output)
	}
	permissionContext := appState["toolPermissionContext"].(map[string]any)
	if permissionContext["mode"] != "default" || appState["hasExitedPlanMode"] != true || appState["needsPlanModeExitAttachment"] != true {
		t.Fatalf("appState = %#v, want restored mode and plan exit markers", appState)
	}
}

func TestExitPlanModeContextReadsPlanFromFileWhenInputPlanMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(path, []byte("1. Existing disk plan"), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	appState := map[string]any{
		"toolPermissionContext": map[string]any{
			"mode":        "plan",
			"prePlanMode": "acceptEdits",
		},
	}
	tool := tools.NewExitPlanModeTool()
	result, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"planFilePath":"` + strings.ReplaceAll(path, `\`, `\\`) + `"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err != nil {
		t.Fatalf("exit plan mode: %v", err)
	}
	if !strings.Contains(result.Output, `"plan":"1. Existing disk plan"`) {
		t.Fatalf("output = %q, want plan recovered from disk", result.Output)
	}
	permissionContext := appState["toolPermissionContext"].(map[string]any)
	if permissionContext["mode"] != "acceptEdits" {
		t.Fatalf("appState = %#v, want restored pre-plan mode", appState)
	}
}

func TestExitPlanModeContextRejectsOutsidePlanModeWithoutMutatingState(t *testing.T) {
	appState := map[string]any{
		"toolPermissionContext": map[string]any{
			"mode": "default",
		},
	}
	tool := tools.NewExitPlanModeTool()
	_, err := tool.(tools.ContextualTool).InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:    `{"plan":"approved"}`,
		AppState: appState,
		SetAppState: func(update func(map[string]any) map[string]any) {
			appState = update(appState)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "only for exiting plan mode") {
		t.Fatalf("error = %v, want outside-plan-mode rejection", err)
	}
	if appState["hasExitedPlanMode"] != nil || appState["needsPlanModeExitAttachment"] != nil {
		t.Fatalf("appState = %#v, should not mutate outside plan mode", appState)
	}
}
