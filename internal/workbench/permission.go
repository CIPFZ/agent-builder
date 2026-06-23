package workbench

import (
	"github.com/CIPFZ/agent-builder/internal/apitypes"
	"github.com/CIPFZ/agent-builder/internal/permission"
)

// GrantPermission grants, denies, or persistently grants a permission
// request.
func (b *Service) GrantPermission(workspaceID string, req apitypes.PermissionGrant) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	perm := permission.PermissionRequest{
		ID:          req.Permission.ID,
		SessionID:   req.Permission.SessionID,
		TurnID:      req.Permission.TurnID,
		ToolCallID:  req.Permission.ToolCallID,
		ToolName:    req.Permission.ToolName,
		Description: req.Permission.Description,
		Action:      req.Permission.Action,
		Params:      req.Permission.Params,
		Path:        req.Permission.Path,
		Risk:        permission.Risk(req.Permission.Risk),
		Status:      req.Permission.Status,
		CreatedAt:   req.Permission.CreatedAt,
		DecidedAt:   req.Permission.DecidedAt,
	}

	switch req.Action {
	case apitypes.PermissionAllow:
		ws.Permissions.Grant(perm)
	case apitypes.PermissionAllowForSession:
		ws.Permissions.GrantPersistent(perm)
	case apitypes.PermissionDeny:
		ws.Permissions.Deny(perm)
	default:
		return ErrInvalidPermissionAction
	}
	return nil
}

// SetPermissionsSkip sets whether permission prompts are skipped.
func (b *Service) SetPermissionsSkip(workspaceID string, skip bool) error {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return err
	}

	ws.Permissions.SetSkipRequests(skip)
	return nil
}

// GetPermissionsSkip returns whether permission prompts are skipped.
func (b *Service) GetPermissionsSkip(workspaceID string) (bool, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return false, err
	}

	return ws.Permissions.SkipRequests(), nil
}
