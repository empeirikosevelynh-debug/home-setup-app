package command

import (
	"context"
	"golden-gate-setup/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Process(ctx context.Context, c domain.Command) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = Environment(c)
	if !c.Interactive {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Cancel = func() error {
		if c.Interactive {
			return cmd.Process.Signal(os.Interrupt)
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
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
