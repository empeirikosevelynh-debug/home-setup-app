package inspect_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/inspect"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const brew = "/opt/homebrew/bin/brew"
const installed = `{"formulae":[{"name":"fish","full_name":"fish","installed":[{"version":"4.2.0"}]}],"casks":[{"token":"zed","tap":"homebrew/cask","installed":"0.226.2"}]}`

func fixture(t *testing.T) (inspect.Inspector, *testutil.FakeRunner) {
	t.Helper()
	h := testutil.FreshHost(t.TempDir())
	f := &testutil.FakeRunner{Responses: map[string]testutil.Response{brew + " --prefix": {Output: "/opt/homebrew\n"}, brew + " info --json=v2 --installed": {Output: installed}}}
	i := inspect.Inspector{Home: h.Home, Runner: f, Platform: func(context.Context) (domain.Host, error) { return h, nil }, AppsDirs: []string{filepath.Join(h.Home, "Applications")}, Lookup: func(p string) (string, error) {
		if p == brew {
			return brew, nil
		}
		return "", fs.ErrNotExist
	}}
	return i, f
}
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			result[p] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func read(t *testing.T, i inspect.Inspector) domain.Host {
	t.Helper()
	h, err := i.Read(context.Background(), plan.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestPreviewDoesNotWrite(t *testing.T) {
	i, f := fixture(t)
	os.WriteFile(filepath.Join(i.Home, "keep"), []byte("private"), 0600)
	before := snapshot(t, i.Home)
	h := read(t, i)
	p, e := plan.Build(h, plan.DefaultOptions())
	if e != nil || !p.Supported {
		t.Fatalf("compatible preview failed: %v %+v", e, p)
	}
	if !reflect.DeepEqual(before, snapshot(t, i.Home)) {
		t.Fatal("inspection wrote home files")
	}
	for _, c := range f.Calls {
		if c.Path != brew {
			t.Fatal("unexpected command")
		}
	}
	if h.Packages["formula:fish"].Version != "4.2.0" || h.Packages["cask:zed"].Version != "0.226.2" {
		t.Fatal("installed JSON was not read")
	}
}
func TestMissingBrew(t *testing.T) {
	i, _ := fixture(t)
	i.Lookup = func(string) (string, error) { return "", fs.ErrNotExist }
	h := read(t, i)
	p, _ := plan.Build(h, plan.DefaultOptions())
	if h.BrewPath != "" || p.Supported || len(p.Problems) == 0 {
		t.Fatal("missing bootstrap prerequisite was hidden")
	}
}
func TestConflictingPrefix(t *testing.T) {
	i, f := fixture(t)
	f.Responses[brew+" --prefix"] = testutil.Response{Output: "/usr/local\n"}
	h := read(t, i)
	p, _ := plan.Build(h, plan.DefaultOptions())
	if p.Supported {
		t.Fatal("non-native prefix accepted")
	}
}
func TestRosettaAndUnsupportedOS(t *testing.T) {
	for _, h := range []domain.Host{{OS: "darwin", Arch: "amd64", Version: "27.0", Rosetta: true}, {OS: "darwin", Arch: "arm64", Version: "26.0"}, {OS: "linux", Arch: "arm64", Version: "27.0"}} {
		i, _ := fixture(t)
		i.Platform = func(context.Context) (domain.Host, error) { return h, nil }
		p, _ := plan.Build(read(t, i), plan.DefaultOptions())
		if p.Supported {
			t.Fatalf("unsupported host accepted: %+v", h)
		}
	}
}
func TestMalformedProbeOutput(t *testing.T) {
	for _, data := range []string{`{broken}`, `{"formulae":[{"name":"fish","installed":[]}],"casks":[]}`, `{"formulae":null}`, `{"formulae":[],"casks":[{"token":"zed","installed":false}]}`} {
		i, f := fixture(t)
		f.Responses[brew+" info --json=v2 --installed"] = testutil.Response{Output: data}
		if _, err := i.Read(context.Background(), plan.DefaultOptions()); err == nil {
			t.Fatalf("malformed data became fresh state: %s", data)
		}
	}
}
func TestChezMoiDirtySource(t *testing.T) {
	i, f := fixture(t)
	source := filepath.Join(i.Home, "dotfiles")
	os.MkdirAll(filepath.Join(source, ".git"), 0700)
	i.Lookup = func(p string) (string, error) {
		switch p {
		case brew:
			return brew, nil
		case "chezmoi":
			return "/fake/chezmoi", nil
		case "git":
			return "/fake/git", nil
		}
		return "", fs.ErrNotExist
	}
	f.Responses["/fake/chezmoi source-path"] = testutil.Response{Output: source + "\n"}
	f.Responses["/fake/git -C "+source+" status --porcelain --untracked-files=all"] = testutil.Response{Output: " M dot_config/fish/config.fish\n"}
	h := read(t, i)
	if !h.ChezmoiDirty || h.ChezmoiDir != source {
		t.Fatal("pending source edits missed")
	}
	p, _ := plan.Build(h, plan.DefaultOptions())
	for _, s := range p.Steps {
		if s.Kind == "chezmoi" {
			t.Fatal("dirty source was adopted")
		}
	}
}
func TestAppsAndReceipts(t *testing.T) {
	i, _ := fixture(t)
	os.MkdirAll(filepath.Join(i.AppsDirs[0], "Zed.app/Contents/_MASReceipt"), 0700)
	os.WriteFile(filepath.Join(i.AppsDirs[0], "Zed.app/Contents/_MASReceipt/receipt"), []byte("receipt"), 0600)
	h := read(t, i)
	if len(h.Apps) != 1 || !h.Apps[0].StoreReceipt || !h.Apps[0].Registered {
		t.Fatal("app channel metadata lost")
	}
}
func TestFisherStateFromUniversalStore(t *testing.T) {
	i, f := fixture(t)
	path := filepath.Join(i.Home, ".config/fish/fish_variables")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("# This file contains fish universal variable definitions.\n# VERSION: 3.0\nSETUVAR __fish_initialized:3800\nSETUVAR _fisher_plugins:jorgebucaran/fisher\\x1ePatrickF1/fzf\\x2efish\\x1ecaf\\u00e9/plugin\nSETUVAR --export --path EXAMPLE:/a\\x1e/b\n"), 0600)
	h := read(t, i)
	if !reflect.DeepEqual(h.FishInstalledPlugins, []string{"jorgebucaran/fisher", "PatrickF1/fzf.fish", "café/plugin"}) || h.FishLegacy {
		t.Fatalf("stored plugins misread: %q %v", h.FishInstalledPlugins, h.FishLegacy)
	}
	for _, c := range f.Calls {
		if c.Path != brew {
			t.Fatal("fish ran to read its own store", c)
		}
	}
	os.WriteFile(path, []byte("SETUVAR _fisher_list:a\\x1eb\n"), 0600)
	if h = read(t, i); !h.FishLegacy || len(h.FishInstalledPlugins) != 0 {
		t.Fatal("legacy Fisher state missed")
	}
}
func windowsFixture(t *testing.T, version, list string) (inspect.Inspector, *testutil.FakeRunner) {
	t.Helper()
	h := testutil.FreshWindowsHost(t.TempDir())
	choco := inspect.ChocolateyPath("")
	f := &testutil.FakeRunner{Responses: map[string]testutil.Response{choco + " --version": {Output: version + "\n"}, choco + " list --limit-output": {Output: list}}}
	i := inspect.Inspector{Home: h.Home, Runner: f, Platform: func(context.Context) (domain.Host, error) { return h, nil }, Lookup: func(p string) (string, error) {
		if p == choco {
			return choco, nil
		}
		return "", fs.ErrNotExist
	}}
	return i, f
}
func TestWindowsInspectionReadsChocolatey(t *testing.T) {
	i, f := windowsFixture(t, "2.4.1", "chocolatey|2.4.1\nGit|2.56.0\n")
	h, e := i.Read(context.Background(), plan.DefaultOptionsFor("windows"))
	if e != nil {
		t.Fatal(e)
	}
	if h.ChocoPath != inspect.ChocolateyPath("") || h.ChocoVersion != "2.4.1" || h.Packages["choco:git"].Version != "2.56.0" || h.BrewPath != "" {
		t.Fatalf("Chocolatey state not read: %+v", h)
	}
	if h.LazyGitDir != filepath.Join(h.AppData, "lazygit") || h.AppData == "" {
		t.Fatal("Windows configuration folders not set", h.AppData, h.LazyGitDir)
	}
	for _, c := range f.Calls {
		if c.Path != h.ChocoPath {
			t.Fatal("unexpected command", c)
		}
	}
	if _, ok := h.Files[plan.ZedSettingsPath(h)]; !ok {
		t.Fatal("Windows Zed settings not inspected")
	}
}
func TestOldChocolateyNotListed(t *testing.T) {
	i, f := windowsFixture(t, "1.4.0", "")
	h, e := i.Read(context.Background(), plan.DefaultOptionsFor("windows"))
	if e != nil || h.ChocoVersion != "1.4.0" || len(h.Packages) != 0 || len(f.Calls) != 1 {
		t.Fatal("Chocolatey 1.x list would search the online repository", e, f.Calls)
	}
}
func TestMalformedChocolateyListRejected(t *testing.T) {
	i, _ := windowsFixture(t, "2.4.1", "not a package line\n")
	if _, e := i.Read(context.Background(), plan.DefaultOptionsFor("windows")); e == nil {
		t.Fatal("malformed Chocolatey output became fresh state")
	}
}
