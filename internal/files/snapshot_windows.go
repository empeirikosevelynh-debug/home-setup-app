//go:build windows

package files

import (
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Windows has no openat or O_NOFOLLOW, so every component is checked with
// Lstat before use and again just before a replacement. A link created in
// between those checks can still redirect a write; macOS has no such window.
var errRedirect = errors.New("path passes through a link, junction or non-directory")

// IsRedirect reports whether err comes from a symlink, junction, other
// reparse point, or a non-directory on the way to a path.
func IsRedirect(err error) bool { return errors.Is(err, errRedirect) }

// IsPrivate is always true on Windows: mode bits do not describe access
// there, and files in the user profile inherit its owner-only access lists.
func IsPrivate(fs.FileMode) bool { return true }

func redirected(mode fs.FileMode) bool { return mode&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

// root picks the longest reviewed root containing path, like the Unix version.
func (m Manager) root(path string, create bool) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("path must be clean and absolute")
	}
	root := ""
	for _, r := range m.Roots {
		r = filepath.Clean(r)
		rel, e := filepath.Rel(r, path)
		if e == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && (root == "" || len(r) > len(root)) {
			root = r
		}
	}
	if root == "" {
		return "", fmt.Errorf("path is outside reviewed roots: %s", path)
	}
	if create {
		if _, e := os.Lstat(root); os.IsNotExist(e) {
			for _, candidate := range m.Roots {
				candidate = filepath.Clean(candidate)
				rel, err := filepath.Rel(candidate, root)
				if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && candidate != root {
					if st, e := os.Lstat(candidate); e == nil && st.IsDir() {
						root = candidate
						break
					}
				}
			}
		}
	}
	return root, nil
}

// parent checks every directory from the volume down to path's parent,
// refusing links and junctions, and creates missing directories below the
// selected root when create is set.
func (m Manager) parent(path string, create bool) (string, error) {
	root, e := m.root(path, create)
	if e != nil {
		return "", e
	}
	dir := filepath.Dir(path)
	volume := filepath.VolumeName(dir)
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(dir[len(volume):], string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if os.IsNotExist(err) && create && len(current) >= len(root) {
			if err = os.Mkdir(current, 0700); err == nil || os.IsExist(err) {
				st, err = os.Lstat(current)
			}
		}
		if err != nil {
			return "", err
		}
		if redirected(st.Mode()) || !st.IsDir() {
			return "", fmt.Errorf("%w: %s", errRedirect, current)
		}
	}
	return dir, nil
}

func snapshot(path string) (domain.FileState, error) {
	s := domain.FileState{Path: path}
	st, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	if redirected(st.Mode()) {
		return s, fmt.Errorf("%w: %s", errRedirect, path)
	}
	s.Exists = true
	s.Mode = st.Mode()
	if !st.Mode().IsRegular() {
		return s, fmt.Errorf("not a regular file: %s", path)
	}
	if st.Size() > 1<<20 {
		return s, fmt.Errorf("configuration exceeds 1 MiB")
	}
	f, e := os.Open(path)
	if e != nil {
		return s, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return s, e
	}
	if len(data) > 1<<20 {
		return s, errors.New("configuration grew past 1 MiB")
	}
	s.Contents = data
	s.SHA256 = digest(data)
	return s, nil
}
func (m Manager) Inspect(path string) (domain.FileState, error) {
	if _, e := m.parent(path, false); e != nil {
		if errors.Is(e, fs.ErrNotExist) {
			return domain.FileState{Path: path}, nil
		}
		return domain.FileState{Path: path}, e
	}
	return snapshot(path)
}
func (m Manager) EnsurePrivateDir(path string) error {
	_, e := m.parent(filepath.Join(path, ".permission-check"), true)
	return e
}
func (m Manager) MakePrivateFile(path string) error {
	if _, e := m.parent(path, false); e != nil {
		return e
	}
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if redirected(st.Mode()) || !st.Mode().IsRegular() {
		return fmt.Errorf("not a regular private file")
	}
	return nil
}
