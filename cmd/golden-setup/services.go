package main

import (
	"context"
	"golden-gate-setup/internal/apply"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/ui"
	"os"
	"path/filepath"
)

func setupServices(probe func(context.Context, domain.Options) (domain.Host, error)) ui.Services {
	home, _ := os.UserHomeDir()
	runner := &command.ProcessRunner{}
	store := apply.SessionStore{Dir: filepath.Join(home, "Library/Application Support/Golden Gate Setup/sessions"), Root: home}
	s := ui.Services{Inspect: probe, Build: plan.Build, LoadLatest: store.Latest}
	s.BindHandoff = func(f func(context.Context, domain.Command) error) { runner.Handoff = f }
	s.Apply = func(ctx context.Context, p domain.Plan, emit func(domain.Event)) (domain.Report, error) {
		h, e := probe(ctx, p.Options)
		if e != nil {
			return domain.Report{}, e
		}
		roots := []string{h.Home}
		for _, w := range p.Options.Workspaces {
			if w.Path != "" && plan.Has(p.Options.Languages, w.Language) {
				roots = append(roots, w.Path)
			}
		}
		runner.Diagnostic = func(line string) {
			if emit != nil {
				emit(domain.Event{Status: "detail", Text: line})
			}
		}
		x := apply.Executor{Inspect: probe, Build: plan.Build, Runner: runner, Store: store, Files: files.Manager{Roots: roots}, Handlers: apply.ConfigurationHandlers(runner)}
		for k, v := range apply.OptionalHandlers(runner, x.Files) {
			x.Handlers[k] = v
		}
		return x.Execute(ctx, p, emit)
	}
	return s
}
