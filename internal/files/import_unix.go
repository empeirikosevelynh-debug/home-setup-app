//go:build !windows

package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"golden-gate-setup/internal/domain"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// Finder recreates these; copying them would only add noise and conflicts.
	importNoise = ".DS_Store"
	// importTemp starts the name of a file that is still being copied.
	importTemp = ".golden-import-"
	// maxSaved bounds the saved versions of one file: notes.txt,
	// notes (2).txt ... notes (100).txt.
	maxSaved = 100
	// smallFile is the size up to which a scan compares contents when only
	// the modification times differ, such as every folder's empty .localized.
	smallFile = 1 << 20
)

var errBlocked = errors.New("a file or link is in the way")

// progressFiles is how many files pass between progress lines, as does every
// gigabyte.
var progressFiles int64 = 1000

// FreeBytes is the space available to this user on path's volume.
func FreeBytes(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// asideName is the name of the nth saved version: notes.txt, notes (2).txt...
func asideName(name string, n int) string {
	if n == 1 {
		return name
	}
	ext := filepath.Ext(name)
	if ext == name {
		ext = ""
	}
	return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
}

func set(values []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}

// bundle reports a folder that Finder shows as one item, such as a Photos
// library, an app or an .rtfd document: one whose name has an extension.
func bundle(name string) bool {
	ext := filepath.Ext(name)
	return len(ext) > 1 && ext != name && !strings.Contains(ext, " ")
}

// contained reports whether everything in the folder dst is also in src, so
// copying src into it only completes it.
func contained(dst, src string) bool {
	entries, err := os.ReadDir(dst)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == importNoise || strings.HasPrefix(name, importTemp) {
			continue
		}
		d, err := entry.Info()
		if err != nil {
			return false
		}
		s, err := os.Lstat(filepath.Join(src, name))
		if err != nil || s.Mode().Type() != d.Mode().Type() {
			return false
		}
		switch {
		case d.IsDir():
			if !contained(filepath.Join(dst, name), filepath.Join(src, name)) {
				return false
			}
		case d.Mode()&fs.ModeSymlink != 0:
			a, _ := os.Readlink(filepath.Join(dst, name))
			b, _ := os.Readlink(filepath.Join(src, name))
			if a != b {
				return false
			}
		case d.Size() != s.Size() || !d.ModTime().Equal(s.ModTime()):
			return false
		}
	}
	return true
}

// separate reports whether a source folder is saved aside whole instead of
// merged: a bundle that is already here with other contents. Merging two
// Photos libraries or apps file by file would break both.
func separate(name, src, dst string) bool {
	if !bundle(name) {
		return false
	}
	st, err := os.Lstat(dst)
	return err == nil && st.IsDir() && !contained(dst, src)
}

// walkSource lists one source folder the way every import pass sees it:
// without links followed, Finder noise, excluded destinations or, at the top
// of a home folder, folders.
func walkSource(job domain.ImportJob, exclude map[string]bool, src, dst string, top bool) ([]fs.FileInfo, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, err
	}
	var infos []fs.FileInfo
	for _, entry := range entries {
		name := entry.Name()
		if name == importNoise || strings.HasPrefix(name, importTemp) || exclude[filepath.Join(dst, name)] || top && job.TopFilesOnly && entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// copies calls found with each saved version of rel there may be: numbered
// names in this import's conflicts folder, then in earlier ones. A version
// takes the first free number, so each search stops at the first gap.
func copies(job domain.ImportJob, rel string, found func(string, fs.FileInfo) bool) bool {
	for _, folder := range append([]string{job.Conflicts}, job.Saved...) {
		p := filepath.Join(folder, filepath.FromSlash(rel))
		for n := 1; n <= maxSaved; n++ {
			candidate := filepath.Join(filepath.Dir(p), asideName(filepath.Base(p), n))
			info, err := os.Lstat(candidate)
			if err != nil {
				break
			}
			if found(candidate, info) {
				return true
			}
		}
	}
	return false
}

// savedLink reports whether a link with the same target is saved.
func savedLink(job domain.ImportJob, rel, target string) bool {
	return copies(job, rel, func(p string, info fs.FileInfo) bool {
		existing, err := os.Readlink(p)
		return info.Mode()&fs.ModeSymlink != 0 && err == nil && existing == target
	})
}

// savedFile reports whether a copy of the source file is saved.
func savedFile(ctx context.Context, job domain.ImportJob, rel, src string, info fs.FileInfo) (bool, error) {
	var err error
	found := copies(job, rel, func(p string, st fs.FileInfo) bool {
		var same bool
		if st.Mode().IsRegular() && st.Size() == info.Size() {
			same, err = identical(ctx, p, src, info)
		}
		return same || err != nil
	})
	return found && err == nil, err
}

// identical reports whether path is a regular file with the source's
// contents. A file that cannot be read counts as different.
func identical(ctx context.Context, path, src string, info fs.FileInfo) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false, nil
	}
	defer f.Close()
	return sameFile(ctx, f, src, info)
}

