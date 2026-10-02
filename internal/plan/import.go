package plan

import (
	"fmt"
	"golden-gate-setup/internal/domain"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ConflictsFolder is where an import saves the previous Mac's version of a
// file that differs from the one here, in a folder per day.
const ConflictsFolder = "Imported conflicts"

// ValidImportSource checks a previous home folder: it must be a clean
// absolute path that neither contains this home nor lies inside it.
func ValidImportSource(h domain.Host, source string) error {
	if !filepath.IsAbs(source) || filepath.Clean(source) != source || strings.ContainsFunc(source, unicode.IsControl) {
		return fmt.Errorf("the previous home folder must be a clean absolute path")
	}
	if within(h.Home, source) || within(source, h.Home) {
		return fmt.Errorf("choose a previous home folder outside this home folder")
	}
	return nil
}

// ValidImportFolder checks a folder chosen inside the previous home folder:
// a name directly inside it, or "." for the files at its top.
func ValidImportFolder(name string) error {
	switch {
	case name == ".":
		return nil
	case name == "" || name == ".." || name != filepath.Base(name) || strings.ContainsRune(name, '/') || strings.ContainsFunc(name, unicode.IsControl):
		return fmt.Errorf("choose folders directly inside the previous home folder: %q", name)
	case name == "Library":
		return fmt.Errorf("Library holds app data that depends on app versions; bring it over with Migration Assistant instead")
	case name == ".Trash":
		return fmt.Errorf("the previous Trash is not imported")
	case name == ConflictsFolder:
		return fmt.Errorf("%s holds copies an earlier import saved; open it in Finder and move what you need", ConflictsFolder)
	}
	return nil
}

// ImportJob describes one chosen folder. Inspection scans it and the plan
// copies it, so both build it here. Copies saved on earlier days count as
// saved, so running the import again never saves them twice, and files the
// dotfiles repository manages are left to it.
func ImportJob(h domain.Host, o domain.Options, folder string) domain.ImportJob {
	sub, job := folder, domain.ImportJob{Source: filepath.Join(o.ImportFrom, folder), Destination: filepath.Join(h.Home, folder)}
	if folder == "." {
		sub, job = "Top of home folder", domain.ImportJob{Source: o.ImportFrom, Destination: h.Home, TopFilesOnly: true}
	}
	base := filepath.Join(h.Home, ConflictsFolder)
	job.Conflicts = filepath.Join(base, o.RecoveryDate, sub)
	for _, date := range h.ConflictDates {
		if date != o.RecoveryDate {
			job.Saved = append(job.Saved, filepath.Join(base, date, sub))
		}
	}
	for _, target := range dotfileTargets(h, o) {
		if job.TopFilesOnly && filepath.Dir(target) == h.Home || !job.TopFilesOnly && target != job.Destination && within(job.Destination, target) {
			job.Exclude = append(job.Exclude, target)
		}
	}
	return job
}

func importName(folder string) string {
	if folder == "." {
		return "the files at the top of your home folder"
	}
	return folder
}

// SizeText writes a byte count the way Finder does (1 GB is 10^9 bytes).
func SizeText(n int64) string {
	for _, unit := range []struct {
		size int64
		name string
	}{{1e12, "TB"}, {1e9, "GB"}, {1e6, "MB"}, {1e3, "KB"}} {
		if n >= unit.size {
			return fmt.Sprintf("%.1f %s", float64(n)/float64(unit.size), unit.name)
		}
	}
	return fmt.Sprintf("%d bytes", n)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// touchesSetup reports whether importing folder can change files the rest
// of setup reads or writes: dotfiles, chezmoi's source or a project
// workspace.
func touchesSetup(h domain.Host, o domain.Options, folder string) bool {
	if strings.HasPrefix(folder, ".") {
		return true
	}
	dest := filepath.Join(h.Home, folder)
	if h.ChezmoiDir != "" && within(dest, h.ChezmoiDir) {
		return true
	}
	for _, w := range o.Workspaces {
		if w.Path != "" && Has(o.Languages, w.Language) && within(dest, w.Path) {
			return true
		}
	}
	return false
}

// addImport plans one import step for each chosen folder that has files to
// copy or compare. Nothing here is replaced; differing files are saved in a
// dated conflicts folder. It reports whether a step can change files the
// rest of setup reads, which is then planned in a second review.
func addImport(p *domain.Plan, h domain.Host, o domain.Options, add func(domain.Step)) (bool, error) {
	if o.ImportFrom == "" {
		if len(o.ImportFolders) > 0 {
			return false, fmt.Errorf("choose the previous home folder to import from")
		}
		return false, nil
	}
	if err := ValidImportSource(h, o.ImportFrom); err != nil {
		return false, err
	}
	if _, err := time.Parse("2006-01-02", o.RecoveryDate); err != nil {
		return false, fmt.Errorf("invalid recovery date")
	}
	if len(h.ImportProblems) > 0 {
		p.Supported = false
		p.Problems = append(p.Problems, h.ImportProblems...)
	}
	var need int64
	later := false
	aside, cloud, special := 0, 0, 0
	seen := map[string]bool{}
	for _, folder := range o.ImportFolders {
		if err := ValidImportFolder(folder); err != nil {
			return false, err
		}
		if seen[folder] {
			return false, fmt.Errorf("%s is chosen twice", importName(folder))
		}
		seen[folder] = true
		var scan *domain.ImportScan
		for i := range h.Import {
			if h.Import[i].Folder == folder {
				scan = &h.Import[i]
			}
		}
		if scan == nil {
			if len(h.ImportProblems) > 0 {
				continue
			}
			return false, fmt.Errorf("%s was not inspected in %s", importName(folder), o.ImportFrom)
		}
		need += scan.CopyBytes + scan.DifferBytes
		aside, cloud, special = aside+scan.Differ+scan.Aside, cloud+scan.CloudOnly, special+scan.Special
		if scan.Copy+scan.Differ == 0 {
			continue
		}
		later = later || touchesSetup(h, o, folder)
		job := ImportJob(h, o, folder)
		label := fmt.Sprintf("Import %s from your previous Mac: %s (%s)", importName(folder), plural(scan.Copy, "new file", "new files"), SizeText(scan.CopyBytes))
		if scan.Differ > 0 {
			label += fmt.Sprintf("; %s to compare", plural(scan.Differ, "existing file", "existing files"))
		}
		add(domain.Step{ID: "import:" + folder, Label: label, Kind: "import", Import: &job, Check: domain.Check{Kind: "import", Target: job.Destination, Expected: scan.Digest}})
	}
	if h.FreeBytes > 0 && need > h.FreeBytes {
		p.Supported = false
		p.Problems = append(p.Problems, fmt.Sprintf("Importing needs %s but only %s is free. Free up space or choose fewer folders.", SizeText(need), SizeText(h.FreeBytes)))
	}
	if aside > 0 {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "import-conflicts", Title: "Compare imported conflicts", Instructions: "Where a file here differs from your previous Mac's version, yours is kept and theirs is saved in " + filepath.Join(h.Home, ConflictsFolder) + ", in a folder named for the day. Compare them and delete the copies you don't need.", Required: false})
	}
	if cloud > 0 {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "import-icloud", Title: "Bring back iCloud-only files", Instructions: plural(cloud, "file was", "files were") + " only in iCloud on your previous Mac, so they are not copied. Sign in to iCloud on this Mac and turn on iCloud Drive, including Desktop & Documents Folders if you used them.", Required: false})
	}
	if special > 0 {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "import-special", Title: "Skipped special files", Instructions: plural(special, "socket, pipe or device file", "sockets, pipes or device files") + " in the previous home folder are not copied; programs recreate them when needed.", Required: false})
	}
	return later, nil
}
