//go:build !windows

package integration

import (
	"bytes"
	"context"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil/sandbox"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, contents string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0700)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func contents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func kinds(p domain.Plan) map[string]int {
	r := map[string]int{}
	for _, s := range p.Steps {
		r[s.Kind]++
		if s.File != nil && s.File.Source != nil {
			r["restore"]++
		}
	}
	return r
}

// TestBringOverPreviousMac moves a previous Mac's apps, dotfiles repository
// and home folder into a fresh home, one review at a time.
func TestBringOverPreviousMac(t *testing.T) {
	s := sandbox.NewSandbox(t.TempDir())
	x := executor(s)
	old := t.TempDir()
	write(t, filepath.Join(old, "Documents/report.txt"), "report")
	write(t, filepath.Join(old, "Documents/notes.txt"), "theirs")
	write(t, filepath.Join(old, ".zshrc"), "old zsh")
	write(t, filepath.Join(old, ".editorconfig"), "root = true")
	write(t, filepath.Join(old, ".ssh/config"), "Host old")
	brewfile := filepath.Join(old, "Golden Gate Recovery/2026-09-30-abc/Homebrew-full.Brewfile")
	write(t, brewfile, "tap \"you/tools\"\nbrew \"jq\"\ncask \"firefox\"\n")
	write(t, filepath.Join(s.Home, "Documents/notes.txt"), "mine")
	s.Dotfiles = map[string]string{"dot_zshrc": "repo zsh", "dot_config/starship.toml": "repo starship", "dot_gitconfig.tmpl": "{{ .email }}"}
	o := plan.DefaultOptions()
	o.CaptureInventory, o.PrepareRecovery = false, false
	o.DotfilesRepo = "https://example.test/you/dotfiles.git"
	o.ImportFrom, o.ImportFolders = old, []string{"Documents", ".", ".ssh"}
	o.PreviousBrewfile, o.PreviousPackages = brewfile, []string{"tap:you/tools", "formula:jq", "cask:firefox"}
	ctx := context.Background()

	// First review: packages and the previous Mac's apps, then the clone.
	p := preview(t, s, o)
	if k := kinds(p); p.Later == "" || k["chezmoi-init"] != 1 || k["tap"] != 1 || k["import"]+k["file"] != 0 || p.Steps[len(p.Steps)-1].Kind != "chezmoi-init" {
		t.Fatalf("first review: %v %q", k, p.Later)
	}
	if r, err := x.Execute(ctx, p, nil); err != nil || r.Status != "complete" {
		t.Fatal(err, r)
	}

	// Second review: the import, which leaves the repository's files alone,
	// and the restore. Hidden folders leave the rest for a third review.
	p = preview(t, s, o)
	if k := kinds(p); p.Later == "" || k["import"] != 3 || k["restore"] != 2 || k["package"]+k["chezmoi-init"] != 0 {
		t.Fatalf("second review: %v %q", k, p.Later)
	}
	if r, err := x.Execute(ctx, p, nil); err != nil || r.Status != "complete" {
		t.Fatal(err, r)
	}
	for path, want := range map[string]string{".zshrc": "repo zsh", ".editorconfig": "root = true", ".ssh/config": "Host old", "Documents/report.txt": "report", "Documents/notes.txt": "mine", ".config/starship.toml": "repo starship"} {
		if got := contents(t, filepath.Join(s.Home, path)); got != want {
			t.Fatalf("%s: %q", path, got)
		}
	}
	if got := contents(t, filepath.Join(s.Home, "Imported conflicts", o.RecoveryDate, "Documents/notes.txt")); got != "theirs" {
		t.Fatal("their version not saved aside:", got)
	}
	sessions, _ := filepath.Glob(filepath.Join(s.Home, "sessions", "*.json"))
	for _, session := range sessions {
		if data, _ := os.ReadFile(session); bytes.Contains(data, []byte("repo zsh")) {
			t.Fatal("restored contents entered a session record")
		}
	}

	// Third review: the rest of setup, keeping the repository's files.
	p = preview(t, s, o)
	k := kinds(p)
	if p.Later != "" || k["import"]+k["restore"] != 0 || k["file"] == 0 {
		t.Fatalf("third review: %v %q", k, p.Later)
	}
	for _, step := range p.Steps {
		if step.ID == "file:"+filepath.Join(s.Home, ".config/starship.toml") {
			t.Fatal("starter template planned over the repository's file")
		}
	}
	if r, err := x.Execute(ctx, p, nil); err != nil || r.Status != "complete" {
		t.Fatal(err, r)
	}
	if contents(t, filepath.Join(s.Home, ".config/starship.toml")) != "repo starship" {
		t.Fatal("repository file replaced")
	}

	// Nothing is left to bring over.
	s.ResetCalls()
	p = preview(t, s, o)
	if k := kinds(p); k["import"]+k["restore"]+k["chezmoi-init"]+k["tap"] != 0 {
		h, _ := s.Inspect(ctx, o)
		t.Fatalf("fourth review: %v %+v %+v", k, p.Steps, h.Import)
	}
	if r, err := x.Execute(ctx, p, nil); err != nil || r.Status != "complete" {
		t.Fatal(err, r)
	}
	for _, c := range s.Calls() {
		if len(c.Args) > 0 && (c.Args[0] == "install" || c.Args[0] == "init" || c.Args[0] == "tap" && len(c.Args) > 1) {
			t.Fatal("repeat mutation", c)
		}
	}
}
