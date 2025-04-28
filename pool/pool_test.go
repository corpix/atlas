package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPoolNewAndConfig(t *testing.T) {
	cfg := DefaultConfig
	cfg.Size = 5
	p := New(cfg)
	defer p.Close()

	if p.Size() != cfg.Size {
		t.Errorf("expected size %d, got %d", cfg.Size, p.Size())
	}
	if p.Backlog() != cfg.Backlog {
		t.Errorf("expected backlog %d, got %d", cfg.Backlog, p.Backlog())
	}
}

func TestPoolRunBasic(t *testing.T) {
	cfg := DefaultConfig
	cfg.Size = 2
	p := New(cfg)
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
	cfg := DefaultConfig
	cfg.Size = 1
	p := New(cfg)
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
	cfg := DefaultConfig
	cfg.Size = 1
	p := New(cfg)
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
	cfg := DefaultConfig
	cfg.Size = 1
	p := New(cfg)
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
	cfg := DefaultConfig
	cfg.Size = 1
	p := New(cfg)
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
	cfg := DefaultConfig
	cfg.Size = 4
	p := New(cfg)
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

func TestPoolJobsChExecutesJob(t *testing.T) {
	cfg := DefaultConfig
	cfg.Size = 1
	p := New(cfg)
	defer p.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	job := p.JobWithContext(context.Background(), func(ctx context.Context) (any, error) {
		wg.Done()
		return nil, nil
	})

	select {
	case p.JobsCh() <- job:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("timed out sending job to JobsCh")
	}

	done := make(chan void)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for job execution signal")
	}
}

func TestPoolJobsChFullBacklog(t *testing.T) {
	cfg := DefaultConfig
	cfg.Size = 1
	cfg.Backlog = 0
	p := New(cfg)
	defer p.Close()

	waitCh := make(chan void)
	blockingFn := func(ctx context.Context) (any, error) {
		<-waitCh
		return "done", nil
	}

	job1 := p.JobWithContext(context.Background(), blockingFn)
	select {
	case p.JobsCh() <- job1:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("timed out sending first job to JobsCh")
	}

	job2 := p.JobWithContext(
		context.Background(),
		func(ctx context.Context) (any, error) {
			return "second", nil
		},
	)
	select {
	case p.JobsCh() <- job2:
		close(waitCh)
		t.Fatal("sending second job should have blocked due to zero backlog")
	case <-time.After(20 * time.Millisecond):
	}
	close(waitCh)
}

func TestPoolJobsChCancellationPreventsCompletionSignal(t *testing.T) {
	cfg := DefaultConfig
	cfg.Size = 1
	p := New(cfg)
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	completionSignal := make(chan void, 1)

	job := p.JobWithContext(ctx, func(ctx context.Context) (any, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			completionSignal <- void{}
			return "completed", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	select {
	case p.JobsCh() <- job:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("timed out sending job to JobsCh")
	}

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	select {
	case <-completionSignal:
		t.Fatal("completion signal received despite context cancellation")
	default:
	}

	time.Sleep(250 * time.Millisecond)
}
