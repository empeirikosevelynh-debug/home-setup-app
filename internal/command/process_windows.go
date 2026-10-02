//go:build windows

package command

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

// isolate starts a non-interactive command in its own console process group,
// so cancellation can send it Ctrl+Break without interrupting this program.
// An interactive command shares the console and receives the user's Ctrl+C
// directly; cancelling it from here ends the process.
func isolate(cmd *exec.Cmd, interactive bool) {
	if !interactive {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	}
	cmd.Cancel = func() error {
		if interactive {
			return cmd.Process.Kill()
		}
		return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
	}
}

// kill ends a command that ignored Ctrl+Break. Unlike the Unix version it
// ends only the process itself; programs it started may keep running.
func kill(cmd *exec.Cmd) { cmd.Process.Kill() }
