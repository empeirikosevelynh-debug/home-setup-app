package apply

import (
	"context"
	"errors"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/inspect"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type runFn func(context.Context, domain.Command, io.Writer) error

func (f runFn) Run(c context.Context, d domain.Command, w io.Writer) error { return f(c, d, w) }
func setup(t *testing.T) (*Executor, *domain.Host, domain.Plan, *int) {
	t.Helper()
	h := testutil.FreshHost(t.TempDir())
	o := plan.DefaultOptions()
	o.Apps = nil
	o.Plugins = nil
	o.ConfigureGit = false
	o.AdoptChezmoi = false
	o.CaptureInventory = false
	o.PrepareRecovery = false
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	p.Accepted = true
	calls := new(int)
	x := &Executor{Inspect: func(context.Context, domain.Options) (domain.Host, error) { return h, nil }, Build: plan.Build, Files: files.Manager{Roots: []string{h.Home}}, Store: SessionStore{Dir: h.Home + "/sessions"}}
	x.Runner = runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		if len(c.Args) == 0 || c.Args[0] != "install" {
			return nil
		}
		*calls++
		h.Packages["formula:"+c.Args[len(c.Args)-1]] = domain.InstalledPackage{Version: "99.0"}
		return nil
	})
	return x, &h, p, calls
}
func TestFailureStopsDependents(t *testing.T) {
	x, _, p, c := setup(t)
	x.Runner = runFn(func(context.Context, domain.Command, io.Writer) error { *c++; return errors.New("failed") })
	r, e := x.Execute(context.Background(), p, nil)
	if e == nil || *c != 1 || r.Status != "failed" {
		t.Fatal(r, e, *c)
	}
}
func TestInstallRechecksRegistration(t *testing.T) {
	x, h, p, c := setup(t)
	reads := 0
	x.Inspect = func(context.Context, domain.Options) (domain.Host, error) {
		reads++
		if reads > 1 {
			h.Packages["formula:"+plan.Core[1]] = domain.InstalledPackage{Version: "99.0"}
		}
		return *h, nil
	}
	_, e := x.Execute(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e)
	}
	if *c != len(plan.Core)-1 {
		t.Fatal("registered package was installed again", *c)
	}
}
func TestCancellationSchedulesNoMoreWork(t *testing.T) {
	x, _, p, c := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	x.Runner = runFn(func(context.Context, domain.Command, io.Writer) error { *c++; cancel(); return nil })
	r, e := x.Execute(ctx, p, nil)
	if !errors.Is(e, context.Canceled) || *c != 1 || r.Status != "interrupted" {
		t.Fatal(r, e, *c)
	}
	if len(r.Steps) == 0 || r.Steps[0].Status == "verified" {
		t.Fatal("canceled action verified")
	}
}
func TestStaleCheckpointReverified(t *testing.T) {
	x, _, p, c := setup(t)
	x.Store.Save(domain.Session{SchemaVersion: 1, Plan: p, Results: []domain.StepResult{{ID: p.Steps[0].ID, Status: "verified"}}})
	_, e := x.Execute(context.Background(), p, nil)
	if e != nil || *c != len(plan.Core) {
		t.Fatal("stale record trusted", *c, e)
	}
}
func TestUnacceptedPlanRejected(t *testing.T) {
	x, _, p, c := setup(t)
	p.Accepted = false
	_, e := x.Execute(context.Background(), p, nil)
	if e == nil || *c != 0 {
		t.Fatal("unaccepted execution", e, *c)
	}
}
func TestPlanStateChangedBeforeApply(t *testing.T) {
	x, h, p, c := setup(t)
	h.Packages["formula:fish"] = domain.InstalledPackage{Version: "99.0"}
	_, e := x.Execute(context.Background(), p, nil)
	if e == nil || *c != 0 {
		t.Fatal("stale approval accepted")
	}
}

var _ command.Runner = runFn(nil)

