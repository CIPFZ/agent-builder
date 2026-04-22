package runtime

import (
	"testing"

	"myclaw/internal/llm"
	"myclaw/internal/permissions"
	"myclaw/internal/session"
	"myclaw/internal/workspace"
)

func TestRunnerCompactionSnapshotUsesResolvedModelLimits(t *testing.T) {
	sessions := session.NewManager(nil)
	sess := sessions.GetOrCreateMain("main")
	if _, err := sessions.AppendMessage(sess.ID, "user", "hello"); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	runner := NewRunnerWithOptions(sessions, llm.NewMockClient(), workspace.NewLoader(""), nil, Options{
		PermissionPolicy: permissions.Policy{Mode: permissions.ModeDangerFullAccess},
		MainLoopModel:    "MiniMax-M2.7",
		ModelCatalog: llm.NewStaticModelCatalog([]llm.ModelDescriptor{
			{
				ID:                 "MiniMax-M2.7",
				ContextWindowTokens: 1000000,
				MaxOutputTokens:    32000,
			},
		}),
	})

	snapshot, err := runner.CompactionSnapshot(sess.ID)
	if err != nil {
		t.Fatalf("CompactionSnapshot: %v", err)
	}
	if snapshot.Analysis.ContextWindowTokens != 968000 {
		t.Fatalf("snapshot = %#v, want effective context window from model metadata", snapshot)
	}
	if snapshot.Analysis.AutoCompactThreshold != 955000 {
		t.Fatalf("snapshot = %#v, want Claude-style auto-compact threshold", snapshot)
	}
	if snapshot.Analysis.BlockingThreshold != 965000 {
		t.Fatalf("snapshot = %#v, want Claude-style blocking threshold", snapshot)
	}
}
