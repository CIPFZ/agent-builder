package queryengine

import (
	"context"
	"myclaw/internal/model"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"strings"
)

func toolResultIdentity(message *session.Message) (string, bool) {
	if message == nil {
		return "", false
	}
	for _, block := range message.Blocks {
		if block.Type == model.MessageBlockToolResult {
			return block.ToolUseID, block.IsError
		}
	}
	return "", false
}

type PermissionHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Decision          permissions.Decision
	Policy            permissions.Policy
}

type PermissionHook interface {
	CheckPermission(context.Context, PermissionHookRequest) (permissions.Decision, bool, error)
}

type PreToolUseHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Policy            permissions.Policy
}

type PreToolUseHookResult struct {
	UpdatedInput          string
	UpdatedInputObject    map[string]any
	HasPermissionDecision bool
	PermissionDecision    permissions.Decision
	BlockingError         string
	PreventContinuation   bool
	StopReason            string
	AdditionalContexts    []string
	HookMessages          []map[string]any
	Cancelled             bool
	ExecutionError        string
}

func (r PreToolUseHookResult) UpdatedInputValue() (string, bool, error) {
	return permissions.Decision{
		UpdatedInput:       r.UpdatedInput,
		UpdatedInputObject: r.UpdatedInputObject,
	}.UpdatedInputValue()
}

type PreToolUseHook interface {
	BeforeToolUse(context.Context, PreToolUseHookRequest) (PreToolUseHookResult, bool, error)
}

type PostToolUseHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	ToolOutput        string
	Policy            permissions.Policy
}

type PostToolUseHookResult struct {
	BlockingError        string
	PreventContinuation  bool
	StopReason           string
	AdditionalContexts   []string
	UpdatedMCPToolOutput string
	HookMessages         []map[string]any
	Cancelled            bool
	ExecutionError       string
}

type PostToolUseHook interface {
	AfterToolUse(context.Context, PostToolUseHookRequest) (PostToolUseHookResult, bool, error)
}

type PostToolUseFailureHookRequest struct {
	Session           session.Session
	RunID             string
	ToolName          string
	ToolInput         string
	ToolInputObject   map[string]any
	ToolUseID         string
	ProviderMessageID string
	Error             string
	IsInterrupt       bool
	Policy            permissions.Policy
}

type PostToolUseFailureHookResult struct {
	BlockingError      string
	AdditionalContexts []string
	HookMessages       []map[string]any
	Cancelled          bool
	ExecutionError     string
}

type PostToolUseFailureHook interface {
	AfterToolUseFailure(context.Context, PostToolUseFailureHookRequest) (PostToolUseFailureHookResult, bool, error)
}

type PermissionUpdatePersister interface {
	PersistPermissionUpdates(context.Context, session.Session, []permissions.PermissionUpdate) error
}

