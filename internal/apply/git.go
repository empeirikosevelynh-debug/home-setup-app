package apply

import (
	"bytes"
	"context"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var GitSettings = [][2]string{{"core.editor", "zed --wait"}, {"core.pager", "delta"}, {"interactive.diffFilter", "delta --color-only"}, {"delta.navigate", "true"}}

func gitTarget(h domain.Host) string {
	primary := filepath.Join(h.Home, ".gitconfig")
	xdg := filepath.Join(h.Home, ".config/git/config")
	if !h.Files[primary].Exists && h.Files[xdg].Exists {
		return xdg
	}
	return primary
}
func ConfigureGit(ctx context.Context, h domain.Host, r command.Runner) error {
	return configureGitAt(ctx, h, r, filepath.Join(h.Home, "Library/Application Support/Golden Gate Setup/configuration-backups"))
}
func configureGitAt(ctx context.Context, h domain.Host, r command.Runner, backups string) error {
	tool := h.Tools["git"]
	if tool == "" {
		return fmt.Errorf("Git unavailable")
	}
	target := gitTarget(h)
	if external := os.Getenv("GIT_CONFIG_GLOBAL"); external != "" && external != target {
		return ErrDeferred
	}
	m := files.Manager{Roots: []string{h.Home}}
	before := h.Files[target]
	if before.Symlink || before.Exists && !before.Mode.IsRegular() {
		return ErrDeferred
	}
	if before.Exists {
		overridden, e := includeOverrides(ctx, r, tool, target)
		if e != nil {
			return e
		}
		if overridden {
			return ErrDeferred
		}
	}
	work := filepath.Join(h.Home, "Library/Application Support/Golden Gate Setup/work")
	if e := m.EnsurePrivateDir(work); e != nil {
		return e
	}
	dir, e := os.MkdirTemp(work, "git-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	temp := filepath.Join(dir, "config")
	if _, e = m.ApplyContext(ctx, domain.FileChange{Path: temp, Desired: before.Contents, Mode: 0600, Decision: domain.Create}, backups); e != nil {
		return e
	}
	for _, setting := range GitSettings {
		if e = ctx.Err(); e != nil {
			return e
		}
		e = r.Run(ctx, domain.Command{Path: tool, Args: []string{"config", "--file", temp, "--unset-all", setting[0]}}, io.Discard)
		if e != nil && command.ExitCode(e) != 5 {
			return e
		}
		if e = r.Run(ctx, domain.Command{Path: tool, Args: []string{"config", "--file", temp, "--add", setting[0], setting[1]}}, io.Discard); e != nil {
			return e
		}
	}
	desired, e := m.Inspect(temp)
	if e != nil {
		return e
	}
	decision := domain.Create
	if before.Exists {
		decision = domain.Replace
	}
	_, e = m.ApplyContext(ctx, domain.FileChange{Path: target, BeforeExists: before.Exists, BeforeSHA256: before.SHA256, Desired: desired.Contents, Mode: 0644, Decision: decision}, backups)
	return e
}

// includeOverrides reports whether a file included from target sets one of
// GitSettings to another value. Only target is edited, and an include that
// follows the edited section would win, so that configuration stays the user's.
func includeOverrides(ctx context.Context, r command.Runner, tool, target string) (bool, error) {
	values := func(key string, includes bool) ([]string, error) {
		args := []string{"config", "--file", target, "--null"}
		if includes {
			args = append(args, "--includes")
		}
		var b bytes.Buffer
		e := r.Run(ctx, domain.Command{Path: tool, Args: append(args, "--get-all", key)}, &b)
		if command.ExitCode(e) == 1 {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		return strings.Split(strings.TrimSuffix(b.String(), "\x00"), "\x00"), nil
	}
	for _, setting := range GitSettings {
		own, e := values(setting[0], false)
		if e != nil {
			return false, e
		}
		all, e := values(setting[0], true)
		if e != nil {
			return false, e
		}
		remaining := map[string]int{}
		for _, v := range own {
			remaining[v]++
		}
		for _, v := range all {
			if remaining[v] > 0 {
				remaining[v]--
			} else if v != setting[1] {
				return true, nil
			}
		}
	}
	return false, nil
}
