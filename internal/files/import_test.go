//go:build !windows

package files

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"golden-gate-setup/internal/domain"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var then = time.Date(2024, 3, 1, 12, 0, 0, 123456789, time.UTC)

// importHomes makes a previous home folder and a new one side by side.
func importHomes(t *testing.T) (Manager, string, string) {
	t.Helper()
	dir := t.TempDir()
	old, home := filepath.Join(dir, "old"), filepath.Join(dir, "home")
	for _, d := range []string{old, home} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return Manager{Roots: []string{home}}, old, home
}

func put(t *testing.T, path, contents string, when time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func importJob(old, home, folder string) domain.ImportJob {
	return domain.ImportJob{Source: filepath.Join(old, folder), Destination: filepath.Join(home, folder), Conflicts: filepath.Join(home, "Imported conflicts", "2026-10-02", folder)}
}

func noTemporaryFiles(t *testing.T, root string) {
	t.Helper()
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), importTemp) {
			t.Fatal("temporary file left behind:", p)
		}
		return nil
	})
}

func TestImportCopiesNewFiles(t *testing.T) {
	m, old, home := importHomes(t)
	put(t, filepath.Join(old, "Documents/notes.txt"), "notes", then)
	put(t, filepath.Join(old, "Documents/Projects/plan.md"), "plan", then)
	os.Chmod(filepath.Join(old, "Documents/Projects/plan.md"), 0750)
	os.Symlink("../notes.txt", filepath.Join(old, "Documents/Projects/link"))
	put(t, filepath.Join(old, "Documents/.DS_Store"), "finder", then)
	os.Chtimes(filepath.Join(old, "Documents/Projects"), then, then)
	job := importJob(old, home, "Documents")
	scan, err := ScanImport(job)
	if err != nil || scan.Copy != 3 || scan.CopyBytes != 9 || scan.Differ+scan.Same != 0 || scan.Digest == "" {
		t.Fatal(scan, err)
	}
	done, err := m.Import(context.Background(), job, nil)
	if err != nil || done.Copy != 3 || done.CopyBytes != 9 {
		t.Fatal(done, err)
	}
	plan := filepath.Join(home, "Documents/Projects/plan.md")
	st, err := os.Lstat(plan)
	if err != nil || read(t, plan) != "plan" || st.Mode().Perm() != 0750 || !st.ModTime().Equal(then) {
		t.Fatal(st.Mode(), st.ModTime(), err)
	}
	if dir, err := os.Lstat(filepath.Dir(plan)); err != nil || !dir.ModTime().Equal(then) {
		t.Fatal("folder time not kept", err)
	}
	if link, err := os.Readlink(filepath.Join(home, "Documents/Projects/link")); err != nil || link != "../notes.txt" {
		t.Fatal(link, err)
	}
	if _, err := os.Lstat(filepath.Join(home, "Documents/.DS_Store")); !os.IsNotExist(err) {
		t.Fatal(".DS_Store copied")
	}
	again, err := ScanImport(job)
	if err != nil || again.Copy+again.Differ != 0 || again.Same != 3 {
		t.Fatal(again, err)
	}
	if ok, err := m.Imported(context.Background(), job); !ok || err != nil {
		t.Fatal(ok, err)
	}
	noTemporaryFiles(t, home)
}

