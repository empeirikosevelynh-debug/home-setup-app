package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"io"
	"os"
	"strings"
)

type prompts struct {
	ctx context.Context
	in  *bufio.Reader
	out io.Writer
}

func (p prompts) ask(label, def string) (string, error) {
	if e := p.ctx.Err(); e != nil {
		return "", e
	}
	prompt := fmt.Sprintf("%s [%s]: ", label, def)
	if def == "" {
		prompt = label + ": "
	}
	if _, e := fmt.Fprint(p.out, prompt); e != nil {
		return "", e
	}
	type answer struct {
		line []byte
		err  error
	}
	answers := make(chan answer, 1)
	go func() { line, e := p.readLine(); answers <- answer{line, e} }()
	var line []byte
	select {
	case <-p.ctx.Done():
		return "", p.ctx.Err()
	case a := <-answers:
		if a.err != nil {
			return "", a.err
		}
		line = a.line
	}
	s := strings.TrimSpace(string(line))
	if s == "" {
		s = def
	}
	return s, nil
}
func (p prompts) yes(label string, def bool) (bool, error) {
	d := "no"
	if def {
		d = "yes"
	}
	s, e := p.ask(label, d)
	if e != nil {
		return false, e
	}
	switch strings.ToLower(s) {
	case "yes", "y":
		return true, nil
	case "no", "n":
		return false, nil
	}
	return false, fmt.Errorf("%s: answer yes or no", label)
}
func (p prompts) list(label string, current, allowed []string) ([]string, error) {
	def := strings.Join(current, ",")
	if def == "" {
		def = "none"
	}
	s, e := p.ask(label+" ("+strings.Join(allowed, ",")+")", def)
	if e != nil {
		return nil, e
	}
	if s == "none" {
		return nil, nil
	}
	var r []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if !plan.Has(allowed, v) {
			return nil, fmt.Errorf("unknown choice %q", v)
		}
		if !plan.Has(r, v) {
			r = append(r, v)
		}
	}
	return r, nil
}
func RunPlain(ctx context.Context, s Services, in io.Reader, out io.Writer) (domain.Report, error) {
	if s.Inspect == nil || s.Build == nil {
		return domain.Report{}, errors.New("inspection/planning service unavailable")
	}
	p := prompts{ctx, bufio.NewReader(in), out}
	o := plan.DefaultOptionsFor(goos)
	intro, question := "Migration first: restore files and settings with Migration Assistant before setup when reinstalling macOS, or bring over only your files and apps from your previous home folder below.", "Have you restored an existing backup? (no is fine for a fresh Mac)"
	if goos == "windows" {
		intro, question = "Restore first: when moving to a new PC, restore your files from your backup before setup.", "Have you restored an existing backup? (no is fine for a fresh PC)"
	}
	fmt.Fprintln(out, "Golden Gate Setup\n"+intro+" Existing files are preserved by default.")
	if _, e := p.yes(question, false); e != nil {
		return domain.Report{}, e
	}
	if s.LoadLatest != nil {
		saved, e := s.LoadLatest()
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			fmt.Fprintln(out, savedChoicesNotice(e))
		}
		if e == nil && saved.Plan.ID != "" {
			yes, e := p.yes("Restore saved choices and review again?", true)
			if e != nil {
				return domain.Report{}, e
			}
			if yes {
				o = restoredOptions(saved.Plan.Options)
			}
		}
	}
	var e error
	offered := plan.Choices(goos)
	o.Apps, e = p.list("Applications", o.Apps, offered.Apps)
	if e != nil {
		return domain.Report{}, e
	}
	if len(offered.Plugins) > 0 {
		o.Plugins, e = p.list("Fish plugins", o.Plugins, offered.Plugins)
		if e != nil {
			return domain.Report{}, e
		}
	}
	o.Languages, e = p.list("Languages", o.Languages, offered.Languages)
	if e != nil {
		return domain.Report{}, e
	}
	questions := []struct {
		label string
		value *bool
	}{{"Configure Git display and editor?", &o.ConfigureGit}, {"Adopt reviewed files in chezmoi?", &o.AdoptChezmoi}, {"Capture complete Homebrew inventory?", &o.CaptureInventory}, {"Prepare recovery notes?", &o.PrepareRecovery}}
	if goos == "windows" {
		questions = questions[:2]
	}
	for _, v := range questions {
		*v.value, e = p.yes(v.label, *v.value)
		if e != nil {
			return domain.Report{}, e
		}
	}
	o.Workspaces = nil
	workspaceLanguages := o.Languages
	if goos == "windows" {
		workspaceLanguages = nil
	}
	for _, l := range workspaceLanguages {
		path, e := p.ask(l+" workspace absolute path (none skips)", "none")
		if e != nil {
			return domain.Report{}, e
		}
		if path == "none" {
			continue
		}
		if !strings.HasPrefix(path, "/") {
			return domain.Report{}, errors.New("workspace needs an absolute path")
		}
		label := "Project/binary name"
		if l == "go" {
			label = "Go module path"
		}
		name, e := p.ask(label, "")
		if e != nil {
			return domain.Report{}, e
		}
		entry := "main.cr"
		if l == "crystal" {
			entry, e = p.ask("Entry point under src/", entry)
			if e != nil {
				return domain.Report{}, e
			}
		}
		create, e := p.yes("Create a new project if absent?", false)
		if e != nil {
			return domain.Report{}, e
		}
		o.Workspaces = append(o.Workspaces, domain.Workspace{Language: l, Path: path, Module: name, EntryPoint: entry, Create: create})
	}
	if goos != "windows" {
		if o.DotfilesRepo, e = p.ask("Dotfiles repository for chezmoi (a GitHub user, user/repo or a Git URL; none skips)", or(o.DotfilesRepo, "none")); e != nil {
			return domain.Report{}, e
		}
		if o.DotfilesRepo == "none" {
			o.DotfilesRepo = ""
		} else if e = plan.ValidDotfilesRepo(o.DotfilesRepo); e != nil {
			return domain.Report{}, e
		}
		if e = askImport(p, &o); e != nil {
			return domain.Report{}, e
		}
		if e = askPrevious(p, &o); e != nil {
			return domain.Report{}, e
		}
	}
	for {
		r, reviewed, e := reviewPlain(ctx, s, p, in, o)
		if e != nil || r.Status != "complete" || reviewed.Later == "" {
			return r, e
		}
		fmt.Fprintln(out, "Next: "+reviewed.Later)
		if again, e := p.yes("Review the rest of setup now?", true); e != nil || !again {
			return r, e
		}
		o.FileChoices = map[string]domain.FileDecision{}
	}
}

