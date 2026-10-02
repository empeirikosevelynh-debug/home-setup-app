package inspect_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/inspect"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
	i := inspect.Inspector{Home: h.Home, Runner: f, Getenv: func(string) string { return "" }, Platform: func(context.Context) (domain.Host, error) { return h, nil }, Lookup: func(p string) (string, error) {
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
func TestPreviousAppListRead(t *testing.T) {
	i, f := fixture(t)
	f.Responses[brew+" tap"] = testutil.Response{Output: "homebrew/bundle\nuser/tools\n"}
	dir := filepath.Join(t.TempDir(), "Golden Gate Recovery", "2026-09-30-abc")
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "Homebrew-full.Brewfile")
	os.WriteFile(path, []byte("tap \"user/tools\"\nbrew \"jq\"\ncask \"firefox\"\nmas \"Keynote\", id: 409183694\n"), 0600)
	o := plan.DefaultOptions()
	o.PreviousBrewfile = path
	h, e := i.Read(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(h.PreviousEntries, []string{"tap:user/tools", "formula:jq", "cask:firefox"}) || len(h.PreviousOther) != 1 || !reflect.DeepEqual(h.Taps, []string{"homebrew/bundle", "user/tools"}) {
		t.Fatalf("previous app list misread: %q %q %q", h.PreviousEntries, h.PreviousOther, h.Taps)
	}
	link := filepath.Join(t.TempDir(), "link.Brewfile")
	os.Symlink(path, link)
	for _, bad := range []string{link, filepath.Join(dir, "missing.Brewfile"), "relative.Brewfile"} {
		o.PreviousBrewfile = bad
		if _, e := i.Read(context.Background(), o); e == nil {
			t.Fatal("unusable previous app list accepted:", bad)
		}
	}
}
func TestImportRead(t *testing.T) {
	i, _ := fixture(t)
	old := filepath.Join(t.TempDir(), "you")
	for path, contents := range map[string]string{"Documents/a.txt": "a", "Documents/sub/b.txt": "bb", ".ssh/id_ed25519": "key", ".zshrc": "zsh", ".config/fish/fish_variables": "SETUVAR _fisher_plugins:jorgebucaran/fisher\n", ".config/fish/config.fish": "set -g old 1\n", ".config/fish/functions/custom.fish": "function custom; end\n", ".config/gh/hosts.yml": "github.com:\n  oauth_token: abc\n", ".zsh_history": "ls\n", ".config/tool/secrets.txt": "x"} {
		os.MkdirAll(filepath.Dir(filepath.Join(old, path)), 0700)
		os.WriteFile(filepath.Join(old, path), []byte(contents), 0600)
	}
	os.MkdirAll(filepath.Join(i.Home, "Imported conflicts", "2026-09-30"), 0700)
	o := plan.DefaultOptions()
	o.RecoveryDate = "2026-10-02"
	o.ImportFrom, o.ImportFolders = old, []string{"Documents", ".ssh", ".", ".config"}
	before, beforeOld := snapshot(t, i.Home), snapshot(t, old)
	h, e := i.Read(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, snapshot(t, i.Home)) || !reflect.DeepEqual(beforeOld, snapshot(t, old)) {
		t.Fatal("inspection wrote files")
	}
	if len(h.Import) != 4 || h.Import[0].Folder != "Documents" || h.Import[0].Copy != 2 || h.Import[0].CopyBytes != 3 || h.Import[1].Copy != 1 || h.Import[2].Copy != 2 || h.FreeBytes <= 0 {
		t.Fatalf("import misread: %+v %d", h.Import, h.FreeBytes)
	}
	if !reflect.DeepEqual(h.ConflictDates, []string{"2026-09-30"}) {
		t.Fatal(h.ConflictDates)
	}
	// Only small text files without keys or secrets are offered to chezmoi.
	var offered []string
	for _, path := range h.ChezmoiCandidates {
		rel, _ := filepath.Rel(i.Home, path)
		offered = append(offered, rel)
	}
	slices.Sort(offered)
	if want := []string{".config/fish/config.fish", ".config/fish/fish_variables", ".config/fish/functions/custom.fish", ".zshrc"}; !reflect.DeepEqual(offered, want) {
		t.Fatalf("offered to chezmoi: %q", offered)
	}
	// The plan is made against what the import brings, in the same review.
	fish := h.Files[filepath.Join(i.Home, ".config/fish/config.fish")]
	if !fish.Exists || !fish.Imported || string(fish.Contents) != "set -g old 1\n" || !h.FishPluginConflict || !reflect.DeepEqual(h.FishInstalledPlugins, []string{"jorgebucaran/fisher"}) {
		t.Fatalf("incoming configuration not projected: %+v %v %q", fish, h.FishPluginConflict, h.FishInstalledPlugins)
	}
	if p, e := plan.Build(h, o); e != nil || len(p.Steps) == 0 || p.Later != "" {
		t.Fatal("import not planned in one review", e, p.Later)
	}
	os.Symlink(filepath.Join(old, "Documents"), filepath.Join(old, "Linked"))
	for _, folders := range [][]string{{"Linked"}, {"Music"}, {"Library"}} {
		o.ImportFolders = folders
		if _, e := i.Read(context.Background(), o); e == nil {
			t.Fatal("unusable folder accepted:", folders)
		}
	}
	if os.Geteuid() != 0 {
		os.Chmod(filepath.Join(old, "Documents", "sub"), 0)
		defer os.Chmod(filepath.Join(old, "Documents", "sub"), 0700)
		o.ImportFolders = []string{"Documents"}
		h, e := i.Read(context.Background(), o)
		if e != nil || len(h.ImportProblems) != 1 || len(h.Import) != 0 {
			t.Fatal("unreadable folder not reported", e, h.ImportProblems)
		}
	}
}

type runFunc func(context.Context, domain.Command, io.Writer) error

func (f runFunc) Run(c context.Context, d domain.Command, w io.Writer) error { return f(c, d, w) }

func TestDotfilesRead(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	source := filepath.Join(h.Home, ".local/share/chezmoi")
	for name, contents := range map[string]string{"dot_zshrc": "zsh", "private_dot_netrc.tmpl": "machine {{ .host }}", "dot_gitconfig.tmpl": "{{ output \"whoami\" }}", "encrypted_dot_secret.age": "age", "symlink_dot_vimrc": ".config/vim/vimrc\n", "exact_dot_old/dot_keep": "keep", "run_once_setup.sh": "true", ".git/config": "[core]"} {
		os.MkdirAll(filepath.Dir(filepath.Join(source, name)), 0700)
		os.WriteFile(filepath.Join(source, name), []byte(contents), 0600)
	}
	os.WriteFile(filepath.Join(h.Home, ".secret"), []byte("decrypted"), 0600)
	os.MkdirAll(filepath.Join(h.Home, ".config/vim"), 0700)
	os.WriteFile(filepath.Join(h.Home, ".config/vim/vimrc"), []byte("set nu"), 0600)
	os.Symlink(".config/vim/vimrc", filepath.Join(h.Home, ".vimrc"))
	targets := map[string]string{".zshrc": "dot_zshrc", ".netrc": "private_dot_netrc.tmpl", ".gitconfig": "dot_gitconfig.tmpl", ".secret": "encrypted_dot_secret.age", ".vimrc": "symlink_dot_vimrc", ".old/.keep": "exact_dot_old/dot_keep"}
	decrypted := sha256.Sum256([]byte("decrypted"))
	origin, state, rendered := "https://github.com/you/dotfiles.git", "", []string{}
	r := runFunc(func(_ context.Context, c domain.Command, w io.Writer) error {
		switch {
		case c.Path == "/fake/git" && c.Args[len(c.Args)-1] == "remote.origin.url":
			io.WriteString(w, origin+"\n")
		case c.Path == "/fake/git":
		case c.Path == "/fake/chezmoi" && len(c.Args) == 1 && c.Args[0] == "source-path":
			io.WriteString(w, source+"\n")
		case c.Path == "/fake/chezmoi" && c.Args[0] == "managed":
			state = c.Args[slices.Index(c.Args, "--persistent-state")+1]
			if !slices.Contains(c.Args, "--skip-secrets") {
				return fmt.Errorf("secrets not skipped")
			}
			var listed []string
			for target := range targets {
				listed = append(listed, filepath.Join(h.Home, target))
			}
			io.WriteString(w, strings.Join(listed, "\x00"))
		case c.Path == "/fake/chezmoi" && c.Args[0] == "source-path":
			for _, target := range c.Args[slices.Index(c.Args, "--")+1:] {
				rel, _ := filepath.Rel(h.Home, target)
				io.WriteString(w, filepath.Join(source, targets[rel])+"\n")
			}
		case c.Path == "/fake/chezmoi" && c.Args[0] == "cat":
			rendered = append(rendered, c.Args[len(c.Args)-1])
			io.WriteString(w, "machine example.test\n")
		case c.Path == "/fake/chezmoi" && c.Args[0] == "state":
			fmt.Fprintf(w, `{"entryState":{%q:{"type":"file","contentsSHA256":%q}}}`, filepath.Join(h.Home, ".secret"), hex.EncodeToString(decrypted[:]))
		default:
			return fmt.Errorf("unexpected command %v", c)
		}
		return nil
	})
	i := inspect.Inspector{Home: h.Home, Runner: r, Platform: func(context.Context) (domain.Host, error) { return h, nil }, AppsDirs: []string{filepath.Join(h.Home, "Applications")}, Lookup: func(p string) (string, error) {
		if p == "git" || p == "chezmoi" {
			return "/fake/" + p, nil
		}
		return "", fs.ErrNotExist
	}}
	o := plan.DefaultOptions()
	o.DotfilesRepo = "you"
	before := snapshot(t, h.Home)
	got, err := i.Read(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, h.Home)) {
		t.Fatal("inspection wrote files")
	}
	if _, err := os.Stat(filepath.Dir(state)); state == "" || !os.IsNotExist(err) {
		t.Fatal("chezmoi's temporary state was not removed:", state)
	}
	if got.DotfilesState != "cloned" || !got.DotfilesScripts || !got.DotfilesRemovals || len(got.Dotfiles) != 6 || len(got.DotfilesManual) != 0 {
		t.Fatalf("dotfiles misread: %+v %q", got.Dotfiles, got.DotfilesManual)
	}
	byName := map[string]domain.Dotfile{}
	for _, d := range got.Dotfiles {
		rel, _ := filepath.Rel(h.Home, d.Target)
		byName[rel] = d
	}
	if d := byName[".zshrc"]; d.Kind != "file" || string(d.Contents) != "zsh" || d.SHA256 == "" || d.Present || d.Interactive {
		t.Fatalf("plain file: %+v", d)
	}
	if d := byName[".netrc"]; d.Kind != "template" || string(d.Contents) != "machine example.test\n" || d.SHA256 == "" || !reflect.DeepEqual(rendered, []string{filepath.Join(h.Home, ".netrc")}) {
		t.Fatalf("template not rendered once by chezmoi: %+v %q", d, rendered)
	}
	if d := byName[".gitconfig"]; d.Kind != "template" || d.SHA256 != "" || !d.Interactive {
		t.Fatalf("a template that runs commands was rendered: %+v", d)
	}
	if d := byName[".secret"]; d.Kind != "encrypted" || d.SHA256 != "" || !d.Interactive || !d.Present {
		t.Fatalf("encrypted file chezmoi wrote not recognized: %+v", d)
	}
	if d := byName[".vimrc"]; d.Kind != "link" || !d.Present {
		t.Fatalf("matching link not recognized: %+v", d)
	}
	origin = "https://github.com/someone/dotfiles.git"
	if got, err = i.Read(context.Background(), o); err != nil || got.DotfilesState != "other" || got.DotfilesOrigin != origin {
		t.Fatal("another repository's source not reported", err, got.DotfilesState)
	}
	os.RemoveAll(source)
	if got, err = i.Read(context.Background(), o); err != nil || got.DotfilesState != "missing" {
		t.Fatal("missing source not reported", err, got.DotfilesState)
	}
}
