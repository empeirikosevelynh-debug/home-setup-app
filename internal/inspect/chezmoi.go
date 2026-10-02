package inspect

import (
	"context"
	"fmt"
	"golden-gate-setup/internal/domain"
	"os"
	"path/filepath"
)

func (i Inspector) readChezmoi(ctx context.Context, h *domain.Host, path string) error {
	source, err := i.capture(ctx, path, "source-path")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(source) {
		return fmt.Errorf("chezmoi returned an invalid source directory")
	}
	h.ChezmoiDir = source
	_, err = os.Stat(filepath.Join(source, ".git"))
	if os.IsNotExist(err) {
		entries, e := os.ReadDir(source)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		h.ChezmoiDirty = len(entries) > 0
		return nil
	}
	if err != nil {
		return err
	}
	git := h.Tools["git"]
	if git == "" {
		return fmt.Errorf("Git is required to inspect the existing chezmoi repository")
	}
	status, err := i.capture(ctx, git, "-C", source, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	h.ChezmoiDirty = status != ""
	return nil
}
