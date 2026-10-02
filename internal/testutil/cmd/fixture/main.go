// Fixture is a separate acceptance executable. No fake services are linked
// into the production command, and every filesystem change stays in its home.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"golden-gate-setup/internal/apply"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	testutil "golden-gate-setup/internal/testutil/sandbox"
	"golden-gate-setup/internal/ui"
	"os"
	"os/signal"
	"path/filepath"
)

func main() { os.Exit(run()) }
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	before, _ := unix.IoctlGetTermios(0, unix.TIOCGETA)
	accessible := flag.Bool("accessible", false, "plain prompts")
	handoff := flag.Bool("handoff", false, "test a foreground terminal child")
	slow := flag.Bool("slow", false, "wait for cancellation during apply")
	result := flag.String("result", "", "write fixture result outside the temporary home")
	flag.Parse()
	home, e := os.MkdirTemp("", "golden-setup-pty-")
	if e != nil {
		panic(e)
	}
	defer os.RemoveAll(home)
	sandbox := testutil.NewSandbox(home)
	m := files.Manager{Roots: []string{home}}
	x := apply.Executor{Inspect: sandbox.Inspect, Build: plan.Build, Runner: sandbox, Files: m, Store: apply.SessionStore{Dir: filepath.Join(home, "sessions"), Root: home}, Handlers: apply.ConfigurationHandlers(sandbox)}
	for k, v := range apply.OptionalHandlers(sandbox, m) {
		x.Handlers[k] = v
	}
	var ownership func(context.Context, domain.Command) error
	var chosen domain.Options
	s := ui.Services{Inspect: func(c context.Context, o domain.Options) (domain.Host, error) {
		chosen = o
		return sandbox.Inspect(c, o)
	}, Build: plan.Build, BindHandoff: func(f func(context.Context, domain.Command) error) { ownership = f }}
	s.Apply = func(c context.Context, p domain.Plan, emit func(domain.Event)) (domain.Report, error) {
		if *slow {
			emit(domain.Event{Status: "running", Text: "Fixture waits for cancellation"})
			<-c.Done()
			return domain.Report{Status: "interrupted", ManualTasks: p.ManualTasks}, c.Err()
		}
		if *handoff {
			if e := ownership(c, domain.Command{Path: "/bin/sh", Args: []string{"-c", "printf 'FOREGROUND INPUT: '; IFS= read -r answer; test \"$answer\" = owned && printf '\\nFOREGROUND OK\\n'"}, Interactive: true}); e != nil {
				status := "failed"
				if c.Err() != nil {
					status = "interrupted"
				}
				return domain.Report{Status: status}, e
			}
		}
		return x.Execute(c, p, emit)
	}
	var r domain.Report
	if *accessible {
		r, e = ui.RunPlain(ctx, s, os.Stdin, os.Stdout)
	} else {
		r, e = ui.Run(ctx, s, os.Stdin, os.Stdout)
	}
	if *result != "" {
		after, _ := unix.IoctlGetTermios(0, unix.TIOCGETA)
		restored := before != nil && after != nil && before.Iflag == after.Iflag && before.Oflag == after.Oflag && before.Cflag == after.Cflag && before.Lflag&^unix.PENDIN == after.Lflag&^unix.PENDIN && before.Cc == after.Cc
		b, _ := json.Marshal(struct {
			Report           domain.Report
			Options          domain.Options
			TerminalRestored bool
		}{r, chosen, restored})
		if er := os.WriteFile(*result, b, 0600); er != nil {
			panic(er)
		}
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	return 0
}
