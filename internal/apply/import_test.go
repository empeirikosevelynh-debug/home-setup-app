//go:build !windows

package apply

import (
	"context"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func previousHome(t *testing.T, files map[string]string) string {
	t.Helper()
	old := t.TempDir()
	for path, contents := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(old, path)), 0700)
		if err := os.WriteFile(filepath.Join(old, path), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return old
}

func TestImportStep(t *testing.T) {
	ctx := context.Background()
	h := testutil.FreshHost(t.TempDir())
	old := previousHome(t, map[string]string{"Documents/a.txt": "theirs", "Documents/b.txt": "new"})
	os.MkdirAll(filepath.Join(h.Home, "Documents"), 0700)
	os.WriteFile(filepath.Join(h.Home, "Documents/a.txt"), []byte("mine, longer"), 0600)
	o := plan.DefaultOptions()
	o.RecoveryDate, o.ImportFrom = "2026-10-02", old
	job := plan.ImportJob(h, o, "Documents")
	step := domain.Step{ID: "import:Documents", Kind: "import", Import: &job}
	handler := OptionalHandlers(runFn(nil), files.Manager{Roots: []string{h.Home}})["import"]
	if ok, err := handler.Verify(ctx, h, domain.Plan{}, step, ""); ok || err != nil {
		t.Fatal("nothing imported yet", ok, err)
	}
	result, err := handler.Apply(ctx, h, domain.Plan{}, step, "")
	if err != nil || result.Message != "Copied 1 new file (3 bytes) · 1 differing file saved in "+job.Conflicts {
		t.Fatal(result.Message, err)
	}
	if ok, err := handler.Verify(ctx, h, domain.Plan{}, step, ""); !ok || err != nil {
		t.Fatal("import not verified", ok, err)
	}
	if _, err := handler.Apply(ctx, h, domain.Plan{}, domain.Step{ID: "import:x", Kind: "import"}, ""); err == nil {
		t.Fatal("import step without a folder accepted")
	}
}

func TestExecuteImportsWithoutRescanning(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	old := previousHome(t, map[string]string{"Documents/a.txt": "a"})
	o := plan.DefaultOptions()
	o.Apps, o.Plugins, o.ConfigureGit, o.AdoptChezmoi, o.CaptureInventory, o.PrepareRecovery = nil, nil, false, false, false, false
	o.RecoveryDate, o.ImportFrom, o.ImportFolders = "2026-10-02", old, []string{"Documents"}
	var scans []int
	inspectHost := func(_ context.Context, o domain.Options) (domain.Host, error) {
		current := h
		scans = append(scans, len(o.ImportFolders))
		for _, folder := range o.ImportFolders {
			scan, err := files.ScanImport(plan.ImportJob(current, o, folder), nil)
			scan.Folder = folder
			current.Import = append(current.Import, scan)
			if err != nil {
				return current, err
			}
		}
		return current, nil
	}
	first, _ := inspectHost(context.Background(), o)
	p, err := plan.Build(first, o)
	if err != nil || p.Later != "" {
		t.Fatal(err, p.Later)
	}
	p.Accepted = true
	scans = nil
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		if len(c.Args) > 0 && c.Args[0] == "install" {
			h.Packages["formula:"+c.Args[len(c.Args)-1]] = domain.InstalledPackage{Version: "99.0"}
		}
		return nil
	})
	m := files.Manager{Roots: []string{h.Home}}
	x := &Executor{Inspect: inspectHost, Build: plan.Build, Runner: r, Files: m, Store: SessionStore{Dir: h.Home + "/sessions"}, Handlers: ConfigurationHandlers(r)}
	for k, v := range OptionalHandlers(r, m) {
		x.Handlers[k] = v
	}
	report, err := x.Execute(context.Background(), p, nil)
	if err != nil || report.Status != "complete" {
		t.Fatal(err, report)
	}
	if data, err := os.ReadFile(filepath.Join(h.Home, "Documents/a.txt")); err != nil || string(data) != "a" {
		t.Fatal("not imported", err)
	}
	if len(scans) < 3 || scans[0] != 1 {
		t.Fatal("unexpected inspections", scans)
	}
	for _, n := range scans[1:] {
		if n != 0 {
			t.Fatal("folders scanned again while applying", scans)
		}
	}
}
