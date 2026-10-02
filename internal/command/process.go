package command

import (
	"context"
	"golden-gate-setup/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Process prepares a command. A non-interactive command gets its own process
// group, so cancellation interrupts it and anything it started.
func Process(ctx context.Context, c domain.Command) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = Environment(c)
	isolate(cmd, c.Interactive)
	cmd.WaitDelay = 4 * time.Second
	return cmd
}

func Environment(c domain.Command) []string {
	var env []string
	for _, v := range os.Environ() {
		name, _, _ := strings.Cut(v, "=")
		skip := false
		for _, pattern := range c.UnsetEnv {
			if matched, _ := filepath.Match(pattern, name); matched {
				skip = true
			}
		}
		if !skip {
			env = append(env, v)
		}
	}
	return append(env, c.Env...)
}
