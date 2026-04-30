package queryengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"myclaw/internal/memory"
	"myclaw/internal/model"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const rejectMessage = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). STOP what you are doing and wait for the user to tell you how to proceed."

const rejectMessageWithReasonPrefix = "The user doesn't want to proceed with this tool use. The tool use was rejected (eg. if it was a file edit, the new_string was NOT written to the file). To tell you how to proceed, the user said:\n"

func rejectMessageWithFeedback(feedback string) string {
	if strings.TrimSpace(feedback) == "" {
		return rejectMessage
	}
	return rejectMessageWithReasonPrefix + strings.TrimSpace(feedback)
}

type toolCall struct {
	name              string
	input             string
	inputObject       map[string]any
	toolUseID         string
	providerMessageID string
	acceptFeedback    string
	contentBlocks     []map[string]any
	skipPermission    bool
}

func (q *QueryEngine) executeTurnLoop(ctx context.Context, sess session.Session, userMessage session.Message, runID string, sink EventSink, pending *toolCall, current *textStreamCollector) (session.Message, error) {
	var lastExecutedToolName string
	var lastExecutedToolInput string
	var lastToolMessage *session.Message
	var deferredToolExecuted bool
	var approvedToolExecuted bool
	for {
		if pending != nil && pending.name != "" {
			if pending.name == lastExecutedToolName && pending.input == lastExecutedToolInput {
				if lastToolMessage != nil {
					return q.completeWithToolResult(ctx, sess, runID, sink, *lastToolMessage)
				}
				return session.Message{}, fmt.Errorf("repeated identical tool call detected: %s %s", pending.name, pending.input)
			}
			toolDef, ok := q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
			if !ok {
				return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
			}
			if q.toolLifecycleDisabled(toolDef) {
				return session.Message{}, fmt.Errorf("tool %q is disabled by extension lifecycle state", strings.TrimSpace(toolDef.Name))
			}
			if deferredToolExecuted && toolDef.ShouldDefer {
				if lastToolMessage != nil {
					return q.completeWithToolResult(ctx, sess, runID, sink, *lastToolMessage)
				}
				return session.Message{}, fmt.Errorf("repeated deferred tool call detected: %s", pending.name)
			}
			skipPolicyEvaluation := false
			toolPermissionResolved := false
			var preHookResult PreToolUseHookResult
			var preHookHandled bool
			if !pending.skipPermission && q.preToolUseHook != nil {
				observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
				var err error
				preHookResult, preHookHandled, err = q.preToolUseHook.BeforeToolUse(ctx, PreToolUseHookRequest{
					Session:           sess,
					RunID:             runID,
					ToolName:          pending.name,
					ToolInput:         observableInput,
					ToolInputObject:   observableInputObject,
					ToolUseID:         pending.toolUseID,
					ProviderMessageID: pending.providerMessageID,
					Policy:            q.PermissionPolicyForSession(sess.ID),
				})
				if err != nil {
					return session.Message{}, err
				}
				if preHookHandled {
					if updated, ok, err := preHookResult.UpdatedInputValue(); err != nil {
						return session.Message{}, err
					} else if ok {
						pending.input = updated
						pending.inputObject = cloneAnyMap(preHookResult.UpdatedInputObject)
						toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
						if !ok {
							return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
						}
					}
					if blockingError := strings.TrimSpace(preHookResult.BlockingError); blockingError != "" {
						decision := permissions.Decision{
							Reason: blockingError,
							DecisionReason: permissions.DecisionReason{
								Type:     permissions.DecisionReasonHook,
								HookName: "PreToolUse:" + pending.name,
								Reason:   blockingError,
							},
						}
						q.recordPermissionDenial(PermissionDenial{
							RunID:     runID,
							SessionID: sess.ID,
							ToolName:  pending.name,
							ToolInput: pending.input,
							Reason:    decision.Reason,
						})
						return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, decision.Reason, decision.ContentBlocks)
					}
					if preHookResult.HasPermissionDecision {
						decision := preHookResult.PermissionDecision
						if decision.Allowed {
							if err := q.applyUpdatedPermissions(ctx, sess, decision.UpdatedPermissions); err != nil {
								return session.Message{}, err
							}
							toolPermissionResolved = true
						} else if !decision.RequiresApproval {
							q.recordPermissionDenial(PermissionDenial{
								RunID:     runID,
								SessionID: sess.ID,
								ToolName:  pending.name,
								ToolInput: pending.input,
								Reason:    decision.Reason,
							})
							return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, decision.Reason, decision.ContentBlocks)
						} else {
							q.recordPermissionDenial(PermissionDenial{
								RunID:     runID,
								SessionID: sess.ID,
								ToolName:  pending.name,
								ToolInput: pending.input,
								Reason:    decision.Reason,
							})
							req := q.approvals.CreateWithPromptMetadata(sess.ID, runID, userMessage.ID, pending.name, pending.input, pending.inputObject, pending.toolUseID, pending.providerMessageID, decision.Reason, decision.SerializedDecisionReason(), decision.AcceptFeedback, decision.ContentBlocks, string(decision.Category), decision.RuleSource)
							_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
								metadata.PendingApprovalID = req.ID
								metadata.PendingApprovalStatus = string(req.Status)
								metadata.PendingApprovalToolName = req.ToolName
								metadata.PendingApprovalToolInput = req.ToolInput
								metadata.PendingApprovalToolInputObject = cloneAnyMap(req.ToolInputObject)
								metadata.PendingApprovalToolUseID = req.ToolUseID
								metadata.PendingApprovalProviderMsgID = req.ProviderMessageID
								metadata.PendingApprovalReason = req.Reason
								metadata.PendingApprovalDecisionReason = req.DecisionReason
								metadata.PendingApprovalAcceptFeedback = req.AcceptFeedback
								metadata.PendingApprovalContentBlocks = cloneAnyMaps(req.ContentBlocks)
								metadata.PendingApprovalRunID = req.RunID
								metadata.PendingApprovalUserMessageID = req.UserMessageID
								metadata.PendingApprovalCategory = req.Category
								metadata.PendingApprovalRuleSource = req.RuleSource
							})
							if err := q.emit(sink, Event{
								Type:                  "permission.required",
								Session:               sess,
								RunID:                 runID,
								ToolUseID:             pending.toolUseID,
								ProviderMessageID:     pending.providerMessageID,
								ToolName:              pending.name,
								ToolInput:             pending.input,
								DecisionReason:        decision.SerializedDecisionReason(),
								DecisionReasonDetails: decision.DecisionReason.Structured(),
								AcceptFeedback:        decision.AcceptFeedback,
								ContentBlocks:         cloneAnyMaps(decision.ContentBlocks),
								Approval:              &req,
							}); err != nil {
								return session.Message{}, err
							}
							return session.Message{}, &ApprovalRequiredError{
								ToolName:  pending.name,
								ToolInput: pending.input,
								Reason:    decision.Reason,
							}
						}
					}
				}
			}
			if !pending.skipPermission {
				if !toolPermissionResolved {
					toolDecision, checked, err := q.tools.CheckPermissionsWithContext(ctx, q.toolUseContext(ctx, sess, pending, runID, sink))
					if err != nil {
						return session.Message{}, err
					}
					if checked {
						if updated, ok, err := toolDecision.UpdatedInputValue(); err != nil {
							return session.Message{}, err
						} else if ok {
							pending.input = updated
							pending.inputObject = cloneAnyMap(toolDecision.UpdatedInputObject)
							toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
							if !ok {
								return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
							}
						}
						if !toolDecision.Allowed && (toolDecision.RequiresApproval || strings.TrimSpace(toolDecision.Reason) != "") {
							if toolDecision.RequiresApproval && q.permissionHook != nil {
								observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
								hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
									Session:           sess,
									RunID:             runID,
									ToolName:          pending.name,
									ToolInput:         observableInput,
									ToolInputObject:   observableInputObject,
									ToolUseID:         pending.toolUseID,
									ProviderMessageID: pending.providerMessageID,
									Decision:          toolDecision,
									Policy:            q.PermissionPolicyForSession(sess.ID),
								})
								if err != nil {
									return session.Message{}, err
								}
								if decided {
									if updated, ok, err := hookDecision.UpdatedInputValue(); err != nil {
										return session.Message{}, err
									} else if ok {
										pending.input = updated
										pending.inputObject = cloneAnyMap(hookDecision.UpdatedInputObject)
										toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
										if !ok {
											return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
										}
									}
									if hookDecision.Allowed {
										if err := q.applyUpdatedPermissions(ctx, sess, hookDecision.UpdatedPermissions); err != nil {
											return session.Message{}, err
										}
										toolPermissionResolved = true
									} else if !hookDecision.RequiresApproval {
										return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, hookDecision.Reason, hookDecision.ContentBlocks)
									} else {
										toolDecision = hookDecision
									}
								}
							}
							if !toolPermissionResolved {
								q.recordPermissionDenial(PermissionDenial{
									RunID:     runID,
									SessionID: sess.ID,
									ToolName:  pending.name,
									ToolInput: pending.input,
									Reason:    toolDecision.Reason,
								})
								if !toolDecision.RequiresApproval {
									return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, toolDecision.Reason, toolDecision.ContentBlocks)
								}
								req := q.approvals.CreateWithPromptMetadata(sess.ID, runID, userMessage.ID, pending.name, pending.input, pending.inputObject, pending.toolUseID, pending.providerMessageID, toolDecision.Reason, toolDecision.SerializedDecisionReason(), toolDecision.AcceptFeedback, toolDecision.ContentBlocks, string(toolDecision.Category), toolDecision.RuleSource)
								_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
									metadata.PendingApprovalID = req.ID
									metadata.PendingApprovalStatus = string(req.Status)
									metadata.PendingApprovalToolName = req.ToolName
									metadata.PendingApprovalToolInput = req.ToolInput
									metadata.PendingApprovalToolInputObject = cloneAnyMap(req.ToolInputObject)
									metadata.PendingApprovalToolUseID = req.ToolUseID
									metadata.PendingApprovalProviderMsgID = req.ProviderMessageID
									metadata.PendingApprovalReason = req.Reason
									metadata.PendingApprovalDecisionReason = req.DecisionReason
									metadata.PendingApprovalAcceptFeedback = req.AcceptFeedback
									metadata.PendingApprovalContentBlocks = cloneAnyMaps(req.ContentBlocks)
									metadata.PendingApprovalRunID = req.RunID
									metadata.PendingApprovalUserMessageID = req.UserMessageID
									metadata.PendingApprovalCategory = req.Category
									metadata.PendingApprovalRuleSource = req.RuleSource
								})
								if err := q.emit(sink, Event{
									Type:                  "permission.required",
									Session:               sess,
									RunID:                 runID,
									ToolUseID:             pending.toolUseID,
									ProviderMessageID:     pending.providerMessageID,
									ToolName:              pending.name,
									ToolInput:             pending.input,
									DecisionReason:        toolDecision.SerializedDecisionReason(),
									DecisionReasonDetails: toolDecision.DecisionReason.Structured(),
									AcceptFeedback:        toolDecision.AcceptFeedback,
									ContentBlocks:         cloneAnyMaps(toolDecision.ContentBlocks),
									Approval:              &req,
								}); err != nil {
									return session.Message{}, err
								}
								return session.Message{}, &ApprovalRequiredError{
									ToolName:  pending.name,
									ToolInput: pending.input,
									Reason:    toolDecision.Reason,
								}
							}
						}
					}
				}
				if !skipPolicyEvaluation {
					autoClassifierInput, _ := q.tools.AutoClassifierInput(pending.name, pending.input)
					decision := q.PermissionPolicyForSession(sess.ID).Evaluate(permissions.Request{
						ToolName:            pending.name,
						Command:             pending.input,
						WorkDir:             resolveWorkDir(sess, q.workspace),
						ReadOnly:            toolDef.ReadOnly,
						Destructive:         toolDef.Destructive,
						AutoClassifierInput: autoClassifierInput,
					})
					if !decision.Allowed {
						if decision.RequiresApproval && q.permissionHook != nil {
							observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
							hookDecision, decided, err := q.permissionHook.CheckPermission(ctx, PermissionHookRequest{
								Session:           sess,
								RunID:             runID,
								ToolName:          pending.name,
								ToolInput:         observableInput,
								ToolInputObject:   observableInputObject,
								ToolUseID:         pending.toolUseID,
								ProviderMessageID: pending.providerMessageID,
								Decision:          decision,
								Policy:            q.PermissionPolicyForSession(sess.ID),
							})
							if err != nil {
								return session.Message{}, err
							}
							if decided {
								if updated, ok, err := hookDecision.UpdatedInputValue(); err != nil {
									return session.Message{}, err
								} else if ok {
									pending.input = updated
									pending.inputObject = cloneAnyMap(hookDecision.UpdatedInputObject)
									toolDef, ok = q.tools.InspectWithPolicy(pending.name, pending.input, q.PermissionPolicyForSession(sess.ID))
									if !ok {
										return session.Message{}, fmt.Errorf("tool %q is not available under the current tool policy", strings.TrimSpace(pending.name))
									}
								}
								if hookDecision.Allowed {
									if err := q.applyUpdatedPermissions(ctx, sess, hookDecision.UpdatedPermissions); err != nil {
										return session.Message{}, err
									}
									skipPolicyEvaluation = true
								} else if !hookDecision.RequiresApproval {
									return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, hookDecision.Reason, hookDecision.ContentBlocks)
								} else {
									decision = hookDecision
								}
							}
						}
					}
					if !decision.Allowed && !skipPolicyEvaluation {
						q.recordPermissionDenial(PermissionDenial{
							RunID:     runID,
							SessionID: sess.ID,
							ToolName:  pending.name,
							ToolInput: pending.input,
							Reason:    decision.Reason,
						})
						if !decision.RequiresApproval {
							return q.completeWithPermissionRejection(ctx, sess, runID, sink, pending, decision.Reason, decision.ContentBlocks)
						}
						req := q.approvals.CreateWithPromptMetadata(sess.ID, runID, userMessage.ID, pending.name, pending.input, pending.inputObject, pending.toolUseID, pending.providerMessageID, decision.Reason, decision.SerializedDecisionReason(), decision.AcceptFeedback, decision.ContentBlocks, string(decision.Category), decision.RuleSource)
						_ = q.sessions.UpdateMetadata(sess.ID, func(metadata *session.SessionMetadata) {
							metadata.PendingApprovalID = req.ID
							metadata.PendingApprovalStatus = string(req.Status)
							metadata.PendingApprovalToolName = req.ToolName
							metadata.PendingApprovalToolInput = req.ToolInput
							metadata.PendingApprovalToolInputObject = cloneAnyMap(req.ToolInputObject)
							metadata.PendingApprovalToolUseID = req.ToolUseID
							metadata.PendingApprovalProviderMsgID = req.ProviderMessageID
							metadata.PendingApprovalReason = req.Reason
							metadata.PendingApprovalDecisionReason = req.DecisionReason
							metadata.PendingApprovalAcceptFeedback = req.AcceptFeedback
							metadata.PendingApprovalContentBlocks = cloneAnyMaps(req.ContentBlocks)
							metadata.PendingApprovalRunID = req.RunID
							metadata.PendingApprovalUserMessageID = req.UserMessageID
							metadata.PendingApprovalCategory = req.Category
							metadata.PendingApprovalRuleSource = req.RuleSource
						})
						if err := q.emit(sink, Event{
							Type:                  "permission.required",
							Session:               sess,
							RunID:                 runID,
							ToolUseID:             pending.toolUseID,
							ProviderMessageID:     pending.providerMessageID,
							ToolName:              pending.name,
							ToolInput:             pending.input,
							DecisionReason:        decision.SerializedDecisionReason(),
							DecisionReasonDetails: decision.DecisionReason.Structured(),
							AcceptFeedback:        decision.AcceptFeedback,
							ContentBlocks:         cloneAnyMaps(decision.ContentBlocks),
							Approval:              &req,
						}); err != nil {
							return session.Message{}, err
						}
						return session.Message{}, &ApprovalRequiredError{
							ToolName:  pending.name,
							ToolInput: pending.input,
							Reason:    decision.Reason,
						}
					}
				}
			}
			toolUseID := strings.TrimSpace(pending.toolUseID)
			if toolUseID == "" {
				toolUseID = fmt.Sprintf("toolu-%s-%s", runID, strings.ReplaceAll(pending.name, ".", "-"))
			}
			providerMessageID := strings.TrimSpace(pending.providerMessageID)
			if providerMessageID == "" {
				providerMessageID = "msg-" + toolUseID
			}
			pending.toolUseID = toolUseID
			pending.providerMessageID = providerMessageID
			observableInput, observableInputObject := q.observableToolInput(pending.name, pending.input, pending.inputObject)
			toolUseMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "assistant", fmt.Sprintf("%s: %s", pending.name, observableInput), providerMessageID, []model.MessageBlock{
				{
					Type:        model.MessageBlockToolUse,
					ID:          toolUseID,
					Name:        pending.name,
					Input:       observableInput,
					InputObject: observableInputObject,
				},
			})
			if err != nil {
				return session.Message{}, err
			}
			q.appendMutableMessage(sess.ID, toolUseMsg)

			if err := q.emit(sink, Event{
				Type:              "tool.called",
				Session:           sess,
				RunID:             runID,
				ToolUseID:         toolUseID,
				ProviderMessageID: providerMessageID,
				ToolName:          pending.name,
				ToolInput:         observableInput,
				ToolInputObject:   observableInputObject,
			}); err != nil {
				return session.Message{}, err
			}
			executionContext := q.toolUseContext(ctx, sess, pending, runID, sink)
			if q.toolLifecycleDisabled(toolDef) {
				return session.Message{}, fmt.Errorf("tool %q is disabled by extension lifecycle state", strings.TrimSpace(toolDef.Name))
			}
			toolResult, err := q.tools.InvokeWithContext(ctx, executionContext)
			if err != nil {
				q.markMCPServerNeedsAuth(pending.name, err)
				errorText := strings.TrimSpace(err.Error())
				if errorText == "" {
					errorText = "tool execution failed"
				}
				toolOutput := strings.TrimSpace(toolResult.Output)
				if toolOutput == "" {
					toolOutput = errorText
				}
				failureBlocks := []model.MessageBlock{
					{
						Type:      model.MessageBlockToolResult,
						ToolUseID: toolUseID,
						Content:   toolOutput,
						IsError:   true,
					},
				}
				if q.postToolUseFailureHook != nil {
					failureResult, handled, hookErr := q.postToolUseFailureHook.AfterToolUseFailure(ctx, PostToolUseFailureHookRequest{
						Session:           sess,
						RunID:             runID,
						ToolName:          pending.name,
						ToolInput:         observableInput,
						ToolInputObject:   observableInputObject,
						ToolUseID:         toolUseID,
						ProviderMessageID: providerMessageID,
						Error:             errorText,
						Policy:            q.PermissionPolicyForSession(sess.ID),
					})
					if hookErr != nil {
						return session.Message{}, hookErr
					}
					if handled {
						failureBlocks = append(failureBlocks, postToolUseFailureHookBlocks(pending.name, toolUseID, failureResult)...)
					}
				}
				toolMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "tool", fmt.Sprintf("%s: %s", pending.name, toolOutput), "", failureBlocks)
				if err != nil {
					return session.Message{}, err
				}
				q.appendMutableMessage(sess.ID, toolMsg)
				if err := q.emit(sink, Event{
					Type:              "tool.result",
					Session:           sess,
					RunID:             runID,
					Message:           &toolMsg,
					ToolUseID:         toolUseID,
					ProviderMessageID: providerMessageID,
					ToolName:          pending.name,
					ToolInput:         observableInput,
					ToolInputObject:   observableInputObject,
					ToolError:         true,
					StructuredContent: toolResult.StructuredContent,
					Meta:              cloneAnyMap(toolResult.Meta),
				}); err != nil {
					return session.Message{}, err
				}
				lastToolMessage = &toolMsg
				lastExecutedToolName = pending.name
				lastExecutedToolInput = pending.input
				if toolDef.ShouldDefer {
					deferredToolExecuted = true
				}
				pending = nil
				current = nil
				continue
			}
			toolOutput := toolResult.Output
			q.recordReadFileState(sess.ID, pending.name, pending.inputObject, pending.toolUseID)
			q.applyToolContextModifier(sess.ID, executionContext, toolResult.ContextModifier)
			var postHookResult PostToolUseHookResult
			var postHookHandled bool
			if q.postToolUseHook != nil {
				postHookResult, postHookHandled, err = q.postToolUseHook.AfterToolUse(ctx, PostToolUseHookRequest{
					Session:           sess,
					RunID:             runID,
					ToolName:          pending.name,
					ToolInput:         observableInput,
					ToolInputObject:   observableInputObject,
					ToolUseID:         toolUseID,
					ProviderMessageID: providerMessageID,
					ToolOutput:        toolOutput,
					Policy:            q.PermissionPolicyForSession(sess.ID),
				})
				if err != nil {
					return session.Message{}, err
				}
				if isMCPToolDefinition(toolDef) {
					if updatedOutput := strings.TrimSpace(postHookResult.UpdatedMCPToolOutput); updatedOutput != "" {
						toolOutput = updatedOutput
					}
				}
			}
			toolResultBlocks := []model.MessageBlock{
				{
					Type:      model.MessageBlockToolResult,
					ToolUseID: toolUseID,
					Content:   toolOutput,
				},
			}
			if feedback := strings.TrimSpace(pending.acceptFeedback); feedback != "" {
				toolResultBlocks = append(toolResultBlocks, model.MessageBlock{
					Type: model.MessageBlockText,
					Text: feedback,
				})
			}
			toolResultBlocks = append(toolResultBlocks, messageBlocksFromContentMaps(pending.contentBlocks)...)
			if preHookHandled {
				toolResultBlocks = append(toolResultBlocks, preToolUseHookBlocks(pending.name, toolUseID, preHookResult)...)
			}
			if postHookHandled {
				toolResultBlocks = append(toolResultBlocks, postToolUseHookBlocks(pending.name, toolUseID, postHookResult)...)
			}
			toolMsg, err := q.sessions.AppendMessageWithBlocks(sess.ID, "tool", fmt.Sprintf("%s: %s", pending.name, toolOutput), "", toolResultBlocks)
			if err != nil {
				return session.Message{}, err
			}
			q.appendMutableMessage(sess.ID, toolMsg)
			for _, newMessage := range toolResult.NewMessages {
				if strings.TrimSpace(newMessage.SessionID) == "" {
					newMessage.SessionID = sess.ID
				}
				if newMessage.CreatedAt.IsZero() {
					newMessage.CreatedAt = time.Now().UTC()
				}
				appended, err := q.sessions.AppendModelMessage(sess.ID, newMessage)
				if err != nil {
					return session.Message{}, err
				}
				q.appendMutableMessage(sess.ID, appended)
			}
			if err := q.emit(sink, Event{
				Type:              "tool.result",
				Session:           sess,
				RunID:             runID,
				Message:           &toolMsg,
				ToolUseID:         toolUseID,
				ProviderMessageID: providerMessageID,
				ToolName:          pending.name,
				ToolInput:         observableInput,
				ToolInputObject:   observableInputObject,
				StructuredContent: toolResult.StructuredContent,
				Meta:              cloneAnyMap(toolResult.Meta),
			}); err != nil {
				return session.Message{}, err
			}
			lastToolMessage = &toolMsg
			lastExecutedToolName = pending.name
			lastExecutedToolInput = pending.input
			if pending.skipPermission {
				approvedToolExecuted = true
			}
			if toolDef.ShouldDefer {
				deferredToolExecuted = true
			}
			if preHookHandled && preHookResult.PreventContinuation {
				return toolMsg, nil
			}
			if postHookHandled && postHookResult.PreventContinuation {
				return toolMsg, nil
			}
			pending = nil
			current = nil
		}

		if current == nil {
			if limit := q.effectiveMaxTurns(sess); limit > 0 && q.State().LastModelPassCount >= limit {
				q.recordMaxTurnsExceeded()
				return session.Message{}, fmt.Errorf("reached maximum number of turns (%d)", limit)
			}
			stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
			if err != nil {
				return session.Message{}, err
			}
			q.recordModelPass()
			current = stream
		}

		if current.ToolName != "" {
			if approvedToolExecuted && lastToolMessage != nil {
				return q.completeWithToolResult(ctx, sess, runID, sink, *lastToolMessage)
			}
			pending = &toolCall{name: current.ToolName, input: current.ToolInput, inputObject: normalizedToolInputObject(current.ToolInput, current.ToolInputObject), toolUseID: current.ToolUseID, providerMessageID: current.ProviderMessageID}
			current = nil
			continue
		}

		reply, err := q.sessions.AppendMessage(sess.ID, "assistant", current.Content())
		if err != nil {
			return session.Message{}, err
		}
		q.appendMutableMessage(sess.ID, reply)
		q.recordUsageEstimate(sess.ID, reply.Content)
		q.setLastAssistantReply(reply.Content)
		if err := q.emit(sink, Event{
			Type:    "message.created",
			Session: sess,
			RunID:   runID,
			Message: &reply,
		}); err != nil {
			return session.Message{}, err
		}
		return reply, nil
	}
}

