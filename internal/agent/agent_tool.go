package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strings"

	"charm.land/fantasy"

	"github.com/CIPFZ/agent-builder/internal/agent/prompt"
	"github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/config"
)

//go:embed templates/agent_tool.md
var agentToolDescription string

type AgentParams struct {
	Description  string   `json:"description,omitempty" description:"Short title for the delegated task"`
	Prompt       string   `json:"prompt" description:"The task for the agent to perform"`
	SubagentType string   `json:"subagent_type,omitempty" description:"Specialized agent role to use"`
	Role         string   `json:"role,omitempty" description:"Agent Builder role id; alias of subagent_type"`
	ParentTaskID string   `json:"parent_task_id,omitempty" description:"Parent task id for nested delegation"`
	TeamID       string   `json:"team_id,omitempty" description:"Stable Agent Team id for coordinated tasks"`
	Dependencies []string `json:"dependencies,omitempty" description:"Task ids that must complete first"`
}

const (
	AgentToolName = "agent"
)

func (c *coordinator) agentTool(ctx context.Context) (fantasy.AgentTool, error) {
	prompt, err := taskPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if err != nil {
		return nil, err
	}
	return fantasy.NewParallelAgentTool(
		AgentToolName,
		agentToolDescription,
		func(ctx context.Context, params AgentParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			params.Prompt = strings.TrimSpace(params.Prompt)
			if params.Prompt == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			role, agentCfg, err := resolveAgentToolRole(params, c.cfg.Config().Agents)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			agent, err := c.buildAgent(ctx, prompt, agentCfg, true)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}

			sessionID := tools.GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session id missing from context")
			}

			agentMessageID := tools.GetMessageFromContext(ctx)
			if agentMessageID == "" {
				return fantasy.ToolResponse{}, errors.New("agent message id missing from context")
			}

			return c.runSubAgent(ctx, subAgentParams{
				Agent:          agent,
				SessionID:      sessionID,
				AgentMessageID: agentMessageID,
				ToolCallID:     call.ID,
				ParentTaskID:   strings.TrimSpace(params.ParentTaskID),
				TeamID:         strings.TrimSpace(params.TeamID),
				Dependencies:   append([]string(nil), params.Dependencies...),
				Prompt:         params.Prompt,
				SessionTitle:   firstNonEmpty(strings.TrimSpace(params.Description), "New Agent Session"),
				Kind:           "subagent",
				Role:           role,
				Name:           firstNonEmpty(strings.TrimSpace(params.Description), AgentToolName),
				AllowedTools:   append([]string(nil), agentCfg.AllowedTools...),
			})
		}), nil
}

func resolveAgentToolRole(params AgentParams, agents map[string]config.Agent) (string, config.Agent, error) {
	role := firstNonEmpty(strings.TrimSpace(params.Role), strings.TrimSpace(params.SubagentType), config.AgentTask)
	agentCfg, ok := agents[role]
	if !ok {
		return "", config.Agent{}, fmt.Errorf("unknown subagent role %q. Available roles: %s", role, strings.Join(availableAgentRoleIDs(agents), ", "))
	}
	return role, agentCfg, nil
}

func availableAgentRoleIDs(agents map[string]config.Agent) []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		if strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
