//go:build !windows

package inspect

import "golden-gate-setup/internal/domain"

func readWindows(*domain.Host) {}

// fallbackDirs is where the native Homebrew installs tools.
func fallbackDirs() []string { return []string{"/opt/homebrew/bin"} }

func uninstallEntries() []UninstallEntry { return nil }
