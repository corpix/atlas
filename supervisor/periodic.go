package supervisor

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"git.tatikoma.dev/corpix/atlas/errors"
)

var (
	ErrFailurePolicyUnsupported = errors.New("failure policy is not supported")
	ErrJitterInvalid            = errors.New("jitter must be in [0, period)")
	ErrMaxConcurrentInvalid     = errors.New("max concurrent must be positive")
)

type FailurePolicy uint8

const (
	FailurePropagate FailurePolicy = iota + 1
	FailureStop
	FailureContinue
)

func (p FailurePolicy) String() string {
	switch p {
	case FailurePropagate:
		return "propagate"
	case FailureStop:
		return "stop"
	case FailureContinue:
		return "continue"
	}
	return "unknown"
}

func (p FailurePolicy) Validate() error {
	switch p {
	case FailurePropagate, FailureStop, FailureContinue:
		return nil
	}
	return errors.Wrapf(ErrFailurePolicyUnsupported, "got %d", uint8(p))
}

// PeriodicConfig defines an epoch-aligned managed schedule.
type PeriodicConfig struct {
	Exec          Exec
	Schedule      Schedule
	Jitter        time.Duration
	MaxConcurrent uint64
	FailurePolicy FailurePolicy
}

func (c PeriodicConfig) Validate() error {
	if c.Exec == nil {
		return ErrExecEmpty
	}
	err := c.Schedule.Validate()
	if err != nil {
		return err
	}
	if c.Jitter < 0 || c.Jitter >= c.Schedule.Period {
		return errors.Wrapf(ErrJitterInvalid, "got %s with period %s", c.Jitter, c.Schedule.Period)
	}
	if c.MaxConcurrent == 0 {
		return ErrMaxConcurrentInvalid
	}
	return c.FailurePolicy.Validate()
}

type PeriodicSnapshot struct {
	LastScheduled     time.Time
	LastDispatched    time.Time
	NextFiring        time.Time
	LastStartDelay    time.Duration
	LastDispatchDelay time.Duration
	Runs              RunStats
	Firings           uint64
	Skips             uint64
	Pending           uint64
	State             State
}

// Periodic is an independently stoppable scheduled execution attached to a group.
type Periodic struct {
	observer Observer
	clock    clock
	group    *Group
	jitter   func(time.Duration) time.Duration
	stats    PeriodicSnapshot
	config   PeriodicConfig
	sequence uint64
	mu       sync.Mutex
}

func (p *Periodic) Name() string { return p.group.Name() }

func (p *Periodic) Path() string { return p.group.Path() }

func (p *Periodic) Snapshot() PeriodicSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := p.stats
	stats.Runs = cloneRunStats(stats.Runs)
	return stats
}

func (p *Periodic) Stop() {
	p.mu.Lock()
	if p.stats.State != StateStopped {
		p.stats.State = StateStopping
		p.stats.NextFiring = time.Time{}
	}
	p.mu.Unlock()
	p.group.Cancel(nil)
}

func (p *Periodic) Wait(ctx Context) error {
	err := p.group.Wait(ctx)
	p.mu.Lock()
	if p.group.Err() != nil {
		p.stats.State = StateStopped
		p.stats.NextFiring = time.Time{}
	}
	p.mu.Unlock()
	return err
}

func (p *Periodic) run(ctx Context) error {
	defer p.markStopping()
	for {
		p.mu.Lock()
		fireAt := p.stats.NextFiring
		p.mu.Unlock()

		delay := max(fireAt.Sub(p.clock.Now()), 0)
		timer := p.clock.Timer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C():
			timer.Stop()
			// Stop() zeroes NextFiring, so the next delay elapses at once and
			// the select may prefer the timer over an already done context
			if ctx.Err() != nil {
				return nil
			}
			err := p.fire(ctx, fireAt)
			if err != nil {
				return err
			}
		}
	}
}

