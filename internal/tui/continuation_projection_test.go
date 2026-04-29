package tui

import "testing"

func TestClientStoreAppliesGatewayContinuationProjection(t *testing.T) {
	store := newClientStore()
	snapshot := store.applyContinuationProjection(map[string]any{
		"pending_approval": map[string]any{
			"id":          "approval-000001",
			"session_id":  "main-000001",
			"run_id":      "run-1",
			"tool_name":   "system.run",
			"tool_input":  "pwd",
			"status":      "pending",
			"reason":      "requires approval",
			"category":    "approval",
			"rule_source": "session",
		},
		"tasks": []any{
			map[string]any{
				"run_id":            "agent-000001",
				"label":             "research",
				"status":            "running",
				"parent_session_id": "main-000001",
				"child_session_id":  "child-000001",
				"child_session_key": "agent:main:child:agent-000001",
				"run_in_background": true,
				"isolation":         "worktree",
				"cwd":               "C:/repo/.worktrees/child",
				"permission_mode":   "ask",
			},
		},
	})

	if snapshot.Approval == nil || snapshot.Approval.ID != "approval-000001" {
		t.Fatalf("approval = %#v, want recovered approval", snapshot.Approval)
	}
	tasks := store.taskSnapshot()
	if tasks.RunningCount != 1 || len(tasks.Tasks) != 1 || tasks.Tasks[0].RunID != "agent-000001" {
		t.Fatalf("tasks = %#v, want recovered task snapshot", tasks)
	}
	if !tasks.Tasks[0].RunInBackground || tasks.Tasks[0].Isolation != "worktree" || tasks.Tasks[0].CWD != "C:/repo/.worktrees/child" || tasks.Tasks[0].PermissionMode != "ask" {
		t.Fatalf("task = %#v, want recovered isolation projection", tasks.Tasks[0])
	}
}
