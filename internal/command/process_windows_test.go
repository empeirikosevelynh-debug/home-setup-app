package command

import (
	"context"
	"errors"
	"golden-gate-setup/internal/domain"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWindowsCancelStopsCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	// ping ignores Ctrl+Break, so this also exercises the final kill.
	e := (ProcessRunner{}).Run(ctx, domain.Command{Path: `C:\Windows\System32\PING.EXE`, Args: []string{"-n", "60", "127.0.0.1"}}, io.Discard)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatal("cancelled command kept running", elapsed)
	}
}
func TestWindowsFailureReportsOutput(t *testing.T) {
	e := (ProcessRunner{}).Run(context.Background(), domain.Command{Path: `C:\Windows\System32\cmd.exe`, Args: []string{"/c", "echo Error: the real cause 1>&2 & exit 3"}}, io.Discard)
	if ExitCode(e) != 3 || !strings.Contains(e.Error(), "Error: the real cause") {
		t.Fatal(e)
	}
}
