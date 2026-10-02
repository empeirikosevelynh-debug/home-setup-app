package plan_test

import (
	"bytes"
	"encoding/json"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"path/filepath"
	"reflect"
	"testing"
)

func mustPlan(t *testing.T, h domain.Host, o domain.Options) domain.Plan {
	t.Helper()
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func tokens(p domain.Plan, kind string) []string {
	var out []string
	for _, s := range p.Steps {
		if s.Package != nil && s.Package.Kind == kind {
			out = append(out, s.Package.Token)
		}
	}
	return out
}
func fileStep(p domain.Plan, path string) *domain.FileChange {
	for _, s := range p.Steps {
		if s.File != nil && s.File.Path == path {
			return s.File
		}
	}
	return nil
}
func manual(p domain.Plan, id string) bool {
	for _, m := range p.ManualTasks {
		if m.ID == id {
			return true
		}
	}
	return false
}
func TestFreshPlan(t *testing.T) {
	p := mustPlan(t, testutil.FreshHost(testutil.TempHome(t)), plan.DefaultOptions())
	if !p.Supported || p.Accepted || p.SchemaVersion != 1 || p.ID == "" {
		t.Fatalf("fresh preview has no supported, unaccepted plan: %+v", p)
	}
	want := []string{"fish", "starship", "zoxide", "chezmoi", "gh", "fzf", "fd", "bat", "eza", "ripgrep", "git-delta", "lazygit"}
	if !reflect.DeepEqual(tokens(p, "formula"), want) {
		t.Fatalf("core install: %v", tokens(p, "formula"))
	}
	if !reflect.DeepEqual(tokens(p, "cask"), []string{"warp", "zed", "applite"}) {
		t.Fatalf("apps: %v", tokens(p, "cask"))
	}
	if !manual(p, "applite-shared-brew") || !manual(p, "time-machine") {
		t.Fatal("missing shared-path/backup finish tasks")
	}
}
func TestOptionalSelections(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	o := plan.DefaultOptions()
	p := mustPlan(t, h, o)
	for _, bad := range []string{"github", "kopiaui", "go", "crystal", "nim"} {
		for _, s := range p.Steps {
			if s.Package != nil && s.Package.Token == bad {
				t.Fatalf("unselected package %s", bad)
			}
		}
	}
	o.Apps = []string{"github", "kopiaui"}
	o.Plugins = nil
	o.Languages = []string{"crystal"}
	p = mustPlan(t, h, o)
	for _, want := range []string{"crystal", "crystalline", "ameba"} {
		found := false
		for _, s := range p.Steps {
			if s.Package != nil && s.Package.Token == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing chosen tool %s", want)
		}
	}
	if !reflect.DeepEqual(tokens(p, "cask"), []string{"github", "kopiaui"}) {
		t.Fatalf("wrong apps: %v", tokens(p, "cask"))
	}
}
func TestRegisteredAndVendorApps(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	h.Packages["cask:warp"] = domain.InstalledPackage{Version: "1"}
	h.Apps = []domain.AppBundle{{Name: "Zed", Path: "/Applications/Zed.app"}}
	p := mustPlan(t, h, plan.DefaultOptions())
	if !reflect.DeepEqual(tokens(p, "cask"), []string{"applite"}) || !manual(p, "vendor:zed") {
		t.Fatalf("existing app channel was not preserved: %+v", p)
	}
	raw, _ := json.Marshal(p)
	if bytes.Contains(raw, []byte("--force")) || bytes.Contains(raw, []byte("--adopt")) {
		t.Fatal("implicit app replacement")
	}
}
func TestExistingSettingsPreserved(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	path := filepath.Join(h.Home, ".config/zed/settings.json")
	h.Files[path] = domain.FileState{Path: path, Exists: true, SHA256: "current", Contents: []byte("// important comment\n{}")}
	h.ChezmoiDirty = true
	p := mustPlan(t, h, plan.DefaultOptions())
	if fileStep(p, path) != nil || !manual(p, "file:"+path) || !manual(p, "chezmoi-conflict") {
		t.Fatal("existing settings or dirty source not preserved")
	}
	o := plan.DefaultOptions()
	o.FileChoices = map[string]domain.FileDecision{path: domain.Replace}
	p = mustPlan(t, h, o)
	c := fileStep(p, path)
	if c == nil || c.BeforeSHA256 != "current" || c.Decision != domain.Replace {
		t.Fatal("reviewed replacement lost before-state")
	}
}
func TestUnknownSelectionRejected(t *testing.T) {
	o := plan.DefaultOptions()
	o.Apps = []string{"zed; rm -rf x"}
	if _, err := plan.Build(testutil.FreshHost(testutil.TempHome(t)), o); err == nil {
		t.Fatal("unknown app accepted")
	}
}
func TestPlanDeterministic(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	a := mustPlan(t, h, plan.DefaultOptions())
	b := mustPlan(t, h, plan.DefaultOptions())
	if a.ID == "" || a.ID != b.ID {
		t.Fatal("unstable plan identifier")
	}
}
func TestInstalledMinimumReported(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	h.Packages["formula:fish"] = domain.InstalledPackage{Version: "3.7.1"}
	p := mustPlan(t, h, plan.DefaultOptions())
	if !manual(p, "update:formula:fish") {
		t.Fatal("old fish not explained")
	}
	for _, s := range p.Steps {
		if s.Package != nil && s.Package.Token == "fish" {
			t.Fatal("implicit fish upgrade")
		}
	}
}
func TestPreviewDoesNotLeakHostContents(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	path := filepath.Join(h.Home, ".config/fish/config.fish")
	h.Files[path] = domain.FileState{Path: path, Exists: true, Contents: []byte("set -gx TOKEN private-test-token")}
	raw, _ := json.Marshal(mustPlan(t, h, plan.DefaultOptions()))
	if bytes.Contains(raw, []byte("private-test-token")) {
		t.Fatal("host content leaked into plan JSON")
	}
}
func TestCustomConfigurationAndTapArePreserved(t *testing.T) {
	h := testutil.FreshHost(testutil.TempHome(t))
	h.ConfigHome = filepath.Join(h.Home, "custom-config")
	p := mustPlan(t, h, plan.DefaultOptions())
	if p.Supported {
		t.Fatal("custom config location ignored")
	}
	h.ConfigHome = ""
	h.Packages["formula:custom/tap/fish"] = domain.InstalledPackage{Version: "4.2.0"}
	p = mustPlan(t, h, plan.DefaultOptions())
	for _, s := range p.Steps {
		if s.Package != nil && s.Package.Token == "fish" {
			t.Fatal("custom formula would be replaced")
		}
	}
	if !manual(p, "custom-package:formula:fish") {
		t.Fatal("custom formula needs a visible review task")
	}
}
