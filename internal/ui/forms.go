package ui

import (
	"charm.land/huh/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"os"
	"strings"
)

func welcomeForm(resume *bool, available bool, notice string, dark func() bool) *huh.Form {
	title := "Migration first"
	desc := "If reinstalling macOS, restore your files and settings with Migration Assistant before running setup. Existing configuration is preserved unless you review and accept a replacement."
	if goos == "windows" {
		title = "Restore first"
		desc = "If you are moving to a new PC, restore your files from your backup before running setup. Existing configuration is preserved unless you review and accept a replacement."
	}
	if notice != "" {
		desc += "\n\n" + notice
	}
	var fields []huh.Field
	fields = append(fields, huh.NewNote().Title(title).Description(desc))
	if available {
		fields = append(fields, huh.NewConfirm().Title("Restore choices from the last session?").Description("All choices are re-inspected; approval is never restored.").Value(resume))
	}
	return huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) }))
}
func selectionForm(o *domain.Options, dark func() bool) (*huh.Form, []*huh.MultiSelect[string]) {
	choices := func(values []string) []huh.Option[string] {
		r := []huh.Option[string]{}
		for _, v := range values {
			r = append(r, huh.NewOption(v, v))
		}
		return r
	}
	offered := plan.Choices(goos)
	if goos == "windows" {
		apps := huh.NewMultiSelect[string]().Title("Applications").Description("Warp and Zed recommended; installed with Chocolatey.").Options(choices(offered.Apps)...).Value(&o.Apps)
		langs := huh.NewMultiSelect[string]().Title("Optional languages").Options(choices(offered.Languages)...).Value(&o.Languages)
		o.Workspaces = nil
		groups := []*huh.Group{huh.NewGroup(apps), huh.NewGroup(langs), huh.NewGroup(huh.NewConfirm().Title("Configure Git display and editor?").Value(&o.ConfigureGit), huh.NewConfirm().Title("Adopt reviewed files into chezmoi?").Value(&o.AdoptChezmoi))}
		return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) })), []*huh.MultiSelect[string]{apps, langs}
	}
	apps := huh.NewMultiSelect[string]().Title("Applications").Description("Warp, Zed and Applite recommended; Cork uses its official licensed installer.").Options(choices(offered.Apps)...).Value(&o.Apps)
	plugins := huh.NewMultiSelect[string]().Title("Fish plugins").Description("Select each plugin explicitly. Existing custom plugins are preserved.").Options(choices(offered.Plugins)...).Value(&o.Plugins)
	langs := huh.NewMultiSelect[string]().Title("Optional languages").Options(choices(offered.Languages)...).Value(&o.Languages)
	groups := []*huh.Group{huh.NewGroup(apps), huh.NewGroup(plugins), huh.NewGroup(langs), huh.NewGroup(huh.NewConfirm().Title("Configure Git display and editor?").Value(&o.ConfigureGit), huh.NewConfirm().Title("Adopt reviewed files into chezmoi?").Value(&o.AdoptChezmoi), huh.NewConfirm().Title("Capture the complete Homebrew inventory?").Value(&o.CaptureInventory), huh.NewConfirm().Title("Prepare recovery notes and exclusions?").Value(&o.PrepareRecovery))}
	work := make([]domain.Workspace, 3)
	for i, l := range plan.Languages {
		work[i] = domain.Workspace{Language: l, EntryPoint: "main.cr"}
		for _, old := range o.Workspaces {
			if old.Language == l {
				work[i] = old
			}
		}
	}
	o.Workspaces = work
	for i := range o.Workspaces {
		w := &o.Workspaces[i]
		title := "Project/binary name"
		if w.Language == "go" {
			title = "Go module path"
		}
		fields := []huh.Field{huh.NewInput().Title(w.Language + " workspace (optional absolute path)").Description("Leave empty for no workspace. Existing projects are preserved.").Value(&w.Path).Validate(func(s string) error {
			if s != "" && !strings.HasPrefix(s, "/") {
				return fmt.Errorf("enter an absolute path")
			}
			return nil
		}), huh.NewInput().Title(title).Value(&w.Module)}
		if w.Language == "crystal" {
			fields = append(fields, huh.NewInput().Title("Entry point under src/").Value(&w.EntryPoint))
		}
		fields = append(fields, huh.NewConfirm().Title("Create a new project if absent or empty?").Value(&w.Create))
		groups = append(groups, huh.NewGroup(fields...).WithHideFunc(func() bool { return !plan.Has(o.Languages, w.Language) }))
	}

	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) })), []*huh.MultiSelect[string]{apps, plugins, langs}
}
func conflictChanges(s Services, h domain.Host, o domain.Options) []domain.FileChange {
	trial := cloneOptions(o)
	for path, f := range h.Files {
		if f.Exists && !f.Symlink && f.Mode.IsRegular() {
			trial.FileChoices[path] = domain.Replace
		}
	}
	p, e := s.Build(h, trial)
	if e != nil {
		return nil
	}
	var r []domain.FileChange
	for _, step := range p.Steps {
		if step.File != nil && step.File.BeforeExists && o.FileChoices[step.File.Path] != domain.Replace {
			r = append(r, *step.File)
		}
	}
	return r
}
func fileForm(changes []domain.FileChange, h domain.Host, replace []bool, dark func() bool) *huh.Form {
	var groups []*huh.Group
	for i, c := range changes {
		groups = append(groups, huh.NewGroup(huh.NewConfirm().Title("Replace "+c.Path+"?").Description(DiffText(h.Files[c.Path].Contents, c.Desired)).Value(&replace[i])))
	}
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) }))
}
func DiffText(before, after []byte) string {
	safe := func(data []byte) string {
		return strings.Map(func(r rune) rune {
			if r == 127 || r < 32 && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, ansi.Strip(string(data)))
	}
	var b strings.Builder
	b.WriteString("Default: preserve. A replacement receives a private backup.\n--- current\n+++ proposed\n")
	for _, line := range strings.Split(safe(before), "\n") {
		b.WriteString("-" + line + "\n")
	}
	for _, line := range strings.Split(safe(after), "\n") {
		b.WriteString("+" + line + "\n")
	}
	return b.String()
}

func commandEnvironment(extra []string) []string { return append(os.Environ(), extra...) }
