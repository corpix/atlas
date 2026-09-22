package supervisor

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"git.tatikoma.dev/corpix/atlas/errors"
)

const (
	DefaultBackoffInitial = time.Second
	DefaultBackoffMaximum = time.Minute
)

var (
	ErrExecEmpty             = errors.New("exec is not set")
	ErrExitPolicyUnsupported = errors.New("exit policy is not supported")
	ErrBackoffInvalid        = errors.New("backoff is invalid")
)

type ExitPolicy uint8

const (
	ExitPropagateFailure ExitPolicy = iota + 1
	ExitStop
	ExitRestartOnFailure
	ExitRestartAlways
)

func (p ExitPolicy) String() string {
	switch p {
	case ExitPropagateFailure:
		return "propagate-failure"
	case ExitStop:
		return "stop"
	case ExitRestartOnFailure:
		return "restart-on-failure"
	case ExitRestartAlways:
		return "restart-always"
	}
	return "unknown"
}

func (p ExitPolicy) Validate() error {
	switch p {
	case ExitPropagateFailure, ExitStop, ExitRestartOnFailure, ExitRestartAlways:
		return nil
	}
	return errors.Wrapf(ErrExitPolicyUnsupported, "got %d", uint8(p))
}

func (p ExitPolicy) restarts(result Result) bool {
	switch p {
	case ExitRestartAlways:
		return result != ResultCanceled
	case ExitRestartOnFailure:
		return result == ResultFailure || result == ResultPanic
	}
	return false
}

type Backoff struct {
	Initial time.Duration
	Maximum time.Duration
}

func (b Backoff) Validate() error {
	if b.Initial < 0 || b.Maximum < 0 {
		return errors.Wrapf(ErrBackoffInvalid, "got initial %s, maximum %s", b.Initial, b.Maximum)
	}
	if b.Initial > 0 && b.Maximum > 0 && b.Maximum < b.Initial {
		return errors.Wrapf(ErrBackoffInvalid, "maximum %s is below initial %s", b.Maximum, b.Initial)
	}
	return nil
}

func (b Backoff) withDefaults() Backoff {
	if b.Initial <= 0 {
		b.Initial = DefaultBackoffInitial
	}
	if b.Maximum <= 0 {
		b.Maximum = DefaultBackoffMaximum
	}
	if b.Maximum < b.Initial {
		b.Maximum = b.Initial
	}
	return b
}

func (b Backoff) delay(attempt int) time.Duration {
	d := b.Initial
	for range attempt - 1 {
		if d >= b.Maximum/2 {
			return b.Maximum
		}
		d *= 2
	}
	return d
}

// WorkerConfig defines one immediately started managed execution.
type WorkerConfig struct {
	Exec       Exec
	Backoff    Backoff
	ExitPolicy ExitPolicy
}

func (c WorkerConfig) Validate() error {
	if c.Exec == nil {
		return ErrExecEmpty
	}
	err := c.ExitPolicy.Validate()
	if err != nil {
		return err
	}
	return c.Backoff.Validate()
}

type WorkerSnapshot struct {
	Runs     RunStats
	Restarts uint64
	State    State
}

// Worker is an independently stoppable managed execution attached to a group.
type Worker struct {
	group    *Group
	observer Observer
	clock    clock
	config   WorkerConfig
	stats    WorkerSnapshot
	mu       sync.Mutex
}

func (w *Worker) Name() string { return w.group.Name() }

func (w *Worker) Path() string { return w.group.Path() }

func (w *Worker) Snapshot() WorkerSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()

	stats := w.stats
	stats.Runs = cloneRunStats(stats.Runs)
	return stats
}

func (w *Worker) Stop() {
	w.mu.Lock()
	if w.stats.State != StateStopped {
		w.stats.State = StateStopping
	}
	w.mu.Unlock()
	w.group.Cancel(nil)
}

func (w *Worker) Wait(ctx Context) error {
	return w.group.Wait(ctx)
}

func (w *Worker) run(ctx Context) (runErr error) {
	defer func() {
		w.mu.Lock()
		w.stats.State = StateStopped
		w.mu.Unlock()
		if runErr == nil {
			w.group.Cancel(nil)
		}
	}()

	reason := RunReasonInitial
	for attempt := 1; ; attempt++ {
		result, err := w.invoke(ctx, reason)
		if result == ResultCanceled || !w.config.ExitPolicy.restarts(result) {
			if err != nil && w.config.ExitPolicy == ExitPropagateFailure {
				return err
			}
			return nil
		}

		w.mu.Lock()
		w.stats.Restarts++
		w.stats.State = StateBackoff
		w.mu.Unlock()

		if !sleep(ctx, w.clock, w.config.Backoff.delay(attempt)) {
			return nil
		}

		w.mu.Lock()
		w.stats.State = StateActive
		w.mu.Unlock()
		reason = RunReasonRestart
	}
}

func (w *Worker) invoke(ctx Context, reason RunReason) (Result, error) {
	started := w.clock.Now()
	w.mu.Lock()
	w.stats.Runs.started(started)
	w.mu.Unlock()
	w.observer.ObserveRun(RunEvent{
		At: started, Name: w.Name(), Path: w.Path(), Kind: EventStarted, Reason: reason,
	})

	result, err := invoke(ctx, w.config.Exec)
	finished := w.clock.Now()
	duration := finished.Sub(started)
	w.mu.Lock()
	w.stats.Runs.finished(finished, duration, result, err)
	w.mu.Unlock()
	w.observer.ObserveRun(RunEvent{
		At: finished, Err: err, Name: w.Name(), Path: w.Path(), Duration: duration,
		Kind: EventFinished, Reason: reason, Result: result,
	})
	return result, err
}

func newWorker(parent *Group, name string, config WorkerConfig, observer Observer, clk clock) (*Worker, error) {
	err := config.Validate()
	if err != nil {
		return nil, err
	}
	group, err := parent.Child(name)
	if err != nil {
		return nil, err
	}
	if observer == nil {
		observer = nopObserver{}
	}
	config.Backoff = config.Backoff.withDefaults()
	w := &Worker{
		group: group, observer: observer, clock: clk, config: config,
		stats: WorkerSnapshot{Runs: RunStats{Results: map[Result]uint64{}}, State: StateActive},
	}
	err = group.Go("worker", w.run)
	if err != nil {
		group.Cancel(err)
		return nil, err
	}
	return w, nil
}

// NewWorker validates and immediately starts a worker below parent.
func NewWorker(parent *Group, name string, config WorkerConfig, observer Observer) (*Worker, error) {
	return newWorker(parent, name, config, observer, realClock{})
}

func invoke(ctx Context, exec Exec) (result Result, err error) {
	defer func() {
		value := recover()
		if value != nil {
			result = ResultPanic
			err = fmt.Errorf("panic: %v\n%s", value, debug.Stack())
		}
	}()

	err = exec(ctx)
	switch {
	case err == nil:
		return ResultSuccess, nil
	case errors.Is(err, context.Canceled):
		return ResultCanceled, err
	default:
		return ResultFailure, err
	}
}

func sleep(ctx Context, clk clock, duration time.Duration) bool {
	timer := clk.Timer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C():
		return true
	}
}
