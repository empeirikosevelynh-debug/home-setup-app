package ui

import (
	"errors"
	"fmt"
	"golden-gate-setup/internal/plan"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// homeDir finds where to look for recovery folders; tests replace it.
var homeDir = os.UserHomeDir

// recoveryInventories lists Homebrew-full.Brewfile files in Golden Gate
// Recovery folders under the given folders, newest first.
func recoveryInventories(folders ...string) []string {
	var found []string
	for _, folder := range folders {
		if folder == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(folder, "Golden Gate Recovery", "*", "Homebrew-full.Brewfile"))
		sort.Sort(sort.Reverse(sort.StringSlice(matches)))
		found = append(found, matches...)
	}
	return found
}

// previousEntries reads the app list chosen in the wizard; inspection reads
// it again before planning.
func previousEntries(path string) ([]string, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("enter an absolute path")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(data) > 1<<20 {
		return nil, errors.New("the app list is larger than 1 MiB")
	}
	entries, _ := plan.ParseBrewfile(data)
	return entries, nil
}

// choosePrevious turns a plain-mode answer into chosen entries.
func choosePrevious(answer string, entries []string) ([]string, error) {
	switch answer {
	case "all":
		return append([]string(nil), entries...), nil
	case "none":
		return nil, nil
	}
	var chosen []string
	for _, v := range strings.Split(answer, ",") {
		v = strings.TrimSpace(v)
		if !plan.Has(entries, v) {
			return nil, fmt.Errorf("%q is not in the app list", v)
		}
		if !plan.Has(chosen, v) {
			chosen = append(chosen, v)
		}
	}
	return chosen, nil
}
