package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPoolNewAndSize(t *testing.T) {
	size := 5
	p := New(size, 1)
	defer p.Close()

	if p.Size() != size {
		t.Errorf("expected size %d, got %d", size, p.Size())
	}
}

func TestPoolRunBasic(t *testing.T) {
	p := New(2, 1)
	defer p.Close()

	expectedVal := "success"
	fn := func(ctx context.Context) (any, error) {
		return expectedVal, nil
	}

	val, err := p.RunContext(context.Background(), fn)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != expectedVal {
		t.Errorf("expected value %v, got %v", expectedVal, val)
	}
}

func TestPoolRunError(t *testing.T) {
	p := New(1, 1)
	defer p.Close()

	expectedErr := errors.New("job failed")
	fn := func(ctx context.Context) (any, error) {
		return nil, expectedErr
	}

	_, err := p.RunContext(context.Background(), fn)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestPoolRunContextCancellation(t *testing.T) {
	p := New(1, 1)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())

	fn := func(ctx context.Context) (any, error) {
		select {
		case <-time.After(1 * time.Second):
			return "should not return", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := p.RunContext(ctx, fn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.canceled error, got %v", err)
	}
}

func TestPoolPanicRecovery(t *testing.T) {
	p := New(1, 1)
	defer p.Close()

	panicMsg := "intentional panic"
	fn := func(ctx context.Context) (any, error) {
		panic(panicMsg)
	}

	_, err := p.RunContext(context.Background(), fn)
	if err == nil {
		t.Errorf("expected an error due to panic, got nil")
	}
	if err.Error() != panicMsg {
		t.Errorf("expected error message '%s', got '%s'", panicMsg, err.Error())
	}
}

func TestPoolClosePreventsNewJobs(t *testing.T) {
	p := New(1, 1)
	p.Close()

	fn := func(ctx context.Context) (any, error) {
		return "should not run", nil
	}

	_, err := p.RunContext(context.Background(), fn)
	if !errors.Is(err, ErrClosing) {
		t.Errorf("expected ErrClosing error, got %v", err)
	}
}

func TestPoolConcurrentRuns(t *testing.T) {
	p := New(4, 1)
	defer p.Close()

	numJobs := 10
	var wg sync.WaitGroup
	wg.Add(numJobs)
	errCh := make(chan error, numJobs)

	for i := range numJobs {
		jobID := i
		go func() {
			defer wg.Done()
			fn := func(ctx context.Context) (any, error) {
				time.Sleep(time.Duration(10+jobID%5) * time.Millisecond)
				return jobID, nil
			}
			val, err := p.RunContext(context.Background(), fn)
			if err != nil {
				errCh <- fmt.Errorf("job %d failed: %w", jobID, err)
				return
			}
			if val.(int) != jobID {
				errCh <- fmt.Errorf("job %d returned wrong value: got %v", jobID, val)
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}
