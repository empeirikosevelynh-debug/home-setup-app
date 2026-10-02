package ui

import (
	"bytes"
	"context"
	"errors"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPlainAnswersRemainSeparate(t *testing.T) {
	s := services(t)
	var got domain.Options
	s.Inspect = func(_ context.Context, o domain.Options) (domain.Host, error) {
		got = o
		return services(t).Inspect(context.Background(), o)
	}
	var out bytes.Buffer
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nnone\nnone\nno\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Apps) != 1 || got.Apps[0] != "zed" || len(got.Plugins) != 0 || got.ConfigureGit {
		t.Fatalf("answers lost: %+v", got)
	}
	if strings.ContainsRune(out.String(), 27) {
		t.Fatal("ANSI in accessible output")
	}
}
func TestCorruptSavedSessionIsReported(t *testing.T) {
	s := services(t)
	s.LoadLatest = func() (domain.Session, error) { return domain.Session{}, errors.New("invalid saved session") }
	var out bytes.Buffer
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nnone\nnone\nno\n"), &out)
	if err != nil {
		t.Fatal("damaged session blocked setup", err)
	}
	if !strings.Contains(out.String(), "invalid saved session") {
		t.Fatal("damaged session silently ignored")
	}
}
func TestPlainProjectNameHasNoPlaceholder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspaces are macOS-only; plain mode takes /-rooted paths")
	}
	s := services(t)
	path := t.TempDir() + "/project"
	var out bytes.Buffer
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnim\nno\nno\nno\nno\n"+path+"\n\nyes\nnone\nnone\n"), &out)
	if err == nil || !strings.Contains(err.Error(), "project/binary name") {
		t.Fatal("empty project name was not rejected", err)
	}
	if !strings.Contains(out.String(), "Project/binary name: ") {
		t.Fatal("project name prompt offers a placeholder default")
	}
}
func TestPlainEOF(t *testing.T) {
	var out bytes.Buffer
	_, err := RunPlain(context.Background(), services(t), strings.NewReader("no\nzed\n"), &out)
	if err == nil || strings.ContainsRune(out.String(), 27) {
		t.Fatal("incomplete input must fail without ANSI")
	}
}
func TestPlainCancellationUnblocks(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := RunPlain(ctx, services(t), &notifyReader{Reader: r, entered: entered}, io.Discard)
		done <- e
	}()
	<-entered
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("canceled prompt still waiting for input")
	}
}

type notifyReader struct {
	io.Reader
	entered chan struct{}
	once    sync.Once
}

func (r *notifyReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	return r.Reader.Read(p)
}
func TestPlainWindowsQuestions(t *testing.T) {
	onWindows(t)
	h := testutil.FreshWindowsHost(t.TempDir())
	var got domain.Options
	s := Services{Inspect: func(_ context.Context, o domain.Options) (domain.Host, error) { got = o; return h, nil }, Build: plan.Build}
	var out bytes.Buffer
	if _, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nno\nno\nno\n"), &out); err != nil {
		t.Fatal(err, out.String())
	}
	text := out.String()
	if strings.Contains(text, "Fish plugins") || strings.Contains(text, "Homebrew") || strings.Contains(text, "workspace") || !strings.Contains(text, "fresh PC") {
		t.Fatal("macOS questions asked on Windows:", text)
	}
	if strings.Join(got.Apps, ",") != "zed" || got.ConfigureGit || len(got.Plugins) != 0 {
		t.Fatalf("answers lost: %+v", got)
	}
}
func TestPlainPreviousAppList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Golden Gate Recovery", "2026-09-30-abc")
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "Homebrew-full.Brewfile")
	os.WriteFile(path, []byte("tap \"user/tools\"\nbrew \"jq\"\ncask \"firefox\"\n"), 0600)
	old := homeDir
	homeDir = func() (string, error) { return filepath.Dir(filepath.Dir(dir)), nil }
	t.Cleanup(func() { homeDir = old })
	s := services(t)
	var got domain.Options
	s.Inspect = func(_ context.Context, o domain.Options) (domain.Host, error) {
		got = o
		h, e := services(t).Inspect(context.Background(), o)
		data, _ := os.ReadFile(o.PreviousBrewfile)
		h.PreviousEntries, h.PreviousOther = plan.ParseBrewfile(data)
		return h, e
	}
	var out bytes.Buffer
	// No previous home folder; Enter accepts the suggested app list, then
	// picks one entry.
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nnone\n\nformula:jq\nno\n"), &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	if got.PreviousBrewfile != path || len(got.PreviousPackages) != 1 || got.PreviousPackages[0] != "formula:jq" {
		t.Fatalf("app list answers lost: %q %q", got.PreviousBrewfile, got.PreviousPackages)
	}
	if !strings.Contains(out.String(), "It lists 3 apps and tools") {
		t.Fatal("entries not shown:", out.String())
	}
}
func TestChoosePrevious(t *testing.T) {
	entries := []string{"tap:user/tools", "formula:jq"}
	if got, _ := choosePrevious("all", entries); len(got) != 2 {
		t.Fatal(got)
	}
	if got, _ := choosePrevious("none", entries); got != nil {
		t.Fatal(got)
	}
	if got, e := choosePrevious(" formula:jq , formula:jq", entries); e != nil || len(got) != 1 {
		t.Fatal(got, e)
	}
	if _, e := choosePrevious("cask:other", entries); e == nil {
		t.Fatal("entry outside the list accepted")
	}
}
