package apply

import (
	"context"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"io"
	"path/filepath"
)

func (e *Executor) verify(ctx context.Context, h domain.Host, p domain.Plan, s domain.Step, dir string) (bool, error) {
	if er := ctx.Err(); er != nil {
		return false, er
	}
	switch s.Kind {
	case "package":
		if s.Package == nil {
			return false, fmt.Errorf("missing package")
		}
		v, ok := h.Packages[s.Check.Target]
		if !ok {
			return false, nil
		}
		if s.Package.MinVersion != "" && !plan.AtLeastVersion(v.Version, s.Package.MinVersion) {
			return false, fmt.Errorf("review an update for %s before continuing", s.Package.Token)
		}
		return true, nil
	case "file":
		if s.File == nil {
			return false, fmt.Errorf("missing file change")
		}
		f, err := e.Files.Inspect(s.File.Path)
		okay := err == nil && f.Exists && f.SHA256 == s.Check.Expected
		if okay && s.File.Path == filepath.Join(h.Home, ".config/fish/config.fish") {
			err = e.Runner.Run(ctx, domain.Command{Path: tool(h, "fish"), Args: []string{"--no-config", "--no-execute", s.File.Path}}, io.Discard)
		}
		return okay && err == nil, err
	default:
		handler, ok := e.Handlers[s.Kind]
		if !ok || handler.Verify == nil {
			return false, fmt.Errorf("%s verification is unavailable", s.Kind)
		}
		return handler.Verify(ctx, h, p, s, dir)
	}
}
