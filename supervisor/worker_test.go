package supervisor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingObserver struct {
	runs      []RunEvent
	schedules []ScheduleEvent
	mu        sync.Mutex
}

func (o *recordingObserver) ObserveRun(event RunEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.runs = append(o.runs, event)
}

func (o *recordingObserver) ObserveSchedule(event ScheduleEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.schedules = append(o.schedules, event)
}

func (o *recordingObserver) runEvents() []RunEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]RunEvent(nil), o.runs...)
}

func (o *recordingObserver) scheduleEvents() []ScheduleEvent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]ScheduleEvent(nil), o.schedules...)
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	require.Eventually(t, condition, 2*time.Second, time.Millisecond)
}

func TestWorkerValidation(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)

	_, err := NewWorker(parent, "empty", WorkerConfig{ExitPolicy: ExitStop}, nil)
	assert.ErrorIs(t, err, ErrExecEmpty)
	_, err = NewWorker(parent, "policy", WorkerConfig{Exec: func(Context) error { return nil }}, nil)
	assert.ErrorIs(t, err, ErrExitPolicyUnsupported)
	_, err = NewWorker(parent, "backoff", WorkerConfig{
		Exec: func(Context) error { return nil }, ExitPolicy: ExitStop,
		Backoff: Backoff{Initial: time.Minute, Maximum: time.Second},
	}, nil)
	assert.ErrorIs(t, err, ErrBackoffInvalid)
}

func TestWorkerStopsWithoutFailingParent(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	expected := errors.New("failed")

	worker, err := NewWorker(parent, "worker", WorkerConfig{
		Exec: func(Context) error { return expected }, ExitPolicy: ExitStop,
	}, nil)
	require.NoError(t, err)
	assert.NoError(t, worker.Wait(context.Background()))
	stats := worker.Snapshot()
	assert.Equal(t, StateStopped, stats.State)
	assert.Equal(t, uint64(1), stats.Runs.Results[ResultFailure])
	assert.ErrorIs(t, stats.Runs.LastErr, expected)

	waitCtx, cancel := context.WithTimeoutCause(context.Background(), 20*time.Millisecond, testTimeout{})
	defer cancel()
	assert.ErrorIs(t, parent.Wait(waitCtx), testTimeout{})
}

func TestWorkerPropagatesFailure(t *testing.T) {
	parent := New(context.Background())
	expected := errors.New("failed")
	worker, err := NewWorker(parent, "worker", WorkerConfig{
		Exec: func(Context) error { return expected }, ExitPolicy: ExitPropagateFailure,
	}, nil)
	require.NoError(t, err)
	assert.ErrorIs(t, worker.Wait(context.Background()), expected)
	assert.ErrorIs(t, parent.Wait(context.Background()), expected)
}

func TestWorkerRestartsWithBackoff(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	clock := newFakeClock(time.Unix(0, 0).UTC())
	attempts := make(chan int, 2)
	var attempt int
	worker, err := newWorker(parent, "worker", WorkerConfig{
		Exec: func(Context) error {
			attempt++
			attempts <- attempt
			if attempt == 1 {
				return errors.New("retry")
			}
			return nil
		},
		ExitPolicy: ExitRestartOnFailure,
		Backoff:    Backoff{Initial: time.Second, Maximum: time.Second},
	}, nil, clock)
	require.NoError(t, err)
	assert.Equal(t, 1, <-attempts)
	clock.BlockUntil(1)
	assert.Equal(t, StateBackoff, worker.Snapshot().State)
	clock.Advance(time.Second)
	assert.Equal(t, 2, <-attempts)
	assert.NoError(t, worker.Wait(context.Background()))
	assert.Equal(t, uint64(1), worker.Snapshot().Restarts)
}

func TestWorkerStopCancelsInvocation(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	started := make(chan void)
	worker, err := NewWorker(parent, "worker", WorkerConfig{
		Exec: func(ctx Context) error {
			close(started)
			<-ctx.Done()
			return context.Cause(ctx)
		},
		ExitPolicy: ExitRestartAlways,
	}, nil)
	require.NoError(t, err)
	<-started
	worker.Stop()
	assert.NoError(t, worker.Wait(context.Background()))
	stats := worker.Snapshot()
	assert.Equal(t, StateStopped, stats.State)
	assert.Equal(t, uint64(1), stats.Runs.Results[ResultCanceled])
	assert.Zero(t, stats.Restarts)
}

func TestWorkerPanicAndObservation(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	observer := &recordingObserver{}
	worker, err := NewWorker(parent, "worker", WorkerConfig{
		Exec: func(Context) error { panic("boom") }, ExitPolicy: ExitStop,
	}, observer)
	require.NoError(t, err)
	assert.NoError(t, worker.Wait(context.Background()))
	events := observer.runEvents()
	require.Len(t, events, 2)
	assert.Equal(t, EventStarted, events[0].Kind)
	assert.Equal(t, RunReasonInitial, events[0].Reason)
	assert.Equal(t, "worker", events[0].Path)
	assert.Equal(t, EventFinished, events[1].Kind)
	assert.Equal(t, ResultPanic, events[1].Result)
	assert.True(t, strings.Contains(events[1].Err.Error(), "goroutine"))
}

func TestWorkerSnapshotIsIndependent(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	worker, err := NewWorker(parent, "worker", WorkerConfig{
		Exec: func(Context) error { return nil }, ExitPolicy: ExitStop,
	}, nil)
	require.NoError(t, err)
	assert.NoError(t, worker.Wait(context.Background()))
	stats := worker.Snapshot()
	stats.Runs.Results[ResultSuccess] = 99
	assert.Equal(t, uint64(1), worker.Snapshot().Runs.Results[ResultSuccess])
}

func TestWorkerWaitHonorsCallerContext(t *testing.T) {
	parent := New(context.Background())
	defer parent.Cancel(nil)
	started := make(chan void)
	worker, err := NewWorker(parent, "worker", WorkerConfig{
		Exec: func(ctx Context) error {
			close(started)
			<-ctx.Done()
			return context.Cause(ctx)
		},
		ExitPolicy: ExitStop,
	}, nil)
	require.NoError(t, err)
	<-started
	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, worker.Wait(waitCtx), context.Canceled)
	assert.Equal(t, StateActive, worker.Snapshot().State)
}

func TestBackoff(t *testing.T) {
	backoff := (Backoff{Initial: time.Second, Maximum: 8 * time.Second}).withDefaults()
	assert.Equal(t, time.Second, backoff.delay(1))
	assert.Equal(t, 2*time.Second, backoff.delay(2))
	assert.Equal(t, 8*time.Second, backoff.delay(8))
	defaults := (Backoff{}).withDefaults()
	assert.Equal(t, DefaultBackoffInitial, defaults.Initial)
	assert.Equal(t, DefaultBackoffMaximum, defaults.Maximum)
}
