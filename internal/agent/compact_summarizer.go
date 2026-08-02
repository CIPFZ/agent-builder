package agent

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/contextmgr"
	"github.com/CIPFZ/agent-builder/internal/message"
)

// CompactSummaryRequest is a single call to the model summarizer. It is a
// pure input/output boundary: no persistence happens inside the summarizer,
// so callers own where the resulting text is stored.
type CompactSummaryRequest struct {
	SessionID     string
	TurnID        string
	Messages      []message.Message
	Instructions  string
	UseSmallModel bool
}

// CompactSummaryResult carries the model's summary text after post-processing
// (analysis block stripped, whitespace trimmed).
type CompactSummaryResult struct {
	Summary string
}

// CompactSummarizer produces a compact summary of a session's transcript
// using the language model configured on this agent. It is deliberately
// separate from Summarize (the old session summary path): compact is called
// mid-turn from PrepareStep or from the manual /compact command and never
// creates DB messages.
type CompactSummarizer interface {
	GenerateCompactSummary(ctx context.Context, req CompactSummaryRequest) (CompactSummaryResult, error)
}

// compactSummaryTimeout is the hard deadline for a single model summary call.
// Longer than this and we abandon the model attempt so the caller can fall
// back to the heuristic summarizer.
const compactSummaryTimeout = 60 * time.Second

// compactMaxSummaryOutputTokens caps the summary size. Aligned with Claude
// Code's MAX_OUTPUT_TOKENS_FOR_SUMMARY (20k). If the selected model's own
// default max output is smaller we clamp down to that.
const compactMaxSummaryOutputTokens = 20_000

// compactMaxPTLRetries bounds how many times the summarizer will drop the
// oldest API round and retry when the model returns "prompt too long" on the
// summary request itself.
const compactMaxPTLRetries = 3

// analysisBlockPattern strips the <analysis>...</analysis> scratchpad the
// prompt asks the model to fill in before writing the summary. The
// scratchpad is high-cost, low-signal and would only bloat the reinjected
// context if we kept it.
var analysisBlockPattern = regexp.MustCompile(`(?is)<analysis>.*?</analysis>\s*`)

// summaryTagPattern lets the model output <summary>...</summary>; we
// remove the wrapping tags but keep the body.
var (
	summaryOpenPattern  = regexp.MustCompile(`(?i)<summary>\s*`)
	summaryClosePattern = regexp.MustCompile(`(?i)\s*</summary>`)
)

