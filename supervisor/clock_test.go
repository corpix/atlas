package supervisor

import (
	"sync"
	"time"
)

type (
	fakeClock struct {
		now     time.Time
		waiters []*fakeTimer
		mu      sync.Mutex
		wake    chan void
	}

	fakeTimer struct {
		deadline time.Time
		ch       chan time.Time
		clock    *fakeClock
	}
)

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Timer(d time.Duration) clockTimer {
	c.mu.Lock()
	defer c.mu.Unlock()

	t := &fakeTimer{
		deadline: c.now.Add(d),
		ch:       make(chan time.Time, 1),
		clock:    c,
	}
	if d <= 0 {
		t.ch <- c.now
		return t
	}

	c.waiters = append(c.waiters, t)
	select {
	case c.wake <- void{}:
	default:
	}

	return t
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()

	for i, w := range t.clock.waiters {
		if w == t {
			t.clock.waiters = append(t.clock.waiters[:i], t.clock.waiters[i+1:]...)
			return
		}
	}
}

// BlockUntil waits until n timers are pending, so a test never advances past a
// deadline the code under test has not registered yet.
func (c *fakeClock) BlockUntil(n int) {
	for {
		c.mu.Lock()
		pending := len(c.waiters)
		c.mu.Unlock()
		if pending >= n {
			return
		}
		<-c.wake
	}
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)

	var (
		fired   []*fakeTimer
		pending []*fakeTimer
	)
	for _, w := range c.waiters {
		if !w.deadline.After(c.now) {
			fired = append(fired, w)
			continue
		}
		pending = append(pending, w)
	}
	c.waiters = pending
	now := c.now
	c.mu.Unlock()

	for _, w := range fired {
		w.ch <- now
	}
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now, wake: make(chan void, 1)}
}

func noJitter(time.Duration) time.Duration { return 0 }
