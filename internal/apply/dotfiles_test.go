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

func TestDotfileStep(t *testing.T) {
	ctx := context.Background()
	h := testutil.FreshHost(t.TempDir())
	h.Tools["chezmoi"] = "/opt/homebrew/bin/chezmoi"
	m := files.Manager{Roots: []string{h.Home}}
	netrc, ssh := filepath.Join(h.Home, ".netrc"), filepath.Join(h.Home, ".ssh/config")
	os.WriteFile(netrc, []byte("old\n"), 0644)
	old := sha256.Sum256([]byte("old\n"))
	var calls []domain.Command
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		calls = append(calls, c)
		target := c.Args[len(c.Args)-1]
		return os.WriteFile(target, []byte("from chezmoi\n"), 0600)
	})
	handler := OptionalHandlers(r, m)["dotfile"]
	step := func(path string, before []byte, decision domain.FileDecision) domain.Step {
		change := domain.FileChange{Path: path, Decision: decision}
		if before != nil {
			sum := sha256.Sum256(before)
			change.BeforeExists, change.BeforeSHA256 = true, hex.EncodeToString(sum[:])
		}
		return domain.Step{ID: "dotfile:" + path, Kind: "dotfile", File: &change, Command: plan.DotfileCommand(path, true), Check: domain.Check{Kind: "dotfile", Target: path}}
	}
	backups := filepath.Join(h.Home, "sessions/backups")
	replace := step(netrc, []byte("old\n"), domain.Replace)
	result, err := handler.Apply(ctx, h, domain.Plan{}, replace, backups)
	if err != nil || result.BackupPath == "" {
		t.Fatal(err, result)
	}
	if saved, _ := os.ReadFile(result.BackupPath); !bytes.Equal(saved, []byte("old\n")) || !strings.Contains(result.BackupPath, hex.EncodeToString(old[:])) {
		t.Fatal("no backup before chezmoi replaced the file")
	}
	if got, _ := os.ReadFile(netrc); string(got) != "from chezmoi\n" || len(calls) != 1 || calls[0].Path != "/opt/homebrew/bin/chezmoi" || !calls[0].Interactive || strings.Join(calls[0].Args, " ") != "apply --force --exclude=scripts,remove,dirs --no-pager -- "+netrc {
		t.Fatalf("chezmoi not run for the reviewed target: %+v", calls)
	}
	// A new file in a missing folder: the folder is created, nothing else.
	if _, err := handler.Apply(ctx, h, domain.Plan{}, step(ssh, nil, domain.Create), backups); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Dir(ssh)); err != nil || st.Mode().Perm() != 0700 {
		t.Fatal("parent folder not created privately", err)
	}
	// A file that changed since the review, or appeared, is refused.
	calls = nil
	os.WriteFile(netrc, []byte("edited\n"), 0644)
	if _, err := handler.Apply(ctx, h, domain.Plan{}, replace, backups); err == nil || !strings.Contains(err.Error(), "changed since preview") {
		t.Fatal("changed file replaced:", err)
	}
	if _, err := handler.Apply(ctx, h, domain.Plan{}, step(ssh, nil, domain.Create), backups); err == nil {
		t.Fatal("a file that appeared was overwritten")
	}
	if len(calls) != 0 {
		t.Fatal("chezmoi ran after a refusal", calls)
	}
	// Verification reads what inspection found chezmoi wrote.
	h.Dotfiles = []domain.Dotfile{{Target: netrc, Kind: "encrypted"}}
	if ok, _ := handler.Verify(ctx, h, domain.Plan{}, replace, ""); ok {
		t.Fatal("verified before chezmoi wrote it")
	}
	h.Dotfiles[0].Present = true
	if ok, _ := handler.Verify(ctx, h, domain.Plan{}, replace, ""); !ok {
		t.Fatal("not verified after chezmoi wrote it")
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
