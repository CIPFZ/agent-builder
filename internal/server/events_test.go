package server

import (
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/skills"
	"github.com/stretchr/testify/require"
)

// TestMessageToProtoToolResult ensures that ToolResult metadata,
// data, and MIME type survive the conversion to proto. Without these
// fields the TUI cannot render rich tool output (e.g. syntax-
// highlighted code from view, diffs from edit, images, etc.) and
// falls back to the raw LLM-facing string.
func TestMessageToProtoToolResult(t *testing.T) {
	t.Parallel()

	src := message.Message{
		ID:   "m1",
		Role: message.Tool,
		Parts: []message.ContentPart{
			message.ToolResult{
				ToolCallID:       "call-1",
				Name:             "view",
				Content:          "<file>\n  1| hi\n</file>",
				Data:             "base64data",
				MIMEType:         "image/png",
				Metadata:         `{"file_path":"/tmp/x","content":"hi"}`,
				IsError:          false,
				DeliveredToModel: true,
				DeliveredAtStep:  2,
				DeliveryReason:   "included_in_model_input",
				StoredPath:       ".crush/results/session/call-1.txt",
				OriginalSize:     64000,
				TruncatedBy:      "single",
			},
		},
	}

	got := messageToProto(src)
	require.Len(t, got.Parts, 1)
	tr, ok := got.Parts[0].(proto.ToolResult)
	require.True(t, ok, "expected proto.ToolResult, got %T", got.Parts[0])
	require.Equal(t, "call-1", tr.ToolCallID)
	require.Equal(t, "view", tr.Name)
	require.Equal(t, "<file>\n  1| hi\n</file>", tr.Content)
	require.Equal(t, "base64data", tr.Data)
	require.Equal(t, "image/png", tr.MIMEType)
	require.Equal(t, `{"file_path":"/tmp/x","content":"hi"}`, tr.Metadata)
	require.False(t, tr.IsError)
	require.True(t, tr.DeliveredToModel)
	require.Equal(t, 2, tr.DeliveredAtStep)
	require.Equal(t, "included_in_model_input", tr.DeliveryReason)
	require.Equal(t, ".crush/results/session/call-1.txt", tr.StoredPath)
	require.Equal(t, int64(64000), tr.OriginalSize)
	require.Equal(t, "single", tr.TruncatedBy)
}

func TestSkillsEventToProto(t *testing.T) {
	t.Parallel()

	got := skillsEventToProto(skills.Event{States: []*skills.SkillState{
		{Name: "ok", Path: "/skills/ok", State: skills.StateNormal},
		{Name: "bad", Path: "/skills/bad", State: skills.StateError, Err: errTestSkill},
	}})

	require.Len(t, got.States, 2)
	require.Equal(t, "ok", got.States[0].Name)
	require.Equal(t, proto.SkillStateNormal, got.States[0].State)
	require.Equal(t, "bad", got.States[1].Name)
	require.Equal(t, proto.SkillStateError, got.States[1].State)
	require.Equal(t, "skill error", got.States[1].Error)
}

var errTestSkill = testSkillError("skill error")

type testSkillError string

func (e testSkillError) Error() string { return string(e) }
