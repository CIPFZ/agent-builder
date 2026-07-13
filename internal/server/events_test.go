package server

import (
	"testing"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/message"
	"github.com/CIPFZ/agent-builder/internal/skills"
	"github.com/stretchr/testify/require"
)

// TestMessageToAPITypeToolResult ensures that ToolResult metadata,
// data, and MIME type survive the conversion to apitypes. Without these
// fields the TUI cannot render rich tool output (e.g. syntax-
// highlighted code from view, diffs from edit, images, etc.) and
// falls back to the raw LLM-facing string.
func TestMessageToAPITypeToolResult(t *testing.T) {
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
				StoredPath:       "runtime://objects/tool-result-1",
				OriginalSize:     64000,
				TruncatedBy:      "single",
			},
		},
	}

	got := messageToAPIType(src)
	require.Len(t, got.Parts, 1)
	tr, ok := got.Parts[0].(apitypes.ToolResult)
	require.True(t, ok, "expected apitypes.ToolResult, got %T", got.Parts[0])
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
	require.Equal(t, "runtime://objects/tool-result-1", tr.StoredPath)
	require.Equal(t, int64(64000), tr.OriginalSize)
	require.Equal(t, "single", tr.TruncatedBy)
}

func TestSkillsEventToAPIType(t *testing.T) {
	t.Parallel()

	got := skillsEventToAPIType(skills.Event{States: []*skills.SkillState{
		{Name: "ok", Path: "/skills/ok", State: skills.StateNormal},
		{Name: "bad", Path: "/skills/bad", State: skills.StateError, Err: errTestSkill},
	}})

	require.Len(t, got.States, 2)
	require.Equal(t, "ok", got.States[0].Name)
	require.Equal(t, apitypes.SkillStateNormal, got.States[0].State)
	require.Equal(t, "bad", got.States[1].Name)
	require.Equal(t, apitypes.SkillStateError, got.States[1].State)
	require.Equal(t, "skill error", got.States[1].Error)
}

var errTestSkill = testSkillError("skill error")

type testSkillError string

func (e testSkillError) Error() string { return string(e) }
