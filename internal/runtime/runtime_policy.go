package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/backend"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/runtimeapi"
)

var runtimePolicyModes = []string{
	string(permission.PolicyModeAsk),
	string(permission.PolicyModeAutoRead),
	string(permission.PolicyModePlan),
	string(permission.PolicyModeDenyAll),
}

type runtimePolicyFile struct {
	Mode      string `json:"mode"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

func defaultRuntimePolicy() RuntimePolicy {
	return RuntimePolicy{
		Mode:        string(permission.PolicyModeAsk),
		Modes:       append([]string(nil), runtimePolicyModes...),
		Description: runtimePolicyDescription(permission.PolicyModeAsk),
	}
}

func normalizeRuntimePolicyMode(mode string) (permission.PolicyMode, error) {
	normalized := permission.NormalizePolicyMode(permission.PolicyMode(strings.TrimSpace(mode)))
	if strings.TrimSpace(mode) != "" && normalized != permission.PolicyMode(strings.TrimSpace(mode)) {
		return "", fmt.Errorf("invalid policy mode: %s", mode)
	}
	return normalized, nil
}

func runtimePolicyDescription(mode permission.PolicyMode) string {
	switch permission.NormalizePolicyMode(mode) {
	case permission.PolicyModeAutoRead:
		return "Read-only tool calls are allowed; other tool calls request approval."
	case permission.PolicyModePlan:
		return "Read-only tool calls are allowed; mutating, execute, network, destructive, and secret tool calls are blocked."
	case permission.PolicyModeDenyAll:
		return "All tool calls are blocked."
	default:
		return "Tool calls request approval unless explicitly pre-approved by runtime policy."
	}
}

func runtimePolicyFromMode(mode permission.PolicyMode, updatedAt int64) RuntimePolicy {
	mode = permission.NormalizePolicyMode(mode)
	return RuntimePolicy{
		Mode:        string(mode),
		Modes:       append([]string(nil), runtimePolicyModes...),
		Description: runtimePolicyDescription(mode),
		UpdatedAt:   updatedAt,
	}
}

func loadRuntimePolicy(layout desktopLayout) (RuntimePolicy, error) {
	data, err := os.ReadFile(layout.PolicyConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return defaultRuntimePolicy(), nil
	}
	if err != nil {
		return RuntimePolicy{}, fmt.Errorf("failed to read runtime policy config: %w", err)
	}
	var file runtimePolicyFile
	if err := json.Unmarshal(data, &file); err != nil {
		return RuntimePolicy{}, fmt.Errorf("failed to parse runtime policy config: %w", err)
	}
	mode, err := normalizeRuntimePolicyMode(file.Mode)
	if err != nil {
		return RuntimePolicy{}, err
	}
	return runtimePolicyFromMode(mode, file.UpdatedAt), nil
}

func saveRuntimePolicy(layout desktopLayout, policy RuntimePolicy) error {
	if err := ensureDesktopLayout(layout); err != nil {
		return err
	}
	data, err := json.MarshalIndent(runtimePolicyFile{
		Mode:      policy.Mode,
		UpdatedAt: policy.UpdatedAt,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode runtime policy config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(layout.PolicyConfigPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write runtime policy config: %w", err)
	}
	return nil
}

func (r *runtimeService) applyPolicyToWorkspace(ctx context.Context, mode permission.PolicyMode) error {
	if r.runtime == nil || r.workspace == nil {
		return nil
	}
	ws, err := r.runtime.GetWorkspace(r.workspace.ID)
	if err != nil {
		return err
	}
	ws.Permissions.SetPolicyMode(mode)
	return nil
}

func (r *runtimeService) GetPolicy(ctx context.Context) (RuntimePolicyResponse, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	policy, err := loadRuntimePolicy(layout)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	r.mu.Lock()
	r.policy = policy
	r.mu.Unlock()
	return RuntimePolicyResponse{Policy: policy}, nil
}

func (r *runtimeService) UpdatePolicy(ctx context.Context, req RuntimePolicyUpdateRequest) (RuntimePolicyResponse, error) {
	mode, err := normalizeRuntimePolicyMode(req.Mode)
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	updated := runtimePolicyFromMode(mode, time.Now().UnixMilli())
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimePolicyResponse{}, err
	}
	if err := saveRuntimePolicy(layout, updated); err != nil {
		return RuntimePolicyResponse{}, err
	}
	r.mu.Lock()
	started := r.runtime != nil && r.workspace != nil
	r.mu.Unlock()
	if started {
		if err := r.applyPolicyToWorkspace(ctx, mode); err != nil {
			return RuntimePolicyResponse{}, err
		}
		if err := r.runtime.UpdateAgent(ctx, r.workspace.ID); err != nil && !errors.Is(err, backend.ErrAgentNotInitialized) {
			return RuntimePolicyResponse{}, err
		}
	}
	r.mu.Lock()
	r.policy = updated
	sessionID := r.sessionID
	r.mu.Unlock()
	r.storeRuntimeEvent(runtimeapi.Event{
		ID:        newRuntimeEventID(),
		Type:      runtimeapi.EventPermissionPolicyApplied,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Payload: map[string]any{
			"mode":        updated.Mode,
			"description": updated.Description,
			"decision":    "configure",
			"reason":      "Runtime policy mode updated.",
		},
	})
	if started {
		r.writeAudit(auditEntry{
			Event:            "permission_policy_applied",
			Timestamp:        time.Now().Format(time.RFC3339Nano),
			SessionID:        sessionID,
			PermissionPolicy: "configure",
			PolicyMode:       updated.Mode,
			PermissionReason: "Runtime policy mode updated.",
		})
	}
	return RuntimePolicyResponse{Policy: updated}, nil
}
