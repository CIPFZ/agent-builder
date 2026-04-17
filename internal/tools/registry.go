package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"myclaw/internal/model"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
)

type Definition struct {
	Name        string
	Aliases     []string
	Description string
	InputSchema map[string]any
	Source      string
	SearchHint  string
	Enabled     bool
	ReadOnly    bool
	Destructive bool
	ShouldDefer bool
	AlwaysLoad  bool
}

type Tool interface {
	Definition() Definition
	Invoke(context.Context, session.Session, string) (string, error)
	IsEnabled() bool
	IsReadOnly(string) bool
	IsDestructive(string) bool
	ShouldDefer() bool
	AlwaysLoad() bool
	PromptDescription() string
	SearchHint() string
}

type PolicyAwareTool interface {
	InvokeWithPolicy(context.Context, session.Session, string, permissions.Policy) (string, error)
}

type StructuredTool interface {
	InvokeWithInput(context.Context, session.Session, map[string]any) (string, error)
}

type StructuredPolicyAwareTool interface {
	InvokeWithInputAndPolicy(context.Context, session.Session, map[string]any, permissions.Policy) (string, error)
}

type ToolResult struct {
	Output            string
	StructuredContent any
	Meta              map[string]any
	IsError           bool
	NewMessages       []model.Message
	ContextModifier   func(ToolUseContext) ToolUseContext
}

type ContextualTool interface {
	InvokeWithContext(context.Context, ToolUseContext) (ToolResult, error)
}

type AutoClassifyingTool interface {
	ToAutoClassifierInput(string) any
}

type ResourceLimits struct {
	MaxTokens    int
	MaxSizeBytes int
	MaxResults   int
}

type MCPConnection struct {
	Name                    string
	Type                    string
	BaseURL                 string
	URL                     string
	Command                 string
	Args                    []string
	Env                     map[string]string
	Headers                 map[string]string
	HeadersHelper           string
	AuthURL                 string
	AuthScope               string
	AuthResourceMetadataURL string
	AuthChallenge           map[string]string
}

type MCPResource struct {
	URI         string
	URITemplate string
	Name        string
	Description string
	MimeType    string
}

type MCPPromptCaller func(ctx context.Context, server, name string, arguments map[string]any) (MCPPromptResult, error)
type MCPResourceLister func(ctx context.Context, server string) ([]MCPResource, error)

type MCPToolCallRequest struct {
	Server            string
	Name              string
	Input             map[string]any
	ToolUseID         string
	Meta              map[string]any
	Timeout           time.Duration
	ReportProgress    ProgressFunc
	HandleElicitation ElicitationFunc
}

type MCPContextualToolCaller func(context.Context, MCPToolCallRequest) (MCPToolResult, error)

type MCPAuthStartResult struct {
	Status              string
	AuthURL             string
	Message             string
	Scope               string
	ResourceMetadataURL string
	Challenge           map[string]string
	Completion          <-chan MCPAuthCompletionResult
}

type MCPAuthCompletionResult struct {
	Status  string
	Message string
	Error   error
}

type MCPAuthenticator func(context.Context, string, MCPConnection) (MCPAuthStartResult, error)

type MCPReconnectResult struct {
	Client    MCPConnection
	Tools     MCPToolsListResult
	Prompts   MCPPromptsListResult
	Skills    []SkillCommand
	Resources []MCPResource
}

type MCPReconnectFunc func(context.Context, string) (MCPReconnectResult, error)

type ToolDecision struct {
	Source             string
	Decision           string
	TimestampUnixMilli int64
}

type PromptRequest struct {
	Message string
	Options []string
}

type PromptResponse struct {
	Value     string
	Cancelled bool
}

type RequestPromptFunc func(sourceName, toolInputSummary string, request PromptRequest) (PromptResponse, error)

type ToolProgress struct {
	ToolUseID string
	Type      string
	Message   string
	Data      map[string]any
}

