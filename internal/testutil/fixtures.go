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
