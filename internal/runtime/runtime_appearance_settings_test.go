package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestSaveAppearanceSettingsRejectsUnsupportedColorMode(t *testing.T) {
	service := &runtimeService{}
	_, err := service.SaveAppearanceSettings(context.Background(), RuntimeAppearanceSettings{
		ColorMode: "sepia",
		ThemeID:   defaultAppearanceThemeID,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported appearance color mode") {
		t.Fatalf("expected unsupported color mode error, got %v", err)
	}
}