type ProgressFunc func(ToolProgress)

type Command struct {
	Type                        string
	Name                        string
	Description                 string
	Source                      string
	LoadedFrom                  string
	HasUserSpecifiedDescription bool
	WhenToUse                   string
	DisableModelInvocation      bool
	UserInvocable               bool
	IsHidden                    bool
}

type AgentDefinitions struct {
	ActiveAgents      []string
	AllowedAgentTypes []string
}

type QueryTracking struct {
	ChainID string
	Depth   int
}

type Notification struct {
	Key      string
	Priority string
	Message  string
	Data     map[string]any
}

type AddNotificationFunc func(Notification)
type RefreshToolsFunc func() []Definition

type ElicitationRequest struct {
	ServerName string
	Params     map[string]any
}

type ElicitationResult struct {
	Value     string
	Cancelled bool
	Data      map[string]any
}

type ElicitationFunc func(context.Context, ElicitationRequest) (ElicitationResult, error)
type SetConversationIDFunc func(string)

type CanUseToolRequest struct {
	ToolName          string
	Input             string
	InputObject       map[string]any
	ToolUseID         string
	ProviderMessageID string
	ForceDecision     *permissions.Decision
}

type CanUseToolFunc func(context.Context, CanUseToolRequest) (permissions.Decision, error)

type ToolUseContext struct {
	AbortContext            context.Context
	Session                 session.Session
	ToolName                string
	ToolUseID               string
	Input                   string
	InputObject             map[string]any
	Policy                  permissions.Policy
	AvailableTools          []Definition
	AgentID                 string
	AgentType               string
	MainLoopModel           string
	LLMProvider             string
	Commands                []Command
	QuerySource             string
	CustomSystemPrompt      string
	AppendSystemPrompt      string
	Debug                   bool
	Verbose                 bool
	ThinkingConfig          map[string]any
	AgentDefinitions        AgentDefinitions
	MaxBudgetUSD            float64
	IsNonInteractive        bool
	RequireCanUseTool       bool
	QueryTracking           QueryTracking
	ReadFileState           map[string]any
	ContentReplacementState map[string]any
	CriticalSystemReminder  string
	PreserveToolUseResults  bool
	RenderedSystemPrompt    string
	Messages                []session.Message
	AppState                map[string]any
	SetAppState             func(func(map[string]any) map[string]any)
	ToolDecisions           map[string]ToolDecision
	FileReadingLimits       ResourceLimits
	GlobLimits              ResourceLimits
	MCPClients              []MCPConnection
	MCPResources            map[string][]MCPResource
	MCPResourceReader       MCPResourceReader
	MCPResourceLister       MCPResourceLister
	MCPContextualToolCaller MCPContextualToolCaller
	MCPOAuthStore           MCPOAuthStore
	MCPAuthenticator        MCPAuthenticator
	MCPReconnect            MCPReconnectFunc
	RequestPrompt           RequestPromptFunc
	ReportProgress          ProgressFunc
	AddNotification         AddNotificationFunc
	RefreshTools            RefreshToolsFunc
	HandleElicitation       ElicitationFunc
	SetConversationID       SetConversationIDFunc
	CanUseTool              CanUseToolFunc
	ContextModifier         func(ToolUseContext) ToolUseContext
}

