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
	if _, e := fmt.Fprintf(p.out, "%s [%s]: ", label, def); e != nil {
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
	o := plan.DefaultOptions()
	fmt.Fprintln(out, "Golden Gate Setup\nMigration first: restore files and settings with Migration Assistant before setup when reinstalling macOS. Existing files are preserved by default.")
	if _, e := p.yes("Have you restored an existing backup? (no is fine for a fresh Mac)", false); e != nil {
		return domain.Report{}, e
	}
	if s.LoadLatest != nil {
		saved, e := s.LoadLatest()
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return domain.Report{}, fmt.Errorf("read saved choices: %w", e)
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
	o.Apps, e = p.list("Applications", o.Apps, plan.Apps)
	if e != nil {
		return domain.Report{}, e
	}
	o.Plugins, e = p.list("Fish plugins", o.Plugins, append(append([]string{}, plan.StartingPlugins...), plan.ExtraPlugins...))
	if e != nil {
		return domain.Report{}, e
	}
	o.Languages, e = p.list("Languages", o.Languages, plan.Languages)
	if e != nil {
		return domain.Report{}, e
	}
	for _, v := range []struct {
		label string
		value *bool
	}{{"Configure Git display and editor?", &o.ConfigureGit}, {"Adopt reviewed files in chezmoi?", &o.AdoptChezmoi}, {"Capture complete Homebrew inventory?", &o.CaptureInventory}, {"Prepare recovery notes?", &o.PrepareRecovery}} {
		*v.value, e = p.yes(v.label, *v.value)
		if e != nil {
			return domain.Report{}, e
		}
	}
	o.Workspaces = nil
	for _, l := range o.Languages {
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
		name, e := p.ask("Module/project name", "none")
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
	h, e := s.Inspect(ctx, o)
	if e != nil {
		return domain.Report{}, e
	}
	for _, c := range conflictChanges(s, h, o) {
		fmt.Fprintln(out, c.Path+"\n"+DiffText(h.Files[c.Path].Contents, c.Desired))
		yes, e := p.yes("Replace this file and save a private backup?", false)
		if e != nil {
			return domain.Report{}, e
		}
		if yes {
			o.FileChoices[c.Path] = domain.Replace
		}
	}
	reviewed, e := s.Build(h, o)
	if e != nil {
		return domain.Report{}, e
	}
	fmt.Fprintln(out, PlanText(reviewed))
	yes, e := p.yes("Apply this reviewed plan?", false)
	if e != nil {
		return domain.Report{}, e
	}
	if !yes || s.Apply == nil {
		return domain.Report{PlanID: reviewed.ID, Status: "preview", ManualTasks: reviewed.ManualTasks}, nil
	}
	if !reviewed.Supported {
		return domain.Report{}, errors.New("resolve the listed prerequisites before applying")
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
	return r, e
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
