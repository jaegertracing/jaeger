// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	if p.stop != nil {
		close(p.stop)
	}
}
