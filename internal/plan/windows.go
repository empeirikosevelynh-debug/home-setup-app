package plan

import (
	"golden-gate-setup/internal/domain"
	"path/filepath"
)

// ZedSettingsPath is Zed's user settings file: ~/.config/zed on macOS and
// %APPDATA%\Zed on Windows.
func ZedSettingsPath(h domain.Host) string {
	if h.OS == "windows" {
		appData := h.AppData
		if appData == "" {
			appData = filepath.Join(h.Home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Zed", "settings.json")
	}
	return filepath.Join(h.Home, ".config/zed/settings.json")
}
