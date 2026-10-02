package plan_test

import (
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestSameRepo(t *testing.T) {
	for repo, origin := range map[string]string{
		"you":                                   "https://github.com/you/dotfiles.git",
		"You/Dots":                              "git@github.com:you/dots.git",
		"gitlab.com/you/dots":                   "https://gitlab.com/you/dots",
		"sr.ht/~you":                            "https://git.sr.ht/~you/dotfiles",
		"https://example.test/you/dotfiles.git": "ssh://git@example.test/you/dotfiles",
	} {
		if !plan.SameRepo(origin, repo) {
			t.Fatal(repo, "does not match", origin)
		}
	}
	for repo, origin := range map[string]string{"you": "https://github.com/someone/dotfiles.git", "you/dots": "", "you/a": "https://github.com/you/b"} {
		if plan.SameRepo(origin, repo) {
			t.Fatal(repo, "matches", origin)
		}
	}
	for _, bad := range []string{"", "-x", "--upgrade", "you dots", "you\x1b[2J"} {
		if plan.ValidDotfilesRepo(bad) == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestSourceAttributes(t *testing.T) {
	type want struct {
		plain, create bool
		mode          fs.FileMode
	}
	for name, w := range map[string]want{
		"dot_zshrc":                 {true, false, 0644},
		"private_dot_netrc":         {true, false, 0600},
		"executable_dot_tool":       {true, false, 0755},
		"private_executable_tool":   {true, false, 0700},
		"readonly_dot_profile":      {true, false, 0444},
		"create_private_dot_npmrc":  {true, true, 0600},
		"dot_notes.tmpl.literal":    {true, false, 0644},
		"literal_run_me":            {true, false, 0644},
		"dot_gitconfig.tmpl":        {},
		"encrypted_private_dot_key": {},
		"modify_dot_settings":       {},
		"symlink_dot_vimrc":         {},
		"run_once_install.sh":       {},
		"remove_dot_old":            {},
	} {
		plain, create, mode := plan.SourceAttributes(name, 10)
		if plain != w.plain || create != w.create || mode != w.mode {
			t.Fatal(name, plain, create, mode)
		}
	}
	if plain, _, _ := plan.SourceAttributes("dot_hushlogin", 0); plain {
		t.Fatal("an empty file without empty_ is removed by chezmoi, not written")
	}
	if plain, _, mode := plan.SourceAttributes("empty_dot_hushlogin", 0); !plain || mode != 0644 {
		t.Fatal("empty_ file not restored")
	}
}

func dotfilesHost(t *testing.T) (domain.Host, domain.Options) {
	h := testutil.FreshHost(t.TempDir())
	o := plan.DefaultOptions()
	o.RecoveryDate = "2026-10-02"
	o.DotfilesRepo = "you"
	source := filepath.Join(h.Home, ".local/share/chezmoi")
	restore := func(target, name, sum string, mode fs.FileMode, create bool) domain.Dotfile {
		return domain.Dotfile{Target: filepath.Join(h.Home, target), Source: domain.FileSource{Root: source, Path: filepath.Join(source, name), SHA256: sum}, Mode: mode, Create: create, Contents: []byte("from the repository")}
	}
	h.DotfilesState = "cloned"
	h.Dotfiles = []domain.Dotfile{
		restore(".zshrc", "dot_zshrc", "zshrc", 0644, false),
		restore(".config/fish/config.fish", "dot_config/fish/config.fish", "fish", 0644, false),
		restore(".npmrc", "create_private_dot_npmrc", "npmrc", 0600, true),
		restore(".netrc", "private_dot_netrc", "netrc", 0600, false),
	}
	h.DotfilesManual = []string{filepath.Join(h.Home, ".gitconfig")}
	h.DotfilesScripts = true
	for _, existing := range []string{".config/fish/config.fish", ".npmrc"} {
		path := filepath.Join(h.Home, existing)
		h.Files[path] = domain.FileState{Path: path, Exists: true, Mode: 0644, SHA256: "mine", Contents: []byte("mine")}
	}
	netrc := filepath.Join(h.Home, ".netrc")
	h.Files[netrc] = domain.FileState{Path: netrc, Exists: true, Mode: 0600, SHA256: "netrc"}
	return h, o
}

func TestDotfilesClonedFirst(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	h.DotfilesState = "missing"
	o := plan.DefaultOptions()
	o.DotfilesRepo, o.ImportFrom, o.ImportFolders = "you", oldHome, []string{"Documents"}
	p, err := plan.Build(h, o)
	if err != nil || p.Later == "" {
		t.Fatal(err, p.Later)
	}
	clone := steps(p, "chezmoi-init")
	if len(clone) != 1 || clone[0].Label != "Clone your dotfiles from github.com/you/dotfiles with chezmoi" || strings.Join(clone[0].Command.Args, " ") != "init -- you" || !clone[0].Command.Interactive || p.Steps[len(p.Steps)-1].Kind != "chezmoi-init" {
		t.Fatalf("clone not planned last: %+v", p.Steps)
	}
	if len(steps(p, "import"))+len(steps(p, "file")) != 0 {
		t.Fatal("import or configuration planned before the repository is known")
	}
	h.DotfilesState = "waiting"
	if p, err = plan.Build(h, o); err != nil || p.Later == "" || len(steps(p, "chezmoi-init")) != 0 {
		t.Fatal("a cloned repository is listed once chezmoi is installed", err)
	}
	h.DotfilesState, h.DotfilesOrigin = "other", "https://github.com/someone/dotfiles.git"
	o.ImportFrom, o.ImportFolders = "", nil
	if p, err = plan.Build(h, o); err != nil || p.Supported || !strings.Contains(strings.Join(p.Problems, "\n"), "already holds other dotfiles (origin https://github.com/someone/dotfiles.git)") {
		t.Fatal(err, p.Problems)
	}
}

func TestDotfilesRestorePlanned(t *testing.T) {
	h, o := dotfilesHost(t)
	p, err := plan.Build(h, o)
	if err != nil || p.Later != "" {
		t.Fatal(err, p.Later)
	}
	var restores []domain.Step
	for _, s := range steps(p, "file") {
		if s.File.Source != nil {
			restores = append(restores, s)
		}
	}
	zshrc := filepath.Join(h.Home, ".zshrc")
	if len(restores) != 1 || restores[0].File.Path != zshrc || restores[0].File.Desired != nil || restores[0].Check.Expected != "zshrc" || restores[0].File.Decision != domain.Create || restores[0].Label != "Restore ~/.zshrc from your dotfiles" {
		t.Fatalf("restores: %+v", restores)
	}
	for _, s := range p.Steps {
		if s.ID == "file:"+filepath.Join(h.Home, ".config/fish/config.fish") || s.Kind == "git" {
			t.Fatal("the starter configuration replaced what the repository manages:", s.ID)
		}
	}
	kept := task(p, "dotfiles-kept")
	if kept == nil || !strings.Contains(kept.Instructions, "~/.config/fish/config.fish") || strings.Contains(kept.Instructions, ".npmrc") {
		t.Fatalf("kept files: %+v", kept)
	}
	rest := task(p, "dotfiles-chezmoi")
	if rest == nil || !strings.Contains(rest.Instructions, "~/.gitconfig") || !strings.Contains(rest.Instructions, "scripts") || task(p, "git-dotfiles") == nil {
		t.Fatalf("follow-ups: %+v", p.ManualTasks)
	}
	// An approved replacement restores the repository's version.
	o.FileChoices[filepath.Join(h.Home, ".config/fish/config.fish")] = domain.Replace
	if p, err = plan.Build(h, o); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range steps(p, "file") {
		found = found || s.File.Source != nil && s.File.Decision == domain.Replace && s.File.BeforeSHA256 == "mine"
	}
	if !found {
		t.Fatal("approved replacement not planned")
	}
}

func TestImportLeavesDotfilesAlone(t *testing.T) {
	h, o := dotfilesHost(t)
	o.ImportFrom = oldHome
	top, local := plan.ImportJob(h, o, "."), plan.ImportJob(h, o, ".local")
	for _, want := range []string{".zshrc", ".netrc", ".gitconfig"} {
		found := false
		for _, e := range top.Exclude {
			found = found || e == filepath.Join(h.Home, want)
		}
		if !found {
			t.Fatal(want, "not excluded from", top.Exclude)
		}
	}
	if len(local.Exclude) != 1 || local.Exclude[0] != filepath.Join(h.Home, ".local/share/chezmoi") {
		t.Fatal("chezmoi's source not excluded:", local.Exclude)
	}
	if documents := plan.ImportJob(h, o, "Documents"); len(documents.Exclude) != 0 {
		t.Fatal(documents.Exclude)
	}
}

func TestWindowsRefusesDotfiles(t *testing.T) {
	o := plan.DefaultOptionsFor("windows")
	o.DotfilesRepo = "you"
	if _, err := plan.Build(testutil.FreshWindowsHost(`C:\Users\you`), o); err == nil || !strings.Contains(err.Error(), "not available on Windows") {
		t.Fatal(err)
	}
}
