//go:build !windows

package integration

import (
	"context"
	"errors"
	"golden-gate-setup/internal/apply"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil/sandbox"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executor(s *sandbox.Sandbox) *apply.Executor {
	m := files.Manager{Roots: []string{s.Home}}
	handlers := apply.ConfigurationHandlers(s)
	for k, v := range apply.OptionalHandlers(s, m) {
		handlers[k] = v
	}
	return &apply.Executor{Inspect: s.Inspect, Build: plan.Build, Runner: s, Files: m, Store: apply.SessionStore{Dir: s.Home + "/sessions"}, Handlers: handlers}
}
func preview(t *testing.T, s *sandbox.Sandbox, o domain.Options) domain.Plan {
	t.Helper()
	h, e := s.Inspect(context.Background(), o)
	if e != nil {
		t.Fatal(e)
	}
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	p.Accepted = true
	return p
}
func TestFreshSetupRepeatAndResume(t *testing.T) {
	s := sandbox.NewSandbox(t.TempDir())
	x := executor(s)
	o := plan.DefaultOptions()
	o.CaptureInventory = false
	o.PrepareRecovery = false
	ctx, cancel := context.WithCancel(context.Background())
	p := preview(t, s, o)
	_, e := x.Execute(ctx, p, func(v domain.Event) {
		if v.Status == "verified" {
			cancel()
		}
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatal("interruption lost", e)
	}
	saved, e := x.Store.Latest()
	if e != nil || saved.Plan.Accepted {
		t.Fatal("resume approval restored", e)
	}
	p = preview(t, s, saved.Plan.Options)
	r, e := x.Execute(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e, r)
	}
	before := map[string][]byte{}
	for _, path := range s.ConfigPaths() {
		b, _ := os.ReadFile(path)
		before[path] = b
	}
	s.ResetCalls()
	p = preview(t, s, o)
	r, e = x.Execute(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range s.Calls() {
		if len(c.Args) > 0 && (c.Args[0] == "install" || c.Args[0] == "add" || c.Args[0] == "init") {
			t.Fatal("repeat mutation", c)
		}
	}
	for path, b := range before {
		got, _ := os.ReadFile(path)
		if string(got) != string(b) {
			t.Fatal("repeat changed", path)
		}
	}
	if len(r.ManualTasks) == 0 {
		t.Fatal("manual steps disappeared")
	}
}
func TestMigratedSetupPreserved(t *testing.T) {
	s := sandbox.NewSandbox(t.TempDir())
	s.Migrated()
	original, _ := os.ReadFile(s.Home + "/.config/zed/settings.json")
	p := preview(t, s, plan.DefaultOptions())
	r, e := executor(s).Execute(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e, r)
	}
	got, _ := os.ReadFile(s.Home + "/.config/zed/settings.json")
	if string(got) != string(original) {
		t.Fatal("migrated JSONC replaced")
	}
	for _, c := range s.Calls() {
		if len(c.Args) >= 3 && c.Args[0] == "install" && c.Args[2] == "zed" {
			t.Fatal("vendor Zed adopted")
		}
	}
	if _, e = os.Stat(s.Home + "/.config/fish/functions/custom.fish"); e != nil {
		t.Fatal("custom Fish lost")
	}
}
func TestOnlySelectedWorkspaces(t *testing.T) {
	s := sandbox.NewSandbox(t.TempDir())
	o := plan.DefaultOptions()
	o.Languages = []string{"go"}
	o.Workspaces = []domain.Workspace{{Language: "go", Path: s.Home + "/Projects/space 日本語", Module: "github.com/evelyn/actual_app", Create: true}, {Language: "nim", Path: s.Home + "/unselected", Module: "no_app", Create: true}}
	p := preview(t, s, o)
	r, e := executor(s).Execute(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e, r)
	}
	if _, e = os.Stat(o.Workspaces[0].Path + "/main.go"); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(o.Workspaces[1].Path); !os.IsNotExist(e) {
		t.Fatal("unselected workspace created")
	}
	b, _ := os.ReadFile(filepath.Join(o.Workspaces[0].Path, "go.mod"))
	if !strings.Contains(string(b), o.Workspaces[0].Module) {
		t.Fatal("wrong module")
	}
}
func TestFinishReportsManualWork(t *testing.T) {
	s := sandbox.NewSandbox(t.TempDir())
	r, e := executor(s).Execute(context.Background(), preview(t, s, plan.DefaultOptions()), nil)
	if e != nil {
		t.Fatal(e)
	}
	found := map[string]bool{}
	for _, task := range r.ManualTasks {
		found[task.ID] = true
	}
	for _, id := range []string{"time-machine", "applite-export", "applite-shared-brew", "dotfiles-remote"} {
		if !found[id] {
			t.Fatal("missing manual task", id)
		}
	}
	if r.Status != "complete" {
		t.Fatal("installation incomplete", r)
	}
	if _, e = os.Stat(filepath.Join(s.Home, "Golden Gate Recovery")); e != nil {
		t.Fatal("recovery records absent")
	}
}