// reviewPlain inspects, asks about each replacement, shows the plan and
// applies it once accepted. Interactive installers get the terminal in.
func reviewPlain(ctx context.Context, s Services, p prompts, in io.Reader, o domain.Options) (domain.Report, domain.Plan, error) {
	out := p.out
	h, e := s.Inspect(ctx, o)
	if e != nil {
		return domain.Report{}, domain.Plan{}, e
	}
	for _, c := range conflictChanges(s, h, o) {
		fmt.Fprintln(out, c.Path+"\n"+DiffText(h.Files[c.Path].Contents, proposed(h, c)))
		yes, e := p.yes("Replace this file and save a private backup?", false)
		if e != nil {
			return domain.Report{}, domain.Plan{}, e
		}
		if yes {
			o.FileChoices[c.Path] = domain.Replace
		}
	}
	reviewed, e := s.Build(h, o)
	if e != nil {
		return domain.Report{}, reviewed, e
	}
	fmt.Fprintln(out, PlanText(reviewed))
	yes, e := p.yes("Apply this reviewed plan?", false)
	if e != nil {
		return domain.Report{}, reviewed, e
	}
	if !yes || s.Apply == nil {
		return domain.Report{PlanID: reviewed.ID, Status: "preview", ManualTasks: reviewed.ManualTasks}, reviewed, nil
	}
	if !reviewed.Supported {
		return domain.Report{}, reviewed, errors.New("resolve the listed prerequisites before applying")
	}
	reviewed.Accepted = true
	if s.BindHandoff != nil {
		s.BindHandoff(func(ctx context.Context, c domain.Command) error {
			if p.in.Buffered() != 0 {
				return errors.New("interactive installation requires unbuffered terminal input; run again in a terminal")
			}
			return plainHandoff(ctx, c, in, out)
		})
	}
	r, e := s.Apply(ctx, reviewed, func(v domain.Event) { fmt.Fprintf(out, "%s: %s\n", v.Status, v.Text) })
	fmt.Fprintln(out, ReportText(r))
	return r, reviewed, e
}

