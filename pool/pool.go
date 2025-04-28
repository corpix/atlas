package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	ErrClosing = fmt.Errorf("pool is closing")
)

type (
	Pool struct {
		closeCh  chan void
		jobs     chan *Job
		sem      chan void
		wg       sync.WaitGroup
		size     int
		isClosed atomic.Uint32
	}
	Job struct {
		Ctx      context.Context
		Fn       Workload
		ResultCh chan Result
	}
	Workload func(ctx context.Context) (any, error)

	Result struct {
		Val any
		Err error
	}

	void = struct{}
)

func (p *Pool) workersRun() {
	p.wg.Add(p.size)
	for range p.size {
		go p.worker()
	}
}

func (p *Pool) workerRecovery(r any) error {
	switch v := r.(type) {
	case error:
		return v
	default:
		return fmt.Errorf("%v", r)
	}
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.closeCh:
			return
		case p.sem <- void{}:
			select {
			case <-p.closeCh:
				return
			case job := <-p.jobs:
				p.workerRunJob(job)
				<-p.sem
			}
		}
	}
}

func (p *Pool) workerRunJob(job *Job) {
	defer func() {
		if r := recover(); r != nil {
			select {
			case <-job.Ctx.Done():
			case job.ResultCh <- Result{Err: p.workerRecovery(r)}:
			default:
			}
		}
	}()

	res, err := job.Fn(job.Ctx)

	select {
	case <-job.Ctx.Done():
	case job.ResultCh <- Result{Val: res, Err: err}:
	default:
	}
}

func (p *Pool) RunContext(ctx context.Context, fn Workload) (any, error) {
	if p.isClosed.Load() == 1 {
		return nil, ErrClosing
	}

	job := Job{
		Fn:       fn,
		Ctx:      ctx,
		ResultCh: make(chan Result, 1),
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.closeCh:
		return nil, ErrClosing
	case p.jobs <- &job:
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.closeCh:
			return nil, ErrClosing
		case r := <-job.ResultCh:
			return r.Val, r.Err
		}
	}
}

func (p *Pool) Run(fn Workload) (any, error) {
	return p.RunContext(context.Background(), fn)
}

func (p *Pool) Size() int { return p.size }

func (p *Pool) Close() {
	if !p.isClosed.CompareAndSwap(0, 1) {
		return
	}
	close(p.closeCh)
	p.wg.Wait()
}

func New(size int, backlog int) *Pool {
	if size <= 0 {
		size = runtime.NumCPU()
	}
	if backlog <= 0 {
		backlog = 1
	}

	p := &Pool{
		size:    size,
		closeCh: make(chan void),
		jobs:    make(chan *Job, backlog),
		sem:     make(chan void, size),
	}
	p.workersRun()
	return p
}
