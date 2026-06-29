package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func (r *runtimeService) TerminalSettings(context.Context) (RuntimeTerminalSettingsResponse, error) {
	settings, err := loadRuntimeTerminalSettings()
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	return RuntimeTerminalSettingsResponse{Settings: settings}, nil
}

func (r *runtimeService) SaveTerminalSettings(_ context.Context, req RuntimeTerminalSettings) (RuntimeTerminalSettingsResponse, error) {
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
	layout, err := resolveDesktopLayout()
	if err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	if err := ensureDesktopLayout(layout); err != nil {
		return RuntimeTerminalSettingsResponse{}, err
	}
	settings := RuntimeTerminalSettings{
		ProfileID: profileID,
		Profiles:  profiles,
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
	profiles := runtimeTerminalAvailableProfiles()
	settings := RuntimeTerminalSettings{
		ProfileID: runtimeTerminalDefaultProfileID(profiles),
		Profiles:  profiles,
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
	if runtimeTerminalProfileAvailable(saved.ProfileID, profiles) {
		settings.ProfileID = saved.ProfileID
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