// GenerateCompactSummary implements CompactSummarizer on sessionAgent. The
// caller decides how to surface the returned text (persist it, feed it to a
// projection, etc.); this method has no side effects on the session.
func (a *sessionAgent) GenerateCompactSummary(ctx context.Context, req CompactSummaryRequest) (CompactSummaryResult, error) {
	if len(req.Messages) == 0 {
		return CompactSummaryResult{}, errors.New("compact summary requires at least one message")
	}
	model := a.largeModel.Get()
	if req.UseSmallModel {
		model = a.smallModel.Get()
	}
	if model.Model == nil {
		return CompactSummaryResult{}, errors.New("compact summary model is not configured")
	}

	callCtx, cancel := context.WithTimeout(ctx, compactSummaryTimeout)
	defer cancel()

	baseMessages, _ := a.preparePrompt(req.Messages, false /* strip images/documents */)
	baseMessages = stripImagesForCompactSummary(baseMessages)

	// Fixed system prompt for the summarizer. Following CC's leaner path we
	// send the 9-section instructions as the user prompt and keep the
	// system prompt minimal so the answer is guaranteed to be plain text.
	system := "You are a helpful AI assistant tasked with summarizing conversations. Follow the user's structure exactly. Respond with plain text only; do NOT call tools."

	// maxOutput: clamp to the model's own default; the plan requires
	// min(20k, model limit) so the summary never blocks on output-token
	// exhaustion.
	maxOutput := int64(compactMaxSummaryOutputTokens)
	if def := model.CatwalkCfg.DefaultMaxTokens; def > 0 && def < maxOutput {
		maxOutput = def
	}

	promptText := buildCompactSummaryPrompt(req.Instructions)

	// The summarizer runs on its own guarded turn id (compact_...). The
	// runtime skips compact guards for turn IDs with the compact_ prefix so
	// this call never triggers another compact check.
	summaryTurnID := "compact_" + strings.TrimSpace(req.SessionID)
	if req.TurnID != "" {
		summaryTurnID = "compact_" + strings.TrimSpace(req.TurnID)
	}

	agent := fantasy.NewAgent(model.Model,
		fantasy.WithSystemPrompt(system),
		fantasy.WithMaxOutputTokens(maxOutput),
		fantasy.WithUserAgent(userAgent),
	)

	messagesForCall := baseMessages
	var lastErr error
	for attempt := 0; attempt <= compactMaxPTLRetries; attempt++ {
		streamCall := fantasy.AgentStreamCall{
			Prompt:   promptText,
			Messages: messagesForCall,
			PrepareStep: func(callContext context.Context, options fantasy.PrepareStepFunctionOptions) (_ context.Context, prepared fantasy.PrepareStepResult, err error) {
				prepared.Messages = options.Messages
				// Route through modelInputBuilder so runtime guards see it
				// as a helper/summary call (prefix compact_ makes
				// isCompactGuardedTurn=true and skips its own compact
				// check).
				prepared.Messages, _, err = a.buildModelInput(callContext, ModelInputSnapshot{
					SessionID: req.SessionID,
					TurnID:    summaryTurnID,
					Step:      stepNumber(prepared.Messages),
					Provider:  model.ModelCfg.Provider,
					Model:     model.ModelCfg.Model,
					Source:    "compact",
					Messages:  prepared.Messages,
				})
				return callContext, prepared, err
			},
		}
		resp, err := agent.Stream(callCtx, streamCall)
		if err == nil && resp != nil {
			text := extractCompactSummaryText(resp)
			if strings.TrimSpace(text) == "" {
				return CompactSummaryResult{}, errors.New("compact summarizer returned empty text")
			}
			return CompactSummaryResult{Summary: text}, nil
		}
		lastErr = err
		if err != nil {
			text := err.Error()
			if !contextmgr.IsContextLengthError(text) {
				return CompactSummaryResult{}, err
			}
			// Prompt-too-long against the summary itself: drop the oldest
			// API round (assistant-anchored group) and retry, up to
			// compactMaxPTLRetries times.
			if attempt < compactMaxPTLRetries {
				trimmed := dropOldestSummaryGroup(messagesForCall)
				if len(trimmed) >= len(messagesForCall) {
					return CompactSummaryResult{}, err
				}
				messagesForCall = trimmed
				continue
			}
			return CompactSummaryResult{}, err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("compact summarizer produced no result")
	}
	return CompactSummaryResult{}, lastErr
}

// buildCompactSummaryPrompt returns the 9-section user prompt following
// Claude Code's structure (16 号文档 §3.4). NO-TOOLS front matter comes
// first so tool-happy models stay in the text lane; a NO-TOOLS trailer is
// appended for the same reason.
func buildCompactSummaryPrompt(instructions string) string {
	var b strings.Builder
	b.WriteString("CRITICAL: Respond with TEXT ONLY. Do NOT call any tools. This is a summarization request; tool calls will be REJECTED and will waste your only turn.\n\n")
	b.WriteString("Your job is to distill the conversation above into a compact summary the next assistant turn can use to continue the work without replaying every prior message.\n\n")
	b.WriteString("First, in a <analysis> block, silently work through the conversation message by message and note (a) the user's explicit requests, (b) decisions made, (c) filenames / code / signatures / errors mentioned, (d) any user corrections. This block is only for your own thinking; it will be stripped from the output.\n\n")
	b.WriteString("Then produce a summary with EXACTLY these nine sections, using the given markdown headings verbatim:\n\n")
	b.WriteString("1. Primary Request and Intent\n")
	b.WriteString("   - The user's overarching goal, in one paragraph.\n\n")
	b.WriteString("2. Key Technical Concepts\n")
	b.WriteString("   - Bullet list of the key concepts, libraries, or systems discussed.\n\n")
	b.WriteString("3. Files and Code Sections\n")
	b.WriteString("   - For each important file, list its path and (when relevant) the specific function / block and why it mattered. Include short code snippets or signatures inline when critical.\n\n")
	b.WriteString("4. Errors and Fixes\n")
	b.WriteString("   - Every error or failed attempt encountered and how it was fixed; call out user feedback that corrected the direction.\n\n")
	b.WriteString("5. Problem Solving\n")
	b.WriteString("   - The reasoning path taken to reach the current state.\n\n")
	b.WriteString("6. All User Messages\n")
	b.WriteString("   - List every non-tool-result user message in order. This prevents drift; keep the wording faithful.\n\n")
	b.WriteString("7. Pending Tasks\n")
	b.WriteString("   - Outstanding work items, including any that the user asked for but the assistant has not yet delivered.\n\n")
	b.WriteString("8. Current Work\n")
	b.WriteString("   - The immediate task the assistant was working on when the compaction happened.\n\n")
	b.WriteString("9. Optional Next Step\n")
	b.WriteString("   - The single most likely next action. It MUST correspond to the most recent explicit user request; quote the relevant user phrasing verbatim so the next turn cannot drift.\n")
	if trimmed := strings.TrimSpace(instructions); trimmed != "" {
		b.WriteString("\n## Additional Instructions\n")
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	b.WriteString("\nCRITICAL: The entire response above must be plain text; do NOT call any tools.\n")
	return b.String()
}

// extractCompactSummaryText pulls the final assistant text from a
// fantasy.AgentResult and strips the <analysis>...</analysis> scratchpad
// and any <summary>...</summary> wrapping.
func extractCompactSummaryText(resp *fantasy.AgentResult) string {
	if resp == nil {
		return ""
	}
	// Prefer Response.Content.Text() but fall back through steps if empty.
	text := strings.TrimSpace(resp.Response.Content.Text())
	if text == "" {
		for i := len(resp.Steps) - 1; i >= 0; i-- {
			if candidate := strings.TrimSpace(resp.Steps[i].Content.Text()); candidate != "" {
				text = candidate
				break
			}
		}
	}
	text = analysisBlockPattern.ReplaceAllString(text, "")
	text = summaryOpenPattern.ReplaceAllString(text, "")
	text = summaryClosePattern.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// stripImagesForCompactSummary replaces image and binary parts with textual
// placeholders. Long summary requests are almost always text-anchored and
// most images blow the input budget for zero gain.
func stripImagesForCompactSummary(messages []fantasy.Message) []fantasy.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		if len(msg.Content) == 0 {
			out = append(out, msg)
			continue
		}
		replaced := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			switch v := part.(type) {
			case fantasy.FilePart:
				kind := "document"
				if strings.HasPrefix(strings.ToLower(v.MediaType), "image/") {
					kind = "image"
				}
				replaced = append(replaced, fantasy.TextPart{Text: "[" + kind + " omitted for compact summary]"})
			case fantasy.ToolResultPart:
				if _, ok := v.Output.(fantasy.ToolResultOutputContentMedia); ok {
					replaced = append(replaced, fantasy.TextPart{Text: "[tool result media omitted for compact summary]"})
					continue
				}
				replaced = append(replaced, part)
			default:
				replaced = append(replaced, part)
			}
		}
		msg.Content = replaced
		out = append(out, msg)
	}
	return out
}

