package plan

import (
	"bytes"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/testutil"
	"path/filepath"
	"strings"
	"testing"
)

func TestLanguageSelectionIsIndependent(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	o := DefaultOptions()
	o.Languages = []string{"nim"}
	p, e := Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range p.Steps {
		if s.Package != nil && (s.Package.Token == "go" || s.Package.Token == "crystal") {
			t.Fatal("unselected language installed")
		}
	}
}
func TestWorkspaceRequiresRealPathAndName(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	for _, w := range []domain.Workspace{{Language: "go", Create: true}, {Language: "go", Path: h.Home + "/project", Create: true}, {Language: "crystal", Path: h.Home + "/p", Module: "okay", EntryPoint: "x;rm.cr", Create: true}} {
		if _, e := WorkspaceChanges(h, w); e == nil {
			t.Fatal("invalid workspace accepted", w)
		}
	}
}
func TestWorkspacePreservesExistingProject(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	path := h.Home + "/project"
	h.Workspaces = map[string]domain.WorkspaceState{path: {Exists: true, Directory: true}}
	original := []byte("// custom\n{}")
	zed := filepath.Join(path, ".zed/settings.json")
	h.Files[zed] = domain.FileState{Path: zed, Exists: true, Mode: 0600, Contents: original}
	c, e := WorkspaceChanges(h, domain.Workspace{Language: "go", Path: path, Module: "github.com/evelyn/project", Create: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range c {
		if v.Path == zed || !strings.Contains(v.Path, "/.zed/") {
			t.Fatal("existing project overwritten", v.Path)
		}
	}
	if !bytes.Equal(h.Files[zed].Contents, original) {
		t.Fatal("original changed")
	}
}
func TestWorkspacePathsAreArguments(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	o := DefaultOptions()
	o.Languages = []string{"go"}
	path := h.Home + "/Projects/日本語 space"
	o.Workspaces = []domain.Workspace{{Language: "go", Path: path, Module: "github.com/evelyn/app", Create: true}}
	p, e := Build(h, o)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, s := range p.Steps {
		if s.Kind == "workspace-init" {
			found = true
			if s.Command == nil || s.Command.Dir != path || strings.Join(s.Command.Args, "|") != "mod|init|github.com/evelyn/app" {
				t.Fatal("path/arguments changed", s.Command)
			}
		}
	}
	if !found {
		t.Fatal("module init missing")
	}
}
func TestNimBinaryAndFormatter(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	c, e := WorkspaceChanges(h, domain.Workspace{Language: "nim", Path: h.Home + "/nim", Module: "actual_app", Create: true})
	if e != nil {
		t.Fatal(e)
	}
	manifest, formatter := false, false
	for _, f := range c {
		if strings.HasSuffix(f.Path, "actual_app.nimble") {
			manifest = bytes.Contains(f.Desired, []byte(`bin = @["actual_app"]`))
		}
		if strings.HasSuffix(f.Path, "settings.json") {
			formatter = bytes.Contains(f.Desired, []byte("--stdin"))
		}
	}
	if !manifest || !formatter {
		t.Fatal("binary manifest or stdin formatter absent")
	}
}