func TestImportKeepsYoursAndSavesTheirs(t *testing.T) {
	m, old, home := importHomes(t)
	ctx := context.Background()
	put(t, filepath.Join(old, "Documents/notes.txt"), "theirs", then)
	put(t, filepath.Join(home, "Documents/notes.txt"), "yours!", then.Add(time.Hour))
	job := importJob(old, home, "Documents")
	if scan, err := ScanImport(job); err != nil || scan.Differ != 1 || scan.DifferBytes != 6 {
		t.Fatal(scan, err)
	}
	if ok, _ := m.Imported(ctx, job); ok {
		t.Fatal("a differing file counted as imported")
	}
	if done, err := m.Import(ctx, job, nil); err != nil || done.Differ != 1 {
		t.Fatal(done, err)
	}
	if read(t, filepath.Join(home, "Documents/notes.txt")) != "yours!" {
		t.Fatal("existing file replaced")
	}
	saved := filepath.Join(job.Conflicts, "notes.txt")
	if st, err := os.Stat(saved); err != nil || read(t, saved) != "theirs" || !st.ModTime().Equal(then) {
		t.Fatal("their version not saved aside", err)
	}
	if ok, err := m.Imported(ctx, job); !ok || err != nil {
		t.Fatal(ok, err)
	}
	// Running again, today or on a later day, saves nothing twice.
	if scan, _ := ScanImport(job); scan.Differ != 0 || scan.Aside != 1 {
		t.Fatal(scan)
	}
	later := job
	later.Conflicts, later.Saved = filepath.Join(home, "Imported conflicts", "2026-10-03", "Documents"), []string{job.Conflicts}
	if scan, _ := ScanImport(later); scan.Differ != 0 || scan.Aside != 1 {
		t.Fatal(scan)
	}
	if done, err := m.Import(ctx, later, nil); err != nil || done.Aside != 1 || done.Differ != 0 {
		t.Fatal(done, err)
	}
	if _, err := os.Lstat(later.Conflicts); !os.IsNotExist(err) {
		t.Fatal("saved again on a later day")
	}
	// A newer version of theirs is saved next to the first.
	put(t, filepath.Join(old, "Documents/notes.txt"), "theirs, edited", then.Add(2*time.Hour))
	if done, err := m.Import(ctx, job, nil); err != nil || done.Differ != 1 {
		t.Fatal(done, err)
	}
	if read(t, filepath.Join(job.Conflicts, "notes (2).txt")) != "theirs, edited" || read(t, saved) != "theirs" {
		t.Fatal("versions not numbered")
	}
	noTemporaryFiles(t, home)
}

func TestImportSkipsIdenticalFiles(t *testing.T) {
	m, old, home := importHomes(t)
	put(t, filepath.Join(old, "Documents/.localized"), "", then)
	put(t, filepath.Join(old, "Documents/same.txt"), "same", then)
	put(t, filepath.Join(home, "Documents/.localized"), "", then.Add(time.Hour))
	put(t, filepath.Join(home, "Documents/same.txt"), "same", then.Add(time.Hour))
	job := importJob(old, home, "Documents")
	if scan, err := ScanImport(job); err != nil || scan.Same != 2 || scan.Copy+scan.Differ != 0 {
		t.Fatal(scan, err)
	}
	if done, err := m.Import(context.Background(), job, nil); err != nil || done.Same != 2 {
		t.Fatal(done, err)
	}
	if _, err := os.Lstat(filepath.Join(home, "Imported conflicts")); !os.IsNotExist(err) {
		t.Fatal("identical files saved aside")
	}
}

func TestImportNeverWritesThroughLinks(t *testing.T) {
	m, old, home := importHomes(t)
	ctx := context.Background()
	elsewhere, other := t.TempDir(), t.TempDir()
	put(t, filepath.Join(old, "Documents/a.txt"), "a", then)
	put(t, filepath.Join(old, "Pictures/sub/b.txt"), "b", then)
	put(t, filepath.Join(other, "secret.txt"), "secret", then)
	os.Symlink(other, filepath.Join(old, "Pictures/elsewhere"))
	os.Symlink(elsewhere, filepath.Join(home, "Documents"))
	os.MkdirAll(filepath.Join(home, "Pictures"), 0700)
	os.Symlink(elsewhere, filepath.Join(home, "Pictures/sub"))
	for _, folder := range []string{"Documents", "Pictures"} {
		job := importJob(old, home, folder)
		scan, err := ScanImport(job)
		if err != nil || scan.Differ != 1 || scan.Copy != map[string]int{"Documents": 0, "Pictures": 1}[folder] {
			t.Fatal(folder, scan, err)
		}
		if _, err := m.Import(ctx, job, nil); err != nil {
			t.Fatal(folder, err)
		}
		if ok, err := m.Imported(ctx, job); !ok || err != nil {
			t.Fatal(folder, ok, err)
		}
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatal("wrote through a link")
	}
	if read(t, filepath.Join(home, "Imported conflicts/2026-10-02/Documents/a.txt")) != "a" || read(t, filepath.Join(home, "Imported conflicts/2026-10-02/Pictures/sub/b.txt")) != "b" {
		t.Fatal("blocked files not saved aside")
	}
	if link, err := os.Readlink(filepath.Join(home, "Pictures/elsewhere")); err != nil || link != other {
		t.Fatal("link not recreated", link, err)
	}
}

