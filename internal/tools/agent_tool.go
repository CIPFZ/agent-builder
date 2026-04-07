package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"myclaw/internal/agent"
	"myclaw/internal/session"
)

type AgentTaskRunner func(context.Context, session.Session, agent.RunContext, string) (string, error)

type AgentTaskTool struct {
	manager *agent.Manager
	run     AgentTaskRunner
}

func NewAgentTaskTool(manager *agent.Manager, run AgentTaskRunner) *AgentTaskTool {
	if manager == nil {
		manager = agent.NewManager()
	}
	return &AgentTaskTool{
		manager: manager,
		run:     run,
	}
}

func (t *AgentTaskTool) Definition() Definition {
	return Definition{
		Name:        "agent.task",
		Description: "Delegate work to a subagent by prompt, or inspect prior delegated runs.",
		AlwaysLoad:  true,
		ShouldDefer: true,
	}
}

func (t *AgentTaskTool) Invoke(ctx context.Context, sess session.Session, input string) (string, error) {
	input = strings.TrimSpace(input)
	switch {
	case input == "":
		return "", fmt.Errorf("agent.task requires a prompt or command")
	case input == "list":
		return t.list(sess), nil
	case strings.HasPrefix(input, "status "):
		return t.status(sess, strings.TrimSpace(strings.TrimPrefix(input, "status ")))
	case strings.HasPrefix(input, "wait "):
		return t.wait(ctx, sess, strings.TrimSpace(strings.TrimPrefix(input, "wait ")))
	case strings.HasPrefix(input, "resume "):
		return t.resume(ctx, sess, strings.TrimSpace(strings.TrimPrefix(input, "resume ")))
	case strings.HasPrefix(input, "steer "):
		return t.steer(sess, strings.TrimSpace(strings.TrimPrefix(input, "steer ")))
	case strings.HasPrefix(input, "stop "):
		return t.stop(sess, strings.TrimSpace(strings.TrimPrefix(input, "stop ")))
	default:
		return t.spawn(ctx, sess, input)
	}
}

func (t *AgentTaskTool) IsEnabled() bool {
	return true
}

func (t *AgentTaskTool) IsReadOnly(input string) bool {
	input = strings.TrimSpace(input)
	return input == "list" || strings.HasPrefix(input, "status ") || strings.HasPrefix(input, "wait ")
}

func (t *AgentTaskTool) IsDestructive(_ string) bool {
	return false
}

func (t *AgentTaskTool) ShouldDefer() bool {
	return true
}

func (t *AgentTaskTool) AlwaysLoad() bool {
	return true
}

func (t *AgentTaskTool) PromptDescription() string {
	return "Delegate work to a subagent. Use a task prompt to spawn, `list` / `status <run-id>` / `wait <run-id>` to inspect runs, `resume <run-id> <prompt>` to continue one, or `steer <run-id> <message>` / `stop <run-id>` to control them."
}

func (t *AgentTaskTool) SearchHint() string {
	return "delegate work to a subagent"
}

func (t *AgentTaskTool) spawn(ctx context.Context, sess session.Session, input string) (string, error) {
	label, prompt := parseAgentTaskInput(input)
	run, err := t.manager.Spawn(ctx, agent.SpawnRequest{
		ParentSessionID: sess.ID,
		ParentAgentID:   sess.AgentID,
		Label:           label,
		Prompt:          prompt,
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			if t.run != nil {
				return t.run(ctx, sess, runCtx, prompt)
			}
			return "delegated: " + prompt, nil
		},
	})
	if err != nil {
		return "", err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	result, err := t.manager.Wait(waitCtx, run.ID, 0)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return formatAgentTaskSummary("spawned", *run, map[string]string{
			"parent_session": run.ParentSessionID,
		}), nil
	}
	return formatAgentTaskSummary("spawned", result, map[string]string{
		"parent_session": run.ParentSessionID,
	}), nil
}

func (t *AgentTaskTool) list(sess session.Session) string {
	runs := make([]agent.Run, 0)
	for _, run := range t.manager.List() {
		if run.ParentSessionID == sess.ID {
			runs = append(runs, run)
		}
	}
	if len(runs) == 0 {
		return "No delegated runs for this session."
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].ID < runs[j].ID
	})
	lines := make([]string, 0, len(runs))
	for _, run := range runs {
		lines = append(lines, formatAgentTaskSummary("list", run, nil))
	}
	return strings.Join(lines, "\n")
}

