package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
)

const openTargetSettingKey = "general.open_target"

func (r *runtimeService) OpenTargetSettings(ctx context.Context) (RuntimeOpenTargetSettingsResponse, error) {
	settings, err := loadRuntimeOpenTargetSettings(ctx)
	return RuntimeOpenTargetSettingsResponse{Settings: settings}, err
}

func (r *runtimeService) SaveOpenTargetSettings(ctx context.Context, req RuntimeOpenTargetSettings) (RuntimeOpenTargetSettingsResponse, error) {
	settings, err := loadRuntimeOpenTargetSettings(ctx)
	if err != nil {
		return RuntimeOpenTargetSettingsResponse{}, err
	}
	targetID := strings.TrimSpace(req.TargetID)
	if !runtimeOpenTargetAvailable(targetID, settings.Options) {
		return RuntimeOpenTargetSettingsResponse{}, fmt.Errorf("open target %q is not available", targetID)
	}
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimeOpenTargetSettingsResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	if _, err = conn.ExecContext(ctx, `INSERT INTO application_settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, openTargetSettingKey, targetID, time.Now().UnixMilli()); err != nil {
		return RuntimeOpenTargetSettingsResponse{}, fmt.Errorf("save open target: %w", err)
	}
	settings.TargetID = targetID
	return RuntimeOpenTargetSettingsResponse{Settings: settings}, nil
}

func loadRuntimeOpenTargetSettings(ctx context.Context) (RuntimeOpenTargetSettings, error) {
	settings := RuntimeOpenTargetSettings{TargetID: "system", Options: runtimeOpenTargetOptions()}
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimeOpenTargetSettings{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	var saved string
	err = conn.QueryRowContext(ctx, `SELECT value_json FROM application_settings WHERE key = ?`, openTargetSettingKey).Scan(&saved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RuntimeOpenTargetSettings{}, fmt.Errorf("load open target: %w", err)
	}
	if runtimeOpenTargetAvailable(saved, settings.Options) {
		settings.TargetID = saved
	}
	return settings, nil
}

func runtimeOpenTargetOptions() []RuntimeOpenTargetOption {
	options := []RuntimeOpenTargetOption{{ID: "system", Label: "系统文件管理器"}}
	for _, candidate := range []struct{ id, label string }{{"vscode", "Visual Studio Code"}, {"cursor", "Cursor"}, {"windsurf", "Windsurf"}} {
		if runtimeOpenTargetCommand(candidate.id) != "" {
			options = append(options, RuntimeOpenTargetOption{ID: candidate.id, Label: candidate.label})
		}
	}
	return options
}

func runtimeOpenTargetAvailable(id string, options []RuntimeOpenTargetOption) bool {
	for _, option := range options {
		if option.ID == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

func runtimeOpenPathInTarget(path, targetID string) error {
	if command := runtimeOpenTargetCommand(targetID); command != "" {
		return exec.Command(command, path).Start()
	}
	return runtimeOpenPathInFileManager(path)
}

func runtimeOpenTargetCommand(targetID string) string {
	candidates := map[string][]string{
		"vscode":   {"code", "code.cmd", runtimeTerminalJoinEnvPath("LocalAppData", "Programs", "Microsoft VS Code", "bin", "code.cmd"), runtimeTerminalJoinEnvPath("ProgramFiles", "Microsoft VS Code", "bin", "code.cmd")},
		"cursor":   {"cursor", "cursor.cmd", runtimeTerminalJoinEnvPath("LocalAppData", "Programs", "cursor", "resources", "app", "bin", "cursor.cmd")},
		"windsurf": {"windsurf", "windsurf.cmd", runtimeTerminalJoinEnvPath("LocalAppData", "Programs", "Windsurf", "bin", "windsurf.cmd")},
	}
	return runtimeTerminalLookPath(candidates[targetID]...)
}
