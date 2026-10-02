package ui

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func oldHomeFolder(t *testing.T) string {
	t.Helper()
	old := filepath.Join(t.TempDir(), "you")
	for _, dir := range []string{"Desktop", "Documents", "Applications", "Library", ".ssh", ".cache", ".Trash", "Imported conflicts"} {
		os.MkdirAll(filepath.Join(old, dir), 0700)
	}
	os.WriteFile(filepath.Join(old, ".zshrc"), []byte("zsh"), 0600)
	os.Symlink(filepath.Join(old, "Documents"), filepath.Join(old, "Linked"))
	return old
}

func TestImportFolders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("previous home folders are macOS-only")
	}
	old := oldHomeFolder(t)
	choices, err := importFolders(old)
	if err != nil {
		t.Fatal(err)
	}
	want := []folderChoice{{".", "Files at the top of the home folder, such as .zshrc", false}, {"Applications", "Applications", false}, {"Desktop", "Desktop", true}, {"Documents", "Documents", true}, {".cache", ".cache (cache)", false}, {".ssh", ".ssh (keys)", false}}
	if !reflect.DeepEqual(choices, want) {
		t.Fatalf("%+v", choices)
	}
	for _, bad := range []string{"relative/you", filepath.Join(old, ".zshrc"), filepath.Join(old, "missing")} {
		if _, err := importFolders(bad); err == nil {
			t.Fatal("unusable previous home folder accepted:", bad)
		}
	}
	defer func(old func() (string, error)) { homeDir = old }(homeDir)
	homeDir = func() (string, error) { return old, nil }
	for _, bad := range []string{old, filepath.Join(old, "Documents"), filepath.Dir(old)} {
		if err := importSource(bad); err == nil {
			t.Fatal("this home folder offered as the previous one:", bad)
		}
	}
}

func TestChooseFolders(t *testing.T) {
	choices := []folderChoice{{name: "."}, {name: "Documents"}, {name: ".ssh"}}
	if got, err := chooseFolders(" Documents ,.ssh,Documents", choices); err != nil || !reflect.DeepEqual(got, []string{"Documents", ".ssh"}) {
		t.Fatal(got, err)
	}
	if got, err := chooseFolders("none", choices); got != nil || err != nil {
		t.Fatal(got, err)
	}
	if _, err := chooseFolders("Music", choices); err == nil {
		t.Fatal("folder outside the list accepted")
	}
}

func TestPlainImport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("previous home folders are macOS-only")
	}
	old := oldHomeFolder(t)
	s := services(t)
	var got domain.Options
	s.Inspect = func(_ context.Context, o domain.Options) (domain.Host, error) {
		got = o
		h, e := services(t).Inspect(context.Background(), o)
		for _, folder := range o.ImportFolders {
			h.Import = append(h.Import, domain.ImportScan{Folder: folder})
		}
		return h, e
	}
	var out bytes.Buffer
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nnone\n"+old+"\n.ssh,Documents\nnone\nno\n"), &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	if got.ImportFrom != old || !reflect.DeepEqual(got.ImportFolders, []string{".ssh", "Documents"}) {
		t.Fatalf("import answers lost: %q %q", got.ImportFrom, got.ImportFolders)
	}
	if text := out.String(); !strings.Contains(text, "It holds: . (the files at the top, such as .zshrc), Applications, Desktop, Documents, .cache (cache), .ssh (keys)") || !strings.Contains(text, "[Desktop,Documents]") {
		t.Fatal("folders not offered as expected:", text)
	}
}

func TestReviewTheRestAfterImport(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 80, 24
	m.preview = domain.Plan{Later: "The rest of setup is planned next."}
	m.Update(applyDone{report: domain.Report{Status: "complete"}})
	if !strings.Contains(m.lines, "Press r to review it now") || !strings.Contains(m.View().Content, "r review the rest") {
		t.Fatal("second review not offered:", m.lines)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}); m.stage != "inspecting" || cmd == nil {
		t.Fatal("r did not start the second review", m.stage)
	}
	// A failed apply offers no second review.
	m = newModel(context.Background(), services(t))
	m.width, m.height = 80, 24
	m.preview = domain.Plan{Later: "The rest of setup is planned next."}
	m.Update(applyDone{report: domain.Report{Status: "failed"}})
	if m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}); m.stage != "done" {
		t.Fatal("second review offered after a failure")
	}
}

func TestPlainReviewsTheRest(t *testing.T) {
	s := services(t)
	inspections := 0
	s.Inspect = func(_ context.Context, o domain.Options) (domain.Host, error) {
		inspections++
		return services(t).Inspect(context.Background(), o)
	}
	s.Build = func(h domain.Host, o domain.Options) (domain.Plan, error) {
		p := domain.Plan{ID: "plan", Supported: true}
		if inspections == 1 {
			p.Later = "The rest of setup is planned next."
		}
		return p, nil
	}
	s.Apply = func(context.Context, domain.Plan, func(domain.Event)) (domain.Report, error) {
		return domain.Report{Status: "complete"}, nil
	}
	var out bytes.Buffer
	r, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nnone\nnone\nnone\nyes\n\nyes\n"), &out)
	if err != nil || r.Status != "complete" || inspections != 2 || !strings.Contains(out.String(), "Review the rest of setup now?") {
		t.Fatal(err, r.Status, inspections, out.String())
	}
}

func TestClearedSourcesDropTheirChoices(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.draft.ImportFolders, m.draft.PreviousPackages = []string{"Documents"}, []string{"formula:jq"}
	m.commitDraft()
	if m.options.ImportFolders != nil || m.options.PreviousPackages != nil {
		t.Fatal("choices kept without their source", m.options.ImportFolders, m.options.PreviousPackages)
	}
}

func TestDotfileReviewShowsTheRepository(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	zshrc, secret := filepath.Join(h.Home, ".zshrc"), filepath.Join(h.Home, ".secret")
	h.DotfilesState = "cloned"
	h.Dotfiles = []domain.Dotfile{{Target: zshrc, Kind: "file", SHA256: "repo", Contents: []byte("export EDITOR=zed\n")}, {Target: secret, Kind: "encrypted", Interactive: true}}
	for _, path := range []string{zshrc, secret} {
		h.Files[path] = domain.FileState{Path: path, Exists: true, Mode: 0644, SHA256: "mine", Contents: []byte("export EDITOR=vi\n")}
	}
	o := plan.DefaultOptions()
	o.DotfilesRepo = "you"
	changes := map[string]domain.FileChange{}
	for _, c := range conflictChanges(Services{Build: plan.Build}, h, o) {
		changes[c.Path] = c
	}
	if len(changes) != 2 {
		t.Fatal("dotfiles not offered for review", changes)
	}
	if text := ChangeText(h, changes[zshrc]); !strings.Contains(text, "-export EDITOR=vi") || !strings.Contains(text, "+export EDITOR=zed") {
		t.Fatal(text)
	}
	if text := ChangeText(h, changes[secret]); !strings.Contains(text, "is encrypted, so it can't be compared before applying") {
		t.Fatal(text)
	}
}
