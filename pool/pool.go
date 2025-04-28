package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

var (
	ErrClosing = fmt.Errorf("pool is closing")

	DefaultConfig = Config{
		Size:    runtime.NumCPU(),
		Backlog: 1,
	}
)

type (
	Pool struct {
		closeCh chan void
		jobs    chan *Job
		wg      sync.WaitGroup
		cfg     Config
	}

	Config struct {
		Size    int
		Backlog int
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
	p.wg.Add(p.cfg.Size)
	for range p.cfg.Size {
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
		case job := <-p.jobs:
			p.workerRunJob(job)
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

func (p *Pool) JobWithContext(ctx context.Context, fn Workload) *Job {
	return &Job{
		Fn:       fn,
		Ctx:      ctx,
		ResultCh: make(chan Result, 1),
	}
}

func (p *Pool) RunContext(ctx context.Context, fn Workload) (any, error) {
	job := p.JobWithContext(ctx, fn)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.closeCh:
		return nil, ErrClosing
	case p.jobs <- job:
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

func (p *Pool) Size() int           { return p.cfg.Size }
func (p *Pool) Backlog() int        { return p.cfg.Backlog }
func (p *Pool) JobsCh() chan<- *Job { return p.jobs }

func (p *Pool) Close() {
	close(p.closeCh)
	p.wg.Wait()
}

func New(c Config) *Pool {
	p := &Pool{
		cfg:     c,
		closeCh: make(chan void),
		jobs:    make(chan *Job, c.Backlog),
	}
	p.workersRun()
	return p
}
