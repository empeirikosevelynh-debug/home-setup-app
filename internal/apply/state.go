package apply

import "path/filepath"

// StateDir holds sessions, backups and scratch work: Application Support on
// macOS and the local (not roaming) AppData folder on Windows.
func StateDir(goos, home string) string {
	if goos == "windows" {
		return filepath.Join(home, "AppData", "Local", "Golden Gate Setup")
	}
	return filepath.Join(home, "Library/Application Support/Golden Gate Setup")
}
