//go:build windows

package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"golden-gate-setup/internal/domain"
	"os"
	"path/filepath"
)

func writeNew(path string, data []byte, mode os.FileMode) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if e != nil {
		return e
	}
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	return e
}

// move renames temp onto path. Without replace it fails when path exists,
// so creating a file never overwrites one that appeared after the preview.
func move(temp, path string, replace bool) error {
	from, e := windows.UTF16PtrFromString(temp)
	if e != nil {
		return e
	}
	to, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	return windows.MoveFileEx(from, to, flags)
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
	dir, e := m.parent(c.Path, true)
	if e != nil {
		return result, e
	}
	before, e := snapshot(c.Path)
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
		backupPath := filepath.Join(backupDir, digest([]byte(c.Path))+"-"+before.SHA256+".bak")
		if _, e = m.parent(backupPath, true); e != nil {
			return result, e
		}
		if e = ctx.Err(); e != nil {
			return result, e
		}
		e = writeNew(backupPath, before.Contents, 0600)
		if os.IsExist(e) {
			old, re := snapshot(backupPath)
			if re != nil || old.SHA256 != before.SHA256 {
				e = fmt.Errorf("existing backup is not a valid private recovery copy")
			} else {
				e = nil
			}
		}
		if e != nil {
			return result, e
		}
		result.BackupPath = backupPath
	}
	var random [12]byte
	if _, e = rand.Read(random[:]); e != nil {
		return result, e
	}
	temp := filepath.Join(dir, ".golden-setup-"+hex.EncodeToString(random[:]))
	defer os.Remove(temp)
	if e = writeNew(temp, c.Desired, mode); e != nil {
		return result, e
	}
	if m.BeforeRename != nil {
		if e = m.BeforeRename(); e != nil {
			return result, e
		}
	}
	if _, e = m.parent(c.Path, false); e != nil {
		return result, e
	}
	current, e := snapshot(c.Path)
	if e != nil {
		return result, e
	}
	if !matches(current, c) {
		return result, fmt.Errorf("file changed during preparation: %s", c.Path)
	}
	if e = ctx.Err(); e != nil {
		return result, e
	}
	if e = move(temp, c.Path, c.BeforeExists); e != nil {
		return result, e
	}
	result.Status = "applied"
	return result, nil
}