// sameFile reports whether an open file has the source's contents: the same
// size and modification time, as every copy keeps them, or the same size and
// bytes. An iCloud placeholder is not read, which would download it.
func sameFile(ctx context.Context, f *os.File, src string, info fs.FileInfo) (bool, error) {
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() != info.Size() {
		return false, nil
	}
	if st.ModTime().Equal(info.ModTime()) {
		return true, nil
	}
	if dataless(st) {
		return false, nil
	}
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	defer in.Close()
	return sameBytes(ctx, f, in)
}

// sameBytes compares two files chunk by chunk. Only a failure to read the
// source is an error.
func sameBytes(ctx context.Context, dst, src io.Reader) (bool, error) {
	a, b := make([]byte, 256<<10), make([]byte, 256<<10)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		n, errSrc := io.ReadFull(src, b)
		if errSrc != nil && errSrc != io.EOF && errSrc != io.ErrUnexpectedEOF {
			return false, errSrc
		}
		m, errDst := io.ReadFull(dst, a)
		if errDst != nil && errDst != io.EOF && errDst != io.ErrUnexpectedEOF || m != n || !bytes.Equal(a[:m], b[:n]) {
			return false, nil
		}
		if errSrc != nil {
			return errDst != nil, nil
		}
	}
}

// ScanImport reports what Import would do without writing anything. The
// digest covers the source and destination entries it compared, so a change
// between preview and apply is noticed.
func ScanImport(job domain.ImportJob) (domain.ImportScan, error) {
	s := scanner{job: job, exclude: set(job.Exclude), digest: sha256.New()}
	root, err := os.Lstat(job.Destination)
	if err != nil && !os.IsNotExist(err) {
		return s.result, err
	}
	blocked := err == nil && (!root.IsDir() || !job.TopFilesOnly && separate(filepath.Base(job.Destination), job.Source, job.Destination))
	err = s.dir(job.Source, job.Destination, "", blocked, true)
	s.result.Digest = hex.EncodeToString(s.digest.Sum(nil))
	return s.result, err
}

type scanner struct {
	job     domain.ImportJob
	exclude map[string]bool
	digest  hash.Hash
	result  domain.ImportScan
}

// describe is what the digest records about an entry. Folder sizes and
// times change with their contents, which the digest covers entry by entry.
func describe(info fs.FileInfo) string {
	switch {
	case info == nil:
		return "absent"
	case info.IsDir():
		return "folder"
	}
	return fmt.Sprintf("%v %d %d", info.Mode().Type(), info.Size(), info.ModTime().UnixNano())
}

// dir scans one source folder. When blocked, the destination folder cannot
// be used, so everything below goes aside.
func (s *scanner) dir(src, dst, rel string, blocked, top bool) error {
	infos, err := walkSource(s.job, s.exclude, src, dst, top)
	if err != nil {
		return err
	}
	for _, info := range infos {
		name := info.Name()
		srcPath, dstPath, relPath := filepath.Join(src, name), filepath.Join(dst, name), path.Join(rel, name)
		var target fs.FileInfo
		if !blocked {
			if target, err = os.Lstat(dstPath); os.IsNotExist(err) {
				target = nil
			} else if err != nil {
				return err
			}
		}
		fmt.Fprintf(s.digest, "%s\t%s\t%s\n", relPath, describe(info), describe(target))
		switch {
		case info.IsDir():
			aside := blocked || target != nil && (!target.IsDir() || separate(name, srcPath, dstPath))
			if err := s.dir(srcPath, dstPath, relPath, aside, false); err != nil {
				return err
			}
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			existing, _ := os.Readlink(dstPath)
			switch {
			case !blocked && target == nil:
				s.result.Copy++
			case !blocked && target.Mode()&fs.ModeSymlink != 0 && existing == link:
				s.result.Same++
			case savedLink(s.job, relPath, link):
				s.result.Aside++
			default:
				s.result.Differ++
			}
		case info.Mode().IsRegular() && dataless(info):
			s.result.CloudOnly++
		case info.Mode().IsRegular():
			same := false
			if !blocked && target != nil && target.Mode().IsRegular() && target.Size() == info.Size() {
				if same = target.ModTime().Equal(info.ModTime()); !same && info.Size() <= smallFile {
					if same, err = identical(context.Background(), dstPath, srcPath, info); err != nil {
						return err
					}
				}
			}
			switch {
			case !blocked && target == nil:
				s.result.Copy++
				s.result.CopyBytes += info.Size()
			case same:
				s.result.Same++
			case copies(s.job, relPath, func(_ string, st fs.FileInfo) bool {
				return st.Mode().IsRegular() && st.Size() == info.Size() && st.ModTime().Equal(info.ModTime())
			}):
				s.result.Aside++
			default:
				s.result.Differ++
				s.result.DifferBytes += info.Size()
			}
		default:
			s.result.Special++
		}
	}
	return nil
}

