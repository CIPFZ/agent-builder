// Package agent is the core orchestration layer for Agent Builder AI agents.
//
// It provides session-based AI agent functionality for managing
// conversations, tool execution, and message handling. It coordinates
// interactions between language models, messages, sessions, and tools while
// handling features like queuing and token management.
package agent

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/bedrock"
	"charm.land/fantasy/providers/google"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openrouter"
	"charm.land/fantasy/providers/vercel"
	"github.com/CIPFZ/agent-builder/internal/agent/hyper"
	"github.com/CIPFZ/agent-builder/internal/agent/notify"
	"github.com/CIPFZ/agent-builder/internal/agent/tools"
	"github.com/CIPFZ/agent-builder/internal/agent/tools/mcp"
	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/csync"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/pubsub"
	"github.com/CIPFZ/agent-builder/internal/session"
	"github.com/CIPFZ/agent-builder/internal/stringext"
	"github.com/CIPFZ/agent-builder/internal/version"
)

const (
	DefaultSessionName = "Untitled Session"
)

var userAgent = fmt.Sprintf("Agent-Builder/%s (https://github.com/CIPFZ/agent-builder)", version.Version)

//go:embed templates/title.md
var titlePrompt []byte

//go:embed templates/summary.md
var summaryPrompt []byte

// Used to remove <think> tags from generated titles.
var (
	thinkTagRegex       = regexp.MustCompile(`(?s)<think>.*?</think>`)
	orphanThinkTagRegex = regexp.MustCompile(`</?think>`)
)

type SessionAgentCall struct {
	SessionID        string
	TurnID           string
	Prompt           string
	ProviderOptions  fantasy.ProviderOptions
	Attachments      []message.Attachment
	MessageMetadata  map[string]string
	MaxOutputTokens  int64
	Temperature      *float64
	TopP             *float64
	TopK             *int64
	FrequencyPenalty *float64
	PresencePenalty  *float64
	NonInteractive   bool
}

type SessionAgent interface {
	Run(context.Context, SessionAgentCall) (*fantasy.AgentResult, error)
	SetModels(large Model, small Model)
	SetTools(tools []fantasy.AgentTool)
	SetSystemPrompt(systemPrompt string)
	SetSystemPromptSections(sections []PromptSectionSummary)
	Cancel(sessionID string)
	CancelAll()
	IsSessionBusy(sessionID string) bool
	IsBusy() bool
	QueuedPrompts(sessionID string) int
	QueuedPromptsList(sessionID string) []string
	ClearQueue(sessionID string)
	Summarize(context.Context, string, fantasy.ProviderOptions) error
	Model() Model
}

type Model struct {
	Model      fantasy.LanguageModel
	CatwalkCfg catwalk.Model
	ModelCfg   config.SelectedModel
	FlatRate   bool
}

type sessionAgent struct {
	largeModel         *csync.Value[Model]
	smallModel         *csync.Value[Model]
	systemPromptPrefix *csync.Value[string]
	systemPrompt       *csync.Value[string]
	systemSections     *csync.Slice[PromptSectionSummary]
	tools              *csync.Slice[fantasy.AgentTool]

	isSubAgent           bool
	sessions             session.Service
	messages             message.Service
	disableAutoSummarize bool
	isYolo               bool
	notify               pubsub.Publisher[notify.Notification]

	messageQueue   *csync.Map[string, []SessionAgentCall]
	activeRequests *csync.Map[string, context.CancelFunc]

	guard             *ToolResultGuard
	guardConfig       config.ToolResultGuardConfig
	modelInputBuilder ModelInputBuilder
	assemblyRecorder  PromptAssemblyRecorder
}

type SessionAgentOptions struct {
	LargeModel           Model
	SmallModel           Model
	SystemPromptPrefix   string
	SystemPrompt         string
	SystemPromptSections []PromptSectionSummary
	IsSubAgent           bool
	DisableAutoSummarize bool
	IsYolo               bool
	Sessions             session.Service
	Messages             message.Service
	Tools                []fantasy.AgentTool
	Notify               pubsub.Publisher[notify.Notification]
	GuardConfig          config.ToolResultGuardConfig
	ModelInputBuilder    ModelInputBuilder
	AssemblyRecorder     PromptAssemblyRecorder
}

func NewSessionAgent(
	opts SessionAgentOptions,
) SessionAgent {
	return &sessionAgent{
		largeModel:           csync.NewValue(opts.LargeModel),
		smallModel:           csync.NewValue(opts.SmallModel),
		systemPromptPrefix:   csync.NewValue(opts.SystemPromptPrefix),
		systemPrompt:         csync.NewValue(opts.SystemPrompt),
		systemSections:       csync.NewSliceFrom(opts.SystemPromptSections),
		isSubAgent:           opts.IsSubAgent,
		sessions:             opts.Sessions,
		messages:             opts.Messages,
		disableAutoSummarize: opts.DisableAutoSummarize,
		tools:                csync.NewSliceFrom(opts.Tools),
		isYolo:               opts.IsYolo,
		notify:               opts.Notify,
		messageQueue:         csync.NewMap[string, []SessionAgentCall](),
		activeRequests:       csync.NewMap[string, context.CancelFunc](),
		guardConfig:          opts.GuardConfig,
		modelInputBuilder:    opts.ModelInputBuilder,
		assemblyRecorder:     opts.AssemblyRecorder,
	}
}

