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

package json

import (
	"strconv"

	"github.com/uber/jaeger/model"
)

type processHashtable struct {
	count     int
	processes map[uint64][]processKey
	extHash   func(*model.Process) uint64
}

type processKey struct {
	process *model.Process
	key     string
}

// getKey assigns a new unique string key to the process, or returns
// a previously assigned value if the process has already been seen.
func (ph *processHashtable) getKey(process *model.Process) string {
	if ph.processes == nil {
		ph.processes = make(map[uint64][]processKey)
	}
	hash := ph.hash(process)
	if keys, ok := ph.processes[hash]; ok {
		for _, k := range keys {
			if k.process.Equal(process) {
				return k.key
			}
		}
		key := ph.nextKey()
		keys = append(keys, processKey{process: process, key: key})
		ph.processes[hash] = keys
		return key
	}
	key := ph.nextKey()
	ph.processes[hash] = []processKey{{process: process, key: key}}
	return key
}

// getMapping returns the accumulated mapping of string keys to processes.
func (ph *processHashtable) getMapping() map[string]*model.Process {
	out := make(map[string]*model.Process)
	for _, keys := range ph.processes {
		for _, key := range keys {
			out[key.key] = key.process
		}
	}
	return out
}

func (ph *processHashtable) nextKey() string {
	ph.count++
	key := "p" + strconv.Itoa(ph.count)
	return key
}

func (ph processHashtable) hash(process *model.Process) uint64 {
	if ph.extHash != nil {
		// for testing collisions
		return ph.extHash(process)
	}
	hc, _ := model.HashCode(process)
	return hc
}
