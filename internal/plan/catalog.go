package plan

import (
	"golden-gate-setup/internal/domain"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var Core = []string{"fish", "starship", "zoxide", "chezmoi", "gh", "fzf", "fd", "bat", "eza", "ripgrep", "git-delta", "lazygit"}
var Apps = []string{"warp", "zed", "applite", "github", "kopiaui"}
var StartingPlugins = []string{"jhillyerd/plugin-git", "PatrickF1/fzf.fish", "jorgebucaran/autopair.fish", "decors/fish-colored-man"}
var ExtraPlugins = []string{"franciscolourenco/done", "nickeb96/puffer-fish", "gazorby/fish-abbreviation-tips", "meaningful-ooo/sponge", "edc/bass", "oh-my-fish/plugin-sudope"}
var Languages = []string{"go", "crystal", "nim"}
var LanguagePackages = map[string][]string{"go": {"go", "golangci-lint", "delve"}, "crystal": {"crystal", "crystalline", "ameba"}, "nim": {"nim"}}
var AppNames = map[string]string{"warp": "Warp", "zed": "Zed", "applite": "Applite", "github": "GitHub Desktop", "kopiaui": "KopiaUI"}

func DefaultOptions() domain.Options {
	return domain.Options{RecoveryDate: time.Now().Format("2006-01-02"),
		Apps: []string{"warp", "zed", "applite"}, Plugins: append([]string(nil), StartingPlugins...), ConfigureGit: true, AdoptChezmoi: true, CaptureInventory: true, PrepareRecovery: true, FileChoices: map[string]domain.FileDecision{}}
}
func Has(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

var versionNumber = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)*`)

func AtLeastVersion(value, minimum string) bool {
	a, b := versionNumber.FindString(value), versionNumber.FindString(minimum)
	if a == "" || b == "" {
		return false
	}
	av, bv := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(av), len(bv)); i++ {
		x, y := 0, 0
		if i < len(av) {
			x, _ = strconv.Atoi(av[i])
		}
		if i < len(bv) {
			y, _ = strconv.Atoi(bv[i])
		}
		if x != y {
			return x > y
		}
	}
	return true
}