// or is value, or def when value is empty.
func or(value, def string) string {
	if value == "" {
		return def
	}
	return value
}
func (p prompts) readLine() ([]byte, error) {
	var line []byte
	for {
		part, e := p.in.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > 65536 {
			return nil, errors.New("answer exceeds 64 KiB")
		}
		if e == bufio.ErrBufferFull {
			continue
		}
		if e != nil {
			return nil, fmt.Errorf("incomplete input: %w", e)
		}
		return line, nil
	}
}

// askImport asks for a previous home folder and which of its folders to
// copy.
func askImport(p prompts, o *domain.Options) error {
	suggestion := "none"
	if o.ImportFrom != "" {
		suggestion = o.ImportFrom
	} else if homes := previousHomes(); len(homes) > 0 {
		suggestion = homes[0]
	}
	path, e := p.ask("Previous home folder to import from (absolute path, none skips)", suggestion)
	if e != nil || path == "none" {
		o.ImportFrom, o.ImportFolders = "", nil
		return e
	}
	choices, e := importFolders(path)
	if e != nil {
		return fmt.Errorf("previous home folder: %w", e)
	}
	var shown, preset []string
	for _, c := range choices {
		label := c.label
		if c.name == "." {
			label = ". (the files at the top, such as .zshrc)"
		}
		shown = append(shown, label)
		if c.preselect {
			preset = append(preset, c.name)
		}
	}
	if path == o.ImportFrom && len(o.ImportFolders) > 0 {
		preset = o.ImportFolders
	}
	def := strings.Join(preset, ",")
	if def == "" {
		def = "none"
	}
	fmt.Fprintf(p.out, "It holds: %s\nNothing here is replaced: where a file differs, yours stays and theirs is saved in ~/%s. Folders marked keys hold private keys.\n", strings.Join(shown, ", "), plan.ConflictsFolder)
	answer, e := p.ask("Import which? (a comma list, or none)", def)
	if e != nil {
		return e
	}
	folders, e := chooseFolders(answer, choices)
	if e != nil || len(folders) == 0 {
		o.ImportFrom, o.ImportFolders = "", nil
		return e
	}
	o.ImportFrom, o.ImportFolders = path, folders
	return nil
}

// askPrevious asks for the previous Mac's app list and which entries to
// reinstall.
func askPrevious(p prompts, o *domain.Options) error {
	suggestion := "none"
	home, _ := homeDir()
	if found := recoveryInventories(o.ImportFrom, home); len(found) > 0 {
		suggestion = found[0]
	}
	path, e := p.ask("Previous app list: a Homebrew-full.Brewfile (none skips)", suggestion)
	if e != nil || path == "none" {
		o.PreviousBrewfile, o.PreviousPackages = "", nil
		return e
	}
	entries, e := previousEntries(path)
	if e != nil {
		return fmt.Errorf("previous app list: %w", e)
	}
	fmt.Fprintf(p.out, "It lists %d apps and tools: %s\n", len(entries), strings.Join(entries, ", "))
	answer, e := p.ask("Reinstall which? (all, none, or a comma list)", "all")
	if e != nil {
		return e
	}
	o.PreviousBrewfile = path
	o.PreviousPackages, e = choosePrevious(answer, entries)
	return e
}
