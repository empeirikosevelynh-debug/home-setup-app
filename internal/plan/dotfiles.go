package plan

import (
	"fmt"
	"golden-gate-setup/internal/domain"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode"
)

// DotfilesSource is chezmoi's source folder: the one chezmoi reports, or
// its default before chezmoi is installed.
func DotfilesSource(h domain.Host) string {
	if h.ChezmoiDir != "" {
		return h.ChezmoiDir
	}
	return filepath.Join(h.Home, ".local/share/chezmoi")
}

// ValidDotfilesRepo checks a repository the way chezmoi init takes it: a
// GitHub user, user/repo or a Git URL, never an option.
func ValidDotfilesRepo(repo string) error {
	if repo == "" || len(repo) > 512 || strings.HasPrefix(repo, "-") || strings.ContainsFunc(repo, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return fmt.Errorf("enter your dotfiles repository as a GitHub user, user/repo or a Git URL")
	}
	return nil
}

// repoKey reduces a repository to host/path, expanding the short forms
// chezmoi init accepts, so it can be compared with a clone's origin.
func repoKey(repo string) string {
	r := strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if scheme := strings.Index(r, "://"); scheme >= 0 {
		r = r[scheme+3:]
		host, path, _ := strings.Cut(r, "/")
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		r = host + "/" + path
	} else if at, colon := strings.Index(r, "@"), strings.Index(r, ":"); at >= 0 && colon > at {
		r = r[at+1:colon] + "/" + r[colon+1:]
	} else if strings.HasPrefix(r, "sr.ht/") {
		r = "git." + r
		if strings.Count(r, "/") == 1 {
			r += "/dotfiles"
		}
	} else {
		switch strings.Count(r, "/") {
		case 0:
			r = "github.com/" + r + "/dotfiles"
		case 1:
			r = "github.com/" + r
		}
	}
	return strings.ToLower(r)
}

// SameRepo reports whether a clone's origin is the chosen repository.
func SameRepo(origin, repo string) bool {
	return origin != "" && repoKey(origin) == repoKey(repo)
}

// SourceAttributes reads what chezmoi does with a source file from its
// name. Plain files are written as they are, with mode; create files only
// where nothing is. Templates, encrypted, modify and script files, links
// and empty files without empty_ need chezmoi itself.
func SourceAttributes(name string, size int64) (plain, create bool, mode fs.FileMode) {
	if literal, ok := strings.CutSuffix(name, ".literal"); ok {
		name = literal
	} else if strings.HasSuffix(name, ".tmpl") {
		return false, false, 0
	}
	name, create = strings.CutPrefix(name, "create_")
	for _, kind := range []string{"modify_", "remove_", "run_", "symlink_", "encrypted_"} {
		if strings.HasPrefix(name, kind) {
			return false, false, 0
		}
	}
	name, private := strings.CutPrefix(name, "private_")
	name, readonly := strings.CutPrefix(name, "readonly_")
	name, empty := strings.CutPrefix(name, "empty_")
	_, executable := strings.CutPrefix(name, "executable_")
	mode = 0666
	if executable {
		mode = 0777
	}
	if private {
		mode &^= 0077
	}
	if readonly {
		mode &^= 0222
	}
	if size == 0 && !empty {
		return false, false, 0
	}
	return true, create, mode &^ 0022
}

// dotfileTargets lists everything chezmoi manages once the repository is
// cloned, and the folders that belong to chezmoi itself; the folder import
// leaves them all alone.
func dotfileTargets(h domain.Host, o domain.Options) []string {
	if o.DotfilesRepo == "" || h.DotfilesState != "cloned" {
		return nil
	}
	targets := []string{DotfilesSource(h), filepath.Join(h.Home, ".config/chezmoi")}
	for _, d := range h.Dotfiles {
		targets = append(targets, d.Target)
	}
	return append(targets, h.DotfilesManual...)
}

