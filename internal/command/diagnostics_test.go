package command

import (
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
func TestFragmentedCredentialsRedacted(t *testing.T) {
	var got string
	d := diagnostics{emit: func(s string) { got += s }, remaining: 65536}
	d.Write([]byte("token="))
	d.Write([]byte("secret-value\n"))
	if strings.Contains(got, "secret-value") {
		t.Fatal("split credential leaked", got)
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
func TestDiagnosticsKeepMultibyteText(t *testing.T) {
	var got []string
	d := diagnostics{emit: func(s string) { got = append(got, s) }, remaining: 65536}
	d.Write([]byte("café ✓ d"))
	d.Write([]byte("one\nsplit \xc3"))
	d.Write([]byte("\xa9\n"))
	if strings.Join(got, "|") != "café ✓ done|split é" {
		t.Fatalf("text garbled: %q", got)
	}
}
