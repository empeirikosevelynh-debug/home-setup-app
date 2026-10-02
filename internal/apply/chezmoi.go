package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func chezmoiConfig(ctx context.Context, h domain.Host) (string, func(), error) {
	m := files.Manager{Roots: []string{h.Home}}
	work := filepath.Join(h.Home, "Library/Application Support/Golden Gate Setup/work")
	if e := m.EnsurePrivateDir(work); e != nil {
		return "", nil, e
	}
	dir, e := os.MkdirTemp(work, "chezmoi-")
	if e != nil {
		return "", nil, e
	}
	cleanup := func() { os.RemoveAll(dir) }
	src := h.ChezmoiDir
	if src == "" {
		src = filepath.Join(h.Home, ".local/share/chezmoi")
	}
	data, _ := json.Marshal(map[string]any{"sourceDir": src, "persistentState": filepath.Join(dir, "state.boltdb"), "cacheDir": filepath.Join(dir, "cache"), "tempDir": dir, "destDir": h.Home, "git": map[string]any{"autoAdd": false, "autoCommit": false, "autoPush": false}, "hooks": map[string]any{}, "color": false})
	path := filepath.Join(dir, "config.json")
	_, e = m.ApplyContext(ctx, domain.FileChange{Path: path, Desired: data, Mode: 0600, Decision: domain.Create}, dir)
	if e != nil {
		cleanup()
		return "", nil, e
	}
	return path, cleanup, nil
}
func managed(ctx context.Context, h domain.Host, r command.Runner, config string) (map[string]bool, error) {
	var b bytes.Buffer
	args := []string{"managed", "--include=files", "--path-style=absolute", "--nul-path-separator", "--config", config, "--no-pager", "--error-on-conflict"}
	if e := r.Run(ctx, domain.Command{Path: h.Tools["chezmoi"], Args: args}, &b); e != nil {
		return nil, e
	}
	result := map[string]bool{}
	for _, path := range strings.Split(b.String(), "\x00") {
		if path != "" {
			result[strings.TrimSpace(path)] = true
		}
	}
	return result, nil
}
func AdoptChezmoi(ctx context.Context, h domain.Host, chosen []string, r command.Runner) error {
	if h.ChezmoiDirty {
		return ErrDeferred
	}
	if len(chosen) == 0 {
		return nil
	}
	if h.Tools["chezmoi"] == "" {
		return fmt.Errorf("chezmoi unavailable")
	}
	config, cleanup, e := chezmoiConfig(ctx, h)
	if e != nil {
		return e
	}
	defer cleanup()
	src := h.ChezmoiDir
	if src == "" {
		src = filepath.Join(h.Home, ".local/share/chezmoi")
	}
	entries, readErr := os.ReadDir(src)
	if os.IsNotExist(readErr) || readErr == nil && len(entries) == 0 {
		if e = r.Run(ctx, domain.Command{Path: h.Tools["chezmoi"], Args: []string{"init", "--config", config, "--no-pager", "--error-on-conflict"}}, io.Discard); e != nil {
			return e
		}
	} else if readErr != nil {
		return readErr
	}
	tracked, e := managed(ctx, h, r, config)
	if e != nil {
		return e
	}
	for _, path := range chosen {
		if tracked[path] {
			continue
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = r.Run(ctx, domain.Command{Path: h.Tools["chezmoi"], Args: []string{"add", "--follow", "--recursive=false", "--autotemplate=false", "--template=false", "--encrypt=false", "--config", config, "--no-pager", "--error-on-conflict", "--", path}}, io.Discard); e != nil {
			return e
		}
	}
	return r.Run(ctx, domain.Command{Path: h.Tools["chezmoi"], Args: append([]string{"status", "--config", config, "--no-pager", "--"}, chosen...)}, io.Discard)
}
