package supervisor

import (
	"context"
)

type (
	void = struct{}

	Context       = context.Context
	ContextCancel = context.CancelCauseFunc
	Cause         error

	Super interface {
		Run(Job)
		Cancel(cause Cause)
		Attach(child Super)
		Wait(ctx Context) error
	}
)

func New(ctx context.Context) *Runner {
	innerCtx, cancel := context.WithCancelCause(ctx)
	return &Runner{
		Context: innerCtx,
		cancel:  cancel,
	}
}
