package runtime

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/config"
)

const (
	runtimeCoordinatorTaskRunnerReasonMissingExecutor     = "coordinator_task_runner_missing_executor"
	runtimeCoordinatorTaskRunnerReasonUnsupportedRole     = "coordinator_task_runner_unsupported_role"
	runtimeCoordinatorTaskRunnerReasonMissingPromptSource = "coordinator_task_runner_missing_prompt_source"
)

type runtimeStartedAgentTaskExecutor interface {
	ExecuteStartedAgentTask(context.Context, agent.StartedAgentTaskExecutionRequest) (agent.StartedAgentTaskExecutionResult, error)
}

type runtimeCoordinatorTaskRunner struct {
	service  *runtimeService
	executor runtimeStartedAgentTaskExecutor
}

func (r runtimeCoordinatorTaskRunner) ExecuteAgentTask(ctx context.Context, req RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error) {
	if r.executor == nil {
		return r.fail(ctx, req, runtimeCoordinatorTaskRunnerReasonMissingExecutor)
	}
	if strings.TrimSpace(req.Role) != config.AgentTask {
		return r.fail(ctx, req, runtimeCoordinatorTaskRunnerReasonUnsupportedRole+":"+strings.TrimSpace(req.Role))
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		var err error
		prompt, err = r.promptSource(ctx, req.TaskID)
		if err != nil {
			return r.fail(ctx, req, err.Error())
		}
	}
	result, err := r.executor.ExecuteStartedAgentTask(ctx, agent.StartedAgentTaskExecutionRequest{
		TaskID:                  req.TaskID,
		ParentSessionID:         req.ParentSessionID,
		ParentTurnID:            req.ParentTurnID,
		ParentToolCallID:        req.ParentToolCallID,
		ChildSessionID:          req.ChildSessionID,
		Title:                   req.Title,
		Kind:                    req.Kind,
		Role:                    req.Role,
		Name:                    req.Name,
		Prompt:                  prompt,
		PromptSummary:           req.PromptSummary,
		Provider:                req.Provider,
		Model:                   req.Model,
		AllowedTools:            append([]string(nil), req.AllowedTools...),
		CapabilityScope:         append([]string(nil), req.CapabilityScope...),
		CWD:                     req.CWD,
		Worktree:                req.Worktree,
		StartedAt:               req.StartedAt,
		StartAlreadyRecorded:    req.StartAlreadyRecorded,
		BackendOnly:             req.BackendOnly,
		EventPayloadRefreshOnly: req.EventPayloadRefreshOnly,
	})
	return RuntimeAgentTaskExecutionResult{
		TaskID:             firstNonEmpty(result.TaskID, req.TaskID),
		Status:             result.Status,
		Terminal:           result.Terminal,
		RefreshTargets:     runtimeRunSchedulerRefreshTargets(),
		ResultSummary:      result.ResultSummary,
		Error:              result.Error,
		NoStaleResume:      result.NoStaleResume,
		CompletionOnlyRefs: result.CompletionOnlyRefs,
	}, err
}

func (r runtimeCoordinatorTaskRunner) promptSource(ctx context.Context, taskID string) (string, error) {
	if r.service == nil || r.service.turns.db == nil {
		return "", errors.New(runtimeCoordinatorTaskRunnerReasonMissingPromptSource + ":runtime database is not available")
	}
	messages, err := newRuntimeAgentTaskMessageStore(r.service.turns.db).ListByTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	for _, msg := range messages {
		if msg.Direction != taskMessageDirectionParentToChild || msg.Kind != taskMessageKindInstruction {
			continue
		}
		if stringFromMap(msg.Payload, "prompt_source") != "runtime_task_instruction" {
			continue
		}
		prompt := strings.TrimSpace(stringFromMap(msg.Payload, "prompt"))
		if prompt != "" {
			return prompt, nil
		}
	}
	return "", errors.New(runtimeCoordinatorTaskRunnerReasonMissingPromptSource)
}

func (r runtimeCoordinatorTaskRunner) fail(ctx context.Context, req RuntimeAgentTaskExecutionRequest, reason string) (RuntimeAgentTaskExecutionResult, error) {
	if r.service != nil {
		recorder := runtimeSchedulerRecorder{service: r.service}
		_ = recorder.AgentTaskFailed(ctx, agent.AgentTaskRecord{
			ID:               req.TaskID,
			ParentTurnID:     req.ParentTurnID,
			ParentSessionID:  req.ParentSessionID,
			ParentToolCallID: req.ParentToolCallID,
			ChildSessionID:   req.ChildSessionID,
			Title:            req.Title,
			Kind:             req.Kind,
			Role:             req.Role,
			Name:             req.Name,
			PromptSummary:    req.PromptSummary,
			Model:            req.Model,
			Provider:         req.Provider,
			AllowedTools:     append([]string(nil), req.AllowedTools...),
			CapabilityScope:  append([]string(nil), req.CapabilityScope...),
			CWD:              req.CWD,
			Worktree:         req.Worktree,
			Status:           agentTaskStatusFailed,
			Progress:         100,
			StartedAt:        req.StartedAt,
			FinishedAt:       time.Now().UnixMilli(),
			Error:            reason,
		})
	}
	return RuntimeAgentTaskExecutionResult{
		TaskID:             req.TaskID,
		Status:             agentTaskStatusFailed,
		Terminal:           true,
		RefreshTargets:     runtimeRunSchedulerRefreshTargets(),
		Error:              reason,
		NoStaleResume:      true,
		CompletionOnlyRefs: true,
	}, errors.New(reason)
}
