package apply

import (
	"context"
	"errors"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type Handler struct {
	Apply  func(context.Context, domain.Host, domain.Plan, domain.Step, string) (domain.StepResult, error)
	Verify func(context.Context, domain.Host, domain.Plan, domain.Step, string) (bool, error)
}
type Executor struct {
	Inspect  func(context.Context, domain.Options) (domain.Host, error)
	Build    func(domain.Host, domain.Options) (domain.Plan, error)
	Runner   command.Runner
	Files    files.Manager
	Store    SessionStore
	Now      func() time.Time
	Handlers map[string]Handler
}

func (e *Executor) Execute(ctx context.Context, p domain.Plan, emit func(domain.Event)) (report domain.Report, err error) {
	report = domain.Report{PlanID: p.ID, Status: "failed", ManualTasks: append([]domain.ManualTask(nil), p.ManualTasks...)}
	if !p.Accepted || !p.Supported {
		return report, errors.New("execution requires acceptance of a supported current plan")
	}
	if e.Inspect == nil || e.Build == nil || e.Runner == nil {
		return report, errors.New("execution services unavailable")
	}
	id, er := plan.Fingerprint(p)
	if er != nil || id != p.ID {
		return report, errors.New("reviewed plan was modified")
	}
	host, er := e.Inspect(ctx, p.Options)
	if er != nil {
		return report, er
	}
	current, er := e.Build(host, p.Options)
	if er != nil {
		return report, er
	}
	if current.ID != p.ID {
		return report, errors.New("state changed since preview; inspect and accept a fresh plan")
	}
	if er = ctx.Err(); er != nil {
		report.Status = "interrupted"
		return report, er
	}
	report.SessionPath = e.Store.Path(p.ID)
	unlock, er := e.Files.Lock(filepath.Join(e.Store.Dir, "apply.lock"))
	if er != nil {
		return report, er
	}
	defer unlock()
	session := domain.Session{SchemaVersion: 1, Plan: p, Status: "running"}
	if er = e.Store.Save(session); er != nil {
		return report, er
	}
	dir := filepath.Join(e.Store.Dir, p.ID+".backups")
	done := map[string]bool{}
	save := func(r domain.StepResult) error {
		report.Steps = append(report.Steps, r)
		session.Results = append(session.Results, r)
		return e.Store.Save(session)
	}
	fail := func(s domain.Step, r domain.StepResult, cause error) (domain.Report, error) {
		r.ID = s.ID
		r.Status = "failed"
		if ctx.Err() != nil {
			report.Status = "interrupted"
			r.Status = "interrupted"
			cause = ctx.Err()
		}
		session.Status = report.Status
		r.Message = cause.Error()
		if se := save(r); se != nil {
			cause = errors.Join(cause, se)
		}
		if emit != nil {
			emit(domain.Event{StepID: s.ID, Status: r.Status, Text: s.Label})
		}
		return report, cause
	}
	for _, step := range p.Steps {
		if er = ctx.Err(); er != nil {
			report.Status = "interrupted"
			session.Status = "interrupted"
			if se := e.Store.Save(session); se != nil {
				er = errors.Join(er, se)
			}
			return report, er
		}
		for _, dependency := range step.DependsOn {
			if !done[dependency] {
				return fail(step, domain.StepResult{}, fmt.Errorf("unsatisfied dependency %s", dependency))
			}
		}
		// host is the latest inspection: the one that confirmed the plan, or the
		// one taken after the previous step acted. Steps that only verify change
		// nothing, so inspecting again before every step would only repeat it.
		satisfied, er := e.verify(ctx, host, p, step, dir)
		if er != nil {
			return fail(step, domain.StepResult{}, er)
		}
		result := domain.StepResult{ID: step.ID}
		if satisfied {
			result.Status = "unchanged"
		} else {
			if emit != nil {
				emit(domain.Event{StepID: step.ID, Status: "running", Text: step.Label})
			}
			if er = ctx.Err(); er != nil {
				return fail(step, result, er)
			}
			for _, expected := range step.BeforeFiles {
				actual := host.Files[expected.Path]
				if actual.Exists != expected.Exists || actual.SHA256 != expected.SHA256 || actual.Symlink != expected.Symlink {
					return fail(step, result, fmt.Errorf("configuration changed since preview: %s", expected.Path))
				}
			}
			title := "Review " + step.Label
			switch step.Kind {
			case "package":
				if step.Package == nil {
					return fail(step, result, errors.New("package step has no package"))
				}
				q := step.Package
				if q.Kind == "cask" {
					vendor := false
					for _, a := range host.Apps {
						if strings.EqualFold(a.Name, plan.AppNames[q.Token]) {
							vendor = true
						}
					}
					if vendor {
						result.Status = "preserved"
						result.Message = "An app appeared outside Homebrew; review its distribution channel."
						title = "Review existing " + q.Token
						break
					}
				}
				er = e.Runner.Run(ctx, domain.Command{Path: host.BrewPath, Stream: true, Interactive: q.Kind == "cask", Args: []string{"install", "--" + q.Kind, q.Token}, Env: []string{"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_INSTALL_CLEANUP=1", "HOMEBREW_NO_INSTALL_UPGRADE=1"}}, io.Discard)
			case "file":
				if step.File == nil {
					return fail(step, result, errors.New("file step has no reviewed change"))
				}
				result, er = e.Files.ApplyContext(ctx, *step.File, dir)
			default:
				handler, ok := e.Handlers[step.Kind]
				if !ok || handler.Apply == nil {
					return fail(step, result, fmt.Errorf("%s handler is unavailable", step.Kind))
				}
				result, er = handler.Apply(ctx, host, p, step, dir)
			}
			if er != nil {
				return fail(step, result, er)
			}
			if er = ctx.Err(); er != nil {
				return fail(step, result, er)
			}
			if result.Status == "preserved" {
				report.ManualTasks = append(report.ManualTasks, domain.ManualTask{ID: "deferred:" + step.ID, Title: title, Instructions: result.Message, Required: true})
			}
			if result.Status != "preserved" {
				host, er = e.Inspect(ctx, p.Options)
				if er != nil {
					return fail(step, result, er)
				}
				satisfied, er = e.verify(ctx, host, p, step, dir)
				if er != nil {
					return fail(step, result, er)
				}
				if !satisfied {
					return fail(step, result, errors.New("the action finished but verification did not pass"))
				}
				result.Status = "verified"
			}
		}
		result.ID = step.ID
		if er = save(result); er != nil {
			return fail(step, result, er)
		}
		done[step.ID] = true
		if emit != nil {
			emit(domain.Event{StepID: step.ID, Status: result.Status, Text: step.Label})
		}
	}
	report.Status = "complete"
	session.Status = "complete"
	if er = e.Store.Save(session); er != nil {
		return report, er
	}
	return report, nil
}
