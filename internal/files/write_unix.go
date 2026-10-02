//go:build !windows

package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/unix"
	"golden-gate-setup/internal/domain"
	"os"
	"path/filepath"
)

func writeAt(fd int, name string, data []byte, mode uint32) error {
	n, e := unix.Openat(fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(n), name)
	if _, e = f.Write(data); e == nil {
		e = f.Chmod(os.FileMode(mode))
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}
func (m Manager) ApplyContext(ctx context.Context, c domain.FileChange, backupDir string) (result domain.StepResult, err error) {
	result.ID = "file:" + c.Path
	result.Status = "failed"
	defer func() {
		if err != nil {
			result.Message = err.Error()
		}
	}()
	if c.Decision == domain.Preserve {
		result.Status = "preserved"
		return result, nil
	}
	if c.Decision != domain.Create && c.Decision != domain.Replace {
		return result, fmt.Errorf("unreviewed file decision")
	}
	if e := ctx.Err(); e != nil {
		return result, e
	}
	fd, name, e := m.parent(c.Path, true)
	if e != nil {
		return result, e
	}
	defer unix.Close(fd)
	before, e := snapshot(fd, name, c.Path)
	if e != nil {
		return result, e
	}
	if before.Exists && before.SHA256 == digest(c.Desired) {
		result.Status = "unchanged"
		return result, nil
	}
	if !matches(before, c) {
		return result, fmt.Errorf("file changed since preview: %s", c.Path)
	}
	if before.Exists && c.Decision != domain.Replace {
		return result, fmt.Errorf("replacement was not approved")
	}
	mode := c.Mode.Perm()
	if mode == 0 {
		mode = 0644
	}
	if before.Exists {
		mode = before.Mode.Perm()
	}
	if before.Exists && !m.SkipBackup {
		if result.BackupPath, e = m.saveBackup(ctx, before, backupDir); e != nil {
			return result, e
		}
	}
	var random [12]byte
	if _, e = rand.Read(random[:]); e != nil {
		return result, e
	}
	temp := ".golden-setup-" + hex.EncodeToString(random[:])
	defer unix.Unlinkat(fd, temp, 0)
	if e = writeAt(fd, temp, c.Desired, uint32(mode)); e != nil {
		return result, e
	}
	if m.BeforeRename != nil {
		if e = m.BeforeRename(); e != nil {
			return result, e
		}
	}
	current, e := snapshot(fd, name, c.Path)
	if e != nil {
		return result, e
	}
	if !matches(current, c) {
		return result, fmt.Errorf("file changed during preparation: %s", c.Path)
	}
	if e = ctx.Err(); e != nil {
		return result, e
	}
	if c.BeforeExists {
		e = unix.Renameat(fd, temp, fd, name)
	} else {
		e = unix.Linkat(fd, temp, fd, name, 0)
	}
	if e != nil {
		return result, e
	}
	if e = unix.Fsync(fd); e != nil {
		return result, e
	}
	result.Status = "applied"
	return result, nil
}

// saveBackup writes a private copy of a file's reviewed contents, keeping an
// identical copy that is already there.
func (m Manager) saveBackup(ctx context.Context, before domain.FileState, backupDir string) (string, error) {
	backupPath := filepath.Join(backupDir, digest([]byte(before.Path))+"-"+before.SHA256+".bak")
	bfd, bname, e := m.parent(backupPath, true)
	if e != nil {
		return "", e
	}
	defer unix.Close(bfd)
	if e = unix.Fchmod(bfd, 0700); e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	e = writeAt(bfd, bname, before.Contents, 0600)
	if e == unix.EEXIST {
		old, re := snapshot(bfd, bname, backupPath)
		if re != nil || old.SHA256 != before.SHA256 || !IsPrivate(old.Mode) {
			return "", fmt.Errorf("existing backup is not a valid private recovery copy")
		}
		e = nil
	}
	if e == nil {
		e = unix.Fsync(bfd)
	}
	return backupPath, e
}

// Backup saves a private copy of path before another program replaces it,
// refusing a file that changed since it was reviewed.
func (m Manager) Backup(ctx context.Context, path, sha, backupDir string) (string, error) {
	fd, name, e := m.parent(path, false)
	if e != nil {
		return "", e
	}
	defer unix.Close(fd)
	before, e := snapshot(fd, name, path)
	if e != nil {
		return "", e
	}
	if !before.Exists || before.SHA256 != sha {
		return "", fmt.Errorf("file changed since preview: %s", path)
	}
	return m.saveBackup(ctx, before, backupDir)
}

// EnsureParent creates the missing folders above path, privately, without
// changing folders that exist or following links.
func (m Manager) EnsureParent(path string) error {
	fd, _, e := m.parent(path, true)
	if e == nil {
		unix.Close(fd)
	}
	return e
}
