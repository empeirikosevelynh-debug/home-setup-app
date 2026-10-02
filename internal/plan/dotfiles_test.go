package plan_test

import (
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
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

func TestSourceKind(t *testing.T) {
	type want struct {
		kind       string
		create, ok bool
	}
	for name, w := range map[string]want{
		"dot_zshrc":                        {"file", false, true},
		"private_executable_tool":          {"file", false, true},
		"create_private_dot_npmrc":         {"file", true, true},
		"dot_notes.tmpl.literal":           {"file", false, true},
		"literal_run_me":                   {"file", false, true},
		"dot_gitconfig.tmpl":               {"template", false, true},
		"create_dot_npmrc.tmpl":            {"template", true, true},
		"encrypted_private_dot_key.age":    {"encrypted", false, true},
		"modify_dot_settings":              {"modify", false, true},
		"symlink_dot_vimrc":                {"link", false, true},
		"symlink_dot_vimrc.tmpl":           {"link-template", false, true},
		"run_once_install.sh":              {"", false, false},
		"remove_dot_old":                   {"", false, false},
		"encrypted_private_dot_netrc.tmpl": {"encrypted", false, true},
	} {
		kind, create, ok := plan.SourceKind(name, 10)
		if kind != w.kind || create != w.create || ok != w.ok {
			t.Fatal(name, kind, create, ok)
		}
	}
	if _, _, ok := plan.SourceKind("dot_hushlogin", 0); ok {
		t.Fatal("an empty file without empty_ is removed by chezmoi, not written")
	}
	if kind, _, ok := plan.SourceKind("empty_dot_hushlogin", 0); !ok || kind != "file" {
		t.Fatal("empty_ file not offered")
	}
}

func dotfilesHost(t *testing.T) (domain.Host, domain.Options) {
	h := testutil.FreshHost(t.TempDir())
	o := plan.DefaultOptions()
	o.RecoveryDate = "2026-10-02"
	o.DotfilesRepo = "you"
	dotfile := func(target, kind, sum string, create bool) domain.Dotfile {
		return domain.Dotfile{Target: filepath.Join(h.Home, target), Kind: kind, SHA256: sum, Create: create, Interactive: sum == "", Contents: []byte("from the repository")}
	}
	h.DotfilesState = "cloned"
	h.Dotfiles = []domain.Dotfile{
		dotfile(".zshrc", "file", "zshrc", false),
		dotfile(".gitconfig", "template", "gitconfig", false),
		dotfile(".ssh/config", "encrypted", "", false),
		dotfile(".config/fish/config.fish", "file", "fish", false),
		dotfile(".npmrc", "file", "npmrc", true),
		dotfile(".netrc", "encrypted", "", false),
		dotfile(".profile", "file", "profile", false),
	}
	h.Dotfiles[6].Present = true
	h.DotfilesManual = []string{"/etc/outside"}
	h.DotfilesScripts, h.DotfilesRemovals = true, true
	for _, existing := range []string{".config/fish/config.fish", ".npmrc", ".netrc"} {
		path := filepath.Join(h.Home, existing)
		h.Files[path] = domain.FileState{Path: path, Exists: true, Mode: 0644, SHA256: "mine", Contents: []byte("mine")}
	}
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

func TestDotfilesPlanned(t *testing.T) {
	h, o := dotfilesHost(t)
	p, err := plan.Build(h, o)
	if err != nil || p.Later != "" {
		t.Fatal(err, p.Later)
	}
	labels := map[string]domain.Step{}
	for _, s := range steps(p, "dotfile") {
		labels[s.Label] = s
	}
	want := []string{"chezmoi: create ~/.zshrc", "chezmoi: create ~/.gitconfig (template)", "chezmoi: create ~/.ssh/config (encrypted; may ask for your passphrase)"}
	if len(labels) != len(want) {
		t.Fatalf("dotfile steps: %+v", labels)
	}
	for _, label := range want {
		if _, ok := labels[label]; !ok {
			t.Fatalf("missing %q in %+v", label, labels)
		}
	}
	zshrc := labels[want[0]]
	if zshrc.File.Desired != nil || zshrc.Check.Expected != "zshrc" || strings.Join(zshrc.Command.Args, " ") != "apply --force --exclude=scripts,remove,dirs --no-pager --no-tty -- "+filepath.Join(h.Home, ".zshrc") || zshrc.Command.Interactive {
		t.Fatalf("%+v", zshrc)
	}
	if ssh := labels[want[2]]; !ssh.Command.Interactive || ssh.Check.Expected != "" {
		t.Fatalf("encrypted file not handed the terminal: %+v", ssh)
	}
	for _, s := range p.Steps {
		if s.ID == "file:"+filepath.Join(h.Home, ".config/fish/config.fish") || s.Kind == "git" {
			t.Fatal("the starter configuration replaced what the repository manages:", s.ID)
		}
	}
	kept := task(p, "dotfiles-kept")
	if kept == nil || !strings.Contains(kept.Instructions, "~/.config/fish/config.fish") || !strings.Contains(kept.Instructions, "~/.netrc") || strings.Contains(kept.Instructions, ".npmrc") {
		t.Fatalf("kept files: %+v", kept)
	}
	for _, id := range []string{"dotfiles-chezmoi", "dotfiles-scripts", "dotfiles-removals", "git-dotfiles"} {
		if task(p, id) == nil {
			t.Fatal("missing follow-up", id, p.ManualTasks)
		}
	}
	// Approved replacements are applied by chezmoi after a backup, even when
	// the repository's version is known only once applied.
	o.FileChoices[filepath.Join(h.Home, ".config/fish/config.fish")] = domain.Replace
	o.FileChoices[filepath.Join(h.Home, ".netrc")] = domain.Replace
	if p, err = plan.Build(h, o); err != nil {
		t.Fatal(err)
	}
	replaced := 0
	for _, s := range steps(p, "dotfile") {
		if s.File.Decision == domain.Replace && s.File.BeforeSHA256 == "mine" && strings.Contains(s.Label, "backup saved first") {
			replaced++
		}
	}
	if replaced != 2 || task(p, "dotfiles-kept") != nil {
		t.Fatal("approved replacements not planned", replaced)
	}
}

func TestImportLeavesDotfilesAlone(t *testing.T) {
	h, o := dotfilesHost(t)
	o.ImportFrom = oldHome
	top, local := plan.ImportJob(h, o, "."), plan.ImportJob(h, o, ".local")
	for _, want := range []string{".zshrc", ".netrc", ".gitconfig", ".profile"} {
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
	config := plan.ImportJob(h, o, ".config")
	for _, want := range []string{"chezmoi/chezmoi.toml", "chezmoi/chezmoistate.boltdb", "fish/config.fish"} {
		found := false
		for _, e := range config.Exclude {
			found = found || e == filepath.Join(h.Home, ".config", want)
		}
		if !found {
			t.Fatal(want, "not excluded from", config.Exclude)
		}
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