func (a *sessionAgent) Run(ctx context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
	if call.Prompt == "" && !message.ContainsTextAttachment(call.Attachments) {
		return nil, ErrEmptyPrompt
	}
	if call.SessionID == "" {
		return nil, ErrSessionMissing
	}

	// Queue the message if busy
	if a.IsSessionBusy(call.SessionID) {
		existing, ok := a.messageQueue.Get(call.SessionID)
		if !ok {
			existing = []SessionAgentCall{}
		}
		existing = append(existing, call)
		a.messageQueue.Set(call.SessionID, existing)
		return nil, nil
	}

	// Copy mutable fields under lock to avoid races with SetTools/SetModels.
	agentTools := a.tools.Copy()
	largeModel := a.largeModel.Get()
	systemPrompt := a.systemPrompt.Get()
	systemSections := a.systemSections.Copy()
	promptPrefix := a.systemPromptPrefix.Get()
	var instructions strings.Builder
	var mcpInstructions []mcpInstructionSnapshot

	for _, server := range mcp.GetStates() {
		if server.State != mcp.StateConnected {
			continue
		}
		if s := server.Client.InitializeResult().Instructions; s != "" {
			mcpInstructions = append(mcpInstructions, mcpInstructionSnapshot{
				Server:  server.Name,
				Content: s,
			})
			instructions.WriteString(s)
			instructions.WriteString("\n\n")
		}
	}

	if s := instructions.String(); s != "" {
		systemPrompt += "\n\n<mcp-instructions>\n" + s + "\n</mcp-instructions>"
	}

	if len(agentTools) > 0 {
		// Add Anthropic caching to the last tool.
		agentTools[len(agentTools)-1].SetProviderOptions(a.getCacheControlOptions())
	}

	agent := fantasy.NewAgent(
		largeModel.Model,
		fantasy.WithSystemPrompt(systemPrompt),
		fantasy.WithTools(agentTools...),
		fantasy.WithRepairToolCall(repairToolCallJSON),
		fantasy.WithUserAgent(userAgent),
	)

	sessionLock := sync.Mutex{}
	currentSession, err := a.sessions.Get(ctx, call.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	guardCfg := a.guardConfig
	if !guardCfg.Enabled && guardCfg.MaxResultChars == 0 {
		guardCfg = config.DefaultToolResultGuardConfig()
	}
	a.guard = NewToolResultGuard(guardCfg, currentSession.ID)
	a.guard.CleanupOldFiles()

	msgs, err := a.getSessionMessages(ctx, currentSession)
	if err != nil {
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}

	var wg sync.WaitGroup
	// Generate title if first message.
	if len(msgs) == 0 {
		titleCtx := ctx // Copy to avoid race with ctx reassignment below.
		wg.Go(func() {
			a.generateTitle(titleCtx, call.SessionID, call.Prompt)
		})
	}
	defer wg.Wait()

	// Add the user message to the session.
	_, err = a.createUserMessage(ctx, call)
	if err != nil {
		return nil, err
	}

	// Add the session to the context.
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, call.SessionID)
	if call.TurnID != "" {
		ctx = context.WithValue(ctx, tools.TurnIDContextKey, call.TurnID)
	}

	genCtx, cancel := context.WithCancel(ctx)
	a.activeRequests.Set(call.SessionID, cancel)

	defer cancel()
	defer a.activeRequests.Del(call.SessionID)

	history, files := a.preparePrompt(msgs, largeModel.CatwalkCfg.SupportsImages, call.Attachments...)

	startTime := time.Now()
	a.eventPromptSent(call.SessionID)

	var currentAssistant *message.Message
	var stepMessages []fantasy.Message
	// Don't send MaxOutputTokens if 0 — some providers (e.g. LM Studio) reject it
	var maxOutputTokens *int64
	if call.MaxOutputTokens > 0 {
		maxOutputTokens = &call.MaxOutputTokens
	}
	result, err := agent.Stream(genCtx, fantasy.AgentStreamCall{
		Prompt:           message.PromptWithTextAttachments(call.Prompt, call.Attachments),
		Files:            files,
		Messages:         history,
		ProviderOptions:  call.ProviderOptions,
		MaxOutputTokens:  maxOutputTokens,
		TopP:             call.TopP,
		Temperature:      call.Temperature,
		PresencePenalty:  call.PresencePenalty,
		TopK:             call.TopK,
		FrequencyPenalty: call.FrequencyPenalty,
		PrepareStep: func(callContext context.Context, options fantasy.PrepareStepFunctionOptions) (_ context.Context, prepared fantasy.PrepareStepResult, err error) {
			a.guard.ResetTurn()
			prepared.Messages = options.Messages
			for i := range prepared.Messages {
				prepared.Messages[i].ProviderOptions = nil
			}

			// Use latest tools (updated by SetTools when MCP tools change).
			prepared.Tools = a.tools.Copy()
			allToolNames := agentToolNames(prepared.Tools)
			selectedToolNames := append([]string(nil), allToolNames...)
			var omittedToolNames []string
			if discoveryRecorder, ok := unwrapToolDiscoveryRecorder(prepared.Tools); ok {
				selected := selectToolsForPreparedStep(callContext, prepared.Tools, discoveryRecorder, call.SessionID, call.TurnID)
				selectedToolNames = agentToolNames(selected)
				omittedToolNames = omittedAgentToolNames(allToolNames, selectedToolNames)
				prepared.Tools = selected
			}

			queuedCalls, _ := a.messageQueue.Get(call.SessionID)
			a.messageQueue.Del(call.SessionID)
			for _, queued := range queuedCalls {
				userMessage, createErr := a.createUserMessage(callContext, queued)
				if createErr != nil {
					return callContext, prepared, createErr
				}
				prepared.Messages = append(prepared.Messages, userMessage.ToAIMessage()...)
			}

			prepared.Messages = a.workaroundProviderMediaLimitations(prepared.Messages, largeModel)

			lastSystemRoleInx := 0
			systemMessageUpdated := false
			for i, msg := range prepared.Messages {
				// Only add cache control to the last message.
				if msg.Role == fantasy.MessageRoleSystem {
					lastSystemRoleInx = i
				} else if !systemMessageUpdated {
					prepared.Messages[lastSystemRoleInx].ProviderOptions = a.getCacheControlOptions()
					systemMessageUpdated = true
				}
				// Than add cache control to the last 2 messages.
				if i > len(prepared.Messages)-3 {
					prepared.Messages[i].ProviderOptions = a.getCacheControlOptions()
				}
			}

			if promptPrefix != "" {
				prepared.Messages = append([]fantasy.Message{fantasy.NewSystemMessage(promptPrefix)}, prepared.Messages...)
			}

			projectionID := ""
			projectedMessages, projection, buildErr := a.buildModelInput(callContext, ModelInputSnapshot{
				SessionID: call.SessionID,
				TurnID:    call.TurnID,
				Step:      stepNumber(prepared.Messages),
				Provider:  largeModel.ModelCfg.Provider,
				Model:     largeModel.ModelCfg.Model,
				Source:    "main_turn",
				Messages:  prepared.Messages,
			})
			if buildErr != nil {
				return callContext, prepared, buildErr
			}
			projectionID = projection.ProjectionID
			prepared.Messages = projectedMessages

			a.recordPromptAssembly(callContext, call, projectionID, largeModel, systemPrompt, systemSections, promptPrefix, mcpInstructions, prepared.Messages, prepared.Tools, selectedToolNames, omittedToolNames)

			sessionLock.Lock()
			stepMessages = cloneFantasyMessages(prepared.Messages)
			sessionLock.Unlock()

			var assistantMsg message.Message
			assistantMsg, err = a.messages.Create(callContext, call.SessionID, message.CreateMessageParams{
				Role:     message.Assistant,
				Parts:    []message.ContentPart{},
				Model:    largeModel.ModelCfg.Model,
				Provider: largeModel.ModelCfg.Provider,
			})
			if err != nil {
				return callContext, prepared, err
			}
			if err := a.markDeliveredToolResults(callContext, call.SessionID, prepared.Messages, stepNumber(prepared.Messages)); err != nil {
				return callContext, prepared, err
			}
			callContext = context.WithValue(callContext, tools.MessageIDContextKey, assistantMsg.ID)
			callContext = context.WithValue(callContext, tools.SupportsImagesContextKey, largeModel.CatwalkCfg.SupportsImages)
			callContext = context.WithValue(callContext, tools.ModelNameContextKey, largeModel.CatwalkCfg.Name)
			currentAssistant = &assistantMsg
			return callContext, prepared, err
		},
		OnReasoningStart: func(id string, reasoning fantasy.ReasoningContent) error {
			currentAssistant.AppendReasoningContent(reasoning.Text)
			return a.messages.Update(genCtx, *currentAssistant)
		},
		OnReasoningDelta: func(id string, text string) error {
			currentAssistant.AppendReasoningContent(text)
			return a.messages.Update(genCtx, *currentAssistant)
		},
		OnReasoningEnd: func(id string, reasoning fantasy.ReasoningContent) error {
			// handle anthropic signature
			if anthropicData, ok := reasoning.ProviderMetadata[anthropic.Name]; ok {
				if reasoning, ok := anthropicData.(*anthropic.ReasoningOptionMetadata); ok {
					currentAssistant.AppendReasoningSignature(reasoning.Signature)
				}
			}
			if googleData, ok := reasoning.ProviderMetadata[google.Name]; ok {
				if reasoning, ok := googleData.(*google.ReasoningMetadata); ok {
					currentAssistant.AppendThoughtSignature(reasoning.Signature, reasoning.ToolID)
				}
			}
			if openaiData, ok := reasoning.ProviderMetadata[openai.Name]; ok {
				if reasoning, ok := openaiData.(*openai.ResponsesReasoningMetadata); ok {
					currentAssistant.SetReasoningResponsesData(reasoning)
				}
			}
			currentAssistant.FinishThinking()
			return a.messages.Update(genCtx, *currentAssistant)
		},
		OnTextDelta: func(id string, text string) error {
			// Strip leading newline from initial text content. This is is
			// particularly important in non-interactive mode where leading
			// newlines are very visible.
			if len(currentAssistant.Parts) == 0 {
				text = strings.TrimPrefix(text, "\n")
			}

			currentAssistant.AppendContent(text)
			return a.messages.Update(genCtx, *currentAssistant)
		},
		OnToolInputStart: func(id string, toolName string) error {
			toolCall := message.ToolCall{
				ID:               id,
				Name:             toolName,
				ProviderExecuted: false,
				Finished:         false,
			}
			currentAssistant.AddToolCall(toolCall)
			// Use parent ctx instead of genCtx to ensure the update succeeds
			// even if the request is canceled mid-stream
			return a.messages.Update(ctx, *currentAssistant)
		},
		OnRetry: func(err *fantasy.ProviderError, delay time.Duration) {
			slog.Warn("Provider request failed, retrying", providerRetryLogFields(err, delay)...)
		},
		OnToolCall: func(tc fantasy.ToolCallContent) error {
			toolCall := message.ToolCall{
				ID:               tc.ToolCallID,
				Name:             tc.ToolName,
				Input:            tc.Input,
				ProviderExecuted: false,
				Finished:         true,
			}
			currentAssistant.AddToolCall(toolCall)
			// Use parent ctx instead of genCtx to ensure the update succeeds
			// even if the request is canceled mid-stream
			return a.messages.Update(ctx, *currentAssistant)
		},
		OnToolResult: func(result fantasy.ToolResultContent) error {
			toolResult := a.convertToToolResult(result)
			processed := a.guard.Process(toolResult)
			// Use parent ctx instead of genCtx to ensure the message is created
			// even if the request is canceled mid-stream
			_, createMsgErr := a.messages.Create(ctx, currentAssistant.SessionID, message.CreateMessageParams{
				Role: message.Tool,
				Parts: []message.ContentPart{
					processed,
				},
			})
			return createMsgErr
		},
		OnStepFinish: func(stepResult fantasy.StepResult) error {
			finishReason := message.FinishReasonUnknown
			switch stepResult.FinishReason {
			case fantasy.FinishReasonLength:
				finishReason = message.FinishReasonMaxTokens
			case fantasy.FinishReasonStop:
				finishReason = message.FinishReasonEndTurn
			case fantasy.FinishReasonToolCalls:
				finishReason = message.FinishReasonToolUse
			}
			// If a tool result halted the turn (e.g. a hook halt or a
			// permission denial), the step ends on FinishReasonToolCalls but
			// the model will not be called again. Treat it as the end of the
			// turn so the UI can render the assistant footer.
			if finishReason == message.FinishReasonToolUse {
				for _, tr := range stepResult.Content.ToolResults() {
					if tr.StopTurn {
						finishReason = message.FinishReasonEndTurn
						break
					}
				}
			}
			currentAssistant.AddFinish(finishReason, "", "")
			sessionLock.Lock()
			defer sessionLock.Unlock()

			updatedSession, getSessionErr := a.sessions.Get(ctx, call.SessionID)
			if getSessionErr != nil {
				return getSessionErr
			}
			usage, estimated := fallbackStepUsage(stepMessages, stepResult)
			a.updateSessionUsage(largeModel, &updatedSession, usage, a.openrouterCost(stepResult.ProviderMetadata), estimated)
			_, sessionErr := a.sessions.Save(ctx, updatedSession)
			if sessionErr != nil {
				return sessionErr
			}
			currentSession = updatedSession
			// Only real provider usage is persisted on the message: estimated
			// usage must never masquerade as a context metering anchor.
			if !estimated {
				currentAssistant.Usage = messageUsageFromFantasy(usage)
			}
			return a.messages.Update(genCtx, *currentAssistant)
		},
		StopWhen: []fantasy.StopCondition{
			func(steps []fantasy.StepResult) bool {
				return hasRepeatedToolCalls(steps, loopDetectionWindowSize, loopDetectionMaxRepeats)
			},
		},
	})

	a.eventPromptResponded(call.SessionID, time.Since(startTime).Truncate(time.Second))

	if err != nil {
		isHyper := largeModel.ModelCfg.Provider == hyper.Name
		isCancelErr := errors.Is(err, context.Canceled)
		if currentAssistant == nil {
			return result, err
		}
		// Ensure we finish thinking on error to close the reasoning state.
		currentAssistant.FinishThinking()
		toolCalls := currentAssistant.ToolCalls()
		// INFO: we use the parent context here because the genCtx has been cancelled.
		msgs, createErr := a.messages.List(ctx, currentAssistant.SessionID)
		if createErr != nil {
			return nil, createErr
		}
		for _, tc := range toolCalls {
			if !tc.Finished {
				tc.Finished = true
				tc.Input = "{}"
				currentAssistant.AddToolCall(tc)
				updateErr := a.messages.Update(ctx, *currentAssistant)
				if updateErr != nil {
					return nil, updateErr
				}
			}

			found := false
			for _, msg := range msgs {
				if msg.Role == message.Tool {
					for _, tr := range msg.ToolResults() {
						if tr.ToolCallID == tc.ID {
							found = true
							break
						}
					}
				}
				if found {
					break
				}
			}
			if found {
				continue
			}
			content := "There was an error while executing the tool"
			if isCancelErr {
				content = "Error: user cancelled assistant tool calling"
			}
			toolResult := message.ToolResult{
				ToolCallID: tc.ID,
				Name:       tc.Name,
				Content:    content,
				IsError:    true,
			}
			_, createErr = a.messages.Create(ctx, currentAssistant.SessionID, message.CreateMessageParams{
				Role: message.Tool,
				Parts: []message.ContentPart{
					toolResult,
				},
			})
			if createErr != nil {
				return nil, createErr
			}
		}
		var fantasyErr *fantasy.Error
		var providerErr *fantasy.ProviderError
		const defaultTitle = "Provider Error"
		if isCancelErr {
			currentAssistant.AddFinish(message.FinishReasonCanceled, "User canceled request", "")
		} else if isHyper && errors.As(err, &providerErr) && providerErr.StatusCode == http.StatusUnauthorized {
			currentAssistant.AddFinish(message.FinishReasonError, "Unauthorized", `Please re-authenticate with Hyper. You can also run "agent-builder auth" to re-authenticate.`)
			if a.notify != nil {
				a.notify.Publish(pubsub.CreatedEvent, notify.Notification{
					SessionID:    call.SessionID,
					SessionTitle: currentSession.Title,
					Type:         notify.TypeReAuthenticate,
					ProviderID:   largeModel.ModelCfg.Provider,
				})
			}
		} else if isHyper && errors.As(err, &providerErr) && providerErr.StatusCode == http.StatusPaymentRequired {
			url := hyper.BaseURL()
			currentAssistant.AddFinish(message.FinishReasonError, "No credits", "You're out of credits. Add more at "+url)
		} else if errors.As(err, &providerErr) {
			if providerErr.Message == "The requested model is not supported." {
				url := "https://github.com/settings/copilot/features"
				currentAssistant.AddFinish(
					message.FinishReasonError,
					"Copilot model not enabled",
					fmt.Sprintf("%q is not enabled in Copilot. Go to the following page to enable it. Then, wait 5 minutes before trying again. %s", largeModel.CatwalkCfg.Name, url),
				)
			} else {
				currentAssistant.AddFinish(message.FinishReasonError, cmp.Or(stringext.Capitalize(providerErr.Title), defaultTitle), providerErr.Message)
			}
		} else if errors.As(err, &fantasyErr) {
			currentAssistant.AddFinish(message.FinishReasonError, cmp.Or(stringext.Capitalize(fantasyErr.Title), defaultTitle), fantasyErr.Message)
		} else {
			currentAssistant.AddFinish(message.FinishReasonError, defaultTitle, err.Error())
		}
		// Note: we use the parent context here because the genCtx has been
		// cancelled.
		updateErr := a.messages.Update(ctx, *currentAssistant)
		if updateErr != nil {
			return nil, updateErr
		}
		return nil, err
	}

	// Release active request before publishing the notification so
	// subscribers see the final busy state at the moment of receipt.
	a.activeRequests.Del(call.SessionID)
	cancel()

	// Send notification that agent has finished its turn (skip for
	// nested/non-interactive sessions).
	if !call.NonInteractive && a.notify != nil {
		a.notify.Publish(pubsub.CreatedEvent, notify.Notification{
			SessionID:    call.SessionID,
			SessionTitle: currentSession.Title,
			Type:         notify.TypeAgentFinished,
		})
	}

	queuedMessages, ok := a.messageQueue.Get(call.SessionID)
	if !ok || len(queuedMessages) == 0 {
		return result, err
	}
	// There are queued messages restart the loop.
	firstQueuedMessage := queuedMessages[0]
	a.messageQueue.Set(call.SessionID, queuedMessages[1:])
	return a.Run(ctx, firstQueuedMessage)
}

