package command

import (
	"bytes"
	"context"
	"golden-gate-setup/internal/domain"
	"io"
	"strings"
	"testing"
)

func TestInteractiveHandoffHasOneOwner(t *testing.T) {
	calls := 0
	r := ProcessRunner{Handoff: func(_ context.Context, c domain.Command) error {
		calls++
		if c.Args[0] != "path with spaces" {
			t.Fatal("arguments changed")
		}
		return nil
	}}
	if e := r.Run(context.Background(), domain.Command{Path: "not executable", Args: []string{"path with spaces"}, Interactive: true}, io.Discard); e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
	if e := (ProcessRunner{}).Run(context.Background(), domain.Command{Interactive: true}, io.Discard); e == nil {
		t.Fatal("missing handoff accepted")
	}
}
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
func TestFragmentedCredentialsRedacted(t *testing.T) {
	var got string
	d := diagnostics{emit: func(s string) { got += s }, remaining: 65536}
	d.Write([]byte("token="))
	d.Write([]byte("secret-value\n"))
	if strings.Contains(got, "secret-value") {
		t.Fatal("split credential leaked", got)
	}
}
func TestInteractiveProcessKeepsForegroundGroup(t *testing.T) {
	p := Process(context.Background(), domain.Command{Path: "/bin/sh", Interactive: true})
	if p.SysProcAttr != nil && p.SysProcAttr.Setpgid {
		t.Fatal("interactive reader is isolated from foreground terminal group")
	}
}
func TestMultiWordAuthorizationRedacted(t *testing.T) {
	var got string
	d := diagnostics{emit: func(s string) { got += s }, remaining: 65536}
	d.Write([]byte("Authorization: Bearer sensitive-token\npassword=secret with spaces\n"))
	if strings.Contains(got, "sensitive-token") || strings.Contains(got, "with spaces") {
		t.Fatal("credential tail leaked", got)
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
