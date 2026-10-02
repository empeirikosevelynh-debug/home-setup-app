package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"golden-gate-setup/internal/domain"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Manager struct {
	Roots        []string
	BeforeRename func() error
	// SkipBackup replaces files without keeping their previous contents. Only
	// the installer's own records use it; user configuration is always backed up.
	SkipBackup bool
}

func digest(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

// All descendants are traversed through directory descriptors with O_NOFOLLOW.
// The selected root is opened once; replacing a parent cannot redirect a write.
func (m Manager) parent(path string, create bool) (int, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, "", fmt.Errorf("path must be clean and absolute")
	}
	root := ""
	for _, r := range m.Roots {
		r = filepath.Clean(r)
		rel, e := filepath.Rel(r, path)
		if e == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, "../") && (root == "" || len(r) > len(root)) {
			root = r
		}
	}
	if root == "" {
		return -1, "", fmt.Errorf("path is outside reviewed roots: %s", path)
	}
	if create {
		if _, e := os.Lstat(root); os.IsNotExist(e) {
			for _, candidate := range m.Roots {
				candidate = filepath.Clean(candidate)
				rel, err := filepath.Rel(candidate, root)
				if err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && candidate != root {
					if st, e := os.Lstat(candidate); e == nil && st.IsDir() {
						root = candidate
						break
					}
				}
			}
		}
	}
	fd, e := openRoot(root, create)
	if e != nil {
		return -1, "", e
	}
	rel, _ := filepath.Rel(root, path)
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err == unix.ENOENT && create {
			err = unix.Mkdirat(fd, part, 0700)
			if err == nil || err == unix.EEXIST {
				next, err = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if err != nil {
			return -1, "", err
		}
		fd = next
	}
	return fd, parts[len(parts)-1], nil
}
func snapshot(fd int, name, path string) (domain.FileState, error) {
	s := domain.FileState{Path: path}
	n, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e == unix.ENOENT {
		return s, nil
	}
	if e != nil {
		return s, e
	}
	f := os.NewFile(uintptr(n), path)
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return s, e
	}
	s.Exists = true
	s.Mode = st.Mode()
	if !st.Mode().IsRegular() {
		return s, fmt.Errorf("not a regular file: %s", path)
	}
	if st.Size() > 1<<20 {
		return s, fmt.Errorf("configuration exceeds 1 MiB")
	}
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
	fd, name, e := m.parent(path, false)
	if errors.Is(e, unix.ENOENT) {
		return domain.FileState{Path: path}, nil
	}
	if e != nil {
		return domain.FileState{Path: path}, e
	}
	defer unix.Close(fd)
	return snapshot(fd, name, path)
}

func openRoot(path string, create bool) (int, error) {
	if !filepath.IsAbs(path) {
		return -1, fmt.Errorf("root must be absolute")
	}
	if runtime.GOOS == "darwin" {
		for _, alias := range []string{"var", "tmp"} {
			prefix := "/" + alias
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				resolved, e := filepath.EvalSymlinks(prefix)
				if e != nil || resolved != "/private/"+alias {
					return -1, fmt.Errorf("unexpected system path alias")
				}
				path = "/private" + path
			}
		}
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return -1, e
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err == unix.ENOENT && create && i == len(parts)-1 {
			err = unix.Mkdirat(fd, part, 0700)
			if err == nil || err == unix.EEXIST {
				next, err = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
	}
	return fd, nil
}
func (m Manager) EnsurePrivateDir(path string) error {
	fd, _, e := m.parent(filepath.Join(path, ".permission-check"), true)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	return unix.Fchmod(fd, 0700)
}
func (m Manager) MakePrivateFile(path string) error {
	fd, name, e := m.parent(path, false)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	n, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(n)
	var st unix.Stat_t
	if e = unix.Fstat(n, &st); e != nil {
		return e
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("not a regular private file")
	}
	return unix.Fchmod(n, 0600)
}
