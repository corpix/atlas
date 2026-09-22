package supervisor

import "time"

type (
	clock interface {
		Now() time.Time
		Timer(d time.Duration) clockTimer
	}

	clockTimer interface {
		C() <-chan time.Time
		Stop()
	}

	realClock struct{}

	realTimer struct {
		timer *time.Timer
	}
)

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Timer(d time.Duration) clockTimer {
	return &realTimer{timer: time.NewTimer(d)}
}

func (t *realTimer) C() <-chan time.Time { return t.timer.C }

func (t *realTimer) Stop() { t.timer.Stop() }
