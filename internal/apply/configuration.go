package apply

import (
	"bytes"
	"context"
	"errors"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"path/filepath"
	"strings"
)

func SelectedFiles(h domain.Host, p domain.Plan) []string {
	allowed := map[string]bool{filepath.Join(h.Home, ".config/fish/config.fish"): true, filepath.Join(h.Home, ".config/starship.toml"): true, plan.ZedSettingsPath(h): true, filepath.Join(h.LazyGitDir, "config.yml"): true, filepath.Join(h.Home, ".Brewfile"): true, filepath.Join(h.Home, ".kopiaignore"): true}
	var result []string
	for _, s := range p.Steps {
		if s.File != nil && allowed[s.File.Path] {
			result = append(result, s.File.Path)
		}
		if s.Kind == "plugins" {
			result = append(result, filepath.Join(h.Home, ".config/fish/fish_plugins"))
		}
	}
	return result
}
func ConfigurationHandlers(r command.Runner) map[string]Handler {
	action := func(f func(context.Context, domain.Host, domain.Plan, string) error) func(context.Context, domain.Host, domain.Plan, domain.Step, string) (domain.StepResult, error) {
		return func(ctx context.Context, h domain.Host, p domain.Plan, s domain.Step, dir string) (domain.StepResult, error) {
			e := f(ctx, h, p, dir)
			if errors.Is(e, ErrDeferred) {
				return domain.StepResult{ID: s.ID, Status: "preserved", Message: "Existing configuration needs individual review; no destructive reconciliation was performed."}, nil
			}
			return domain.StepResult{ID: s.ID}, e
		}
	}
	return map[string]Handler{
		"git": {Apply: action(func(c context.Context, h domain.Host, p domain.Plan, d string) error {
			return configureGitAt(c, h, r, d)
		}), Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			if h.Tools["git"] == "" {
				return false, nil
			}
			target := gitTarget(h)
			if !h.Files[target].Exists {
				return false, nil
			}
			for _, v := range GitSettings {
				var b bytes.Buffer
				e := r.Run(c, domain.Command{Path: h.Tools["git"], Args: []string{"config", "--file", target, "--includes", "--get", v[0]}}, &b)
				if e != nil && command.ExitCode(e) == 1 {
					return false, nil
				}
				if e != nil {
					return false, e
				}
				if strings.TrimSpace(b.String()) != v[1] {
					return false, nil
				}
			}
			return true, nil
		}},
		"plugins": {Apply: action(func(c context.Context, h domain.Host, p domain.Plan, d string) error {
			return InstallPlugins(c, h, p.Options.Plugins, r)
		}), Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			return plan.PluginsSatisfied(h, p.Options.Plugins), nil
		}},
		"chezmoi": {Apply: action(func(c context.Context, h domain.Host, p domain.Plan, d string) error {
			return AdoptChezmoi(c, h, SelectedFiles(h, p), r)
		}), Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			chosen := SelectedFiles(h, p)
			if len(chosen) == 0 {
				return true, nil
			}
			if h.Tools["chezmoi"] == "" {
				return false, nil
			}
			cfg, cleanup, e := chezmoiConfig(c, h)
			if e != nil {
				return false, e
			}
			defer cleanup()
			tracked, e := managed(c, h, r, cfg)
			if e != nil {
				return false, e
			}
			for _, path := range chosen {
				if !tracked[path] {
					return false, nil
				}
			}
			return true, nil
		}},
	}
}
