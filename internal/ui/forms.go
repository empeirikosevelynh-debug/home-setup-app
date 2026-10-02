package ui

import (
	"charm.land/huh/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"os"
	"path/filepath"
	"strings"
)

func welcomeForm(resume *bool, available bool, notice string, dark func() bool) *huh.Form {
	title := "Migration first"
	desc := "If reinstalling macOS, restore your files and settings with Migration Assistant before running setup. To bring over only your files and apps, choose your previous home folder later in setup instead. Existing configuration is preserved unless you review and accept a replacement."
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
	dotfiles := huh.NewInput().Title("Dotfiles repository (optional)").Description("Your chezmoi dotfiles: a GitHub user, user/repo, a Git URL, or your previous Mac's chezmoi source. Setup clones it, you review each file it would change, then chezmoi applies the ones you approve. Its scripts are not run. Leave empty to skip.").SuggestionsFunc(func() []string {
		if repo := previousRepo(o.ImportFrom); repo != "" {
			return []string{repo}
		}
		return nil
	}, &o.ImportFrom).Value(&o.DotfilesRepo).Validate(func(s string) error {
		if s == "" {
			return nil
		}
		return plan.ValidDotfilesRepo(s)
	})
	imports, folderList := importForm(o)
	previous, previousList := previousForm(o)
	groups = append(append(append(groups, imports...), huh.NewGroup(dotfiles)), previous...)
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) })), []*huh.MultiSelect[string]{apps, plugins, langs, folderList, previousList}
}

// importForm asks for a previous home folder and which of its folders to
// copy. The folders appear once a home folder is chosen, preselected as
// importFolders suggests unless restored choices say otherwise.
func importForm(o *domain.Options) ([]*huh.Group, *huh.MultiSelect[string]) {
	restored := o.ImportFrom
	list := huh.NewMultiSelect[string]().Title("Folders to import").Description("Copied into this home folder. Nothing here is replaced: where a file differs, yours stays and theirs is saved in ~/"+plan.ConflictsFolder+". Hidden folders marked keys hold private keys; bring them only if this Mac should use them.").OptionsFunc(func() []huh.Option[string] {
		choices, _ := importFolders(o.ImportFrom)
		preset := o.ImportFrom != restored || len(o.ImportFolders) == 0
		options := []huh.Option[string]{}
		for _, c := range choices {
			options = append(options, huh.NewOption(c.label, c.name).Selected(preset && c.preselect || !preset && plan.Has(o.ImportFolders, c.name)))
		}
		return options
	}, &o.ImportFrom).Value(&o.ImportFolders).Filterable(true)
	path := huh.NewInput().Title("Previous home folder (optional)").Description("Your previous Mac's home folder, on a drive, a share or restored from a backup, such as /Volumes/Backup/Users/you. Library and app data are left to Migration Assistant. Leave empty to skip.").Suggestions(previousHomes()).Value(&o.ImportFrom).Validate(func(s string) error {
		if s == "" {
			return nil
		}
		_, e := importFolders(s)
		return e
	})
	return []*huh.Group{huh.NewGroup(path), huh.NewGroup(list).WithHideFunc(func() bool { return o.ImportFrom == "" })}, list
}

// previousForm asks what to bring over from a previous Mac. Each question is
// optional; the app list appears once a file is chosen, with every entry
// selected unless restored choices say otherwise.
func previousForm(o *domain.Options) ([]*huh.Group, *huh.MultiSelect[string]) {
	home, _ := homeDir()
	restored := o.PreviousBrewfile
	list := huh.NewMultiSelect[string]().Title("Apps and tools to reinstall").Description("From your previous Mac's app list. Installed ones are skipped and nothing is upgraded.").OptionsFunc(func() []huh.Option[string] {
		entries, _ := previousEntries(o.PreviousBrewfile)
		all := o.PreviousBrewfile != restored || len(o.PreviousPackages) == 0
		options := []huh.Option[string]{}
		for _, e := range entries {
			options = append(options, huh.NewOption(e, e).Selected(all || plan.Has(o.PreviousPackages, e)))
		}
		return options
	}, &o.PreviousBrewfile).Value(&o.PreviousPackages).Filterable(true)
	path := huh.NewInput().Title("Previous app list (optional)").Description("A Homebrew-full.Brewfile from a Golden Gate Recovery folder, or a Brewfile from your dotfiles, to reinstall what your previous Mac had. Leave empty to skip.").SuggestionsFunc(func() []string { return brewfileSuggestions(o.ImportFrom, home) }, &o.ImportFrom).Value(&o.PreviousBrewfile).Validate(func(s string) error {
		if s == "" {
			return nil
		}
		_, e := previousEntries(s)
		return e
	})
	return []*huh.Group{huh.NewGroup(path), huh.NewGroup(list).WithHideFunc(func() bool { return o.PreviousBrewfile == "" })}, list
}

// chezmoiForm asks which imported dotfiles to add to chezmoi's source.
// Nothing is ticked: what is added can end up in a public repository.
func chezmoiForm(h domain.Host, chosen *[]string, dark func() bool) (*huh.Form, *huh.MultiSelect[string]) {
	options := []huh.Option[string]{}
	for _, path := range h.ChezmoiCandidates {
		options = append(options, huh.NewOption(homeName(h.Home, path), path).Selected(plan.Has(*chosen, path)))
	}
	list := huh.NewMultiSelect[string]().Title("Add imported dotfiles to chezmoi?").Description("Chosen files join chezmoi's source, so chezmoi manages them from now on; you commit and push them yourself, and a public repository makes them public. Keys, tokens and caches are never offered.").Options(options...).Value(chosen).Filterable(true)
	return huh.NewForm(huh.NewGroup(list)).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) })), list
}

// homeName writes a path in this home as ~/path.
func homeName(home, path string) string {
	if rel, err := filepath.Rel(home, path); err == nil && inside(home, path) {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
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
		groups = append(groups, huh.NewGroup(huh.NewConfirm().Title("Replace "+c.Path+"?").Description(ChangeText(h, c)).Value(&replace[i])))
	}
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(dark()) }))
}

// ChangeText shows a file change for review: a diff against what will be
// written, or for a dotfile whose contents chezmoi produces only when it
// applies them, what will happen instead.
func ChangeText(h domain.Host, c domain.FileChange) string {
	for _, d := range h.Dotfiles {
		if d.Target != c.Path {
			continue
		}
		if d.SHA256 == "" {
			what := map[string]string{"encrypted": "is encrypted", "modify": "is made by your repository's modify script", "template": "comes from a template that uses secrets or commands"}[d.Kind]
			if what == "" {
				what = "is known only when chezmoi applies it"
			}
			return "Your dotfiles' version " + what + ", so it can't be compared before applying. Default: keep yours. Replacing saves a private backup first; compare afterwards with chezmoi diff."
		}
		return DiffText(h.Files[c.Path].Contents, d.Contents)
	}
	return DiffText(h.Files[c.Path].Contents, c.Desired)
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
