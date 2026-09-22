package supervisor

import (
	"context"
	"sync"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type Group struct {
	Context
	cancel ContextCancel
	parent *Group
	tasks  map[string]*task
	childs map[string]*Group
	name   string
	wg     sync.WaitGroup
	mu     sync.Mutex
}

func (g *Group) Name() string { return g.name }

func (g *Group) Path() string {
	if g.parent == nil {
		return g.name
	}
	parent := g.parent.Path()
	if parent == "" {
		return g.name
	}
	return parent + "/" + g.name
}

func (g *Group) Go(name string, exec Exec) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	err := g.reserve(name)
	if err != nil {
		return err
	}

	t := &task{name: name, exec: exec}
	g.tasks[name] = t
	g.wg.Add(1)
	go g.runTask(t)

	return nil
}

func (g *Group) Child(name string) (*Group, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	err := g.reserve(name)
	if err != nil {
		return nil, err
	}

	child := newGroup(g.Context, g, name)
	g.childs[name] = child
	g.wg.Add(1)
	go g.runChild(child)

	return child, nil
}

func (g *Group) Cancel(cause Cause) { g.cancel(cause) }

// Wait drains a stopped group, returns nil for clean cancellation, and keeps an idle group running.
func (g *Group) Wait(ctx Context) error {
	err := context.Cause(ctx)
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-g.Done():
		err = context.Cause(g)
		g.wg.Wait()
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
}

func (g *Group) reserve(name string) error {
	if name == "" {
		return errors.Wrapf(ErrNameEmpty, "group %q", g.Path())
	}

	select {
	case <-g.Done():
		return errors.Wrapf(ErrStopped, "group %q, name %q", g.Path(), name)
	default:
	}

	_, taken := g.tasks[name]
	if !taken {
		_, taken = g.childs[name]
	}
	if taken {
		return errors.Wrapf(ErrNameConflict, "group %q, name %q", g.Path(), name)
	}

	return nil
}

func (g *Group) runTask(t *task) {
	defer g.wg.Done()

	err := t.exec(g.Context)

	g.mu.Lock()
	delete(g.tasks, t.name)
	g.mu.Unlock()

	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	g.cancel(&Error{Err: err, group: g.Path(), name: t.name})
}

func (g *Group) runChild(child *Group) {
	defer g.wg.Done()

	// propagate before draining: a child task that ignores cancellation must
	// not hide the failure that cancelled its group
	<-child.Done()
	err := context.Cause(child)
	if err != nil && !errors.Is(err, context.Canceled) {
		g.cancel(err)
	}

	// the child descends from our context, so a background wait drains it fully
	_ = child.Wait(context.Background())

	g.mu.Lock()
	delete(g.childs, child.name)
	g.mu.Unlock()
}

func newGroup(ctx Context, parent *Group, name string) *Group {
	innerCtx, cancel := context.WithCancelCause(ctx)
	return &Group{
		Context: innerCtx,
		cancel:  cancel,
		parent:  parent,
		tasks:   map[string]*task{},
		childs:  map[string]*Group{},
		name:    name,
	}
}
