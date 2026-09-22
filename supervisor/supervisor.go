package supervisor

import (
	"context"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type (
	void = struct{}

	Context       = context.Context
	ContextCancel = context.CancelCauseFunc
	Cause         error

	Exec func(ctx Context) error
)

var (
	ErrNameEmpty    = errors.New("name is empty")
	ErrNameConflict = errors.New("name conflicts with an existing entry")
	ErrStopped      = errors.New("group is stopped")
)

func New(ctx Context) *Group {
	return newGroup(ctx, nil, "")
}
