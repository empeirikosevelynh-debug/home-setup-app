package plan

import (
	"regexp"
	"strings"
)

// brewName is a formula, cask or tap name, optionally qualified by its tap.
var brewName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@+._-]*(/[A-Za-z0-9][A-Za-z0-9@+._-]*){0,2}$`)
var brewLine = regexp.MustCompile(`^(\w+)\s+"([^"]*)"(.*)$`)

// ParseBrewfile reads the tap, brew and cask entries of a Brewfile written
// by `brew bundle dump` as keys such as tap:user/repo, formula:fish and
// cask:zed. Anything else (App Store apps, editor extensions, taps with a
// custom URL, unusual names) is returned as written for manual follow-up.
// Commented lines are ignored, so the commented starter Brewfile lists
// nothing.
func ParseBrewfile(data []byte) (entries, other []string) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := brewLine.FindStringSubmatch(line)
		kind := ""
		if m != nil {
			kind = map[string]string{"tap": "tap", "brew": "formula", "cask": "cask"}[m[1]]
		}
		if kind == "" || !brewName.MatchString(m[2]) || kind == "tap" && (strings.Count(m[2], "/") != 1 || strings.Contains(m[3], `"`)) {
			other = append(other, line)
			continue
		}
		if kind == "tap" && (m[2] == "homebrew/core" || m[2] == "homebrew/cask") {
			continue
		}
		if key := kind + ":" + m[2]; !Has(entries, key) {
			entries = append(entries, key)
		}
	}
	return entries, other
}
