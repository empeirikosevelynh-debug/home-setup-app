//go:build !darwin && !windows

package files

import (
	"golang.org/x/sys/unix"
	"io/fs"
)

// dataless is always false where files cannot be iCloud placeholders.
func dataless(fs.FileInfo) bool { return false }

// syncBefore makes a copy's contents reach the disk before its name does.
func syncBefore(fd int) error { return unix.Fsync(fd) }
