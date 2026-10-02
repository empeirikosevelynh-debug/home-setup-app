package testutil

import (
	"golden-gate-setup/internal/domain"
	"path/filepath"
	"testing"
)

func TempHome(t *testing.T) string { t.Helper(); return t.TempDir() }
func FreshHost(home string) domain.Host {
	return domain.Host{OS: "darwin", Arch: "arm64", Version: "27.0.1", Home: home,
		BrewPath: "/opt/homebrew/bin/brew", BrewPrefix: "/opt/homebrew", LazyGitDir: filepath.Join(home, "Library/Application Support/lazygit"),
		Packages: map[string]domain.InstalledPackage{}, Files: map[string]domain.FileState{}, Tools: map[string]string{}}
}

// FreshWindowsHost is a supported, elevated Windows 11 machine with
// Chocolatey 2 and nothing else installed.
func FreshWindowsHost(home string) domain.Host {
	appData := filepath.Join(home, "AppData", "Roaming")
	return domain.Host{OS: "windows", Arch: "amd64", Version: "10.0.26100", Home: home, Elevated: true,
		ChocoPath: `C:\ProgramData\chocolatey\bin\choco.exe`, ChocoVersion: "2.4.1", AppData: appData, LazyGitDir: filepath.Join(appData, "lazygit"),
		Packages: map[string]domain.InstalledPackage{"choco:chocolatey": {Version: "2.4.1"}}, Files: map[string]domain.FileState{}, Tools: map[string]string{}}
}
