// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