func unwrapToolDiscoveryRecorder(agentTools []fantasy.AgentTool) (ToolDiscoveryRecorder, bool) {
	for _, tool := range agentTools {
		if searchTool, ok := tool.(*toolSearchTool); ok && searchTool.recorder != nil {
			return searchTool.recorder, true
		}
		if scheduled, ok := tool.(*schedulerTool); ok {
			if searchTool, ok := scheduled.inner.(*toolSearchTool); ok && searchTool.recorder != nil {
				return searchTool.recorder, true
			}
		}
	}
	return nil, false
}

type mcpInstructionSnapshot struct {
	Server  string
	Content string
}

func (a *sessionAgent) recordPromptAssembly(ctx context.Context, call SessionAgentCall, projectionID string, model Model, systemPrompt string, systemSections []PromptSectionSummary, promptPrefix string, mcpInstructions []mcpInstructionSnapshot, messages []fantasy.Message, selectedTools []fantasy.AgentTool, selectedToolNames, omittedToolNames []string) {
	if a.assemblyRecorder == nil {
		return
	}
	step := stepNumber(messages)
	sections := append([]PromptSectionSummary(nil), systemSections...)
	sections = appendRuntimePromptSections(sections, promptPrefix, mcpInstructions)
	systemSummary := PromptSystemSummary{
		Source:             "coder_prompt",
		Hash:               hashPromptText(systemPrompt),
		Length:             len(systemPrompt),
		TokenEstimate:      estimatePromptTokens(systemPrompt),
		PromptPrefix:       strings.TrimSpace(promptPrefix) != "",
		PromptPrefixHash:   hashPromptText(promptPrefix),
		PromptPrefixTokens: estimatePromptTokens(promptPrefix),
		SourceRefs:         []string{"runtime://system-prompt/coder"},
		Redacted:           true,
	}
	skillSummary := promptSkillSummaryFromSystem(systemPrompt)
	mcpInstructionText := strings.Join(mcpInstructionContents(mcpInstructions), "\n\n")
	mcpInstructionServers := mcpInstructionServerNames(mcpInstructions)
	mcpSummary := PromptMCPSummary{
		ServerCount:      len(mcpInstructionServers),
		InstructionCount: len(mcpInstructionServers),
		Servers:          append([]string(nil), mcpInstructionServers...),
		ServerListHash:   hashStringList(mcpInstructionServers),
		InstructionHash:  hashPromptText(mcpInstructionText),
		TokenEstimate:    estimatePromptTokens(mcpInstructionText),
		RawContentStored: false,
	}
	toolSummary := PromptToolSummary{
		Selected:      append([]string(nil), selectedToolNames...),
		Omitted:       append([]string(nil), omittedToolNames...),
		SelectedCount: len(selectedTools),
		OmittedCount:  len(omittedToolNames),
	}
	_ = a.assemblyRecorder.RecordPromptAssembly(ctx, PromptAssemblySnapshot{
		SessionID:    call.SessionID,
		TurnID:       call.TurnID,
		ProjectionID: projectionID,
		Step:         step,
		Provider:     model.ModelCfg.Provider,
		Model:        model.ModelCfg.Model,
		Sections:     sections,
		System:       systemSummary,
		Messages:     buildPromptMessageSummary(messages, call.Attachments),
		Tools:        toolSummary,
		Skills:       skillSummary,
		MCP:          mcpSummary,
		CreatedAt:    time.Now().UTC().UnixMilli(),
	})
}