// addClone plans cloning the dotfiles repository when chezmoi has no source
// yet. It reports whether the rest of setup waits for a second review: what
// the repository restores is known only once it is cloned.
func addClone(p *domain.Plan, h domain.Host, o domain.Options, add func(domain.Step)) (bool, error) {
	if o.DotfilesRepo == "" {
		return false, nil
	}
	if err := ValidDotfilesRepo(o.DotfilesRepo); err != nil {
		return false, err
	}
	switch h.DotfilesState {
	case "missing":
		add(domain.Step{ID: "chezmoi-init", Label: "Clone your dotfiles from " + repoKey(o.DotfilesRepo) + " with chezmoi", Kind: "chezmoi-init", Command: &domain.Command{Args: []string{"init", "--", o.DotfilesRepo}, Interactive: true}, Check: domain.Check{Kind: "chezmoi-init", Target: DotfilesSource(h), Expected: o.DotfilesRepo}})
		return true, nil
	case "waiting":
		return true, nil
	case "other":
		origin := "no remote"
		if h.DotfilesOrigin != "" {
			origin = "origin " + h.DotfilesOrigin
		}
		p.Supported = false
		p.Problems = append(p.Problems, "chezmoi's source folder "+DotfilesSource(h)+" already holds other dotfiles ("+origin+"). Setup does not replace it: move it aside or clear the dotfiles repository, then inspect again.")
	case "cloned":
	default:
		return false, fmt.Errorf("the dotfiles repository was not inspected")
	}
	return false, nil
}

// homeList names paths relative to home, the first dozen of them.
func homeList(home string, paths []string) string {
	var names []string
	for i, path := range paths {
		if i == 12 {
			names = append(names, fmt.Sprintf("and %d more", len(paths)-i))
			break
		}
		if rel, err := filepath.Rel(home, path); err == nil && within(home, path) {
			path = "~/" + rel
		}
		names = append(names, path)
	}
	return strings.Join(names, ", ")
}

// addRestore plans the repository's plain files through the usual review:
// created where missing, kept where different unless a replacement is
// approved. It returns every target the repository manages, which the
// starter templates leave alone.
func addRestore(p *domain.Plan, h domain.Host, o domain.Options, add func(domain.Step)) map[string]bool {
	restored := map[string]bool{}
	if o.DotfilesRepo == "" || h.DotfilesState != "cloned" {
		return restored
	}
	manual := append([]string(nil), h.DotfilesManual...)
	var kept []string
	for _, d := range h.Dotfiles {
		restored[d.Target] = true
		if !within(h.Home, d.Target) || d.Target == h.Home {
			manual = append(manual, d.Target)
			continue
		}
		before := h.Files[d.Target]
		if before.Exists && !before.Symlink && before.SHA256 == d.Source.SHA256 {
			continue
		}
		decision := domain.Create
		if before.Exists {
			if d.Create {
				continue
			}
			decision = domain.Preserve
			if value, ok := o.FileChoices[d.Target]; ok {
				decision = value
			}
			if before.Symlink || !before.Mode.IsRegular() && before.Mode != 0 || decision != domain.Replace {
				kept = append(kept, d.Target)
				continue
			}
		}
		source := d.Source
		change := domain.FileChange{Path: d.Target, BeforeSHA256: before.SHA256, BeforeExists: before.Exists, Mode: d.Mode, Decision: decision, Source: &source}
		add(domain.Step{ID: "file:" + d.Target, Label: "Restore " + homeList(h.Home, []string{d.Target}) + " from your dotfiles", Kind: "file", File: &change, Check: domain.Check{Kind: "file", Target: d.Target, Expected: source.SHA256}})
	}
	for _, target := range h.DotfilesManual {
		restored[target] = true
	}
	switch {
	case h.DotfilesProblem != "":
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-chezmoi", Title: "Restore your dotfiles with chezmoi", Instructions: "Setup could not list your dotfiles: " + h.DotfilesProblem + ". Review the changes with chezmoi diff, then apply them with chezmoi apply.", Required: false})
	case len(manual) > 0 || h.DotfilesScripts:
		text := ""
		if len(manual) > 0 {
			text = "These need chezmoi itself (templates, encrypted or modified files, links): " + homeList(h.Home, manual) + ". "
		}
		if h.DotfilesScripts {
			text += "Setup does not run your repository's scripts. "
		}
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-chezmoi", Title: "Finish restoring your dotfiles", Instructions: text + "Review the changes with chezmoi diff, then apply them with chezmoi apply.", Required: false})
	}
	if len(kept) > 0 {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-kept", Title: "Compare the dotfiles kept here", Instructions: "Your dotfiles repository has other versions of " + homeList(h.Home, kept) + ". The files here were kept; compare them with chezmoi diff and apply the ones you want with chezmoi apply followed by their paths.", Required: false})
	}
	return restored
}
