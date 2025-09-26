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

func TestGroup(t *testing.T) {
	timeout := 1 * time.Second
	t.Run("basic task execution", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)

		sup.Run(func(ctx context.Context) error {
			return nil
		}, TaskName("test-task"))

		select {
		case <-sup.DrainCh():
		case <-time.After(timeout):
			t.Error("supervisor failed to drain")
		}
	})

	t.Run("task error handling", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		expectedErr := errors.New("task failed")
		successCtxDone := false

		sup.Run(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				successCtxDone = true
			case <-time.After(500 * time.Millisecond):
			}
			return nil
		}, TaskName("success-task"))

		sup.Run(func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return expectedErr
		}, TaskName("error-task"))

		select {
		case err := <-sup.ErrorsCh():
			if supErr, ok := err.(Error); !ok {
				t.Errorf("expected %T, got %T", Error{}, err)
			} else if supErr.name != "error-task" {
				t.Errorf("expected task name 'error-task', got %q", supErr.name)
			} else if !errors.Is(supErr.Err, expectedErr) {
				t.Errorf("expected error %v, got %v", expectedErr, supErr.Err)
			}
		case <-time.After(timeout):
			t.Error("timeout waiting for error")
		}

		select {
		case <-sup.DrainCh():
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
			sup.Run(func(ctx context.Context) error {
				defer close(completed[i])
				time.Sleep(50 * time.Millisecond)
				return nil
			}, TaskName("task-"+fmt.Sprintf("%d", i)))
		}

		for i := range tasksCount {
			select {
			case <-completed[i]:
			case <-time.After(timeout):
				t.Fatalf("task %d failed to complete", i)
			}
		}

		select {
		case <-sup.DrainCh():
		case err := <-sup.ErrorsCh():
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
			sup.Run(func(ctx context.Context) error {
				<-ctx.Done()
				close(allTasksDone[i])
				return nil
			}, TaskName(fmt.Sprintf("wait-task-%d", i)))
		}

		time.Sleep(50 * time.Millisecond)
		sup.Cancel()

		for _, doneCh := range allTasksDone {
			<-doneCh
		}

		select {
		case <-sup.DrainCh():
		case <-time.After(timeout):
			t.Error("supervisor failed to drain after cancellation")
		}
	})

	t.Run("weak task success does not stop supervisor", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		timeout := 500 * time.Millisecond

		mainTaskRunning := make(chan void)
		mainTaskCanceled := make(chan void)

		sup.Run(func(ctx context.Context) error {
			close(mainTaskRunning)
			<-ctx.Done()
			close(mainTaskCanceled)
			return nil
		}, TaskName("main-task"))

		sup.Run(func(ctx context.Context) error {
			return nil
		}, TaskName("weak-task"), TaskWeak())

		select {
		case <-mainTaskRunning:
		case <-time.After(timeout):
			t.Fatal("main task did not start")
		}

		time.Sleep(100 * time.Millisecond)

		select {
		case <-sup.DrainCh():
			t.Fatal("supervisor drained prematurely")
		case err := <-sup.ErrorsCh():
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
		case <-sup.DrainCh():
		case <-time.After(timeout):
			t.Error("supervisor failed to drain after cancellation")
		}
	})

	t.Run("weak task failure stops supervisor", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		expectedErr := errors.New("weak task failed")

		sup.Run(func(ctx context.Context) error {
			return expectedErr
		}, TaskName("weak-task-fail"), TaskWeak())

		select {
		case err := <-sup.ErrorsCh():
			if supErr, ok := err.(Error); !ok {
				t.Errorf("expected %T, got %T", Error{}, err)
			} else if !errors.Is(supErr.Err, expectedErr) {
				t.Errorf("expected error %v, got %v", expectedErr, supErr.Err)
			}
		case <-time.After(timeout):
			t.Error("timeout waiting for weak task error")
		}
	})

	t.Run("all tasks weak", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)

		sup.Run(func(ctx context.Context) error {
			return nil
		}, TaskName("weak-task"), TaskWeak())

		select {
		case err := <-sup.ErrorsCh():
			t.Error(err)
		case <-sup.DrainCh():
		case <-time.After(timeout):
			t.Error("timeout")
		}

		// note: https://git.tatikoma.dev/corpix/atlas/issues/4
		sup.Run(func(ctx context.Context) error {
			return nil
		}, TaskName("weak-task"), TaskWeak())

		select {
		case err := <-sup.ErrorsCh():
			t.Error(err)
		case <-sup.DrainCh():
		case <-time.After(timeout):
			t.Error("timeout")
		}
	})

	t.Run("weak task failure stops supervisor", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		expectedErr := errors.New("weak task failed")

		sup.Run(func(ctx context.Context) error {
			return expectedErr
		}, TaskName("weak-task-fail"), TaskWeak())

		select {
		case err := <-sup.ErrorsCh():
			if supErr, ok := err.(Error); !ok {
				t.Errorf("expected %T, got %T", Error{}, err)
			} else if !errors.Is(supErr.Err, expectedErr) {
				t.Errorf("expected error %v, got %v", expectedErr, supErr.Err)
			}
		case <-time.After(timeout):
			t.Error("timeout waiting for weak task error")
		}
	})

	t.Run("multiple weak tasks complete without leaks", func(t *testing.T) {
		ctx := context.Background()
		sup := New(ctx)
		timeout := 200 * time.Millisecond

		persistentTaskDone := make(chan void)
		sup.Run(func(ctx context.Context) error {
			<-ctx.Done()
			close(persistentTaskDone)
			return nil
		}, TaskName("persistent-task"))

		for i := range 50 {
			sup.Run(
				func(ctx context.Context) error { return nil },
				TaskName(fmt.Sprintf("weak-task-%d", i)),
				TaskWeak(),
			)
		}

		time.Sleep(timeout)

		select {
		case <-sup.DrainCh():
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
		case <-sup.DrainCh():
		case <-time.After(timeout):
			t.Error("supervisor did not drain after cleanup")
		}

		if sup.tasks != nil {
			t.Errorf("some tasks leaked: %+v", *sup.tasks)
		}
	})
}