func (q *QueryEngine) completeWithToolResult(ctx context.Context, sess session.Session, runID string, sink EventSink, toolMsg session.Message) (session.Message, error) {
	userMessage := q.latestUserMessage(sess.ID)
	if userMessage.ID == "" {
		userMessage = toolMsg
	}
	if limit := q.effectiveMaxTurns(sess); limit > 0 && q.State().LastModelPassCount >= limit {
		q.recordMaxTurnsExceeded()
		return session.Message{}, fmt.Errorf("reached maximum number of turns (%d)", limit)
	}
	stream, err := q.runModelPass(ctx, sess, userMessage, runID, sink)
	if err != nil {
		return session.Message{}, err
	}
	q.recordModelPass()
	return q.executeTurnLoop(ctx, sess, userMessage, runID, sink, nil, stream)
}

func (q *QueryEngine) recordReadFileState(sessionID, toolName string, input map[string]any, toolUseID string) {
	if !strings.EqualFold(strings.TrimSpace(toolName), "Read") {
		return
	}
	rawPath, _ := input["file_path"].(string)
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return
	}
	sess, ok := q.sessions.GetByID(sessionID)
	if !ok {
		return
	}
	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(sess.Metadata.AgentWorktreePath)
		if base == "" {
			base = strings.TrimSpace(sess.Metadata.AgentCWD)
		}
		if base == "" {
			base = resolveWorkDir(sess, q.workspace)
		}
		if strings.TrimSpace(base) != "" {
			path = filepath.Join(base, path)
		}
	}
	meta, err := readFileMetadata(path, toolUseID)
	if err != nil {
		return
	}
	meta.LastReadAt = time.Now().UTC()
	_ = q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		upsertReadFileMetadata(metadata, meta)
	})
}

