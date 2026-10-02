package plan_test

import (
	"bytes"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/templates"
	"golden-gate-setup/internal/testutil"
	"strings"
	"testing"
)

func windowsPlan(t *testing.T, h domain.Host, o domain.Options) domain.Plan {
	t.Helper()
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func tasks(p domain.Plan) map[string]bool {
	found := map[string]bool{}
	for _, task := range p.ManualTasks {
		found[task.ID] = true
	}
	return found
}

func TestWindowsDefaultPlan(t *testing.T) {
	h := testutil.FreshWindowsHost(t.TempDir())
	p := windowsPlan(t, h, plan.DefaultOptionsFor("windows"))
	if !p.Supported || len(p.Problems) != 0 {
		t.Fatal("fresh Windows host unsupported", p.Problems)
	}
	var packages []string
	kinds := map[string]int{}
	for _, s := range p.Steps {
		kinds[s.Kind]++
		if s.Kind == "package" {
			if s.Package.Kind != "choco" {
				t.Fatal("non-Chocolatey package on Windows", s.Package)
			}
			packages = append(packages, s.Package.Token)
		}
	}
	want := append(append([]string{}, plan.WindowsCore...), "warp-terminal", "zed-editor")
	if strings.Join(packages, ",") != strings.Join(want, ",") {
		t.Fatal("unexpected packages", packages)
	}
	if kinds["file"] != 3 || kinds["git"] != 1 || kinds["chezmoi"] != 1 || kinds["plugins"]+kinds["recovery"]+kinds["language-tools"] != 0 {
		t.Fatal("unexpected steps", kinds)
	}
	found := tasks(p)
	for _, id := range []string{"warp", "zed", "chocolatey-updates", "windows-backup", "dotfiles-remote", "github-auth"} {
		if !found[id] {
			t.Fatal("missing follow-up task", id)
		}
	}
	for _, id := range []string{"applite-shared-brew", "cork", "time-machine", "maintenance"} {
		if found[id] {
			t.Fatal("macOS follow-up task on Windows", id)
		}
	}
	again := windowsPlan(t, h, plan.DefaultOptionsFor("windows"))
	if again.ID != p.ID {
		t.Fatal("Windows plan is not deterministic")
	}
}
func TestWindowsZedSettingsTemplate(t *testing.T) {
	h := testutil.FreshWindowsHost(t.TempDir())
	p := windowsPlan(t, h, plan.DefaultOptionsFor("windows"))
	want, _ := templates.Load("zed/settings.windows.json")
	for _, s := range p.Steps {
		if s.File != nil && s.File.Path == plan.ZedSettingsPath(h) {
			if !bytes.Equal(s.File.Desired, want) || bytes.Contains(s.File.Desired, []byte("fish")) {
				t.Fatal("Zed settings are not the Windows template")
			}
			return
		}
	}
	t.Fatal("Zed settings not planned at", plan.ZedSettingsPath(h))
}
func TestWindowsUnsupportedHosts(t *testing.T) {
	for name, change := range map[string]func(*domain.Host){
		"not elevated":   func(h *domain.Host) { h.Elevated = false },
		"no Chocolatey":  func(h *domain.Host) { h.ChocoPath, h.ChocoVersion = "", "" },
		"Chocolatey 1.x": func(h *domain.Host) { h.ChocoVersion = "1.4.0" },
		"Windows 10":     func(h *domain.Host) { h.Version = "10.0.19045" },
		"32-bit":         func(h *domain.Host) { h.Arch = "386" },
	} {
		h := testutil.FreshWindowsHost(t.TempDir())
		change(&h)
		p := windowsPlan(t, h, plan.DefaultOptionsFor("windows"))
		if p.Supported || len(p.Problems) == 0 {
			t.Fatal("accepted:", name)
		}
		if name == "no Chocolatey" && !tasks(p)["install-chocolatey"] {
			t.Fatal("no Chocolatey install task")
		}
	}
}
func TestWindowsRejectsUnavailableChoices(t *testing.T) {
	for name, change := range map[string]func(*domain.Options){
		"plugins":   func(o *domain.Options) { o.Plugins = []string{"PatrickF1/fzf.fish"} },
		"crystal":   func(o *domain.Options) { o.Languages = []string{"crystal"} },
		"applite":   func(o *domain.Options) { o.Apps = []string{"applite"} },
		"recovery":  func(o *domain.Options) { o.PrepareRecovery = true },
		"inventory": func(o *domain.Options) { o.CaptureInventory = true },
		"workspace": func(o *domain.Options) {
			o.Languages = []string{"go"}
			o.Workspaces = []domain.Workspace{{Language: "go", Path: `C:\work\app`, Module: "example.com/me/app", Create: true}}
		},
		"unknown": func(o *domain.Options) { o.Apps = []string{"nonexistent"} },
	} {
		o := plan.DefaultOptionsFor("windows")
		change(&o)
		if _, e := plan.Build(testutil.FreshWindowsHost(t.TempDir()), o); e == nil {
			t.Fatal("accepted:", name)
		}
	}
}
func TestWindowsExistingPackagesAndApps(t *testing.T) {
	h := testutil.FreshWindowsHost(t.TempDir())
	h.Packages["choco:lazygit"] = domain.InstalledPackage{Version: "0.50.0"}
	h.Packages["choco:git"] = domain.InstalledPackage{Version: "2.56.0"}
	h.Apps = []domain.AppBundle{{Name: "Zed", Path: `C:\Program Files\Zed`}}
	p := windowsPlan(t, h, plan.DefaultOptionsFor("windows"))
	found := tasks(p)
	if p.Supported || !found["update:choco:lazygit"] || !found["vendor:zed"] {
		t.Fatal("old lazygit or vendor Zed not reviewed", p.Problems)
	}
	for _, s := range p.Steps {
		if s.Package != nil && (s.Package.Token == "git" || s.Package.Token == "lazygit" || s.Package.Token == "zed-editor") {
			t.Fatal("installed or vendor package planned again", s.Package.Token)
		}
	}
}
func TestWindowsLanguages(t *testing.T) {
	o := plan.DefaultOptionsFor("windows")
	o.Languages = []string{"go", "nim"}
	p := windowsPlan(t, testutil.FreshWindowsHost(t.TempDir()), o)
	golang, tools := false, false
	for _, s := range p.Steps {
		if s.Package != nil && s.Package.Token == "golang" {
			golang = s.Package.MinVersion == "1.26.0"
		}
		tools = tools || s.Kind == "language-tools"
	}
	found := tasks(p)
	if !golang || !tools || !found["delve"] || !found["language:nim"] {
		t.Fatal("languages not planned", golang, tools, found)
	}
}
