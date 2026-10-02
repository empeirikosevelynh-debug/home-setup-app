//go:build windows

package inspect

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golden-gate-setup/internal/domain"
	"os"
	"path/filepath"
)

// readWindows records the Windows version and whether this process runs
// elevated, which Chocolatey needs to install system-wide.
func readWindows(h *domain.Host) {
	v := windows.RtlGetVersion()
	h.Version = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	h.Elevated = windows.GetCurrentProcessToken().IsElevated()
}

// fallbackDirs adds the PATH saved in the registry and Chocolatey's shims.
// Installers update the saved PATH, not this process's copy, so tools
// installed during a run are found only here.
func fallbackDirs() []string {
	var dirs []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`}, {registry.CURRENT_USER, `Environment`}} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			continue
		}
		if expanded, err := registry.ExpandString(value); err == nil {
			value = expanded
		}
		dirs = append(dirs, filepath.SplitList(value)...)
	}
	return append(dirs, filepath.Dir(ChocolateyPath(os.Getenv("ChocolateyInstall"))))
}

// uninstallEntries lists installed programs from the machine (64- and 32-bit)
// and user Uninstall keys.
func uninstallEntries() []UninstallEntry {
	var entries []UninstallEntry
	for _, k := range []struct {
		root registry.Key
		path string
	}{{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`}, {registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`}, {registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`}} {
		key, err := registry.OpenKey(k.root, k.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := key.ReadSubKeyNames(-1)
		key.Close()
		for _, name := range names {
			sub, err := registry.OpenKey(k.root, k.path+`\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			display, _, err := sub.GetStringValue("DisplayName")
			location, _, _ := sub.GetStringValue("InstallLocation")
			sub.Close()
			if err == nil && display != "" {
				entries = append(entries, UninstallEntry{Name: display, Location: location})
			}
		}
	}
	return entries
}
