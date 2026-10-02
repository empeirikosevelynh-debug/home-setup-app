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

// Chocolatey package ids for the same choices on Windows. Fish and its
// plugins, Applite, Crystal and Delve have no Chocolatey package.
var WindowsCore = []string{"starship", "zoxide", "chezmoi", "gh", "fzf", "fd", "bat", "eza", "ripgrep", "delta", "lazygit", "git"}
var WindowsApps = []string{"warp", "zed", "github", "kopiaui"}
var WindowsAppPackages = map[string]string{"warp": "warp-terminal", "zed": "zed-editor", "github": "github-desktop", "kopiaui": "kopiaui"}
var WindowsLanguages = []string{"go", "nim"}
var WindowsLanguagePackages = map[string][]string{"go": {"golang", "golangci-lint"}, "nim": {"nim"}}

func DefaultOptions() domain.Options {
	return domain.Options{RecoveryDate: time.Now().Format("2006-01-02"),
		Apps: []string{"warp", "zed", "applite"}, Plugins: append([]string(nil), StartingPlugins...), ConfigureGit: true, AdoptChezmoi: true, CaptureInventory: true, PrepareRecovery: true, FileChoices: map[string]domain.FileDecision{}}
}

// AppFor names the app a package installs (a catalog key such as "zed"),
// or returns "" for a command-line package.
func AppFor(q domain.Package) string {
	switch q.Kind {
	case "cask":
		return q.Token
	case "choco":
		for app, id := range WindowsAppPackages {
			if id == q.Token {
				return app
			}
		}
	}
	return ""
}

// Choice is what the wizard offers on one platform.
type Choice struct{ Apps, Plugins, Languages []string }

func Choices(goos string) Choice {
	if goos == "windows" {
		return Choice{Apps: WindowsApps, Languages: WindowsLanguages}
	}
	return Choice{Apps: Apps, Plugins: append(append([]string{}, StartingPlugins...), ExtraPlugins...), Languages: Languages}
}

// DefaultOptionsFor is DefaultOptions for the given platform. Windows has no
// Fish plugins or recovery records yet.
func DefaultOptionsFor(goos string) domain.Options {
	if goos != "windows" {
		return DefaultOptions()
	}
	return domain.Options{RecoveryDate: time.Now().Format("2006-01-02"), Apps: []string{"warp", "zed"}, ConfigureGit: true, AdoptChezmoi: true, FileChoices: map[string]domain.FileDecision{}}
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
