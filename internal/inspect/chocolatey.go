package inspect

import (
	"context"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"path/filepath"
	"strings"
)

// ChocolateyPath is where Chocolatey keeps choco.exe, given the
// ChocolateyInstall environment variable (empty means the default location).
func ChocolateyPath(install string) string {
	if install == "" {
		install = `C:\ProgramData\chocolatey`
	}
	return filepath.Join(install, "bin", "choco.exe")
}

// readChocolatey records Chocolatey and its installed packages. Before 2.0,
// `choco list` searched the online repository, so only 2.x is listed.
func (i Inspector) readChocolatey(ctx context.Context, h *domain.Host, install string) error {
	p, e := i.lookup(ChocolateyPath(install))
	if e != nil {
		return nil
	}
	h.ChocoPath = p
	h.ChocoVersion, e = i.capture(ctx, p, "--version")
	if e != nil {
		return e
	}
	if !plan.AtLeastVersion(h.ChocoVersion, "2.0.0") {
		return nil
	}
	raw, e := i.capture(ctx, p, "list", "--limit-output")
	if e != nil {
		return e
	}
	h.Packages, e = readChocoPackages(raw)
	return e
}

// readChocoPackages parses `choco list --limit-output`: one id|version per line.
func readChocoPackages(raw string) (map[string]domain.InstalledPackage, error) {
	packages := map[string]domain.InstalledPackage{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id, version, ok := strings.Cut(line, "|")
		if !ok || id == "" || version == "" || strings.Contains(version, "|") {
			return nil, fmt.Errorf("cannot read installed Chocolatey packages: %q", line)
		}
		packages["choco:"+strings.ToLower(id)] = domain.InstalledPackage{Version: version}
	}
	return packages, nil
}

// UninstallEntry is one program from the Windows Uninstall registry keys.
type UninstallEntry struct{ Name, Location string }

// windowsApps keeps the offered apps among installed programs, under their
// catalog names, so a vendor installation is preserved like on macOS.
// Installers often append a version to the display name.
func windowsApps(entries []UninstallEntry, packages map[string]domain.InstalledPackage) []domain.AppBundle {
	apps := []domain.AppBundle{}
	for _, token := range plan.WindowsApps {
		name := plan.AppNames[token]
		for _, entry := range entries {
			display := strings.ToLower(strings.TrimSpace(entry.Name))
			if display == strings.ToLower(name) || strings.HasPrefix(display, strings.ToLower(name)+" ") {
				_, registered := packages["choco:"+plan.WindowsAppPackages[token]]
				apps = append(apps, domain.AppBundle{Name: name, Path: entry.Location, Registered: registered})
				break
			}
		}
	}
	return apps
}