// dropOldestSummaryGroup removes the oldest API round from messages so the
// PTL retry sends a shorter transcript. Grouping is anchored on assistant
// messages: everything preceding the first assistant message becomes the
// first group; the group boundary is the next assistant message. Preserves
// invariants by preferring group-aligned drops.
func dropOldestSummaryGroup(messages []fantasy.Message) []fantasy.Message {
	if len(messages) <= 1 {
		return messages
	}
	// Find the first assistant message. Everything up to and including it
	// forms group #1; drop it.
	firstAssistant := -1
	for i, msg := range messages {
		if msg.Role == fantasy.MessageRoleAssistant {
			firstAssistant = i
			break
		}
	}
	if firstAssistant < 0 {
		// No assistant messages: nothing safe to trim. Drop the first
		// message to make forward progress but keep at least one.
		return messages[1:]
	}
	drop := firstAssistant + 1
	if drop >= len(messages) {
		// Trailing messages are all user/system; leave a single message.
		return messages[len(messages)-1:]
	}
	return messages[drop:]
}

// estimateCompactMessageRuneCount is used by tests to size test transcripts.
// It mirrors the simple rune-based estimator.
func estimateCompactMessageRuneCount(text string) int {
	return utf8.RuneCountInString(strings.TrimSpace(text))
}
