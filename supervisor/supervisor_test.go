package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestSupervisor(t *testing.T) {
	timeout := 1 * time.Second
	t.Run("basic task execution", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)

		sup.Run("test-task", func(ctx context.Context) error {
			return nil
		})

		select {
		case <-sup.DrainChan():
		case <-time.After(timeout):
			t.Error("supervisor failed to drain")
		}
	})

	t.Run("task error handling", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		expectedErr := errors.New("task failed")
		successCtxDone := false

		sup.Run("success-task", func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				successCtxDone = true
			case <-time.After(500 * time.Millisecond):
			}
			return nil
		})

		sup.Run("error-task", func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return expectedErr
		})

		select {
		case err := <-sup.ErrorsChan():
			if supErr, ok := err.(Error); !ok {
				t.Errorf("expected SupervisorError, got %T", err)
			} else if supErr.name != "error-task" {
				t.Errorf("expected task name 'error-task', got %q", supErr.name)
			} else if !errors.Is(supErr.Err, expectedErr) {
				t.Errorf("expected error %v, got %v", expectedErr, supErr.Err)
			}
		case <-time.After(timeout):
			t.Error("timeout waiting for error")
		}

		select {
		case <-sup.DrainChan():
			if !successCtxDone {
				t.Errorf("expected 'success-task' context to be canceled")
			}
		case <-time.After(timeout):
			t.Error("timeout waiting for drain")
		}
	})

	t.Run("multiple tasks success", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		tasksCount := 100
		completed := make([]chan void, tasksCount)

		for i := range tasksCount {
			completed[i] = make(chan void)
			sup.Run("task-"+fmt.Sprintf("%d", i), func(ctx context.Context) error {
				defer close(completed[i])
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		}

		for i := range tasksCount {
			select {
			case <-completed[i]:
			case <-time.After(timeout):
				t.Fatalf("task %d failed to complete", i)
			}
		}

		select {
		case <-sup.DrainChan():
		case err := <-sup.ErrorsChan():
			t.Errorf("unexpected error: %v", err)
		case <-time.After(timeout):
			t.Error("supervisor failed to drain")
		}
	})

	t.Run("cancel waits for all tasks to complete", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		taskCount := 5
		allTasksDone := make([]chan struct{}, taskCount)

		for i := range taskCount {
			allTasksDone[i] = make(chan struct{})
			sup.Run(fmt.Sprintf("wait-task-%d", i), func(ctx context.Context) error {
				<-ctx.Done()
				close(allTasksDone[i])
				return nil
			})
		}

		time.Sleep(50 * time.Millisecond)
		sup.Cancel()

		for _, doneChan := range allTasksDone {
			select {
			case <-doneChan:
			}
		}

		select {
		case <-sup.DrainChan():
		case <-time.After(timeout):
			t.Error("supervisor failed to drain after cancellation")
		}
	})

	t.Run("optional task success does not stop supervisor", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		timeout := 500 * time.Millisecond

		mainTaskRunning := make(chan void)
		mainTaskCanceled := make(chan void)

		sup.Run("main-task", func(ctx context.Context) error {
			close(mainTaskRunning)
			<-ctx.Done()
			close(mainTaskCanceled)
			return nil
		})

		sup.Run("optional-task", func(ctx context.Context) error {
			return nil
		}, TaskOptional())

		select {
		case <-mainTaskRunning:
		case <-time.After(timeout):
			t.Fatal("main task did not start")
		}

		time.Sleep(100 * time.Millisecond)

		select {
		case <-sup.DrainChan():
			t.Fatal("supervisor drained prematurely")
		case err := <-sup.ErrorsChan():
			t.Fatalf("supervisor errored unexpectedly: %v", err)
		default:
		}

		sup.Cancel()

		select {
		case <-mainTaskCanceled:
		case <-time.After(timeout):
			t.Fatal("main task was not canceled")
		}

		select {
		case <-sup.DrainChan():
		case <-time.After(timeout):
			t.Error("supervisor failed to drain after cancellation")
		}
	})

	t.Run("optional task failure stops supervisor", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		expectedErr := errors.New("optional task failed")

		sup.Run("optional-task-fail", func(ctx context.Context) error {
			return expectedErr
		}, TaskOptional())

		select {
		case err := <-sup.ErrorsChan():
			if supErr, ok := err.(Error); !ok {
				t.Errorf("expected Error, got %T", err)
			} else if !errors.Is(supErr.Err, expectedErr) {
				t.Errorf("expected error %v, got %v", expectedErr, supErr.Err)
			}
		case <-time.After(timeout):
			t.Error("timeout waiting for optional task error")
		}
	})

	t.Run("multiple optional tasks complete without leaks", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		timeout := 200 * time.Millisecond

		persistentTaskDone := make(chan void)
		sup.Run("persistent-task", func(ctx context.Context) error {
			<-ctx.Done()
			close(persistentTaskDone)
			return nil
		})

		for i := range 50 {
			sup.Run(
				fmt.Sprintf("optional-task-%d", i),
				func(ctx context.Context) error {
					return nil
				},
				TaskOptional(),
			)
		}

		time.Sleep(timeout)

		select {
		case <-sup.DrainChan():
			t.Fatal("supervisor drained prematurely")
		default:
		}

		sup.Cancel()

		select {
		case <-persistentTaskDone:
		case <-time.After(timeout):
			t.Error("persistent task was not canceled")
		}

		select {
		case <-sup.DrainChan():
		case <-time.After(timeout):
			t.Error("supervisor did not drain after cleanup")
		}

		if sup.chain != nil {
			t.Errorf("some tasks leaked: %+v", *sup.chain)
		}
	})
}

