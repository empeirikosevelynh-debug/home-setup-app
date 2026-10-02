//go:build !windows

package command

import (
	"bytes"
	"context"
	"golden-gate-setup/internal/domain"
	"io"
	"strings"
	"testing"
)

func TestArgumentsRemainSeparate(t *testing.T) {
	var b bytes.Buffer
	if e := (ProcessRunner{}).Run(context.Background(), domain.Command{Path: "/usr/bin/printf", Args: []string{"%s", "path with spaces; $(no command)"}}, &b); e != nil || b.String() != "path with spaces; $(no command)" {
		t.Fatal(b.String(), e)
	}
}
func TestSanitizedDiagnostics(t *testing.T) {
	var lines []string
	r := ProcessRunner{Diagnostic: func(s string) { lines = append(lines, s) }}
	r.Run(context.Background(), domain.Command{Path: "/bin/sh", Args: []string{"-c", "printf 'token=secret-value\\n' >&2"}}, io.Discard)
	if len(lines) == 0 || strings.Contains(strings.Join(lines, ""), "secret-value") {
		t.Fatal("missing or unsafe diagnostics", lines)
	}
}
func TestInteractiveProcessKeepsForegroundGroup(t *testing.T) {
	p := Process(context.Background(), domain.Command{Path: "/bin/sh", Interactive: true})
	if p.SysProcAttr != nil && p.SysProcAttr.Setpgid {
		t.Fatal("interactive reader is isolated from foreground terminal group")
	}
}
func TestSelectedCommandStreamsBoundedDiagnostics(t *testing.T) {
	var got string
	r := ProcessRunner{Diagnostic: func(s string) { got += s }}
	if e := r.Run(context.Background(), domain.Command{Path: "/usr/bin/printf", Args: []string{"progress line\n"}, Stream: true}, io.Discard); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(got, "progress line") {
		t.Fatal("stdout progress missing")
	}
}
func TestFailureKeepsFinalOutput(t *testing.T) {
	var lines []string
	r := ProcessRunner{Diagnostic: func(s string) { lines = append(lines, s) }}
	script := `i=0; while [ $i -lt 5000 ]; do echo "progress line $i"; i=$((i+1)); done; echo "Error: the real cause" >&2; exit 3`
	e := r.Run(context.Background(), domain.Command{Path: "/bin/sh", Args: []string{"-c", script}, Stream: true}, io.Discard)
	if ExitCode(e) != 3 || !strings.Contains(e.Error(), "Error: the real cause") {
		t.Fatal("failure cause missing from error", e)
	}
	if len(lines) == 0 || lines[len(lines)-1] != "Error: the real cause" || !strings.Contains(strings.Join(lines, "\n"), "lines omitted") {
		t.Fatal("final output not shown after the budget", len(lines))
	}
}