func (q *QueryEngine) persistMemoryItems(sessionID string, items []memory.Item) {
	metadataItems := memory.MetadataFromItems(items)
	_ = q.sessions.UpdateMetadata(sessionID, func(metadata *session.SessionMetadata) {
		metadata.MemoryItems = metadataItems
	})
}

func upsertReadFileMetadata(metadata *session.SessionMetadata, item model.ReadFileMetadata) {
	for i := range metadata.ReadFiles {
		if sameFilePath(metadata.ReadFiles[i].Path, item.Path) {
			metadata.ReadFiles[i] = item
			return
		}
	}
	metadata.ReadFiles = append(metadata.ReadFiles, item)
}

func sameFilePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func readFileMetadata(path, toolUseID string) (model.ReadFileMetadata, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return model.ReadFileMetadata{}, fmt.Errorf("missing path")
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return model.ReadFileMetadata{}, err
	}
	info, err := os.Stat(clean)
	if err != nil {
		return model.ReadFileMetadata{}, err
	}
	sum := sha256.Sum256(data)
	return model.ReadFileMetadata{
		Path:       clean,
		Hash:       hex.EncodeToString(sum[:]),
		Size:       info.Size(),
		ModTime:    info.ModTime().UTC(),
		ToolUseID:  strings.TrimSpace(toolUseID),
		LastReadAt: time.Now().UTC(),
	}, nil
}
