package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"context"
	"errors"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"io"
	"os"
	"strings"
)

type Services struct {
	Inspect     func(context.Context, domain.Options) (domain.Host, error)
	Build       func(domain.Host, domain.Options) (domain.Plan, error)
	Apply       func(context.Context, domain.Plan, func(domain.Event)) (domain.Report, error)
	LoadLatest  func() (domain.Session, error)
	BindHandoff func(func(context.Context, domain.Command) error)
}
type HandoffMsg struct {
	Context context.Context
	Command domain.Command
	Done    chan error
}
type handoffDone struct {
	done chan error
	err  error
}
type inspected struct {
	host domain.Host
	plan domain.Plan
	err  error
}
type applyDone struct {
	report domain.Report
	err    error
}
type externalCancel struct{}
type resumeMsg struct{ session domain.Session }
type model struct {
	conflictIndex         int
	ctx                   context.Context
	services              Services
	options, draft        domain.Options
	width, height, scroll int
	dark, canceled        bool
	stage, lines          string
	form                  *huh.Form
	lists                 []*huh.MultiSelect[string]
	host                  domain.Host
	preview               domain.Plan
	report                domain.Report
	err                   error
	conflicts             []domain.FileChange
	replacements          []bool
	resume                domain.Session
	useResume             bool
	events                chan tea.Msg
	cancel                context.CancelFunc
}

