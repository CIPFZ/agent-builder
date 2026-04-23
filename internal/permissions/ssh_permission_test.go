package permissions_test

import (
	"testing"

	"myclaw/internal/permissions"
)

// TestSSHConservativePermissionSemantics verifies SSH cannot be auto-allowed
// by local workspace boundary in workspace-write/auto modes
func TestSSHConservativePermissionSemantics(t *testing.T) {
	tests := []struct {
		name   string
		mode   permissions.Mode
		workdir string
		roots  []string
	}{
		{
			name:    "workspace-write mode with workspace workdir",
			mode:    permissions.ModeWorkspaceWrite,
			workdir: "/workspace/project",
			roots:   []string{"/workspace"},
		},
		{
			name:    "auto mode with workspace workdir",
			mode:    permissions.ModeAuto,
			workdir: "/workspace/project",
			roots:   []string{"/workspace"},
		},
		{
			name:    "workspace-write mode outside workspace",
			mode:    permissions.ModeWorkspaceWrite,
			workdir: "/tmp",
			roots:   []string{"/workspace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := permissions.Policy{
				Mode:           tt.mode,
				WorkspaceRoots: tt.roots,
			}

			decision := policy.Evaluate(permissions.Request{
				ToolName: "SSH",
				Command:  "docker ps",
				WorkDir:  tt.workdir,
			})

			if !decision.RequiresApproval {
				t.Errorf("SSH should require approval in %s mode, got allowed=%v approval=%v",
					tt.mode, decision.Allowed, decision.RequiresApproval)
			}
			if decision.Category != permissions.CategoryApproval {
				t.Errorf("SSH should have approval category, got %v", decision.Category)
			}
			if decision.Allowed {
				t.Errorf("SSH should not be auto-allowed by workspace boundary, got allowed=true")
			}
		})
	}
}

// TestSSHDangerFullAccessStillAllows verifies SSH can be allowed in danger-full-access mode
func TestSSHDangerFullAccessStillAllows(t *testing.T) {
	policy := permissions.Policy{
		Mode: permissions.ModeDangerFullAccess,
	}

	decision := policy.Evaluate(permissions.Request{
		ToolName: "SSH",
		Command:  "ls",
		WorkDir:  "/workspace",
	})

	if !decision.Allowed {
		t.Errorf("SSH should be allowed in danger-full-access mode, got allowed=%v", decision.Allowed)
	}
	if decision.RequiresApproval {
		t.Errorf("SSH should not require approval in danger-full-access mode, got approval=%v", decision.RequiresApproval)
	}
}

// TestLocalShellToolsNotAffectedBySSHConservativeSemantics verifies
// Bash/PowerShell/system.run still follow workspace boundary semantics
func TestLocalShellToolsNotAffectedBySSHConservativeSemantics(t *testing.T) {
	policy := permissions.Policy{
		Mode:           permissions.ModeWorkspaceWrite,
		WorkspaceRoots: []string{"/workspace"},
	}

	localTools := []string{"Bash", "PowerShell", "system.run"}
	for _, toolName := range localTools {
		t.Run(toolName, func(t *testing.T) {
			decision := policy.Evaluate(permissions.Request{
				ToolName: toolName,
				Command:  "ls",
				WorkDir:  "/workspace/project",
			})

			if !decision.Allowed {
				t.Errorf("%s should be allowed inside workspace in workspace-write mode, got allowed=%v",
					toolName, decision.Allowed)
			}
			if decision.RequiresApproval {
				t.Errorf("%s should not require approval inside workspace, got approval=%v",
					toolName, decision.RequiresApproval)
			}
		})
	}
}
