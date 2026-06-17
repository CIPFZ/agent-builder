package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestRuntimeNormalizePromptInput(t *testing.T) {
	service := newRuntimeService()
	normalized, _, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModePrompt,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: " hello runtime ",
		}},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Prompt != "hello runtime" || !normalized.ShouldQuery || normalized.Mode != runtimeInputModePrompt {
		t.Fatalf("normalized = %#v", normalized)
	}
	if len(normalized.Messages) != 1 || normalized.Messages[0].Hidden || normalized.Messages[0].Content != "hello runtime" {
		t.Fatalf("messages = %#v", normalized.Messages)
	}
}

func TestRuntimeNormalizeImageInputAttachmentEvidence(t *testing.T) {
	service := newRuntimeService()
	normalized, items, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModePrompt,
		Items: []RuntimeUserInputItem{
			{Type: runtimeInputItemText, Text: "describe this"},
			{
				Type:       runtimeInputItemImage,
				Data:       "aGVsbG8=",
				MIMEType:   "image/png",
				FileName:   "paste.png",
				SourcePath: "C:\\tmp\\paste.png",
				Metadata:   map[string]string{"width": "64", "height": "32"},
			},
		},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if len(normalized.Attachments) != 1 {
		t.Fatalf("attachments = %#v", normalized.Attachments)
	}
	attachment := normalized.Attachments[0]
	if attachment.MIMEType != "image/png" || attachment.FileName != "paste.png" || attachment.SourcePath == "" || attachment.SizeBytes != 5 {
		t.Fatalf("attachment = %#v", attachment)
	}
	if attachment.Metadata["width"] != "64" || attachment.Metadata["hasData"] != "true" {
		t.Fatalf("attachment metadata = %#v", attachment.Metadata)
	}
}

func TestRuntimeNormalizeVoiceTranscript(t *testing.T) {
	service := newRuntimeService()
	normalized, _, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModeVoice,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemAudioTranscript,
			Text: "summarize the current file",
			Metadata: map[string]string{
				"durationMs": "1200",
			},
		}},
		Options: RuntimeUserInputOptions{VoiceSource: "microphone"},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Prompt != "summarize the current file" || normalized.Mode != runtimeInputModeVoice || !normalized.ShouldQuery {
		t.Fatalf("normalized = %#v", normalized)
	}
	if normalized.Messages[0].Metadata["voiceSource"] != "microphone" || normalized.Messages[0].Metadata["voice.durationMs"] != "1200" {
		t.Fatalf("voice metadata = %#v", normalized.Messages[0].Metadata)
	}
}

func TestRuntimeNormalizeKnownSlashCommandDoesNotQuery(t *testing.T) {
	service := newRuntimeService()
	normalized, _, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModeSlash,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: "/status",
		}},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ShouldQuery || normalized.Command == nil || !normalized.Command.Known || normalized.Command.Name != "status" {
		t.Fatalf("normalized command = %#v", normalized.Command)
	}
	if len(normalized.Messages) < 2 || normalized.Messages[1].Role != "assistant" || !strings.Contains(normalized.Messages[1].Content, "runtime status") {
		t.Fatalf("command messages = %#v", normalized.Messages)
	}
}

func TestRuntimeSubmitLocalSlashCommandDoesNotRequireConfiguredModel(t *testing.T) {
	root := runtimeDevTestRoot(t, "phase02-local-slash-no-model")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	service := newRuntimeService()
	response, err := service.SubmitUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModeSlash,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: "/status",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.TurnID != "" {
		t.Fatalf("local slash command should not create a turn, got %q", response.TurnID)
	}
	if response.NormalizedInput == nil || response.NormalizedInput.ShouldQuery || response.NormalizedInput.Command == nil {
		t.Fatalf("normalized input = %#v", response.NormalizedInput)
	}
	if response.Status.Ready {
		t.Fatalf("status ready = true, want false without configured model")
	}
	stored, err := service.UserInput(context.Background(), response.NormalizedInput.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != response.NormalizedInput.ID || stored.Command == nil || stored.Command.Name != "status" {
		t.Fatalf("stored input = %#v", stored)
	}
}

func TestRuntimeSubmitPromptStillRequiresConfiguredModel(t *testing.T) {
	root := runtimeDevTestRoot(t, "phase02-prompt-no-model")
	t.Setenv("AGENT_BUILDER_DESKTOP_ROOT", root)

	service := newRuntimeService()
	_, err := service.SubmitUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModePrompt,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: "hello",
		}},
	})
	if err == nil || err != errSelectedModelMissing {
		t.Fatalf("err = %v, want errSelectedModelMissing", err)
	}
}

func TestRuntimeNormalizeUnknownSlashCommandDoesNotQuery(t *testing.T) {
	service := newRuntimeService()
	normalized, _, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModeSlash,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: "/does-not-exist arg",
		}},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ShouldQuery || normalized.Command == nil || normalized.Command.Known || normalized.Command.Strategy != "unknown_slash_no_query" {
		t.Fatalf("unknown command = %#v", normalized.Command)
	}
	if !strings.Contains(normalized.Command.ResultText, "Unknown runtime command") {
		t.Fatalf("unknown command text = %q", normalized.Command.ResultText)
	}
}

func TestRuntimeNormalizeMetaPromptIsHidden(t *testing.T) {
	service := newRuntimeService()
	normalized, _, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModeMeta,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: "hidden scheduler prompt",
		}},
		Options: RuntimeUserInputOptions{IsMeta: true},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Mode != runtimeInputModeMeta || !normalized.Messages[0].Hidden || normalized.Messages[0].Metadata["isMeta"] != "true" {
		t.Fatalf("meta normalized = %#v", normalized)
	}
}

func TestRuntimeNormalizeShellModeIsAgentDelegated(t *testing.T) {
	service := newRuntimeService()
	normalized, _, err := service.normalizeRuntimeUserInput(context.Background(), RuntimeUserInputRequest{
		Mode: runtimeInputModeShell,
		Items: []RuntimeUserInputItem{{
			Type: runtimeInputItemText,
			Text: "go test ./internal/runtime",
		}},
	}, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.ShouldQuery || normalized.Command == nil || normalized.Command.Strategy != "agent_delegated" {
		t.Fatalf("shell normalized = %#v", normalized)
	}
	if normalized.Command.Metadata["executeInReact"] != "false" {
		t.Fatalf("shell metadata = %#v", normalized.Command.Metadata)
	}
}