func TestImportSkipsSpecialFiles(t *testing.T) {
	m, old, home := importHomes(t)
	os.MkdirAll(filepath.Join(old, "Documents"), 0700)
	if err := syscall.Mkfifo(filepath.Join(old, "Documents/pipe"), 0600); err != nil {
		t.Skip("no named pipes here:", err)
	}
	job := importJob(old, home, "Documents")
	if scan, err := ScanImport(job); err != nil || scan.Special != 1 || scan.Copy != 0 {
		t.Fatal(scan, err)
	}
	if done, err := m.Import(context.Background(), job, nil); err != nil || done.Special != 1 {
		t.Fatal(done, err)
	}
	if _, err := os.Lstat(filepath.Join(home, "Documents/pipe")); !os.IsNotExist(err) {
		t.Fatal("special file copied")
	}
}

func TestImportKeepsBundlesWhole(t *testing.T) {
	m, old, home := importHomes(t)
	ctx := context.Background()
	library := "Pictures/Photos Library.photoslibrary"
	put(t, filepath.Join(old, library, "database/Photos.sqlite"), "old db", then)
	put(t, filepath.Join(old, library, "originals/1.jpg"), "photo", then)
	put(t, filepath.Join(home, library, "database/Photos.sqlite"), "new db", then.Add(time.Hour))
	job := importJob(old, home, "Pictures")
	if scan, err := ScanImport(job); err != nil || scan.Differ != 2 || scan.Copy != 0 {
		t.Fatal(scan, err)
	}
	if _, err := m.Import(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, library, "originals")); !os.IsNotExist(err) {
		t.Fatal("merged into the existing library")
	}
	aside := filepath.Join(job.Conflicts, "Photos Library.photoslibrary")
	if read(t, filepath.Join(aside, "database/Photos.sqlite")) != "old db" || read(t, filepath.Join(aside, "originals/1.jpg")) != "photo" {
		t.Fatal("library not saved aside whole")
	}
	// A library this import started is completed instead.
	m2, old2, home2 := importHomes(t)
	put(t, filepath.Join(old2, library, "database/Photos.sqlite"), "old db", then)
	put(t, filepath.Join(old2, library, "originals/1.jpg"), "photo", then)
	put(t, filepath.Join(home2, library, "originals/1.jpg"), "photo", then)
	job2 := importJob(old2, home2, "Pictures")
	if scan, err := ScanImport(job2); err != nil || scan.Copy != 1 || scan.Same != 1 {
		t.Fatal(scan, err)
	}
	if done, err := m2.Import(ctx, job2, nil); err != nil || done.Copy != 1 {
		t.Fatal(done, err)
	}
}

