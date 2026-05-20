package main

type localModelConfigResult struct {
	Applied      bool
	Path         string
	CheckedPaths []string
	Config       RuntimeModelConfig
	Error        error
}

type desktopLayout struct {
	Root            string
	ConfigDir       string
	DataDir         string
	LogsDir         string
	ModelConfigPath string
}

type runtimeRequestState struct {
	SessionID       string       `json:"sessionId"`
	Provider        string       `json:"provider,omitempty"`
	Model           string       `json:"model,omitempty"`
	PromptPreview   string       `json:"promptPreview,omitempty"`
	StartedAt       int64        `json:"startedAt"`
	FinishedAt      int64        `json:"finishedAt,omitempty"`
	Status          string       `json:"status"`
	UsageBefore     RuntimeUsage `json:"usageBefore,omitempty"`
	UsageAfter      RuntimeUsage `json:"usageAfter,omitempty"`
	UsageDelta      RuntimeUsage `json:"usageDelta,omitempty"`
	LatestMessageID string       `json:"latestMessageId,omitempty"`
	Finished        bool         `json:"finished"`
	Cancelled       bool         `json:"cancelled,omitempty"`
	Error           string       `json:"error,omitempty"`
}

type runtimeToolEventState struct {
	Started   bool
	Completed bool
	Output    bool
}

type runtimeEventStats struct {
	lastEventAt      int64
	messageEvents    int64
	sessionEvents    int64
	otherEvents      int64
	assistantEvents  int64
	permissionEvents int64
}
