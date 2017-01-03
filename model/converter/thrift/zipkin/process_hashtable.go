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

package zipkin

import "github.com/uber/jaeger/model"

type processHashtable struct {
	processes map[uint64][]*model.Process
	extHash   func(*model.Process) uint64
}

func newProcessHashtable() *processHashtable {
	processes := make(map[uint64][]*model.Process)
	return &processHashtable{processes: processes}
}

func (ph processHashtable) hash(process *model.Process) uint64 {
	if ph.extHash != nil {
		// for testing collisions
		return ph.extHash(process)
	}
	hc, _ := model.HashCode(process)
	return hc
}

// add checks if identical Process already exists in the hash table and returns it.
// Otherwise it adds process to the table and returns it.
func (ph processHashtable) add(process *model.Process) *model.Process {
	hash := ph.hash(process)
	if pp, ok := ph.processes[hash]; ok {
		for _, p := range pp {
			if p.Equal(process) {
				return p // reuse existing Process object
			}
		}
		pp = append(pp, process)
		ph.processes[hash] = pp
	} else {
		ph.processes[hash] = []*model.Process{process}
	}
	return process
}
