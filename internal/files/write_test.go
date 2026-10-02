package files

import (
	"bytes"
	"errors"
	"golden-gate-setup/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func change(t *testing.T, root string, original []byte) (Manager, domain.FileChange, string) {
	t.Helper()
	m := Manager{Roots: []string{root}}
	p := filepath.Join(root, ".config/zed/settings.json")
	if original != nil {
		os.MkdirAll(filepath.Dir(p), 0700)
		os.WriteFile(p, original, 0640)
	}
	s, e := m.Inspect(p)
	if e != nil {
		t.Fatal(e)
	}
	return m, domain.FileChange{Path: p, BeforeExists: s.Exists, BeforeSHA256: s.SHA256, Mode: 0644, Desired: []byte("new settings"), Decision: domain.Create}, filepath.Join(root, "backups")
}
func TestCreateAndRepeat(t *testing.T) {
	m, c, b := change(t, t.TempDir(), nil)
	if _, e := m.Apply(c, b); e != nil {
		t.Fatal(e)
	}
	r, e := m.Apply(c, b)
	if e != nil || r.Status != "unchanged" {
		t.Fatal(r, e)
	}
}
func TestDeclinedConflictUnchanged(t *testing.T) {
	original := []byte("// retain comments\n{\"custom\": true}")
	m, c, b := change(t, t.TempDir(), original)
	c.Decision = domain.Preserve
	if _, e := m.Apply(c, b); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(c.Path)
	if !bytes.Equal(got, original) {
		t.Fatal("preserved JSONC changed")
	}
}
func TestBackupBeforeReplace(t *testing.T) {
	original := []byte("original")
	m, c, b := change(t, t.TempDir(), original)
	c.Decision = domain.Replace
	m.BeforeRename = func() error {
		entries, e := os.ReadDir(b)
		if e != nil || len(entries) != 1 {
			t.Fatal("backup must exist before rename")
		}
		live, _ := os.ReadFile(c.Path)
		if !bytes.Equal(live, original) {
			t.Fatal("live changed before backup")
		}
		return nil
	}
	r, e := m.Apply(c, b)
	if e != nil {
		t.Fatal(e)
	}
	backup, _ := os.ReadFile(r.BackupPath)
	live, _ := os.ReadFile(c.Path)
	if r.BackupPath == "" || !bytes.Equal(backup, original) || !bytes.Equal(live, c.Desired) {
		t.Fatal("backup or live incorrect")
	}
	st, _ := os.Stat(r.BackupPath)
	if st.Mode().Perm() != 0600 {
		t.Fatal("backup is not private")
	}
	st, _ = os.Stat(c.Path)
	if st.Mode().Perm() != 0640 {
		t.Fatal("live mode lost")
	}
}
func TestConcurrentEditRejected(t *testing.T) {
	m, c, b := change(t, t.TempDir(), []byte("before"))
	c.Decision = domain.Replace
	os.WriteFile(c.Path, []byte("later"), 0600)
	if _, e := m.Apply(c, b); e == nil {
		t.Fatal("concurrent edit accepted")
	}
	got, _ := os.ReadFile(c.Path)
	if string(got) != "later" {
		t.Fatal("edited file changed")
	}
}
func TestSymlinkedParentRejected(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	os.Symlink(outside, filepath.Join(root, ".config"))
	m := Manager{Roots: []string{root}}
	c := domain.FileChange{Path: filepath.Join(root, ".config/escape"), Desired: []byte("oops"), Decision: domain.Create}
	if _, e := m.Apply(c, filepath.Join(root, "backup")); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(e) {
		t.Fatal("escaped root")
	}
}
func TestWriteFailurePreservesOriginal(t *testing.T) {
	original := []byte("before")
	m, c, b := change(t, t.TempDir(), original)
	c.Decision = domain.Replace
	m.BeforeRename = func() error { return errors.New("disk failure") }
	r, e := m.Apply(c, b)
	if e == nil || r.Status != "failed" || r.BackupPath == "" {
		t.Fatal(r, e)
	}
	got, _ := os.ReadFile(c.Path)
	if !bytes.Equal(got, original) {
		t.Fatal("original lost")
	}
}
func TestRootAncestorSymlinkRejected(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(outside, "workspace"), 0700)
	os.Symlink(outside, filepath.Join(home, "link"))
	root := filepath.Join(home, "link/workspace")
	m := Manager{Roots: []string{root}}
	_, e := m.Apply(domain.FileChange{Path: filepath.Join(root, "escaped"), Decision: domain.Create, Desired: []byte("bad")}, filepath.Join(root, "backups"))
	if e == nil {
		t.Fatal("root ancestor symlink accepted")
	}
}
