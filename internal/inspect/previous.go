package inspect

import (
	"context"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"path/filepath"
	"strings"
)

// readPrevious reads the previous Mac's app list and the current taps, only
// when an app list was chosen.
func (i Inspector) readPrevious(ctx context.Context, h *domain.Host, o domain.Options) error {
	path := o.PreviousBrewfile
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("the previous app list must be a clean absolute path")
	}
	s, e := ReadFileState(filepath.Dir(path), path)
	if e != nil {
		return fmt.Errorf("read the previous app list: %w", e)
	}
	if s.Symlink {
		return fmt.Errorf("choose the previous app list itself; links are not followed: %s", path)
	}
	if !s.Exists {
		return fmt.Errorf("previous app list not found: %s", path)
	}
	h.PreviousEntries, h.PreviousOther = plan.ParseBrewfile(s.Contents)
	if h.BrewPath == "" {
		return nil
	}
	raw, e := i.capture(ctx, h.BrewPath, "tap")
	if e != nil {
		return e
	}
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			h.Taps = append(h.Taps, line)
		}
	}
	return nil
}
