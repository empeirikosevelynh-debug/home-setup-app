package inspect

import (
	"context"
	"errors"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// readDotfiles finds out whether chezmoi's source is the chosen dotfiles
// repository and, once it is, which files it restores as they are. It only
// reads: chezmoi keeps its state and cache in a temporary folder that is
// removed afterwards.
func (i Inspector) readDotfiles(ctx context.Context, h *domain.Host, o domain.Options) error {
	if o.DotfilesRepo == "" {
		return nil
	}
	if err := plan.ValidDotfilesRepo(o.DotfilesRepo); err != nil {
		return err
	}
	source := plan.DotfilesSource(*h)
	entries, err := os.ReadDir(source)
	if os.IsNotExist(err) || err == nil && len(entries) == 0 {
		h.DotfilesState = "missing"
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(source, ".git")); err != nil {
		h.DotfilesState = "other"
		return nil
	}
	git := h.Tools["git"]
	if git == "" {
		return fmt.Errorf("Git is required to inspect the existing chezmoi source")
	}
	origin, err := i.capture(ctx, git, "-C", source, "config", "--get", "remote.origin.url")
	if err != nil && command.ExitCode(err) != 1 {
		return err
	}
	if !plan.SameRepo(origin, o.DotfilesRepo) {
		h.DotfilesState, h.DotfilesOrigin = "other", origin
		return nil
	}
	chezmoi := h.Tools["chezmoi"]
	if chezmoi == "" {
		h.DotfilesState = "waiting"
		return nil
	}
	h.DotfilesState = "cloned"
	h.DotfilesScripts = hasScripts(source)
	work, err := os.MkdirTemp("", "golden-chezmoi-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	private := []string{"--persistent-state", filepath.Join(work, "state.boltdb"), "--cache", filepath.Join(work, "cache"), "--no-pager", "--no-tty"}
	listed, err := i.capture(ctx, chezmoi, append([]string{"managed", "--include=files,symlinks", "--path-style=absolute", "--nul-path-separator"}, private...)...)
	if err != nil {
		h.DotfilesProblem = "chezmoi managed failed (" + strings.TrimPrefix(err.Error(), chezmoi+" failed: ") + ")"
		return nil
	}
	var targets []string
	for _, target := range strings.Split(listed, "\x00") {
		if target != "" {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	named, err := i.capture(ctx, chezmoi, append(append(append([]string{"source-path"}, private...), "--"), targets...)...)
	sources := strings.Split(named, "\n")
	if err != nil || len(sources) != len(targets) {
		h.DotfilesProblem = "chezmoi could not name the source of each file"
		return nil
	}
	root, err := filepath.EvalSymlinks(source)
	if err != nil {
		return err
	}
	for k, target := range targets {
		if d, ok := restorable(root, source, sources[k], target, h.Home); ok {
			state, err := ReadFileState(h.Home, target)
			if err != nil {
				return err
			}
			h.Files[target] = state
			h.Dotfiles = append(h.Dotfiles, d)
			continue
		}
		h.DotfilesManual = append(h.DotfilesManual, target)
	}
	return nil
}

// restorable reads a source file setup can write as it is: a plain file,
// directly in chezmoi's source, below this home, and at most 1 MiB.
func restorable(root, source, path, target, home string) (domain.Dotfile, bool) {
	rel, err := filepath.Rel(source, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(target) {
		return domain.Dotfile{}, false
	}
	if t, err := filepath.Rel(home, target); err != nil || t == "." || t == ".." || strings.HasPrefix(t, ".."+string(filepath.Separator)) {
		return domain.Dotfile{}, false
	}
	for _, dir := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		if strings.HasPrefix(dir, "external_") {
			return domain.Dotfile{}, false
		}
	}
	path = filepath.Join(root, rel)
	state, err := (files.Manager{Roots: []string{root}}).Inspect(path)
	if err != nil || !state.Exists {
		return domain.Dotfile{}, false
	}
	plain, create, mode := plan.SourceAttributes(filepath.Base(path), int64(len(state.Contents)))
	if !plain {
		return domain.Dotfile{}, false
	}
	return domain.Dotfile{Target: target, Source: domain.FileSource{Root: root, Path: path, SHA256: state.SHA256}, Mode: mode, Create: create, Contents: state.Contents}, true
}

// hasScripts reports whether the repository has chezmoi scripts, which
// setup never runs.
func hasScripts(source string) bool {
	found := errors.New("found")
	err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.Name() == ".chezmoiscripts" || strings.HasPrefix(d.Name(), "run_"):
			return found
		}
		return nil
	})
	return err == found
}
