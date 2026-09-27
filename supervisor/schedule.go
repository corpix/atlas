package supervisor

import (
	"time"

	"git.tatikoma.dev/corpix/atlas/errors"
)

var (
	ErrPeriodInvalid = errors.New("schedule period must be positive")
	ErrOffsetInvalid = errors.New("schedule offset must be in [0, period)")
	ErrJitterInvalid = errors.New("schedule jitter must be in [0, period)")
)

type Schedule struct {
	Period time.Duration
	Offset time.Duration
	Jitter time.Duration
}

// Next returns the smallest floor(t/Period)*Period+Offset strictly greater than
// t, anchored to the unix epoch in UTC.
func (s Schedule) Next(t time.Time) time.Time {
	var (
		period = int64(s.Period)
		now    = t.UTC().UnixNano()
	)

	rem := now % period
	if rem < 0 {
		rem += period
	}

	next := now - rem + int64(s.Offset)
	if next <= now {
		next += period
	}

	return time.Unix(0, next).UTC()
}

func (s Schedule) Validate() error {
	if s.Period <= 0 {
		return errors.Wrapf(ErrPeriodInvalid, "got %s", s.Period)
	}
	if s.Offset < 0 || s.Offset >= s.Period {
		return errors.Wrapf(ErrOffsetInvalid, "got %s with period %s", s.Offset, s.Period)
	}
	if s.Jitter < 0 || s.Jitter >= s.Period {
		return errors.Wrapf(ErrJitterInvalid, "got %s with period %s", s.Jitter, s.Period)
	}
	return nil
}
