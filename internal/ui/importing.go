package ui

import (
	"errors"
	"fmt"
	"golden-gate-setup/internal/plan"
	"os"
	"path/filepath"
	"strings"
)

// hiddenNotes describes hidden folders that are often in a home folder, so
// keys are never brought over unnoticed.
var hiddenNotes = map[string]string{
	".ssh": "keys", ".gnupg": "keys", ".aws": "keys", ".azure": "keys", ".kube": "keys", ".docker": "keys", ".password-store": "keys",
	".config": "settings; may hold tokens", ".local": "programs and data",
	".cache": "cache", ".npm": "cache", ".gradle": "cache", ".m2": "cache", ".cargo": "cache and tools", ".rustup": "toolchains", ".pyenv": "toolchains", ".nvm": "toolchains", ".rbenv": "toolchains",
}

type folderChoice struct {
	name, label string
	preselect   bool
}

// importFolders lists what can be imported from a previous home folder: the
// files at its top, its folders, then its hidden folders. Visible folders
// are preselected, except Applications, whose apps are reinstalled instead.
func importFolders(source string) ([]folderChoice, error) {
	if err := importSource(source); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, err
	}
	choices := []folderChoice{{name: ".", label: "Files at the top of the home folder, such as .zshrc"}}
	var hidden []folderChoice
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || plan.ValidImportFolder(name) != nil {
			continue
		}
		if strings.HasPrefix(name, ".") {
			label := name
			if note := hiddenNotes[name]; note != "" {
				label += " (" + note + ")"
			}
			hidden = append(hidden, folderChoice{name: name, label: label})
			continue
		}
		choices = append(choices, folderChoice{name: name, label: name, preselect: name != "Applications"})
	}
	return append(choices, hidden...), nil
}

// importSource checks a previous home folder typed in the wizard; inspection
// checks it again before planning.
func importSource(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("enter a clean absolute path")
	}
	if home, err := homeDir(); err == nil && (inside(home, path) || inside(path, home)) {
		return errors.New("choose a home folder other than this one")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return errors.New("choose a folder")
	}
	return nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// volumesDir is where other drives appear; tests replace it.
var volumesDir = "/Volumes"

// previousHomes suggests home folders on other drives.
func previousHomes() []string {
	found, _ := filepath.Glob(filepath.Join(volumesDir, "*", "Users", "*"))
	var homes []string
	for _, path := range found {
		if name := filepath.Base(path); name != "Shared" && name != "Guest" && !strings.HasPrefix(name, ".") {
			if st, err := os.Stat(path); err == nil && st.IsDir() {
				homes = append(homes, path)
			}
		}
	}
	return homes
}

// chooseFolders turns a plain-mode answer into chosen folders.
func chooseFolders(answer string, choices []folderChoice) ([]string, error) {
	if answer == "none" {
		return nil, nil
	}
	var chosen []string
	for _, v := range strings.Split(answer, ",") {
		v = strings.TrimSpace(v)
		known := false
		for _, c := range choices {
			known = known || c.name == v
		}
		if !known {
			return nil, fmt.Errorf("%q is not a folder offered for import", v)
		}
		if !plan.Has(chosen, v) {
			chosen = append(chosen, v)
		}
	}
	return chosen, nil
}
