//go:build !windows

package apply

import (
	"context"
	"errors"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"io"
	"path/filepath"
	"testing"
)

type syntaxRunner struct{}

func (syntaxRunner) Run(c context.Context, cmd domain.Command, out io.Writer) error {
	return errors.New("invalid fish syntax")
}
func TestFishSyntaxIsPartOfVerification(t *testing.T) {
	root := t.TempDir()
	m := files.Manager{Roots: []string{root}}
	path := filepath.Join(root, ".config/fish/config.fish")
	change := domain.FileChange{Path: path, Decision: domain.Create, Desired: []byte("if\n"), Mode: 0644}
	if _, e := m.Apply(change, filepath.Join(root, "backups")); e != nil {
		t.Fatal(e)
	}
	state, _ := m.Inspect(path)
	x := Executor{Files: m, Runner: syntaxRunner{}}
	okay, e := x.verify(context.Background(), domain.Host{Home: root, Tools: map[string]string{"fish": "/fake/fish"}}, domain.Plan{}, domain.Step{Kind: "file", File: &change, Check: domain.Check{Expected: state.SHA256}}, "")
	if okay || e == nil {
		t.Fatal("invalid shell configuration was verified")
	}
}