func postToolUseHookBlocks(toolName, toolUseID string, result PostToolUseHookResult) []model.MessageBlock {
	blocks := make([]model.MessageBlock, 0, len(result.HookMessages)+5)
	hookName := "PostToolUse:" + toolName
	for _, message := range result.HookMessages {
		if message == nil {
			continue
		}
		if message["type"] == "hook_blocking_error" {
			continue
		}
		blocks = append(blocks, hookMessageBlock(message, hookName, toolUseID, "PostToolUse"))
	}
	if blockingError := strings.TrimSpace(result.BlockingError); blockingError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_blocking_error"),
			Raw: map[string]any{
				"type":          "hook_blocking_error",
				"hookName":      hookName,
				"toolUseID":     toolUseID,
				"hookEvent":     "PostToolUse",
				"blockingError": blockingError,
			},
		})
	}
	if result.Cancelled {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_cancelled"),
			Raw: map[string]any{
				"type":      "hook_cancelled",
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	if len(result.AdditionalContexts) > 0 {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_additional_context"),
			Raw: map[string]any{
				"type":      "hook_additional_context",
				"content":   append([]string(nil), result.AdditionalContexts...),
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	if executionError := strings.TrimSpace(result.ExecutionError); executionError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_error_during_execution"),
			Raw: map[string]any{
				"type":      "hook_error_during_execution",
				"content":   executionError,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	if result.PreventContinuation {
		stopReason := strings.TrimSpace(result.StopReason)
		if stopReason == "" {
			stopReason = "Execution stopped by PostToolUse hook"
		}
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_stopped_continuation"),
			Raw: map[string]any{
				"type":      "hook_stopped_continuation",
				"message":   stopReason,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUse",
			},
		})
	}
	return blocks
}

func preToolUseHookBlocks(toolName, toolUseID string, result PreToolUseHookResult) []model.MessageBlock {
	blocks := make([]model.MessageBlock, 0, len(result.HookMessages)+4)
	hookName := "PreToolUse:" + toolName
	for _, message := range result.HookMessages {
		if message == nil {
			continue
		}
		blocks = append(blocks, hookMessageBlock(message, hookName, toolUseID, "PreToolUse"))
	}
	if len(result.AdditionalContexts) > 0 {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_additional_context"),
			Raw: map[string]any{
				"type":      "hook_additional_context",
				"content":   append([]string(nil), result.AdditionalContexts...),
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	if result.Cancelled {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_cancelled"),
			Raw: map[string]any{
				"type":      "hook_cancelled",
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	if executionError := strings.TrimSpace(result.ExecutionError); executionError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_error_during_execution"),
			Raw: map[string]any{
				"type":      "hook_error_during_execution",
				"content":   executionError,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	if result.PreventContinuation {
		stopReason := strings.TrimSpace(result.StopReason)
		if stopReason == "" {
			stopReason = "Execution stopped by hook"
		}
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_stopped_continuation"),
			Raw: map[string]any{
				"type":      "hook_stopped_continuation",
				"message":   stopReason,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PreToolUse",
			},
		})
	}
	return blocks
}

func postToolUseFailureHookBlocks(toolName, toolUseID string, result PostToolUseFailureHookResult) []model.MessageBlock {
	blocks := make([]model.MessageBlock, 0, len(result.HookMessages)+4)
	hookName := "PostToolUseFailure:" + toolName
	for _, message := range result.HookMessages {
		if message == nil {
			continue
		}
		if message["type"] == "hook_blocking_error" {
			continue
		}
		blocks = append(blocks, hookMessageBlock(message, hookName, toolUseID, "PostToolUseFailure"))
	}
	if blockingError := strings.TrimSpace(result.BlockingError); blockingError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_blocking_error"),
			Raw: map[string]any{
				"type":          "hook_blocking_error",
				"hookName":      hookName,
				"toolUseID":     toolUseID,
				"hookEvent":     "PostToolUseFailure",
				"blockingError": blockingError,
			},
		})
	}
	if len(result.AdditionalContexts) > 0 {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_additional_context"),
			Raw: map[string]any{
				"type":      "hook_additional_context",
				"content":   append([]string(nil), result.AdditionalContexts...),
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUseFailure",
			},
		})
	}
	if result.Cancelled {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_cancelled"),
			Raw: map[string]any{
				"type":      "hook_cancelled",
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUseFailure",
			},
		})
	}
	if executionError := strings.TrimSpace(result.ExecutionError); executionError != "" {
		blocks = append(blocks, model.MessageBlock{
			Type: model.MessageBlockType("hook_error_during_execution"),
			Raw: map[string]any{
				"type":      "hook_error_during_execution",
				"content":   executionError,
				"hookName":  hookName,
				"toolUseID": toolUseID,
				"hookEvent": "PostToolUseFailure",
			},
		})
	}
	return blocks
}

func hookMessageBlock(message map[string]any, hookName, toolUseID, hookEvent string) model.MessageBlock {
	raw := cloneAnyMap(message)
	if _, ok := raw["type"]; !ok {
		raw["type"] = "hook_message"
	}
	if _, ok := raw["hookName"]; !ok {
		raw["hookName"] = hookName
	}
	if _, ok := raw["toolUseID"]; !ok {
		raw["toolUseID"] = toolUseID
	}
	if _, ok := raw["hookEvent"]; !ok {
		raw["hookEvent"] = hookEvent
	}
	blockType, _ := raw["type"].(string)
	return model.MessageBlock{
		Type: model.MessageBlockType(blockType),
		Raw:  raw,
	}
}
