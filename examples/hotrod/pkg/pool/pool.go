package pool

// Pool is a simple worker pool
type Pool struct {
	jobs chan func()
	stop chan struct{}
}

// New creates a new pool with the given number of workers
func New(workers int) *Pool {
	jobs := make(chan func())
	stop := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			for {
				select {
				case job := <-jobs:
					job()
				case <-stop:
					return
				}
			}
		}()
	}
	return &Pool{
		jobs: jobs,
		stop: stop,
	}
}

// Execute enqueues the job to be executed by one of the workers in the pool
func (p *Pool) Execute(job func()) {
	p.jobs <- job
}

// Stop halts all the workers
func (p *Pool) Stop() {
	p.stop <- struct{}{}
}
