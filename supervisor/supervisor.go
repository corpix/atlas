package supevisor

import (
	"context"
	"fmt"
	"sync"
)

type (
	void = struct{}

	SupervisorReturned void
	SupervisorClosing  void
	SupervisorCanceled void
	SupervisorError    struct {
		Err  error
		name string
	}
	SupervisorRunFunc func(ctx context.Context) error
)

func (SupervisorReturned) Error() string { return "returned" }
func (SupervisorCanceled) Error() string { return "canceled" }
func (e SupervisorError) Error() string {
	return fmt.Sprintf("task %q failed: %s", e.name, e.Err.Error())
}

type supervisorCancelNode struct {
	fn   context.CancelCauseFunc
	done chan void
	next *supervisorCancelNode
}

func (c *supervisorCancelNode) cancel(cause error) {
	if c != nil {
		c.fn(cause)
		<-c.done
		c.next.cancel(cause)
	}
}

type Supervisor struct {
	context.Context
	mu          sync.Mutex
	running     int
	cancelChain *supervisorCancelNode
	drain       chan void
	errs        chan error
}

func (s *Supervisor) DrainChan() <-chan void   { return s.drain }
func (s *Supervisor) ErrorsChan() <-chan error { return s.errs }

func (s *Supervisor) Run(name string, fn SupervisorRunFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.Done():
	default:
		s.running++
		ctx, cancel := context.WithCancelCause(context.Background())
		cancelNode := &supervisorCancelNode{
			fn:   cancel,
			done: make(chan void),
			next: s.cancelChain,
		}
		s.cancelChain = cancelNode

		go func() {
			var err error
			defer func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.running--

				close(cancelNode.done)
				if s.running <= 0 {
					close(s.drain)
				}
			}()
			defer func() {
				if context.Cause(ctx) == nil {
					s.cancel(SupervisorReturned{})
				}
			}()

			err = fn(ctx)
			if err != nil {
				s.errs <- SupervisorError{
					Err:  err,
					name: name,
				}
			}
		}()
	}
}

func (s *Supervisor) cancel(cause error) {
	s.mu.Lock()
	cancelNode := s.cancelChain
	s.mu.Unlock()
	go cancelNode.cancel(cause)
}

func (s *Supervisor) Cancel() {
	s.cancel(SupervisorCanceled{})
}

func NewSupervisor(ctx context.Context) *Supervisor {
	s := &Supervisor{
		Context: ctx,
		drain:   make(chan void),
		errs:    make(chan error),
	}
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cancelChain.cancel(context.Cause(ctx))
	}()

	return s
}
