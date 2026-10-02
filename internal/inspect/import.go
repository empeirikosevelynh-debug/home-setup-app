package inspect

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
		var landed func(dst, src string)
		if strings.HasPrefix(folder, ".") {
			landed = func(dst, src string) {
				if len(h.ChezmoiCandidates) < maxCandidates && offered(h.Home, dst, src) {
					h.ChezmoiCandidates = append(h.ChezmoiCandidates, dst)
				}
			}
		}
		scan, err := files.ScanImport(job, landed)
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
	projectImport(h, o)
	return nil
}

// maxCandidates bounds the imported dotfiles offered for adding to chezmoi.
const maxCandidates = 200

// neverOffered matches paths that hold keys, tokens, caches, histories or
// chezmoi's own files; they are never offered for adding to chezmoi.
var neverOffered = regexp.MustCompile(`(?i)(^|/)(\.gnupg|\.ssh|\.aws|\.azure|\.kube|\.docker|\.password-store|\.cache|\.npm|\.gradle|\.m2|\.cargo|\.rustup|\.pyenv|\.nvm|\.rbenv|\.local/share|\.local/state|\.config/chezmoi|\.config/op|\.config/gcloud)(/|$)|(^|/)(\.netrc|\.npmrc|\.pypirc|\.git-credentials|\.vault-token|\.viminfo|\.lesshst|\.CFUserTextEncoding|\.localized|\.zcompdump\S*|\.\w*_history)$|id_|\.pem$|\.key$|\.p12$|token|secret|credential|password`)

// credentials matches contents that look like a key or a stored secret.
var credentials = regexp.MustCompile(`(?i)(token|passw(or)?d|secret|authorization|api[_-]?key)\s*[=:]|-----BEGIN`)

// offered reports whether an imported file can be offered for adding to
// chezmoi: small text without keys or secrets, outside caches and key
// folders. SSH's and AWS's config files, which hold no keys, are offered.
func offered(home, dst, src string) bool {
	rel, err := filepath.Rel(home, dst)
	if err != nil || rel != ".ssh/config" && rel != ".aws/config" && neverOffered.MatchString(filepath.ToSlash(rel)) {
		return false
	}
	st, err := os.Lstat(src)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 256<<10 {
		return false
	}
	data, err := os.ReadFile(src)
	return err == nil && !bytes.ContainsRune(data, 0) && !credentials.Match(data)
}

// projectImport records what the chosen folders will bring to the files
// setup itself reads: configuration, Fish plugin state, project workspaces
// and chezmoi's source. The import runs first, so the rest of the plan is
// made against them and each later step's check still holds.
func projectImport(h *domain.Host, o domain.Options) {
	var jobs []domain.ImportJob
	for _, scan := range h.Import {
		jobs = append(jobs, plan.ImportJob(*h, o, scan.Folder))
	}
	incoming := func(path string) (string, bool) {
		for _, job := range jobs {
			if slices.Contains(job.Exclude, path) {
				continue
			}
			if job.TopFilesOnly {
				if filepath.Dir(path) == h.Home {
					return filepath.Join(job.Source, filepath.Base(path)), true
				}
				continue
			}
			if rel, err := filepath.Rel(job.Destination, path); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return filepath.Join(job.Source, rel), true
			}
		}
		return "", false
	}
	for _, path := range Paths(*h, o) {
		if src, ok := incoming(path); ok && !h.Files[path].Exists {
			if state, ok := incomingState(src, path); ok {
				h.Files[path] = state
			}
		}
	}
	fish := filepath.Join(h.Home, ".config/fish")
	for _, sub := range []string{"functions", "conf.d", "completions"} {
		if src, ok := incoming(filepath.Join(fish, sub)); ok {
			if entries, err := os.ReadDir(src); err == nil && len(entries) > 0 {
				h.FishPluginConflict = true
			}
		}
	}
	if variables := filepath.Join(fish, "fish_variables"); h.ConfigHome == "" || h.ConfigHome == filepath.Join(h.Home, ".config") {
		if _, err := os.Lstat(variables); os.IsNotExist(err) {
			if src, ok := incoming(variables); ok {
				if universal, err := ReadFishVariables(src); err == nil {
					h.FishInstalledPlugins = universal["_fisher_plugins"]
					for _, key := range []string{"_fisher_list", "fisher_path"} {
						if _, ok := universal[key]; ok {
							h.FishLegacy = true
						}
					}
				}
			}
		}
	}
	for path, state := range h.Workspaces {
		if src, ok := incoming(path); ok && (!state.Exists || state.Empty) {
			if entries, err := os.ReadDir(src); err == nil && len(entries) > 0 {
				h.Workspaces[path] = domain.WorkspaceState{Exists: true, Directory: true}
			}
		}
	}
	if source := plan.DotfilesSource(*h); o.DotfilesRepo == "" {
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			if src, ok := incoming(source); ok {
				if st, err := os.Lstat(src); err == nil && st.IsDir() {
					h.ChezmoiIncoming = true
				}
			}
		}
	}
}

// incomingState describes a file the import will bring, as inspection
// describes files here.
func incomingState(src, path string) (domain.FileState, bool) {
	st, err := os.Lstat(src)
	if err != nil || st.IsDir() {
		return domain.FileState{}, false
	}
	state := domain.FileState{Path: path, Exists: true, Imported: true, Mode: st.Mode()}
	if st.Mode()&fs.ModeSymlink != 0 {
		state.Symlink = true
		return state, true
	}
	if !st.Mode().IsRegular() || st.Size() > 1<<20 {
		return state, true
	}
	data, err := os.ReadFile(src)
	if err != nil || int64(len(data)) != st.Size() {
		return domain.FileState{}, false
	}
	sum := sha256.Sum256(data)
	state.Contents, state.SHA256 = data, hex.EncodeToString(sum[:])
	return state, true
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
