package runtime

import "context"

type runtimeAgentTaskRunner interface {
	ExecuteAgentTask(context.Context, RuntimeAgentTaskExecutionRequest) (RuntimeAgentTaskExecutionResult, error)
}

func runtimeAgentTaskExecutionRequest(run RuntimeRun, task RuntimeAgentTask, prompt string) RuntimeAgentTaskExecutionRequest {
	return RuntimeAgentTaskExecutionRequest{
		RunID:                   run.ID,
		TaskID:                  task.ID,
		ParentSessionID:         task.ParentSessionID,
		ParentTurnID:            task.ParentTurnID,
		ParentToolCallID:        task.ParentToolCallID,
		ParentTaskID:            task.ParentTaskID,
		ChildSessionID:          task.ChildSessionID,
		TeamID:                  task.TeamID,
		Dependencies:            append([]string(nil), task.Dependencies...),
		Title:                   task.Title,
		Kind:                    task.Kind,
		Role:                    task.Role,
		Name:                    task.Name,
		Prompt:                  prompt,
		PromptSummary:           task.PromptSummary,
		Provider:                task.Provider,
		Model:                   task.Model,
		AllowedTools:            append([]string(nil), task.AllowedTools...),
		CapabilityScope:         append([]string(nil), task.CapabilityScope...),
		CWD:                     task.CWD,
		Worktree:                task.Worktree,
		StartedAt:               task.StartedAt,
		StartAlreadyRecorded:    true,
		WorkbenchOnly:           true,
		EventPayloadRefreshOnly: true,
	}
}