// Import copies the job's files that are missing here and keeps every
// existing file. A file that differs is saved under job.Conflicts instead.
// Nothing is overwritten or deleted, and a re-run skips what is already
// here. report receives occasional progress lines.
func (m Manager) Import(ctx context.Context, job domain.ImportJob, report func(string)) (domain.ImportScan, error) {
	im := &importer{m: m, ctx: ctx, job: job, exclude: set(job.Exclude), report: report, name: filepath.Base(job.Conflicts)}
	source, err := os.Stat(job.Source)
	if err != nil {
		return im.result, err
	}
	fd := -1
	if job.TopFilesOnly || !separate(filepath.Base(job.Destination), job.Source, job.Destination) {
		if fd, err = m.openDestination(job.Destination, source.Mode()); err != nil && !errors.Is(err, errBlocked) {
			return im.result, err
		}
	}
	if fd >= 0 {
		defer unix.Close(fd)
	}
	err = im.dir(job.Source, fd, job.Destination, "", true)
	return im.result, err
}

// openDestination opens the destination folder without following links,
// creating it when missing. A home folder that is itself a root is opened
// directly.
func (m Manager) openDestination(dest string, mode fs.FileMode) (int, error) {
	for _, root := range m.Roots {
		if filepath.Clean(root) == dest {
			return openRoot(dest, false)
		}
	}
	parent, name, err := m.parent(dest, true)
	if err != nil {
		return -1, err
	}
	defer unix.Close(parent)
	fd, _, err := openDir(parent, name, mode)
	return fd, err
}

// openDir opens or creates a folder below parent, never through a link.
func openDir(parent int, name string, mode fs.FileMode) (int, bool, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == nil {
		return fd, false, nil
	}
	if err == unix.ENOENT {
		if err = unix.Mkdirat(parent, name, 0700); err == nil || err == unix.EEXIST {
			created := err == nil
			fd, err = unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err == nil {
				if created {
					unix.Fchmod(fd, uint32(mode.Perm()))
				}
				return fd, created, nil
			}
		}
	}
	if err == unix.ELOOP || err == unix.ENOTDIR {
		return -1, false, errBlocked
	}
	return -1, false, err
}

type importer struct {
	m            Manager
	ctx          context.Context
	job          domain.ImportJob
	exclude      map[string]bool
	report       func(string)
	name         string
	result       domain.ImportScan
	files, bytes int64
}

// progress reports after every thousand files and every gigabyte.
func (im *importer) progress() {
	files, bytes := int64(im.result.Copy+im.result.Same+im.result.Differ+im.result.Aside), im.result.CopyBytes+im.result.DifferBytes
	if im.report == nil || files-im.files < progressFiles && bytes-im.bytes < 1e9 {
		return
	}
	im.files, im.bytes = files, bytes
	im.report(fmt.Sprintf("%s: %d files copied (%.1f GB), %d already here, %d saved aside", im.name, im.result.Copy, float64(im.result.CopyBytes)/1e9, im.result.Same, im.result.Differ))
}

