package ui

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// brewfileName matches Brewfiles kept in a chezmoi source: Brewfile,
// dot_Brewfile with any attributes, or name.Brewfile. Templates are left out:
// only chezmoi can render them.
var brewfileName = regexp.MustCompile(`(?i)(^|_)brewfile$|\.brewfile$`)

// repoBrewfiles finds Brewfiles in chezmoi sources, so a dotfiles
// repository's app list can be reinstalled.
func repoBrewfiles(sources ...string) []string {
	var found []string
	for _, source := range sources {
		if source == "" {
			continue
		}
		filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return nil
			case d.IsDir() && (d.Name() == ".git" || strings.Count(strings.TrimPrefix(path, source), string(filepath.Separator)) > 3):
				return filepath.SkipDir
			case d.Type().IsRegular() && brewfileName.MatchString(d.Name()):
				found = append(found, path)
			}
			return nil
		})
	}
	return found
}

// chezmoiSource is where chezmoi keeps its source in a home folder, unless
// configured otherwise.
func chezmoiSource(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local/share/chezmoi")
}

// previousRepo suggests the dotfiles repository a previous home folder's
// chezmoi source came from, or that source itself when it has no remote.
func previousRepo(importFrom string) string {
	source := chezmoiSource(importFrom)
	if st, err := os.Stat(source); err != nil || !st.IsDir() {
		return ""
	}
	f, err := os.Open(filepath.Join(source, ".git", "config"))
	if err != nil {
		return source
	}
	defer f.Close()
	origin := false
	for lines := bufio.NewScanner(f); lines.Scan(); {
		line := strings.TrimSpace(lines.Text())
		if strings.HasPrefix(line, "[") {
			origin = line == `[remote "origin"]`
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && origin && strings.TrimSpace(key) == "url" {
			return strings.TrimSpace(value)
		}
	}
	return source
}

// brewfileSuggestions lists app lists to reinstall from: recovery records,
// Brewfiles in this Mac's or the previous Mac's chezmoi source, and the
// previous home folder's ~/.Brewfile.
func brewfileSuggestions(importFrom, home string) []string {
	found := append(recoveryInventories(importFrom, home), repoBrewfiles(chezmoiSource(home), chezmoiSource(importFrom))...)
	if importFrom != "" {
		if st, err := os.Lstat(filepath.Join(importFrom, ".Brewfile")); err == nil && st.Mode().IsRegular() {
			found = append(found, filepath.Join(importFrom, ".Brewfile"))
		}
	}
	return found
}
