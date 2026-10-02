package apply

import (
	"context"
	"golden-gate-setup/internal/domain"
	"io"
)

type runFn func(context.Context, domain.Command, io.Writer) error

func (f runFn) Run(c context.Context, d domain.Command, w io.Writer) error { return f(c, d, w) }
