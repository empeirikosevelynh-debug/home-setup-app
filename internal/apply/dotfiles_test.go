//go:build !windows

package apply

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreReadsTheReviewedSource(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	source := filepath.Join(h.Home, ".local/share/chezmoi")
	secret := []byte("machine example.test password hunter2\n")
	os.MkdirAll(source, 0700)
	os.WriteFile(filepath.Join(source, "private_dot_netrc"), secret, 0600)
	target := filepath.Join(h.Home, ".netrc")
	os.WriteFile(target, []byte("old\n"), 0644)
	sum := sha256.Sum256(secret)
	h.DotfilesState = "cloned"
	h.Dotfiles = []domain.Dotfile{{Target: target, Source: domain.FileSource{Root: source, Path: filepath.Join(source, "private_dot_netrc"), SHA256: hex.EncodeToString(sum[:])}, Mode: 0600, Contents: secret}}
	old := sha256.Sum256([]byte("old\n"))
	h.Files[target] = domain.FileState{Path: target, Exists: true, Mode: 0644, SHA256: hex.EncodeToString(old[:]), Contents: []byte("old\n")}
	o := plan.DefaultOptions()
	o.Apps, o.Plugins, o.ConfigureGit, o.AdoptChezmoi, o.CaptureInventory, o.PrepareRecovery, o.Languages = nil, nil, false, false, false, false, nil
	o.DotfilesRepo, o.FileChoices = "you", map[string]domain.FileDecision{target: domain.Replace}
	for _, token := range plan.Core {
		h.Packages["formula:"+token] = domain.InstalledPackage{Version: "99.0"}
	}
	p, err := plan.Build(h, o)
	if err != nil || len(p.Steps) == 0 {
		t.Fatal(err, p.Steps)
	}
	p.Accepted = true
	m := files.Manager{Roots: []string{h.Home}}
	store := SessionStore{Dir: filepath.Join(h.Home, "sessions")}
	x := &Executor{Inspect: func(context.Context, domain.Options) (domain.Host, error) {
		current := h
		current.Files = map[string]domain.FileState{}
		for k, v := range h.Files {
			current.Files[k] = v
		}
		if f, err := m.Inspect(target); err == nil {
			current.Files[target] = f
		}
		return current, nil
	}, Build: plan.Build, Runner: runFn(func(context.Context, domain.Command, io.Writer) error { return nil }), Files: m, Store: store, Handlers: ConfigurationHandlers(nil)}
	report, err := x.Execute(context.Background(), p, nil)
	if err != nil || report.Status != "complete" {
		t.Fatal(err, report)
	}
	st, err := os.Stat(target)
	if got, _ := os.ReadFile(target); err != nil || !bytes.Equal(got, secret) || st.Mode().Perm() != 0600 {
		t.Fatal("restored file wrong", err)
	}
	record, _ := os.ReadFile(store.Path(p.ID))
	if len(record) == 0 || bytes.Contains(record, []byte("hunter2")) {
		t.Fatal("restored contents entered the session record")
	}
	// A source edited after review is refused, even past the first check.
	os.WriteFile(target, []byte("old\n"), 0644)
	os.Chmod(target, 0644)
	os.WriteFile(filepath.Join(source, "private_dot_netrc"), []byte("edited\n"), 0600)
	if _, err := x.Execute(context.Background(), p, nil); err == nil || !strings.Contains(err.Error(), "dotfiles changed since preview") {
		t.Fatal("edited source accepted:", err)
	}
}

func TestCloneStep(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	h.Tools["chezmoi"] = "/opt/homebrew/bin/chezmoi"
	var calls []domain.Command
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error { calls = append(calls, c); return nil })
	handler := OptionalHandlers(r, files.Manager{Roots: []string{h.Home}})["chezmoi-init"]
	step := domain.Step{ID: "chezmoi-init", Kind: "chezmoi-init", Command: &domain.Command{Args: []string{"init", "--", "you"}, Interactive: true}}
	h.DotfilesState = "missing"
	if ok, _ := handler.Verify(context.Background(), h, domain.Plan{}, step, ""); ok {
		t.Fatal("missing clone verified")
	}
	if _, err := handler.Apply(context.Background(), h, domain.Plan{}, step, ""); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Path != "/opt/homebrew/bin/chezmoi" || strings.Join(calls[0].Args, " ") != "init -- you" || !calls[0].Interactive {
		t.Fatalf("%+v", calls)
	}
	h.DotfilesState = "cloned"
	if ok, _ := handler.Verify(context.Background(), h, domain.Plan{}, step, ""); !ok {
		t.Fatal("clone not verified")
	}
}
