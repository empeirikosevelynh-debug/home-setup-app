package plan

import (
	"fmt"
	"golden-gate-setup/internal/domain"
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

// SourceKind reads what chezmoi does with a source file from its name: a
// plain file, a template, an encrypted file, a link or a modify script, and
// whether it is only created where nothing is. ok is false for entries
// that are not files chezmoi writes, such as scripts and removals, and for
// empty files without empty_, which chezmoi removes instead.
func SourceKind(name string, size int64) (kind string, create, ok bool) {
	templated := false
	if literal, found := strings.CutSuffix(name, ".literal"); found {
		name = literal
	} else {
		name, templated = strings.CutSuffix(name, ".tmpl")
	}
	name, create = strings.CutPrefix(name, "create_")
	kind = "file"
	switch {
	case strings.HasPrefix(name, "remove_") || strings.HasPrefix(name, "run_"):
		return "", false, false
	case strings.HasPrefix(name, "modify_"):
		return "modify", false, true
	case strings.HasPrefix(name, "symlink_"):
		kind = "link"
	case strings.HasPrefix(name, "encrypted_"):
		return "encrypted", create, true
	}
	if templated {
		if kind == "link" {
			return "link-template", false, true
		}
		return "template", create, true
	}
	for _, prefix := range []string{"private_", "readonly_"} {
		name = strings.TrimPrefix(name, prefix)
	}
	if _, empty := strings.CutPrefix(name, "empty_"); kind == "file" && size == 0 && !empty {
		return "", false, false
	}
	return kind, create, true
}

// dotfileTargets lists what the folder import leaves alone: chezmoi's
// state, which would mark run-once scripts as run, and once a repository
// is chosen, its configuration, its source and everything it manages.
func dotfileTargets(h domain.Host, o domain.Options) []string {
	config := filepath.Join(h.Home, ".config/chezmoi")
	targets := []string{filepath.Join(config, "chezmoistate.boltdb")}
	if o.DotfilesRepo == "" {
		return targets
	}
	targets = append(targets, DotfilesSource(h))
	for _, format := range []string{"toml", "yaml", "json", "jsonc"} {
		targets = append(targets, filepath.Join(config, "chezmoi."+format))
	}
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

// DotfileCommand is how chezmoi applies one reviewed target: only that
// target, without scripts, removals or folder changes, and without asking,
// since setup has already backed up a file it replaces.
func DotfileCommand(target string, interactive bool) *domain.Command {
	args := []string{"apply", "--force", "--exclude=scripts,remove,dirs", "--no-pager"}
	if !interactive {
		args = append(args, "--no-tty")
	}
	return &domain.Command{Args: append(args, "--", target), Interactive: interactive}
}

// dotfileLabel describes a dotfile step in the plan.
func dotfileLabel(h domain.Host, d domain.Dotfile, replace bool) string {
	label := "chezmoi: create " + homeList(h.Home, []string{d.Target})
	if replace {
		label = "chezmoi: replace " + homeList(h.Home, []string{d.Target})
	}
	switch d.Kind {
	case "template", "link-template":
		label += " (template)"
	case "encrypted":
		label += " (encrypted; may ask for your passphrase)"
	case "modify":
		label += " (your repository's modify script edits it)"
	case "link":
		label += " (link)"
	}
	if replace {
		label += "; backup saved first"
	}
	return label
}

// addDotfiles plans what chezmoi applies through the usual review: targets
// created where missing, and existing ones replaced only when approved,
// after a backup. It returns every target the repository manages, which
// the starter configuration leaves alone.
func addDotfiles(p *domain.Plan, h domain.Host, o domain.Options, add func(domain.Step)) map[string]bool {
	managed := map[string]bool{}
	if o.DotfilesRepo == "" || h.DotfilesState != "cloned" {
		return managed
	}
	manual := append([]string(nil), h.DotfilesManual...)
	var kept []string
	for _, d := range h.Dotfiles {
		managed[d.Target] = true
		if !within(h.Home, d.Target) || d.Target == h.Home {
			manual = append(manual, d.Target)
			continue
		}
		if d.Present {
			continue
		}
		before := h.Files[d.Target]
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
		change := domain.FileChange{Path: d.Target, BeforeSHA256: before.SHA256, BeforeExists: before.Exists, Mode: before.Mode, Decision: decision}
		add(domain.Step{ID: "dotfile:" + d.Target, Label: dotfileLabel(h, d, before.Exists), Kind: "dotfile", File: &change, Command: DotfileCommand(d.Target, d.Interactive), Check: domain.Check{Kind: "dotfile", Target: d.Target, Expected: d.SHA256}})
	}
	for _, target := range h.DotfilesManual {
		managed[target] = true
	}
	switch {
	case h.DotfilesProblem != "":
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-chezmoi", Title: "Apply your dotfiles with chezmoi", Instructions: "Setup could not list your dotfiles: " + h.DotfilesProblem + ". Review the changes with chezmoi diff, then apply them with chezmoi apply.", Required: false})
	case len(manual) > 0:
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-chezmoi", Title: "Finish applying your dotfiles", Instructions: "These are outside your home folder or come from an external source, so setup leaves them to chezmoi: " + homeList(h.Home, manual) + ". Review them with chezmoi diff, then apply them with chezmoi apply.", Required: false})
	}
	if h.DotfilesScripts {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-scripts", Title: "Run your dotfiles' scripts", Instructions: "Setup does not run your repository's scripts. Review them in chezmoi's source, then run them with chezmoi apply --include=scripts.", Required: false})
	}
	if h.DotfilesRemovals {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-removals", Title: "Review what your dotfiles remove", Instructions: "Your repository removes files (remove_ entries, exact_ folders or .chezmoiremove). Setup never deletes anything; review with chezmoi diff and apply removals yourself with chezmoi apply.", Required: false})
	}
	if len(kept) > 0 {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-kept", Title: "Compare the dotfiles kept here", Instructions: "Your dotfiles repository has other versions of " + homeList(h.Home, kept) + ". The files here were kept; compare them with chezmoi diff and apply the ones you want with chezmoi apply followed by their paths.", Required: false})
	}
	return managed
}