func cloneOptions(o domain.Options) domain.Options {
	o.Apps = append([]string(nil), o.Apps...)
	o.Plugins = append([]string(nil), o.Plugins...)
	o.Languages = append([]string(nil), o.Languages...)
	o.Workspaces = append([]domain.Workspace(nil), o.Workspaces...)
	m := map[string]domain.FileDecision{}
	for k, v := range o.FileChoices {
		m[k] = v
	}
	o.FileChoices = m
	return o
}
func newModel(ctx context.Context, s Services) *model {
	o := plan.DefaultOptions()
	return &model{ctx: ctx, services: s, options: o, draft: cloneOptions(o), dark: true, stage: "welcome"}
}
func (m *model) commitDraft() { m.options = cloneOptions(m.draft) }
func (m *model) cancelDraft() {
	m.draft = cloneOptions(m.options)
	m.form = nil
	m.stage = "preview"
	m.lines = "Selection canceled. Press e to edit again, or Esc to exit."
}
func (m *model) edit() {
	m.stage = "select"
	m.draft = cloneOptions(m.options)
	m.form, m.lists = selectionForm(&m.draft, func() bool { return m.dark })
	m.resizeForm()
}
func (m *model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, func() tea.Msg {
		var s domain.Session
		if m.services.LoadLatest != nil {
			var e error
			s, e = m.services.LoadLatest()
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return inspected{err: fmt.Errorf("read saved choices: %w", e)}
			}
		}
		return resumeMsg{s}
	})
}
func (m *model) resizeForm() {
	if m.form == nil {
		return
	}
	w, h := min(66, max(1, m.width-6)), max(3, m.height-9)
	for _, l := range m.lists {
		l.Height(max(2, h-5))
	}
	m.form.WithWidth(w).WithHeight(h)
}
func (m *model) inspectCmd() tea.Cmd {
	o := cloneOptions(m.options)
	return func() tea.Msg {
		h, e := m.services.Inspect(m.ctx, o)
		if e != nil {
			return inspected{err: e}
		}
		p, e := m.services.Build(h, o)
		return inspected{h, p, e}
	}
}
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case externalCancel:
		if m.stage == "applying" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, nil
		}
		m.canceled = true
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.resizeForm()
	case tea.BackgroundColorMsg:
		m.dark = v.IsDark()
		if m.form != nil {
			m.form.WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(m.dark) }))
		}
	case resumeMsg:
		m.resume = v.session
		m.form = welcomeForm(&m.useResume, v.session.Plan.ID != "", func() bool { return m.dark })
		m.resizeForm()
		return m, m.form.Init()
	case inspected:
		if v.err != nil {
			m.err = v.err
			m.stage = "done"
			m.lines = v.err.Error()
			return m, nil
		}
		m.host, m.preview = v.host, v.plan
		m.conflicts = conflictChanges(m.services, m.host, m.options)
		m.replacements = make([]bool, len(m.conflicts))
		if len(m.conflicts) > 0 {
			m.conflictIndex = 0
			m.showConflict()
			return m, nil
		}
		m.showPreview()
		return m, nil
	case domain.Event:
		m.lines += v.Status + ": " + v.Text + "\n"
		if len(m.lines) > 65536 {
			m.lines = m.lines[len(m.lines)-65536:]
		}
		return m, m.readEvent()
	case applyDone:
		m.report, m.err = v.report, v.err
		m.stage = "done"
		m.scroll = 0
		m.lines = ReportText(v.report)
		if v.err != nil {
			m.lines += "\n" + v.err.Error()
		}
		if m.ctx.Err() != nil {
			return m, tea.Quit
		}
		return m, nil
	case HandoffMsg:
		ctx := v.Context
		if ctx == nil {
			ctx = m.ctx
		}
		child := command.Process(ctx, v.Command)
		return m, tea.ExecProcess(child, func(err error) tea.Msg { return handoffDone{v.Done, err} })
	case handoffDone:
		v.done <- v.err
		return m, nil
	case tea.KeyPressMsg:
		if v.String() == "ctrl+c" || v.String() == "esc" {
			if m.stage == "applying" {
				if m.cancel != nil {
					m.cancel()
				}
				m.lines += "Cancel requested; waiting for the current process.\n"
				return m, nil
			}
			m.canceled = true
			return m, tea.Quit
		}
		if m.width < 48 || m.height < 18 {
			return m, nil
		}
		if m.stage == "files" {
			switch v.String() {
			case "up":
				m.scroll = max(0, m.scroll-1)
			case "down":
				m.scroll++
			case "pgdown":
				m.scroll += max(1, m.height-8)
			case "pgup":
				m.scroll = max(0, m.scroll-m.height+8)
			case "y", "n", "enter":
				if v.String() == "y" {
					m.options.FileChoices[m.conflicts[m.conflictIndex].Path] = domain.Replace
				}
				m.conflictIndex++
				if m.conflictIndex < len(m.conflicts) {
					m.showConflict()
				} else {
					p, e := m.services.Build(m.host, m.options)
					m.preview = p
					if e != nil {
						m.err = e
						m.stage = "done"
						m.lines = e.Error()
					} else {
						m.showPreview()
					}
				}
			}
			return m, nil
		}
		if m.stage == "done" || m.stage == "applying" {
			switch v.String() {
			case "up":
				m.scroll = max(0, m.scroll-1)
				return m, nil
			case "down":
				m.scroll++
				return m, nil
			case "pgdown":
				m.scroll += max(1, m.height-8)
				return m, nil
			case "pgup":
				m.scroll = max(0, m.scroll-m.height+8)
				return m, nil
			}
		}
		if m.stage == "done" {
			if v.String() == "q" || v.String() == "enter" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.stage == "preview" {
			switch v.String() {
			case "e":
				m.edit()
				return m, m.form.Init()
			case "up":
				m.scroll = max(0, m.scroll-1)
			case "down":
				m.scroll++
			case "pgdown":
				m.scroll += max(1, m.height-8)
			case "pgup":
				m.scroll = max(0, m.scroll-m.height+8)
			case "a":
				if m.preview.Supported && m.services.Apply != nil {
					m.preview.Accepted = true
					m.stage = "applying"
					m.scroll = 0
					m.lines = "Applying the reviewed plan…\n"
					ctx, cancel := context.WithCancel(m.ctx)
					m.cancel = cancel
					m.events = make(chan tea.Msg, 32)
					p := m.preview
					go func() {
						r, e := m.services.Apply(ctx, p, func(e domain.Event) {
							select {
							case m.events <- e:
							case <-ctx.Done():
							}
						})
						m.events <- applyDone{r, e}
					}()
					return m, m.readEvent()
				}
			}
			return m, nil
		}
	case tea.PasteMsg:
		if m.width < 48 || m.height < 18 {
			return m, nil
		}
	}
	if m.form != nil && (m.stage == "welcome" || m.stage == "select" || m.stage == "files") {
		next, cmd := m.form.Update(msg)
		if f, ok := next.(*huh.Form); ok {
			m.form = f
		}
		if m.form.State == huh.StateCompleted {
			switch m.stage {
			case "welcome":
				if m.useResume {
					m.options = restoredOptions(m.resume.Plan.Options)
				}
				m.edit()
				return m, m.form.Init()
			case "select":
				m.commitDraft()
				m.stage = "inspecting"
				m.form = nil
				return m, m.inspectCmd()
			case "files":
				for i, c := range m.conflicts {
					if m.replacements[i] {
						m.options.FileChoices[c.Path] = domain.Replace
					}
				}
				p, e := m.services.Build(m.host, m.options)
				m.preview = p
				if e != nil {
					m.err = e
					m.stage = "done"
					m.lines = e.Error()
				} else {
					m.showPreview()
				}
				return m, nil
			}
		}
		if m.form.State == huh.StateAborted {
			m.canceled = true
			return m, tea.Quit
		}
		return m, cmd
	}
	return m, nil
}
func (m *model) readEvent() tea.Cmd { return func() tea.Msg { return <-m.events } }
func (m *model) showPreview() {
	m.form = nil
	m.stage = "preview"
	m.scroll = 0
	m.lines = PlanText(m.preview)
	if m.services.Apply == nil {
		m.lines += "\nPreview milestone: installation is unavailable."
	}
}
func Run(ctx context.Context, s Services, in io.Reader, out io.Writer) (domain.Report, error) {
	if s.Inspect == nil || s.Build == nil {
		return domain.Report{}, errors.New("inspection/planning service unavailable")
	}
	m := newModel(ctx, s)
	lifecycle, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := tea.NewProgram(m, tea.WithContext(lifecycle), tea.WithoutSignalHandler(), tea.WithInput(in), tea.WithOutput(out))
	go func() {
		select {
		case <-ctx.Done():
			p.Send(externalCancel{})
		case <-lifecycle.Done():
		}
	}()
	if s.BindHandoff != nil {
		s.BindHandoff(func(ctx context.Context, c domain.Command) error {
			done := make(chan error, 1)
			p.Send(HandoffMsg{Command: c, Done: done, Context: ctx})
			select {
			case e := <-done:
				return e
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	_, e := p.Run()
	if e != nil {
		return m.report, e
	}
	if m.err != nil {
		return m.report, m.err
	}
	if m.canceled {
		return m.report, context.Canceled
	}
	if m.report.Status == "" {
		m.report = domain.Report{PlanID: m.preview.ID, Status: "preview", ManualTasks: m.preview.ManualTasks}
	}
	return m.report, nil
}
func PlanText(p domain.Plan) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Review the current plan")
	for _, s := range p.Problems {
		fmt.Fprintln(&b, "Needs attention: "+s)
	}
	for i, s := range p.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s.Label)
	}
	fmt.Fprintln(&b, "\nManual follow-up:")
	for _, s := range p.ManualTasks {
		fmt.Fprintf(&b, "• %s — %s\n", s.Title, s.Instructions)
	}
	return b.String()
}
func ReportText(r domain.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Setup status: %s\n", r.Status)
	for _, s := range r.Steps {
		fmt.Fprintf(&b, "%s: %s %s\n", s.Status, s.ID, s.Message)
		if s.BackupPath != "" {
			fmt.Fprintln(&b, "Backup: "+s.BackupPath)
		}
	}
	if r.SessionPath != "" {
		fmt.Fprintln(&b, "Session: "+r.SessionPath)
	}
	for _, s := range r.ManualTasks {
		fmt.Fprintf(&b, "\nRemaining: %s\n%s\n%s\n", s.Title, s.Instructions, s.URL)
	}
	return b.String()
}

func (m *model) showConflict() {
	m.stage = "files"
	m.form = nil
	m.scroll = 0
	c := m.conflicts[m.conflictIndex]
	m.lines = c.Path + "\n" + DiffText(m.host.Files[c.Path].Contents, c.Desired)
}

func restoredOptions(o domain.Options) domain.Options {
	o = cloneOptions(o)
	o.FileChoices = map[string]domain.FileDecision{}
	return o
}