// dir copies one source folder into the destination folder open as fd, or
// sets it all aside when fd is -1.
func (im *importer) dir(src string, fd int, dst, rel string, top bool) error {
	infos, err := walkSource(im.job, im.exclude, src, dst, top)
	if err != nil {
		return err
	}
	for _, info := range infos {
		if err := im.ctx.Err(); err != nil {
			return err
		}
		name := info.Name()
		srcPath, dstPath, relPath := filepath.Join(src, name), filepath.Join(dst, name), path.Join(rel, name)
		switch {
		case info.IsDir():
			err = im.subdir(fd, name, srcPath, dstPath, relPath, info)
		case info.Mode()&fs.ModeSymlink != 0:
			err = im.link(fd, name, srcPath, relPath)
		case info.Mode().IsRegular() && dataless(info):
			im.result.CloudOnly++
		case info.Mode().IsRegular():
			err = im.file(fd, name, srcPath, relPath, info)
		default:
			im.result.Special++
		}
		if err != nil {
			return err
		}
		im.progress()
	}
	return nil
}

// subdir copies a source folder into the folder of the same name below fd,
// creating it when missing. It sets the folder aside when a file or link is
// in the way, or when it is a different bundle.
func (im *importer) subdir(fd int, name, src, dst, rel string, info fs.FileInfo) error {
	child, created := -1, false
	if fd >= 0 && !separate(name, src, dst) {
		var err error
		if child, created, err = openDir(fd, name, info.Mode()); err != nil && !errors.Is(err, errBlocked) {
			return err
		}
	}
	if child < 0 {
		return im.dir(src, -1, dst, rel, false)
	}
	defer unix.Close(child)
	err := im.dir(src, child, dst, rel, false)
	if err == nil && created {
		unix.UtimesNanoAt(fd, name, times(info), unix.AT_SYMLINK_NOFOLLOW)
	}
	return err
}

func times(info fs.FileInfo) []unix.Timespec {
	t := unix.NsecToTimespec(info.ModTime().UnixNano())
	return []unix.Timespec{t, t}
}

// file brings one regular file over: copied when missing, skipped when the
// same, saved aside when different.
func (im *importer) file(fd int, name, src, rel string, info fs.FileInfo) error {
	if fd >= 0 {
		var st unix.Stat_t
		err := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if err == unix.ENOENT {
			placed, err := im.place(fd, name, src, info)
			if err != nil || placed {
				if placed {
					im.result.Copy++
					im.result.CopyBytes += info.Size()
				}
				return err
			}
			// Something took the name meanwhile; compare with it.
			err = unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
		}
		if err != nil {
			return err
		}
		same, err := im.sameAt(fd, name, &st, src, info)
		if err != nil || same {
			if same {
				im.result.Same++
			}
			return err
		}
	}
	return im.aside(src, rel, info)
}

// sameAt reports whether name below fd, described by st, has the source's
// contents.
func (im *importer) sameAt(fd int, name string, st *unix.Stat_t, src string, info fs.FileInfo) (bool, error) {
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size != info.Size() {
		return false, nil
	}
	if time.Unix(st.Mtim.Unix()).Equal(info.ModTime()) {
		return true, nil
	}
	n, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, nil
	}
	f := os.NewFile(uintptr(n), name)
	defer f.Close()
	return sameFile(im.ctx, f, src, info)
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// place copies src into a new file called name below fd. Contents, extended
// attributes, permissions and modification time go into a temporary file,
// which is linked into place only if name is still free. It reports false
// when name was taken meanwhile.
func (im *importer) place(fd int, name, src string, info fs.FileInfo) (bool, error) {
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	defer in.Close()
	var random [12]byte
	if _, err = rand.Read(random[:]); err != nil {
		return false, err
	}
	temp := importTemp + hex.EncodeToString(random[:])
	n, err := unix.Openat(fd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return false, err
	}
	defer unix.Unlinkat(fd, temp, 0)
	out := os.NewFile(uintptr(n), temp)
	if info.Size() >= 1e9 && im.report != nil {
		im.report(fmt.Sprintf("%s: copying %s (%.1f GB)", im.name, name, float64(info.Size())/1e9))
	}
	copied, err := io.Copy(out, ctxReader{im.ctx, in})
	if err == nil && copied != info.Size() {
		err = fmt.Errorf("%s changed while it was copied", src)
	}
	if err == nil {
		copyXattrs(in, out)
		err = out.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = unix.UtimesNanoAt(fd, temp, times(info), unix.AT_SYMLINK_NOFOLLOW)
	}
	if err == nil {
		err = syncBefore(n)
	}
	if ce := out.Close(); err == nil {
		err = ce
	}
	if err != nil {
		return false, err
	}
	if err = unix.Linkat(fd, temp, fd, name, 0); err == unix.EEXIST {
		return false, nil
	}
	return err == nil, err
}

