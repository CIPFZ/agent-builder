package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

type WebFetcher interface {
	Fetch(context.Context, string, string) (string, error)
}

type webFetchTool struct {
	fetcher WebFetcher
}

func NewWebFetchTool(fetcher WebFetcher) Tool {
	if fetcher == nil {
		fetcher = httpWebFetcher{client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}}
	}
	return &webFetchTool{fetcher: fetcher}
}

type httpWebFetcher struct {
	client *http.Client
}

func (f httpWebFetcher) Fetch(ctx context.Context, rawURL, prompt string) (string, error) {
	return f.fetch(ctx, rawURL, prompt, 0)
}

func (f httpWebFetcher) fetch(ctx context.Context, rawURL, prompt string, redirects int) (string, error) {
	if redirects > 10 {
		return "", fmt.Errorf("WebFetch exceeded redirect limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "myclaw-webfetch/1.0")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location != "" {
			redirectURL, err := req.URL.Parse(location)
			if err != nil {
				return "", err
			}
			if isSafeWebFetchRedirect(req.URL, redirectURL) {
				return f.fetch(ctx, redirectURL.String(), prompt, redirects+1)
			}
			message := fmt.Sprintf("WebFetch was redirected to %s. Please make a new WebFetch request with the redirected URL.", redirectURL.String())
			return webFetchPayloadWithRedirect(0, resp.StatusCode, resp.Status, message, rawURL, map[string]any{
				"originalUrl": rawURL,
				"redirectUrl": redirectURL.String(),
				"statusCode":  resp.StatusCode,
				"statusText":  resp.Status,
			}), nil
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	text := htmlToText(string(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("WebFetch failed with HTTP %d %s: %s", resp.StatusCode, resp.Status, text)
	}
	return webFetchPayload(len(body), resp.StatusCode, resp.Status, fmt.Sprintf("URL: %s\nPrompt: %s\n\n%s", rawURL, prompt, text), rawURL), nil
}

func isSafeWebFetchRedirect(from, to *url.URL) bool {
	if from == nil || to == nil {
		return false
	}
	if !strings.EqualFold(from.Scheme, to.Scheme) {
		return false
	}
	if to.User != nil {
		return false
	}
	if normalizeWebFetchHost(from.Hostname()) != normalizeWebFetchHost(to.Hostname()) {
		return false
	}
	return from.Port() == to.Port()
}

func normalizeWebFetchHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimPrefix(host, "www.")
}

func htmlToText(input string) string {
	text := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(input, "")
	text = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr)>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(text, "")
	for old, newValue := range map[string]string{
		"&nbsp;": " ",
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": `"`,
		"&#39;":  "'",
	} {
		text = strings.ReplaceAll(text, old, newValue)
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func (t *webFetchTool) Definition() Definition {
	return Definition{
		Name:        "WebFetch",
		Description: "Fetches content from a URL and processes it using the provided prompt.",
		InputSchema: objectSchema(map[string]any{
			"url":    map[string]any{"type": "string"},
			"prompt": map[string]any{"type": "string"},
		}, []string{"url", "prompt"}),
		Enabled:     true,
		ReadOnly:    true,
		Destructive: false,
	}
}

func (t *webFetchTool) Invoke(ctx context.Context, _ session.Session, input string) (string, error) {
	object, err := objectInput(input)
	if err != nil {
		return "", err
	}
	rawURL := stringField(object, "url")
	prompt := stringField(object, "prompt")
	if rawURL == "" || prompt == "" {
		return "", fmt.Errorf("WebFetch requires url and prompt")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("WebFetch requires an absolute URL")
	}
	return t.fetcher.Fetch(ctx, rawURL, prompt)
}

func (t *webFetchTool) CheckPermissionsWithContext(_ context.Context, toolCtx ToolUseContext) (permissions.Decision, error) {
	toolCtx = toolCtx.Normalized()
	object := toolCtx.InputObject
	rawURL := stringField(object, "url")
	if rawURL == "" {
		return permissions.Decision{}, fmt.Errorf("WebFetch requires url")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return permissions.Decision{}, fmt.Errorf("WebFetch requires an absolute URL")
	}
	return toolCtx.Policy.Evaluate(permissions.Request{
		ToolName:    "WebFetch",
		Command:     "domain:" + strings.ToLower(parsed.Hostname()),
		ReadOnly:    true,
		Destructive: false,
	}), nil
}

func (t *webFetchTool) IsEnabled() bool           { return true }
func (t *webFetchTool) IsReadOnly(string) bool    { return true }
func (t *webFetchTool) IsDestructive(string) bool { return false }
func (t *webFetchTool) ShouldDefer() bool         { return false }
func (t *webFetchTool) AlwaysLoad() bool          { return false }
func (t *webFetchTool) PromptDescription() string {
	return strings.TrimSpace(`Fetches content from a URL and processes it using the provided prompt.

Authentication, private networks, and private URLs may fail. Prefer a dedicated MCP web fetch tool when one is available. HTTP URLs may redirect or upgrade to HTTPS; if WebFetch reports a redirect, call WebFetch again with the redirected URL. For GitHub resources, prefer gh when it is available.`)
}
func (t *webFetchTool) SearchHint() string { return "web fetch url website http https" }

func webFetchPayload(bytes int, code int, codeText, result, rawURL string) string {
	return webFetchPayloadWithRedirect(bytes, code, codeText, result, rawURL, nil)
}

func webFetchPayloadWithRedirect(bytes int, code int, codeText, result, rawURL string, redirect map[string]any) string {
	if bytes == 0 {
		bytes = len([]byte(result))
	}
	payload := map[string]any{
		"bytes":      bytes,
		"code":       code,
		"codeText":   codeText,
		"result":     result,
		"durationMs": 0,
		"url":        rawURL,
	}
	if redirect != nil {
		payload["redirect"] = redirect
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return result
	}
	return string(encoded)
}

type webSearchTool struct{}

func NewWebSearchTool() Tool { return webSearchTool{} }

func (webSearchTool) Definition() Definition {
	return Definition{
		Name:        "WebSearch",
		Description: "Searches the web for current information.",
		InputSchema: objectSchema(map[string]any{
			"query":           map[string]any{"type": "string"},
			"allowed_domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"blocked_domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}, []string{"query"}),
		Enabled:     true,
		ReadOnly:    true,
		Destructive: false,
	}
}

func (webSearchTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	object, err := objectInput(input)
	if err != nil {
		return "", err
	}
	if stringField(object, "query") == "" {
		return "", fmt.Errorf("WebSearch requires query")
	}
	if len(anySlice(object["allowed_domains"])) > 0 && len(anySlice(object["blocked_domains"])) > 0 {
		return "", fmt.Errorf("allowed_domains and blocked_domains cannot both be set")
	}
	return "", fmt.Errorf("WebSearch backend is not configured")
}

func (webSearchTool) IsEnabled() bool           { return true }
func (webSearchTool) IsReadOnly(string) bool    { return true }
func (webSearchTool) IsDestructive(string) bool { return false }
func (webSearchTool) ShouldDefer() bool         { return false }
func (webSearchTool) AlwaysLoad() bool          { return false }
func (webSearchTool) PromptDescription() string { return webSearchTool{}.Definition().Description }
func (webSearchTool) SearchHint() string        { return "web search current information sources" }

type listMcpResourcesTool struct{}
type readMcpResourceTool struct{}

func NewListMcpResourcesTool() listMcpResourcesTool { return listMcpResourcesTool{} }
func NewReadMcpResourceTool() readMcpResourceTool   { return readMcpResourceTool{} }

func (listMcpResourcesTool) Definition() Definition {
	return Definition{
		Name:        "ListMcpResources",
		Description: "Lists resources exposed by connected MCP servers.",
		InputSchema: objectSchema(map[string]any{"server": map[string]any{"type": "string"}}, nil),
		Enabled:     true,
		ReadOnly:    true,
	}
}

func (listMcpResourcesTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return "", fmt.Errorf("ListMcpResources requires ToolUseContext")
}

func (listMcpResourcesTool) InvokeWithContext(_ context.Context, toolCtx ToolUseContext) (ToolResult, error) {
	filter := stringField(toolCtx.InputObject, "server")
	if filter == "" {
		if object, ok := parseObjectInput(toolCtx.Input); ok {
			filter = stringField(object, "server")
		}
	}
	var servers []string
	for server := range toolCtx.MCPResources {
		if filter == "" || server == filter {
			servers = append(servers, server)
		}
	}
	sort.Strings(servers)
	if len(servers) == 0 {
		return ToolResult{Output: "No resources found."}, nil
	}
	var lines []string
	for _, server := range servers {
		for _, resource := range toolCtx.MCPResources[server] {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s", server, resource.URI, resource.Name, resource.Description))
		}
	}
	return ToolResult{Output: strings.Join(lines, "\n")}, nil
}

func (readMcpResourceTool) Definition() Definition {
	return Definition{
		Name:        "ReadMcpResource",
		Description: "Reads a resource exposed by a connected MCP server.",
		InputSchema: objectSchema(map[string]any{
			"server": map[string]any{"type": "string"},
			"uri":    map[string]any{"type": "string"},
		}, []string{"server", "uri"}),
		Enabled:  true,
		ReadOnly: true,
	}
}

func (readMcpResourceTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	return "", fmt.Errorf("ReadMcpResource requires ToolUseContext")
}

func (readMcpResourceTool) InvokeWithContext(_ context.Context, toolCtx ToolUseContext) (ToolResult, error) {
	object := toolCtx.InputObject
	if object == nil {
		object, _ = parseObjectInput(toolCtx.Input)
	}
	server := stringField(object, "server")
	uri := stringField(object, "uri")
	for _, resource := range toolCtx.MCPResources[server] {
		if resource.URI == uri {
			text := resource.Description
			if text == "" {
				text = resource.Name
			}
			encoded, err := json.Marshal(map[string]any{
				"contents": []map[string]any{{
					"uri":  resource.URI,
					"text": text,
				}},
			})
			if err != nil {
				return ToolResult{}, err
			}
			return ToolResult{Output: string(encoded)}, nil
		}
	}
	return ToolResult{}, fmt.Errorf("MCP resource %q not found on server %q", uri, server)
}

func (listMcpResourcesTool) IsEnabled() bool           { return true }
func (listMcpResourcesTool) IsReadOnly(string) bool    { return true }
func (listMcpResourcesTool) IsDestructive(string) bool { return false }
func (listMcpResourcesTool) ShouldDefer() bool         { return false }
func (listMcpResourcesTool) AlwaysLoad() bool          { return false }
func (listMcpResourcesTool) PromptDescription() string {
	return listMcpResourcesTool{}.Definition().Description
}
func (listMcpResourcesTool) SearchHint() string { return "mcp resources list" }

func (readMcpResourceTool) IsEnabled() bool           { return true }
func (readMcpResourceTool) IsReadOnly(string) bool    { return true }
func (readMcpResourceTool) IsDestructive(string) bool { return false }
func (readMcpResourceTool) ShouldDefer() bool         { return false }
func (readMcpResourceTool) AlwaysLoad() bool          { return false }
func (readMcpResourceTool) PromptDescription() string {
	return readMcpResourceTool{}.Definition().Description
}
func (readMcpResourceTool) SearchHint() string { return "mcp resource read uri" }

type MCPToolDefinition struct {
	Server      string
	Name        string
	Description string
	InputSchema map[string]any
	ReadOnly    bool
	Destructive bool
}

type MCPToolResult struct {
	Content           []map[string]any `json:"content,omitempty"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	Meta              map[string]any   `json:"_meta,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type MCPToolCaller func(context.Context, string, string, map[string]any) (MCPToolResult, error)

type mcpTool struct {
	def    MCPToolDefinition
	caller MCPToolCaller
}

func NewMCPTool(def MCPToolDefinition, caller MCPToolCaller) Tool {
	return &mcpTool{def: def, caller: caller}
}

func BuildMCPToolName(server, name string) string {
	return "mcp__" + normalizeMCPName(server) + "__" + normalizeMCPName(name)
}

func normalizeMCPName(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	previousUnderscore := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if valid {
			builder.WriteRune(r)
			previousUnderscore = false
			continue
		}
		if !previousUnderscore {
			builder.WriteByte('_')
			previousUnderscore = true
		}
	}
	normalized := strings.Trim(builder.String(), "_")
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func (t *mcpTool) Definition() Definition {
	return Definition{
		Name:        BuildMCPToolName(t.def.Server, t.def.Name),
		Description: strings.TrimSpace(t.def.Description),
		InputSchema: deepCloneAnyMap(t.def.InputSchema),
		Source:      "mcp",
		Enabled:     true,
		ReadOnly:    !t.def.Destructive,
		Destructive: t.def.Destructive,
	}
}

func (t *mcpTool) Invoke(ctx context.Context, _ session.Session, input string) (string, error) {
	if t.caller == nil {
		return "", fmt.Errorf("MCP tool %s/%s has no caller", t.def.Server, t.def.Name)
	}
	object, err := objectInput(input)
	if err != nil {
		return "", err
	}
	result, err := t.caller(ctx, t.def.Server, t.def.Name, object)
	if err != nil {
		return "", fmt.Errorf("MCP tool %s/%s failed: %w", t.def.Server, t.def.Name, err)
	}
	encoded, err := json.Marshal(mcpResultEnvelope{
		Content:           result.Content,
		StructuredContent: result.StructuredContent,
		Meta:              result.Meta,
		IsError:           result.IsError,
	})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (t *mcpTool) IsEnabled() bool           { return true }
func (t *mcpTool) IsReadOnly(string) bool    { return !t.def.Destructive }
func (t *mcpTool) IsDestructive(string) bool { return t.def.Destructive }
func (t *mcpTool) ShouldDefer() bool         { return false }
func (t *mcpTool) AlwaysLoad() bool          { return false }
func (t *mcpTool) PromptDescription() string { return t.Definition().Description }
func (t *mcpTool) SearchHint() string {
	return strings.Join([]string{"mcp", t.def.Server, t.def.Name}, " ")
}

type mcpResultEnvelope struct {
	Content           []map[string]any `json:"content,omitempty"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	Meta              map[string]any   `json:"_meta,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type skillTool struct{}

func NewSkillTool() skillTool { return skillTool{} }

func (skillTool) Definition() Definition {
	return Definition{
		Name:        "Skill",
		Description: "Loads and invokes a local skill by name.",
		InputSchema: objectSchema(map[string]any{
			"skill": map[string]any{"type": "string"},
			"args":  map[string]any{"type": "string"},
		}, []string{"skill"}),
		Enabled: true,
	}
}

func (skillTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	object, err := objectInput(input)
	if err != nil {
		return "", err
	}
	command, err := resolveSkillCommand(object, nil)
	if err != nil {
		return "", err
	}
	if command.DisableModelInvocation {
		return "", fmt.Errorf("Skill %q has disable-model-invocation enabled", command.Name)
	}
	return "Launching skill: " + command.Name + "\n\n" + command.renderedContent(stringField(object, "args"), ""), nil
}

func (skillTool) InvokeWithContext(_ context.Context, toolCtx ToolUseContext) (ToolResult, error) {
	toolCtx = toolCtx.Normalized()
	object := toolCtx.InputObject
	if object == nil {
		object, _ = parseObjectInput(toolCtx.Input)
	}
	command, err := resolveSkillCommand(object, toolCtx.AppState)
	if err != nil {
		return ToolResult{}, err
	}
	if command.DisableModelInvocation {
		return ToolResult{}, fmt.Errorf("Skill %q has disable-model-invocation enabled", command.Name)
	}
	args := stringField(object, "args")
	if command.MCPPrompt {
		caller := mcpPromptCallerFromAppState(toolCtx.AppState)
		if caller == nil {
			return ToolResult{}, fmt.Errorf("MCP prompt skill %q has no prompt caller", command.Name)
		}
		arguments := mapSkillArgsToPromptArguments(args, command.ArgumentNames)
		promptResult, err := caller(toolCtx.AbortContext, command.MCPServer, command.MCPPromptName, arguments)
		if err != nil {
			return ToolResult{}, err
		}
		content := renderMCPPromptMessages(promptResult)
		toolCtx.SetAppState(func(previous map[string]any) map[string]any {
			next := cloneAnyMap(previous)
			if next == nil {
				next = make(map[string]any)
			}
			AddInvokedSkill(next, InvokedSkillInfo{
				SkillName: command.Name,
				SkillPath: command.Path,
				Content:   content,
				InvokedAt: time.Now().UTC(),
				AgentID:   toolCtx.AgentID,
			})
			return next
		})
		output := map[string]any{
			"success":     true,
			"commandName": command.Name,
			"status":      "inline",
			"content":     "Launching skill: " + command.Name + "\n\n" + content,
		}
		encoded, err := json.Marshal(output)
		if err != nil {
			return ToolResult{}, err
		}
		return ToolResult{Output: string(encoded)}, nil
	}
	content := command.renderedContent(args, toolCtx.Session.ID)
	toolCtx.SetAppState(func(previous map[string]any) map[string]any {
		next := cloneAnyMap(previous)
		if next == nil {
			next = make(map[string]any)
		}
		AddInvokedSkill(next, InvokedSkillInfo{
			SkillName: command.Name,
			SkillPath: command.Path,
			Content:   content,
			InvokedAt: time.Now().UTC(),
			AgentID:   toolCtx.AgentID,
		})
		return next
	})
	if strings.EqualFold(command.Context, "fork") {
		if executor := skillForkExecutorFromAppState(toolCtx.AppState); executor != nil {
			forked, err := executor(toolCtx.AbortContext, SkillForkRequest{
				Command:     command,
				Args:        args,
				ToolContext: toolCtx,
			})
			if err != nil {
				return ToolResult{}, err
			}
			return forked, nil
		}
		output := map[string]any{
			"success":      true,
			"commandName":  command.Name,
			"status":       "forked",
			"content":      "Launching skill: " + command.Name + "\n\n" + content,
			"allowedTools": command.AllowedTools,
			"agent":        command.Agent,
			"result":       "Skill requires forked execution.",
		}
		if command.Model != "" {
			output["model"] = command.Model
		}
		encoded, err := json.Marshal(output)
		if err != nil {
			return ToolResult{}, err
		}
		return ToolResult{Output: string(encoded)}, nil
	}
	output := map[string]any{
		"success":      true,
		"commandName":  command.Name,
		"status":       "inline",
		"content":      "Launching skill: " + command.Name + "\n\n" + content,
		"allowedTools": command.AllowedTools,
	}
	if command.Model != "" {
		output["model"] = command.Model
	}
	if strings.EqualFold(command.Context, "fork") {
		output["status"] = "forked"
		output["agent"] = command.Agent
		output["result"] = "Skill requires forked execution."
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Output: string(encoded)}, nil
}

type skillCommand struct {
	Name                   string
	Path                   string
	Content                string
	ArgumentNames          []string
	AllowedTools           []string
	Model                  string
	Context                string
	Agent                  string
	DisableModelInvocation bool
	MCPPrompt              bool
	MCPServer              string
	MCPPromptName          string
}

func (c skillCommand) renderedContent(args, sessionID string) string {
	content := c.Content
	baseDir := skillBaseDir(c.Path)
	if baseDir != "" {
		content = "Base directory for this skill: " + baseDir + "\n\n" + content
	}
	content = substituteSkillArguments(content, args, c.ArgumentNames)
	if baseDir != "" {
		normalizedDir := baseDir
		if runtime.GOOS == "windows" {
			normalizedDir = strings.ReplaceAll(normalizedDir, `\`, `/`)
		}
		content = strings.ReplaceAll(content, "${CLAUDE_SKILL_DIR}", normalizedDir)
	}
	content = strings.ReplaceAll(content, "${CLAUDE_SESSION_ID}", sessionID)
	return content
}

func resolveSkillCommand(object map[string]any, appState map[string]any) (skillCommand, error) {
	path := stringField(object, "path")
	name := stringField(object, "skill")
	if name == "" {
		name = stringField(object, "name")
	}
	if name == "" {
		return skillCommand{}, fmt.Errorf("Skill requires skill")
	}
	if command, ok := resolveMCPPromptSkill(name, appState); ok {
		return command, nil
	}
	if path == "" {
		for _, root := range stringList(appState["skillRoots"]) {
			candidate := filepath.Join(root, name, "SKILL.md")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		return skillCommand{}, fmt.Errorf("Skill %q is not available without a local path", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return skillCommand{}, err
	}
	command := parseSkillFile(name, path, string(data))
	return command, nil
}

func resolveMCPPromptSkill(name string, appState map[string]any) (skillCommand, bool) {
	if appState == nil {
		return skillCommand{}, false
	}
	servers, _ := appState["mcpPrompts"].(map[string]MCPPromptsListResult)
	if len(servers) == 0 {
		return skillCommand{}, false
	}
	for server, result := range servers {
		prefix := "mcp__" + normalizeMCPName(server) + "__"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		promptName := strings.TrimPrefix(name, prefix)
		for _, prompt := range result.Prompts {
			if prompt.Name != promptName {
				continue
			}
			args := make([]string, 0, len(prompt.Arguments))
			for _, argument := range prompt.Arguments {
				args = append(args, argument.Name)
			}
			return skillCommand{
				Name:          name,
				Content:       prompt.Description,
				ArgumentNames: args,
				MCPPrompt:     true,
				MCPServer:     server,
				MCPPromptName: prompt.Name,
			}, true
		}
	}
	return skillCommand{}, false
}

func mcpPromptCallerFromAppState(appState map[string]any) MCPPromptCaller {
	if appState == nil {
		return nil
	}
	caller, _ := appState["mcpPromptCaller"].(MCPPromptCaller)
	return caller
}

func mapSkillArgsToPromptArguments(args string, names []string) map[string]any {
	arguments := make(map[string]any)
	values := splitSkillArguments(args)
	for i, name := range names {
		if name == "" {
			continue
		}
		arguments[name] = valueAt(values, i)
	}
	return arguments
}

func renderMCPPromptMessages(result MCPPromptResult) string {
	parts := make([]string, 0, len(result.Messages))
	for _, message := range result.Messages {
		text := strings.TrimSpace(renderMCPPromptContent(message.Content))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func renderMCPPromptContent(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case map[string]any:
		if text := stringField(typed, "text"); text != "" {
			return text
		}
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(renderMCPPromptContent(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		encoded, _ := json.Marshal(typed)
		return string(encoded)
	}
}

func parseSkillFile(name, path, data string) skillCommand {
	command := skillCommand{Name: name, Path: path, Content: data}
	if strings.HasPrefix(data, "---") {
		rest := strings.TrimPrefix(data, "---")
		rest = strings.TrimPrefix(rest, "\r\n")
		rest = strings.TrimPrefix(rest, "\n")
		if idx := strings.Index(rest, "\n---"); idx >= 0 {
			frontmatter := rest[:idx]
			body := rest[idx+len("\n---"):]
			body = strings.TrimPrefix(body, "\r\n")
			body = strings.TrimPrefix(body, "\n")
			command.Content = body
			for _, line := range strings.Split(frontmatter, "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				key = strings.TrimSpace(strings.ToLower(key))
				value = strings.TrimSpace(value)
				switch key {
				case "name":
					// Claude keeps the directory/file-derived command name.
					// Frontmatter name is only the user-facing display name.
				case "allowed-tools":
					command.AllowedTools = splitSkillFrontmatterList(value)
				case "arguments":
					command.ArgumentNames = splitSkillFrontmatterList(value)
				case "model":
					command.Model = value
				case "context":
					command.Context = value
				case "agent":
					command.Agent = value
				case "disable-model-invocation":
					command.DisableModelInvocation = strings.EqualFold(value, "true")
				}
			}
		}
	}
	return command
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func (skillTool) IsEnabled() bool           { return true }
func (skillTool) IsReadOnly(string) bool    { return true }
func (skillTool) IsDestructive(string) bool { return false }
func (skillTool) ShouldDefer() bool         { return false }
func (skillTool) AlwaysLoad() bool          { return false }
func (skillTool) PromptDescription() string { return skillTool{}.Definition().Description }
func (skillTool) SearchHint() string        { return "skill invoke local instructions" }

type notebookEditTool struct{}

func NewNotebookEditTool() Tool { return notebookEditTool{} }

func (notebookEditTool) Definition() Definition {
	return Definition{
		Name:        "NotebookEdit",
		Description: "Edits a Jupyter notebook cell.",
		InputSchema: objectSchema(map[string]any{
			"notebook_path": map[string]any{"type": "string"},
			"cell_id":       map[string]any{"type": "string"},
			"new_source":    map[string]any{"type": "string"},
			"cell_type":     map[string]any{"type": "string"},
			"edit_mode":     map[string]any{"type": "string"},
		}, []string{"notebook_path", "new_source"}),
		Enabled:     true,
		ReadOnly:    false,
		Destructive: true,
	}
}

func (notebookEditTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	object, err := objectInput(input)
	if err != nil {
		return "", err
	}
	path := stringField(object, "notebook_path")
	newSource, _ := object["new_source"].(string)
	if path == "" || newSource == "" {
		return "", fmt.Errorf("NotebookEdit requires notebook_path and new_source")
	}
	if !strings.EqualFold(filepath.Ext(path), ".ipynb") {
		return "", fmt.Errorf("NotebookEdit requires a .ipynb notebook_path")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var notebook map[string]any
	if err := json.Unmarshal(data, &notebook); err != nil {
		return "", fmt.Errorf("invalid notebook JSON: %w", err)
	}
	cells, ok := notebook["cells"].([]any)
	if !ok {
		return "", fmt.Errorf("notebook is missing cells")
	}
	mode := stringField(object, "edit_mode")
	if mode == "" {
		mode = "replace"
	}
	cellID := stringField(object, "cell_id")
	cellType := stringField(object, "cell_type")
	if mode == "insert" && cellType == "" {
		return "", fmt.Errorf("NotebookEdit insert requires cell_type")
	}
	if cellType == "" {
		cellType = "code"
	}
	index := -1
	if cellID != "" {
		for i, cell := range cells {
			cellMap, _ := cell.(map[string]any)
			if stringField(cellMap, "id") == cellID {
				index = i
				break
			}
		}
	}
	switch mode {
	case "replace":
		if index < 0 {
			return "", fmt.Errorf("cell %q not found", cellID)
		}
		cell, ok := cells[index].(map[string]any)
		if !ok {
			return "", fmt.Errorf("cell %q is invalid", cellID)
		}
		updated := cloneAnyMap(cell)
		updated["source"] = notebookSourceLines(newSource)
		updated["cell_type"] = cellType
		if cellType == "code" {
			updated["outputs"] = []any{}
			updated["execution_count"] = nil
		}
		cells[index] = updated
	case "insert":
		newCell := map[string]any{
			"cell_type": cellType,
			"id":        generatedNotebookCellID(len(cells) + 1),
			"metadata":  map[string]any{},
			"source":    notebookSourceLines(newSource),
		}
		if cellType == "code" {
			newCell["outputs"] = []any{}
			newCell["execution_count"] = nil
		}
		insertAt := len(cells)
		if index >= 0 {
			insertAt = index + 1
		}
		cells = append(cells[:insertAt], append([]any{newCell}, cells[insertAt:]...)...)
		cellID = stringField(newCell, "id")
	case "delete":
		if index < 0 {
			return "", fmt.Errorf("cell %q not found", cellID)
		}
		cells = append(cells[:index], cells[index+1:]...)
	default:
		return "", fmt.Errorf("unsupported notebook edit_mode %q", mode)
	}
	notebook["cells"] = cells
	encoded, err := json.MarshalIndent(notebook, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return "", err
	}
	if cellID == "" {
		cellID = "notebook"
	}
	return fmt.Sprintf("Notebook edited: %s (%s %s)", path, mode, cellID), nil
}

func (notebookEditTool) IsEnabled() bool           { return true }
func (notebookEditTool) IsReadOnly(string) bool    { return false }
func (notebookEditTool) IsDestructive(string) bool { return true }
func (notebookEditTool) ShouldDefer() bool         { return false }
func (notebookEditTool) AlwaysLoad() bool          { return false }
func (notebookEditTool) PromptDescription() string {
	return notebookEditTool{}.Definition().Description
}
func (notebookEditTool) SearchHint() string { return "notebook jupyter edit ipynb cell" }

type planModeTool struct {
	name string
}

func NewEnterPlanModeTool() Tool { return planModeTool{name: "EnterPlanMode"} }
func NewExitPlanModeTool() Tool  { return planModeTool{name: "ExitPlanMode"} }

func (t planModeTool) Definition() Definition {
	required := []string{}
	properties := map[string]any{
		"plan": map[string]any{"type": "string"},
	}
	if t.name == "ExitPlanMode" {
		required = nil
		properties["allowedPrompts"] = map[string]any{
			"type": "array",
			"items": objectSchema(map[string]any{
				"toolName":    map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
			}, []string{"toolName", "description"}),
		}
		properties["planFilePath"] = map[string]any{"type": "string"}
	}
	return Definition{
		Name:        t.name,
		Description: strings.TrimSpace(strings.TrimPrefix(t.name, "Enter")),
		InputSchema: objectSchema(properties, required),
		Enabled:     true,
		ReadOnly:    t.name != "ExitPlanMode",
		Destructive: t.name == "ExitPlanMode",
	}
}

func (t planModeTool) Invoke(_ context.Context, _ session.Session, input string) (string, error) {
	object, _ := objectInput(input)
	plan := stringField(object, "plan")
	if t.name == "EnterPlanMode" {
		if plan == "" {
			return "Plan mode entered.", nil
		}
		return "Plan mode entered.\n\n" + plan, nil
	}
	if plan == "" {
		return "", fmt.Errorf("ExitPlanMode requires plan")
	}
	return "Plan mode exited.\n\n" + plan, nil
}

func (t planModeTool) InvokeWithContext(_ context.Context, toolCtx ToolUseContext) (ToolResult, error) {
	toolCtx = toolCtx.Normalized()
	object := toolCtx.InputObject
	if object == nil {
		object, _ = parseObjectInput(toolCtx.Input)
	}
	plan := stringField(object, "plan")
	if t.name == "EnterPlanMode" {
		toolCtx.SetAppState(func(previous map[string]any) map[string]any {
			next := cloneAnyMap(previous)
			if next == nil {
				next = make(map[string]any)
			}
			permissionContext := cloneAnyMap(mapField(next, "toolPermissionContext"))
			if permissionContext == nil {
				permissionContext = make(map[string]any)
			}
			if permissionContext["mode"] != "plan" {
				permissionContext["prePlanMode"] = permissionContext["mode"]
			}
			permissionContext["mode"] = "plan"
			next["toolPermissionContext"] = permissionContext
			return next
		})
		output := map[string]any{"mode": "plan", "plan": plan}
		encoded, err := json.Marshal(output)
		if err != nil {
			return ToolResult{}, err
		}
		return ToolResult{Output: string(encoded)}, nil
	}
	permissionContext := mapField(toolCtx.AppState, "toolPermissionContext")
	if mode := stringField(permissionContext, "mode"); mode != "" && mode != "plan" {
		return ToolResult{}, fmt.Errorf("You are not in plan mode. This tool is only for exiting plan mode after writing a plan. If your plan was already approved, continue with implementation.")
	}
	planFilePath := stringField(object, "planFilePath")
	if plan == "" && planFilePath != "" {
		data, err := os.ReadFile(planFilePath)
		if err != nil {
			return ToolResult{}, fmt.Errorf("No plan file found at %s. Please write your plan to this file before calling ExitPlanMode: %w", planFilePath, err)
		}
		plan = string(data)
	}
	if plan == "" {
		return ToolResult{}, fmt.Errorf("ExitPlanMode requires plan")
	}
	if planFilePath != "" {
		if dir := filepath.Dir(planFilePath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return ToolResult{}, err
			}
		}
		if err := os.WriteFile(planFilePath, []byte(plan), 0o644); err != nil {
			return ToolResult{}, err
		}
	}
	allowedPrompts := anySlice(object["allowedPrompts"])
	toolCtx.SetAppState(func(previous map[string]any) map[string]any {
		next := cloneAnyMap(previous)
		if next == nil {
			next = make(map[string]any)
		}
		permissionContext := cloneAnyMap(mapField(next, "toolPermissionContext"))
		if permissionContext == nil {
			permissionContext = make(map[string]any)
		}
		prePlanMode := stringField(permissionContext, "prePlanMode")
		if prePlanMode == "" {
			prePlanMode = string(permissions.ModeDefault)
		}
		permissionContext["mode"] = prePlanMode
		next["toolPermissionContext"] = permissionContext
		next["hasExitedPlanMode"] = true
		next["needsPlanModeExitAttachment"] = true
		return next
	})
	output := map[string]any{
		"plan":           plan,
		"allowedPrompts": allowedPrompts,
	}
	if planFilePath != "" {
		output["planFilePath"] = planFilePath
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Output: string(encoded)}, nil
}

func (t planModeTool) IsEnabled() bool           { return true }
func (t planModeTool) IsReadOnly(string) bool    { return t.name != "ExitPlanMode" }
func (t planModeTool) IsDestructive(string) bool { return t.name == "ExitPlanMode" }
func (t planModeTool) ShouldDefer() bool         { return false }
func (t planModeTool) AlwaysLoad() bool          { return false }
func (t planModeTool) PromptDescription() string { return t.Definition().Description }
func (t planModeTool) SearchHint() string        { return strings.ToLower(t.name + " plan mode") }

func notebookSourceLines(source string) []string {
	if source == "" {
		return []string{}
	}
	lines := strings.SplitAfter(source, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{source}
	}
	return lines
}

func generatedNotebookCellID(index int) string {
	return fmt.Sprintf("cell-%d", index)
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

func mapField(object map[string]any, key string) map[string]any {
	if object == nil {
		return nil
	}
	value, _ := object[key].(map[string]any)
	return value
}
