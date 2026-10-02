package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"strings"
	"testing"
)

func services(t *testing.T) Services {
	h := testutil.FreshHost(t.TempDir())
	return Services{Inspect: func(context.Context, domain.Options) (domain.Host, error) { return h, nil }, Build: plan.Build}
}
func TestSelectionReachesPlan(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.draft.Apps = []string{"zed"}
	m.commitDraft()
	if !plan.Has(m.options.Apps, "zed") || plan.Has(m.options.Apps, "warp") {
		t.Fatal("selection lost")
	}
}
func TestCanceledDraftPreservesSelection(t *testing.T) {
	m := newModel(context.Background(), services(t))
	before := strings.Join(m.options.Apps, ",")
	m.draft.Apps = nil
	m.cancelDraft()
	if strings.Join(m.options.Apps, ",") != before {
		t.Fatal("canceled draft changed choices")
	}
}
func TestQIsTextWhileEditing(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 80, 24
	m.edit()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.stage == "done" || m.canceled {
		t.Fatal("q ended editing")
	}
	_ = cmd
}
func TestLayoutBounds(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, size := range [][2]int{{120, 40}, {80, 24}, {48, 18}, {40, 12}, {20, 6}, {1, 1}, {0, 0}} {
			m := newModel(context.Background(), services(t))
			m.width, m.height = size[0], size[1]
			m.dark = dark
			m.stage = "preview"
			m.lines = strings.Repeat("日本語 wide labels and long validation content\n", 100)
			v := m.View().Content
			if size[0] == 0 || size[1] == 0 {
				if v != "" {
					t.Fatal("zero-sized view must be empty")
				}
				continue
			}
			if lipgloss.Width(v) > size[0] || lipgloss.Height(v) > size[1] {
				t.Fatalf("%v exceeds bounds: %dx%d", size, lipgloss.Width(v), lipgloss.Height(v))
			}
		}
	}
}
func TestHiddenInputPaused(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 40, 12
	m.edit()
	before := strings.Join(m.draft.Apps, ",")
	m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m.Update(tea.PasteMsg{Content: "malicious hidden input"})
	if strings.Join(m.draft.Apps, ",") != before {
		t.Fatal("hidden input changed selection")
	}
}
func TestResumeRequiresNewFileReview(t *testing.T) {
	o := plan.DefaultOptions()
	o.FileChoices["/home/settings"] = domain.Replace
	o.RecoveryDate = "2000-01-01"
	r := restoredOptions(o)
	if r.FileChoices["/home/settings"] == domain.Replace {
		t.Fatal("old replacement decision restored")
	}
	if r.RecoveryDate != plan.DefaultOptions().RecoveryDate {
		t.Fatal("recovery records would carry the old session's date", r.RecoveryDate)
	}
	if len(r.Apps) != len(o.Apps) {
		t.Fatal("ordinary choices lost")
	}
}
func TestFinishedReportScrolls(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 80, 24
	m.stage = "done"
	m.lines = ""
	for i := 0; i < 60; i++ {
		m.lines += fmt.Sprintf("row %d\n", i)
	}
	m.lines += "final recovery action"
	before := m.View().Content
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.View().Content == before {
		t.Fatal("summary did not scroll")
	}
}
func TestEscapeExitsSelectionsIncludingSmallScreen(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 12}} {
		m := newModel(context.Background(), services(t))
		m.width, m.height = size[0], size[1]
		m.edit()
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.canceled || cmd == nil {
			t.Fatal("Escape did not request exit", size)
		}
	}
}
func TestCardAndSummaryStartAtTop(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 120, 40
	m.stage = "done"
	m.lines = "finished"
	if !strings.Contains(m.View().Content, "╭") {
		t.Fatal("missing rounded card")
	}
	m.scroll = 100
	m.Update(applyDone{report: domain.Report{Status: "complete"}})
	if m.scroll != 0 {
		t.Fatal("summary inherited preview scroll")
	}
}
func TestFormLayoutBounds(t *testing.T) {
	for _, dark := range []bool{true, false} {
		for _, size := range [][2]int{{120, 40}, {80, 24}, {48, 18}, {40, 12}, {20, 6}, {1, 1}, {0, 0}} {
			m := newModel(context.Background(), services(t))
			m.dark = dark
			m.width, m.height = size[0], size[1]
			m.edit()
			view := m.View().Content
			if size[0] == 0 {
				if view != "" {
					t.Fatal("zero form is not empty")
				}
				continue
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatal("form escaped terminal bounds", size)
			}
		}
	}
}
func TestDiffDoesNotRenderTerminalControls(t *testing.T) {
	text := DiffText([]byte("# \x1b]52;c;payload\x07\n\x1b[31mtext"), []byte("safe\n"))
	if strings.ContainsAny(text, "\x1b\x07") {
		t.Fatal("file content can control terminal")
	}
}
func TestSavedSessionErrorDoesNotBlockWizard(t *testing.T) {
	s := services(t)
	s.LoadLatest = func() (domain.Session, error) { return domain.Session{}, errors.New("invalid or incompatible session") }
	m := newModel(context.Background(), s)
	m.width, m.height = 80, 24
	var resume tea.Msg
	for _, cmd := range m.Init()().(tea.BatchMsg) {
		if msg, ok := cmd().(resumeMsg); ok {
			resume = msg
		}
	}
	if resume == nil {
		t.Fatal("saved-session error ended the wizard")
	}
	m.Update(resume)
	if m.stage != "welcome" || m.form == nil || m.err != nil || !strings.Contains(m.View().Content, "not restored") {
		t.Fatal("saved-session problem not shown on the welcome page", m.stage, m.err)
	}
}
func TestInvalidWorkspaceReturnsToChoices(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 80, 24
	m.options.Languages = []string{"nim"}
	m.options.Workspaces = []domain.Workspace{{Language: "nim", Path: "/Users/me/proj/", Module: "app", Create: true}}
	m.Update(m.inspectCmd()())
	if m.stage != "select" || m.form == nil || m.err != nil {
		t.Fatal("validation error ended the wizard", m.stage, m.err)
	}
	if !plan.Has(m.draft.Languages, "nim") || m.draft.Workspaces[2].Path != "/Users/me/proj/" {
		t.Fatal("choices lost", m.draft)
	}
	if !strings.Contains(m.View().Content, "Needs attention") {
		t.Fatal("validation error not shown")
	}
	for _, size := range [][2]int{{80, 24}, {48, 18}} {
		m.width, m.height = size[0], size[1]
		m.resizeForm()
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] || !strings.Contains(view, "Applications") {
			t.Fatal("notice pushed the form out of the terminal", size)
		}
	}
}
func TestEditAsksAboutReplacementsAgain(t *testing.T) {
	s := services(t)
	h, _ := s.Inspect(context.Background(), domain.Options{})
	path := h.Home + "/.config/fish/config.fish"
	h.Files[path] = domain.FileState{Path: path, Exists: true, Mode: 0644, Contents: []byte("# changed after approval\n"), SHA256: "changed"}
	s.Inspect = func(context.Context, domain.Options) (domain.Host, error) { return h, nil }
	m := newModel(context.Background(), s)
	m.width, m.height = 80, 24
	m.options.FileChoices[path] = domain.Replace
	m.Update(m.inspectCmd()())
	if m.stage != "files" || len(m.conflicts) == 0 || m.conflicts[0].Path != path {
		t.Fatal("earlier approval reused without showing the current diff", m.stage)
	}
	for m.stage == "files" {
		m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	}
	for _, step := range m.preview.Steps {
		if step.File != nil && step.File.Path == path {
			t.Fatal("declined replacement still planned")
		}
	}
}
func TestApplyingViewFollowsOutput(t *testing.T) {
	m := newModel(context.Background(), services(t))
	m.width, m.height = 80, 24
	m.stage, m.follow, m.lines = "applying", true, "Applying the reviewed plan…\n"
	for i := 0; i < 60; i++ {
		m.Update(domain.Event{Status: "detail", Text: fmt.Sprintf("row %d", i)})
	}
	if !strings.Contains(m.View().Content, "row 59") {
		t.Fatal("latest progress is off screen")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m.Update(domain.Event{Status: "detail", Text: "row 60"})
	if strings.Contains(m.View().Content, "row 60") || !strings.Contains(m.View().Content, "row 58") {
		t.Fatal("scrolling up did not hold the view")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m.Update(domain.Event{Status: "detail", Text: "row 61"})
	if !strings.Contains(m.View().Content, "row 61") {
		t.Fatal("view did not follow again at the end")
	}
}
