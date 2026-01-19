package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"git.tatikoma.dev/corpix/atlas/errors"
	"git.tatikoma.dev/corpix/atlas/log"
	"git.tatikoma.dev/corpix/atlas/supervisor"
)

type (
	void = struct{}

	Context  = cli.Context
	Super    = supervisor.Super
	Command  = cli.Command
	Commands = []*Command

	Config interface {
		FromFile(path string) error
	}

	Application[C Config] interface {
		Configure(path string) (C, error)
		Signals(...SignalGroup) Signals
		Flags() Flags
		Commands() Commands
		Services() Services
		Notify(Signal)
		Ready() <-chan void
		Watchdog(*cli.Context)
		Init(*Runtime)
		PreRun(*cli.Context) error
		Run(*cli.Context) error
		Exec(args []string) error
		Error(error)
		Close() error
	}

	App[C Config] struct {
		Config C
		self   Application[C]
		*Runtime
		ready       chan void
		readyWg     sync.WaitGroup
		stopTimeout time.Duration
	}

	Service interface {
		Name() string
		Enabled() bool
		Run(context.Context, *sync.WaitGroup) error
		Signal(os.Signal)
		Close() error
	}
	Services = []Service
)

const (
	DefaultStopTimeout = 10 * time.Second
)

func (a *App[C]) Configure(path string) (C, error) {
	log.Ctx(a.Runtime.Super).
		Info().
		Str("config", path).
		Msg("loading config")

	var c C
	typ := reflect.TypeOf((*C)(nil)).Elem()
	if typ.Kind() == reflect.Pointer {
		c = reflect.New(typ.Elem()).Interface().(C)
	}
	err := c.FromFile(path)
	if err != nil {
		return c, errors.Wrapf(err, "failed to load config from %q", path)
	}
	a.Config = c
	return c, nil
}

func (*App[C]) Signals(sgids ...SignalGroup) Signals {
	if len(sgids) == 0 {
		sgids = SignalGroups
	}

	var sigs Signals
	for _, sgid := range sgids {
		switch sgid {
		case SignalGroupStop:
			sigs = append(sigs, syscall.SIGINT, syscall.SIGTERM)
		case SignalGroupNotify:
			sigs = append(sigs, syscall.SIGUSR1)
		}
	}
	return sigs
}

func (*App[C]) Flags() Flags {
	return Flags{
		&PathFlag{
			Name:    FlagConfig,
			Aliases: []string{"c"},
			Usage:   "configuration file path",
			Value:   "config.json",
		},
		&BoolFlag{
			Name:  FlagVerbose,
			Usage: "set info log level",
			Value: false,
		},
		&BoolFlag{
			Name:     FlagDebug,
			Usage:    "set debug log level",
			Value:    false,
			Category: "debug",
		},
	}
}

func (*App[C]) Commands() Commands {
	return nil
}

func (a *App[C]) Services() Services {
	return nil
}

func (a *App[C]) Notify(sig Signal) {
	for _, service := range a.self.Services() {
		service.Signal(sig)
	}
}

func (a *App[C]) Ready() <-chan void {
	return a.ready
}

func (a *App[C]) Init(r *Runtime) {
	r.Cli.Flags = a.self.Flags()
	r.Cli.Commands = a.self.Commands()
	r.Cli.Before = a.self.PreRun
	r.Cli.Action = a.self.Run
}

func (a *App[C]) Watchdog(ctx *cli.Context) {
	sigs := a.self.Signals()
	sgids := GroupSignals(a.self)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, sigs...)

	exit := make(chan error)
	go func() {
		exit <- a.Runtime.Super.Wait(ctx.Context)
	}()

watchdog:
	for {
		select {
		case <-ctx.Done():
			break watchdog
		case err := <-exit:
			log.Error().
				Err(err).
				Msg("supervisor has been shutdown, exiting")
			os.Exit(1)
		case sig := <-sigCh:
			log.Info().
				Str("signal", sig.String()).
				Msg("received signal")
			switch sgids[sig] {
			case SignalGroupNotify:
				a.self.Notify(sig)
			case SignalGroupStop:
				log.Warn().Msg("shutting down supervisor")
				a.Runtime.Super.Cancel(nil)
				break watchdog
			default:
				log.Warn().
					Str("signal", sig.String()).
					Msg("unsupported signal, ignoring")
			}
		}
	}
	log.Warn().
		Str("timeout", a.stopTimeout.String()).
		Msg("shutting down...")

wait:
	for {
		select {
		case err := <-exit:
			log.Error().
				Err(err).
				Msg("supervisor got error, exiting")
			os.Exit(1)
		case sig := <-sigCh:
			log.Warn().
				Msgf("received signal: %v, forcing exit", sig)
			os.Exit(1)
		case <-time.After(a.stopTimeout):
			log.Fatal().
				Err(errors.Errorf("timed out waiting all components to stop, forcing exit")).
				Msg("exiting")
			break wait
		}
	}

	log.Warn().Msg("exiting")
}

func (a *App[C]) PreRun(ctx *cli.Context) error {
	var err error

	verbose := ctx.Bool(FlagVerbose)
	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	err = MetaRegister(FlagVerbose, verbose)
	if err != nil {
		return err
	}

	debug := ctx.Bool(FlagDebug)
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	err = MetaRegister(FlagDebug, debug)
	if err != nil {
		return err
	}

	config := ctx.Path(FlagConfig)
	if config != "" {
		a.Config, err = a.self.Configure(config)
		if err != nil {
			return err
		}
	}
	err = MetaRegister(FlagConfig, config)
	if err != nil {
		return err
	}

	return nil
}

func (a *App[C]) runService(srv Service) error {
	ctx := log.Ctx(a.Super).
		With().
		Str("service", srv.Name()).
		Logger().
		WithContext(a.Super)

	log.Ctx(ctx).Info().Msg("running...")
	defer log.Ctx(ctx).Warn().Msg("stopped")

	defer errors.LogCallErrCtx(ctx, srv.Close, "failed to close service")
	return srv.Run(ctx, &a.readyWg)
}

func (a *App[C]) Run(ctx *cli.Context) error {
	a.Super.Run(func(ctx context.Context) error {
		a.Watcher.Run(ctx)
		return nil
	})

	for _, srv := range a.self.Services() {
		if !srv.Enabled() {
			continue
		}

		srv := srv
		a.readyWg.Add(1)
		a.Super.Run(func(ctx context.Context) error {
			return a.runService(srv)
		})
	}

	a.self.Watchdog(ctx)

	return nil
}

func (a *App[C]) Exec(args []string) error {
	go func() {
		a.readyWg.Wait()
		close(a.ready)
	}()
	return a.Runtime.Run(args)
}

func (a *App[C]) Error(err error) {
	Error(err)
}

func (*App[C]) Close() error {
	return nil
}

func newAppWithRuntime[C Config](r *Runtime) *App[C] {
	return &App[C]{
		Runtime:     r,
		ready:       make(chan void),
		stopTimeout: DefaultStopTimeout,
	}
}

// New creates an App with the provided runtime.
// It is expected that caller invoke Init on self.
func New[C Config](r *Runtime, self Application[C]) *App[C] {
	a := newAppWithRuntime[C](r)
	a.self = self
	return a
}

func Error(err error) {
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}
