package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const runtimeTerminalDefaultProfileID = "git-bash"

var runtimeTerminalProfiles = []RuntimeTerminalProfile{
	{ID: "git-bash", Label: "Git Bash"},
	{ID: "powershell", Label: "PowerShell"},
	{ID: "cmd", Label: "Command Prompt"},
}

func (r *runtimeService) TerminalSettings(context.Context) (RuntimeTerminalSettingsResponse, error) {
	settings, err := loadRuntimeTerminalSettings()
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	return RuntimeTerminalSettingsResponse{Settings: settings}, nil
}

func (r *runtimeService) SaveTerminalSettings(_ context.Context, req RuntimeTerminalSettings) (RuntimeTerminalSettingsResponse, error) {
	profileID := strings.TrimSpace(req.ProfileID)
	if !runtimeTerminalProfileIDSupported(profileID) {
		return RuntimeTerminalSettingsResponse{}, fmt.Errorf("unsupported terminal profile %q", profileID)
	}
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	settings := RuntimeTerminalSettings{
		ProfileID: profileID,
		Profiles:  append([]RuntimeTerminalProfile(nil), runtimeTerminalProfiles...),
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	if err := os.WriteFile(layout.TerminalConfigPath, data, 0o600); err != nil {
		return RuntimeTerminalSettingsResponse{}, fmt.Errorf("failed to write terminal config: %w", err)
	}
	return RuntimeTerminalSettingsResponse{Settings: settings}, nil
}

func loadRuntimeTerminalSettings() (RuntimeTerminalSettings, error) {
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeTerminalSettings{}, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return RuntimeTerminalSettings{}, err
	}
	settings := RuntimeTerminalSettings{
		ProfileID: runtimeTerminalDefaultProfileID,
		Profiles:  append([]RuntimeTerminalProfile(nil), runtimeTerminalProfiles...),
	}
	data, err := os.ReadFile(layout.TerminalConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return RuntimeTerminalSettings{}, fmt.Errorf("failed to read terminal config: %w", err)
	}
	var saved RuntimeTerminalSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		return RuntimeTerminalSettings{}, fmt.Errorf("failed to parse terminal config: %w", err)
	}
	if runtimeTerminalProfileIDSupported(saved.ProfileID) {
		settings.ProfileID = saved.ProfileID
	}
	return settings, nil
}

func runtimeTerminalProfileIDSupported(profileID string) bool {
	for _, profile := range runtimeTerminalProfiles {
		if profile.ID == profileID {
			return true
		}
	}
	return false
}