func (t *AgentTaskTool) status(sess session.Session, runID string) (string, error) {
	existing, ok := t.manager.Get(runID)
	if !ok || existing.ParentSessionID != sess.ID {
		return "", fmt.Errorf("agent run %q not found", runID)
	}
	return formatAgentTaskSummary("status", existing, nil), nil
}

func (t *AgentTaskTool) wait(ctx context.Context, sess session.Session, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("wait requires <run-id>")
	}
	run, ok := t.manager.Get(runID)
	if !ok || run.ParentSessionID != sess.ID {
		return "", fmt.Errorf("agent run %q not found", runID)
	}
	result, err := t.manager.Wait(ctx, runID, 0)
	if err != nil {
		return "", err
	}
	return formatAgentTaskSummary("wait", result, nil), nil
}

func (t *AgentTaskTool) resume(ctx context.Context, sess session.Session, input string) (string, error) {
	runID, prompt, ok := strings.Cut(strings.TrimSpace(input), " ")
	if !ok || strings.TrimSpace(runID) == "" || strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("resume requires <run-id> <prompt>")
	}
	previous, ok := t.manager.Get(runID)
	if !ok || previous.ParentSessionID != sess.ID {
		return "", fmt.Errorf("agent run %q not found", runID)
	}
	run, err := t.manager.Spawn(ctx, agent.SpawnRequest{
		ParentSessionID: previous.ParentSessionID,
		ParentAgentID:   previous.ParentAgentID,
		ChildSessionID:  previous.ChildSessionID,
		ChildSessionKey: previous.ChildSessionKey,
		Label:           previous.Label,
		Prompt:          strings.TrimSpace(prompt),
		Run: func(ctx context.Context, runCtx agent.RunContext) (string, error) {
			if t.run != nil {
				return t.run(ctx, sess, runCtx, strings.TrimSpace(prompt))
			}
			return "delegated: " + strings.TrimSpace(prompt), nil
		},
	})
	if err != nil {
		return "", err
	}
	return formatAgentTaskSummary("resumed", *run, map[string]string{
		"from": previous.ID,
	}), nil
}

func (t *AgentTaskTool) steer(sess session.Session, input string) (string, error) {
	runID, message, ok := strings.Cut(strings.TrimSpace(input), " ")
	if !ok || strings.TrimSpace(runID) == "" || strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("steer requires <run-id> <message>")
	}
	run, ok := t.manager.Get(runID)
	if !ok || run.ParentSessionID != sess.ID {
		return "", fmt.Errorf("agent run %q not found", runID)
	}
	if err := t.manager.Steer(runID, strings.TrimSpace(message)); err != nil {
		return "", err
	}
	return formatAgentTaskSummary("steered", run, nil), nil
}

func (t *AgentTaskTool) stop(sess session.Session, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("stop requires <run-id>")
	}
	run, ok := t.manager.Get(runID)
	if !ok || run.ParentSessionID != sess.ID {
		return "", fmt.Errorf("agent run %q not found", runID)
	}
	if err := t.manager.Stop(runID); err != nil {
		return "", err
	}
	run.Status = agent.StatusStopped
	return formatAgentTaskSummary("stopped", run, nil), nil
}

func parseAgentTaskInput(input string) (label, prompt string) {
	input = strings.TrimSpace(input)
	if before, after, ok := strings.Cut(input, ":"); ok {
		label = strings.TrimSpace(before)
		prompt = strings.TrimSpace(after)
	}
	if label == "" {
		label = "task"
	}
	if prompt == "" {
		prompt = input
	}
	return label, prompt
}

func formatAgentTaskSummary(action string, run agent.Run, extra map[string]string) string {
	parts := []string{
		"action=" + action,
		"id=" + run.ID,
		"status=" + string(run.Status),
		fmt.Sprintf("label=%q", run.Label),
		"child_session=" + run.ChildSessionID,
	}
	if run.Output != "" {
		parts = append(parts, "output="+run.Output)
	}
	if run.Err != nil {
		parts = append(parts, "error="+run.Err.Error())
	}
	if len(extra) > 0 {
		keys := make([]string, 0, len(extra))
		for key := range extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := extra[key]
			if value == "" {
				continue
			}
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, " ")
}
