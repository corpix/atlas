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
	ctx       context.Context
	fn        RunFunc
	cancelCtx context.CancelCauseFunc
	done      chan void
	next      *task
	name      string
	optional  bool
}

func (c *task) cancel(cause error) {
	c.cancelCtx(cause)
	<-c.done
	if c.next != nil {
		c.next.cancel(cause)
	}
}

type Supervisor struct {
	context.Context
	tasks  *task
	drain  chan void
	errs   chan error
	active int
	mu     sync.Mutex
}

func (s *Supervisor) Run(name string, fn RunFunc, options ...RunOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancelCause(s.Context)
	taskNode := &task{
		name:      name,
		fn:        fn,
		ctx:       ctx,
		cancelCtx: cancel,
		done:      make(chan void),
		next:      s.tasks,
	}
	for _, option := range options {
		option(taskNode)
	}

	s.tasks = taskNode
	s.active++

	go s.run(ctx, taskNode)
}

func (s *Supervisor) run(ctx context.Context, taskNode *task) {
	var err error
	defer func() {
		close(taskNode.done)

		s.mu.Lock()
		defer s.mu.Unlock()
		s.active--

		if !taskNode.optional || err != nil {
			cause := err
			if cause == nil {
				cause = context.Cause(ctx)
			}
			s.cancel(cause)
		}

		if s.active == 0 {
			s.drain <- void{}
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

func (s *Supervisor) Nested(name string, nested *Supervisor) {
	s.Run(name, func(ctx context.Context) error {
		go func() {
			<-ctx.Done()
			nested.Cancel()
		}()

		return nested.Select(ctx)
	})
}

func (s *Supervisor) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel(Canceled{})
}
func (s *Supervisor) DrainChan() <-chan void   { return s.drain }
func (s *Supervisor) ErrorsChan() <-chan error { return s.errs }
func (s *Supervisor) Wait()                    { <-s.drain }

func (s *Supervisor) cancel(cause error) {
	s.tasks.cancel(cause)
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
	return &Supervisor{
		Context: ctx,
		drain:   make(chan void),
		errs:    make(chan error),
	}
}
