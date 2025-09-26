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

//

func TaskName(name string) RunOption {
	return func(t *Task) {
		t.name = name
	}
}

func TaskWeak() RunOption {
	return func(t *Task) {
		t.weak = true
	}
}

//

type Task struct {
	ctx  context.Context
	fn   RunFunc
	done chan void
	next *Task
	prev *Task
	name string // optional label for task
	weak bool   // weak tasks doesn't make whole group to exit if they exit without error
}

type Group struct {
	context.Context
	cancelCtx context.CancelCauseFunc
	tasks     *Task
	mounts    map[*Group]void
	wg        sync.WaitGroup
	errs      chan error
	mu        sync.Mutex
}

func (s *Group) Run(fn RunFunc, options ...RunOption) {
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

	s.wg.Add(1)
	go s.run(ctx, task)
}

func (s *Group) run(ctx context.Context, task *Task) {
	var err error
	defer func() {
		close(task.done)

		s.mu.Lock()
		defer s.mu.Unlock()

		s.remove(task)
		s.wg.Add(-1)

		if !task.weak || err != nil {
			cause := err
			if cause == nil {
				cause = context.Cause(ctx)
			}
			s.cancel(cause)
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

func (s *Group) remove(t *Task) {
	if t.prev != nil {
		t.prev.next = t.next
	} else {
		s.tasks = t.next
	}
	if t.next != nil {
		t.next.prev = t.prev
	}
}

func (s *Group) Mount(super *Group, options ...RunOption) {
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

func (s *Group) Cancel() {
	s.cancel(Canceled{})
}
func (s *Group) DrainCh() <-chan void {
	drainCh := make(chan void)
	go func() {
		s.wg.Wait()
		close(drainCh)
	}()
	return drainCh
}

func (s *Group) ErrorsCh() <-chan error {
	return s.errs
}

func (s *Group) Wait() {
	s.wg.Wait()
}

func (s *Group) cancel(cause error) {
	s.cancelCtx(cause)
}

func (s *Group) Select(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-s.ErrorsCh():
		return err
	case <-s.DrainCh():
		return nil
	}
}

func New(ctx context.Context) *Group {
	innerCtx, cancel := context.WithCancelCause(ctx)
	return &Group{
		Context:   innerCtx,
		cancelCtx: cancel,
		mounts:    map[*Group]void{},
		errs:      make(chan error, 1),
	}
}
