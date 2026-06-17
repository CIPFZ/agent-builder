package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"
)

const (
	runtimeInputModePrompt = "prompt"
	runtimeInputModeSlash  = "slash"
	runtimeInputModeShell  = "shell"
	runtimeInputModeVoice  = "voice"
	runtimeInputModeMeta   = "meta"

	runtimeInputItemText            = "text"
	runtimeInputItemImage           = "image"
	runtimeInputItemAudioTranscript = "audio_transcript"
	runtimeInputItemFileRef         = "file_ref"
	runtimeInputItemIDESelection    = "ide_selection"
	runtimeInputItemPastedText      = "pasted_text"
)

var knownRuntimeSlashCommands = map[string]string{
	"help":   "Show available runtime commands.",
	"status": "Show runtime status.",
}

func (r *runtimeService) SubmitUserInput(ctx context.Context, req RuntimeUserInputRequest) (RuntimeChatResponse, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeChatResponse{}, err
	}
	normalized, items, err := r.normalizeRuntimeUserInput(ctx, req, "")
	if err != nil {
		return RuntimeChatResponse{}, err
	}
	if !normalized.ShouldQuery {
		if r.userInputs.db != nil {
			stored, storeErr := r.userInputs.Upsert(ctx, normalized, items, "")
			if storeErr != nil {
				return RuntimeChatResponse{}, storeErr
			}
			normalized = stored
		}
		status, err := r.Status(ctx)
		if err != nil {
			return RuntimeChatResponse{}, err
		}
		return RuntimeChatResponse{
			RequestID:       normalized.ID,
			Status:          status,
			NormalizedInput: &normalized,
		}, nil
	}
	if err := r.ensureStarted(ctx); err != nil {
		if errors.Is(err, errSelectedModelMissing) {
			return RuntimeChatResponse{}, errSelectedModelMissing
		}
		return RuntimeChatResponse{}, err
	}
	return r.submitNormalizedInput(ctx, normalized, items)
}

func (r *runtimeService) UserInput(ctx context.Context, inputID string) (RuntimeNormalizedInput, error) {
	if err := r.ensureWorkspaceStarted(ctx, false); err != nil {
		return RuntimeNormalizedInput{}, err
	}
	inputID = strings.TrimSpace(inputID)
	if inputID == "" {
		return RuntimeNormalizedInput{}, errors.New("input id is required")
	}
	if r.userInputs.db == nil {
		return RuntimeNormalizedInput{}, errors.New("runtime user input database is not available")
	}
	input, err := r.userInputs.Get(ctx, inputID)
	if err != nil {
		return RuntimeNormalizedInput{}, fmt.Errorf("user input %s was not found: %w", inputID, err)
	}
	return input, nil
}

