package testutil

import (
	"context"
	"fmt"
	"golden-gate-setup/internal/domain"
	"io"
	"strings"
	"sync"
)

type Response struct {
	Output string
	Err    error
}
type FakeRunner struct {
	mu        sync.Mutex
	Responses map[string]Response
	Calls     []domain.Command
}

func CommandKey(c domain.Command) string { return c.Path + " " + strings.Join(c.Args, " ") }
func (f *FakeRunner) Run(ctx context.Context, c domain.Command, w io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, c)
	if err := ctx.Err(); err != nil {
		return err
	}
	r, ok := f.Responses[CommandKey(c)]
	if !ok {
		return fmt.Errorf("unexpected fixture command: %s", CommandKey(c))
	}
	if w != nil {
		io.WriteString(w, r.Output)
	}
	return r.Err
}