func TestCancelBeforeFileDispatch(t *testing.T) {
	x, h, _, _ := setup(t)
	for _, name := range plan.Core {
		h.Packages["formula:"+name] = domain.InstalledPackage{Version: "99.0"}
	}
	o := plan.DefaultOptions()
	o.Apps = nil
	o.Plugins = nil
	o.ConfigureGit = false
	o.AdoptChezmoi = false
	o.CaptureInventory = false
	o.PrepareRecovery = false
	p, _ := plan.Build(*h, o)
	p.Accepted = true
	ctx, cancel := context.WithCancel(context.Background())
	_, e := x.Execute(ctx, p, func(v domain.Event) {
		if v.Status == "running" {
			cancel()
		}
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Stat(p.Steps[0].File.Path); !os.IsNotExist(e) {
		t.Fatal("file was dispatched after cancel")
	}
}
func TestRepeatRunInspectsOnce(t *testing.T) {
	x, h, p, c := setup(t)
	if _, e := x.Execute(context.Background(), p, nil); e != nil {
		t.Fatal(e)
	}
	*c = 0
	reads := 0
	x.Inspect = func(context.Context, domain.Options) (domain.Host, error) { reads++; return *h, nil }
	p, _ = plan.Build(*h, p.Options)
	p.Accepted = true
	if _, e := x.Execute(context.Background(), p, nil); e != nil {
		t.Fatal(e)
	}
	if *c != 0 || reads != 1 {
		t.Fatal("satisfied steps inspected the machine again", reads, *c)
	}
}
func TestUnchangedSymlinkedGitConfigIsPreserved(t *testing.T) {
	git, e := exec.LookPath("git")
	if e != nil {
		t.Skip("Git unavailable")
	}
	h := testutil.FreshHost(t.TempDir())
	for _, name := range plan.Core {
		h.Packages["formula:"+name] = domain.InstalledPackage{Version: "99.0"}
	}
	h.Tools["git"] = git
	target := filepath.Join(h.Home, "dotfiles/gitconfig")
	os.MkdirAll(filepath.Dir(target), 0700)
	os.WriteFile(target, []byte("[user]\n name = Example\n"), 0600)
	os.Symlink(target, filepath.Join(h.Home, ".gitconfig"))
	inspectHost := func(context.Context, domain.Options) (domain.Host, error) {
		for _, path := range []string{filepath.Join(h.Home, ".gitconfig"), filepath.Join(h.Home, ".config/git/config")} {
			s, e := inspect.ReadFileState(h.Home, path)
			if e != nil {
				return h, e
			}
			h.Files[path] = s
		}
		return h, nil
	}
	o := plan.DefaultOptions()
	o.Apps, o.Plugins, o.AdoptChezmoi, o.CaptureInventory, o.PrepareRecovery = nil, nil, false, false, false
	current, _ := inspectHost(context.Background(), o)
	p, e := plan.Build(current, o)
	if e != nil {
		t.Fatal(e)
	}
	p.Accepted = true
	r := runFn(func(ctx context.Context, c domain.Command, w io.Writer) error {
		if c.Path == git {
			return command.ProcessRunner{}.Run(ctx, c, w)
		}
		return nil
	})
	x := &Executor{Inspect: inspectHost, Build: plan.Build, Runner: r, Files: files.Manager{Roots: []string{h.Home}}, Store: SessionStore{Dir: h.Home + "/sessions"}, Handlers: ConfigurationHandlers(r)}
	report, e := x.Execute(context.Background(), p, nil)
	if e != nil || report.Status != "complete" {
		t.Fatal("symlinked Git configuration stopped the apply", e)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "[user]\n name = Example\n" {
		t.Fatal("symlink target was changed")
	}
	found := false
	for _, task := range report.ManualTasks {
		found = found || task.ID == "deferred:git"
	}
	if !found {
		t.Fatal("symlinked Git configuration not left for review", report.ManualTasks)
	}
}
func TestVendorAppDuringApplyReportedOnce(t *testing.T) {
	x, h, _, _ := setup(t)
	o := plan.DefaultOptions()
	o.Apps = []string{"zed"}
	o.Plugins, o.ConfigureGit, o.AdoptChezmoi, o.CaptureInventory, o.PrepareRecovery = nil, false, false, false, false
	p, _ := plan.Build(*h, o)
	p.Accepted = true
	reads := 0
	x.Inspect = func(context.Context, domain.Options) (domain.Host, error) {
		reads++
		current := *h
		if reads > 1 {
			current.Apps = []domain.AppBundle{{Name: "Zed", Path: "/Applications/Zed.app"}}
		}
		return current, nil
	}
	r, e := x.Execute(context.Background(), p, nil)
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, task := range r.ManualTasks {
		if strings.Contains(task.Instructions, "appeared outside Homebrew") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("vendor app reported", count, "times", r.ManualTasks)
	}
}
