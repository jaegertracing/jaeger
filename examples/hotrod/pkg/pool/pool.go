// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
	for range workers {
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