func (r *runtimeService) normalizeRuntimeUserInput(ctx context.Context, req RuntimeUserInputRequest, sessionID string) (RuntimeNormalizedInput, []RuntimeUserInputItem, error) {
	mode := normalizeRuntimeInputMode(req.Mode, req.Options)
	items := normalizeRuntimeUserInputItems(req.Items, mode)
	if len(items) == 0 {
		return RuntimeNormalizedInput{}, nil, errors.New("input items are required")
	}
	promptParts := make([]string, 0, len(items))
	itemTypes := make([]string, 0, len(items))
	attachments := make([]RuntimeAttachmentDraft, 0)
	attachmentIDs := make([]string, 0)
	metadata := map[string]string{}
	for index, item := range items {
		itemTypes = append(itemTypes, item.Type)
		switch item.Type {
		case runtimeInputItemText, runtimeInputItemPastedText, runtimeInputItemIDESelection:
			if text := strings.TrimSpace(item.Text); text != "" {
				promptParts = append(promptParts, text)
			}
		case runtimeInputItemAudioTranscript:
			if text := strings.TrimSpace(item.Text); text != "" {
				promptParts = append(promptParts, text)
			}
			if req.Options.VoiceSource != "" {
				metadata["voiceSource"] = req.Options.VoiceSource
			}
			for key, value := range item.Metadata {
				if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
					metadata["voice."+key] = value
				}
			}
		case runtimeInputItemImage:
			attachment, err := normalizeRuntimeImageAttachment(item, index)
			if err != nil {
				return RuntimeNormalizedInput{}, nil, err
			}
			attachments = append(attachments, attachment)
			attachmentIDs = append(attachmentIDs, attachment.ID)
		case runtimeInputItemFileRef:
			attachment := RuntimeAttachmentDraft{
				ID:         newRuntimeInputAttachmentID(),
				Type:       runtimeInputItemFileRef,
				MIMEType:   strings.TrimSpace(item.MIMEType),
				FileName:   strings.TrimSpace(item.FileName),
				SourcePath: strings.TrimSpace(item.SourcePath),
				Metadata:   cloneStringMap(item.Metadata),
			}
			attachments = append(attachments, attachment)
			attachmentIDs = append(attachmentIDs, attachment.ID)
			if text := strings.TrimSpace(item.Text); text != "" {
				promptParts = append(promptParts, text)
			}
		default:
			return RuntimeNormalizedInput{}, nil, fmt.Errorf("unsupported input item type %q", item.Type)
		}
	}
	prompt := strings.TrimSpace(strings.Join(promptParts, "\n\n"))
	command := runtimeInputCommandFor(mode, prompt, req.Options)
	shouldQuery := command == nil || command.ShouldQuery
	if mode == runtimeInputModeShell {
		metadata["shellMode"] = "agent_delegated"
	}
	if mode == runtimeInputModeMeta || req.Options.IsMeta {
		metadata["isMeta"] = "true"
	}
	if req.Options.BridgeOrigin {
		metadata["bridgeOrigin"] = "true"
	}
	if req.Options.ClientRequestID != "" {
		metadata["clientRequestId"] = req.Options.ClientRequestID
	}
	if prompt == "" && len(attachments) == 0 && shouldQuery {
		return RuntimeNormalizedInput{}, nil, errors.New("prompt text or attachment is required")
	}
	createdAt := time.Now().UnixMilli()
	normalized := RuntimeNormalizedInput{
		ID:          newRuntimeInputID(),
		SessionID:   strings.TrimSpace(sessionID),
		ProjectID:   strings.TrimSpace(req.ProjectID),
		Scope:       strings.TrimSpace(req.Scope),
		Mode:        mode,
		Prompt:      prompt,
		Attachments: attachments,
		ShouldQuery: shouldQuery,
		Command:     command,
		CreatedAt:   createdAt,
	}
	normalized.Messages = []RuntimeMessageDraft{{
		Role:          "user",
		Content:       prompt,
		Hidden:        mode == runtimeInputModeMeta || req.Options.IsMeta,
		Mode:          mode,
		ItemTypes:     itemTypes,
		Metadata:      metadata,
		AttachmentIDs: attachmentIDs,
	}}
	if command != nil && !command.ShouldQuery {
		normalized.Messages = append(normalized.Messages, RuntimeMessageDraft{
			Role:     "assistant",
			Content:  command.ResultText,
			Hidden:   false,
			Mode:     mode,
			Metadata: map[string]string{"command": command.Name, "strategy": command.Strategy},
		})
	}
	return normalized, items, nil
}

func normalizeRuntimeInputMode(mode string, options RuntimeUserInputOptions) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if options.IsMeta {
		return runtimeInputModeMeta
	}
	switch mode {
	case runtimeInputModeSlash, runtimeInputModeShell, runtimeInputModeVoice, runtimeInputModeMeta:
		return mode
	default:
		return runtimeInputModePrompt
	}
}

