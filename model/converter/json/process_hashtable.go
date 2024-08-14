// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"strconv"

	"github.com/jaegertracing/jaeger/model"
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
