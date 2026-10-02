package inspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	h.DotfilesScripts, h.DotfilesRemovals = sourceFeatures(source)
	work, err := os.MkdirTemp("", "golden-chezmoi-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	private := []string{"--persistent-state", filepath.Join(work, "state.boltdb"), "--cache", filepath.Join(work, "cache"), "--no-pager", "--no-tty", "--skip-secrets"}
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
	written := i.lastWritten(ctx, chezmoi)
	for k, target := range targets {
		d, ok := i.dotfile(ctx, chezmoi, private, root, source, sources[k], target, h.Home)
		state, err := ReadFileState(h.Home, target)
		if !ok || err != nil {
			h.DotfilesManual = append(h.DotfilesManual, target)
			continue
		}
		h.Files[target] = state
		d.Present = present(d, state, written[target])
		h.Dotfiles = append(h.Dotfiles, d)
	}
	return nil
}

// risky names template functions that run commands, read secrets or reach
// the network. Templates using them are not rendered while inspecting;
// chezmoi renders them when it applies the reviewed file.
var risky = regexp.MustCompile(`\b(output|outputList|exec|include|includeTemplate|template|onepassword\w*|bitwarden\w*|pass\w*|gopass\w*|keepassxc\w*|keeper\w*|lastpass\w*|vault|secret\w*|awsSecretsManager\w*|azureKeyVault\w*|dashlane\w*|doppler\w*|ejson\w*|hcpVaultSecret\w*|keyring|protonPass\w*|rbw\w*|gitHub\w*|decrypt)\b`)

// dotfile describes one target chezmoi manages, with the contents chezmoi
// will write when they can be known without running anything: plain files
// and links from the source, templates rendered by chezmoi cat. Encrypted
// files, modify scripts and risky templates are known only once applied.
func (i Inspector) dotfile(ctx context.Context, chezmoi string, private []string, root, source, path, target, home string) (domain.Dotfile, bool) {
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
	src, err := (files.Manager{Roots: []string{root}}).Inspect(filepath.Join(root, rel))
	if err != nil || !src.Exists {
		return domain.Dotfile{}, false
	}
	kind, create, ok := plan.SourceKind(filepath.Base(rel), int64(len(src.Contents)))
	if !ok {
		return domain.Dotfile{}, false
	}
	d := domain.Dotfile{Target: target, Kind: kind, Create: create}
	switch kind {
	case "file":
		d.Contents = src.Contents
	case "link":
		d.Contents = []byte(strings.TrimSpace(string(src.Contents)))
	case "template", "link-template":
		if !risky.Match(src.Contents) {
			out, err := i.captureRaw(ctx, chezmoi, append(append([]string{"cat"}, private...), "--", target)...)
			if err == nil && len(out) == 0 {
				return domain.Dotfile{}, false
			}
			if err == nil && kind == "link-template" {
				out = []byte(strings.TrimSpace(string(out)))
			}
			if err == nil {
				d.Contents = out
			}
		}
	}
	if d.Contents == nil {
		d.Interactive = true
		return d, true
	}
	sum := sha256.Sum256(d.Contents)
	d.SHA256 = hex.EncodeToString(sum[:])
	return d, true
}

// present reports whether a target already has what chezmoi would write:
// the same contents or link, or, when those are known only once applied,
// what chezmoi itself last wrote there.
func present(d domain.Dotfile, current domain.FileState, written string) bool {
	if d.Kind == "link" || d.Kind == "link-template" {
		link, err := os.Readlink(d.Target)
		sum := sha256.Sum256([]byte(link))
		return err == nil && (d.SHA256 == hex.EncodeToString(sum[:]) || d.SHA256 == "" && written == hex.EncodeToString(sum[:]))
	}
	if !current.Exists || current.Symlink {
		return false
	}
	if d.SHA256 != "" {
		return current.SHA256 == d.SHA256
	}
	return written != "" && current.SHA256 == written
}

// lastWritten reads, from chezmoi's own records, the contents it last wrote
// to each target. It only reads.
func (i Inspector) lastWritten(ctx context.Context, chezmoi string) map[string]string {
	out, err := i.captureRaw(ctx, chezmoi, "state", "dump", "--format=json", "--no-pager", "--no-tty")
	var dump struct {
		EntryState map[string]struct {
			ContentsSHA256 string `json:"contentsSHA256"`
		} `json:"entryState"`
	}
	written := map[string]string{}
	if err == nil && json.Unmarshal(out, &dump) == nil {
		for target, entry := range dump.EntryState {
			written[target] = strings.ToLower(entry.ContentsSHA256)
		}
	}
	return written
}

// sourceFeatures reports whether the repository has scripts, which setup
// never runs, and removals, which setup never makes.
func sourceFeatures(source string) (scripts, removals bool) {
	filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		switch name := d.Name(); {
		case err != nil:
			return nil
		case d.IsDir() && name == ".git":
			return filepath.SkipDir
		case name == ".chezmoiscripts" || strings.HasPrefix(name, "run_"):
			scripts = true
		case name == ".chezmoiremove" || strings.HasPrefix(name, "remove_") || strings.HasPrefix(name, "exact_"):
			removals = true
		}
		return nil
	})
	return scripts, removals
}
