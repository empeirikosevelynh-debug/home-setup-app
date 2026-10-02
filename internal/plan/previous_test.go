package plan_test

import (
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"reflect"
	"strings"
	"testing"
)

const dump = `tap "homebrew/bundle"
tap "homebrew/core"
tap "user/tools"
tap "corp/private", "https://example.test/tap.git"
# brew "commented-out"
brew "fish"
brew "jq", restart_service: :changed
brew "user/tools/thing"
brew "python@3.12"
brew "--force"
cask "zed"
cask "firefox"
mas "Keynote", id: 409183694
vscode "golang.go"
brew "jq"
`

func TestParseBrewfile(t *testing.T) {
	entries, other := plan.ParseBrewfile([]byte(dump))
	want := []string{"tap:homebrew/bundle", "tap:user/tools", "formula:fish", "formula:jq", "formula:user/tools/thing", "formula:python@3.12", "cask:zed", "cask:firefox"}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries: %q", entries)
	}
	if len(other) != 4 || !strings.Contains(strings.Join(other, "\n"), "corp/private") || !strings.Contains(strings.Join(other, "\n"), "--force") {
		t.Fatalf("other: %q", other)
	}
	if entries, _ := plan.ParseBrewfile([]byte("# cask \"zed\"\n# brew \"fish\"\n")); len(entries) != 0 {
		t.Fatal("commented starter Brewfile listed packages", entries)
	}
}
func previousHost(t *testing.T) domain.Host {
	h := testutil.FreshHost(t.TempDir())
	h.PreviousEntries, h.PreviousOther = plan.ParseBrewfile([]byte(dump))
	h.Taps = []string{"homebrew/bundle"}
	h.Packages["formula:jq"] = domain.InstalledPackage{Version: "1.7"}
	return h
}
func TestPreviousAppListPlanned(t *testing.T) {
	h := previousHost(t)
	o := plan.DefaultOptions()
	o.PreviousBrewfile = "/Volumes/Old/Golden Gate Recovery/2026-09-30-abc/Homebrew-full.Brewfile"
	o.PreviousPackages = append([]string(nil), h.PreviousEntries...)
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	ids := map[string]int{}
	order := []string{}
	for _, s := range p.Steps {
		ids[s.ID]++
		order = append(order, s.ID)
	}
	for _, id := range []string{"tap:user/tools", "package:formula:user/tools/thing", "package:formula:python@3.12", "package:cask:firefox"} {
		if ids[id] != 1 {
			t.Fatal("missing or repeated step", id, ids[id])
		}
	}
	for _, id := range []string{"tap:homebrew/bundle", "package:formula:jq"} {
		if ids[id] != 0 {
			t.Fatal("installed entry planned again", id)
		}
	}
	if ids["package:formula:fish"] != 1 || ids["package:cask:zed"] != 1 {
		t.Fatal("catalog package repeated or lost", ids["package:formula:fish"], ids["package:cask:zed"])
	}
	tap, thing := -1, -1
	for i, id := range order {
		if id == "tap:user/tools" {
			tap = i
		}
		if id == "package:formula:user/tools/thing" {
			thing = i
		}
	}
	if tap > thing {
		t.Fatal("formula planned before its tap")
	}
	found := false
	for _, task := range p.ManualTasks {
		found = found || task.ID == "previous-other" && strings.Contains(task.Instructions, "Keynote")
	}
	if !found {
		t.Fatal("unsupported entries not listed for follow-up")
	}
}
func TestPreviousAppListOnlyChosen(t *testing.T) {
	h := previousHost(t)
	o := plan.DefaultOptions()
	o.PreviousBrewfile = "/x/Homebrew-full.Brewfile"
	o.PreviousPackages = []string{"cask:firefox"}
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range p.Steps {
		if s.ID == "tap:user/tools" || s.ID == "package:formula:user/tools/thing" {
			t.Fatal("unchosen entry planned", s.ID)
		}
	}
	o.PreviousPackages = []string{"cask:not-in-the-list"}
	if _, e := plan.Build(h, o); e == nil {
		t.Fatal("entry outside the app list accepted")
	}
}
func TestWindowsRefusesPreviousAppList(t *testing.T) {
	o := plan.DefaultOptionsFor("windows")
	o.PreviousBrewfile = `C:\old\Homebrew-full.Brewfile`
	if _, e := plan.Build(testutil.FreshWindowsHost(t.TempDir()), o); e == nil {
		t.Fatal("previous Mac app list accepted on Windows")
	}
}