func (c ToolUseContext) Normalized() ToolUseContext {
	out := c
	if out.AbortContext == nil {
		out.AbortContext = context.Background()
	}
	if out.AgentID == "" {
		out.AgentID = out.Session.AgentID
	}
	if out.InputObject == nil {
		if inputObject, ok := parseObjectInput(out.Input); ok {
			out.InputObject = inputObject
		}
	} else {
		out.InputObject = cloneAnyMap(out.InputObject)
	}
	out.AvailableTools = append([]Definition(nil), out.AvailableTools...)
	out.Commands = append([]Command(nil), out.Commands...)
	out.ThinkingConfig = cloneAnyMap(out.ThinkingConfig)
	out.AgentDefinitions.ActiveAgents = append([]string(nil), out.AgentDefinitions.ActiveAgents...)
	out.AgentDefinitions.AllowedAgentTypes = append([]string(nil), out.AgentDefinitions.AllowedAgentTypes...)
	out.ReadFileState = cloneAnyMap(out.ReadFileState)
	out.ContentReplacementState = cloneAnyMap(out.ContentReplacementState)
	out.Messages = append([]session.Message(nil), out.Messages...)
	out.AppState = cloneAnyMap(out.AppState)
	out.ToolDecisions = cloneToolDecisions(out.ToolDecisions)
	out.MCPClients = append([]MCPConnection(nil), out.MCPClients...)
	out.MCPResources = cloneMCPResources(out.MCPResources)
	if out.SetAppState == nil {
		out.SetAppState = func(func(map[string]any) map[string]any) {}
	}
	if out.RequestPrompt == nil {
		out.RequestPrompt = func(string, string, PromptRequest) (PromptResponse, error) {
			return PromptResponse{}, nil
		}
	}
	if out.ReportProgress == nil {
		out.ReportProgress = func(ToolProgress) {}
	}
	if out.AddNotification == nil {
		out.AddNotification = func(Notification) {}
	}
	if out.RefreshTools == nil {
		out.RefreshTools = func() []Definition {
			return append([]Definition(nil), out.AvailableTools...)
		}
	}
	if out.HandleElicitation == nil {
		out.HandleElicitation = func(context.Context, ElicitationRequest) (ElicitationResult, error) {
			return ElicitationResult{}, nil
		}
	}
	if out.SetConversationID == nil {
		out.SetConversationID = func(string) {}
	}
	if out.CanUseTool == nil {
		out.CanUseTool = func(context.Context, CanUseToolRequest) (permissions.Decision, error) {
			return permissions.Decision{Allowed: true}, nil
		}
	}
	return out
}

type ContextualPermissionCheckingTool interface {
	CheckPermissionsWithContext(context.Context, ToolUseContext) (permissions.Decision, error)
}

type PermissionCheckingTool interface {
	CheckPermissions(context.Context, session.Session, string, permissions.Policy) (permissions.Decision, error)
}

type StructuredPermissionCheckingTool interface {
	CheckPermissionsWithInput(context.Context, session.Session, map[string]any, permissions.Policy) (permissions.Decision, error)
}

type ObservableInputBackfillingTool interface {
	BackfillObservableInput(map[string]any)
}

type Registry struct {
	tools   map[string]Tool
	aliases map[string]string
	order   []string
}

type AssembleOptions struct {
	Policy permissions.Policy
}

type ExposeOptions struct {
	IncludeDeferred bool
	Policy          permissions.Policy
}

type SearchOptions struct {
	Policy       permissions.Policy
	DeferredOnly bool
	MaxResults   int
}

func NewRegistry(toolList ...Tool) *Registry {
	registry := &Registry{
		tools:   make(map[string]Tool),
		aliases: make(map[string]string),
	}
	for _, tool := range toolList {
		registry.Register(tool)
	}
	return registry
}

func (r *Registry) Register(tool Tool) {
	def := normalizeDefinition(tool.Definition())
	if _, exists := r.tools[def.Name]; !exists {
		r.order = append(r.order, def.Name)
	}
	r.tools[def.Name] = tool
	for alias, canonical := range r.aliases {
		if canonical == def.Name {
			delete(r.aliases, alias)
		}
	}
	for _, alias := range def.Aliases {
		r.aliases[alias] = def.Name
	}
}

func (r *Registry) Unregister(name string) {
	name = strings.TrimSpace(name)
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}
	if _, exists := r.tools[name]; !exists {
		return
	}
	delete(r.tools, name)
	for alias, canonical := range r.aliases {
		if alias == name || canonical == name {
			delete(r.aliases, alias)
		}
	}
	filtered := r.order[:0]
	for _, existing := range r.order {
		if existing != name {
			filtered = append(filtered, existing)
		}
	}
	r.order = filtered
}