func TestSupervisorNested(t *testing.T) {
	timeout := 1 * time.Second

	t.Run("nested supervisor success", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		nestedTaskDone := make(chan void)
		nested.Run("nested-task", func(ctx context.Context) error {
			defer close(nestedTaskDone)
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		parent.Nested("nested-supervisor", nested)

		select {
		case <-nestedTaskDone:
		case <-time.After(timeout):
			t.Error("nested task failed to complete")
		}

		select {
		case <-parent.DrainChan():
		case err := <-parent.ErrorsChan():
			t.Errorf("unexpected error: %v", err)
		case <-time.After(timeout):
			t.Error("parent supervisor failed to drain")
		}
	})

	t.Run("nested supervisor error propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		expectedErr := errors.New("nested task failed")
		nested.Run("error-task", func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return expectedErr
		})

		parent.Nested("nested-supervisor", nested)

		var err error
		select {
		case err = <-parent.ErrorsChan():
		case <-time.After(timeout):
			t.Fatal("timeout waiting for error")
		}

		supErr, ok := err.(Error)
		if !ok {
			t.Fatalf("expected SupervisorError, got %T", err)
		}
		if supErr.name != "nested-supervisor" {
			t.Errorf("expected task name 'nested-supervisor', got %q", supErr.name)
		}

		nestedSupErr, ok := supErr.Err.(Error)
		if !ok {
			t.Fatalf("expected nested SupervisorError, got %T", supErr.Err)
		}
		if nestedSupErr.name != "error-task" {
			t.Errorf("expected nested task name 'error-task', got %q", nestedSupErr.name)
		}
		if !errors.Is(nestedSupErr.Err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, nestedSupErr.Err)
		}

		select {
		case <-parent.DrainChan():
		case <-time.After(timeout):
			t.Error("parent failed to drain after error")
		}
	})

	t.Run("optional nested supervisor success does not stop parent", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)
		timeout := 500 * time.Millisecond

		parentTaskRunning := make(chan void)
		parentTaskCanceled := make(chan void)
		parent.Run("parent-main-task", func(ctx context.Context) error {
			close(parentTaskRunning)
			<-ctx.Done()
			close(parentTaskCanceled)
			return nil
		})

		nestedTaskDone := make(chan void)
		nested.Run("nested-task", func(ctx context.Context) error {
			defer close(nestedTaskDone)
			return nil
		})

		parent.Nested("optional-nested-supervisor", nested, TaskOptional())

		select {
		case <-nestedTaskDone:
		case <-time.After(timeout):
			t.Fatal("nested task did not complete")
		}

		select {
		case <-nested.DrainChan():
		case <-time.After(timeout):
			t.Fatal("nested supervisor did not drain")
		}

		select {
		case <-parent.DrainChan():
			t.Fatal("parent supervisor drained prematurely")
		case err := <-parent.ErrorsChan():
			t.Fatalf("parent supervisor errored unexpectedly: %v", err)
		default:
		}

		parent.Cancel()

		select {
		case <-parentTaskCanceled:
		case <-time.After(timeout):
			t.Fatal("parent main task was not canceled")
		}

		select {
		case <-parent.DrainChan():
		case <-time.After(timeout):
			t.Fatal("parent supervisor failed to drain after cancellation")
		}
	})

	t.Run("parent cancellation propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		nestedTaskCanceled := make(chan void)
		nested.Run("long-task", func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				close(nestedTaskCanceled)
				return context.Cause(ctx)
			case <-time.After(5 * time.Second):
				return nil
			}
		})

		parent.Nested("nested-supervisor", nested)

		time.Sleep(100 * time.Millisecond)
		parent.Cancel()

		select {
		case <-nestedTaskCanceled:
		case <-time.After(timeout):
			t.Error("nested task was not canceled")
		}

		select {
		case <-parent.DrainChan():
		case <-time.After(timeout):
			t.Error("parent failed to drain after cancellation")
		}
	})

	t.Run("concurrent cancellation does not deadlock", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		parent.Run("waiter", func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})

		parent.Nested("nested-supervisor", nested)

		done := make(chan void)
		go func() {
			defer close(done)
			parent.Cancel()
			parent.Cancel()
			parent.Cancel()
		}()

		select {
		case <-done:
			select {
			case <-parent.DrainChan():
			case <-time.After(timeout):
				t.Error("parent failed to drain after cancellation")
			}
		case <-time.After(timeout):
			t.Error("supervisor deadlocked and failed to drain")
		}
	})

	t.Run("deep nesting", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		middle := New(ctx)
		child := New(ctx)

		taskCompleted := make(chan void)
		child.Run("deep-task", func(ctx context.Context) error {
			defer close(taskCompleted)
			time.Sleep(100 * time.Millisecond)
			return nil
		})

		middle.Nested("child-supervisor", child)
		parent.Nested("middle-supervisor", middle)

		select {
		case <-taskCompleted:
		case <-time.After(timeout):
			t.Error("deep task failed to complete")
		}

		select {
		case <-parent.DrainChan():
		case err := <-parent.ErrorsChan():
			t.Errorf("unexpected error: %v", err)
		case <-time.After(timeout):
			t.Error("supervisors failed to drain")
		}
	})

	t.Run("multiple nested supervisors", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)

		nestedCount := 3
		taskCount := 5
		var wg sync.WaitGroup
		wg.Add(nestedCount * taskCount)

		for i := range nestedCount {
			nested := New(ctx)
			for j := range taskCount {
				nested.Run(fmt.Sprintf("nested-%d-task-%d", i, j), func(ctx context.Context) error {
					defer wg.Done()
					time.Sleep(50 * time.Millisecond)
					return nil
				})
			}

			parent.Nested(fmt.Sprintf("nested-supervisor-%d", i), nested)
		}

		done := make(chan void)
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(timeout * 2):
			t.Fatal("not all tasks completed in time")
		}

		select {
		case <-parent.DrainChan():
		case err := <-parent.ErrorsChan():
			t.Errorf("unexpected error: %v", err)
		case <-time.After(timeout):
			t.Error("parent failed to drain")
		}
	})
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
