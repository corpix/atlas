package supervisor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

type testCanceled struct{}

func (e testCanceled) Error() string { return fmt.Sprintf("%T", e) }

type testTimeout struct{}

func (e testTimeout) Error() string { return fmt.Sprintf("%T", e) }

func TestRunner(t *testing.T) {
	t.Run("basic task execution", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)

		sup.Run(func(ctx Context) error {
			sup.Cancel(nil)
			return nil
		})

		err := sup.Wait(context.Background())
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("tasks error handling", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		expectedErr := errors.New("task failed")
		successDone := make(chan void)

		// success task
		sup.Run(func(ctx Context) error {
			select {
			case <-ctx.Done():
				close(successDone)
			case <-time.After(500 * time.Millisecond):
				return errors.New("success task should not exit before error task")
			}
			return nil
		})

		// error task
		sup.Run(func(ctx Context) error {
			time.Sleep(100 * time.Millisecond)
			return expectedErr
		})

		err := sup.Wait(context.Background())
		if err == nil {
			t.Fatal("expected error from failing task")
		}

		supErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T: %v", err, err)
		} else if !errors.Is(supErr.Err, expectedErr) {
			t.Fatalf("expected wrapped error %v, got %v", expectedErr, supErr.Err)
		}

		select {
		case <-successDone:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("expected success task to exit")
		}
	})

	t.Run("multiple tasks success", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		tasksCount := 100
		completed := make(chan void, tasksCount)

		for range tasksCount {
			sup.Run(func(ctx Context) error {
				defer func() { completed <- void{} }()
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		}

		go func() {
			n := 0
			for range completed {
				n++
				if n == tasksCount {
					sup.Cancel(nil)
					return
				}
			}
		}()

		waitCtx, cancelWait := context.WithTimeout(ctx, 1*time.Second)
		defer cancelWait()
		err := sup.Wait(waitCtx)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRunnerAttach(t *testing.T) {
	t.Run("child supervisor error propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		child := New(ctx)

		expectedErr := errors.New("child task failed")
		child.Run(func(ctx Context) error {
			time.Sleep(100 * time.Millisecond)
			return expectedErr
		})

		parent.Attach(child)

		err := parent.Wait(context.Background())
		if err == nil {
			t.Fatal("expected error from child")
		}

		supErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T: %v", err, err)
		}

		childErr, ok := supErr.Err.(*Error)
		if !ok {
			t.Fatalf("expected child *Error, got %T", supErr.Err)
		}
		if !errors.Is(childErr.Err, expectedErr) {
			t.Fatalf("expected error %v, got %v", expectedErr, childErr.Err)
		}

		if !strings.Contains(supErr.Error(), "task ") {
			t.Fatalf("parent error missing location: %s", supErr.Error())
		}
		if !strings.Contains(childErr.Error(), "task ") {
			t.Fatalf("child error missing location: %s", childErr.Error())
		}
	})

	t.Run("parent cancellation propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		child := New(ctx)

		childTaskCanceled := make(chan void)
		child.Run(func(ctx Context) error {
			select {
			case <-ctx.Done():
				close(childTaskCanceled)
				return context.Cause(ctx)
			case <-time.After(1 * time.Second):
				t.Fatal("expected child task to exit")
				return nil
			}
		})

		parent.Attach(child)

		time.Sleep(100 * time.Millisecond)
		parent.Cancel(testCanceled{})

		select {
		case <-childTaskCanceled:
		case <-time.After(1 * time.Second):
			t.Fatal("child task was not canceled")
		}

		err := parent.Wait(context.Background())
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	})

	t.Run("child cancelation does not propagates to parent if err is nil", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		child := New(ctx)

		child.Run(func(ctx Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})

		parent.Attach(child)
		child.Cancel(nil)

		waitCtx, cancelWait := context.WithTimeoutCause(ctx, 500*time.Millisecond, testTimeout{})
		defer cancelWait()
		err := child.Wait(waitCtx)
		assert.ErrorIs(t, err, context.Canceled)

		err = parent.Wait(waitCtx)
		assert.ErrorIs(t, err, testTimeout{})
	})

	t.Run("child cancelation propagates to parent if err is not nil", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		child := New(ctx)

		child.Run(func(ctx Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})

		parent.Attach(child)
		child.Cancel(testCanceled{})

		waitCtx, cancelWait := context.WithTimeoutCause(ctx, 5*time.Second, testTimeout{})
		defer cancelWait()
		err := child.Wait(waitCtx)
		assert.ErrorIs(t, err, testCanceled{})

		err = parent.Wait(waitCtx)
		assert.ErrorIs(t, err, testCanceled{})
	})
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
