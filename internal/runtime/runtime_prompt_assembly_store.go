package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type runtimePromptAssemblyStore struct {
	db *sql.DB
}

func newRuntimePromptAssemblyStore(db *sql.DB) runtimePromptAssemblyStore {
	return runtimePromptAssemblyStore{db: db}
}

func (s runtimePromptAssemblyStore) Upsert(ctx context.Context, assembly RuntimePromptAssembly) (RuntimePromptAssembly, error) {
	if s.db == nil {
		return RuntimePromptAssembly{}, errors.New("runtime prompt assembly database is not available")
	}
	assembly.ID = strings.TrimSpace(assembly.ID)
	assembly.SessionID = strings.TrimSpace(assembly.SessionID)
	assembly.TurnID = strings.TrimSpace(assembly.TurnID)
	if assembly.ID == "" {
		return RuntimePromptAssembly{}, errors.New("prompt assembly id is required")
	}
	if assembly.SessionID == "" {
		return RuntimePromptAssembly{}, errors.New("prompt assembly session id is required")
	}
	if assembly.TurnID == "" {
		return RuntimePromptAssembly{}, errors.New("prompt assembly turn id is required")
	}
	if assembly.Step <= 0 {
		assembly.Step = 1
	}
	if assembly.CreatedAt == 0 {
		assembly.CreatedAt = time.Now().UTC().UnixMilli()
	}
	systemJSON, err := marshalJSON(assembly.System)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	messagesJSON, err := marshalJSON(assembly.Messages)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	toolsJSON, err := marshalJSON(assembly.Tools)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	skillsJSON, err := marshalJSON(assembly.Skills)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	mcpJSON, err := marshalJSON(assembly.MCP)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	contextJSON, err := marshalJSON(assembly.ContextSources)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	compactJSON, err := marshalJSON(assembly.Compact)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	budgetJSON, err := marshalJSON(assembly.Budget)
	if err != nil {
		return RuntimePromptAssembly{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_prompt_assemblies (
    id, session_id, turn_id, projection_id, step, provider, model,
    system_json, messages_json, tools_json, skills_json, mcp_json,
    context_sources_json, compact_json, budget_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    session_id = excluded.session_id,
    turn_id = excluded.turn_id,
    projection_id = excluded.projection_id,
    step = excluded.step,
    provider = excluded.provider,
    model = excluded.model,
    system_json = excluded.system_json,
    messages_json = excluded.messages_json,
    tools_json = excluded.tools_json,
    skills_json = excluded.skills_json,
    mcp_json = excluded.mcp_json,
    context_sources_json = excluded.context_sources_json,
    compact_json = excluded.compact_json,
    budget_json = excluded.budget_json,
    created_at = excluded.created_at`,
		assembly.ID, assembly.SessionID, assembly.TurnID, nullableString(assembly.ProjectionID), assembly.Step,
		nullableString(assembly.Provider), nullableString(assembly.Model),
		systemJSON, messagesJSON, toolsJSON, skillsJSON, mcpJSON,
		contextJSON, compactJSON, budgetJSON, assembly.CreatedAt,
	)
	if err != nil {
		return RuntimePromptAssembly{}, fmt.Errorf("failed to upsert prompt assembly: %w", err)
	}
	return s.Get(ctx, assembly.ID)
}

func (s runtimePromptAssemblyStore) Get(ctx context.Context, id string) (RuntimePromptAssembly, error) {
	if s.db == nil {
		return RuntimePromptAssembly{}, errors.New("runtime prompt assembly database is not available")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, turn_id, projection_id, step, provider, model,
    system_json, messages_json, tools_json, skills_json, mcp_json,
    context_sources_json, compact_json, budget_json, created_at
FROM runtime_prompt_assemblies
WHERE id = ?`, strings.TrimSpace(id))
	return scanRuntimePromptAssembly(row)
}

func (s runtimePromptAssemblyStore) ListByTurn(ctx context.Context, turnID string) ([]RuntimePromptAssembly, error) {
	if s.db == nil {
		return nil, errors.New("runtime prompt assembly database is not available")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, step, provider, model,
    system_json, messages_json, tools_json, skills_json, mcp_json,
    context_sources_json, compact_json, budget_json, created_at
FROM runtime_prompt_assemblies
WHERE turn_id = ?
ORDER BY step ASC, created_at ASC`, strings.TrimSpace(turnID))
	if err != nil {
		return nil, fmt.Errorf("failed to list turn prompt assemblies: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	return scanRuntimePromptAssemblyRows(rows)
}

func (s runtimePromptAssemblyStore) ListBySession(ctx context.Context, sessionID string, limit int) ([]RuntimePromptAssembly, error) {
	if s.db == nil {
		return nil, errors.New("runtime prompt assembly database is not available")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, turn_id, projection_id, step, provider, model,
    system_json, messages_json, tools_json, skills_json, mcp_json,
    context_sources_json, compact_json, budget_json, created_at
FROM runtime_prompt_assemblies
WHERE session_id = ?
ORDER BY created_at DESC
LIMIT ?`, strings.TrimSpace(sessionID), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list session prompt assemblies: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	assemblies, err := scanRuntimePromptAssemblyRows(rows)
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(assemblies)-1; left < right; left, right = left+1, right-1 {
		assemblies[left], assemblies[right] = assemblies[right], assemblies[left]
	}
	return assemblies, nil
}

type runtimePromptAssemblyScanner interface {
	Scan(dest ...any) error
}

func scanRuntimePromptAssemblyRows(rows *sql.Rows) ([]RuntimePromptAssembly, error) {
	var out []RuntimePromptAssembly
	for rows.Next() {
		assembly, err := scanRuntimePromptAssembly(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, assembly)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate prompt assemblies: %w", err)
	}
	return out, nil
}

func scanRuntimePromptAssembly(scanner runtimePromptAssemblyScanner) (RuntimePromptAssembly, error) {
	var assembly RuntimePromptAssembly
	var projectionID, provider, model sql.NullString
	var systemJSON, messagesJSON, toolsJSON, skillsJSON, mcpJSON, contextJSON, compactJSON, budgetJSON sql.NullString
	if err := scanner.Scan(
		&assembly.ID, &assembly.SessionID, &assembly.TurnID, &projectionID, &assembly.Step, &provider, &model,
		&systemJSON, &messagesJSON, &toolsJSON, &skillsJSON, &mcpJSON,
		&contextJSON, &compactJSON, &budgetJSON, &assembly.CreatedAt,
	); err != nil {
		return RuntimePromptAssembly{}, err
	}
	assembly.ProjectionID = projectionID.String
	assembly.Provider = provider.String
	assembly.Model = model.String
	if err := decodePromptAssemblyJSON(systemJSON.String, &assembly.System); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(messagesJSON.String, &assembly.Messages); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(toolsJSON.String, &assembly.Tools); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(skillsJSON.String, &assembly.Skills); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(mcpJSON.String, &assembly.MCP); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(contextJSON.String, &assembly.ContextSources); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(compactJSON.String, &assembly.Compact); err != nil {
		return RuntimePromptAssembly{}, err
	}
	if err := decodePromptAssemblyJSON(budgetJSON.String, &assembly.Budget); err != nil {
		return RuntimePromptAssembly{}, err
	}
	return assembly, nil
}

func decodePromptAssemblyJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("failed to decode prompt assembly JSON: %w", err)
	}
	return nil
}
