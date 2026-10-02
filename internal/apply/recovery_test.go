package apply

import (
	"bytes"
	"context"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryNeverOverwritesCuratedBrewfile(t *testing.T) {
	h := testutil.FreshHost(t.TempDir())
	curated := h.Home + "/.Brewfile"
	original := []byte("# curated\nbrew \"fish\"\n")
	os.WriteFile(curated, original, 0600)
	dir := h.Home + "/Golden Gate Recovery/2026-09-30-test"
	r := runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		if c.Args[0] != "bundle" || c.Args[1] != "dump" {
			t.Fatal("unexpected recovery command", c)
		}
		for _, a := range c.Args {
			if a == "--force" {
				t.Fatal("forced inventory")
			}
			if len(a) > 7 && a[:7] == "--file=" {
				return os.WriteFile(a[7:], []byte("brew \"fish\"\ncask \"zed\"\n"), 0600)
			}
		}
		t.Fatal("missing output path")
		return nil
	})
	path, e := PrepareRecovery(context.Background(), h, dir, r)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(curated)
	if !bytes.Equal(original, got) || path == curated {
		t.Fatal("curated file overwritten")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("inventory absent")
	}
	if filepath.Dir(path) != dir {
		t.Fatal("not dated recovery path")
	}
}
func TestNativeExportRemainsSeparate(t *testing.T) {
	o := plan.DefaultOptions()
	tasks := plan.FinishTasks(o)
	found := false
	for _, v := range tasks {
		if v.ID == "applite-export" {
			found = true
		}
	}
	if !found {
		t.Fatal("native export instructions absent")
	}
	h := testutil.FreshHost(t.TempDir())
	_, e := PrepareRecovery(context.Background(), h, h.Home+"/recovery", runFn(func(_ context.Context, c domain.Command, _ io.Writer) error {
		if c.Path != h.BrewPath {
			t.Fatal("native app migration invoked automatically")
		}
		for _, a := range c.Args {
			if len(a) > 7 && a[:7] == "--file=" {
				return os.WriteFile(a[7:], []byte("brew \"fish\""), 0600)
			}
		}
		return nil
	}))
	if e != nil {
		t.Fatal(e)
	}
}
