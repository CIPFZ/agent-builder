package workspace

import (
	"testing"

	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/proto"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/charmbracelet/crush/internal/skills"
	"github.com/stretchr/testify/require"
)

// TestProtoToMessageToolResult ensures that ToolResult metadata,
// data, and MIME type survive the conversion from proto on the
// client. Without these fields the TUI cannot render rich tool
// output (e.g. syntax-highlighted code from view, diffs from edit,
// images, etc.) and falls back to the raw LLM-facing string.
func TestProtoToMessageToolResult(t *testing.T) {
	t.Parallel()

	src := proto.Message{
		ID:   "m1",
		Role: proto.Tool,
		Parts: []proto.ContentPart{
			proto.ToolResult{
				ToolCallID: "call-1",
				Name:       "view",
				Content:    "<file>\n  1| hi\n</file>",
				Data:       "base64data",
				MIMEType:   "image/png",
				Metadata:   `{"file_path":"/tmp/x","content":"hi"}`,
				IsError:    false,
			},
		},
	}

	got := protoToMessage(src)
	require.Len(t, got.Parts, 1)
	tr, ok := got.Parts[0].(message.ToolResult)
	require.True(t, ok, "expected message.ToolResult, got %T", got.Parts[0])
	require.Equal(t, "call-1", tr.ToolCallID)
	require.Equal(t, "view", tr.Name)
	require.Equal(t, "<file>\n  1| hi\n</file>", tr.Content)
	require.Equal(t, "base64data", tr.Data)
	require.Equal(t, "image/png", tr.MIMEType)
	require.Equal(t, `{"file_path":"/tmp/x","content":"hi"}`, tr.Metadata)
	require.False(t, tr.IsError)
}

func TestProtoToSkillStates(t *testing.T) {
	t.Parallel()

	got := protoToSkillStates([]proto.SkillState{
		{Name: "ok", Path: "/skills/ok", State: proto.SkillStateNormal},
		{Name: "bad", Path: "/skills/bad", State: proto.SkillStateError, Error: "skill error"},
	})

	require.Len(t, got, 2)
	require.Equal(t, "ok", got[0].Name)
	require.Equal(t, skills.StateNormal, got[0].State)
	require.NoError(t, got[0].Err)
	require.Equal(t, "bad", got[1].Name)
	require.Equal(t, skills.StateError, got[1].State)
	require.EqualError(t, got[1].Err, "skill error")
}

func TestTranslateEventSkills(t *testing.T) {
	t.Parallel()

	ev := pubsub.Event[proto.SkillsEvent]{
		Type: pubsub.UpdatedEvent,
		Payload: proto.SkillsEvent{States: []proto.SkillState{
			{Name: "from-server", Path: "/skills/from-server", State: proto.SkillStateNormal},
		}},
	}

	out := translateEvent(ev)
	got, ok := out.(pubsub.Event[skills.Event])
	require.True(t, ok, "expected pubsub.Event[skills.Event], got %T", out)
	require.Equal(t, pubsub.UpdatedEvent, got.Type)
	require.Len(t, got.Payload.States, 1)
	require.Equal(t, "from-server", got.Payload.States[0].Name)
}