func (r *Registry) Definitions() []Definition {
	defs := make([]Definition, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		defs = append(defs, normalizeDefinition(toolMetadata(tool)))
	}
	return defs
}

func (r *Registry) Assemble(opts AssembleOptions) []Definition {
	defs := r.Expose(ExposeOptions{
		IncludeDeferred: true,
		Policy:          opts.Policy,
	})
	out := make([]Definition, 0, len(defs))
	return append(out, defs...)
}

func (r *Registry) Expose(opts ExposeOptions) []Definition {
	defs := r.Definitions()
	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		if !def.Enabled {
			continue
		}
		if isBlanketDenied(def.Name, opts.Policy.Rules) {
			continue
		}
		if def.ShouldDefer && !def.AlwaysLoad && !opts.IncludeDeferred {
			continue
		}
		out = append(out, def)
	}
	return out
}

func (r *Registry) Invoke(ctx context.Context, sess session.Session, name, input string) (string, error) {
	tool, ok := r.resolve(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	if structured, ok := tool.(StructuredTool); ok {
		if inputObject, ok := parseObjectInput(input); ok {
			return structured.InvokeWithInput(ctx, sess, inputObject)
		}
	}
	return tool.Invoke(ctx, sess, input)
}

func (r *Registry) InvokeWithPolicy(ctx context.Context, sess session.Session, name, input string, policy permissions.Policy) (string, error) {
	result, err := r.InvokeWithContext(ctx, ToolUseContext{
		Session:  sess,
		ToolName: name,
		Input:    input,
		Policy:   policy,
	})
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (r *Registry) InvokeWithContext(ctx context.Context, toolCtx ToolUseContext) (ToolResult, error) {
	toolCtx = toolCtx.Normalized()
	name := toolCtx.ToolName
	input := toolCtx.Input
	policy := toolCtx.Policy
	sess := toolCtx.Session
	tool, def, ok := r.resolveAvailable(name, input, policy)
	if !ok {
		return ToolResult{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(name))
	}
	if !def.Enabled {
		return ToolResult{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(name))
	}
	if len(toolCtx.AvailableTools) == 0 {
		toolCtx.AvailableTools = r.Expose(ExposeOptions{
			IncludeDeferred: true,
			Policy:          policy,
		})
	}
	if contextual, ok := tool.(ContextualTool); ok {
		return contextual.InvokeWithContext(ctx, toolCtx)
	}
	if structuredPolicyAware, ok := tool.(StructuredPolicyAwareTool); ok {
		if inputObject, ok := parseObjectInput(input); ok {
			output, err := structuredPolicyAware.InvokeWithInputAndPolicy(ctx, sess, inputObject, policy)
			return ToolResult{Output: output}, err
		}
	}
	if structured, ok := tool.(StructuredTool); ok {
		if inputObject, ok := parseObjectInput(input); ok {
			output, err := structured.InvokeWithInput(ctx, sess, inputObject)
			return ToolResult{Output: output}, err
		}
	}
	if policyAware, ok := tool.(PolicyAwareTool); ok {
		output, err := policyAware.InvokeWithPolicy(ctx, sess, input, policy)
		return ToolResult{Output: output}, err
	}
	output, err := tool.Invoke(ctx, sess, input)
	return ToolResult{Output: output}, err
}

func (r *Registry) CheckPermissions(ctx context.Context, sess session.Session, name, input string, policy permissions.Policy) (permissions.Decision, bool, error) {
	return r.CheckPermissionsWithContext(ctx, ToolUseContext{
		Session:  sess,
		ToolName: name,
		Input:    input,
		Policy:   policy,
	})
}

func (r *Registry) CheckPermissionsWithContext(ctx context.Context, toolCtx ToolUseContext) (permissions.Decision, bool, error) {
	toolCtx = toolCtx.Normalized()
	name := toolCtx.ToolName
	input := toolCtx.Input
	policy := toolCtx.Policy
	sess := toolCtx.Session
	tool, _, ok := r.resolveAvailable(name, input, policy)
	if !ok {
		return permissions.Decision{}, false, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(name))
	}
	if len(toolCtx.AvailableTools) == 0 {
		toolCtx.AvailableTools = r.Expose(ExposeOptions{
			IncludeDeferred: true,
			Policy:          policy,
		})
	}
	if contextualChecker, ok := tool.(ContextualPermissionCheckingTool); ok {
		decision, err := contextualChecker.CheckPermissionsWithContext(ctx, toolCtx)
		if err != nil {
			return permissions.Decision{}, true, err
		}
		return decision, true, nil
	}
	if structuredChecker, ok := tool.(StructuredPermissionCheckingTool); ok {
		if inputObject, ok := parseObjectInput(input); ok {
			decision, err := structuredChecker.CheckPermissionsWithInput(ctx, sess, inputObject, policy)
			if err != nil {
				return permissions.Decision{}, true, err
			}
			return decision, true, nil
		}
	}
	if checker, ok := tool.(PermissionCheckingTool); ok {
		decision, err := checker.CheckPermissions(ctx, sess, input, policy)
		if err != nil {
			return permissions.Decision{}, true, err
		}
		return decision, true, nil
	}
	return permissions.Decision{}, false, nil
}

func (r *Registry) Inspect(name, input string) (Definition, bool) {
	tool, ok := r.resolve(name)
	if !ok {
		return Definition{}, false
	}
	def := normalizeDefinition(tool.Definition())
	if promptText := strings.TrimSpace(tool.PromptDescription()); promptText != "" {
		def.Description = promptText
	}
	if searchHint := strings.TrimSpace(tool.SearchHint()); searchHint != "" {
		def.SearchHint = searchHint
	}
	def.Enabled = tool.IsEnabled()
	def.ReadOnly = tool.IsReadOnly(input)
	def.Destructive = tool.IsDestructive(input)
	def.ShouldDefer = tool.ShouldDefer()
	def.AlwaysLoad = tool.AlwaysLoad()
	return def, true
}

func (r *Registry) InspectWithPolicy(name, input string, policy permissions.Policy) (Definition, bool) {
	_, def, ok := r.resolveAvailable(name, input, policy)
	if !ok {
		return Definition{}, false
	}
	return def, true
}

func (r *Registry) BackfillObservableInput(name string, input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := cloneAnyMap(input)
	tool, ok := r.resolve(name)
	if !ok {
		return out
	}
	backfiller, ok := tool.(ObservableInputBackfillingTool)
	if !ok {
		return out
	}
	backfilled := cloneAnyMap(input)
	backfiller.BackfillObservableInput(backfilled)
	for key, value := range backfilled {
		if _, exists := input[key]; !exists {
			out[key] = value
		}
	}
	return out
}

func (r *Registry) AutoClassifierInput(name, input string) (any, bool) {
	tool, ok := r.resolve(name)
	if !ok {
		return nil, false
	}
	classifier, ok := tool.(AutoClassifyingTool)
	if !ok {
		return input, false
	}
	return classifier.ToAutoClassifierInput(input), true
}

func (r *Registry) Search(query string, opts SearchOptions) []Definition {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	if strings.HasPrefix(query, "select:") {
		selected := strings.TrimSpace(strings.TrimPrefix(query, "select:"))
		if selected == "" {
			return nil
		}
		requested := strings.Split(selected, ",")
		found := make([]Definition, 0, len(requested))
		seen := make(map[string]struct{}, len(requested))
		for _, item := range requested {
			toolName := strings.TrimSpace(item)
			if toolName == "" {
				continue
			}
			if def, ok := r.findSelectedTool(toolName, opts.Policy, true); ok {
				if _, exists := seen[def.Name]; !exists {
					seen[def.Name] = struct{}{}
					found = append(found, def)
				}
				continue
			}
			if def, ok := r.findSelectedTool(toolName, opts.Policy, false); ok {
				if _, exists := seen[def.Name]; !exists {
					seen[def.Name] = struct{}{}
					found = append(found, def)
				}
			}
		}
		if len(found) == 0 {
			return nil
		}
		return found
	}
	if def, ok := r.findSelectedTool(query, opts.Policy, true); ok {
		return []Definition{def}
	}
	if def, ok := r.findSelectedTool(query, opts.Policy, false); ok {
		return []Definition{def}
	}
	queryTerms := strings.Fields(query)
	requiredTerms := make([]string, 0, len(queryTerms))
	optionalTerms := make([]string, 0, len(queryTerms))
	for _, term := range queryTerms {
		if strings.HasPrefix(term, "+") && len(term) > 1 {
			requiredTerms = append(requiredTerms, strings.TrimPrefix(term, "+"))
			continue
		}
		optionalTerms = append(optionalTerms, term)
	}
	scoringTerms := queryTerms
	if len(requiredTerms) > 0 {
		scoringTerms = append(append([]string{}, requiredTerms...), optionalTerms...)
	}

	type scored struct {
		def   Definition
		score int
		index int
	}

	var matches []scored
	for idx, name := range r.order {
		tool := r.tools[name]
		def := normalizeDefinition(toolMetadata(tool))
		if !def.Enabled {
			continue
		}
		if isBlanketDenied(def.Name, opts.Policy.Rules) {
			continue
		}
		if opts.DeferredOnly && !def.ShouldDefer {
			continue
		}
		if !matchesRequiredTerms(def, requiredTerms) {
			continue
		}
		score := searchScore(scoringTerms, def)
		if score == 0 {
			continue
		}
		matches = append(matches, scored{def: def, score: score, index: idx})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].index < matches[j].index
	})

	out := make([]Definition, 0, len(matches))
	for _, item := range matches {
		out = append(out, item.def)
		if opts.MaxResults > 0 && len(out) >= opts.MaxResults {
			break
		}
	}
	return out
}