// copyXattrs copies extended attributes, such as Finder tags and info,
// where both file systems support them.
func copyXattrs(in, out *os.File) {
	size, err := unix.Flistxattr(int(in.Fd()), nil)
	if err != nil || size <= 0 {
		return
	}
	names := make([]byte, size)
	if size, err = unix.Flistxattr(int(in.Fd()), names); err != nil {
		return
	}
	for _, attr := range strings.Split(string(names[:size]), "\x00") {
		if attr == "" {
			continue
		}
		n, err := unix.Fgetxattr(int(in.Fd()), attr, nil)
		if err != nil {
			continue
		}
		value := make([]byte, n)
		if n, err = unix.Fgetxattr(int(in.Fd()), attr, value); err == nil {
			unix.Fsetxattr(int(out.Fd()), attr, value[:n], 0)
		}
	}
}

// aside saves a differing source file in the conflicts folder, at the same
// relative path, numbering the name when other versions are there. A copy
// already saved, here or by an earlier import, is kept.
func (im *importer) aside(src, rel string, info fs.FileInfo) error {
	saved, err := savedFile(im.ctx, im.job, rel, src, info)
	if err != nil || saved {
		if saved {
			im.result.Aside++
		}
		return err
	}
	fd, name, err := im.m.parent(filepath.Join(im.job.Conflicts, filepath.FromSlash(rel)), true)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	for n := 1; n <= maxSaved; n++ {
		var st unix.Stat_t
		candidate := asideName(name, n)
		if unix.Fstatat(fd, candidate, &st, unix.AT_SYMLINK_NOFOLLOW) != unix.ENOENT {
			continue
		}
		placed, err := im.place(fd, candidate, src, info)
		if err != nil || placed {
			if placed {
				im.result.Differ++
				im.result.DifferBytes += info.Size()
			}
			return err
		}
	}
	return fmt.Errorf("too many saved versions of %s", rel)
}

// link recreates a symbolic link as written, without following it.
func (im *importer) link(fd int, name, src, rel string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if fd >= 0 {
		if err = unix.Symlinkat(target, fd, name); err == nil {
			im.result.Copy++
			return nil
		}
		if err != unix.EEXIST {
			return err
		}
		if existing, e := readlinkat(fd, name); e == nil && existing == target {
			im.result.Same++
			return nil
		}
	}
	if savedLink(im.job, rel, target) {
		im.result.Aside++
		return nil
	}
	parent, base, err := im.m.parent(filepath.Join(im.job.Conflicts, filepath.FromSlash(rel)), true)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	for n := 1; n <= maxSaved; n++ {
		if err = unix.Symlinkat(target, parent, asideName(base, n)); err != unix.EEXIST {
			if err == nil {
				im.result.Differ++
			}
			return err
		}
	}
	return fmt.Errorf("too many saved versions of %s", rel)
}

func readlinkat(fd int, name string) (string, error) {
	buf := make([]byte, 4096)
	n, err := unix.Readlinkat(fd, name, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

// Imported reports whether every file the job covers is here: an identical
// file at its destination, or a copy in a conflicts folder. It only reads.
func (m Manager) Imported(ctx context.Context, job domain.ImportJob) (bool, error) {
	return imported(ctx, job, set(job.Exclude), job.Source, job.Destination, "", true)
}

func imported(ctx context.Context, job domain.ImportJob, exclude map[string]bool, src, dst, rel string, top bool) (bool, error) {
	infos, err := walkSource(job, exclude, src, dst, top)
	if err != nil {
		return false, err
	}
	for _, info := range infos {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		name := info.Name()
		srcPath, dstPath, relPath := filepath.Join(src, name), filepath.Join(dst, name), path.Join(rel, name)
		ok := true
		switch {
		case info.IsDir():
			ok, err = imported(ctx, job, exclude, srcPath, dstPath, relPath, false)
		case info.Mode()&fs.ModeSymlink != 0:
			var target string
			if target, err = os.Readlink(srcPath); err == nil {
				existing, e := os.Readlink(dstPath)
				ok = e == nil && existing == target || savedLink(job, relPath, target)
			}
		case info.Mode().IsRegular() && !dataless(info):
			if ok, err = identical(ctx, dstPath, srcPath, info); err == nil && !ok {
				ok, err = savedFile(ctx, job, relPath, srcPath, info)
			}
		}
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}
