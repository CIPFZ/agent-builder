package main

type RuntimeStatus struct {
	Ready       bool              `json:"ready"`
	WorkspaceID string            `json:"workspaceId"`
	SessionID   string            `json:"sessionId"`
	WorkingDir  string            `json:"workingDir"`
	Model       string            `json:"model"`
	Provider    string            `json:"provider"`
	Busy        bool              `json:"busy"`
	Usage       RuntimeUsage      `json:"usage"`
	Events      RuntimeEventStats `json:"events"`
	Requests    RuntimeRequests   `json:"requests"`
}

type RuntimeModel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Selected bool   `json:"selected"`
}

type RuntimeModelsResponse struct {
	Models []RuntimeModel `json:"models"`
}

type RuntimeConfigResponse struct {
	Config RuntimeModelConfig `json:"config"`
}

type RuntimeChatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"sessionId,omitempty"`
}

type RuntimeChatResponse struct {
	RequestID string        `json:"requestId"`
	TurnID    string        `json:"turnId"`
	Status    RuntimeStatus `json:"status"`
}

type RuntimeTurn struct {
	ID              string         `json:"id"`
	SessionID       string         `json:"sessionId"`
	Status          string         `json:"status"`
	StartedAt       int64          `json:"startedAt"`
	FinishedAt      int64          `json:"finishedAt,omitempty"`
	DurationMS      int64          `json:"durationMs,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	Model           string         `json:"model,omitempty"`
	PromptPreview   string         `json:"promptPreview,omitempty"`
	UsageBefore     RuntimeUsage   `json:"usageBefore,omitempty"`
	UsageAfter      RuntimeUsage   `json:"usageAfter,omitempty"`
	UsageDelta      RuntimeUsage   `json:"usageDelta,omitempty"`
	LatestMessageID string         `json:"latestMessageId,omitempty"`
	LatestAssistant RuntimeMessage `json:"latestAssistant,omitempty"`
	Error           string         `json:"error,omitempty"`
}

type RuntimeTurnResponse struct {
	Turn RuntimeTurn `json:"turn"`
}

type RuntimeMessage struct {
	ID           string               `json:"id"`
	SessionID    string               `json:"sessionId"`
	Role         string               `json:"role"`
	Content      string               `json:"content"`
	Parts        []RuntimeMessagePart `json:"parts,omitempty"`
	Provider     string               `json:"provider,omitempty"`
	Model        string               `json:"model,omitempty"`
	CreatedAt    int64                `json:"createdAt"`
	UpdatedAt    int64                `json:"updatedAt"`
	Finished     bool                 `json:"finished"`
	FinishReason string               `json:"finishReason,omitempty"`
	Error        string               `json:"error,omitempty"`
}

type RuntimeMessagePart struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Thinking   string `json:"thinking,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	Name       string `json:"name,omitempty"`
	Input      string `json:"input,omitempty"`
	Finished   bool   `json:"finished,omitempty"`
	Content    string `json:"content,omitempty"`
	Data       string `json:"data,omitempty"`
	MIMEType   string `json:"mimeType,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
	IsError    bool   `json:"isError,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Message    string `json:"message,omitempty"`
	Details    string `json:"details,omitempty"`
}

type RuntimeSession struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	MessageCount     int64        `json:"messageCount"`
	PromptTokens     int64        `json:"promptTokens"`
	CompletionTokens int64        `json:"completionTokens"`
	Cost             float64      `json:"cost"`
	CreatedAt        int64        `json:"createdAt"`
	UpdatedAt        int64        `json:"updatedAt"`
	Active           bool         `json:"active"`
	Usage            RuntimeUsage `json:"usage"`
}

type RuntimeSessionsResponse struct {
	Sessions []RuntimeSession `json:"sessions"`
}

type RuntimeSessionResponse struct {
	Session RuntimeSession `json:"session"`
}

type RuntimeSessionUpdateRequest struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
}

type RuntimeMessagesResponse struct {
	Messages []RuntimeMessage `json:"messages"`
}

type RuntimePermissionRequest struct {
	ID          string `json:"id"`
	SessionID   string `json:"sessionId"`
	ToolCallID  string `json:"toolCallId"`
	ToolName    string `json:"toolName"`
	Description string `json:"description,omitempty"`
	Action      string `json:"action"`
	Params      any    `json:"params,omitempty"`
	Path        string `json:"path,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
}

type RuntimePermissionsResponse struct {
	Permissions []RuntimePermissionRequest `json:"permissions"`
}