func TestImportResumesAfterCancel(t *testing.T) {
	m, old, home := importHomes(t)
	defer func(n int64) { progressFiles = n }(progressFiles)
	progressFiles = 10
	for i := range 25 {
		put(t, filepath.Join(old, "Documents", fmt.Sprintf("%02d.txt", i)), "x", then)
	}
	job := importJob(old, home, "Documents")
	ctx, cancel := context.WithCancel(context.Background())
	var lines []string
	first, err := m.Import(ctx, job, func(line string) { lines = append(lines, line); cancel() })
	if !errors.Is(err, context.Canceled) || len(lines) != 1 || !strings.HasPrefix(lines[0], "Documents: 10 files copied") {
		t.Fatal(err, lines)
	}
	second, err := m.Import(context.Background(), job, nil)
	if err != nil || second.Same != first.Copy || first.Copy+second.Copy != 25 {
		t.Fatal(first, second, err)
	}
	if ok, err := m.Imported(context.Background(), job); !ok || err != nil {
		t.Fatal(ok, err)
	}
	noTemporaryFiles(t, home)
}

func TestImportCopiesExtendedAttributes(t *testing.T) {
	m, old, home := importHomes(t)
	source := filepath.Join(old, "Documents/tagged.txt")
	put(t, source, "tagged", then)
	name := "user.golden-test"
	if err := unix.Setxattr(source, name, []byte("blue"), 0); err != nil {
		t.Skip("no extended attributes here:", err)
	}
	if _, err := m.Import(context.Background(), importJob(old, home, "Documents"), nil); err != nil {
		t.Fatal(err)
	}
	value := make([]byte, 16)
	n, err := unix.Getxattr(filepath.Join(home, "Documents/tagged.txt"), name, value)
	if err != nil || string(value[:n]) != "blue" {
		t.Fatal("extended attribute not copied", err)
	}
}

func TestImportScanDigest(t *testing.T) {
	_, old, home := importHomes(t)
	put(t, filepath.Join(old, "Documents/sub/a.txt"), "a", then)
	put(t, filepath.Join(home, "Documents/sub/other.txt"), "mine", then)
	job := importJob(old, home, "Documents")
	first, _ := ScanImport(job)
	// Files that are not in the source and folder times are not compared.
	put(t, filepath.Join(home, "Documents/sub/new.txt"), "new", then)
	if again, _ := ScanImport(job); again.Digest != first.Digest {
		t.Fatal("unrelated change altered the digest")
	}
	put(t, filepath.Join(home, "Documents/sub/a.txt"), "b", then.Add(time.Minute))
	if changed, _ := ScanImport(job); changed.Digest == first.Digest || changed.Differ != 1 {
		t.Fatal("a compared file changed without notice", changed)
	}
}

func TestImportTopFilesAndExclusions(t *testing.T) {
	m, old, home := importHomes(t)
	put(t, filepath.Join(old, ".zshrc"), "zsh", then)
	put(t, filepath.Join(old, ".gitconfig"), "git", then)
	put(t, filepath.Join(old, "Documents/a.txt"), "a", then)
	job := domain.ImportJob{Source: old, Destination: home, Conflicts: filepath.Join(home, "Imported conflicts/2026-10-02/Top of home folder"), TopFilesOnly: true, Exclude: []string{filepath.Join(home, ".gitconfig")}}
	if scan, err := ScanImport(job); err != nil || scan.Copy != 1 {
		t.Fatal(scan, err)
	}
	if _, err := m.Import(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(home, ".zshrc")) != "zsh" {
		t.Fatal("top file not copied")
	}
	for _, skipped := range []string{".gitconfig", "Documents"} {
		if _, err := os.Lstat(filepath.Join(home, skipped)); !os.IsNotExist(err) {
			t.Fatal(skipped, "copied")
		}
	}
	if ok, err := m.Imported(context.Background(), job); !ok || err != nil {
		t.Fatal(ok, err)
	}
}

func TestAsideNames(t *testing.T) {
	for name, want := range map[string]string{"notes.txt": "notes (2).txt", ".zshrc": ".zshrc (2)", "README": "README (2)"} {
		if got := asideName(name, 2); got != want || asideName(name, 1) != name {
			t.Fatal(name, got)
		}
	}
	if free, err := FreeBytes(t.TempDir()); err != nil || free <= 0 {
		t.Fatal(free, err)
	}
}
