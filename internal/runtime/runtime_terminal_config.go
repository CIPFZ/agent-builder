package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
)

const terminalProfileSettingKey = "terminal.selected_profile_id"

func (r *runtimeService) TerminalSettings(ctx context.Context) (RuntimeTerminalSettingsResponse, error) {
	settings, err := r.loadRuntimeTerminalSettings(ctx)
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	return RuntimeTerminalSettingsResponse{Settings: settings}, nil
}

func (r *runtimeService) SaveTerminalSettings(ctx context.Context, req RuntimeTerminalSettings) (RuntimeTerminalSettingsResponse, error) {
	profileID := strings.TrimSpace(req.ProfileID)
	profiles := runtimeTerminalAvailableProfiles()
	if profileID == "" {
		profileID = runtimeTerminalDefaultProfileID(profiles)
	}
	if len(profiles) == 0 {
		return RuntimeTerminalSettingsResponse{}, errors.New("no terminal profiles are available on this machine")
	}
	if !runtimeTerminalProfileAvailable(profileID, profiles) {
		return RuntimeTerminalSettingsResponse{}, fmt.Errorf("terminal profile %q is not available on this machine", profileID)
	}
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	settings := RuntimeTerminalSettings{
		ProfileID: profileID,
		Profiles:  profiles,
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO application_settings (key, value_json, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, terminalProfileSettingKey, profileID, time.Now().UnixMilli()); err != nil {
		return RuntimeTerminalSettingsResponse{}, fmt.Errorf("save terminal profile setting: %w", err)
	}
	return RuntimeTerminalSettingsResponse{Settings: settings}, nil
}

func (r *runtimeService) loadRuntimeTerminalSettings(ctx context.Context) (RuntimeTerminalSettings, error) {
	profiles := runtimeTerminalAvailableProfiles()
	settings := RuntimeTerminalSettings{
		ProfileID: runtimeTerminalDefaultProfileID(profiles),
		Profiles:  profiles,
	}
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimeTerminalSettings{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck
	var savedProfileID string
	err = conn.QueryRowContext(ctx, `SELECT value_json FROM application_settings WHERE key = ?`, terminalProfileSettingKey).Scan(&savedProfileID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RuntimeTerminalSettings{}, fmt.Errorf("load terminal profile setting: %w", err)
	}
	if runtimeTerminalProfileAvailable(savedProfileID, profiles) {
		settings.ProfileID = savedProfileID
	}
	return settings, nil
}

func runtimeTerminalDefaultProfileID(profiles []RuntimeTerminalProfile) string {
	preferred := []string{"bash", "zsh", "fish", "sh", "pwsh"}
	switch runtime.GOOS {
	case "windows":
		preferred = []string{"git-bash", "pwsh", "powershell", "cmd"}
	case "darwin":
		preferred = []string{"zsh", "bash", "fish", "sh", "pwsh"}
	}
	for _, id := range preferred {
		if runtimeTerminalProfileAvailable(id, profiles) {
			return id
		}
	}
	if len(profiles) > 0 {
		return profiles[0].ID
	}
	return ""
}

func runtimeTerminalProfileAvailable(profileID string, profiles []RuntimeTerminalProfile) bool {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return false
	}
	for _, profile := range profiles {
		if profile.ID == profileID {
			return true
		}
	}
	return false
}
