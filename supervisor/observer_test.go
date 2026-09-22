package supervisor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// the string forms are consumed as metric label values, so a typo ships silently
func TestEnumStrings(t *testing.T) {
	testCases := []struct {
		value    interface{ String() string }
		name     string
		expected string
	}{
		{name: "result success", value: ResultSuccess, expected: "success"},
		{name: "result failure", value: ResultFailure, expected: "failure"},
		{name: "result panic", value: ResultPanic, expected: "panic"},
		{name: "result canceled", value: ResultCanceled, expected: "canceled"},
		{name: "result unknown", value: Result(0), expected: "unknown"},
		{name: "state active", value: StateActive, expected: "active"},
		{name: "state backoff", value: StateBackoff, expected: "backoff"},
		{name: "state stopping", value: StateStopping, expected: "stopping"},
		{name: "state stopped", value: StateStopped, expected: "stopped"},
		{name: "state unknown", value: State(0), expected: "unknown"},
		{name: "reason initial", value: RunReasonInitial, expected: "initial"},
		{name: "reason restart", value: RunReasonRestart, expected: "restart"},
		{name: "reason scheduled", value: RunReasonScheduled, expected: "scheduled"},
		{name: "reason unknown", value: RunReason(0), expected: "unknown"},
		{name: "event started", value: EventStarted, expected: "started"},
		{name: "event finished", value: EventFinished, expected: "finished"},
		{name: "event unknown", value: EventKind(0), expected: "unknown"},
		{name: "schedule started", value: ScheduleStarted, expected: "started"},
		{name: "schedule skipped", value: ScheduleSkippedCapacity, expected: "skipped-capacity"},
		{name: "schedule unknown", value: ScheduleResult(0), expected: "unknown"},
		{name: "exit propagate", value: ExitPropagateFailure, expected: "propagate-failure"},
		{name: "exit stop", value: ExitStop, expected: "stop"},
		{name: "exit restart on failure", value: ExitRestartOnFailure, expected: "restart-on-failure"},
		{name: "exit restart always", value: ExitRestartAlways, expected: "restart-always"},
		{name: "exit unknown", value: ExitPolicy(0), expected: "unknown"},
		{name: "failure propagate", value: FailurePropagate, expected: "propagate"},
		{name: "failure stop", value: FailureStop, expected: "stop"},
		{name: "failure continue", value: FailureContinue, expected: "continue"},
		{name: "failure unknown", value: FailurePolicy(0), expected: "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.value.String())
		})
	}
}

func TestRealClock(t *testing.T) {
	c := realClock{}

	t.Run("now advances", func(t *testing.T) {
		before := c.Now()
		assert.False(t, c.Now().Before(before))
	})

	t.Run("a timer fires and can be stopped", func(t *testing.T) {
		timer := c.Timer(time.Millisecond)
		select {
		case <-timer.C():
		case <-time.After(time.Second):
			t.Fatal("real timer did not fire")
		}

		stopped := c.Timer(time.Hour)
		stopped.Stop()
	})
}
