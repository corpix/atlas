package app

import (
	"context"

	"github.com/urfave/cli/v2"

	"git.tatikoma.dev/corpix/atlas/errors"
	"git.tatikoma.dev/corpix/atlas/supervisor"
	"git.tatikoma.dev/corpix/atlas/watcher"
)

type (
	Runtime struct {
		Super   Super
		Cli     *cli.App
		Watcher *watcher.Watcher
	}
)

func NewRuntime(ctx context.Context) (*Runtime, error) {
	w, err := watcher.New()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create file watcher")
	}

	r := &Runtime{
		Cli:     cli.NewApp(),
		Super:   supervisor.New(ctx),
		Watcher: w,
	}

	return r, nil
}

func (r *Runtime) Run(args []string) error {
	return r.Cli.RunContext(r.Super, args)
}
