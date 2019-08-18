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

package zipkin

import (
	"github.com/jaegertracing/jaeger/model"
)

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
