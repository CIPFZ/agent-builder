package queryengine

import (
	"context"
	"encoding/json"
	"fmt"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/tools"
	"strings"
)

type PermissionDenial struct {
	RunID     string
	SessionID string
	ToolName  string
	ToolInput string
	Reason    string
}

type ApprovalRequiredError struct {
	ToolName  string
	ToolInput string
	Reason    string
}

func (e *ApprovalRequiredError) Error() string {
	return fmt.Sprintf("tool %s requires approval: %s", e.ToolName, e.Reason)
}

func (q *QueryEngine) canUseToolFunc(parentCtx context.Context, sess session.Session) tools.CanUseToolFunc {
	return func(ctx context.Context, req tools.CanUseToolRequest) (permissions.Decision, error) {
		if req.ForceDecision != nil {
			return *req.ForceDecision, nil
		}
		if ctx == nil {
			ctx = parentCtx
		}
		input := strings.TrimSpace(req.Input)
		inputObject := cloneAnyMap(req.InputObject)
		if input == "" && len(inputObject) > 0 {
			encoded, err := json.Marshal(inputObject)
			if err != nil {
				return permissions.Decision{}, err
			}
			input = string(encoded)
		}
		policy := q.PermissionPolicyForSession(sess.ID)
		toolDef, ok := q.tools.InspectWithPolicy(req.ToolName, input, policy)
		if !ok {
			return permissions.Decision{
				Category: permissions.CategoryRuleDenied,
				Reason:   fmt.Sprintf("tool %q is not available under the current tool policy", strings.TrimSpace(req.ToolName)),
			}, nil
		}
		toolDecision, checked, err := q.tools.CheckPermissionsWithContext(ctx, tools.ToolUseContext{
			AbortContext:       ctx,
			Session:            sess,
			ToolName:           req.ToolName,
			Input:              input,
			InputObject:        inputObject,
			Policy:             policy,
			AvailableTools:     q.exposedTools(sess.ID, true),
			AgentID:            sess.AgentID,
			MainLoopModel:      q.mainLoopModelForSession(sess.ID),
			LLMProvider:        q.llmProvider,
			Commands:           append([]tools.Command(nil), q.commands...),
			QuerySource:        q.querySource,
			CustomSystemPrompt: q.toolCustomSystemPrompt,
			AppendSystemPrompt: q.toolAppendSystemPrompt,
			Debug:              q.debug,
			Verbose:            q.verbose,
			ThinkingConfig:     cloneAnyMap(q.thinkingConfig),
			AgentDefinitions: tools.AgentDefinitions{
				ActiveAgents:      append([]string(nil), q.agentDefinitions.ActiveAgents...),
				AllowedAgentTypes: append([]string(nil), q.agentDefinitions.AllowedAgentTypes...),
				Definitions:       cloneAgentDefinitions(q.agentDefinitions.Definitions),
			},
			MaxBudgetUSD:            q.maxBudgetUSD,
			IsNonInteractive:        q.isNonInteractiveSession,
			RequireCanUseTool:       q.requireCanUseTool,
			QueryTracking:           q.queryTracking,
			ReadFileState:           cloneAnyMap(q.readFileState),
			ContentReplacementState: cloneAnyMap(q.contentReplacementState),
			CriticalSystemReminder:  q.criticalSystemReminder,
			PreserveToolUseResults:  q.preserveToolUseResults,
			RenderedSystemPrompt:    q.renderedSystemPrompt,
			Messages:                q.Messages(sess.ID),
			CanUseTool:              q.canUseToolFunc(ctx, sess),
		})
		if err != nil {
			return permissions.Decision{}, err
		}
		if checked {
			if updated, ok, err := toolDecision.UpdatedInputValue(); err != nil {
				return permissions.Decision{}, err
			} else if ok {
				input = updated
				inputObject = cloneAnyMap(toolDecision.UpdatedInputObject)
				toolDef, ok = q.tools.InspectWithPolicy(req.ToolName, input, policy)
				if !ok {
					return permissions.Decision{
						Category: permissions.CategoryRuleDenied,
						Reason:   fmt.Sprintf("tool %q is not available under the current tool policy", strings.TrimSpace(req.ToolName)),
					}, nil
				}
			}
			if !toolDecision.Allowed && (toolDecision.RequiresApproval || strings.TrimSpace(toolDecision.Reason) != "") {
				if toolDecision.RequiresApproval && q.permissionHook != nil {
					observableInput, observableInputObject := q.observableToolInput(req.ToolName, input, inputObject)
					hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
						Session:           sess,
						ToolName:          req.ToolName,
						ToolInput:         observableInput,
						ToolInputObject:   observableInputObject,
						ToolUseID:         req.ToolUseID,
						ProviderMessageID: req.ProviderMessageID,
						Decision:          toolDecision,
						Policy:            policy,
					})
					if err != nil {
						return permissions.Decision{}, err
					}
					if decided {
						if updated, ok, err := hookDecision.UpdatedInputValue(); err != nil {
							return permissions.Decision{}, err
						} else if ok {
							input = updated
							inputObject = cloneAnyMap(hookDecision.UpdatedInputObject)
							toolDef, ok = q.tools.InspectWithPolicy(req.ToolName, input, policy)
							if !ok {
								return permissions.Decision{
									Category: permissions.CategoryRuleDenied,
									Reason:   fmt.Sprintf("tool %q is not available under the current tool policy", strings.TrimSpace(req.ToolName)),
								}, nil
							}
						}
						if hookDecision.Allowed {
							toolDecision = hookDecision
						} else {
							return hookDecision, nil
						}
					} else {
						return toolDecision, nil
					}
				} else {
					return toolDecision, nil
				}
			}
		}
		autoClassifierInput, _ := q.tools.AutoClassifierInput(req.ToolName, input)
		decision := policy.Evaluate(permissions.Request{
			ToolName:            req.ToolName,
			Command:             input,
			WorkDir:             resolveWorkDir(sess, q.workspace),
			ReadOnly:            toolDef.ReadOnly,
			Destructive:         toolDef.Destructive,
			AutoClassifierInput: autoClassifierInput,
		})
		if !decision.Allowed && decision.RequiresApproval && q.permissionHook != nil {
			observableInput, observableInputObject := q.observableToolInput(req.ToolName, input, inputObject)
			hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
				Session:           sess,
				ToolName:          req.ToolName,
				ToolInput:         observableInput,
				ToolInputObject:   observableInputObject,
				ToolUseID:         req.ToolUseID,
				ProviderMessageID: req.ProviderMessageID,
				Decision:          decision,
				Policy:            policy,
			})
			if err != nil {
				return permissions.Decision{}, err
			}
			if decided {
				return hookDecision, nil
			}
		}
		if decision.Allowed {
			if updated, ok, err := toolDecision.UpdatedInputValue(); err != nil {
				return permissions.Decision{}, err
			} else if ok {
				decision.UpdatedInput = updated
				decision.UpdatedInputObject = cloneAnyMap(toolDecision.UpdatedInputObject)
			}
		}
		return decision, nil
	}
}

