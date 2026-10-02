//go:build !windows

package command

import (
	"os"
	"os/exec"
	"syscall"
)

func isolate(cmd *exec.Cmd, interactive bool) {
	if !interactive {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Cancel = func() error {
		if interactive {
			return cmd.Process.Signal(os.Interrupt)
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
}

// kill ends the process group of a command that ignored the interrupt.
func kill(cmd *exec.Cmd) { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
