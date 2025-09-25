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
	RunOption func(t *Task)
)

func (Closing) Error() string  { return "closing" }
func (Canceled) Error() string { return "canceled" }
func (e Error) Error() string {
	var name string
	if e.name != "" {
		name = fmt.Sprintf(" %q", e.name)
	}
	return fmt.Sprintf("task%s failed: %s", name, e.Err.Error())
}

func TaskName(name string) RunOption {
	return func(t *Task) {
		t.name = name
	}
}
func TaskOptional() RunOption {
	return func(t *Task) {
		t.optional = true
	}
}

type Task struct {
	ctx      context.Context
	fn       RunFunc
	done     chan void
	next     *Task
	prev     *Task
	name     string
	optional bool
}

type Supervisor struct {
	context.Context
	cancelCtx context.CancelCauseFunc
	tasks     *Task
	mounts    map[*Supervisor]void
	drain     chan void
	errs      chan error
	active    int
	mu        sync.Mutex
}

func (s *Supervisor) Run(fn RunFunc, options ...RunOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := s.Context
	task := &Task{
		fn:   fn,
		ctx:  ctx,
		done: make(chan void),
		next: s.tasks,
	}
	for _, option := range options {
		option(task)
	}

	if s.tasks != nil {
		s.tasks.prev = task
	}
	s.tasks = task
	s.active++

	go s.run(ctx, task)
}

func (s *Supervisor) run(ctx context.Context, task *Task) {
	var err error
	defer func() {
		close(task.done)

		s.mu.Lock()
		defer s.mu.Unlock()

		s.remove(task)
		s.active--

		if !task.optional || err != nil {
			cause := err
			if cause == nil {
				cause = context.Cause(ctx)
			}
			s.cancel(cause)
		}

		if s.active == 0 {
			// fixme: what if we would have only optional tasks?
			// this will panic
			// https://git.tatikoma.dev/corpix/atlas/issues/4
			close(s.drain)
		}
	}()

	err = task.fn(ctx)
	if err != nil {
		s.errs <- Error{
			Err:  err,
			name: task.name,
		}
	}
}

func (s *Supervisor) remove(t *Task) {
	if t.prev != nil {
		t.prev.next = t.next
	} else {
		s.tasks = t.next
	}
	if t.next != nil {
		t.next.prev = t.prev
	}
}

func (s *Supervisor) Mount(super *Supervisor, options ...RunOption) {
	s.mu.Lock()
	_, mounted := s.mounts[super]
	s.mounts[super] = void{}
	s.mu.Unlock()

	if mounted {
		return
	}

	s.Run(func(ctx context.Context) error {
		defer func() {
			super.Cancel()

			s.mu.Lock()
			delete(s.mounts, super)
			s.mu.Unlock()
		}()
		return super.Select(ctx)
	}, options...)
}

func (s *Supervisor) Cancel() {
	s.cancel(Canceled{})
}
func (s *Supervisor) DrainCh() <-chan void   { return s.drain }
func (s *Supervisor) ErrorsCh() <-chan error { return s.errs }
func (s *Supervisor) Wait()                  { <-s.drain }

func (s *Supervisor) cancel(cause error) {
	s.cancelCtx(cause)
}

func (s *Supervisor) Select(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-s.ErrorsCh():
		return err
	case <-s.DrainCh():
		return nil
	}
}

func New(ctx context.Context) *Supervisor {
	innerCtx, cancel := context.WithCancelCause(ctx)
	return &Supervisor{
		Context:   innerCtx,
		cancelCtx: cancel,
		mounts:    map[*Supervisor]void{},
		drain:     make(chan void),
		errs:      make(chan error, 1),
	}
}