func (r *Registry) findSelectedTool(name string, policy permissions.Policy, deferredOnly bool) (Definition, bool) {
	for _, toolName := range r.order {
		tool := r.tools[toolName]
		def := normalizeDefinition(toolMetadata(tool))
		if !def.Enabled {
			continue
		}
		if isBlanketDenied(def.Name, policy.Rules) {
			continue
		}
		if deferredOnly && !def.ShouldDefer {
			continue
		}
		if strings.EqualFold(def.Name, name) {
			return def, true
		}
	}
	return Definition{}, false
}

func (r *Registry) resolve(name string) (Tool, bool) {
	name = strings.TrimSpace(name)
	if tool, ok := r.tools[name]; ok {
		return tool, true
	}
	if canonical, ok := r.aliases[name]; ok {
		tool, ok := r.tools[canonical]
		return tool, ok
	}
	return nil, false
}

func (r *Registry) resolveAvailable(name, input string, policy permissions.Policy) (Tool, Definition, bool) {
	tool, ok := r.resolve(name)
	if !ok {
		return nil, Definition{}, false
	}
	def := normalizeDefinition(tool.Definition())
	if promptText := strings.TrimSpace(tool.PromptDescription()); promptText != "" {
		def.Description = promptText
	}
	if searchHint := strings.TrimSpace(tool.SearchHint()); searchHint != "" {
		def.SearchHint = searchHint
	}
	def.Enabled = tool.IsEnabled()
	def.ReadOnly = tool.IsReadOnly(input)
	def.Destructive = tool.IsDestructive(input)
	def.ShouldDefer = tool.ShouldDefer()
	def.AlwaysLoad = tool.AlwaysLoad()
	if !def.Enabled {
		return nil, Definition{}, false
	}
	if isBlanketDenied(def.Name, policy.Rules) {
		return nil, Definition{}, false
	}
	return tool, def, true
}

