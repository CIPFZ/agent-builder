package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/config"
	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

var errRuntimeAgentRoleNotFound = errors.New("runtime agent role not found")

type runtimeAgentRoleStore struct {
	db *sql.DB
}

func newRuntimeAgentRoleStore(db *sql.DB) runtimeAgentRoleStore {
	return runtimeAgentRoleStore{db: db}
}

func (s runtimeAgentRoleStore) Upsert(ctx context.Context, role RuntimeAgentRoleDefinition) (RuntimeAgentRoleDefinition, error) {
	if s.db == nil {
		return RuntimeAgentRoleDefinition{}, errors.New("runtime agent role database is not available")
	}
	role = normalizeRuntimeAgentRole(role)
	allowedTools, err := encodeStringSlice(role.AllowedTools)
	if err != nil {
		return RuntimeAgentRoleDefinition{}, err
	}
	capabilityScope, err := encodeStringSlice(role.CapabilityScope)
	if err != nil {
		return RuntimeAgentRoleDefinition{}, err
	}
	policyMeta, err := encodeStringMap(role.PolicyMetadata)
	if err != nil {
		return RuntimeAgentRoleDefinition{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_agent_roles (
    id, name, title, description, prompt_summary, allowed_tools_json, capability_scope_json,
    model, provider, cwd, worktree, risk, policy_metadata_json, source, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    name = excluded.name,
    title = excluded.title,
    description = excluded.description,
    prompt_summary = excluded.prompt_summary,
    allowed_tools_json = excluded.allowed_tools_json,
    capability_scope_json = excluded.capability_scope_json,
    model = excluded.model,
    provider = excluded.provider,
    cwd = excluded.cwd,
    worktree = excluded.worktree,
    risk = excluded.risk,
    policy_metadata_json = excluded.policy_metadata_json,
    source = excluded.source,
    updated_at = excluded.updated_at`,
		role.ID,
		role.Name,
		nullableString(role.Title),
		nullableString(role.Description),
		nullableString(role.PromptSummary),
		allowedTools,
		capabilityScope,
		nullableString(role.Model),
		nullableString(role.Provider),
		nullableString(role.CWD),
		nullableString(role.Worktree),
		nullableString(role.Risk),
		policyMeta,
		nullableString(role.Source),
		role.CreatedAt,
		role.UpdatedAt,
	)
	if err != nil {
		return RuntimeAgentRoleDefinition{}, fmt.Errorf("failed to upsert runtime agent role: %w", err)
	}
	return s.Get(ctx, role.ID)
}

func (s runtimeAgentRoleStore) Get(ctx context.Context, id string) (RuntimeAgentRoleDefinition, error) {
	if s.db == nil {
		return RuntimeAgentRoleDefinition{}, errors.New("runtime agent role database is not available")
	}
	row := s.db.QueryRowContext(ctx, runtimeAgentRoleSelectSQL()+` WHERE id = ?`, strings.TrimSpace(id))
	role, err := scanRuntimeAgentRole(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeAgentRoleDefinition{}, errRuntimeAgentRoleNotFound
	}
	return role, err
}

func (s runtimeAgentRoleStore) List(ctx context.Context) ([]RuntimeAgentRoleDefinition, error) {
	if s.db == nil {
		return nil, errors.New("runtime agent role database is not available")
	}
	rows, err := s.db.QueryContext(ctx, runtimeAgentRoleSelectSQL()+` ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list runtime agent roles: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var roles []RuntimeAgentRoleDefinition
	for rows.Next() {
		role, err := scanRuntimeAgentRole(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func runtimeAgentRoleSelectSQL() string {
	return `
SELECT id, name, title, description, prompt_summary, allowed_tools_json, capability_scope_json,
    model, provider, cwd, worktree, risk, policy_metadata_json, source, created_at, updated_at
FROM runtime_agent_roles`
}

type runtimeAgentRoleScanner interface {
	Scan(dest ...any) error
}

func scanRuntimeAgentRole(scanner runtimeAgentRoleScanner) (RuntimeAgentRoleDefinition, error) {
	var role RuntimeAgentRoleDefinition
	var title, description, promptSummary, allowedTools, capabilityScope, model, provider, cwd, worktree, risk, policyMeta, source sql.NullString
	if err := scanner.Scan(
		&role.ID,
		&role.Name,
		&title,
		&description,
		&promptSummary,
		&allowedTools,
		&capabilityScope,
		&model,
		&provider,
		&cwd,
		&worktree,
		&risk,
		&policyMeta,
		&source,
		&role.CreatedAt,
		&role.UpdatedAt,
	); err != nil {
		return RuntimeAgentRoleDefinition{}, err
	}
	role.Title = title.String
	role.Description = description.String
	role.PromptSummary = promptSummary.String
	role.AllowedTools = decodeStringSlice(allowedTools.String)
	role.CapabilityScope = decodeStringSlice(capabilityScope.String)
	role.Model = model.String
	role.Provider = provider.String
	role.CWD = cwd.String
	role.Worktree = worktree.String
	role.Risk = risk.String
	role.PolicyMetadata = decodeStringMap(policyMeta.String)
	role.Source = source.String
	return role, nil
}

func normalizeRuntimeAgentRole(role RuntimeAgentRoleDefinition) RuntimeAgentRoleDefinition {
	role.ID = strings.TrimSpace(role.ID)
	role.Name = strings.TrimSpace(role.Name)
	if role.ID == "" {
		role.ID = role.Name
	}
	if role.Name == "" {
		role.Name = role.ID
	}
	if role.Title == "" {
		role.Title = role.Name
	}
	role.AllowedTools = normalizedStringSet(role.AllowedTools)
	role.CapabilityScope = normalizedStringSet(role.CapabilityScope)
	if role.PolicyMetadata == nil {
		role.PolicyMetadata = map[string]string{}
	}
	now := time.Now().UnixMilli()
	if role.CreatedAt == 0 {
		role.CreatedAt = now
	}
	role.UpdatedAt = now
	return role
}

func (r *runtimeService) ensureAgentRolesLoaded(ctx context.Context) error {
	r.mu.Lock()
	runtimeWorkbench := r.runtime
	workspace := r.workspace
	r.mu.Unlock()
	if runtimeWorkbench == nil || workspace == nil {
		return errors.New("runtime workspace is not started")
	}
	ws, err := runtimeWorkbench.GetWorkspace(workspace.ID)
	if err != nil {
		return err
	}
	dataDir := ws.Cfg.Config().Options.DataDirectory
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		return err
	}
	defer db.Release(dataDir) //nolint:errcheck
	store := newRuntimeAgentRoleStore(conn)
	roles := r.defaultAgentRoles()
	for _, role := range roles {
		desired := normalizeRuntimeAgentRole(role)
		existing, getErr := store.Get(ctx, desired.ID)
		if getErr == nil && runtimeAgentRolesSemanticallyEqual(existing, desired) {
			continue
		}
		if getErr != nil && !errors.Is(getErr, errRuntimeAgentRoleNotFound) {
			return getErr
		}
		if getErr == nil {
			desired.CreatedAt = existing.CreatedAt
		}
		stored, err := store.Upsert(ctx, desired)
		if err != nil {
			return err
		}
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventTaskRoleLoaded,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Payload: map[string]any{
				"role_id":       stored.ID,
				"name":          stored.Name,
				"source":        stored.Source,
				"allowed_tools": stored.AllowedTools,
				"summary":       stored.Title,
			},
		})
	}
	return nil
}

func runtimeAgentRolesSemanticallyEqual(left, right RuntimeAgentRoleDefinition) bool {
	return left.ID == right.ID &&
		left.Name == right.Name &&
		left.Title == right.Title &&
		left.Description == right.Description &&
		left.PromptSummary == right.PromptSummary &&
		slices.Equal(left.AllowedTools, right.AllowedTools) &&
		slices.Equal(left.CapabilityScope, right.CapabilityScope) &&
		left.Model == right.Model &&
		left.Provider == right.Provider &&
		left.CWD == right.CWD &&
		left.Worktree == right.Worktree &&
		left.Risk == right.Risk &&
		maps.Equal(left.PolicyMetadata, right.PolicyMetadata) &&
		left.Source == right.Source
}

func (r *runtimeService) defaultAgentRoles() []RuntimeAgentRoleDefinition {
	r.mu.Lock()
	workspace := r.workspace
	r.mu.Unlock()
	var roles []RuntimeAgentRoleDefinition
	if workspace == nil || workspace.Config == nil {
		return []RuntimeAgentRoleDefinition{defaultTaskAgentRole(nil, "")}
	}
	agentCfg := workspace.Config.Agents[config.AgentTask]
	roles = append(roles, defaultTaskAgentRole(&agentCfg, workspace.Path))
	return roles
}

func defaultTaskAgentRole(agentCfg *config.Agent, cwd string) RuntimeAgentRoleDefinition {
	allowed := []string{"view", "grep", "glob", "ls", "todos"}
	model := string(config.SelectedModelTypeSmall)
	description := "General subagent for scoped background analysis and implementation support."
	if agentCfg != nil {
		if len(agentCfg.AllowedTools) > 0 {
			allowed = append([]string(nil), agentCfg.AllowedTools...)
		}
		if agentCfg.Model != "" {
			model = string(agentCfg.Model)
		}
		if agentCfg.Description != "" {
			description = agentCfg.Description
		}
	}
	return RuntimeAgentRoleDefinition{
		ID:              config.AgentTask,
		Name:            config.AgentTask,
		Title:           "Task Agent",
		Description:     description,
		PromptSummary:   "Execute delegated work within the parent task scope and report structured progress/results.",
		AllowedTools:    allowed,
		CapabilityScope: []string{"read", "write", "execute", "network"},
		Model:           model,
		CWD:             cwd,
		Risk:            "execute",
		PolicyMetadata: map[string]string{
			"scope":  "inherits_parent_task",
			"source": "runtime_default",
		},
		Source: "runtime_default",
	}
}

func (r *runtimeService) AgentRoles(ctx context.Context) (RuntimeAgentRolesResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeAgentRolesResponse{}, err
	}
	dataDir := cfg.Config().Options.DataDirectory
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		return RuntimeAgentRolesResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	roles, err := newRuntimeAgentRoleStore(conn).List(ctx)
	if err != nil {
		return RuntimeAgentRolesResponse{}, err
	}
	return RuntimeAgentRolesResponse{Roles: roles}, nil
}

func (r *runtimeService) AgentRole(ctx context.Context, id string) (RuntimeAgentRoleResponse, error) {
	cfg, _, err := r.workspaceConfig(ctx)
	if err != nil {
		return RuntimeAgentRoleResponse{}, err
	}
	dataDir := cfg.Config().Options.DataDirectory
	conn, err := db.Connect(ctx, dataDir)
	if err != nil {
		return RuntimeAgentRoleResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	role, err := newRuntimeAgentRoleStore(conn).Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return RuntimeAgentRoleResponse{}, err
	}
	return RuntimeAgentRoleResponse{Role: role}, nil
}

func normalizedStringSet(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
