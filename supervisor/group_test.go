package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

type testCanceled struct{}

func (e testCanceled) Error() string { return fmt.Sprintf("%T", e) }

type testTimeout struct{}

func (e testTimeout) Error() string { return fmt.Sprintf("%T", e) }

func TestGroupGo(t *testing.T) {
	t.Run("basic task execution", func(t *testing.T) {
		g := New(context.Background())

		assert.NoError(t, g.Go("cancel", func(ctx Context) error {
			g.Cancel(nil)
			return nil
		}))

		assert.NoError(t, g.Wait(context.Background()))
	})

	t.Run("tasks error handling", func(t *testing.T) {
		g := New(context.Background())
		expectedErr := errors.New("task failed")
		successDone := make(chan void)

		assert.NoError(t, g.Go("success", func(ctx Context) error {
			select {
			case <-ctx.Done():
				close(successDone)
			case <-time.After(500 * time.Millisecond):
				return errors.New("success task should not exit before error task")
			}
			return nil
		}))
		assert.NoError(t, g.Go("failure", func(ctx Context) error {
			time.Sleep(10 * time.Millisecond)
			return expectedErr
		}))

		err := g.Wait(context.Background())

		var groupErr *Error
		if assert.ErrorAs(t, err, &groupErr) {
			assert.ErrorIs(t, groupErr.Err, expectedErr)
			assert.Contains(t, groupErr.Error(), "failure")
		}

		select {
		case <-successDone:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected success task to exit")
		}
	})

	t.Run("multiple tasks success", func(t *testing.T) {
		g := New(context.Background())
		tasksCount := 100
		completed := make(chan void, tasksCount)

		for n := range tasksCount {
			assert.NoError(t, g.Go(fmt.Sprintf("task-%d", n), func(ctx Context) error {
				defer func() { completed <- void{} }()
				time.Sleep(10 * time.Millisecond)
				return nil
			}))
		}

		go func() {
			n := 0
			for range completed {
				n++
				if n == tasksCount {
					g.Cancel(nil)
					return
				}
			}
		}()

		waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelWait()
		assert.NoError(t, g.Wait(waitCtx))
	})

	t.Run("a task returning a cancellation does not cancel the group", func(t *testing.T) {
		g := New(context.Background())
		defer g.Cancel(nil)

		assert.NoError(t, g.Go("canceled", func(ctx Context) error {
			return context.Canceled
		}))

		waitCtx, cancelWait := context.WithTimeoutCause(
			context.Background(), 100*time.Millisecond, testTimeout{},
		)
		defer cancelWait()
		assert.ErrorIs(t, g.Wait(waitCtx), testTimeout{})
	})

	t.Run("wait does not return when every task returns nil", func(t *testing.T) {
		g := New(context.Background())
		defer g.Cancel(nil)

		assert.NoError(t, g.Go("done", func(ctx Context) error { return nil }))

		waitCtx, cancelWait := context.WithTimeoutCause(
			context.Background(), 100*time.Millisecond, testTimeout{},
		)
		defer cancelWait()
		assert.ErrorIs(t, g.Wait(waitCtx), testTimeout{})
	})
}

func TestGroupNames(t *testing.T) {
	t.Run("empty name is rejected", func(t *testing.T) {
		g := New(context.Background())
		defer g.Cancel(nil)

		assert.ErrorIs(t, g.Go("", func(ctx Context) error { return nil }), ErrNameEmpty)

		_, err := g.Child("")
		assert.ErrorIs(t, err, ErrNameEmpty)
	})

	t.Run("duplicate sibling name is rejected", func(t *testing.T) {
		g := New(context.Background())
		defer g.Cancel(nil)

		block := func(ctx Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		}

		assert.NoError(t, g.Go("worker", block))
		assert.ErrorIs(t, g.Go("worker", block), ErrNameConflict)

		_, err := g.Child("worker")
		assert.ErrorIs(t, err, ErrNameConflict)

		_, err = g.Child("sub")
		assert.NoError(t, err)
		assert.ErrorIs(t, g.Go("sub", block), ErrNameConflict)
	})

	t.Run("a name is freed once its task drains", func(t *testing.T) {
		g := New(context.Background())
		defer g.Cancel(nil)

		done := make(chan void)
		assert.NoError(t, g.Go("worker", func(ctx Context) error {
			close(done)
			return nil
		}))
		<-done

		assert.Eventually(t, func() bool {
			return g.Go("worker", func(ctx Context) error { return nil }) == nil
		}, time.Second, time.Millisecond)
	})

	t.Run("tracked entries are removed on drain", func(t *testing.T) {
		g := New(context.Background())

		for n := range 32 {
			assert.NoError(t, g.Go(fmt.Sprintf("task-%d", n), func(ctx Context) error {
				return nil
			}))
			child, err := g.Child(fmt.Sprintf("child-%d", n))
			assert.NoError(t, err)
			child.Cancel(nil)
		}

		assert.Eventually(t, func() bool {
			g.mu.Lock()
			defer g.mu.Unlock()
			return len(g.tasks) == 0 && len(g.childs) == 0
		}, time.Second, time.Millisecond)

		g.Cancel(nil)
		assert.NoError(t, g.Wait(context.Background()))
	})

	t.Run("a stopped group accepts no work", func(t *testing.T) {
		g := New(context.Background())
		g.Cancel(nil)
		assert.NoError(t, g.Wait(context.Background()))

		assert.ErrorIs(t, g.Go("worker", func(ctx Context) error { return nil }), ErrStopped)

		_, err := g.Child("sub")
		assert.ErrorIs(t, err, ErrStopped)
	})

	t.Run("path is the chain of names", func(t *testing.T) {
		root := New(context.Background())
		defer root.Cancel(nil)

		child, err := root.Child("telegram")
		assert.NoError(t, err)
		grandchild, err := child.Child("bot")
		assert.NoError(t, err)

		assert.Equal(t, "", root.Path())
		assert.Equal(t, "telegram", child.Path())
		assert.Equal(t, "bot", grandchild.Name())
		assert.Equal(t, "telegram/bot", grandchild.Path())
	})
}

