package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func periodicConfig(exec Exec) PeriodicConfig {
	return PeriodicConfig{
		Exec: exec, Schedule: Schedule{Period: time.Hour}, MaxConcurrent: 1,
		FailurePolicy: FailureContinue,
	}
}

func TestPeriodicValidation(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	valid := periodicConfig(func(Context) error { return nil })

	testCases := []struct {
		expected error
		mutate   func(PeriodicConfig) PeriodicConfig
	}{
		{expected: ErrExecEmpty, mutate: func(c PeriodicConfig) PeriodicConfig { c.Exec = nil; return c }},
		{expected: ErrPeriodInvalid, mutate: func(c PeriodicConfig) PeriodicConfig { c.Schedule = Schedule{}; return c }},
		{expected: ErrJitterInvalid, mutate: func(c PeriodicConfig) PeriodicConfig { c.Jitter = time.Hour; return c }},
		{expected: ErrMaxConcurrentInvalid, mutate: func(c PeriodicConfig) PeriodicConfig { c.MaxConcurrent = 0; return c }},
		{expected: ErrFailurePolicyUnsupported, mutate: func(c PeriodicConfig) PeriodicConfig { c.FailurePolicy = 0; return c }},
	}
	for n, tc := range testCases {
		_, err := NewPeriodic(parent, string(rune('a'+n)), tc.mutate(valid), nil)
		assert.ErrorIs(t, err, tc.expected)
	}
}

func TestPeriodicRunsOnAbsoluteBoundary(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	clock := newFakeClock(time.Unix(0, 0).UTC())
	runs := make(chan void, 2)
	periodic, err := newPeriodic(parent, "sweep", periodicConfig(func(Context) error {
		runs <- void{}
		return nil
	}), nil, clock, noJitter)
	require.NoError(t, err)
	assert.Equal(t, time.Unix(0, 0).UTC().Add(time.Hour), periodic.Snapshot().NextFiring)

	clock.BlockUntil(1)
	clock.Advance(59 * time.Minute)
	assert.Empty(t, runs)
	clock.Advance(time.Minute)
	<-runs
	eventually(t, func() bool { return periodic.Snapshot().Runs.Results[ResultSuccess] == 1 })
	assert.Equal(t, time.Unix(0, 0).UTC().Add(2*time.Hour), periodic.Snapshot().NextFiring)
}

func TestPeriodicLimitsConcurrency(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	clock := newFakeClock(time.Unix(0, 0).UTC())
	started := make(chan void, 3)
	release := make(chan void)
	config := periodicConfig(func(Context) error {
		started <- void{}
		<-release
		return nil
	})
	config.MaxConcurrent = 2
	periodic, err := newPeriodic(parent, "sweep", config, nil, clock, noJitter)
	require.NoError(t, err)

	for range 2 {
		clock.BlockUntil(1)
		clock.Advance(time.Hour)
		<-started
	}
	clock.BlockUntil(1)
	clock.Advance(time.Hour)
	eventually(t, func() bool { return periodic.Snapshot().Skips == 1 })
	assert.Equal(t, uint64(2), periodic.Snapshot().Runs.Active)
	close(release)
	eventually(t, func() bool { return periodic.Snapshot().Runs.Results[ResultSuccess] == 2 })
}

func TestPeriodicDoesNotBackfill(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	clock := newFakeClock(time.Unix(0, 0).UTC())
	runs := make(chan void, 2)
	periodic, err := newPeriodic(parent, "sweep", periodicConfig(func(Context) error {
		runs <- void{}
		return nil
	}), nil, clock, noJitter)
	require.NoError(t, err)
	clock.BlockUntil(1)
	clock.Advance(10 * time.Hour)
	<-runs
	eventually(t, func() bool { return periodic.Snapshot().Firings == 1 })
	assert.Equal(t, time.Unix(0, 0).UTC().Add(11*time.Hour), periodic.Snapshot().NextFiring)
	assert.Empty(t, runs)
}

func TestPeriodicFailurePolicies(t *testing.T) {
	expected := errors.New("failed")

	t.Run("continue", func(t *testing.T) {
		parent := New(context.Background())
		defer parent.Cancel(nil)
		clock := newFakeClock(time.Unix(0, 0).UTC())
		periodic, err := newPeriodic(parent, "continue", periodicConfig(func(Context) error {
			return expected
		}), nil, clock, noJitter)
		require.NoError(t, err)
		clock.BlockUntil(1)
		clock.Advance(time.Hour)
		eventually(t, func() bool { return periodic.Snapshot().Runs.Results[ResultFailure] == 1 })
		assert.Equal(t, StateActive, periodic.Snapshot().State)
	})

	t.Run("stop", func(t *testing.T) {
		parent := New(context.Background())
		defer parent.Cancel(nil)
		clock := newFakeClock(time.Unix(0, 0).UTC())
		config := periodicConfig(func(Context) error { return expected })
		config.FailurePolicy = FailureStop
		periodic, err := newPeriodic(parent, "stop", config, nil, clock, noJitter)
		require.NoError(t, err)
		clock.BlockUntil(1)
		clock.Advance(time.Hour)
		assert.NoError(t, periodic.Wait(context.Background()))
		assert.Equal(t, StateStopped, periodic.Snapshot().State)
	})

	t.Run("propagate", func(t *testing.T) {
		parent := New(context.Background())
		clock := newFakeClock(time.Unix(0, 0).UTC())
		config := periodicConfig(func(Context) error { return expected })
		config.FailurePolicy = FailurePropagate
		_, err := newPeriodic(parent, "propagate", config, nil, clock, noJitter)
		require.NoError(t, err)
		clock.BlockUntil(1)
		clock.Advance(time.Hour)
		assert.ErrorIs(t, parent.Wait(context.Background()), expected)
	})
}