func (a *sessionAgent) buildModelInput(ctx context.Context, snapshot ModelInputSnapshot) ([]fantasy.Message, ModelInputProjection, error) {
	if a.modelInputBuilder == nil {
		return snapshot.Messages, ModelInputProjection{Messages: snapshot.Messages}, nil
	}
	if strings.TrimSpace(snapshot.TurnID) == "" {
		snapshot.TurnID = "helper_" + strings.TrimSpace(snapshot.SessionID)
	}
	if snapshot.Step <= 0 {
		snapshot.Step = stepNumber(snapshot.Messages)
	}
	projection, err := a.modelInputBuilder.BuildModelInput(ctx, snapshot)
	if err != nil {
		return nil, ModelInputProjection{}, err
	}
	if projection.Messages == nil {
		projection.Messages = snapshot.Messages
	}
	return projection.Messages, projection, nil
}

func appendRuntimePromptSections(sections []PromptSectionSummary, promptPrefix string, mcpInstructions []mcpInstructionSnapshot) []PromptSectionSummary {
	out := make([]PromptSectionSummary, 0, len(sections)+1+len(mcpInstructions))
	if strings.TrimSpace(promptPrefix) != "" {
		out = append(out, PromptSectionSummary{
			ID:            "provider_prefix",
			Name:          "Provider prefix",
			Kind:          "provider_prefix",
			Role:          "system",
			Order:         len(out) + 1,
			CachePolicy:   "session_cached",
			Source:        "provider_config",
			SourceRefs:    []string{"config://provider/system_prompt_prefix"},
			Scope:         "provider",
			Hash:          hashPromptText(promptPrefix),
			Length:        len(promptPrefix),
			TokenEstimate: estimatePromptTokens(promptPrefix),
			Redacted:      true,
			RawStored:     false,
			Diagnostics:   "provider prefix is emitted as a separate system message before joined runtime prompt",
		})
	}
	for _, section := range sections {
		section.Order = len(out) + 1
		section.SourceRefs = append([]string(nil), section.SourceRefs...)
		out = append(out, section)
	}
	for _, instruction := range mcpInstructions {
		if strings.TrimSpace(instruction.Content) == "" {
			continue
		}
		out = append(out, PromptSectionSummary{
			ID:            "mcp_instructions_delta:" + instruction.Server,
			Name:          "MCP instructions: " + instruction.Server,
			Kind:          "mcp_instructions_delta",
			Role:          "system",
			Order:         len(out) + 1,
			CachePolicy:   "uncached",
			Source:        "mcp",
			SourceRefs:    []string{"mcp://" + instruction.Server + "/instructions"},
			Scope:         "mcp",
			Hash:          hashPromptText(instruction.Content),
			Length:        len(instruction.Content),
			TokenEstimate: estimatePromptTokens(instruction.Content),
			Redacted:      true,
			RawStored:     false,
			Diagnostics:   "reason=server_instructions; uncached because MCP servers can connect or disconnect between turns",
		})
	}
	return out
}