func (p *Periodic) fire(ctx Context, scheduledAt time.Time) error {
	dispatchedAt := p.clock.Now()
	p.mu.Lock()
	p.stats.NextFiring = p.next(dispatchedAt)
	event := ScheduleEvent{
		At: dispatchedAt, ScheduledAt: scheduledAt, Next: p.stats.NextFiring,
		Name: p.Name(), Path: p.Path(), Active: p.stats.Runs.Active,
		Pending: p.stats.Pending, MaxConcurrent: p.config.MaxConcurrent,
	}
	if p.stats.Runs.Active+p.stats.Pending >= p.config.MaxConcurrent {
		p.stats.Skips++
		event.Result = ScheduleSkippedCapacity
		p.mu.Unlock()
		p.observer.ObserveSchedule(event)
		return nil
	}

	p.sequence++
	sequence := p.sequence
	p.stats.Firings++
	p.stats.Pending++
	event.Pending = p.stats.Pending
	event.Result = ScheduleStarted
	p.mu.Unlock()

	err := p.group.Go(fmt.Sprintf("run#%d", sequence), func(runCtx Context) error {
		return p.invoke(runCtx, scheduledAt, dispatchedAt)
	})
	if err != nil {
		p.mu.Lock()
		p.stats.Firings--
		p.stats.Pending--
		p.mu.Unlock()
		if errors.Is(err, ErrStopped) || ctx.Err() != nil {
			return nil
		}
		return err
	}

	p.observer.ObserveSchedule(event)
	return nil
}

func (p *Periodic) invoke(ctx Context, scheduledAt time.Time, dispatchedAt time.Time) error {
	started := p.clock.Now()
	p.mu.Lock()
	p.stats.Pending--
	p.stats.LastScheduled = scheduledAt
	p.stats.LastDispatched = dispatchedAt
	p.stats.LastStartDelay = max(started.Sub(scheduledAt), 0)
	p.stats.LastDispatchDelay = max(dispatchedAt.Sub(scheduledAt), 0)
	p.stats.Runs.started(started)
	p.mu.Unlock()
	p.observer.ObserveRun(RunEvent{
		At: started, ScheduledAt: scheduledAt, DispatchedAt: dispatchedAt,
		Name: p.Name(), Path: p.Path(), Kind: EventStarted, Reason: RunReasonScheduled,
	})

	result, err := invoke(ctx, p.config.Exec)
	finished := p.clock.Now()
	duration := finished.Sub(started)
	p.mu.Lock()
	p.stats.Runs.finished(finished, duration, result, err)
	p.markStoppedLocked()
	p.mu.Unlock()
	p.observer.ObserveRun(RunEvent{
		At: finished, ScheduledAt: scheduledAt, DispatchedAt: dispatchedAt,
		Err: err, Name: p.Name(), Path: p.Path(), Duration: duration,
		Kind: EventFinished, Reason: RunReasonScheduled, Result: result,
	})

	if result == ResultSuccess || result == ResultCanceled {
		return nil
	}
	switch p.config.FailurePolicy {
	case FailureContinue:
		return nil
	case FailureStop:
		p.Stop()
		return nil
	case FailurePropagate:
		return err
	}
	return nil
}

func (p *Periodic) next(now time.Time) time.Time {
	return p.config.Schedule.Next(now).Add(p.jitter(p.config.Jitter))
}

func (p *Periodic) markStopping() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stats.Runs.Active == 0 && p.stats.Pending == 0 {
		p.stats.State = StateStopped
	} else {
		p.stats.State = StateStopping
	}
	p.stats.NextFiring = time.Time{}
}

func (p *Periodic) markStoppedLocked() {
	if p.stats.State == StateStopping && p.stats.Runs.Active == 0 && p.stats.Pending == 0 {
		p.stats.State = StateStopped
	}
}

func defaultJitter(duration time.Duration) time.Duration {
	if duration <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(duration)))
}

func newPeriodic(
	parent *Group,
	name string,
	config PeriodicConfig,
	observer Observer,
	clk clock,
	jitter func(time.Duration) time.Duration,
) (*Periodic, error) {
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
	p := &Periodic{
		group: group, observer: observer, clock: clk, jitter: jitter, config: config,
		stats: PeriodicSnapshot{Runs: RunStats{Results: map[Result]uint64{}}, State: StateActive},
	}
	p.stats.NextFiring = p.next(clk.Now())
	err = group.Go("schedule", p.run)
	if err != nil {
		group.Cancel(err)
		return nil, err
	}
	return p, nil
}

// NewPeriodic validates and immediately starts a periodic execution below parent.
func NewPeriodic(parent *Group, name string, config PeriodicConfig, observer Observer) (*Periodic, error) {
	return newPeriodic(parent, name, config, observer, realClock{}, defaultJitter)
}