func TestPeriodicStopDrainsInvocations(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	clock := newFakeClock(time.Unix(0, 0).UTC())
	started := make(chan void)
	periodic, err := newPeriodic(parent, "sweep", periodicConfig(func(ctx Context) error {
		close(started)
		<-ctx.Done()
		return context.Cause(ctx)
	}), nil, clock, noJitter)
	require.NoError(t, err)
	clock.BlockUntil(1)
	clock.Advance(time.Hour)
	<-started
	periodic.Stop()
	assert.NoError(t, periodic.Wait(context.Background()))
	stats := periodic.Snapshot()
	assert.Equal(t, StateStopped, stats.State)
	assert.Equal(t, uint64(1), stats.Runs.Results[ResultCanceled])
	assert.True(t, stats.NextFiring.IsZero())
}

func TestPeriodicObservationAndJitter(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	clock := newFakeClock(time.Unix(0, 0).UTC())
	observer := &recordingObserver{}
	periodic, err := newPeriodic(parent, "sweep", PeriodicConfig{
		Exec: func(Context) error { return nil }, Schedule: Schedule{Period: time.Hour},
		Jitter: 10 * time.Minute, MaxConcurrent: 1, FailurePolicy: FailureContinue,
	}, observer, clock, func(duration time.Duration) time.Duration { return duration / 2 })
	require.NoError(t, err)
	assert.Equal(t, time.Unix(0, 0).UTC().Add(time.Hour+5*time.Minute), periodic.Snapshot().NextFiring)
	clock.BlockUntil(1)
	clock.Advance(time.Hour + 5*time.Minute)
	eventually(t, func() bool { return len(observer.runEvents()) == 2 })
	schedules := observer.scheduleEvents()
	require.Len(t, schedules, 1)
	assert.Equal(t, ScheduleStarted, schedules[0].Result)
	assert.Zero(t, schedules[0].Active)
	assert.Equal(t, uint64(1), schedules[0].Pending)
	runs := observer.runEvents()
	assert.Equal(t, EventStarted, runs[0].Kind)
	assert.Equal(t, RunReasonScheduled, runs[0].Reason)
	assert.Equal(t, EventFinished, runs[1].Kind)
}

func TestDefaultJitter(t *testing.T) {
	for range 64 {
		jitter := defaultJitter(time.Minute)
		assert.GreaterOrEqual(t, jitter, time.Duration(0))
		assert.Less(t, jitter, time.Minute)
	}
	assert.Zero(t, defaultJitter(0))
}

// a firing that races Stop must not be accounted: the schedule used to count
// the boundary and mark an invocation started before it knew the group would
// refuse to spawn it, leaving Started above the recorded results forever
func TestPeriodicStopDoesNotAccountAFiringThatNeverRan(t *testing.T) {
	for attempt := range 300 {
		root := New(context.Background())
		periodic, err := NewPeriodic(root, "sweep", PeriodicConfig{
			Exec:          func(Context) error { return nil },
			Schedule:      Schedule{Period: time.Millisecond},
			MaxConcurrent: 1,
			FailurePolicy: FailureContinue,
		}, nil)
		require.NoError(t, err)

		time.Sleep(time.Duration(attempt%7) * 200 * time.Microsecond)
		periodic.Stop()
		require.NoError(t, periodic.Wait(context.Background()))

		var (
			stats    = periodic.Snapshot()
			finished uint64
		)
		for _, count := range stats.Runs.Results {
			finished += count
		}
		require.Equal(t, stats.Runs.Started, finished, "started invocations without a result")
		require.Equal(t, uint64(0), stats.Runs.Active)
		require.Equal(t, stats.Firings, finished, "counted firings that never ran")

		root.Cancel(nil)
		require.NoError(t, root.Wait(context.Background()))
	}
}

func TestPeriodicMeasuresExecutionFromInvocationStart(t *testing.T) {
	root := New(context.Background())
	defer root.Cancel(nil)
	group, err := root.Child("sweep")
	require.NoError(t, err)

	var (
		scheduledAt  = time.Unix(0, 0).UTC().Add(time.Hour)
		dispatchedAt = scheduledAt.Add(2 * time.Second)
		startedAt    = dispatchedAt.Add(3 * time.Second)
		clock        = newFakeClock(startedAt)
		observer     = &recordingObserver{}
	)
	periodic := &Periodic{
		group: group, observer: observer, clock: clock,
		config: periodicConfig(func(Context) error {
			clock.Advance(7 * time.Second)
			return nil
		}),
		stats: PeriodicSnapshot{
			Runs: RunStats{Results: map[Result]uint64{}}, Pending: 1, State: StateActive,
		},
	}

	assert.NoError(t, periodic.invoke(context.Background(), scheduledAt, dispatchedAt))
	stats := periodic.Snapshot()
	assert.Equal(t, scheduledAt, stats.LastScheduled)
	assert.Equal(t, dispatchedAt, stats.LastDispatched)
	assert.Equal(t, startedAt, stats.Runs.LastStart)
	assert.Equal(t, 5*time.Second, stats.LastStartDelay)
	assert.Equal(t, 2*time.Second, stats.LastDispatchDelay)
	assert.Equal(t, 7*time.Second, stats.Runs.LastDuration)
	assert.Zero(t, stats.Pending)
	assert.Zero(t, stats.Runs.Active)

	events := observer.runEvents()
	require.Len(t, events, 2)
	assert.Equal(t, scheduledAt, events[0].ScheduledAt)
	assert.Equal(t, dispatchedAt, events[0].DispatchedAt)
	assert.Equal(t, startedAt, events[0].At)
}
