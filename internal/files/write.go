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

func matches(s domain.FileState, c domain.FileChange) bool {
	return s.Exists == c.BeforeExists && (!s.Exists || s.SHA256 == c.BeforeSHA256)
}
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
func (m Manager) Apply(c domain.FileChange, backupDir string) (domain.StepResult, error) {
	return m.ApplyContext(context.Background(), c, backupDir)
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
		backupName := digest([]byte(c.Path)) + "-" + before.SHA256 + ".bak"
		backupPath := filepath.Join(backupDir, backupName)
		bfd, bname, e := m.parent(backupPath, true)
		if e != nil {
			return result, e
		}
		if e = unix.Fchmod(bfd, 0700); e != nil {
			unix.Close(bfd)
			return result, e
		}
		if e = ctx.Err(); e != nil {
			unix.Close(bfd)
			return result, e
		}
		e = writeAt(bfd, bname, before.Contents, 0600)
		if e == unix.EEXIST {
			old, re := snapshot(bfd, bname, backupPath)
			if re != nil || old.SHA256 != before.SHA256 || old.Mode.Perm()&0077 != 0 {
				e = fmt.Errorf("existing backup is not a valid private recovery copy")
			} else {
				e = nil
			}
		}
		if e == nil {
			e = unix.Fsync(bfd)
		}
		unix.Close(bfd)
		if e != nil {
			return result, e
		}
		result.BackupPath = backupPath
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
