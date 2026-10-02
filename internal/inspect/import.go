package inspect

import (
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// readImport scans the folders chosen in a previous home folder, only when
// some were chosen. It only reads: what is missing here, what is the same,
// what differs, and how much space copying needs.
func readImport(h *domain.Host, o domain.Options) error {
	if o.ImportFrom == "" || len(o.ImportFolders) == 0 {
		return nil
	}
	if err := plan.ValidImportSource(*h, o.ImportFrom); err != nil {
		return err
	}
	st, err := os.Stat(o.ImportFrom)
	switch {
	case errors.Is(err, fs.ErrPermission):
		h.ImportProblems = append(h.ImportProblems, unreadable(o.ImportFrom))
		return nil
	case err != nil:
		return fmt.Errorf("previous home folder: %w", err)
	case !st.IsDir():
		return fmt.Errorf("the previous home folder is not a folder: %s", o.ImportFrom)
	}
	if h.FreeBytes, err = files.FreeBytes(h.Home); err != nil {
		return err
	}
	h.ConflictDates = conflictDates(h.Home)
	for _, folder := range o.ImportFolders {
		if err := plan.ValidImportFolder(folder); err != nil {
			return err
		}
		job := plan.ImportJob(*h, o, folder)
		if folder != "." {
			st, err := os.Lstat(job.Source)
			switch {
			case errors.Is(err, fs.ErrPermission):
				h.ImportProblems = append(h.ImportProblems, unreadable(job.Source))
				continue
			case err != nil:
				return fmt.Errorf("%s is not in the previous home folder %s", folder, o.ImportFrom)
			case !st.IsDir():
				return fmt.Errorf("%s is not a folder in %s; links are not followed", folder, o.ImportFrom)
			}
		}
		scan, err := files.ScanImport(job)
		var pathErr *fs.PathError
		if errors.Is(err, fs.ErrPermission) && errors.As(err, &pathErr) {
			h.ImportProblems = append(h.ImportProblems, unreadable(pathErr.Path))
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s in the previous home folder: %w", folder, err)
		}
		scan.Folder = folder
		h.Import = append(h.Import, scan)
	}
	return nil
}

// unreadable explains a folder that macOS privacy protection or its
// permissions keep setup from reading.
func unreadable(path string) string {
	path = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '?'
		}
		return r
	}, path)
	return "Setup cannot read " + path + ". On another drive, allow your terminal app in System Settings → Privacy & Security → Files and Folders (Removable or Network Volumes) or Full Disk Access; otherwise check the folder's permissions with Finder's Get Info. Then inspect again."
}

// conflictDates lists the dated folders earlier imports saved conflicts in.
func conflictDates(home string) []string {
	entries, err := os.ReadDir(filepath.Join(home, plan.ConflictsFolder))
	if err != nil {
		return nil
	}
	var dates []string
	for _, entry := range entries {
		if _, err := time.Parse("2006-01-02", entry.Name()); err == nil && entry.IsDir() {
			dates = append(dates, entry.Name())
		}
	}
	return dates
}
