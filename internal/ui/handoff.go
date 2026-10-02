package ui

import (
	"context"
	"errors"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"io"
	"os"
)

func plainHandoff(ctx context.Context, c domain.Command, in io.Reader, out io.Writer) error {
	f, ok := in.(*os.File)
	if !ok {
		return errors.New("interactive installation requires direct terminal input")
	}
	p := command.Process(ctx, c)
	p.Stdin = f
	p.Stdout = out
	p.Stderr = out
	return p.Run()
}
