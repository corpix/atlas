package supervisor

import (
	"context"
	"slices"
	"sync"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type Runner struct {
	Context
	cancel ContextCancel
	tasks  Tasks
	childs []Super
	wg     sync.WaitGroup
	sync.Mutex
}

func (r *Runner) Cancel(cause Cause) {
	r.Lock()
	defer r.Unlock()
	r.cancel(cause)

	for _, child := range r.childs {
		child.Cancel(cause)
	}
}

func (r *Runner) Attach(child Super) {
	r.Lock()
	defer r.Unlock()
	n := len(r.childs)
	r.childs = append(r.childs, child)

	r.run(func(ctx Context) error {
		err := child.Wait(ctx)

		r.Lock()
		defer r.Unlock()
		r.childs = slices.Delete(r.childs, n, n)

		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
}

func (r *Runner) Run(j Job) {
	r.Lock()
	defer r.Unlock()

	r.run(j)
}

func (r *Runner) run(j Job) {
	select {
	case <-r.Done():
		// skip new tasks if we are done
		return
	default:
	}

	task := &Task{
		ctx:  r.Context,
		fn:   j,
		done: make(chan void),
	}
	n := len(r.tasks)
	r.tasks = append(r.tasks, task)

	r.wg.Add(1)
	go r.runTask(n, task)
}

func (r *Runner) runTask(n int, task *Task) {
	defer r.wg.Add(-1)
	defer close(task.done)

	err := task.fn(task.ctx)
	r.Lock()
	defer r.Unlock()
	r.tasks = slices.Delete(r.tasks, n, n)

	if err != nil {
		r.cancel(&Error{
			Err:  err,
			task: task,
		})
	}
}

func (r *Runner) Wait(ctx Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-r.Done():
		err := context.Cause(r)
		r.wg.Wait() // wait for runner to drain
		return err
	}
}

var _ Super = new(Runner)
