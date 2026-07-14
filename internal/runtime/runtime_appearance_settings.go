package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CIPFZ/agent-builder/internal/db"
)

const (
	appearanceColorModeSettingKey = "appearance.color_mode"
	appearanceThemeIDSettingKey   = "appearance.theme_id"
	defaultAppearanceColorMode    = "system"
	defaultAppearanceThemeID      = "builtin.default"
)

func (r *runtimeService) AppearanceSettings(ctx context.Context) (RuntimeAppearanceSettingsResponse, error) {
	settings, err := loadRuntimeAppearanceSettings(ctx)
	if err != nil {
		return RuntimeAppearanceSettingsResponse{}, err
	}
	return RuntimeAppearanceSettingsResponse{Settings: settings}, nil
}

func (r *runtimeService) SaveAppearanceSettings(ctx context.Context, req RuntimeAppearanceSettings) (RuntimeAppearanceSettingsResponse, error) {
	settings := RuntimeAppearanceSettings{
		ColorMode: strings.TrimSpace(req.ColorMode),
		ThemeID:   strings.TrimSpace(req.ThemeID),
	}
	if settings.ColorMode == "" {
		settings.ColorMode = defaultAppearanceColorMode
	}
	if settings.ThemeID == "" {
		settings.ThemeID = defaultAppearanceThemeID
	}
	if settings.ColorMode != "system" && settings.ColorMode != "light" && settings.ColorMode != "dark" {
		return RuntimeAppearanceSettingsResponse{}, fmt.Errorf("unsupported appearance color mode %q", settings.ColorMode)
	}

	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimeAppearanceSettingsResponse{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck

	now := time.Now().UnixMilli()
	for key, value := range map[string]string{
		appearanceColorModeSettingKey: settings.ColorMode,
		appearanceThemeIDSettingKey:   settings.ThemeID,
	} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO application_settings (key, value_json, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, key, value, now); err != nil {
			return RuntimeAppearanceSettingsResponse{}, fmt.Errorf("save appearance setting %s: %w", key, err)
		}
	}
	return RuntimeAppearanceSettingsResponse{Settings: settings}, nil
}

func loadRuntimeAppearanceSettings(ctx context.Context) (RuntimeAppearanceSettings, error) {
	settings := RuntimeAppearanceSettings{ColorMode: defaultAppearanceColorMode, ThemeID: defaultAppearanceThemeID}
	conn, dataDir, err := openDesktopDB(ctx)
	if err != nil {
		return RuntimeAppearanceSettings{}, err
	}
	defer db.Release(dataDir) //nolint:errcheck

	for key, target := range map[string]*string{
		appearanceColorModeSettingKey: &settings.ColorMode,
		appearanceThemeIDSettingKey:   &settings.ThemeID,
	} {
		var value string
		err := conn.QueryRowContext(ctx, `SELECT value_json FROM application_settings WHERE key = ?`, key).Scan(&value)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return RuntimeAppearanceSettings{}, fmt.Errorf("load appearance setting %s: %w", key, err)
		}
		if err == nil && strings.TrimSpace(value) != "" {
			*target = strings.TrimSpace(value)
		}
	}
	if settings.ColorMode != "system" && settings.ColorMode != "light" && settings.ColorMode != "dark" {
		settings.ColorMode = defaultAppearanceColorMode
	}
	return settings, nil
}
