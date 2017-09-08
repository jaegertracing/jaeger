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
