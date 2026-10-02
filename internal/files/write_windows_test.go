package files

import (
	"context"
	"errors"
	"golden-gate-setup/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func windowsChange(t *testing.T, path, desired string) domain.FileChange {
	t.Helper()
	before, e := snapshot(path)
	if e != nil {
		t.Fatal(e)
	}
	decision := domain.Create
	if before.Exists {
		decision = domain.Replace
	}
	return domain.FileChange{Path: path, BeforeExists: before.Exists, BeforeSHA256: before.SHA256, Desired: []byte(desired), Mode: 0644, Decision: decision}
}

func TestWindowsCreateReplaceWithBackup(t *testing.T) {
	home := t.TempDir()
	m := Manager{Roots: []string{home}}
	path := filepath.Join(home, ".config", "starship.toml")
	backups := filepath.Join(home, "backups")
	r, e := m.ApplyContext(context.Background(), windowsChange(t, path, "first"), backups)
	if e != nil || r.Status != "applied" {
		t.Fatal(r, e)
	}
	if r, e = m.ApplyContext(context.Background(), windowsChange(t, path, "first"), backups); e != nil || r.Status != "unchanged" {
		t.Fatal("repeat changed the file", r, e)
	}
	r, e = m.ApplyContext(context.Background(), windowsChange(t, path, "second"), backups)
	if e != nil || r.Status != "applied" || r.BackupPath == "" {
		t.Fatal(r, e)
	}
	got, _ := os.ReadFile(path)
	saved, _ := os.ReadFile(r.BackupPath)
	if string(got) != "second" || string(saved) != "first" {
		t.Fatal("replacement or backup wrong", string(got), string(saved))
	}
}
func TestWindowsCreateNeverOverwrites(t *testing.T) {
	home := t.TempDir()
	m := Manager{Roots: []string{home}}
	path := filepath.Join(home, "settings.json")
	change := windowsChange(t, path, "ours")
	os.WriteFile(path, []byte("appeared later"), 0644)
	if _, e := m.ApplyContext(context.Background(), change, filepath.Join(home, "backups")); e == nil {
		t.Fatal("file created after the preview was overwritten")
	}
	m.BeforeRename = func() error { return os.WriteFile(path, []byte("edited during apply"), 0644) }
	os.Remove(path)
	if _, e := m.ApplyContext(context.Background(), windowsChange(t, path, "ours"), filepath.Join(home, "backups")); e == nil {
		t.Fatal("concurrent edit was overwritten")
	}
	if got, _ := os.ReadFile(path); string(got) != "edited during apply" {
		t.Fatal("concurrent edit lost", string(got))
	}
}
func TestWindowsJunctionRejected(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(home, "config")
	if out, e := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); e != nil {
		t.Skip("cannot create a junction here:", string(out))
	}
	m := Manager{Roots: []string{home}}
	path := filepath.Join(link, "settings.json")
	if _, e := m.ApplyContext(context.Background(), domain.FileChange{Path: path, Desired: []byte("x"), Decision: domain.Create}, filepath.Join(home, "backups")); !IsRedirect(e) {
		t.Fatal("write followed a junction", e)
	}
	if _, e := m.Inspect(path); !IsRedirect(e) {
		t.Fatal("inspection followed a junction", e)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("file written outside the reviewed root")
	}
}
func TestWindowsLockExclusive(t *testing.T) {
	m := Manager{Roots: []string{t.TempDir()}}
	path := filepath.Join(m.Roots[0], "sessions", "apply.lock")
	unlock, e := m.Lock(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Lock(path); e == nil {
		t.Fatal("second lock acquired")
	}
	unlock()
	again, e := m.Lock(path)
	if e != nil {
		t.Fatal("lock not released", e)
	}
	again()
}
func TestWindowsOutsideRootRejected(t *testing.T) {
	m := Manager{Roots: []string{t.TempDir()}}
	_, e := m.ApplyContext(context.Background(), domain.FileChange{Path: filepath.Join(t.TempDir(), "x"), Desired: []byte("x"), Decision: domain.Create}, t.TempDir())
	if e == nil || errors.Is(e, os.ErrNotExist) {
		t.Fatal("write outside reviewed roots", e)
	}
}