func TestGroupMount(t *testing.T) {
	timeout := 1 * time.Second

	t.Run("mount supervisor success", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		nestedTaskDone := make(chan void)
		nested.Run(func(ctx context.Context) error {
			defer close(nestedTaskDone)
			time.Sleep(100 * time.Millisecond)
			return nil
		}, TaskName("nested-task"))

		parent.Mount(nested, TaskName("nested-supervisor"))

		select {
		case <-nestedTaskDone:
		case <-time.After(timeout):
			t.Error("nested task failed to complete")
		}

		select {
		case <-parent.DrainCh():
		case err := <-parent.ErrorsCh():
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
		nested.Run(func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return expectedErr
		}, TaskName("error-task"))

		parent.Mount(nested, TaskName("nested-supervisor"))

		var err error
		select {
		case err = <-parent.ErrorsCh():
		case <-time.After(timeout):
			t.Fatal("timeout waiting for error")
		}

		supErr, ok := err.(Error)
		if !ok {
			t.Fatalf("expected %T, got %T", Error{}, err)
		}
		if supErr.name != "nested-supervisor" {
			t.Errorf("expected task name 'nested-supervisor', got %q", supErr.name)
		}

		nestedSupErr, ok := supErr.Err.(Error)
		if !ok {
			t.Fatalf("expected nested %T, got %T", Error{}, supErr.Err)
		}
		if nestedSupErr.name != "error-task" {
			t.Errorf("expected nested task name 'error-task', got %q", nestedSupErr.name)
		}
		if !errors.Is(nestedSupErr.Err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, nestedSupErr.Err)
		}

		select {
		case <-parent.DrainCh():
		case <-time.After(timeout):
			t.Error("parent failed to drain after error")
		}
	})

	t.Run("weak nested supervisor success does not stop parent", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)
		timeout := 500 * time.Millisecond

		parentTaskRunning := make(chan void)
		parentTaskCanceled := make(chan void)
		parent.Run(func(ctx context.Context) error {
			close(parentTaskRunning)
			<-ctx.Done()
			close(parentTaskCanceled)
			return nil
		}, TaskName("parent-main-task"))

		nestedTaskDone := make(chan void)
		nested.Run(func(ctx context.Context) error {
			defer close(nestedTaskDone)
			return nil
		}, TaskName("nested-task"))

		parent.Mount(nested, TaskName("weak-nested-supervisor"), TaskWeak())

		select {
		case <-nestedTaskDone:
		case <-time.After(timeout):
			t.Fatal("nested task did not complete")
		}

		select {
		case <-nested.DrainCh():
		case <-time.After(timeout):
			t.Fatal("nested supervisor did not drain")
		}

		select {
		case <-parent.DrainCh():
			t.Fatal("parent supervisor drained prematurely")
		case err := <-parent.ErrorsCh():
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
		case <-parent.DrainCh():
		case <-time.After(timeout):
			t.Fatal("parent supervisor failed to drain after cancellation")
		}
	})

	t.Run("weak nested supervisor error propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)
		timeout := 500 * time.Millisecond
		raise := make(chan void)

		expectedErr := errors.New("weak nested task failed")

		nested.Run(func(ctx context.Context) error {
			<-raise
			return expectedErr
		}, TaskName("failing-task"))

		parent.Mount(nested, TaskName("weak-nested-supervisor"), TaskWeak())

		close(raise)
		select {
		case err := <-parent.ErrorsCh():
			supErr, ok := err.(Error)
			if !ok {
				t.Fatalf("expected %T, got %T", Error{}, err)
			}
			if supErr.name != "weak-nested-supervisor" {
				t.Errorf("expected task name 'weak-nested-supervisor', got %q", supErr.name)
			}

			nestedSupErr, ok := supErr.Err.(Error)
			if !ok {
				t.Fatalf("expected nested %T, got %T", Error{}, supErr.Err)
			}
			if nestedSupErr.name != "failing-task" {
				t.Errorf("expected nested task name 'failing-task', got %q", nestedSupErr.name)
			}
			if !errors.Is(nestedSupErr.Err, expectedErr) {
				t.Errorf("expected error %v, got %v", expectedErr, nestedSupErr.Err)
			}
		case <-time.After(timeout):
			t.Fatal("parent supervisor did not receive error from weak nested supervisor")
		}

		select {
		case <-parent.DrainCh():
		case <-time.After(timeout):
			t.Fatal("parent supervisor failed to drain after weak nested error")
		}
	})

	t.Run("parent cancellation propagation", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		nestedTaskCanceled := make(chan void)
		nested.Run(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				close(nestedTaskCanceled)
				return context.Cause(ctx)
			case <-time.After(5 * time.Second):
				return nil
			}
		}, TaskName("long-task"))

		parent.Mount(nested, TaskName("nested-supervisor"))

		time.Sleep(100 * time.Millisecond)
		parent.Cancel()

		select {
		case <-nestedTaskCanceled:
		case <-time.After(timeout):
			t.Error("nested task was not canceled")
		}

		select {
		case <-parent.DrainCh():
		case <-time.After(timeout):
			t.Error("parent failed to drain after cancellation")
		}
	})

	t.Run("concurrent cancellation does not deadlock", func(t *testing.T) {
		ctx := context.Background()
		parent := New(ctx)
		nested := New(ctx)

		parent.Run(func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}, TaskName("waiter"))

		parent.Mount(nested, TaskName("nested-supervisor"))

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
			case <-parent.DrainCh():
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
		child.Run(func(ctx context.Context) error {
			defer close(taskCompleted)
			time.Sleep(100 * time.Millisecond)
			return nil
		}, TaskName("deep-task"))

		middle.Mount(child, TaskName("child-supervisor"))
		parent.Mount(middle, TaskName("middle-supervisor"))

		select {
		case <-taskCompleted:
		case <-time.After(timeout):
			t.Error("deep task failed to complete")
		}

		select {
		case <-parent.DrainCh():
		case err := <-parent.ErrorsCh():
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
				nested.Run(func(ctx context.Context) error {
					defer wg.Done()
					time.Sleep(50 * time.Millisecond)
					return nil
				}, TaskName(fmt.Sprintf("nested-%d-task-%d", i, j)))
			}

			parent.Mount(nested, TaskName(fmt.Sprintf("nested-supervisor-%d", i)))
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
		case <-parent.DrainCh():
		case err := <-parent.ErrorsCh():
			t.Errorf("unexpected error: %v", err)
		case <-time.After(timeout):
			t.Error("parent failed to drain")
		}
	})

	t.Run("mount to multiple parents", func(t *testing.T) {
		ctx := context.Background()
		parent1 := New(ctx)
		parent2 := New(ctx)
		child := New(ctx)

		childTaskCanceled := make(chan void)
		child.Run(func(ctx context.Context) error {
			<-ctx.Done()
			close(childTaskCanceled)
			return nil
		}, TaskName("long-running-task"))

		// note: if child or some of parents canceled/errored
		// then both parents and child should be drain

		parent1.Mount(child)
		parent2.Mount(child)

		time.Sleep(100 * time.Millisecond)
		parent1.Cancel()

		select {
		case <-childTaskCanceled:
		case <-time.After(timeout):
			t.Fatal("child task was not canceled")
		}

		select {
		case <-child.DrainCh():
		case <-time.After(timeout):
			t.Fatal("child supervisor did not drain")
		}

		select {
		case <-parent1.DrainCh():
		case <-time.After(timeout):
			t.Fatal("parent1 did not drain")
		}

		select {
		case <-parent2.DrainCh():
		case err := <-parent2.ErrorsCh():
			t.Fatalf("parent2 errored unexpectedly: %v", err)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("parent2 did not drain")
		}
	})
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