type RuntimePermissionDecision struct {
	PermissionID string `json:"permissionId"`
	Action       string `json:"action"`
}

type RuntimeRequests struct {
	ActiveRequestID  string `json:"activeRequestId,omitempty"`
	ActiveStartedAt  int64  `json:"activeStartedAt,omitempty"`
	ActiveDurationMS int64  `json:"activeDurationMs,omitempty"`
	Running          int    `json:"running"`
}

type RuntimeUsage struct {
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	TotalTokens      int64   `json:"totalTokens"`
	Cost             float64 `json:"cost"`
}

type RuntimeEventStats struct {
	LastEventAt      int64 `json:"lastEventAt"`
	MessageEvents    int64 `json:"messageEvents"`
	SessionEvents    int64 `json:"sessionEvents"`
	OtherEvents      int64 `json:"otherEvents"`
	AssistantEvents  int64 `json:"assistantEvents"`
	PermissionEvents int64 `json:"permissionEvents"`
}

type RuntimeEventsResponse struct {
	Events []RuntimeEvent `json:"events"`
}

type RuntimeEventsEndpointResponse struct {
	URL string `json:"url"`
}

type RuntimeSkill struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Builtin       bool   `json:"builtin"`
	Enabled       bool   `json:"enabled"`
	Path          string `json:"path,omitempty"`
	SkillFilePath string `json:"skill_file_path,omitempty"`
	State         string `json:"state"`
	Error         string `json:"error,omitempty"`
}

type RuntimeSkillsResponse struct {
	Skills []RuntimeSkill `json:"skills"`
}

type RuntimeSkillCreateRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Directory    string `json:"directory,omitempty"`
}

type RuntimeSkillPathRequest struct {
	Path string `json:"path"`
}

type RuntimeSkillToggleRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPCounts struct {
	Tools     int `json:"tools"`
	Prompts   int `json:"prompts"`
	Resources int `json:"resources"`
}

type RuntimeMCPServer struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Disabled      bool              `json:"disabled"`
	State         string            `json:"state"`
	Counts        RuntimeMCPCounts  `json:"counts"`
	Error         string            `json:"error,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	EnabledTools  []string          `json:"enabled_tools,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
}

type RuntimeMCPServersResponse struct {
	Servers []RuntimeMCPServer `json:"servers"`
}

type RuntimeMCPServerConfigRequest struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Disabled      bool              `json:"disabled"`
	EnabledTools  []string          `json:"enabled_tools,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
}

type RuntimeMCPServerToggleRequest struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPTool struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	InputSchema any    `json:"input_schema,omitempty"`
}

type RuntimeMCPToolsResponse struct {
	Tools []RuntimeMCPTool `json:"tools"`
}

type RuntimeMCPToolToggleRequest struct {
	Server  string `json:"server"`
	Tool    string `json:"tool"`
	Enabled bool   `json:"enabled"`
}

type RuntimeMCPResource struct {
	Server      string `json:"server"`
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

type RuntimeMCPResourcesResponse struct {
	Resources []RuntimeMCPResource `json:"resources"`
}

type RuntimeMCPPrompt struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type RuntimeMCPPromptsResponse struct {
	Prompts []RuntimeMCPPrompt `json:"prompts"`
}

type RuntimeCapability struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Source      string `json:"source,omitempty"`
	Enabled     bool   `json:"enabled"`
	Risk        string `json:"risk"`
	Description string `json:"description,omitempty"`
}

type RuntimeCapabilitiesResponse struct {
	Capabilities []RuntimeCapability `json:"capabilities"`
}

type RuntimeModelConfig struct {
	Protocol   string   `json:"protocol"`
	URL        string   `json:"url"`
	APIKey     string   `json:"apiKey,omitempty"`
	Model      string   `json:"model"`
	Proxy      string   `json:"proxy,omitempty"`
	Models     []string `json:"models,omitempty"`
	HasAPIKey  bool     `json:"hasApiKey"`
	ConfigPath string   `json:"configPath,omitempty"`
}

type RuntimeModelVerifyResponse struct {
	OK       bool     `json:"ok"`
	Protocol string   `json:"protocol"`
	Model    string   `json:"model"`
	Models   []string `json:"models,omitempty"`
	Error    string   `json:"error,omitempty"`
}

type RuntimeModelDiscoveryResponse struct {
	Protocol string   `json:"protocol"`
	Model    string   `json:"model,omitempty"`
	Models   []string `json:"models"`
	Error    string   `json:"error,omitempty"`
}
