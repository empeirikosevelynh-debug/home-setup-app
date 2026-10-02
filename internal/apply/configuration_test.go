package apply

import (
	"context"
	"errors"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitPreservesIdentityAndIncludes(t *testing.T) {
	git, e := exec.LookPath("git")
	if e != nil {
		t.Skip("Git unavailable")
	}
	h := testutil.FreshHost(t.TempDir())
	path := filepath.Join(h.Home, ".gitconfig")
	include := filepath.Join(h.Home, "extra.gitconfig")
	os.WriteFile(include, []byte("[user]\n name = Evelyn\n email = evelyn@example.test\n"), 0600)
	os.WriteFile(path, []byte("[include]\n path = "+include+"\n[core]\n pager = less\n[credential]\n helper = osxkeychain\n"), 0600)
	h.Tools["git"] = git
	s, _ := (files.Manager{Roots: []string{h.Home}}).Inspect(path)
	h.Files[path] = s
	if e = ConfigureGit(context.Background(), h, command.ProcessRunner{}); e != nil {
		t.Fatal(e)
	}
	get := func(k string) string {
		b, e := exec.Command(git, "config", "--file", path, "--includes", "--get", k).Output()
		if e != nil {
			t.Fatal(e)
		}
		return strings.TrimSpace(string(b))
	}
	if get("user.name") != "Evelyn" || get("user.email") != "evelyn@example.test" || get("include.path") != include || get("credential.helper") != "osxkeychain" {
		t.Fatal("unrelated Git configuration changed")
	}
	if get("core.editor") != "zed --wait" || get("core.pager") != "delta" {
		t.Fatal("guide settings missing")
	}
}
func pluginHost(t *testing.T) domain.Host {
	h := testutil.FreshHost(t.TempDir())
	h.Tools["fish"] = "/fake/fish"
	return h
}
func TestFisherBootstrapBeforeManifest(t *testing.T) {
	h := pluginHost(t)
	calls := 0
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		calls++
		if strings.Contains(strings.Join(c.Args, " "), "fisher install") {
			data, e := os.ReadFile(filepath.Join(h.Home, ".config/fish/functions/fisher.fish"))
			if e != nil || !strings.Contains(string(data), "4.4.8") {
				t.Fatal("Fisher not bootstrapped first")
			}
		}
		return nil
	})
	if e := InstallPlugins(context.Background(), h, []string{"jorgebucaran/autopair.fish"}, r); e != nil || calls < 2 {
		t.Fatal(e, calls)
	}
}
func TestOnlySelectedPluginsInstalled(t *testing.T) {
	h := pluginHost(t)
	var args []string
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		if strings.Contains(strings.Join(c.Args, " "), "fisher install") {
			args = c.Args
		}
		return nil
	})
	if e := InstallPlugins(context.Background(), h, []string{"jorgebucaran/autopair.fish"}, r); e != nil {
		t.Fatal(e)
	}
	if len(args) == 0 || args[len(args)-1] != "jorgebucaran/autopair.fish" || strings.Contains(strings.Join(args, " "), "plugin-git") {
		t.Fatal("selection not respected", args)
	}
}
func TestCustomFishFilesPreserved(t *testing.T) {
	h := pluginHost(t)
	h.FishPluginConflict = true
	path := filepath.Join(h.Home, ".config/fish/functions/custom.fish")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("custom"), 0600)
	calls := 0
	e := InstallPlugins(context.Background(), h, []string{"jorgebucaran/autopair.fish"}, runFn(func(context.Context, domain.Command, io.Writer) error { calls++; return nil }))
	got, _ := os.ReadFile(path)
	if !errors.Is(e, ErrDeferred) || calls != 0 || string(got) != "custom" {
		t.Fatal(e, calls, string(got))
	}
}
func TestExistingPluginsNotRemoved(t *testing.T) {
	h := pluginHost(t)
	path := filepath.Join(h.Home, ".config/fish/fish_plugins")
	os.MkdirAll(filepath.Dir(path), 0700)
	original := []byte("# custom\nexample/custom\n")
	os.WriteFile(path, original, 0600)
	s, _ := (files.Manager{Roots: []string{h.Home}}).Inspect(path)
	h.Files[path] = s
	e := InstallPlugins(context.Background(), h, []string{"jorgebucaran/autopair.fish"}, runFn(func(context.Context, domain.Command, io.Writer) error {
		t.Fatal("existing manifest should defer")
		return nil
	}))
	got, _ := os.ReadFile(path)
	if !errors.Is(e, ErrDeferred) || string(got) != string(original) {
		t.Fatal("existing plugin manifest changed", e)
	}
}
func TestDirtyChezMoiDeferred(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	h.ChezmoiDirty = true
	e := AdoptChezmoi(context.Background(), h, []string{h.Home + "/.config/fish/config.fish"}, runFn(func(context.Context, domain.Command, io.Writer) error { t.Fatal("dirty source touched"); return nil }))
	if !errors.Is(e, ErrDeferred) {
		t.Fatal(e)
	}
}
func TestAdoptionNamesOnlyChosenFiles(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	h.Tools["chezmoi"] = "/fake/chezmoi"
	chosen := h.Home + "/.config/fish/config.fish"
	var calls []domain.Command
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error { calls = append(calls, c); return nil })
	if e := AdoptChezmoi(context.Background(), h, []string{chosen}, r); e != nil {
		t.Fatal(e)
	}
	adds := 0
	for _, c := range calls {
		if c.Args[0] == "add" {
			adds++
			if c.Args[len(c.Args)-1] != chosen || !strings.Contains(strings.Join(c.Args, " "), "--recursive=false") {
				t.Fatal("adoption is recursive or unrelated", c)
			}
		}
		if strings.Contains(strings.Join(c.Args, " "), "push") {
			t.Fatal("remote touched")
		}
	}
	if adds != 1 {
		t.Fatal(calls)
	}
}
func TestGoCustomBinAndOnlySelectedServer(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	h.Tools["go"] = "/fake/go"
	bin := h.Home + "/custom Go binaries"
	installed := false
	r := runFn(func(_ context.Context, c domain.Command, w io.Writer) error {
		if c.Path != h.Tools["go"] {
			t.Fatal("unselected server invoked")
		}
		if c.Args[0] == "env" {
			io.WriteString(w, `{"GOBIN":"`+bin+`","GOPATH":"ignored"}`)
			return nil
		}
		if strings.Join(c.Args, " ") != "install golang.org/x/tools/gopls@v0.23.0" {
			t.Fatal(c)
		}
		installed = true
		os.MkdirAll(bin, 0700)
		return os.WriteFile(filepath.Join(bin, "gopls"), []byte("fake"), 0755)
	})
	if e := InstallLanguageTools(context.Background(), h, []string{"go"}, r); e != nil || !installed {
		t.Fatal(e)
	}
	ok, e := languagesPresent(context.Background(), h, []string{"go"}, r)
	if e != nil || !ok {
		t.Fatal("custom server not verified", e)
	}
}

func TestNimServerVersionAndGlobalScope(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	h.Tools["nimble"] = "/fake/nimble"
	called := false
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		called = true
		if strings.Join(c.Args, " ") != "install -g nimlangserver@1.14.0" || c.Dir != h.Home || !c.Interactive {
			t.Errorf("unreviewed Nim install scope: %+v", c)
		}
		return nil
	})
	if e := InstallLanguageTools(context.Background(), h, []string{"nim"}, r); e != nil || !called {
		t.Fatal("Nim install missing", e)
	}
}