func normalizeDefinition(def Definition) Definition {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	def.Source = strings.TrimSpace(def.Source)
	def.SearchHint = strings.TrimSpace(def.SearchHint)
	def.InputSchema = cloneAnyMap(def.InputSchema)
	if def.Source == "" {
		def.Source = "builtin"
	}
	aliases := make([]string, 0, len(def.Aliases))
	for _, alias := range def.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == def.Name {
			continue
		}
		aliases = append(aliases, alias)
	}
	def.Aliases = aliases
	return def
}

func toolMetadata(tool Tool) Definition {
	def := tool.Definition()
	if promptText := strings.TrimSpace(tool.PromptDescription()); promptText != "" {
		def.Description = promptText
	}
	if searchHint := strings.TrimSpace(tool.SearchHint()); searchHint != "" {
		def.SearchHint = searchHint
	}
	def.Enabled = tool.IsEnabled()
	def.ReadOnly = tool.IsReadOnly("")
	def.Destructive = tool.IsDestructive("")
	def.ShouldDefer = tool.ShouldDefer()
	def.AlwaysLoad = tool.AlwaysLoad()
	return def
}

func isBlanketDenied(toolName string, rules []permissions.Rule) bool {
	for _, rule := range rules {
		if !permissions.ToolNameMatchesRule(rule.ToolName, toolName) {
			continue
		}
		if rule.Action != permissions.ActionDeny {
			continue
		}
		if len(rule.Match.CommandContains) > 0 || len(rule.Match.WorkDirPrefixes) > 0 {
			continue
		}
		return true
	}
	return false
}