func (q *QueryEngine) PermissionPolicyForSession(sessionID string) permissions.Policy {
	q.policyMu.RLock()
	policy, ok := q.policies[sessionID]
	q.policyMu.RUnlock()
	if ok {
		return policy
	}
	return q.policy
}

func (q *QueryEngine) HasSessionPermissionPolicy(sessionID string) bool {
	q.policyMu.RLock()
	defer q.policyMu.RUnlock()
	_, ok := q.policies[sessionID]
	return ok
}

func (q *QueryEngine) SetSessionPermissionPolicy(sessionID string, policy permissions.Policy) {
	q.policyMu.Lock()
	defer q.policyMu.Unlock()
	q.policies[sessionID] = policy
}

func (q *QueryEngine) applyUpdatedPermissions(ctx context.Context, sess session.Session, updates []permissions.PermissionUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	if q.permissionUpdatePersister != nil {
		if err := q.permissionUpdatePersister.PersistPermissionUpdates(ctx, sess, updates); err != nil {
			return err
		}
	}
	q.policyMu.Lock()
	defer q.policyMu.Unlock()
	policy, ok := q.policies[sess.ID]
	if !ok {
		policy = q.policy
	}
	q.policies[sess.ID] = policy.ApplyPermissionUpdates(updates)
	return nil
}
