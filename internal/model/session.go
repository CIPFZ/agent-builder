package model

import "time"

type SessionMetadata struct {
	LastUserMessageID              string               `json:"last_user_message_id,omitempty"`
	LastAssistantMessageID         string               `json:"last_assistant_message_id,omitempty"`
	LastCompactBoundaryID          string               `json:"last_compact_boundary_id,omitempty"`
	LastCompactionSummaryID        string               `json:"last_compaction_summary_id,omitempty"`
	LastSummarizedMessageID        string               `json:"last_summarized_message_id,omitempty"`
	LastCompactionReason           string               `json:"last_compaction_reason,omitempty"`
	LastCompactedAt                time.Time            `json:"last_compacted_at,omitempty"`
	LastActivityAt                 time.Time            `json:"last_activity_at,omitempty"`
	InitialMainLoopModel           string               `json:"initial_main_loop_model,omitempty"`
	MainLoopModelOverride          string               `json:"main_loop_model_override,omitempty"`
	MainLoopEffortOverride         string               `json:"main_loop_effort_override,omitempty"`
	AgentType                      string               `json:"agent_type,omitempty"`
	AgentSystemPrompt              string               `json:"agent_system_prompt,omitempty"`
	AgentMemoryScope               string               `json:"agent_memory_scope,omitempty"`
	AgentMaxTurns                  int                  `json:"agent_max_turns,omitempty"`
	AgentIsolation                 string               `json:"agent_isolation,omitempty"`
	AgentCWD                       string               `json:"agent_cwd,omitempty"`
	AgentWorktreePath              string               `json:"agent_worktree_path,omitempty"`
	AgentWorktreeBranch            string               `json:"agent_worktree_branch,omitempty"`
	AgentWorktreeHeadCommit        string               `json:"agent_worktree_head_commit,omitempty"`
	AgentWorktreeGitRoot           string               `json:"agent_worktree_git_root,omitempty"`
	PendingApprovalID              string               `json:"pending_approval_id,omitempty"`
	PendingApprovalStatus          string               `json:"pending_approval_status,omitempty"`
	PendingApprovalToolName        string               `json:"pending_approval_tool_name,omitempty"`
	PendingApprovalToolInput       string               `json:"pending_approval_tool_input,omitempty"`
	PendingApprovalToolInputObject map[string]any       `json:"pending_approval_tool_input_object,omitempty"`
	PendingApprovalToolUseID       string               `json:"pending_approval_tool_use_id,omitempty"`
	PendingApprovalProviderMsgID   string               `json:"pending_approval_provider_message_id,omitempty"`
	PendingApprovalReason          string               `json:"pending_approval_reason,omitempty"`
	PendingApprovalDecisionReason  string               `json:"pending_approval_decision_reason,omitempty"`
	PendingApprovalAcceptFeedback  string               `json:"pending_approval_accept_feedback,omitempty"`
	PendingApprovalContentBlocks   []map[string]any     `json:"pending_approval_content_blocks,omitempty"`
	PendingApprovalRunID           string               `json:"pending_approval_run_id,omitempty"`
	PendingApprovalUserMessageID   string               `json:"pending_approval_user_message_id,omitempty"`
	PendingApprovalCategory        string               `json:"pending_approval_category,omitempty"`
	PendingApprovalRuleSource      string               `json:"pending_approval_rule_source,omitempty"`
	AgentRuns                      []AgentRunMetadata   `json:"agent_runs,omitempty"`
	ReadFiles                      []ReadFileMetadata   `json:"read_files,omitempty"`
	MemoryItems                    []MemoryMetadata     `json:"memory_items,omitempty"`
	ContextCache                   ContextCacheMetadata `json:"context_cache,omitempty"`
}

type Session struct {
	ID       string          `json:"id"`
	Key      string          `json:"key"`
	AgentID  string          `json:"agent_id"`
	IsMain   bool            `json:"is_main"`
	Metadata SessionMetadata `json:"metadata,omitempty"`
}

type AgentRunMetadata struct {
	ID                      string    `json:"id"`
	ParentSessionID         string    `json:"parent_session_id,omitempty"`
	ParentAgentID           string    `json:"parent_agent_id,omitempty"`
	ChildSessionID          string    `json:"child_session_id,omitempty"`
	ChildSessionKey         string    `json:"child_session_key,omitempty"`
	Label                   string    `json:"label,omitempty"`
	Prompt                  string    `json:"prompt,omitempty"`
	AllowedTools            []string  `json:"allowed_tools,omitempty"`
	Model                   string    `json:"model,omitempty"`
	Effort                  string    `json:"effort,omitempty"`
	RunInBackground         bool      `json:"run_in_background,omitempty"`
	Isolation               string    `json:"isolation,omitempty"`
	CWD                     string    `json:"cwd,omitempty"`
	RemoteIsolationBoundary string    `json:"remote_isolation_boundary,omitempty"`
	PermissionMode          string    `json:"permission_mode,omitempty"`
	PermissionInherited     bool      `json:"permission_inherited,omitempty"`
	ParentRunID             string    `json:"parent_run_id,omitempty"`
	ContinuationMode        string    `json:"continuation_mode,omitempty"`
	Status                  string    `json:"status,omitempty"`
	LastAction              string    `json:"last_action,omitempty"`
	Attempt                 int       `json:"attempt,omitempty"`
	Output                  string    `json:"output,omitempty"`
	OutputFile              string    `json:"output_file,omitempty"`
	ErrorSummary            string    `json:"error_summary,omitempty"`
	ControlMessages         []string  `json:"control_messages,omitempty"`
	CreatedAt               time.Time `json:"created_at,omitempty"`
	StartedAt               time.Time `json:"started_at,omitempty"`
	UpdatedAt               time.Time `json:"updated_at,omitempty"`
	CompletedAt             time.Time `json:"completed_at,omitempty"`
	LastActionAt            time.Time `json:"last_action_at,omitempty"`
}

type ReadFileMetadata struct {
	Path        string    `json:"path"`
	Hash        string    `json:"hash,omitempty"`
	Size        int64     `json:"size,omitempty"`
	ModTime     time.Time `json:"mod_time,omitempty"`
	LastReadAt  time.Time `json:"last_read_at,omitempty"`
	ToolUseID   string    `json:"tool_use_id,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
}

type MemoryMetadata struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type ContextCacheMetadata struct {
	Key            string    `json:"key,omitempty"`
	WorkspaceHash  string    `json:"workspace_hash,omitempty"`
	HistoryHash    string    `json:"history_hash,omitempty"`
	MemoryHash     string    `json:"memory_hash,omitempty"`
	LastRebuiltAt  time.Time `json:"last_rebuilt_at,omitempty"`
	LastCacheHitAt time.Time `json:"last_cache_hit_at,omitempty"`
}
