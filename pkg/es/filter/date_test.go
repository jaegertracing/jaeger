// Copyright (c) 2021 The Jaeger Authors.
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

package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/es/client"
)

func TestByDate(t *testing.T) {
	beforeDateFilter := time.Date(2021, 10, 10, 12, 0o0, 0o0, 0o0, time.Local)
	expectedIndices := []client.Index{
		{
			Index:        "jaeger-span-0006",
			CreationTime: time.Date(2021, 7, 7, 7, 10, 10, 10, time.Local),
		},
		{
			Index:        "jaeger-span-0004",
			CreationTime: time.Date(2021, 9, 16, 11, 0o0, 0o0, 0o0, time.Local),
			Aliases: map[string]bool{
				"jaeger-span-other": true,
			},
		},
		{
			Index:        "jaeger-span-0005",
			CreationTime: time.Date(2021, 10, 10, 9, 56, 34, 25, time.Local),
			Aliases: map[string]bool{
				"custom-alias": true,
			},
		},
	}
	indices := []client.Index{
		{
			Index:        "jaeger-span-0006",
			CreationTime: time.Date(2021, 7, 7, 7, 10, 10, 10, time.Local),
		},
		{
			Index:        "jaeger-span-0004",
			CreationTime: time.Date(2021, 9, 16, 11, 0o0, 0o0, 0o0, time.Local),
			Aliases: map[string]bool{
				"jaeger-span-other": true,
			},
		},
		{
			Index:        "jaeger-span-0005",
			CreationTime: time.Date(2021, 10, 10, 9, 56, 34, 25, time.Local),
			Aliases: map[string]bool{
				"custom-alias": true,
			},
		},
		{
			Index:        "jaeger-span-0001",
			CreationTime: time.Date(2021, 10, 10, 12, 0o0, 0o0, 0o0, time.Local),
			Aliases: map[string]bool{
				"jaeger-span-write": true,
				"jaeger-span-read":  true,
			},
		},
		{
			Index:        "jaeger-span-0002",
			CreationTime: time.Date(2021, 11, 10, 12, 30, 0o0, 0o0, time.Local),
			Aliases: map[string]bool{
				"jaeger-span-read": true,
			},
		},
		{
			Index:        "jaeger-span-0003",
			CreationTime: time.Date(2021, 12, 10, 2, 15, 20, 0o1, time.Local),
			Aliases: map[string]bool{
				"jaeger-span-read": true,
			},
		},
	}

	result := ByDate(indices, beforeDateFilter)
	assert.Equal(t, expectedIndices, result)
}
