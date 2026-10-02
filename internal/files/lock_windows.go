//go:build windows

package files

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"sync"
)

// Lock keeps the file open and locked for the complete apply operation. A
// crashed process releases its lock automatically; the file remains.
func (m Manager) Lock(path string) (func(), error) {
	if _, e := m.parent(path, true); e != nil {
		return nil, e
	}
	if st, e := os.Lstat(path); e == nil && (redirected(st.Mode()) || !st.Mode().IsRegular()) {
		return nil, fmt.Errorf("installer lock is not a regular file")
	}
	f, e := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if e != nil {
		return nil, e
	}
	overlapped := new(windows.Overlapped)
	e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
	if e != nil {
		f.Close()
		return nil, fmt.Errorf("cannot acquire installer lock; close any other setup session: %w", e)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, overlapped)
			f.Close()
		})
	}, nil
}
