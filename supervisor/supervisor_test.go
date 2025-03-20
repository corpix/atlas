package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
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
			if supErr, ok := err.(SupervisorError); !ok {
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
		completed := make([]chan struct{}, tasksCount)

		for i := range tasksCount {
			completed[i] = make(chan struct{})
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
}

func TestSupervisorNested(t *testing.T) {
	timeout := 1 * time.Second

	t.Run("nested supervisor success", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		nestedTaskDone := make(chan struct{})
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

		supErr, ok := err.(SupervisorError)
		if !ok {
			t.Fatalf("expected SupervisorError, got %T", err)
		}
		if supErr.name != "nested-supervisor" {
			t.Errorf("expected task name 'nested-supervisor', got %q", supErr.name)
		}

		nestedSupErr, ok := supErr.Err.(SupervisorError)
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

	t.Run("parent cancellation propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		nestedTaskCanceled := make(chan struct{})
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

	t.Run("deep nesting", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		middle := New(ctx)
		child := New(ctx)

		taskCompleted := make(chan struct{})
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

		done := make(chan struct{})
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
