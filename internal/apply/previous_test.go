package apply

import (
	"context"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/testutil"
	"io"
	"strings"
	"testing"
)

func TestTapStep(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	var calls []domain.Command
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error { calls = append(calls, c); return nil })
	handler := OptionalHandlers(r, files.Manager{Roots: []string{h.Home}})["tap"]
	step := domain.Step{ID: "tap:user/tools", Kind: "tap", Check: domain.Check{Kind: "tap", Target: "user/tools"}}
	if ok, _ := handler.Verify(context.Background(), h, domain.Plan{}, step, ""); ok {
		t.Fatal("missing tap verified")
	}
	if _, e := handler.Apply(context.Background(), h, domain.Plan{}, step, ""); e != nil {
		t.Fatal(e)
	}
	if len(calls) != 1 || calls[0].Path != h.BrewPath || strings.Join(calls[0].Args, " ") != "tap user/tools" {
		t.Fatal("unexpected tap command", calls)
	}
	h.Taps = []string{"user/tools"}
	if ok, _ := handler.Verify(context.Background(), h, domain.Plan{}, step, ""); !ok {
		t.Fatal("tap not verified")
	}
}