func normalizeRuntimeUserInputItems(items []RuntimeUserInputItem, mode string) []RuntimeUserInputItem {
	out := make([]RuntimeUserInputItem, 0, len(items))
	for _, item := range items {
		item.Type = strings.ToLower(strings.TrimSpace(item.Type))
		if item.Type == "" {
			item.Type = runtimeInputItemText
			if mode == runtimeInputModeVoice {
				item.Type = runtimeInputItemAudioTranscript
			}
		}
		item.Text = strings.TrimSpace(item.Text)
		item.MIMEType = strings.TrimSpace(item.MIMEType)
		item.FileName = strings.TrimSpace(item.FileName)
		item.SourcePath = strings.TrimSpace(item.SourcePath)
		item.Metadata = cloneStringMap(item.Metadata)
		out = append(out, item)
	}
	return out
}

func normalizeRuntimeImageAttachment(item RuntimeUserInputItem, index int) (RuntimeAttachmentDraft, error) {
	mimeType := strings.ToLower(strings.TrimSpace(item.MIMEType))
	if mimeType == "" && item.FileName != "" {
		mimeType = strings.TrimSpace(mime.TypeByExtension(filepath.Ext(item.FileName)))
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return RuntimeAttachmentDraft{}, fmt.Errorf("image input requires image MIME type, got %q", item.MIMEType)
	}
	size := 0
	if data := strings.TrimSpace(item.Data); data != "" {
		if decoded, err := base64.StdEncoding.DecodeString(data); err == nil {
			size = len(decoded)
		} else {
			size = len(data)
		}
	}
	metadata := cloneStringMap(item.Metadata)
	metadata["hasData"] = fmt.Sprintf("%t", strings.TrimSpace(item.Data) != "")
	metadata["itemIndex"] = fmt.Sprintf("%d", index)
	return RuntimeAttachmentDraft{
		ID:         newRuntimeInputAttachmentID(),
		Type:       runtimeInputItemImage,
		MIMEType:   mimeType,
		FileName:   item.FileName,
		SourcePath: item.SourcePath,
		Metadata:   metadata,
		SizeBytes:  size,
	}, nil
}

func runtimeInputCommandFor(mode, prompt string, options RuntimeUserInputOptions) *RuntimeInputCommand {
	if mode == runtimeInputModeShell {
		return &RuntimeInputCommand{
			Name:        "shell",
			Args:        prompt,
			Known:       true,
			Runtime:     true,
			ShouldQuery: true,
			Strategy:    "agent_delegated",
			Metadata:    map[string]string{"executeInReact": "false"},
		}
	}
	if mode != runtimeInputModeSlash || options.SkipSlashCommands {
		return nil
	}
	trimmed := strings.TrimSpace(prompt)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	name, args := parseRuntimeSlashCommand(trimmed)
	if name == "" {
		return &RuntimeInputCommand{
			Name:        "",
			Known:       false,
			Runtime:     true,
			ShouldQuery: false,
			ResultText:  "Commands are in the form `/command [args]`.",
			Strategy:    "invalid_slash",
		}
	}
	if description, ok := knownRuntimeSlashCommands[name]; ok {
		return &RuntimeInputCommand{
			Name:        name,
			Args:        args,
			Known:       true,
			Runtime:     true,
			ShouldQuery: false,
			ResultText:  description,
			Strategy:    "runtime_local",
		}
	}
	return &RuntimeInputCommand{
		Name:        name,
		Args:        args,
		Known:       false,
		Runtime:     true,
		ShouldQuery: false,
		ResultText:  fmt.Sprintf("Unknown runtime command: /%s", name),
		Strategy:    "unknown_slash_no_query",
	}
}

func parseRuntimeSlashCommand(input string) (string, string) {
	input = strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if input == "" {
		return "", ""
	}
	name, args, _ := strings.Cut(input, " ")
	name = strings.TrimSpace(name)
	if strings.ContainsAny(name, `/\`) {
		return "", ""
	}
	return strings.ToLower(name), strings.TrimSpace(args)
}

func newRuntimeInputID() string {
	return "input_" + newRequestID()
}

func newRuntimeInputAttachmentID() string {
	return "att_" + newRequestID()
}
