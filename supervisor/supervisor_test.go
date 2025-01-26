package supervisor

import (
	"fmt"
	"context"
	"errors"
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

		for i := 0; i < tasksCount; i++ {
			completed[i] = make(chan struct{})
			sup.Run("task-"+fmt.Sprintf("%d", i), func(ctx context.Context) error {
				defer close(completed[i])
				time.Sleep(50 * time.Millisecond)
				return nil
			})
		}

		for i := 0; i < tasksCount; i++ {
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
