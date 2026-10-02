//go:build !windows

package files

import (
	"fmt"
	"golang.org/x/sys/unix"
	"sync"
)

// Lock keeps the descriptor open for the complete apply operation. A crashed
// process releases its advisory lock automatically; the private file remains.
func (m Manager) Lock(path string) (func(), error) {
	parent, name, e := m.parent(path, true)
	if e != nil {
		return nil, e
	}
	defer unix.Close(parent)
	fd, e := unix.Openat(parent, name, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	var st unix.Stat_t
	if e = unix.Fstat(fd, &st); e == nil && st.Mode&unix.S_IFMT != unix.S_IFREG {
		e = fmt.Errorf("installer lock is not a regular file")
	}
	if e == nil {
		e = unix.Fchmod(fd, 0600)
	}
	if e == nil {
		e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	}
	if e != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("cannot acquire installer lock; close any other setup session: %w", e)
	}
	var once sync.Once
	return func() { once.Do(func() { unix.Flock(fd, unix.LOCK_UN); unix.Close(fd) }) }, nil
}
