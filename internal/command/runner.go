package command

import (
	"context"
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"io"
	"time"
)

type Runner interface {
	Run(context.Context, domain.Command, io.Writer) error
}
type ProcessRunner struct {
	Diagnostic func(string)
	Handoff    func(context.Context, domain.Command) error
}

func (r ProcessRunner) Run(ctx context.Context, c domain.Command, out io.Writer) error {
	if c.Interactive {
		if r.Handoff == nil {
			return fmt.Errorf("interactive command needs exclusive terminal ownership")
		}
		return r.Handoff(ctx, c)
	}
	cmd := Process(ctx, c)
	cmd.Stdout = out
	diagnostic := &diagnostics{emit: r.Diagnostic, remaining: 65536}
	if c.Stream {
		cmd.Stdout = diagnostic
	}
	cmd.Stderr = diagnostic
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s failed to start: %w", c.Path, err)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
			return
		case <-timer.C:
			kill(cmd)
		}
	}()
	err := cmd.Wait()
	close(done)
	diagnostic.flush()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if last := diagnostic.lastLine(); last != "" {
			return fmt.Errorf("%s failed: %w: %s", c.Path, err, last)
		}
		return fmt.Errorf("%s failed: %w", c.Path, err)
	}
	return nil
}
func ExitCode(err error) int {
	var e interface{ ExitCode() int }
	if errors.As(err, &e) {
		return e.ExitCode()
	}
	return -1
}