func mcpInstructionServerNames(instructions []mcpInstructionSnapshot) []string {
	names := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		if strings.TrimSpace(instruction.Content) != "" && strings.TrimSpace(instruction.Server) != "" {
			names = append(names, instruction.Server)
		}
	}
	slices.Sort(names)
	return names
}

func mcpInstructionContents(instructions []mcpInstructionSnapshot) []string {
	contents := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		if strings.TrimSpace(instruction.Content) != "" {
			contents = append(contents, instruction.Content)
		}
	}
	slices.Sort(contents)
	return contents
}

func hashStringList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	slices.Sort(clean)
	return hashPromptText(strings.Join(clean, "\n"))
}

func agentToolNames(agentTools []fantasy.AgentTool) []string {
	names := make([]string, 0, len(agentTools))
	for _, tool := range agentTools {
		name := strings.TrimSpace(tool.Info().Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func omittedAgentToolNames(allNames, selectedNames []string) []string {
	selected := make(map[string]struct{}, len(selectedNames))
	for _, name := range selectedNames {
		selected[name] = struct{}{}
	}
	var omitted []string
	for _, name := range allNames {
		if _, ok := selected[name]; !ok {
			omitted = append(omitted, name)
		}
	}
	return omitted
}

func promptSkillSummaryFromSystem(systemPrompt string) PromptSkillSummary {
	lower := strings.ToLower(systemPrompt)
	xmlPresent := strings.Contains(lower, "<skills") || strings.Contains(lower, "<skill")
	names := extractPromptSkillNames(systemPrompt)
	return PromptSkillSummary{
		AvailableCount:   len(names),
		LoadedCount:      len(names),
		Names:            names,
		LoadedNames:      append([]string(nil), names...),
		XMLPresent:       xmlPresent,
		XMLHash:          hashPromptText(skillXMLSlice(systemPrompt)),
		TokenEstimate:    estimatePromptTokens(skillXMLSlice(systemPrompt)),
		RawContentStored: false,
	}
}

func extractPromptSkillNames(systemPrompt string) []string {
	seen := map[string]struct{}{}
	var names []string
	addName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, marker := range []string{`name="`, `name='`} {
		rest := systemPrompt
		for {
			idx := strings.Index(rest, marker)
			if idx < 0 {
				break
			}
			rest = rest[idx+len(marker):]
			end := strings.IndexAny(rest, `"'`)
			if end <= 0 {
				break
			}
			addName(rest[:end])
			rest = rest[end+1:]
		}
	}
	rest := systemPrompt
	for {
		start := strings.Index(strings.ToLower(rest), "<name>")
		if start < 0 {
			break
		}
		rest = rest[start+len("<name>"):]
		end := strings.Index(strings.ToLower(rest), "</name>")
		if end < 0 {
			break
		}
		addName(rest[:end])
		rest = rest[end+len("</name>"):]
	}
	return names
}

func skillXMLSlice(systemPrompt string) string {
	lower := strings.ToLower(systemPrompt)
	start := strings.Index(lower, "<available_skills")
	closeTag := "</available_skills>"
	if start < 0 {
		start = strings.Index(lower, "<skills")
		closeTag = "</skills>"
	}
	if start < 0 {
		start = strings.Index(lower, "<skill")
		closeTag = "</skill>"
	}
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(lower, closeTag)
	if end >= start {
		return systemPrompt[start : end+len(closeTag)]
	}
	return systemPrompt[start:]
}

func selectToolsForPreparedStep(ctx context.Context, agentTools []fantasy.AgentTool, recorder ToolDiscoveryRecorder, sessionID, turnID string) []fantasy.AgentTool {
	metadata := make([]SchedulerToolMetadata, 0, len(agentTools))
	for _, tool := range agentTools {
		info := tool.Info()
		name := strings.TrimSpace(info.Name)
		if name == "" {
			continue
		}
		metadata = append(metadata, SchedulerToolMetadata{
			Name:            name,
			Source:          schedulerSourceForToolName(name),
			CapabilityID:    schedulerCapabilityIDForAgentTool(tool, name),
			Description:     info.Description,
			SchemaSummary:   schedulerSchemaSummary(info),
			SchemaDigest:    schedulerSchemaDigest(info),
			EstimatedTokens: schedulerToolInfoEstimatedTokens(info),
			Base:            isBaseRuntimeTool(name),
			SessionID:       sessionID,
			TurnID:          turnID,
		})
	}
	result, err := recorder.SelectToolsForTurn(ctx, SchedulerToolDisclosureRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		Tools:     metadata,
	})
	if err != nil || len(result.Selected) == 0 {
		return baseToolsForPreparedStep(agentTools)
	}
	selected := make(map[string]struct{}, len(result.Selected))
	for _, name := range result.Selected {
		selected[name] = struct{}{}
	}
	out := make([]fantasy.AgentTool, 0, len(result.Selected))
	for _, tool := range agentTools {
		if _, ok := selected[tool.Info().Name]; ok {
			out = append(out, tool)
		}
	}
	return out
}

func baseToolsForPreparedStep(agentTools []fantasy.AgentTool) []fantasy.AgentTool {
	out := make([]fantasy.AgentTool, 0, len(agentTools))
	for _, tool := range agentTools {
		if isBaseRuntimeTool(tool.Info().Name) {
			out = append(out, tool)
		}
	}
	return out
}

func (a *sessionAgent) Summarize(ctx context.Context, sessionID string, opts fantasy.ProviderOptions) error {
	if a.IsSessionBusy(sessionID) {
		return ErrSessionBusy
	}

	// Copy mutable fields under lock to avoid races with SetModels.
	largeModel := a.largeModel.Get()
	systemPromptPrefix := a.systemPromptPrefix.Get()

	currentSession, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	msgs, err := a.getSessionMessages(ctx, currentSession)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		// Nothing to summarize.
		return nil
	}

	aiMsgs, _ := a.preparePrompt(msgs, largeModel.CatwalkCfg.SupportsImages)

	genCtx, cancel := context.WithCancel(ctx)
	a.activeRequests.Set(sessionID, cancel)
	defer a.activeRequests.Del(sessionID)
	defer cancel()

	agent := fantasy.NewAgent(largeModel.Model,
		fantasy.WithSystemPrompt(string(summaryPrompt)),
		fantasy.WithUserAgent(userAgent),
	)
	summaryMessage, err := a.messages.Create(ctx, sessionID, message.CreateMessageParams{
		Role:             message.Assistant,
		Model:            largeModel.Model.Model(),
		Provider:         largeModel.Model.Provider(),
		IsSummaryMessage: true,
	})
	if err != nil {
		return err
	}

	summaryPromptText := buildSummaryPrompt(currentSession.Todos)

	resp, err := agent.Stream(genCtx, fantasy.AgentStreamCall{
		Prompt:          summaryPromptText,
		Messages:        aiMsgs,
		ProviderOptions: opts,
		PrepareStep: func(callContext context.Context, options fantasy.PrepareStepFunctionOptions) (_ context.Context, prepared fantasy.PrepareStepResult, err error) {
			prepared.Messages = options.Messages
			if systemPromptPrefix != "" {
				prepared.Messages = append([]fantasy.Message{fantasy.NewSystemMessage(systemPromptPrefix)}, prepared.Messages...)
			}
			prepared.Messages, _, err = a.buildModelInput(callContext, ModelInputSnapshot{
				SessionID: sessionID,
				TurnID:    "summary_" + sessionID,
				Step:      stepNumber(prepared.Messages),
				Provider:  largeModel.ModelCfg.Provider,
				Model:     largeModel.ModelCfg.Model,
				Source:    "summary",
				Messages:  prepared.Messages,
			})
			return callContext, prepared, err
		},
		OnReasoningDelta: func(id string, text string) error {
			summaryMessage.AppendReasoningContent(text)
			return a.messages.Update(genCtx, summaryMessage)
		},
		OnReasoningEnd: func(id string, reasoning fantasy.ReasoningContent) error {
			// Handle anthropic signature.
			if anthropicData, ok := reasoning.ProviderMetadata["anthropic"]; ok {
				if signature, ok := anthropicData.(*anthropic.ReasoningOptionMetadata); ok && signature.Signature != "" {
					summaryMessage.AppendReasoningSignature(signature.Signature)
				}
			}
			summaryMessage.FinishThinking()
			return a.messages.Update(genCtx, summaryMessage)
		},
		OnTextDelta: func(id, text string) error {
			summaryMessage.AppendContent(text)
			return a.messages.Update(genCtx, summaryMessage)
		},
	})
	if err != nil {
		isCancelErr := errors.Is(err, context.Canceled)
		if isCancelErr {
			// User cancelled summarize we need to remove the summary message.
			deleteErr := a.messages.Delete(ctx, summaryMessage.ID)
			return deleteErr
		}
		// Mark the summary message as finished with an error so the UI
		// stops spinning.
		summaryMessage.AddFinish(message.FinishReasonError, "Summarization Error", err.Error())
		if updateErr := a.messages.Update(ctx, summaryMessage); updateErr != nil {
			return updateErr
		}
		return err
	}

	summaryMessage.AddFinish(message.FinishReasonEndTurn, "", "")
	err = a.messages.Update(genCtx, summaryMessage)
	if err != nil {
		return err
	}

	var openrouterCost *float64
	for _, step := range resp.Steps {
		stepCost := a.openrouterCost(step.ProviderMetadata)
		if stepCost != nil {
			newCost := *stepCost
			if openrouterCost != nil {
				newCost += *openrouterCost
			}
			openrouterCost = &newCost
		}
	}

	a.updateSessionUsage(largeModel, &currentSession, resp.TotalUsage, openrouterCost, false)

	// Just in case, get just the last usage info.
	usage := resp.Response.Usage
	currentSession.SummaryMessageID = summaryMessage.ID
	currentSession.CompletionTokens = summaryCompletionTokens(usage, summaryMessage)
	currentSession.PromptTokens = 0
	_, err = a.sessions.Save(genCtx, currentSession)
	if err != nil {
		return err
	}

	// Release the active request before processing queued messages so that
	// Run() does not see the session as busy.
	a.activeRequests.Del(sessionID)
	cancel()

	// Process any messages that were queued while summarizing.
	queuedMessages, ok := a.messageQueue.Get(sessionID)
	if !ok || len(queuedMessages) == 0 {
		return nil
	}
	firstQueuedMessage := queuedMessages[0]
	a.messageQueue.Set(sessionID, queuedMessages[1:])
	_, qErr := a.Run(ctx, firstQueuedMessage)
	return qErr
}

func (a *sessionAgent) getCacheControlOptions() fantasy.ProviderOptions {
	if t, _ := strconv.ParseBool(os.Getenv("AGENT_BUILDER_DISABLE_ANTHROPIC_CACHE")); t {
		return fantasy.ProviderOptions{}
	}
	return fantasy.ProviderOptions{
		anthropic.Name: &anthropic.ProviderCacheControlOptions{
			CacheControl: anthropic.CacheControl{Type: "ephemeral"},
		},
		bedrock.Name: &anthropic.ProviderCacheControlOptions{
			CacheControl: anthropic.CacheControl{Type: "ephemeral"},
		},
		vercel.Name: &anthropic.ProviderCacheControlOptions{
			CacheControl: anthropic.CacheControl{Type: "ephemeral"},
		},
	}
}

func (a *sessionAgent) createUserMessage(ctx context.Context, call SessionAgentCall) (message.Message, error) {
	parts := []message.ContentPart{message.TextContent{Text: call.Prompt}}
	var attachmentParts []message.ContentPart
	for _, attachment := range call.Attachments {
		attachmentParts = append(attachmentParts, message.BinaryContent{Path: attachment.FilePath, MIMEType: attachment.MimeType, Data: attachment.Content})
	}
	parts = append(parts, attachmentParts...)
	msg, err := a.messages.Create(ctx, call.SessionID, message.CreateMessageParams{
		Role:     message.User,
		Parts:    parts,
		Metadata: cloneAgentStringMap(call.MessageMetadata),
	})
	if err != nil {
		return message.Message{}, fmt.Errorf("failed to create user message: %w", err)
	}
	return msg, nil
}

func cloneAgentStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (a *sessionAgent) preparePrompt(msgs []message.Message, supportsImages bool, attachments ...message.Attachment) ([]fantasy.Message, []fantasy.FilePart) {
	var history []fantasy.Message
	if !a.isSubAgent {
		history = append(history, fantasy.NewUserMessage(
			fmt.Sprintf("<system_reminder>%s</system_reminder>",
				`This is a reminder that your todo list is currently empty. DO NOT mention this to the user explicitly because they are already aware.
If you are working on tasks that would benefit from a todo list please use the "todos" tool to create one.
If not, please feel free to ignore. Again do not mention this message to the user.`,
			),
		))
	}
	hygiene, err := ApplyHistoryHygiene(msgs, HistoryHygieneOptions{SupportsImages: supportsImages})
	if err != nil {
		slog.Warn("Provider history hygiene failed", "error", err)
	}
	history = append(history, hygiene.Messages...)

	var files []fantasy.FilePart
	for _, attachment := range attachments {
		if attachment.IsText() {
			continue
		}
		files = append(files, fantasy.FilePart{
			Filename:  attachment.FileName,
			Data:      attachment.Content,
			MediaType: attachment.MimeType,
		})
	}

	return history, files
}

func stepNumber(messages []fantasy.Message) int {
	steps := 0
	for _, msg := range messages {
		if msg.Role == fantasy.MessageRoleAssistant {
			steps++
		}
	}
	if steps == 0 {
		return 1
	}
	return steps
}

func (a *sessionAgent) markDeliveredToolResults(ctx context.Context, sessionID string, prepared []fantasy.Message, deliveredAtStep int) error {
	deliveredIDs := toolResultIDsInPreparedMessages(prepared)
	if len(deliveredIDs) == 0 {
		return nil
	}
	msgs, err := a.messages.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		if msg.Role != message.Tool {
			continue
		}
		parts := make([]message.ContentPart, len(msg.Parts))
		copy(parts, msg.Parts)
		updated := false
		for i, part := range parts {
			result, ok := part.(message.ToolResult)
			if !ok {
				continue
			}
			if _, ok := deliveredIDs[result.ToolCallID]; !ok || result.DeliveredToModel {
				continue
			}
			result.DeliveredToModel = true
			result.DeliveredAtStep = deliveredAtStep
			result.DeliveryReason = "included_in_model_input"
			parts[i] = result
			updated = true
		}
		if !updated {
			continue
		}
		msg.Parts = parts
		if err := a.messages.Update(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func toolResultIDsInPreparedMessages(messages []fantasy.Message) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, msg := range messages {
		if msg.Role != fantasy.MessageRoleTool {
			continue
		}
		for _, part := range msg.Content {
			if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok && tr.ToolCallID != "" {
				ids[tr.ToolCallID] = struct{}{}
			}
		}
	}
	return ids
}

func (a *sessionAgent) getSessionMessages(ctx context.Context, session session.Session) ([]message.Message, error) {
	msgs, err := a.messages.List(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	if session.SummaryMessageID != "" {
		summaryMsgIndex := -1
		for i, msg := range msgs {
			if msg.ID == session.SummaryMessageID {
				summaryMsgIndex = i
				break
			}
		}
		if summaryMsgIndex != -1 {
			msgs = msgs[summaryMsgIndex:]
			msgs[0].Role = message.User
		}
	}
	return msgs, nil
}

// generateTitle generates a session titled based on the initial prompt.
func (a *sessionAgent) generateTitle(ctx context.Context, sessionID string, userPrompt string) {
	if userPrompt == "" {
		return
	}

	smallModel := a.smallModel.Get()
	largeModel := a.largeModel.Get()
	systemPromptPrefix := a.systemPromptPrefix.Get()

	var maxOutputTokens int64 = 40
	if smallModel.CatwalkCfg.CanReason {
		maxOutputTokens = smallModel.CatwalkCfg.DefaultMaxTokens
	}

	newAgent := func(m fantasy.LanguageModel, p []byte, tok int64) fantasy.Agent {
		return fantasy.NewAgent(m,
			fantasy.WithSystemPrompt(string(p)+"\n /no_think"),
			fantasy.WithMaxOutputTokens(tok),
			fantasy.WithUserAgent(userAgent),
		)
	}

	model := smallModel
	streamCall := fantasy.AgentStreamCall{
		Prompt: fmt.Sprintf("Generate a concise title for the following content:\n\n%s\n <think>\n\n</think>", userPrompt),
		PrepareStep: func(callCtx context.Context, opts fantasy.PrepareStepFunctionOptions) (_ context.Context, prepared fantasy.PrepareStepResult, err error) {
			prepared.Messages = opts.Messages
			if systemPromptPrefix != "" {
				prepared.Messages = append([]fantasy.Message{
					fantasy.NewSystemMessage(systemPromptPrefix),
				}, prepared.Messages...)
			}
			prepared.Messages, _, err = a.buildModelInput(callCtx, ModelInputSnapshot{
				SessionID: sessionID,
				TurnID:    "title_" + sessionID,
				Step:      stepNumber(prepared.Messages),
				Provider:  model.ModelCfg.Provider,
				Model:     model.ModelCfg.Model,
				Source:    "title",
				Messages:  prepared.Messages,
			})
			return callCtx, prepared, err
		},
	}

	// Use the small model to generate the title.
	agent := newAgent(model.Model, titlePrompt, maxOutputTokens)
	resp, err := agent.Stream(ctx, streamCall)
	if err == nil {
		// We successfully generated a title with the small model.
		slog.Debug("Generated title with small model")
	} else {
		// It didn't work. Let's try with the big model.
		slog.Error("Error generating title with small model; trying big model", "err", err)
		model = largeModel
		agent = newAgent(model.Model, titlePrompt, maxOutputTokens)
		resp, err = agent.Stream(ctx, streamCall)
		if err == nil {
			slog.Debug("Generated title with large model")
		} else {
			// Welp, the large model didn't work either. Use the default
			// session name and return.
			slog.Error("Error generating title with large model", "err", err)
			saveErr := a.sessions.Rename(ctx, sessionID, DefaultSessionName)
			if saveErr != nil {
				slog.Error("Failed to save session title", "error", saveErr)
			}
			return
		}
	}

	if resp == nil {
		// Actually, we didn't get a response so we can't. Use the default
		// session name and return.
		slog.Error("Response is nil; can't generate title")
		saveErr := a.sessions.Rename(ctx, sessionID, DefaultSessionName)
		if saveErr != nil {
			slog.Error("Failed to save session title", "error", saveErr)
		}
		return
	}

	// Clean up title.
	var title string
	title = strings.ReplaceAll(resp.Response.Content.Text(), "\n", " ")

	// Remove thinking tags if present.
	title = thinkTagRegex.ReplaceAllString(title, "")
	title = orphanThinkTagRegex.ReplaceAllString(title, "")

	title = strings.TrimSpace(title)
	title = cmp.Or(title, DefaultSessionName)

	// Calculate usage and cost.
	var openrouterCost *float64
	for _, step := range resp.Steps {
		stepCost := a.openrouterCost(step.ProviderMetadata)
		if stepCost != nil {
			newCost := *stepCost
			if openrouterCost != nil {
				newCost += *openrouterCost
			}
			openrouterCost = &newCost
		}
	}

	modelConfig := model.CatwalkCfg
	cost := modelConfig.CostPer1MInCached/1e6*float64(resp.TotalUsage.CacheCreationTokens) +
		modelConfig.CostPer1MOutCached/1e6*float64(resp.TotalUsage.CacheReadTokens) +
		modelConfig.CostPer1MIn/1e6*float64(resp.TotalUsage.InputTokens) +
		modelConfig.CostPer1MOut/1e6*float64(resp.TotalUsage.OutputTokens)

	// Use override cost if available (e.g., from OpenRouter).
	if openrouterCost != nil {
		cost = *openrouterCost
	}

	// Skip cost accumulation
	if model.FlatRate {
		cost = 0
	}

	promptTokens := resp.TotalUsage.InputTokens + resp.TotalUsage.CacheCreationTokens
	completionTokens := resp.TotalUsage.OutputTokens

	// Atomically update only title and usage fields to avoid overriding other
	// concurrent session updates.
	saveErr := a.sessions.UpdateTitleAndUsage(ctx, sessionID, title, promptTokens, completionTokens, cost)
	if saveErr != nil {
		slog.Error("Failed to save session title and usage", "error", saveErr)
		return
	}
}

func (a *sessionAgent) openrouterCost(metadata fantasy.ProviderMetadata) *float64 {
	openrouterMetadata, ok := metadata[openrouter.Name]
	if !ok {
		return nil
	}

	opts, ok := openrouterMetadata.(*openrouter.ProviderMetadata)
	if !ok {
		return nil
	}
	return &opts.Usage.Cost
}

func (a *sessionAgent) updateSessionUsage(model Model, session *session.Session, usage fantasy.Usage, overrideCost *float64, estimated bool) {
	modelConfig := model.CatwalkCfg
	cost := modelConfig.CostPer1MInCached/1e6*float64(usage.CacheCreationTokens) +
		modelConfig.CostPer1MOutCached/1e6*float64(usage.CacheReadTokens) +
		modelConfig.CostPer1MIn/1e6*float64(usage.InputTokens) +
		modelConfig.CostPer1MOut/1e6*float64(usage.OutputTokens)

	if !estimated {
		a.eventTokensUsed(session.ID, model, usage, cost)
	}

	if estimated {
		cost = 0
	} else {
		// Use override cost if available (e.g., from OpenRouter).
		if overrideCost != nil {
			cost = *overrideCost
		}

		// Skip cost accumulation
		if model.FlatRate {
			cost = 0
		}
	}

	session.Cost += cost
	updateSessionTokenCounters(session, usage)
}

func updateSessionTokenCounters(session *session.Session, usage fantasy.Usage) {
	if usage.OutputTokens != 0 {
		session.CompletionTokens = usage.OutputTokens
	}
	if promptTokens := usage.InputTokens + usage.CacheReadTokens; promptTokens != 0 {
		session.PromptTokens = promptTokens
	}
}

func summaryCompletionTokens(usage fantasy.Usage, summaryMessage message.Message) int64 {
	if usage.OutputTokens != 0 {
		return usage.OutputTokens
	}
	return approxTokenCount(summaryMessage.Content().Text) + approxTokenCount(summaryMessage.ReasoningContent().String())
}

func (a *sessionAgent) Cancel(sessionID string) {
	// Cancel regular requests. Don't use Take() here - we need the entry to
	// remain in activeRequests so IsBusy() returns true until the goroutine
	// fully completes (including error handling that may access the DB).
	// The defer in processRequest will clean up the entry.
	if cancel, ok := a.activeRequests.Get(sessionID); ok && cancel != nil {
		slog.Debug("Request cancellation initiated", "session_id", sessionID)
		cancel()
	}

	// Also check for summarize requests.
	if cancel, ok := a.activeRequests.Get(sessionID + "-summarize"); ok && cancel != nil {
		slog.Debug("Summarize cancellation initiated", "session_id", sessionID)
		cancel()
	}

	if a.QueuedPrompts(sessionID) > 0 {
		slog.Debug("Clearing queued prompts", "session_id", sessionID)
		a.messageQueue.Del(sessionID)
	}
}

func (a *sessionAgent) ClearQueue(sessionID string) {
	if a.QueuedPrompts(sessionID) > 0 {
		slog.Debug("Clearing queued prompts", "session_id", sessionID)
		a.messageQueue.Del(sessionID)
	}
}

func (a *sessionAgent) CancelAll() {
	if !a.IsBusy() {
		return
	}
	for key := range a.activeRequests.Seq2() {
		a.Cancel(key) // key is sessionID
	}

	timeout := time.After(5 * time.Second)
	for a.IsBusy() {
		select {
		case <-timeout:
			return
		default:
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func (a *sessionAgent) IsBusy() bool {
	var busy bool
	for cancelFunc := range a.activeRequests.Seq() {
		if cancelFunc != nil {
			busy = true
			break
		}
	}
	return busy
}

func (a *sessionAgent) IsSessionBusy(sessionID string) bool {
	_, busy := a.activeRequests.Get(sessionID)
	return busy
}

func (a *sessionAgent) QueuedPrompts(sessionID string) int {
	l, ok := a.messageQueue.Get(sessionID)
	if !ok {
		return 0
	}
	return len(l)
}

func (a *sessionAgent) QueuedPromptsList(sessionID string) []string {
	l, ok := a.messageQueue.Get(sessionID)
	if !ok {
		return nil
	}
	prompts := make([]string, len(l))
	for i, call := range l {
		prompts[i] = call.Prompt
	}
	return prompts
}

func (a *sessionAgent) SetModels(large Model, small Model) {
	a.largeModel.Set(large)
	a.smallModel.Set(small)
}

func (a *sessionAgent) SetTools(tools []fantasy.AgentTool) {
	a.tools.SetSlice(tools)
}

func (a *sessionAgent) SetSystemPrompt(systemPrompt string) {
	a.systemPrompt.Set(systemPrompt)
}

func (a *sessionAgent) SetSystemPromptSections(sections []PromptSectionSummary) {
	a.systemSections.SetSlice(sections)
}

func (a *sessionAgent) Model() Model {
	return a.largeModel.Get()
}

// convertToToolResult converts a fantasy tool result to a message tool result.
func (a *sessionAgent) convertToToolResult(result fantasy.ToolResultContent) message.ToolResult {
	baseResult := message.ToolResult{
		ToolCallID: result.ToolCallID,
		Name:       result.ToolName,
		Metadata:   result.ClientMetadata,
	}

	switch result.Result.GetType() {
	case fantasy.ToolResultContentTypeText:
		if r, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Result); ok {
			baseResult.Content = r.Text
		}
	case fantasy.ToolResultContentTypeError:
		if r, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Result); ok {
			baseResult.Content = r.Error.Error()
			baseResult.IsError = true
		}
	case fantasy.ToolResultContentTypeMedia:
		if r, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](result.Result); ok {
			if !stringext.IsValidBase64(r.Data) {
				slog.Warn("Tool returned media with invalid base64 data, discarding image",
					"tool", result.ToolName,
					"tool_call_id", result.ToolCallID,
				)
				baseResult.Content = "Tool returned image data with invalid encoding"
				baseResult.IsError = true
			} else {
				content := r.Text
				if content == "" {
					content = fmt.Sprintf("Loaded %s content", r.MediaType)
				}
				baseResult.Content = content
				baseResult.Data = r.Data
				baseResult.MIMEType = r.MediaType
			}
		}
	}

	return baseResult
}

// workaroundProviderMediaLimitations converts media content in tool results to
// user messages for providers that don't natively support images in tool results.
//
// Problem: OpenAI, Google, OpenRouter, and other OpenAI-compatible providers
// don't support sending images/media in tool result messages - they only accept
// text in tool results. However, they DO support images in user messages.
//
// If we send media in tool results to these providers, the API returns an error.
//
// Solution: For these providers, we:
//  1. Replace the media in the tool result with a text placeholder
//  2. Inject a user message immediately after with the image as a file attachment
//  3. This maintains the tool execution flow while working around API limitations
//
// Anthropic and Bedrock support images natively in tool results, so we skip
// this workaround for them.
//
// Example transformation:
//
//	BEFORE: [tool result: image data]
//	AFTER:  [tool result: "Image loaded - see attached"], [user: image attachment]
func (a *sessionAgent) workaroundProviderMediaLimitations(messages []fantasy.Message, largeModel Model) []fantasy.Message {
	providerSupportsMedia := largeModel.ModelCfg.Provider == string(catwalk.InferenceProviderAnthropic) ||
		largeModel.ModelCfg.Provider == string(catwalk.InferenceProviderBedrock)

	if providerSupportsMedia {
		return messages
	}

	convertedMessages := make([]fantasy.Message, 0, len(messages))

	for _, msg := range messages {
		if msg.Role != fantasy.MessageRoleTool {
			convertedMessages = append(convertedMessages, msg)
			continue
		}

		textParts := make([]fantasy.MessagePart, 0, len(msg.Content))
		var mediaFiles []fantasy.FilePart

		for _, part := range msg.Content {
			toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok {
				textParts = append(textParts, part)
				continue
			}

			if media, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](toolResult.Output); ok {
				decoded, err := base64.StdEncoding.DecodeString(media.Data)
				if err != nil {
					slog.Warn("Failed to decode media data", "error", err)
					textParts = append(textParts, part)
					continue
				}

				mediaFiles = append(mediaFiles, fantasy.FilePart{
					Data:      decoded,
					MediaType: media.MediaType,
					Filename:  fmt.Sprintf("tool-result-%s", toolResult.ToolCallID),
				})

				textParts = append(textParts, fantasy.ToolResultPart{
					ToolCallID: toolResult.ToolCallID,
					Output: fantasy.ToolResultOutputContentText{
						Text: "[Image/media content loaded - see attached file]",
					},
					ProviderOptions: toolResult.ProviderOptions,
				})
			} else {
				textParts = append(textParts, part)
			}
		}

		convertedMessages = append(convertedMessages, fantasy.Message{
			Role:    fantasy.MessageRoleTool,
			Content: textParts,
		})

		if len(mediaFiles) > 0 {
			convertedMessages = append(convertedMessages, fantasy.NewUserMessage(
				"Here is the media content from the tool result:",
				mediaFiles...,
			))
		}
	}

	return convertedMessages
}

// buildSummaryPrompt constructs the prompt text for session summarization.
func buildSummaryPrompt(todos []session.Todo) string {
	var sb strings.Builder
	sb.WriteString("Provide a detailed summary of our conversation above.")
	if len(todos) > 0 {
		sb.WriteString("\n\n## Current Todo List\n\n")
		for _, t := range todos {
			fmt.Fprintf(&sb, "- [%s] %s\n", t.Status, t.Content)
		}
		sb.WriteString("\nInclude these tasks and their statuses in your summary. ")
		sb.WriteString("Instruct the resuming assistant to use the `todos` tool to continue tracking progress on these tasks.")
	}
	return sb.String()
}

func providerRetryLogFields(err *fantasy.ProviderError, delay time.Duration) []any {
	fields := []any{
		"retry_delay", delay.String(),
	}
	if err == nil {
		return fields
	}
	fields = append(fields, "status_code", err.StatusCode)
	if err.Title != "" {
		fields = append(fields, "title", err.Title)
	}
	if err.Message != "" {
		fields = append(fields, "message", err.Message)
	}
	return fields
}
