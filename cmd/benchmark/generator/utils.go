// Copyright (c) 2020 The Jaeger Authors.
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

package generator

import (
	"encoding/binary"

	"github.com/google/uuid"

	"github.com/jaegertracing/jaeger/cmd/benchmark/generator/data"
	"github.com/jaegertracing/jaeger/model"
)

const wordSeparator = "_"

func generateTraceID() model.TraceID {
	id := uuid.New()
	traceID := model.TraceID{}
	traceID.High = binary.BigEndian.Uint64(id[:8])
	traceID.Low = binary.BigEndian.Uint64(id[8:])
	return traceID
}

func generateProcesses(num int, minTags int, template []*TagTemplate) []Process {
	processes := make([]Process, num)
	names := generateWords(num)
	for i, srvName := range names {
		processes[i].Process = &model.Process{
			ServiceName: srvName,
			Tags:        generateTagsFromPool(template, minTags),
		}
		processes[i].Id = uuid.New().String()
	}
	return processes
}

func generateWords(max int) []string {
	return generateRandStrings(data.Words, wordSeparator, max)
}

func generateRandStrings(pool []string, separator string, max int) []string {
	size := len(pool)
	tagKeys := make([]string, max)
	count := 0

	for i := 0; i < size && count < max; i, count = i+1, count+1 {
		tagKeys[count] = pool[i]
	}

	for {
		m := count
		for i := 0; i < m; i++ {
			prefix := tagKeys[i]
			for k := 0; k < size; k++ {
				key := prefix + separator + pool[k]
				count++
				if count >= max {
					return tagKeys
				}
				tagKeys[count] = key
			}
		}
	}
}
