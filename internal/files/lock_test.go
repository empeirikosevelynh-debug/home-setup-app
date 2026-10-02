//go:build !windows

package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveLockAndRelease(t *testing.T) {
	root := t.TempDir()
	m := Manager{Roots: []string{root}}
	path := filepath.Join(root, "sessions", "apply.lock")
	release, e := m.Lock(path)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if second, e := m.Lock(path); e == nil {
		second()
		t.Fatal("overlapping installers accepted")
	}
	release()
	third, e := m.Lock(path)
	if e != nil {
		t.Fatal("lock not released", e)
	}
	third()
	st, e := os.Stat(path)
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatal("lock is not private", e)
	}
}
func TestLockRejectsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "sessions"))
	if unlock, e := (Manager{Roots: []string{root}}).Lock(filepath.Join(root, "sessions", "apply.lock")); e == nil {
		unlock()
		t.Fatal("lock followed symlink")
	}
	if _, e := os.Stat(filepath.Join(outside, "apply.lock")); !os.IsNotExist(e) {
		t.Fatal("outside lock created")
	}
}
