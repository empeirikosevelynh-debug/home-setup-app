package apply

import (
	"bytes"
	"context"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/templates"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsInstallsWithChocolatey(t *testing.T) {
	h := testutil.FreshWindowsHost(t.TempDir())
	o := plan.DefaultOptionsFor("windows")
	o.ConfigureGit, o.AdoptChezmoi = false, false
	p, e := plan.Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	p.Accepted = true
	var installs []domain.Command
	x := &Executor{Inspect: func(context.Context, domain.Options) (domain.Host, error) { return h, nil }, Build: plan.Build, Files: files.Manager{Roots: []string{h.Home}}, Store: SessionStore{Dir: filepath.Join(h.Home, "sessions")}}
	x.Runner = runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		if len(c.Args) > 0 && c.Args[0] == "install" {
			installs = append(installs, c)
			h.Packages["choco:"+c.Args[1]] = domain.InstalledPackage{Version: "1.0"}
		}
		return nil
	})
	r, e := x.Execute(context.Background(), p, nil)
	if e != nil || r.Status != "complete" {
		t.Fatal(r, e)
	}
	if len(installs) != len(plan.WindowsCore)+2 {
		t.Fatal("unexpected installs", len(installs))
	}
	for _, c := range installs {
		if c.Path != h.ChocoPath || len(c.Args) != 4 || c.Args[2] != "--yes" || c.Args[3] != "--no-progress" || c.Interactive || len(c.Env) != 0 {
			t.Fatal("unreviewed Chocolatey command", c)
		}
	}
	want, _ := templates.Load("zed/settings.windows.json")
	if got, _ := os.ReadFile(plan.ZedSettingsPath(h)); !bytes.Equal(got, want) {
		t.Fatal("Windows Zed settings not written")
	}
}