func TestGroupChild(t *testing.T) {
	t.Run("child error cancels the parent", func(t *testing.T) {
		parent := New(context.Background())
		child, err := parent.Child("child")
		assert.NoError(t, err)

		expectedErr := errors.New("child task failed")
		assert.NoError(t, child.Go("worker", func(ctx Context) error {
			time.Sleep(10 * time.Millisecond)
			return expectedErr
		}))

		err = parent.Wait(context.Background())

		var groupErr *Error
		if assert.ErrorAs(t, err, &groupErr) {
			assert.ErrorIs(t, groupErr.Err, expectedErr)
			assert.Contains(t, groupErr.Error(), "child/worker")
		}
	})

	t.Run("parent cancellation reaches a grandchild with the cause", func(t *testing.T) {
		root := New(context.Background())
		child, err := root.Child("child")
		assert.NoError(t, err)
		grandchild, err := child.Child("grandchild")
		assert.NoError(t, err)

		canceled := make(chan void)
		assert.NoError(t, grandchild.Go("worker", func(ctx Context) error {
			<-ctx.Done()
			close(canceled)
			return context.Cause(ctx)
		}))

		root.Cancel(testCanceled{})

		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("grandchild task was not canceled")
		}

		assert.ErrorIs(t, root.Wait(context.Background()), testCanceled{})
		assert.ErrorIs(t, context.Cause(grandchild), testCanceled{})
	})

	t.Run("a cancelled child does not cancel the parent", func(t *testing.T) {
		parent := New(context.Background())
		defer parent.Cancel(nil)

		child, err := parent.Child("child")
		assert.NoError(t, err)
		assert.NoError(t, child.Go("worker", func(ctx Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		}))

		child.Cancel(nil)
		assert.NoError(t, child.Wait(context.Background()))

		waitCtx, cancelWait := context.WithTimeoutCause(
			context.Background(), 100*time.Millisecond, testTimeout{},
		)
		defer cancelWait()
		assert.ErrorIs(t, parent.Wait(waitCtx), testTimeout{})
	})

	t.Run("a child cancelled with a cause cancels the parent", func(t *testing.T) {
		parent := New(context.Background())
		child, err := parent.Child("child")
		assert.NoError(t, err)

		assert.NoError(t, child.Go("worker", func(ctx Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		}))
		child.Cancel(testCanceled{})

		waitCtx, cancelWait := context.WithTimeoutCause(
			context.Background(), 5*time.Second, testTimeout{},
		)
		defer cancelWait()
		assert.ErrorIs(t, child.Wait(waitCtx), testCanceled{})
		assert.ErrorIs(t, parent.Wait(waitCtx), testCanceled{})
	})

	t.Run("the parent drains its children before wait returns", func(t *testing.T) {
		root := New(context.Background())
		child, err := root.Child("child")
		assert.NoError(t, err)

		var drained atomic.Bool
		assert.NoError(t, child.Go("slow", func(ctx Context) error {
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond)
			drained.Store(true)
			return context.Cause(ctx)
		}))

		root.Cancel(nil)
		assert.NoError(t, root.Wait(context.Background()))
		assert.True(t, drained.Load(), "root wait returned before the child drained")
	})
}

func TestGroupWaitPrefersAnAlreadyCanceledCaller(t *testing.T) {
	g := New(context.Background())
	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	g.Cancel(nil)

	assert.ErrorIs(t, g.Wait(waitCtx), context.Canceled)
	assert.ErrorIs(t, context.Cause(g), context.Canceled)
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestGroupChildFailureIsNotHiddenByADrain(t *testing.T) {
	root := New(context.Background())
	child, err := root.Child("child")
	assert.NoError(t, err)

	var (
		expectedErr = errors.New("child task failed")
		release     = make(chan void)
		drained     atomic.Bool
	)

	assert.NoError(t, child.Go("stubborn", func(ctx Context) error {
		<-release
		drained.Store(true)
		return nil
	}))
	assert.NoError(t, child.Go("failing", func(ctx Context) error {
		return expectedErr
	}))

	select {
	case <-root.Done():
	case <-time.After(time.Second):
		t.Fatal("the child failure never reached the root")
	}
	assert.False(t, drained.Load(), "the root was cancelled only after the child drained")

	close(release)

	var groupErr *Error
	if assert.ErrorAs(t, root.Wait(context.Background()), &groupErr) {
		assert.ErrorIs(t, groupErr.Err, expectedErr)
	}
	assert.True(t, drained.Load(), "root wait returned before the child drained")
}
