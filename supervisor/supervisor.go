package supervisor

import (
	"context"
	"fmt"
	"sync"
)

type (
	void = struct{}

	SupervisorClosing  void
	SupervisorCanceled void
	SupervisorError    struct {
		Err  error
		name string
	}
	SupervisorRunFunc func(ctx context.Context) error
)

func (SupervisorCanceled) Error() string { return "canceled" }
func (e SupervisorError) Error() string {
	return fmt.Sprintf("task %q failed: %s", e.name, e.Err.Error())
}

type task struct {
	name      string
	fn        SupervisorRunFunc
	ctx       context.Context
	cancelCtx context.CancelCauseFunc
	done      chan void
	next      *task
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
	mu     sync.Mutex
	tasks  *task
	active int
	drain  chan void
	errs   chan error
}

func (s *Supervisor) Run(name string, fn SupervisorRunFunc) {
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

	s.tasks = taskNode
	s.active++

	go s.run(ctx, taskNode)
}

func (s *Supervisor) run(ctx context.Context, taskNode *task) {
	defer func() {
		close(taskNode.done)

		s.mu.Lock()
		defer s.mu.Unlock()
		s.active--

		s.cancel(context.Cause(ctx))

		if s.active == 0 {
			s.drain <- void{}
		}
	}()

	err := taskNode.fn(ctx)
	if err != nil {
		s.errs <- SupervisorError{
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
	s.cancel(SupervisorCanceled{})
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
