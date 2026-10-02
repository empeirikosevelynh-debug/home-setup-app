package plan

import (
	"fmt"
	"golden-gate-setup/internal/domain"
	"path/filepath"
	"strconv"
	"strings"
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

// windows11 accepts versions such as 10.0.22631: Windows 11 reports major
// version 10 and starts at build 22000.
func windows11(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 3 || parts[0] != "10" {
		return false
	}
	build, e := strconv.Atoi(parts[2])
	return e == nil && build >= 22000
}

// buildWindows plans the Chocolatey setup. Fish plugins, Crystal, project
// workspaces and recovery records are not available on Windows yet.
func buildWindows(h domain.Host, o domain.Options, p domain.Plan) (domain.Plan, error) {
	offered := Choices("windows")
	for _, choice := range []struct {
		values, offered, known []string
		name                   string
	}{{o.Apps, offered.Apps, Apps, "app"}, {o.Languages, offered.Languages, Languages, "language"}} {
		for _, value := range choice.values {
			if Has(choice.offered, value) {
				continue
			}
			if Has(choice.known, value) {
				return p, fmt.Errorf("the %s %s is not available on Windows yet", value, choice.name)
			}
			return p, fmt.Errorf("unknown %s selection: %s", choice.name, value)
		}
	}
	if len(o.Plugins) > 0 {
		return p, fmt.Errorf("Fish plugins are not available on Windows")
	}
	if o.PreviousBrewfile != "" || len(o.PreviousPackages) > 0 || o.ImportFrom != "" || len(o.ImportFolders) > 0 || o.DotfilesRepo != "" {
		return p, fmt.Errorf("bringing over a previous Mac or dotfiles is not available on Windows yet")
	}
	if o.CaptureInventory || o.PrepareRecovery {
		return p, fmt.Errorf("recovery records are not available on Windows yet")
	}
	for _, w := range o.Workspaces {
		if w.Path != "" && Has(o.Languages, w.Language) {
			return p, fmt.Errorf("project workspaces are not available on Windows yet")
		}
	}
	if !filepath.IsAbs(h.Home) {
		return p, fmt.Errorf("home directory must be absolute")
	}
	if h.Arch != "amd64" && h.Arch != "arm64" || !windows11(h.Version) {
		p.Supported = false
		p.Problems = append(p.Problems, "This setup requires Windows 11 on x64 or Arm.")
	}
	if h.ChocoPath == "" {
		p.Supported = false
		p.Problems = append(p.Problems, "Install Chocolatey 2 with its official instructions, then inspect again.")
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "install-chocolatey", Title: "Install Chocolatey", Instructions: "In PowerShell opened with Run as administrator, follow Chocolatey's official install instructions, then run setup again.", URL: "https://chocolatey.org/install", Required: true})
	} else if !AtLeastVersion(h.ChocoVersion, "2.0.0") {
		p.Supported = false
		p.Problems = append(p.Problems, "Upgrade Chocolatey to 2.0 or later with choco upgrade chocolatey, then inspect again.")
	}
	if !h.Elevated {
		p.Supported = false
		p.Problems = append(p.Problems, "Run setup from a terminal opened with Run as administrator: Chocolatey installs software for the whole PC.")
	}
	add := func(s domain.Step) {
		if len(p.Steps) > 0 {
			s.DependsOn = []string{p.Steps[len(p.Steps)-1].ID}
		}
		p.Steps = append(p.Steps, s)
	}
	pkg := func(id, minimum, app string) {
		key := "choco:" + id
		if installed, ok := h.Packages[key]; ok {
			if minimum != "" && !AtLeastVersion(installed.Version, minimum) {
				p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "update:" + key, Title: "Review the required update for " + id, Instructions: "Selected features require " + id + " " + minimum + " or later. Update it deliberately with choco upgrade " + id + ", then inspect again.", Required: true})
				p.Supported = false
				p.Problems = append(p.Problems, id+" needs a reviewed minimum-version update.")
			}
			return
		}
		if app != "" {
			for _, a := range h.Apps {
				if strings.EqualFold(a.Name, AppNames[app]) {
					p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "vendor:" + app, Title: "Keep the existing " + a.Name + " installation", Instructions: "This app is already installed outside Chocolatey. Keep its own updates; review any switch to Chocolatey individually.", URL: "https://docs.chocolatey.org/en-us/choco/commands/install/", Required: false})
					return
				}
			}
		}
		q := domain.Package{Kind: "choco", Token: id, MinVersion: minimum}
		add(domain.Step{ID: "package:" + key, Kind: "package", Label: "Install " + id, Package: &q, Check: domain.Check{Kind: "package", Target: key, Expected: minimum}})
	}
	for _, id := range WindowsCore {
		minimum := ""
		if id == "lazygit" {
			minimum = "0.65.1"
		}
		pkg(id, minimum, "")
	}
	for _, app := range WindowsApps {
		if Has(o.Apps, app) {
			pkg(WindowsAppPackages[app], "", app)
		}
	}
	for _, lang := range WindowsLanguages {
		if Has(o.Languages, lang) {
			for _, id := range WindowsLanguagePackages[lang] {
				minimum := ""
				if id == "golang" {
					minimum = "1.26.0"
				}
				pkg(id, minimum, "")
			}
		}
	}
	for _, c := range []configFile{{"starship.toml", filepath.Join(h.Home, ".config", "starship.toml")}, {"zed/settings.windows.json", ZedSettingsPath(h)}, {"lazygit/config.yml", filepath.Join(h.LazyGitDir, "config.yml")}} {
		if err := addConfig(&p, h, o, c, add); err != nil {
			return p, err
		}
	}
	if o.ConfigureGit {
		add(gitStep(h))
	}
	if len(o.Languages) > 0 {
		add(languageStep(o))
	}
	if o.AdoptChezmoi {
		addChezmoi(&p, h, add)
	}
	p.ManualTasks = append(p.ManualTasks, WindowsFinishTasks(o)...)
	var err error
	p.ID, err = Fingerprint(p)
	return p, err
}
