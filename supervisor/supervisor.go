package supervisor

import (
	"context"
	"fmt"
	"sync"
)

type (
	void = struct{}

	Closing  void
	Canceled void
	Error    struct {
		Err  error
		name string
	}
	RunFunc   func(ctx context.Context) error
	RunOption func(t *task)
)

func (Canceled) Error() string { return "canceled" }
func (e Error) Error() string {
	return fmt.Sprintf("task %q failed: %s", e.name, e.Err.Error())
}

func TaskOptional() RunOption {
	return func(t *task) {
		t.optional = true
	}
}

type task struct {
	ctx      context.Context
	fn       RunFunc
	done     chan void
	next     *task
	prev     *task
	name     string
	optional bool
}

type Supervisor struct {
	context.Context
	cancelCtx context.CancelCauseFunc
	chain     *task
	drain     chan void
	errs      chan error
	active    int
	mu        sync.Mutex
}

func (s *Supervisor) Run(name string, fn RunFunc, options ...RunOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := s.Context
	taskNode := &task{
		name: name,
		fn:   fn,
		ctx:  ctx,
		done: make(chan void),
		next: s.chain,
	}
	for _, option := range options {
		option(taskNode)
	}

	if s.chain != nil {
		s.chain.prev = taskNode
	}
	s.chain = taskNode
	s.active++

	go s.run(ctx, taskNode)
}

func (s *Supervisor) run(ctx context.Context, taskNode *task) {
	var err error
	defer func() {
		close(taskNode.done)

		s.mu.Lock()
		defer s.mu.Unlock()

		s.remove(taskNode)
		s.active--

		if !taskNode.optional || err != nil {
			cause := err
			if cause == nil {
				cause = context.Cause(ctx)
			}
			s.cancel(cause)
		}

		if s.active == 0 {
			close(s.drain)
		}
	}()

	err = taskNode.fn(ctx)
	if err != nil {
		s.errs <- Error{
			Err:  err,
			name: taskNode.name,
		}
	}
}

func (s *Supervisor) remove(t *task) {
	if t.prev != nil {
		t.prev.next = t.next
	} else {
		s.chain = t.next
	}
	if t.next != nil {
		t.next.prev = t.prev
	}
}

func (s *Supervisor) Nested(name string, nested *Supervisor, options ...RunOption) {
	s.Run(name, func(ctx context.Context) error {
		defer nested.Cancel()
		return nested.Select(ctx)
	}, options...)
}

func (s *Supervisor) Cancel() {
	s.cancel(Canceled{})
}
func (s *Supervisor) DrainChan() <-chan void   { return s.drain }
func (s *Supervisor) ErrorsChan() <-chan error { return s.errs }
func (s *Supervisor) Wait()                    { <-s.drain }

func (s *Supervisor) cancel(cause error) {
	s.cancelCtx(cause)
}

func (s *Supervisor) Select(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-s.ErrorsChan():
		return err
	case <-s.DrainChan():
		return nil
	}
}

func New(ctx context.Context) *Supervisor {
	innerCtx, cancel := context.WithCancelCause(ctx)
	return &Supervisor{
		Context:   innerCtx,
		cancelCtx: cancel,
		drain:     make(chan void),
		errs:      make(chan error, 1),
	}
}
