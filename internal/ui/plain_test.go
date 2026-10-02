package ui

import (
	"bytes"
	"context"
	"errors"
	"golden-gate-setup/internal/domain"
	"io"
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
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nno\n"), &out)
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
	_, err := RunPlain(context.Background(), s, strings.NewReader("no\nzed\nnone\nnone\nno\nno\nno\nno\nno\n"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid saved session") {
		t.Fatal("damaged session silently ignored", err)
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