func parseObjectInput(input string) (map[string]any, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, false
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		return nil, false
	}
	if parsed == nil {
		return nil, false
	}
	return parsed, true
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func deepCloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = deepCloneAny(value)
	}
	return cloned
}

func deepCloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = deepCloneAny(item)
		}
		return out
	default:
		return typed
	}
}

func cloneToolDecisions(input map[string]ToolDecision) map[string]ToolDecision {
	if input == nil {
		return nil
	}
	cloned := make(map[string]ToolDecision, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneMCPResources(input map[string][]MCPResource) map[string][]MCPResource {
	if input == nil {
		return nil
	}
	cloned := make(map[string][]MCPResource, len(input))
	for key, value := range input {
		cloned[key] = append([]MCPResource(nil), value...)
	}
	return cloned
}

func searchScore(terms []string, def Definition) int {
	score := 0
	for _, term := range terms {
		if term == "" || strings.HasPrefix(term, "+") {
			continue
		}
		if strings.Contains(strings.ToLower(def.Name), term) {
			score += 10
		}
		for _, alias := range def.Aliases {
			if strings.Contains(strings.ToLower(alias), term) {
				score += 8
				break
			}
		}
		if strings.Contains(strings.ToLower(def.Description), term) {
			score += 5
		}
		if strings.Contains(strings.ToLower(def.SearchHint), term) {
			score += 7
		}
	}
	return score
}

func matchesRequiredTerms(def Definition, requiredTerms []string) bool {
	for _, term := range requiredTerms {
		if term == "" {
			continue
		}
		if strings.Contains(strings.ToLower(def.Name), term) {
			continue
		}
		matchedAlias := false
		for _, alias := range def.Aliases {
			if strings.Contains(strings.ToLower(alias), term) {
				matchedAlias = true
				break
			}
		}
		if matchedAlias {
			continue
		}
		if strings.Contains(strings.ToLower(def.Description), term) {
			continue
		}
		if strings.Contains(strings.ToLower(def.SearchHint), term) {
			continue
		}
		return false
	}
	return true
}
