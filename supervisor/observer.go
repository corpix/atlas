package supervisor

import "time"

type Result uint8

const (
	ResultSuccess Result = iota + 1
	ResultFailure
	ResultPanic
	ResultCanceled
)

func (r Result) String() string {
	switch r {
	case ResultSuccess:
		return "success"
	case ResultFailure:
		return "failure"
	case ResultPanic:
		return "panic"
	case ResultCanceled:
		return "canceled"
	}
	return "unknown"
}

type State uint8

const (
	StateActive State = iota + 1
	StateBackoff
	StateStopping
	StateStopped
)

func (s State) String() string {
	switch s {
	case StateActive:
		return "active"
	case StateBackoff:
		return "backoff"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	}
	return "unknown"
}

type RunReason uint8

const (
	RunReasonInitial RunReason = iota + 1
	RunReasonRestart
	RunReasonScheduled
)

func (r RunReason) String() string {
	switch r {
	case RunReasonInitial:
		return "initial"
	case RunReasonRestart:
		return "restart"
	case RunReasonScheduled:
		return "scheduled"
	}
	return "unknown"
}

type EventKind uint8

const (
	EventStarted EventKind = iota + 1
	EventFinished
)

func (k EventKind) String() string {
	switch k {
	case EventStarted:
		return "started"
	case EventFinished:
		return "finished"
	}
	return "unknown"
}

type ScheduleResult uint8

const (
	ScheduleStarted ScheduleResult = iota + 1
	ScheduleSkippedCapacity
)

func (r ScheduleResult) String() string {
	switch r {
	case ScheduleStarted:
		return "started"
	case ScheduleSkippedCapacity:
		return "skipped-capacity"
	}
	return "unknown"
}

type (
	RunEvent struct {
		At           time.Time
		ScheduledAt  time.Time
		DispatchedAt time.Time
		Err          error
		Name         string
		Path         string
		Duration     time.Duration
		Kind         EventKind
		Reason       RunReason
		Result       Result
	}

	ScheduleEvent struct {
		At            time.Time
		ScheduledAt   time.Time
		Next          time.Time
		Name          string
		Path          string
		Active        uint64
		Pending       uint64
		MaxConcurrent uint64
		Result        ScheduleResult
	}

	// Observer callbacks may be concurrent and must return promptly.
	Observer interface {
		ObserveRun(RunEvent)
		ObserveSchedule(ScheduleEvent)
	}

	nopObserver struct{}

	RunStats struct {
		LastStart    time.Time
		LastFinish   time.Time
		LastSuccess  time.Time
		LastErr      error
		Results      map[Result]uint64
		LastDuration time.Duration
		Started      uint64
		Active       uint64
		LastResult   Result
	}
)

func (nopObserver) ObserveRun(RunEvent)           {}
func (nopObserver) ObserveSchedule(ScheduleEvent) {}

func (s *RunStats) started(at time.Time) {
	s.LastStart = at
	s.Started++
	s.Active++
}

func (s *RunStats) finished(at time.Time, duration time.Duration, result Result, err error) {
	s.LastFinish = at
	s.LastErr = err
	s.LastDuration = duration
	s.LastResult = result
	s.Results[result]++
	s.Active--
	if result == ResultSuccess {
		s.LastSuccess = at
	}
}

func cloneRunStats(stats RunStats) RunStats {
	results := stats.Results
	stats.Results = make(map[Result]uint64, len(stats.Results))
	for result, count := range results {
		stats.Results[result] = count
	}
	return stats
}
