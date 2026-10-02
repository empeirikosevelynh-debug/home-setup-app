package files

import (
	"golang.org/x/sys/unix"
	"io/fs"
	"syscall"
)

// dataless reports an iCloud placeholder whose contents are not on this disk.
func dataless(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Flags&unix.SF_DATALESS != 0
}

// syncBefore makes a copy's contents reach the disk before its name does.
// A barrier keeps that order without the cost of a full flush per file.
func syncBefore(fd int) error {
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_BARRIERFSYNC, 0); err == nil {
		return nil
	}
	return unix.Fsync(fd)
}
